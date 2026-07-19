package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"
)

// --- fixtures --------------------------------------------------------------

// makeJPEG gera um JPEG w×h com um retângulo colorido bem no meio, cercado de
// branco. O retângulo é o "produto": é a forma dele que os testes usam pra
// provar que nada foi esticado nem cortado.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	return encodeJPEG(t, drawSubject(w, h))
}

func drawSubject(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Produto = um bloco vermelho ocupando a metade central de cada eixo.
	for y := h / 4; y < 3*h/4; y++ {
		for x := w / 4; x < 3*w/4; x++ {
			img.Set(x, y, color.RGBA{220, 30, 30, 255})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b.Bytes()
}

func normalizeOK(t *testing.T, src []byte) *Result {
	t.Helper()
	res, err := Normalize(context.Background(), src, DefaultLimits())
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return res
}

func rendition(t *testing.T, res *Result, name string) Rendition {
	t.Helper()
	for _, r := range res.Renditions {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("variante %q não foi gerada", name)
	return Rendition{}
}

// --- proporção e não-deformação --------------------------------------------

// A regra central da feature: TODA imagem sai quadrada, seja qual for a
// proporção da original. É o que permite a grade da vitrine não pular e o
// carrossel não mudar de altura entre slides.
func TestNormalize_TodaSaidaEQuadradaIndependenteDaEntrada(t *testing.T) {
	casos := []struct {
		nome string
		w, h int
	}{
		{"paisagem 16:9", 1200, 675},
		{"retrato 9:16", 675, 1200},
		{"quadrada", 900, 900},
		{"panorâmica extrema", 1400, 240},
		{"coluna extrema", 240, 1400},
		{"quase quadrada", 801, 799},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			res := normalizeOK(t, makeJPEG(t, tc.w, tc.h))
			if len(res.Renditions) == 0 {
				t.Fatal("nenhuma variante gerada")
			}
			for _, r := range res.Renditions {
				if r.Width != r.Height {
					t.Errorf("variante %s saiu %dx%d; a saída tem que ser 1:1",
						r.Name, r.Width, r.Height)
				}
				// Confere no PIXEL, não só no metadado: o campo poderia estar
				// certo e o encode errado.
				cfg, _, err := image.DecodeConfig(bytes.NewReader(r.Bytes))
				if err != nil {
					t.Fatalf("variante %s não decodifica: %v", r.Name, err)
				}
				if cfg.Width != r.Width || cfg.Height != r.Height {
					t.Errorf("variante %s: metadado diz %dx%d, pixels dizem %dx%d",
						r.Name, r.Width, r.Height, cfg.Width, cfg.Height)
				}
			}
		})
	}
}

// Regressão do modo de falha que o dono nomeou: "foto 16:9 de uma furadeira
// esticada para quadrado fica ridícula".
//
// O teste mede a PROPORÇÃO DO OBJETO dentro da tela. Na origem o bloco vermelho
// é metade da largura por metade da altura, logo tem a mesma razão w/h da
// imagem. Se tivéssemos esticado até preencher o quadrado, o bloco sairia
// quadrado (razão 1). Como fazemos letterbox, a razão do bloco tem que ser
// PRESERVADA.
func TestNormalize_NuncaDeformaOProduto(t *testing.T) {
	casos := []struct {
		nome     string
		w, h     int
		wantAspc float64
	}{
		{"paisagem 2:1", 1200, 600, 2.0},
		{"retrato 1:2", 600, 1200, 0.5},
		{"paisagem 16:9", 1200, 675, 1200.0 / 675.0},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			res := normalizeOK(t, makeJPEG(t, tc.w, tc.h))
			img := decodeRendition(t, rendition(t, res, "medium"))

			bw, bh := subjectBounds(img)
			if bw == 0 || bh == 0 {
				t.Fatal("não achei o produto na imagem normalizada")
			}
			got := float64(bw) / float64(bh)
			// 8% de folga: reamostragem move a borda do bloco em 1-2px.
			if got < tc.wantAspc*0.92 || got > tc.wantAspc*1.08 {
				t.Errorf("proporção do produto = %.3f, queria ~%.3f (%dx%d na saída) — "+
					"a imagem foi DEFORMADA", got, tc.wantAspc, bw, bh)
			}
		})
	}
}

// A outra metade da regra: `cover` cego cortaria a ponta da ferramenta. Aqui a
// imagem de origem tem uma marca vermelha em CADA CANTO; se algum sumir, houve
// corte.
func TestNormalize_NuncaCortaAsExtremidades(t *testing.T) {
	const w, h = 1400, 490 // panorâmica: é onde `cover` cortaria mais
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.Set(x, y, color.White)
		}
	}
	// Marcas grossas nos 4 cantos (a "ponta da broca").
	const m = 42
	for _, c := range [][2]int{{0, 0}, {w - m, 0}, {0, h - m}, {w - m, h - m}} {
		for y := c[1]; y < c[1]+m; y++ {
			for x := c[0]; x < c[0]+m; x++ {
				src.Set(x, y, color.RGBA{220, 30, 30, 255})
			}
		}
	}

	res := normalizeOK(t, encodeJPEG(t, src))
	img := decodeRendition(t, rendition(t, res, "medium"))

	// A imagem escalada ocupa uma faixa horizontal centrada. Procura vermelho
	// perto de cada extremidade dessa faixa.
	x0, y0, x1, y1 := subjectBox(img)
	if x1 <= x0 || y1 <= y0 {
		t.Fatal("não achei nenhuma marca vermelha — a imagem inteira sumiu")
	}
	// As marcas estão nos extremos horizontais da origem; depois do `contain`
	// elas têm que encostar nas bordas laterais da tela quadrada.
	b := img.Bounds()
	if x0 > 4 || x1 < b.Dx()-5 {
		t.Errorf("marcas nos cantos em x=[%d,%d] numa tela de %d — as extremidades "+
			"foram CORTADAS (isso é `cover`, não `contain`)", x0, x1, b.Dx())
	}
}

// O fundo do letterbox é branco puro. Não é detalhe estético: em fundo preto
// (RGBA zerado, que é o default de uma tela nova) um PNG com transparência sai
// emoldurado de preto no meio de uma vitrine branca.
func TestNormalize_LetterboxTemFundoBrancoENaoPreto(t *testing.T) {
	// PNG com alpha zero fora do produto — o caso que exporia o bug.
	src := image.NewRGBA(image.Rect(0, 0, 1200, 400))
	for y := 150; y < 250; y++ {
		for x := 500; x < 700; x++ {
			src.Set(x, y, color.RGBA{0, 0, 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	res := normalizeOK(t, buf.Bytes())
	img := decodeRendition(t, rendition(t, res, "medium"))

	// O topo da tela é faixa de letterbox pura (a imagem é bem mais larga que
	// alta, então a faixa existe).
	r, g, b, _ := img.At(img.Bounds().Dx()/2, 3).RGBA()
	if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Errorf("faixa do letterbox = rgb(%d,%d,%d); tem que ser branca — "+
			"fundo escuro emoldura o produto na vitrine", r>>8, g>>8, b>>8)
	}
}

// --- múltiplas resoluções e peso -------------------------------------------

// O card da vitrine não pode baixar a imagem de zoom. Este teste trava a
// existência das três resoluções E a relação de peso entre elas.
func TestNormalize_GeraTresResolucoesEAMiniaturaEMuitoMaisLeve(t *testing.T) {
	res := normalizeOK(t, makeJPEG(t, 2400, 1800))

	if len(res.Renditions) != 3 {
		t.Fatalf("geradas %d variantes, queria 3 (thumb/medium/large)", len(res.Renditions))
	}
	thumb := rendition(t, res, "thumb")
	medium := rendition(t, res, "medium")
	large := rendition(t, res, "large")

	if thumb.Width != 300 || medium.Width != 800 || large.Width != 1600 {
		t.Errorf("lados = %d/%d/%d, queria 300/800/1600",
			thumb.Width, medium.Width, large.Width)
	}
	// Se a miniatura não for MUITO menor, ela não resolve o problema que
	// justificou existir (vitrine travando no celular).
	if len(thumb.Bytes)*4 > len(large.Bytes) {
		t.Errorf("thumb=%d bytes vs large=%d — a miniatura tinha que ser ao menos "+
			"4x mais leve, senão o card continua pesando", len(thumb.Bytes), len(large.Bytes))
	}
	if len(thumb.Bytes) >= len(medium.Bytes) || len(medium.Bytes) >= len(large.Bytes) {
		t.Errorf("peso não cresce com o tamanho: %d/%d/%d",
			len(thumb.Bytes), len(medium.Bytes), len(large.Bytes))
	}
	for _, r := range res.Renditions {
		if r.ContentType != "image/jpeg" || r.Ext != "jpg" {
			t.Errorf("variante %s: content-type=%q ext=%q", r.Name, r.ContentType, r.Ext)
		}
	}
}

// Nunca inventamos pixel: origem menor que o alvo não vira variante ampliada,
// vira ALIAS pra maior que existe. O contrato com o frontend (as três chaves
// sempre presentes) continua valendo.
func TestNormalize_NaoAmpliaOrigemPequenaEUsaAlias(t *testing.T) {
	res := normalizeOK(t, makeJPEG(t, 500, 400))

	if len(res.Renditions) != 1 {
		t.Fatalf("origem de 500px gerou %d variantes; só o thumb deveria ser gerado",
			len(res.Renditions))
	}
	for _, name := range []string{"thumb", "medium", "large"} {
		target, ok := res.Aliases[name]
		if !ok || target == "" {
			t.Errorf("variante %q sem alias — o frontend receberia URL vazia", name)
		}
	}
	if res.Aliases["large"] != "thumb" || res.Aliases["medium"] != "thumb" {
		t.Errorf("aliases = %v; medium e large deviam apontar pro thumb", res.Aliases)
	}
}

// --- EXIF ------------------------------------------------------------------

// Foto de celular vem deitada com um EXIF dizendo "gire". Quem ignora o campo
// publica metade do catálogo de lado.
func TestNormalize_RespeitaRotacaoEXIF(t *testing.T) {
	// Origem 400x800 (retrato) com orientation=6 ("gire 90° horário"), ou seja:
	// depois de aplicada, a imagem correta é 800x400 (paisagem).
	base := drawSubject(400, 800)
	// Uma marca só no topo-esquerdo, pra saber onde ela foi parar.
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			base.Set(x, y, color.RGBA{0, 200, 0, 255})
		}
	}
	src := withEXIFOrientation(t, encodeJPEG(t, base), 6)

	res := normalizeOK(t, src)
	if res.Orientation != 6 {
		t.Fatalf("orientation lida = %d, queria 6 — o EXIF não foi interpretado",
			res.Orientation)
	}
	// Depois da rotação a origem é 800 de largura por 400 de altura.
	if res.SourceWidth != 800 || res.SourceHeight != 400 {
		t.Errorf("dimensões pós-rotação = %dx%d, queria 800x400 — a foto ficaria deitada",
			res.SourceWidth, res.SourceHeight)
	}
}

// PORQUÊ este teste existe e é de SEGURANÇA, não de qualidade: o EXIF de
// celular carrega GPS. Publicar a coordenada de onde a foto foi tirada — a casa
// de quem fotografou, o depósito da loja — é vazamento silencioso, e ninguém
// percebe olhando a imagem.
func TestNormalize_RemoveMetadadosEXIFDaSaida(t *testing.T) {
	src := withEXIFOrientation(t, makeJPEG(t, 1200, 900), 3)
	if !bytes.Contains(src, []byte("Exif")) {
		t.Fatal("fixture inválida: a origem deveria conter EXIF")
	}

	res := normalizeOK(t, src)
	for _, r := range res.Renditions {
		if bytes.Contains(r.Bytes, []byte("Exif")) {
			t.Errorf("variante %s ainda contém o bloco Exif", r.Name)
		}
		// APP1 (0xFFE1) é o segmento que carrega EXIF/XMP. Nenhuma saída nossa
		// deve ter um: image/jpeg escreve só os segmentos que ele mesmo gera.
		if hasJPEGSegment(r.Bytes, 0xE1) {
			t.Errorf("variante %s tem segmento APP1 — pode conter GPS", r.Name)
		}
		// APP13 = IPTC/Photoshop, outra fonte comum de metadado pessoal.
		if hasJPEGSegment(r.Bytes, 0xED) {
			t.Errorf("variante %s tem segmento APP13 (IPTC)", r.Name)
		}
	}
}

// --- recusas de segurança --------------------------------------------------

// Nunca confiar em extensão nem em Content-Type. O que decide é o byte.
func TestNormalize_RecusaArquivoQueNaoEImagem(t *testing.T) {
	casos := []struct {
		nome string
		data []byte
	}{
		{"script php disfarçado", []byte("<?php system($_GET['c']); ?>")},
		{"html com script", []byte("<html><script>alert(1)</script></html>")},
		{"svg (é XML, executa script)", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"elf", []byte{0x7F, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0}},
		{"zip", []byte{'P', 'K', 0x03, 0x04, 0, 0, 0, 0, 0, 0}},
		{"vazio", []byte{}},
		{"texto", []byte(strings.Repeat("plain text ", 40))},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if _, err := Normalize(context.Background(), tc.data, DefaultLimits()); err == nil {
				t.Fatal("aceitou um arquivo que não é imagem")
			}
		})
	}
}

// Polyglot: JPEG estruturalmente VÁLIDO com payload enfiado num comentário.
// Passa no magic number de propósito — é aqui que se prova que a defesa real é
// a RECODIFICAÇÃO, não o sniff.
func TestNormalize_RecodificaEDescartaPayloadEmbutidoEmJPEGValido(t *testing.T) {
	payload := []byte("<?php system($_GET['cmd']); ?>")
	src := withJPEGComment(t, makeJPEG(t, 1200, 900), payload)

	if !bytes.Contains(src, payload) {
		t.Fatal("fixture inválida: o payload deveria estar no arquivo de origem")
	}
	if _, ok := Sniff(src); !ok {
		t.Fatal("fixture inválida: o polyglot deveria passar no sniff")
	}

	res := normalizeOK(t, src)
	for _, r := range res.Renditions {
		if bytes.Contains(r.Bytes, payload) {
			t.Errorf("variante %s ainda carrega o payload — os bytes originais "+
				"chegaram à saída em vez de serem recodificados", r.Name)
		}
	}
}

// Bomba de descompressão: arquivo pequeno que declara um bitmap gigante.
// 30.000×30.000 em RGBA são ~3,6 GB. Se a checagem viesse depois do decode,
// o processo morreria antes de poder recusar.
func TestNormalize_RecusaBombaDeDescompressao(t *testing.T) {
	// PNG de 25.000×25.000 de uma cor só: comprime pra poucos KB.
	bomba := hugePNGHeader(t, 25_000, 25_000)
	if len(bomba) > 200<<10 {
		t.Fatalf("fixture com %d bytes; a graça da bomba é ser pequena", len(bomba))
	}

	start := time.Now()
	_, err := Normalize(context.Background(), bomba, DefaultLimits())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("aceitou uma imagem de 625 megapixels")
	}
	// A recusa vem do CABEÇALHO. Se demorou, é porque decodificou primeiro —
	// e aí a memória já tinha ido embora.
	if elapsed > 2*time.Second {
		t.Errorf("levou %v para recusar; a checagem tem que ser no cabeçalho, "+
			"antes de materializar o bitmap", elapsed)
	}
}

func TestNormalize_RecusaDimensaoAbsurdaPorLado(t *testing.T) {
	// 20.000 × 10: pouco pixel no total, mas um lado patológico.
	lim := DefaultLimits()
	src := hugePNGHeader(t, 20_000, 10)
	_, err := Normalize(context.Background(), src, lim)
	if err == nil {
		t.Fatal("aceitou lado de 20.000px")
	}
}

func TestNormalize_RecusaArquivoAcimaDoLimiteDeBytes(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxBytes = 1024
	if _, err := Normalize(context.Background(), makeJPEG(t, 800, 800), lim); err == nil {
		t.Fatal("aceitou arquivo acima de MaxBytes")
	}
}

func TestNormalize_RecusaImagemPequenaDemais(t *testing.T) {
	if _, err := Normalize(context.Background(), makeJPEG(t, 60, 40), DefaultLimits()); err == nil {
		t.Fatal("aceitou imagem de 60px — não dá nem pro thumb")
	}
}

func TestNormalize_RecusaJPEGTruncado(t *testing.T) {
	full := makeJPEG(t, 1000, 1000)
	if _, err := Normalize(context.Background(), full[:len(full)/3], DefaultLimits()); err == nil {
		t.Fatal("aceitou JPEG truncado")
	}
}

// Imagem patológica não pode segurar uma goroutine para sempre. Com timeout
// zerado o processamento tem que desistir em vez de bloquear.
func TestNormalize_RespeitaTimeout(t *testing.T) {
	lim := DefaultLimits()
	lim.Timeout = time.Nanosecond

	done := make(chan error, 1)
	go func() {
		_, err := Normalize(context.Background(), makeJPEG(t, 1800, 1800), lim)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("nenhum erro com timeout de 1ns")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Normalize ficou pendurada — o timeout não interrompe o handler")
	}
}

// Checksum é o que deduplica e o que torna o reprocessamento idempotente.
func TestNormalize_ChecksumEstavelEDistinto(t *testing.T) {
	a := makeJPEG(t, 900, 600)
	b := makeJPEG(t, 900, 601)

	r1 := normalizeOK(t, a)
	r2 := normalizeOK(t, a)
	r3 := normalizeOK(t, b)

	if r1.Checksum != r2.Checksum {
		t.Error("mesmo arquivo gerou checksums diferentes — dedupe não funcionaria")
	}
	if r1.Checksum == r3.Checksum {
		t.Error("arquivos diferentes colidiram no checksum")
	}
	if len(r1.Checksum) != 64 {
		t.Errorf("checksum com %d caracteres, queria 64 (sha256 hex)", len(r1.Checksum))
	}
}

// --- Sniff -----------------------------------------------------------------

func TestSniff(t *testing.T) {
	casos := []struct {
		nome   string
		data   []byte
		want   Format
		wantOK bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, FormatJPEG, true},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0}, FormatPNG, true},
		{"gif87a", []byte("GIF87a__"), FormatGIF, true},
		{"gif89a", []byte("GIF89a__"), FormatGIF, true},
		{"webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), 'V', 'P'), FormatWebP, true},
		// RIFF sozinho é WAV/AVI. Aceitar só por "RIFF" deixaria áudio entrar
		// como imagem.
		{"wav (RIFF mas não WEBP)", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), "", false},
		{"svg", []byte("<svg xmlns="), "", false},
		{"bmp", []byte("BM\x00\x00\x00\x00\x00\x00"), "", false},
		{"curto", []byte{0xFF}, "", false},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			got, ok := Sniff(tc.data)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("Sniff = (%q,%v), queria (%q,%v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// EXIF forjado não pode virar panic: os offsets vêm do arquivo, e indexar sem
// checar é o jeito clássico de derrubar o serviço com um upload.
func TestExifOrientation_NaoQuebraComEntradaMaliciosa(t *testing.T) {
	casos := [][]byte{
		{0xFF, 0xD8},
		{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 'E', 'x', 'i', 'f', 0, 0},
		{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 0xFF, 0xFF, 0xFF, 0xFF},
		{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x0A, 'E', 'x', 'i', 'f', 0, 0, 'M', 'M'},
		append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x30, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 0xFF, 0xFF}, make([]byte, 30)...),
	}
	for i, c := range casos {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("caso %d: panic com EXIF forjado: %v", i, r)
				}
			}()
			if got := exifOrientation(c); got < 1 || got > 8 {
				t.Errorf("caso %d: orientation fora de 1..8: %d", i, got)
			}
		}()
	}
}

// --- helpers de teste ------------------------------------------------------

func decodeRendition(t *testing.T, r Rendition) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(r.Bytes))
	if err != nil {
		t.Fatalf("decode da variante %s: %v", r.Name, err)
	}
	return img
}

// isRed reconhece o bloco "produto" das fixtures mesmo depois da perda do JPEG.
func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 > 140 && g>>8 < 110 && b>>8 < 110
}

// subjectBox devolve o retângulo que contém todos os pixels do produto.
func subjectBox(img image.Image) (x0, y0, x1, y1 int) {
	b := img.Bounds()
	x0, y0 = b.Dx(), b.Dy()
	x1, y1 = -1, -1
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if isRed(img.At(b.Min.X+x, b.Min.Y+y)) {
				if x < x0 {
					x0 = x
				}
				if x > x1 {
					x1 = x
				}
				if y < y0 {
					y0 = y
				}
				if y > y1 {
					y1 = y
				}
			}
		}
	}
	return
}

func subjectBounds(img image.Image) (w, h int) {
	x0, y0, x1, y1 := subjectBox(img)
	if x1 < x0 || y1 < y0 {
		return 0, 0
	}
	return x1 - x0 + 1, y1 - y0 + 1
}

// hasJPEGSegment procura um marcador APPn específico varrendo os segmentos.
func hasJPEGSegment(b []byte, marker byte) bool {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return false
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return false
		}
		m := b[i+1]
		if m == 0xDA || m == 0xD9 {
			return false
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if m == marker {
			return true
		}
		if size < 2 {
			return false
		}
		i += 2 + size
	}
	return false
}

// withEXIFOrientation injeta um APP1/Exif mínimo com a tag Orientation logo
// depois do SOI.
func withEXIFOrientation(t *testing.T, jpg []byte, orientation uint16) []byte {
	t.Helper()

	// TIFF little-endian: header (8) + contagem (2) + 1 entrada (12) + next (4)
	tiff := make([]byte, 0, 26)
	tiff = append(tiff, 'I', 'I', 42, 0)
	tiff = append(tiff, 8, 0, 0, 0) // offset do IFD0
	tiff = append(tiff, 1, 0)       // 1 entrada
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry[0:2], 0x0112) // tag Orientation
	binary.LittleEndian.PutUint16(entry[2:4], 3)      // tipo SHORT
	binary.LittleEndian.PutUint32(entry[4:8], 1)      // 1 valor
	binary.LittleEndian.PutUint16(entry[8:10], orientation)
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0) // sem próximo IFD

	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := make([]byte, 0, len(payload)+4)
	seg = append(seg, 0xFF, 0xE1)
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(payload)+2))
	seg = append(seg, size...)
	seg = append(seg, payload...)

	out := make([]byte, 0, len(jpg)+len(seg))
	out = append(out, jpg[:2]...) // SOI
	out = append(out, seg...)
	out = append(out, jpg[2:]...)
	return out
}

// withJPEGComment enfia bytes arbitrários num segmento COM (0xFFFE) — o
// polyglot clássico. O arquivo continua um JPEG perfeitamente válido.
func withJPEGComment(t *testing.T, jpg, payload []byte) []byte {
	t.Helper()
	seg := make([]byte, 0, len(payload)+4)
	seg = append(seg, 0xFF, 0xFE)
	size := make([]byte, 2)
	binary.BigEndian.PutUint16(size, uint16(len(payload)+2))
	seg = append(seg, size...)
	seg = append(seg, payload...)

	out := make([]byte, 0, len(jpg)+len(seg))
	out = append(out, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, jpg[2:]...)
	return out
}

// hugePNGHeader gera um PNG REAL de w×h de uma cor só. Comprime a quase nada —
// é exatamente a propriedade que faz a bomba de descompressão funcionar.
func hugePNGHeader(t *testing.T, w, h int) []byte {
	t.Helper()
	// Monta o PNG à mão: alocar 25.000×25.000 de RGBA no teste seria a própria
	// bomba que estamos tentando não detonar.
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(h))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor
	writePNGChunk(&out, "IHDR", ihdr)
	// Sem IDAT válido: o DecodeConfig já leu as dimensões do IHDR, que é onde a
	// recusa tem que acontecer.
	writePNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writePNGChunk(w *bytes.Buffer, typ string, data []byte) {
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	w.Write(length)
	body := append([]byte(typ), data...)
	w.Write(body)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, pngCRC(body))
	w.Write(crc)
}

var crcTable [256]uint32

func init() {
	for n := 0; n < 256; n++ {
		c := uint32(n)
		for k := 0; k < 8; k++ {
			if c&1 != 0 {
				c = 0xEDB88320 ^ (c >> 1)
			} else {
				c >>= 1
			}
		}
		crcTable[n] = c
	}
}

func pngCRC(b []byte) uint32 {
	c := uint32(0xFFFFFFFF)
	for _, x := range b {
		c = crcTable[(c^uint32(x))&0xFF] ^ (c >> 8)
	}
	return c ^ 0xFFFFFFFF
}
