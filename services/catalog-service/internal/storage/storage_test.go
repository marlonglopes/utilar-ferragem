package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O ataque clássico de upload: o nome do arquivo vira caminho no disco e
// `../../etc/passwd` escreve fora da pasta de mídia.
//
// Na prática não usamos o nome do cliente como chave (storage.Key gera a partir
// do UUID do produto e do hash do conteúdo) — este teste trava o CINTO DE
// SEGURANÇA, para que um caminho de código futuro que deixe entrada externa
// chegar aqui não abra o buraco.
func TestSafeKey_RecusaTravessiaDeCaminho(t *testing.T) {
	maliciosas := []string{
		"../../etc/passwd",
		"../etc/passwd",
		"produtos/../../etc/shadow",
		"/etc/passwd",
		"/absoluto/x.jpg",
		`..\..\windows\system32\config`,
		`produtos\..\..\x.jpg`,
		"produtos//x.jpg",
		"produtos/./x.jpg",
		"produtos/../x.jpg",
		"./x.jpg",
		"..",
		".",
		"",
		"produtos/x.jpg\x00.png", // NUL truncation
		"produtos/x jpg",         // espaço não está na allowlist
		"produtos/ção.jpg",       // fora do ASCII permitido
		"produtos/$(whoami).jpg",
		"produtos/x.jpg;rm -rf /",
		"produtos/\nfake-log-line",
		strings.Repeat("a/", 400) + "x.jpg", // acima de 512
	}
	for _, k := range maliciosas {
		t.Run(k, func(t *testing.T) {
			if err := SafeKey(k); err == nil {
				t.Errorf("SafeKey(%q) aceitou — escaparia da pasta de mídia", k)
			}
		})
	}
}

func TestSafeKey_AceitaChaveCanonica(t *testing.T) {
	boas := []string{
		"produtos/9f1c/ab12cd34-thumb.jpg",
		"produtos/9f1c8e2a-4b3d-11ee-be56-0242ac120002/deadbeef12345678-large.jpg",
		"a.jpg",
	}
	for _, k := range boas {
		if err := SafeKey(k); err != nil {
			t.Errorf("SafeKey(%q) recusou uma chave válida: %v", k, err)
		}
	}
}

// A chave é montada SÓ de valores que nós controlamos. Mesmo que o nome do
// arquivo do cliente fosse hostil, ele não participa da composição.
func TestKey_NaoUsaNadaDoCliente(t *testing.T) {
	const (
		productID = "9f1c8e2a-4b3d-11ee-be56-0242ac120002"
		checksum  = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	)
	k := Key(productID, checksum, "thumb", "jpg")
	if k != "produtos/"+productID+"/abcdef0123456789-thumb.jpg" {
		t.Fatalf("chave inesperada: %q", k)
	}
	if err := SafeKey(k); err != nil {
		t.Fatalf("a chave que nós mesmos geramos não passou no SafeKey: %v", err)
	}
}

// Escrever fora da raiz é a falha que este driver não pode ter, e a checagem é
// dupla (SafeKey + confinamento do caminho resolvido) de propósito.
func TestLocal_NaoEscreveForaDaRaiz(t *testing.T) {
	root := t.TempDir()
	l, err := NewLocal(filepath.Join(root, "media"), "/media")
	if err != nil {
		t.Fatal(err)
	}

	alvo := filepath.Join(root, "roubado.txt")
	for _, k := range []string{"../roubado.txt", "../../roubado.txt", "/tmp/roubado.txt"} {
		if err := l.Put(context.Background(), k, "image/jpeg", []byte("x")); err == nil {
			t.Errorf("Put(%q) não deu erro", k)
		}
	}
	if _, err := os.Stat(alvo); err == nil {
		t.Fatal("arquivo foi escrito FORA da raiz de mídia")
	}
}

func TestLocal_PutReadDeleteERoundTrip(t *testing.T) {
	l, err := NewLocal(filepath.Join(t.TempDir(), "media"), "/media")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "produtos/abc/dead-thumb.jpg"
	data := []byte{0xFF, 0xD8, 0xFF, 0x01, 0x02}

	if err := l.Put(ctx, key, "image/jpeg", data); err != nil {
		t.Fatal(err)
	}
	got, err := l.Read(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Error("bytes lidos diferem dos gravados")
	}

	// Nunca executável: bit de execução em pasta de upload é como um arquivo
	// enviado vira código rodando.
	info, err := os.Stat(filepath.Join(l.Root(), filepath.FromSlash(key)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Errorf("objeto gravado com permissão %v — não pode ser executável", info.Mode().Perm())
	}

	if err := l.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Read(key); err == nil {
		t.Error("Read funcionou depois do Delete")
	}
	// Delete é idempotente: o caminho de exclusão de imagem não pode falhar
	// porque o objeto já não estava lá.
	if err := l.Delete(ctx, key); err != nil {
		t.Errorf("Delete do que não existe deu erro: %v", err)
	}
}

// A URL é DERIVADA da chave. É o que permite migrar disco→S3/CDN trocando uma
// variável de ambiente em vez de reescrever `product_images`.
func TestPrefixResolver_DerivaURLDaChave(t *testing.T) {
	casos := []struct {
		prefix, key, want string
	}{
		{"/media", "produtos/a/b-thumb.jpg", "/media/produtos/a/b-thumb.jpg"},
		{"/media/", "produtos/a/b-thumb.jpg", "/media/produtos/a/b-thumb.jpg"},
		{"https://cdn.utilar.com.br", "produtos/a/b-thumb.jpg", "https://cdn.utilar.com.br/produtos/a/b-thumb.jpg"},
		{"/media", "", ""},
		// Imagem EXTERNA (as 288 do Wikimedia): a URL absoluta passa intacta.
		// É isso que faz o legado conviver com o upload no mesmo modelo.
		{"/media", "https://upload.wikimedia.org/x.jpg", "https://upload.wikimedia.org/x.jpg"},
	}
	for _, tc := range casos {
		if got := PrefixResolver(tc.prefix).URL(tc.key); got != tc.want {
			t.Errorf("PrefixResolver(%q).URL(%q) = %q, queria %q", tc.prefix, tc.key, got, tc.want)
		}
	}
}

// Fail-closed: pedir s3 hoje faz o serviço NÃO SUBIR. Cair silenciosamente pro
// disco local em produção daria um catálogo cujas fotos somem no próximo
// deploy, e ninguém descobriria até um cliente reclamar.
func TestNew_S3FalhaExplicitamenteEDriverDesconhecidoTambem(t *testing.T) {
	if _, err := New("s3", t.TempDir(), "/media"); err == nil {
		t.Error("driver s3 subiu sem estar implementado")
	}
	if _, err := New("gcs", t.TempDir(), "/media"); err == nil {
		t.Error("driver desconhecido foi aceito")
	}
	s, err := New("local", filepath.Join(t.TempDir(), "m"), "/media")
	if err != nil || s.Driver() != "local" {
		t.Errorf("driver local: %v / %q", err, s.Driver())
	}
	// Sem STORAGE_DRIVER definido o default é local (roda `make dev` sem
	// configurar nada).
	s, err = New("", filepath.Join(t.TempDir(), "m"), "/media")
	if err != nil || s.Driver() != "local" {
		t.Errorf("default: %v", err)
	}
}
