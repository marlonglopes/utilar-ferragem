package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local grava no disco. É o driver de DESENVOLVIMENTO e de ambiente de teste —
// em produção o driver é o S3 (ver s3.go). O contrato é idêntico, então trocar
// é `STORAGE_DRIVER=s3` e nada mais.
//
// Limitação conhecida e aceita: disco local não sobrevive a mais de uma
// instância do serviço nem a um container recriado. É exatamente por isso que
// não é o driver de produção.
type Local struct {
	root     string
	resolver PrefixResolver
}

// NewLocal cria o driver de disco. `root` é a pasta raiz, `baseURL` é o prefixo
// público (ex.: "/media").
func NewLocal(root, baseURL string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	return &Local{root: abs, resolver: PrefixResolver(baseURL)}, nil
}

func (l *Local) Driver() string        { return "local" }
func (l *Local) URL(key string) string { return l.resolver.URL(key) }
func (l *Local) Root() string          { return l.root }
func (l *Local) resolvePath(key string) (string, error) {
	if err := SafeKey(key); err != nil {
		return "", err
	}
	p := filepath.Join(l.root, filepath.FromSlash(key))
	// Cinto E suspensório: mesmo com SafeKey aprovando, confirmamos que o
	// caminho final ainda está DENTRO da raiz. SafeKey é análise de string;
	// esta é a verificação do resultado. Se um dia as duas discordarem, é a
	// segunda que evita a escrita fora da pasta.
	if !strings.HasPrefix(p, l.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: resolveu fora da raiz", ErrUnsafeKey)
	}
	return p, nil
}

func (l *Local) Put(ctx context.Context, key, contentType string, data []byte) error {
	p, err := l.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}

	// Escrita atômica: grava num temporário na MESMA pasta e renomeia. Sem
	// isso, um crash no meio do Write deixaria um JPEG truncado com a chave
	// definitiva — e como a chave é o hash, ele nunca seria reescrito. Meia
	// imagem cacheada para sempre.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op se o Rename deu certo

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 0644: legível por quem serve, NUNCA executável. Bit de execução em pasta
	// de upload é como um arquivo enviado vira código rodando.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}

func (l *Local) Delete(ctx context.Context, key string) error {
	p, err := l.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Read devolve os bytes de um objeto. Existe para o handler que serve /media
// no driver local — no S3 o objeto é servido pelo bucket/CDN e este método não
// é usado.
func (l *Local) Read(key string) ([]byte, error) {
	p, err := l.resolvePath(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p) // #nosec G304 — `p` passou por SafeKey + confinamento na raiz
}
