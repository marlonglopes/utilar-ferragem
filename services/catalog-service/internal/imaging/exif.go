package imaging

import (
	"encoding/binary"
	"image"
)

// --- Orientação EXIF -------------------------------------------------------
//
// PORQUÊ isto existe: foto de celular quase sempre é gravada com o sensor na
// horizontal e um campo EXIF `Orientation` dizendo "gire 90°". Quem abre com
// image/jpeg da stdlib recebe a imagem DEITADA — a stdlib decodifica pixel, não
// interpreta metadado. Sem este passo, metade do catálogo entraria de lado.
//
// PORQUÊ escrevemos o parser em vez de puxar uma lib de EXIF: precisamos de UM
// campo (0x0112) de um único IFD. Uma dependência de EXIF completa traz um
// parser de formato hostil — exatamente o tipo de superfície que este arquivo
// existe pra reduzir.
//
// O metadado é LIDO aqui e some depois: Normalize reencoda com image/jpeg, que
// escreve só os segmentos que ele mesmo gera (SOI/DQT/SOF/DHT/SOS). Não há
// APP1, então não há EXIF — e portanto não há GPS. Publicar a coordenada de
// onde a foto foi tirada (a casa de quem fotografou, o depósito da loja) é
// vazamento silencioso, e a única defesa confiável é não copiar o bloco.

const (
	orientationNormal = 1
	orientationMax    = 8
)

// exifOrientation varre os segmentos do JPEG procurando o APP1/Exif e devolve
// o valor da tag Orientation. Qualquer coisa fora do esperado devolve 1
// ("normal") — falhar aqui é imagem torta, nunca erro de upload.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return orientationNormal
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return orientationNormal
		}
		marker := b[i+1]
		// SOS (0xDA) marca o início dos dados comprimidos: daqui pra frente não
		// há mais metadado a procurar.
		if marker == 0xDA || marker == 0xD9 {
			return orientationNormal
		}
		// Marcadores sem payload (RSTn, TEM).
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if size < 2 || i+2+size > len(b) {
			return orientationNormal
		}
		if marker == 0xE1 { // APP1
			seg := b[i+4 : i+2+size]
			if o, ok := parseExifOrientation(seg); ok {
				return o
			}
		}
		i += 2 + size
	}
	return orientationNormal
}

// parseExifOrientation lê o TIFF header embutido no segmento APP1 e procura a
// tag 0x0112 no IFD0.
func parseExifOrientation(seg []byte) (int, bool) {
	const hdr = "Exif\x00\x00"
	if len(seg) < len(hdr)+8 || string(seg[:len(hdr)]) != hdr {
		return 0, false
	}
	tiff := seg[len(hdr):]

	var bo binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		bo = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		bo = binary.BigEndian
	default:
		return 0, false
	}
	if bo.Uint16(tiff[2:4]) != 42 {
		return 0, false
	}

	off := int(bo.Uint32(tiff[4:8]))
	// Offsets vêm do arquivo: qualquer aritmética aqui precisa ser checada
	// contra o tamanho real antes de indexar, ou um EXIF forjado vira panic.
	if off < 8 || off+2 > len(tiff) {
		return 0, false
	}
	n := int(bo.Uint16(tiff[off : off+2]))
	entries := tiff[off+2:]
	if n <= 0 || n*12 > len(entries) {
		return 0, false
	}
	for k := 0; k < n; k++ {
		e := entries[k*12 : k*12+12]
		if bo.Uint16(e[0:2]) != 0x0112 {
			continue
		}
		// Tipo SHORT (3), 1 valor: o dado cabe no próprio campo de offset e
		// mora nos 2 primeiros bytes dele.
		v := int(bo.Uint16(e[8:10]))
		if v >= orientationNormal && v <= orientationMax {
			return v, true
		}
		return 0, false
	}
	return 0, false
}

// applyOrientation devolve a imagem já na posição correta para exibição.
//
// Os 8 valores do EXIF combinam rotação e espelhamento. Espelhamento aparece em
// foto de câmera frontal; ignorá-lo devolveria o texto da embalagem invertido.
func applyOrientation(src image.Image, orientation int) image.Image {
	if orientation <= orientationNormal || orientation > orientationMax {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	// 5..8 trocam os eixos, então a saída tem largura e altura invertidas.
	swap := orientation >= 5
	ow, oh := w, h
	if swap {
		ow, oh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nx, ny int
			switch orientation {
			case 2: // espelhado na horizontal
				nx, ny = w-1-x, y
			case 3: // 180°
				nx, ny = w-1-x, h-1-y
			case 4: // espelhado na vertical
				nx, ny = x, h-1-y
			case 5: // transposto
				nx, ny = y, x
			case 6: // 90° horário
				nx, ny = h-1-y, x
			case 7: // transverso
				nx, ny = h-1-y, w-1-x
			case 8: // 90° anti-horário
				nx, ny = y, w-1-x
			}
			dst.Set(nx, ny, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
