package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/catalog-service/internal/storage"
)

// --- infra de teste --------------------------------------------------------

const testJWTSecret = "segredo-de-teste-com-mais-de-32-caracteres-ok"

// uploadRouter monta o mesmo encadeamento do main: o grupo /admin protegido por
// RequireAdmin. Testar o handler solto provaria que ele funciona, não que ele
// está PROTEGIDO — e é a proteção que interessa aqui.
func uploadRouter(t *testing.T, db *sql.DB) (*gin.Engine, storage.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	store, err := storage.NewLocal(filepath.Join(t.TempDir(), "media"), "/media")
	if err != nil {
		t.Fatal(err)
	}
	h := handler.NewProductImageHandler(db, store)

	r := gin.New()
	// devMode=false de propósito: o fallback X-User-Role é conveniência de dev
	// e não pode ser o que faz o teste de autorização passar.
	admin := r.Group("/api/v1/admin", handler.RequireAdmin(testJWTSecret, false))
	admin.GET("/products/by-id/:id/images", h.List)
	admin.POST("/products/by-id/:id/images/upload", h.Upload)
	admin.PUT("/products/by-id/:id/images/order", h.Reorder)
	admin.PUT("/products/by-id/:id/images/:imageId/cover", h.SetCover)
	admin.DELETE("/products/by-id/:id/images/:imageId", h.Delete)

	r.GET("/media/*path", handler.NewMediaHandler(store).Serve)
	return r, store
}

func tokenFor(t *testing.T, role string) string {
	t.Helper()
	return signToken(t, testJWTSecret, "user-1", role)
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := h / 4; y < 3*h/4; y++ {
		for x := w / 4; x < 3*w/4; x++ {
			img.Set(x, y, color.RGBA{200, 40, 40, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type imgFile struct {
	field    string
	filename string
	data     []byte
}

func multipartBody(t *testing.T, files []imgFile) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range files {
		field := f.field
		if field == "" {
			field = "files"
		}
		fw, err := w.CreateFormFile(field, f.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func doUpload(t *testing.T, r *gin.Engine, productID, token string, files []imgFile) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, files)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/products/by-id/"+productID+"/images/upload", body)
	req.Header.Set("Content-Type", ct)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// firstProductID pega um produto qualquer do seed pra usar de alvo.
func firstProductID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM products ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("sem produto no seed: %v", err)
	}
	return id
}

// cleanupImages isola o teste: apaga as imagens PRÓPRIAS (checksum não nulo)
// do produto antes de começar, e de novo no fim.
//
// ⚠️ A limpeza precisa ser feita ANTES também, e não só no t.Cleanup. Os testes
// de integração daqui usam `defer db.Close()`, e `defer` roda ANTES dos
// callbacks de t.Cleanup — então o DELETE do final pega a conexão já fechada e
// não apaga nada. Sem a limpeza inicial, o teste seguinte encontrava as fotos
// do anterior e a capa era a errada.
//
// As imagens EXTERNAS (checksum NULL, o legado Wikimedia) nunca são tocadas.
func cleanupImages(t *testing.T, db *sql.DB, productID string) {
	t.Helper()
	purge := func() {
		_, _ = db.Exec(`DELETE FROM product_images WHERE product_id::text=$1 AND checksum IS NOT NULL`, productID)
	}
	purge()
	t.Cleanup(purge)
}

// --- autorização -----------------------------------------------------------

// Upload é escrita no catálogo. Anônimo e `customer` não passam, e
// `store_operator` também não: operador de balcão VÊ o catálogo, não escreve
// nele — confundir os dois daria a todo vendedor o poder de trocar a foto de
// qualquer produto da loja.
func TestUploadImagem_SoAdminPodeSubir(t *testing.T) {
	r, _ := uploadRouter(t, nil)
	files := []imgFile{{filename: "foto.jpg", data: testJPEG(t, 800, 600)}}

	casos := []struct {
		nome  string
		token string
		want  int
	}{
		{"anônimo", "", http.StatusUnauthorized},
		{"customer", tokenFor(t, "customer"), http.StatusForbidden},
		{"seller (lojista do marketplace)", tokenFor(t, "seller"), http.StatusForbidden},
		{"store_operator (vendedor de balcão)", tokenFor(t, "store_operator"), http.StatusForbidden},
		{"token inválido", "lixo.lixo.lixo", http.StatusUnauthorized},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := doUpload(t, r, "00000000-0000-0000-0000-000000000000", tc.token, files)
			if w.Code != tc.want {
				t.Errorf("status = %d, queria %d: %s", w.Code, tc.want, w.Body.String())
			}
			// O corpo não pode vazar nada do processamento: a recusa acontece
			// ANTES de qualquer byte ser lido.
			if strings.Contains(w.Body.String(), "variants") {
				t.Error("resposta de recusa vazou dados de imagem")
			}
		})
	}
}

// Reordenar e definir capa também são escrita no catálogo.
func TestOrdenacaoECapa_SoAdmin(t *testing.T) {
	r, _ := uploadRouter(t, nil)
	const pid = "00000000-0000-0000-0000-000000000000"

	casos := []struct {
		method, path string
		body         string
	}{
		{http.MethodPut, "/api/v1/admin/products/by-id/" + pid + "/images/order", `{"order":["x"]}`},
		{http.MethodPut, "/api/v1/admin/products/by-id/" + pid + "/images/x/cover", ``},
		{http.MethodDelete, "/api/v1/admin/products/by-id/" + pid + "/images/x", ``},
		{http.MethodGet, "/api/v1/admin/products/by-id/" + pid + "/images", ``},
	}
	for _, tc := range casos {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			for _, role := range []string{"", "customer", "store_operator"} {
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				if role != "" {
					req.Header.Set("Authorization", "Bearer "+tokenFor(t, role))
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
					t.Errorf("role %q recebeu %d — devia ser 401/403", role, w.Code)
				}
			}
		})
	}
}

// --- nome de arquivo malicioso ---------------------------------------------

// `../../etc/passwd` como nome de arquivo é o ataque clássico de upload. Duas
// coisas têm que ser verdade: o nome nunca vira caminho (a chave é gerada por
// nós) e o nome ECOADO na resposta/log está sanitizado — nome cru é injeção de
// linha de log e XSS refletido no painel do admin.
func TestSanitizeFilename_NeutralizaNomeMalicioso(t *testing.T) {
	casos := []struct{ entrada, want string }{
		{"../../etc/passwd", "passwd"},
		{"../../../../../../etc/shadow", "shadow"},
		{`..\..\windows\system32\cmd.exe`, "cmd.exe"},
		{"/etc/passwd", "passwd"},
		{"foto.jpg", "foto.jpg"},
		{"furadeira 500w.jpg", "furadeira 500w.jpg"},
		{"parafuso-6x40_zincado.JPG", "parafuso-6x40_zincado.JPG"},
		{"..", "arquivo"},
		{".", "arquivo"},
		{"", "arquivo"},
		{"...", "arquivo"},
		{".htaccess", "htaccess"},
		{"x.jpg\x00.php", "x.jpg_.php"},
		// filepath.Base corta em "/" — o "</script>" faz o nome virar o último segmento.
		{"<script>alert(1)</script>.jpg", "script_.jpg"},
		{"nome\ncom\rquebra.jpg", "nome_com_quebra.jpg"},
		{"shell$(id).jpg", "shell__id_.jpg"},
	}
	for _, tc := range casos {
		t.Run(tc.entrada, func(t *testing.T) {
			got := handler.SanitizeFilename(tc.entrada)
			// A checagem que realmente importa: o resultado nunca é caminho.
			if strings.ContainsAny(got, `/\`) || strings.Contains(got, "..") {
				t.Fatalf("SanitizeFilename(%q) = %q — ainda parece caminho", tc.entrada, got)
			}
			if strings.ContainsAny(got, "\n\r\x00") {
				t.Fatalf("SanitizeFilename(%q) = %q — sobrou byte de controle", tc.entrada, got)
			}
			if got != tc.want {
				t.Errorf("SanitizeFilename(%q) = %q, queria %q", tc.entrada, got, tc.want)
			}
		})
	}
}

func TestSanitizeFilename_TruncaNomeAbsurdo(t *testing.T) {
	got := handler.SanitizeFilename(strings.Repeat("a", 5000) + ".jpg")
	if len(got) > 120 {
		t.Errorf("nome de %d caracteres não foi truncado", len(got))
	}
}

// --- rota de mídia ---------------------------------------------------------

// Servir upload é o outro lado do buraco: um arquivo enviado que sai com o
// Content-Type errado (ou adivinhado) vira HTML executado na origem da loja.
func TestMedia_ServeComTipoCorretoESemExecucao(t *testing.T) {
	r, store := uploadRouter(t, nil)
	local := store.(*storage.Local)

	key := "produtos/abc/dead0000beef1111-thumb.jpg"
	if err := local.Put(t.Context(), key, "image/jpeg", testJPEG(t, 300, 300)); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/media/"+key, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		t.Errorf("Content-Type = %q, queria image/jpeg", ct)
	}
	// Sem nosniff o browser pode reinterpretar o conteúdo e ignorar o
	// Content-Type que acabamos de declarar.
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("faltou X-Content-Type-Options: nosniff na rota de mídia")
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "inline") {
		t.Error("faltou Content-Disposition: inline")
	}
}

func TestMedia_RecusaTravessiaEExtensaoForaDaAllowlist(t *testing.T) {
	r, store := uploadRouter(t, nil)
	local := store.(*storage.Local)

	// Um arquivo perigoso plantado na raiz da mídia (simula um bug em outro
	// caminho de código). Nem assim ele pode sair pela rota.
	if err := os.WriteFile(filepath.Join(local.Root(), "shell.php"),
		[]byte("<?php system($_GET['c']); ?>"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		"/media/shell.php",
		"/media/../../etc/passwd",
		"/media/produtos/../../../etc/passwd",
		"/media/produtos/x.html",
		"/media/produtos/x.svg",
		"/media/",
	} {
		t.Run(p, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
			if w.Code == http.StatusOK {
				t.Errorf("serviu %q com 200: %q", p, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "<?php") {
				t.Errorf("VAZOU conteúdo executável em %q", p)
			}
		})
	}
}

// --- integração (precisa de Postgres) --------------------------------------

// O caminho feliz completo, de ponta a ponta: várias imagens de proporções
// diferentes numa chamada só, todas saindo quadradas, com as três variantes,
// e a galeria montada na ordem certa.
func TestUploadImagem_VariasImagensDeProporcoesDiferentes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, store := uploadRouter(t, db)
	pid := firstProductID(t, db)
	cleanupImages(t, db, pid)

	files := []imgFile{
		{filename: "paisagem.jpg", data: testJPEG(t, 1600, 900)},
		{filename: "retrato.jpg", data: testJPEG(t, 900, 1600)},
		{filename: "quadrada.jpg", data: testJPEG(t, 1200, 1200)},
	}
	w := doUpload(t, r, pid, tokenFor(t, "admin"), files)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Uploaded []struct {
			ID            string            `json:"id"`
			URL           string            `json:"url"`
			Variants      map[string]string `json:"variants"`
			Width, Height int
			OriginalBytes int `json:"originalBytes"`
			Bytes         int `json:"bytes"`
			SortOrder     int `json:"sortOrder"`
		} `json:"uploaded"`
		Rejected []map[string]string `json:"rejected"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Uploaded) != 3 || len(resp.Rejected) != 0 {
		t.Fatalf("subiram %d e foram recusadas %d; queria 3/0: %s",
			len(resp.Uploaded), len(resp.Rejected), w.Body.String())
	}

	seenOrder := map[int]bool{}
	for i, u := range resp.Uploaded {
		// TODAS quadradas, independente da proporção que entrou — é o requisito
		// central do dono.
		if u.Width != u.Height {
			t.Errorf("imagem %d saiu %dx%d; tinha que ser quadrada", i, u.Width, u.Height)
		}
		for _, v := range []string{"thumb", "medium", "large"} {
			if u.Variants[v] == "" {
				t.Errorf("imagem %d sem variante %q — o frontend receberia URL vazia", i, v)
			}
			if !strings.HasPrefix(u.Variants[v], "/media/produtos/") {
				t.Errorf("variante %q = %q; não parece URL derivada da chave", v, u.Variants[v])
			}
		}
		if u.Bytes == 0 || u.OriginalBytes == 0 {
			t.Errorf("imagem %d sem antes/depois de bytes registrado", i)
		}
		if seenOrder[u.SortOrder] {
			t.Errorf("sort_order %d repetido — duas fotos disputariam a capa", u.SortOrder)
		}
		seenOrder[u.SortOrder] = true

		// A variante realmente EXISTE no storage e é um JPEG de verdade.
		key := strings.TrimPrefix(u.Variants["thumb"], "/media/")
		data, err := store.(*storage.Local).Read(key)
		if err != nil {
			t.Fatalf("variante thumb não está no storage: %v", err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("thumb não é JPEG válido: %v", err)
		}
		if cfg.Width != 300 || cfg.Height != 300 {
			t.Errorf("thumb salvo em %dx%d, queria 300x300", cfg.Width, cfg.Height)
		}
	}

	// O banco guarda a CHAVE, nunca a URL absoluta nem caminho de disco: é isso
	// que faz migrar disco→S3 ser configuração em vez de UPDATE na tabela
	// inteira, com o catálogo no ar.
	rows, err := db.Query(`SELECT storage_key, variants::text FROM product_images
	                       WHERE product_id::text=$1 AND checksum IS NOT NULL`, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, variants string
		if err := rows.Scan(&key, &variants); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{key, variants} {
			if strings.Contains(s, "http://") || strings.Contains(s, "https://") ||
				strings.Contains(s, "/media/") || strings.HasPrefix(s, "/") ||
				strings.Contains(s, os.TempDir()) {
				t.Errorf("banco guardou URL/caminho absoluto em vez de chave lógica: %q", s)
			}
		}
		if !strings.HasPrefix(key, "produtos/") {
			t.Errorf("storage_key = %q; queria prefixo produtos/", key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Recusa é POR ARQUIVO: o lojista seleciona 12 fotos no celular e uma é um
// print de tela. Rejeitar as 12 por causa de uma faz ele repetir tudo.
func TestUploadImagem_RecusaOArquivoRuimESalvaOsBons(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)
	cleanupImages(t, db, pid)

	files := []imgFile{
		{filename: "boa.jpg", data: testJPEG(t, 1000, 800)},
		// Extensão e nome mentem: é um PHP. Só o byte decide.
		{filename: "inocente.jpg", data: []byte("<?php system($_GET['c']); ?>")},
		{filename: "vazio.jpg", data: []byte{}},
	}
	w := doUpload(t, r, pid, tokenFor(t, "admin"), files)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Uploaded []map[string]any `json:"uploaded"`
		Rejected []struct {
			Filename, Reason, Code string
		} `json:"rejected"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Uploaded) != 1 {
		t.Errorf("subiram %d; a foto boa tinha que entrar sozinha", len(resp.Uploaded))
	}
	if len(resp.Rejected) != 2 {
		t.Fatalf("recusadas %d, queria 2: %s", len(resp.Rejected), w.Body.String())
	}
	for _, rj := range resp.Rejected {
		if rj.Code != "not_an_image" {
			t.Errorf("código de recusa = %q, queria not_an_image", rj.Code)
		}
	}
}

// Todos recusados = 400, com o motivo de cada um. Não 201 com lista vazia: o
// admin precisa saber que nada entrou.
func TestUploadImagem_TodosRecusadosDevolve400(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)

	w := doUpload(t, r, pid, tokenFor(t, "admin"), []imgFile{
		{filename: "a.jpg", data: []byte("nao sou imagem")},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, queria 400: %s", w.Code, w.Body.String())
	}
}

// Bomba de descompressão pelo HTTP: a recusa tem que acontecer no handler, não
// só na unidade de imaging.
func TestUploadImagem_RecusaBombaDeDescompressao(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)

	w := doUpload(t, r, pid, tokenFor(t, "admin"), []imgFile{
		{filename: "bomba.png", data: pngBomb(t, 25_000, 25_000)},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, queria 400 — aceitou 625 megapixels: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "image_too_large") {
		t.Errorf("código de recusa inesperado: %s", w.Body.String())
	}
}

func TestUploadImagem_RecusaMaisArquivosQueOLimite(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)

	files := make([]imgFile, 25)
	small := testJPEG(t, 300, 300)
	for i := range files {
		files[i] = imgFile{filename: fmt.Sprintf("f%d.jpg", i), data: small}
	}
	w := doUpload(t, r, pid, tokenFor(t, "admin"), files)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d — aceitou 25 arquivos numa requisição", w.Code)
	}
}

// A mesma foto subida duas vezes não pode virar dois slides do carrossel nem
// dois objetos no storage.
func TestUploadImagem_DeduplicaMesmaFoto(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)
	cleanupImages(t, db, pid)

	data := testJPEG(t, 1100, 700)
	tok := tokenFor(t, "admin")

	w1 := doUpload(t, r, pid, tok, []imgFile{{filename: "a.jpg", data: data}})
	if w1.Code != http.StatusCreated {
		t.Fatalf("primeiro upload: %d %s", w1.Code, w1.Body.String())
	}
	// Mesmo conteúdo, outro nome — o hash é do CONTEÚDO.
	w2 := doUpload(t, r, pid, tok, []imgFile{{filename: "copia-da-mesma-foto.jpg", data: data}})
	if w2.Code != http.StatusCreated {
		t.Fatalf("segundo upload: %d %s", w2.Code, w2.Body.String())
	}

	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM product_images WHERE product_id::text=$1 AND checksum IS NOT NULL`,
		pid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d linhas para a mesma foto; a dedupe por checksum falhou", n)
	}
	if !strings.Contains(w2.Body.String(), `"deduplicated":true`) {
		t.Errorf("a resposta não sinalizou dedupe: %s", w2.Body.String())
	}
}

// Ordenação e capa continuam funcionando agora que a linha carrega variantes.
func TestUploadImagem_OrdenacaoECapa(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)
	pid := firstProductID(t, db)
	cleanupImages(t, db, pid)
	tok := tokenFor(t, "admin")

	w := doUpload(t, r, pid, tok, []imgFile{
		{filename: "um.jpg", data: testJPEG(t, 900, 600)},
		{filename: "dois.jpg", data: testJPEG(t, 901, 600)},
		{filename: "tres.jpg", data: testJPEG(t, 902, 600)},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	ids := uploadedIDs(t, w)
	if len(ids) != 3 {
		t.Fatalf("queria 3 ids, veio %d", len(ids))
	}

	// Inverte a ordem: o último vira a capa.
	inverso := []string{ids[2], ids[1], ids[0]}
	body, _ := json.Marshal(map[string]any{"order": inverso})
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/products/by-id/"+pid+"/images/order", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("reorder: %d %s", w2.Code, w2.Body.String())
	}
	if got := coverID(t, db, pid); got != inverso[0] {
		t.Errorf("capa = %s, queria %s depois do reorder", got, inverso[0])
	}

	// Promove o do meio a capa sem mandar a lista inteira.
	req = httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/products/by-id/"+pid+"/images/"+ids[1]+"/cover", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("cover: %d %s", w3.Code, w3.Body.String())
	}
	if got := coverID(t, db, pid); got != ids[1] {
		t.Errorf("capa = %s, queria %s depois de SetCover", got, ids[1])
	}
}

// Regressão de IDOR: sem casar product_id no WHERE, um admin reordenaria a
// imagem de OUTRO produto passando o id dela — e a foto migraria de galeria
// sem nenhum erro aparente.
func TestReorder_NaoAceitaImagemDeOutroProduto(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)

	var outro string
	if err := db.QueryRow(`SELECT product_id::text FROM product_images LIMIT 1`).Scan(&outro); err != nil {
		t.Skipf("sem imagem no seed: %v", err)
	}
	var imgID string
	if err := db.QueryRow(`SELECT id::text FROM product_images WHERE product_id::text=$1 LIMIT 1`,
		outro).Scan(&imgID); err != nil {
		t.Fatal(err)
	}
	var alvo string
	if err := db.QueryRow(`SELECT id::text FROM products WHERE id::text <> $1 LIMIT 1`,
		outro).Scan(&alvo); err != nil {
		t.Skip("sem segundo produto")
	}

	body, _ := json.Marshal(map[string]any{"order": []string{imgID}})
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/products/by-id/"+alvo+"/images/order", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, "admin"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Error("reordenou imagem que pertence a outro produto")
	}

	var dono string
	if err := db.QueryRow(`SELECT product_id::text FROM product_images WHERE id::text=$1`,
		imgID).Scan(&dono); err != nil {
		t.Fatal(err)
	}
	if dono != outro {
		t.Errorf("a imagem MUDOU de produto: %s → %s", outro, dono)
	}
}

func TestUploadImagem_ProdutoInexistenteDa404(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)

	w := doUpload(t, r, "00000000-0000-0000-0000-000000000000", tokenFor(t, "admin"),
		[]imgFile{{filename: "a.jpg", data: testJPEG(t, 800, 800)}})
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, queria 404: %s", w.Code, w.Body.String())
	}
}

// As 288 imagens CC0 do Wikimedia (URL externa, sem variantes) não podem
// quebrar. Elas ficam na MESMA tabela e na mesma galeria — e a ausência de
// `variants` é justamente como o frontend distingue os dois tipos.
func TestImagemExterna_ContinuaFuncionandoSemVariantes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	r, _ := uploadRouter(t, db)

	var pid string
	err := db.QueryRow(`
		SELECT product_id::text FROM product_images
		WHERE checksum IS NULL AND url LIKE 'http%' LIMIT 1`).Scan(&pid)
	if err == sql.ErrNoRows {
		t.Skip("sem imagem externa no seed")
	}
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/by-id/"+pid+"/images", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, "admin"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			URL      string            `json:"url"`
			Variants map[string]string `json:"variants"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("galeria vazia")
	}
	achou := false
	for _, im := range resp.Data {
		if len(im.Variants) == 0 {
			achou = true
			if !strings.HasPrefix(im.URL, "http") {
				t.Errorf("imagem externa com url %q — devia ser a URL absoluta original", im.URL)
			}
		}
	}
	if !achou {
		t.Error("nenhuma imagem externa na galeria — o legado sumiu")
	}
}

// --- helpers ---------------------------------------------------------------

func uploadedIDs(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var resp struct {
		Uploaded []struct {
			ID string `json:"id"`
		} `json:"uploaded"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(resp.Uploaded))
	for _, u := range resp.Uploaded {
		out = append(out, u.ID)
	}
	return out
}

func coverID(t *testing.T, db *sql.DB, productID string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`
		SELECT id::text FROM product_images WHERE product_id::text=$1
		ORDER BY sort_order ASC, created_at ASC LIMIT 1`, productID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// pngBomb: PNG minúsculo que DECLARA um bitmap gigante. Alocar 25.000×25.000 de
// verdade no teste seria detonar a própria bomba.
func pngBomb(t *testing.T, w, h int) []byte {
	t.Helper()
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	be32(ihdr[0:4], uint32(w))
	be32(ihdr[4:8], uint32(h))
	ihdr[8], ihdr[9] = 8, 2
	pngChunk(&out, "IHDR", ihdr)
	pngChunk(&out, "IEND", nil)
	return out.Bytes()
}

func be32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

func pngChunk(w *bytes.Buffer, typ string, data []byte) {
	l := make([]byte, 4)
	be32(l, uint32(len(data)))
	w.Write(l)
	body := append([]byte(typ), data...)
	w.Write(body)
	crc := make([]byte, 4)
	be32(crc, pngCRC32(body))
	w.Write(crc)
}

func pngCRC32(b []byte) uint32 {
	c := uint32(0xFFFFFFFF)
	for _, x := range b {
		c ^= uint32(x)
		for k := 0; k < 8; k++ {
			if c&1 != 0 {
				c = 0xEDB88320 ^ (c >> 1)
			} else {
				c >>= 1
			}
		}
	}
	return c ^ 0xFFFFFFFF
}
