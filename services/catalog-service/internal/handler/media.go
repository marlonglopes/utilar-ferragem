package handler

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/storage"
)

// readAllLimited lê no máximo `limit` bytes e ERRA se houver mais.
//
// PORQUÊ não io.ReadAll: ReadAll cresce o buffer até o fim do stream, e o fim
// do stream é decidido pelo cliente. PORQUÊ não io.LimitReader puro: ele
// TRUNCA em silêncio, e uma imagem truncada é indistinguível de uma imagem
// corrompida — o lojista veria "arquivo corrompido" num JPEG perfeitamente
// válido, só grande demais. Lê limit+1 pra saber a diferença.
func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("arquivo excede %d bytes", limit)
	}
	return b, nil
}

// MediaHandler serve os objetos gravados pelo driver LOCAL.
//
// ⚠️ Existe só para desenvolvimento e teste. Em produção o driver é S3 e quem
// serve é o CloudFront — esta rota nem é registrada. Um serviço de aplicação
// servindo arquivo estático é desperdício de conexão e de CPU.
//
// PORQUÊ não gin.Static / http.FileServer:
//
//  1. Eles fazem sniffing de conteúdo: o Content-Type sai do que os bytes
//     parecem ser. Numa pasta de UPLOAD isso é o defeito exato que não podemos
//     ter — o servidor deve dizer o que NÓS gravamos, não adivinhar.
//  2. FileServer lista diretório quando não há index. Listar a pasta de mídia
//     entrega o catálogo inteiro, inclusive rascunho não publicado.
//  3. Eles resolvem o caminho por conta própria; aqui a chave passa pelo mesmo
//     storage.SafeKey que o caminho de escrita usa.
type MediaHandler struct {
	local *storage.Local
}

func NewMediaHandler(local *storage.Local) *MediaHandler {
	return &MediaHandler{local: local}
}

// contentTypeByExt é uma ALLOWLIST. Extensão fora dela é 404, não
// "application/octet-stream" — porque octet-stream ainda é um download, e um
// arquivo que não deveria existir na pasta não deve sair dela de jeito nenhum.
// Como toda a saída da normalização é JPEG, a lista tem um item só.
var contentTypeByExt = map[string]string{
	".jpg": "image/jpeg",
}

// Serve — GET /media/*path
func (h *MediaHandler) Serve(c *gin.Context) {
	key := strings.TrimPrefix(c.Param("path"), "/")
	if err := storage.SafeKey(key); err != nil {
		// 404 e não 400: dizer "chave inválida" confirma pro atacante que o
		// endpoint interpreta caminho. Mídia inexistente e mídia recusada
		// respondem igual.
		c.Status(http.StatusNotFound)
		return
	}
	ct, ok := contentTypeByExt[strings.ToLower(path.Ext(key))]
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := h.local.Read(key)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	// nosniff: sem ele o browser pode reinterpretar o conteúdo e ignorar o
	// Content-Type que acabamos de declarar — que é justamente o buraco pelo
	// qual um upload "de imagem" vira HTML executado na origem da loja.
	c.Header("X-Content-Type-Options", "nosniff")
	// inline + nome fixo: nunca ecoamos o nome enviado pelo cliente num header.
	c.Header("Content-Disposition", "inline")
	// A chave contém o hash do conteúdo, então o objeto é imutável: mudou a
	// foto, mudou a chave. Cache longo é seguro por construção.
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, ct, data)
}
