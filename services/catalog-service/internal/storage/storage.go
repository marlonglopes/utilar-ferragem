// Package storage é a fronteira entre o catálogo e onde os bytes da imagem
// realmente moram.
//
// # A regra que sustenta a migração pro S3
//
// O banco guarda a CHAVE LÓGICA do objeto (`produtos/<id>/<hash>-thumb.jpg`),
// nunca um caminho de disco nem uma URL absoluta. A URL pública é DERIVADA da
// chave na hora de responder, a partir de um prefixo configurável.
//
// PORQUÊ isso não é preciosismo: se `https://loja.local/media/...` fosse
// gravado em `product_images`, trocar disco→S3/CDN exigiria um UPDATE em toda
// a tabela — em produção, com o catálogo no ar. Com a chave gravada, a troca é
// uma variável de ambiente e zero linha de banco tocada.
//
// Por isso o handler nunca sabe qual driver está ativo. Ele chama Put/Delete e
// resolve URLs por URLResolver; `STORAGE_DRIVER=local|s3` decide o resto.
package storage

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// Store é o contrato mínimo que disco local e S3 conseguem cumprir igual.
type Store interface {
	// Put grava (ou sobrescreve) o objeto. Sobrescrever é intencional: a chave
	// deriva do hash do conteúdo, então regravar a mesma chave é regravar o
	// mesmo conteúdo — o que torna o reprocessamento idempotente.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Delete é idempotente: apagar o que não existe não é erro. O caminho de
	// exclusão de imagem não pode falhar porque o objeto já sumiu.
	Delete(ctx context.Context, key string) error
	// Driver identifica a implementação, pra log e /health.
	Driver() string
	URLResolver
}

// URLResolver transforma chave lógica em URL pública. Separado de Store porque
// quem SERVE o catálogo (ProductHandler) só precisa resolver URL — não deve nem
// conseguir gravar.
type URLResolver interface {
	URL(key string) string
}

// PrefixResolver monta a URL concatenando um prefixo. Cobre tanto o disco local
// (`/media`) quanto CDN (`https://cdn.utilar.com.br`).
type PrefixResolver string

func (p PrefixResolver) URL(key string) string {
	if key == "" {
		return ""
	}
	// Chave que já é URL absoluta passa intacta: é assim que as 288 imagens
	// legadas do Wikimedia continuam funcionando dentro do mesmo modelo.
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key
	}
	return strings.TrimRight(string(p), "/") + "/" + strings.TrimLeft(key, "/")
}

// ErrUnsafeKey é a recusa de qualquer chave que possa escapar do prefixo.
var ErrUnsafeKey = errors.New("storage: chave de objeto inválida")

// SafeKey valida a chave antes de ela virar caminho no disco (ou objeto no
// bucket).
//
// PORQUÊ existe: o nome que o cliente manda no multipart é texto arbitrário.
// `../../etc/passwd`, `..\..\config`, `/etc/cron.d/x` — o clássico. Nós nunca
// usamos o nome do cliente como chave (ele vira só metadado de exibição, ver
// SanitizeFilename), mas esta função é o cinto de segurança: se algum caminho
// de código futuro deixar entrada externa chegar aqui, ela para.
//
// A validação é por ALLOWLIST, não por lista de proibidos. Blocklist de path
// traversal é uma corrida perdida (`%2e%2e`, `....//`, unicode); allowlist de
// caracteres tem o inverso da propriedade: o que não foi previsto é recusado.
func SafeKey(key string) error {
	if key == "" || len(key) > 512 {
		return fmt.Errorf("%w: tamanho", ErrUnsafeKey)
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return fmt.Errorf("%w: caminho absoluto ou separador inválido", ErrUnsafeKey)
	}
	if key != path.Clean(key) {
		// path.Clean resolve "..", "." e "//". Se o resultado difere da
		// entrada, a entrada tinha travessia ou lixo — recusa em vez de usar a
		// versão limpa, porque "limpar" esconde a tentativa do log.
		return fmt.Errorf("%w: travessia de caminho", ErrUnsafeKey)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("%w: segmento vazio ou relativo", ErrUnsafeKey)
		}
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("%w: caractere %q não permitido", ErrUnsafeKey, r)
		}
	}
	return nil
}

// Key monta a chave canônica de uma variante.
//
// O nome vem SÓ de valores que nós controlamos: o UUID do produto, o hash do
// conteúdo e o nome da variante. Nada do que o cliente enviou entra aqui — nem
// o nome do arquivo, nem a extensão declarada. É a diferença entre "sanitizar
// a entrada do atacante" e "não usar a entrada do atacante".
func Key(productID, checksum, variant, ext string) string {
	short := checksum
	if len(short) > 16 {
		short = short[:16]
	}
	return fmt.Sprintf("produtos/%s/%s-%s.%s", productID, short, variant, ext)
}
