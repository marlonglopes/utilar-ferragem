package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ErrS3NotConfigured é o que o serviço devolve — no BOOT, não em runtime — se
// alguém pedir STORAGE_DRIVER=s3 antes do bucket existir.
var ErrS3NotConfigured = errors.New("storage: driver s3 ainda não implementado")

// S3 é o driver de PRODUÇÃO — decisão do dono: "o storage inicial será local
// para teste mas em produção será usado S3".
//
// ⚠️ NÃO IMPLEMENTADO DE PROPÓSITO. A conta AWS dedicada do Utilar ainda não
// existe (ver docs/aws-build-utilar.md), e não se inventa credencial nem se
// configura bucket às cegas. O que está pronto é a FRONTEIRA: este tipo
// satisfaz Store, o handler já chama só a interface, e nada no caminho de
// negócio muda quando o corpo for preenchido.
//
// O que falta, na ordem:
//
//  1. Conta AWS dedicada + bucket (`utilar-media-prod`), região sa-east-1.
//  2. Decidir o modelo de acesso. Recomendação: bucket PRIVADO com Block
//     Public Access ligado, servido por CloudFront com Origin Access Control.
//     Foto de produto é pública por natureza, mas bucket público é o vazamento
//     clássico: um `PutObject` errado numa chave errada e o bucket inteiro vira
//     hospedagem aberta. URL assinada não serve aqui (quebra cache de CDN e a
//     imagem do card precisa ser cacheável).
//  3. Política IAM mínima para o catalog-service: `s3:PutObject` e
//     `s3:DeleteObject` restritos ao prefixo `produtos/*`. Sem `s3:ListBucket`
//     e sem `GetObject` — quem escreve não precisa ler.
//  4. `MEDIA_BASE_URL` apontando para o domínio do CloudFront. Como o banco
//     guarda a CHAVE e não a URL, isso é a única mudança de dado necessária.
//  5. Dependência `github.com/aws/aws-sdk-go-v2/service/s3` (ainda não está no
//     go.mod — entra só quando houver o que testar contra).
//  6. `Cache-Control: public, max-age=31536000, immutable` no PutObject. A
//     chave contém o hash do conteúdo, então o objeto é imutável por
//     construção e pode ser cacheado para sempre.
//  7. Ciclo de vida: expirar objetos órfãos (imagem apagada do banco cuja
//     chave o Delete não alcançou) após 30 dias.
type S3 struct {
	Bucket   string
	Region   string
	Prefix   PrefixResolver
	KeyStyle string
}

func (s *S3) Driver() string        { return "s3" }
func (s *S3) URL(key string) string { return s.Prefix.URL(key) }

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) error {
	return ErrS3NotConfigured
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return ErrS3NotConfigured
}

// New escolhe o driver por configuração. É o único ponto do serviço que sabe
// que existe mais de um.
//
// Fail-closed: pedir `s3` hoje faz o serviço NÃO SUBIR, com a mensagem
// apontando a lista acima. O contrário — cair silenciosamente pro disco local
// em produção — daria um catálogo cujas fotos somem no próximo deploy, e
// ninguém descobriria até um cliente reclamar.
func New(driver, localRoot, baseURL string) (Store, error) {
	switch driver {
	case "", "local":
		return NewLocal(localRoot, baseURL)
	case "s3":
		return nil, fmt.Errorf("%w (ver internal/storage/s3.go para o que falta)", ErrS3NotConfigured)
	default:
		return nil, fmt.Errorf("storage: STORAGE_DRIVER=%q desconhecido (use local|s3)", driver)
	}
}

// FromEnv monta o Store a partir do ambiente. Defaults servem o `make dev`
// sem nenhuma variável configurada.
func FromEnv() (Store, error) {
	return New(
		os.Getenv("STORAGE_DRIVER"),
		envOr("MEDIA_ROOT", "./data/media"),
		envOr("MEDIA_BASE_URL", "/media"),
	)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
