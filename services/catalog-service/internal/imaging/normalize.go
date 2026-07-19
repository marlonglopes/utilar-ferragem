// Package imaging normaliza a foto que o lojista sobe até um formato único e
// previsível de catálogo.
//
// # A decisão de proporção: 1:1 (quadrado), com letterbox branco
//
// Toda imagem sai QUADRADA, seja qual for a proporção da original. É o padrão
// de e-commerce de ferragem, e o motivo é a grade: a vitrine mostra 2 colunas
// no celular e 4 no desktop, e card de altura variável faz a página "pular"
// enquanto as fotos carregam. Quadrado também é o que o carrossel do detalhe
// precisa — sem ele cada slide muda de altura e o botão de comprar dança.
//
// # Nunca deformar, nunca cortar
//
// A imagem é escalada preservando a proporção (`contain`) e CENTRALIZADA numa
// tela quadrada branca. As duas alternativas foram descartadas de propósito:
//
//   - Esticar até o quadrado (`fill`): uma foto 16:9 de furadeira vira uma
//     furadeira gorda. O cliente vê um produto que não é o que vai receber.
//   - Cortar até preencher (`cover`): corta a ponta da broca, a extremidade da
//     trena, a rosca do parafuso — justamente a parte que o cliente foi olhar.
//     Em ferragem a silhueta do objeto É a informação.
//
// O custo do letterbox é a faixa branca. Em catálogo de ferragem isso não
// aparece: a foto de produto já vem em fundo branco, então a faixa se funde com
// o fundo do card. Por isso o fundo é branco puro (#FFFFFF) e não o laranja da
// marca — o neutro some, a cor da marca viraria moldura.
//
// # Segurança
//
// Toda imagem é RECODIFICADA a partir dos pixels decodificados. Nenhum byte do
// arquivo original chega ao disco. É isso — não a checagem de magic number —
// que neutraliza payload escondido dentro de um JPEG estruturalmente válido
// (polyglot JPEG/PHP, comentário com shell, EXIF com script). O sniff serve
// para recusar cedo e barato; a recodificação é a garantia.
package imaging

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decoder WebP (a stdlib não tem)
)

// Erros de recusa. São todos "culpa do cliente" (4xx) — o handler os traduz.
var (
	ErrNotAnImage    = errors.New("arquivo não é uma imagem suportada (jpeg, png, webp ou gif)")
	ErrTooLarge      = errors.New("arquivo excede o tamanho máximo permitido")
	ErrTooManyPixels = errors.New("imagem excede o número máximo de pixels")
	ErrDimensionHuge = errors.New("imagem excede a dimensão máxima por lado")
	ErrTooSmall      = errors.New("imagem pequena demais para o catálogo")
	ErrCorrupt       = errors.New("imagem corrompida ou truncada")
	ErrTimeout       = errors.New("processamento da imagem excedeu o tempo limite")
)

// Variant é um tamanho de saída. O lado é o mesmo nos dois eixos porque a saída
// é quadrada.
type Variant struct {
	Name    string
	Side    int
	Quality int
}

// DefaultVariants — três tamanhos, um por uso real:
//
//	thumb  300px — card da vitrine e miniatura do carrossel.
//	medium 800px — slide do carrossel no detalhe do produto.
//	large 1600px — zoom / tela cheia.
//
// PORQUÊ três e não uma: a vitrine do celular mostra ~20 cards. Servir a imagem
// de zoom nesses cards são ~20 × 400 KB = 8 MB para desenhar miniaturas de
// 150px na tela — é exatamente o que trava a vitrine em 4G. Com o thumb são
// ~20 × 25 KB.
//
// A qualidade do thumb é menor de propósito: em 300px o artefato de JPEG não é
// perceptível, e cada KB ali é multiplicado pelo número de cards da página.
var DefaultVariants = []Variant{
	{Name: "thumb", Side: 300, Quality: 78},
	{Name: "medium", Side: 800, Quality: 82},
	{Name: "large", Side: 1600, Quality: 85},
}

// Limits são as travas de recurso. Todas existem por um modo de falha concreto.
type Limits struct {
	// MaxBytes: teto do arquivo recebido.
	MaxBytes int64
	// MaxPixels: teto de LARGURA × ALTURA, checado no cabeçalho ANTES de
	// decodificar. É a defesa contra bomba de descompressão — um PNG de 40 KB
	// pode declarar 30.000×30.000, e decodificar isso são ~3,6 GB de RGBA.
	// O arquivo passaria folgado em MaxBytes; só a checagem de pixels pega.
	MaxPixels int64
	// MaxDimension: teto por lado. Pega o caso patológico de proporção
	// (1×100.000.000) que passaria no produto de pixels de alguns cenários.
	MaxDimension int
	// MinDimension: piso do lado maior. Abaixo disso não há pixel suficiente
	// nem para o thumb, e o resultado é um borrão que prejudica a venda.
	MinDimension int
	// Timeout por imagem. Imagem patológica (progressivo gigante, GIF com
	// milhares de quadros) não pode segurar uma goroutine para sempre.
	Timeout time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxBytes:     12 << 20, // 12 MB — foto de celular moderna cabe com folga
		MaxPixels:    50_000_000,
		MaxDimension: 12_000,
		MinDimension: 200,
		Timeout:      20 * time.Second,
	}
}

// Rendition é uma variante já codificada, pronta pra ir ao storage.
type Rendition struct {
	Name        string
	Width       int
	Height      int
	Bytes       []byte
	ContentType string
	Ext         string
}

// Result é o retorno de Normalize.
type Result struct {
	SourceFormat  Format
	SourceWidth   int
	SourceHeight  int
	OriginalBytes int
	// Checksum é o SHA-256 do arquivo ORIGINAL. Serve pra deduplicar (a mesma
	// foto subida duas vezes não vira dois objetos) e pra tornar a
	// reprocessagem idempotente.
	Checksum string
	// Orientation é o valor EXIF que foi aplicado (1 = já estava certa).
	Orientation int
	Renditions  []Rendition
	// Aliases mapeia variante pedida → variante realmente produzida, para os
	// casos em que a origem era menor que o tamanho alvo. Ver Normalize.
	Aliases map[string]string
}

// TotalBytes soma o peso de todas as variantes geradas.
func (r *Result) TotalBytes() int {
	n := 0
	for i := range r.Renditions {
		n += len(r.Renditions[i].Bytes)
	}
	return n
}

// Normalize é o pipeline inteiro: verifica, decodifica, endireita, enquadra,
// redimensiona e reencoda.
//
// A ordem importa. As checagens baratas e as que dependem só do CABEÇALHO vêm
// antes de qualquer alocação grande — checar dimensão depois de decodificar
// seria checar depois de já ter estourado a memória.
func Normalize(ctx context.Context, src []byte, lim Limits) (*Result, error) {
	if int64(len(src)) > lim.MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes (máximo %d)", ErrTooLarge, len(src), lim.MaxBytes)
	}
	if len(src) == 0 {
		return nil, ErrNotAnImage
	}

	format, ok := Sniff(src)
	if !ok {
		return nil, ErrNotAnImage
	}

	// DecodeConfig lê só o cabeçalho: descobre as dimensões sem materializar o
	// bitmap. É o que permite recusar a bomba de descompressão a custo zero.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return nil, ErrCorrupt
	}
	if cfg.Width > lim.MaxDimension || cfg.Height > lim.MaxDimension {
		return nil, fmt.Errorf("%w: %dx%d (máximo %d por lado)",
			ErrDimensionHuge, cfg.Width, cfg.Height, lim.MaxDimension)
	}
	if int64(cfg.Width)*int64(cfg.Height) > lim.MaxPixels {
		return nil, fmt.Errorf("%w: %dx%d (máximo %d)",
			ErrTooManyPixels, cfg.Width, cfg.Height, lim.MaxPixels)
	}
	if maxInt(cfg.Width, cfg.Height) < lim.MinDimension {
		return nil, fmt.Errorf("%w: %dx%d (mínimo %dpx no maior lado)",
			ErrTooSmall, cfg.Width, cfg.Height, lim.MinDimension)
	}

	// O trabalho pesado (decodificar + escalar + reencodar) roda numa goroutine
	// para que o timeout seja EFETIVO: sem isso um `ctx` cancelado não
	// interromperia um decoder já dentro do laço, e a requisição ficaria
	// pendurada junto. A goroutine órfã termina sozinha; o que não podemos é
	// deixar o handler preso a ela.
	work, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()

	type outcome struct {
		res *Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := process(src, format, lim)
		done <- outcome{res, err}
	}()

	select {
	case <-work.Done():
		return nil, ErrTimeout
	case o := <-done:
		return o.res, o.err
	}
}

func process(src []byte, format Format, lim Limits) (*Result, error) {
	img, err := decode(src, format)
	if err != nil {
		return nil, ErrCorrupt
	}

	// Endireita ANTES de enquadrar: uma foto retrato marcada como paisagem
	// precisa ser girada primeiro, senão o letterbox calcula a escala sobre os
	// eixos errados e a imagem sai com faixa nos lados errados.
	orientation := orientationNormal
	if format == FormatJPEG {
		orientation = exifOrientation(src)
		img = applyOrientation(img, orientation)
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, ErrCorrupt
	}
	longest := maxInt(w, h)

	sum := sha256.Sum256(src)
	res := &Result{
		SourceFormat:  format,
		SourceWidth:   w,
		SourceHeight:  h,
		OriginalBytes: len(src),
		Checksum:      hex.EncodeToString(sum[:]),
		Orientation:   orientation,
		Aliases:       map[string]string{},
	}

	// Nunca AMPLIAMOS pra inventar pixel: uma variante maior que a origem só
	// gastaria bytes exibindo o mesmo borrão. Quando a origem é menor que o
	// alvo, a variante não é gerada e passa a APONTAR para a maior que existe
	// (Aliases). O frontend continua recebendo as três chaves — a diferença
	// fica invisível pra ele, que é o ponto: contrato estável.
	//
	// O menor tamanho (thumb) é sempre gerado, mesmo que exija ampliar um
	// pouco: sem thumb a vitrine voltaria a baixar a imagem grande no card.
	var produced string
	for i, v := range DefaultVariants {
		if i > 0 && v.Side > longest {
			res.Aliases[v.Name] = produced
			continue
		}
		canvas := letterbox(img, v.Side)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: v.Quality}); err != nil {
			return nil, err
		}
		res.Renditions = append(res.Renditions, Rendition{
			Name:        v.Name,
			Width:       v.Side,
			Height:      v.Side,
			Bytes:       buf.Bytes(),
			ContentType: "image/jpeg",
			Ext:         "jpg",
		})
		produced = v.Name
		res.Aliases[v.Name] = v.Name
	}

	return res, nil
}

// decode usa o decoder do formato detectado, não o registry genérico.
//
// PORQUÊ explícito: image.Decode escolhe pelo magic number registrado, e
// qualquer import futuro (`_ "image/x"`) ampliaria silenciosamente o conjunto
// de formatos aceitos sem passar pelo Sniff. Aqui só decodifica o que o Sniff
// aprovou.
func decode(src []byte, format Format) (image.Image, error) {
	r := bytes.NewReader(src)
	switch format {
	case FormatJPEG:
		return jpeg.Decode(r)
	case FormatPNG:
		return png.Decode(r)
	case FormatGIF:
		// Só o primeiro quadro. Catálogo não tem GIF animado, e decodificar
		// todos os quadros é o vetor de esgotamento de memória do formato.
		return gif.Decode(r)
	case FormatWebP:
		img, _, err := image.Decode(r)
		return img, err
	}
	return nil, ErrNotAnImage
}

// letterbox devolve uma tela side×side branca com a imagem escalada por
// `contain` e centralizada.
//
// CatmullRom é o reamostrador usado na redução: bilinear puro em fator alto
// (2000px → 300px) faz aliasing na rosca do parafuso e na serrilha da lâmina,
// justamente a textura que identifica a peça.
func letterbox(src image.Image, side int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	// Fundo branco opaco. Precisa ser pintado ANTES: PNG com transparência
	// desenharia sobre preto (RGBA zerado) e o produto sairia num quadro preto.
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	// `contain`: a escala é a MENOR das duas, então o lado maior encosta na
	// borda e o menor sobra em branco. Usar a maior seria `cover` (corta).
	scale := float64(side) / float64(w)
	if s := float64(side) / float64(h); s < scale {
		scale = s
	}
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	offX := (side - nw) / 2
	offY := (side - nh) / 2
	target := image.Rect(offX, offY, offX+nw, offY+nh)

	xdraw.CatmullRom.Scale(dst, target, src, b, xdraw.Over, nil)
	return dst
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
