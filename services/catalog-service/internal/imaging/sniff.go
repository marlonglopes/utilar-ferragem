package imaging

import "bytes"

// Format é o formato REAL do arquivo, detectado pelos bytes.
//
// PORQUÊ não usar Content-Type nem extensão: os dois são texto que o cliente
// escreve. `foto.jpg` com `Content-Type: image/jpeg` pode ser um .php, um .svg
// com <script>, ou um zip. O único sinal que o atacante não controla de graça
// é o conteúdo — e mesmo esse só serve pra RECUSAR cedo; a garantia de verdade
// é a recodificação em Normalize, que descarta tudo que não for pixel.
type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWebP Format = "webp"
	FormatGIF  Format = "gif"
)

var (
	magicJPEG = []byte{0xFF, 0xD8, 0xFF}
	magicPNG  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	magicGIF7 = []byte("GIF87a")
	magicGIF9 = []byte("GIF89a")
	magicRIFF = []byte("RIFF")
	magicWEBP = []byte("WEBP")
)

// Sniff devolve o formato detectado pelos primeiros bytes. ok=false para
// qualquer coisa que não seja uma das imagens suportadas — incluindo formatos
// de imagem que recusamos de propósito:
//
//   - SVG: é XML, executa script no browser. Nunca entra num catálogo.
//   - TIFF/BMP/HEIC: sem decoder na stdlib; aceitar sem decodificar seria
//     servir bytes não-verificados.
func Sniff(b []byte) (Format, bool) {
	switch {
	case len(b) >= 3 && bytes.Equal(b[:3], magicJPEG):
		return FormatJPEG, true
	case len(b) >= 8 && bytes.Equal(b[:8], magicPNG):
		return FormatPNG, true
	case len(b) >= 6 && (bytes.Equal(b[:6], magicGIF7) || bytes.Equal(b[:6], magicGIF9)):
		return FormatGIF, true
	// WebP é um container RIFF: "RIFF" + 4 bytes de tamanho + "WEBP".
	// Checar só "RIFF" aceitaria WAV e AVI, que são o mesmo container.
	case len(b) >= 12 && bytes.Equal(b[:4], magicRIFF) && bytes.Equal(b[8:12], magicWEBP):
		return FormatWebP, true
	}
	return "", false
}
