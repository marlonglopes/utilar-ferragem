package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ============================================================================
// SKU e código de barras reais
// ----------------------------------------------------------------------------
// O que estava quebrado: o PDV derivava SKU e código de barras do id (UUID) do
// produto. Um leitor físico não casava com nada, e o vendedor não tinha código
// nenhum pra ditar por telefone.
// ============================================================================

// blocoBalcaoIDs extrai o SQL entre os marcadores de um arquivo.
func blocoBalcaoIDs(t *testing.T, caminho string) string {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	s := string(b)
	ini := strings.Index(s, "-- >>> BALCAO_IDS_BEGIN")
	fim := strings.Index(s, "-- <<< BALCAO_IDS_END")
	if ini < 0 || fim < 0 || fim < ini {
		t.Fatalf("marcadores BALCAO_IDS ausentes em %s", caminho)
	}
	return strings.TrimSpace(s[ini:fim])
}

// REGRESSÃO: o bloco que atribui SKU e código de barras vive em dois arquivos —
// no seed.sql (pra que `make catalog-db-seed` já entregue catálogo escaneável)
// e em balcao_ids.sql (pra rodar DEPOIS da importação do catálogo curado, que
// chega sem código de barras).
//
// Se os dois divergirem, o catálogo importado é rotulado por uma regra
// diferente da do seed — e dois produtos podem receber o mesmo código, ou o
// importado receber um EAN de fabricante enquanto o seed usa a faixa interna.
// Este teste é o que impede a divergência silenciosa.
func TestSeed_BlocoDeIdentificacaoDeBalcaoNaoDivergiu(t *testing.T) {
	doSeed := blocoBalcaoIDs(t, "../../migrations/seed.sql")
	doArquivo := blocoBalcaoIDs(t, "../../migrations/balcao_ids.sql")
	if doSeed != doArquivo {
		t.Errorf("o bloco BALCAO_IDS de seed.sql e balcao_ids.sql divergiu — os dois\n" +
			"precisam rotular o catálogo pela MESMA regra")
	}
}

// ----------------------------------------------------------------------------
// A decisão sobre EAN
// ----------------------------------------------------------------------------

// eanValido confere o dígito verificador do EAN-13. Um código com DV errado é
// lido pelo scanner como leitura falha — o vendedor bipa e não acontece nada.
func eanValido(c string) bool {
	if len(c) != 13 {
		return false
	}
	soma := 0
	for i := 0; i < 12; i++ {
		d := int(c[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if i%2 == 1 { // posições pares (1-based) pesam 3
			d *= 3
		}
		soma += d
	}
	return (10-soma%10)%10 == int(c[12]-'0')
}

// REGRESSÃO NOMEADA PELO QUE PREVINE: o seed antigo gerava
// `'789' || hash(slug)` — um EAN com prefixo de país brasileiro, ou seja, uma
// identidade física FALSA. Aquele número pertence (ou vai pertencer) a um
// produto real de um fabricante real; no dia em que a loja bipar o item de
// verdade, o código casa com o produto errado e a venda sai com preço, custo e
// estoque de outra coisa.
//
// A regra que este teste trava: código gerado pela casa mora na faixa de
// circulação restrita do GS1 (GTIN começando com "2"), que a norma reserva pra
// uso interno de loja e nunca aloca a fabricante.
func TestRegression_SeedNaoInventaEANDeFabricante(t *testing.T) {
	db := setupTestDB(t)

	rows, err := db.Query(`SELECT sku, barcode FROM products WHERE sku LIKE 'UTL-%' AND barcode IS NOT NULL`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var sku, barcode string
		if err := rows.Scan(&sku, &barcode); err != nil {
			t.Fatalf("scan: %v", err)
		}
		n++
		if !strings.HasPrefix(barcode, "2") {
			t.Errorf("%s tem código %q fora da faixa reservada — é EAN de fabricante inventado", sku, barcode)
		}
		if !eanValido(barcode) {
			t.Errorf("%s tem código %q com dígito verificador inválido — o leitor não lê", sku, barcode)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if n == 0 {
		t.Skip("nenhum produto do seed no banco — rode `make catalog-db-reset`")
	}
}

// Código de barras é identidade física: dois produtos com o mesmo código fazem
// o leitor abrir o item errado. O índice único garante, mas o seed poderia
// falhar ao aplicar e ninguém perceberia numa leitura de log.
func TestSeed_CodigoDeBarrasEhUnico(t *testing.T) {
	db := setupTestDB(t)
	var dups int
	err := db.QueryRow(`
		SELECT count(*) FROM (
			SELECT barcode FROM products WHERE barcode IS NOT NULL
			GROUP BY barcode HAVING count(*) > 1
		) d`).Scan(&dups)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if dups != 0 {
		t.Errorf("%d código(s) de barras repetido(s) — o leitor abriria o item errado", dups)
	}
}

// SKU do seed segue o formato humano UTL-<CAT>-<NNNN>: cabe na etiqueta e é
// ditável por telefone, ao contrário do UUID que o PDV usava.
func TestSeed_SKUSegueOFormatoDoBalcao(t *testing.T) {
	db := setupTestDB(t)

	var total, fora int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE 'UTL-%'`).Scan(&total); err != nil {
		t.Fatalf("query: %v", err)
	}
	if total == 0 {
		t.Skip("nenhum produto do seed no banco — rode `make catalog-db-reset`")
	}
	if err := db.QueryRow(`
		SELECT count(*) FROM products
		WHERE sku LIKE 'UTL-%' AND sku !~ '^UTL-[A-Z]{3}-[0-9]{4}$'`).Scan(&fora); err != nil {
		t.Fatalf("query: %v", err)
	}
	if fora != 0 {
		t.Errorf("%d SKU(s) fora do formato UTL-<CAT>-<NNNN>", fora)
	}
}

// Todo produto do seed tem que sair identificado: sem SKU o vendedor não acha
// o item pelo código, e sem código de barras o leitor não serve pra nada.
func TestSeed_TodoProdutoTemSKUECodigoDeBarras(t *testing.T) {
	db := setupTestDB(t)

	var semSKU, semBarcode int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku IS NULL`).Scan(&semSKU); err != nil {
		t.Fatalf("query: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE barcode IS NULL`).Scan(&semBarcode); err != nil {
		t.Fatalf("query: %v", err)
	}
	if semSKU > 0 {
		t.Errorf("%d produto(s) sem SKU — o balcão não consegue chamá-los pelo código", semSKU)
	}
	if semBarcode > 0 {
		t.Errorf("%d produto(s) sem código de barras — o leitor do balcão não casa com eles\n"+
			"(se vieram do catálogo curado, rode migrations/balcao_ids.sql depois da importação)", semBarcode)
	}
}

// ----------------------------------------------------------------------------
// SKU e código de barras na LISTAGEM e no DETALHE
// ----------------------------------------------------------------------------

type balcaoItem struct {
	ID      string  `json:"id"`
	SKU     *string `json:"sku"`
	Barcode *string `json:"barcode"`
}

// REGRESSÃO: o PDV busca e precisa EXIBIR o código. Se `sku`/`barcode` saírem
// da projeção pública, a busca por código continua funcionando mas a tela
// mostra um produto sem identificação — e o vendedor não consegue conferir se
// bipou o item certo.
func TestBalcaoIDs_SKUeBarcodeAparecemNaListagemENoDetalhe(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-IDS-VISIVEL", 42.90, 30.00)
	r := hwRouter(db)

	// Listagem
	w := hwGet(r, "/api/v1/products?sku=TEST-IDS-VISIVEL")
	if w.Code != http.StatusOK {
		t.Fatalf("listagem: %d — %s", w.Code, w.Body.String())
	}
	var lista struct {
		Data []balcaoItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &lista); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(lista.Data) != 1 {
		t.Fatalf("esperava 1 item na listagem, veio %d", len(lista.Data))
	}
	if lista.Data[0].SKU == nil || *lista.Data[0].SKU != "TEST-IDS-VISIVEL" {
		t.Errorf("SKU deveria aparecer na LISTAGEM; veio %v", lista.Data[0].SKU)
	}
	if lista.Data[0].Barcode == nil || *lista.Data[0].Barcode == "" {
		t.Errorf("código de barras deveria aparecer na LISTAGEM; veio %v", lista.Data[0].Barcode)
	}

	// Detalhe
	wd := hwGet(r, "/api/v1/products/by-id/"+id)
	if wd.Code != http.StatusOK {
		t.Fatalf("detalhe: %d — %s", wd.Code, wd.Body.String())
	}
	var detalhe balcaoItem
	if err := json.Unmarshal(wd.Body.Bytes(), &detalhe); err != nil {
		t.Fatalf("json: %v", err)
	}
	if detalhe.SKU == nil || *detalhe.SKU != "TEST-IDS-VISIVEL" {
		t.Errorf("SKU deveria aparecer no DETALHE; veio %v", detalhe.SKU)
	}
	if detalhe.Barcode == nil || *detalhe.Barcode == "" {
		t.Errorf("código de barras deveria aparecer no DETALHE; veio %v", detalhe.Barcode)
	}
}

// ----------------------------------------------------------------------------
// Índices do caminho do leitor
// ----------------------------------------------------------------------------

// O leitor do balcão faz lookup EXATO (`WHERE barcode = $1`). Isso exige índice
// B-TREE. O índice trigram (GIN) que existe pra busca por prefixo NÃO atende
// igualdade — e um catálogo com só o GIN degradaria pra varredura sequencial
// no caminho mais quente da loja, sem nenhum erro aparecer.
//
// Checar o CATÁLOGO do Postgres em vez de EXPLAIN de propósito: com 400 linhas
// o planejador escolhe seq scan mesmo tendo índice, então um teste de EXPLAIN
// falharia por motivo errado. O que precisa ser garantido é a EXISTÊNCIA do
// índice certo.
func TestIndices_LookupExatoDeSKUeBarcodeTemBtree(t *testing.T) {
	db := setupTestDB(t)

	casos := []struct{ coluna, indice string }{
		{"sku", "idx_products_sku"},
		{"barcode", "idx_products_barcode"},
	}
	for _, tc := range casos {
		t.Run(tc.coluna, func(t *testing.T) {
			var def string
			err := db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE tablename='products' AND indexname=$1`,
				tc.indice).Scan(&def)
			if err == sql.ErrNoRows {
				t.Fatalf("índice %s não existe — o lookup exato do leitor vira seq scan", tc.indice)
			}
			if err != nil {
				t.Fatalf("pg_indexes: %v", err)
			}
			if !strings.Contains(def, "USING btree") {
				t.Errorf("%s não é btree, então não atende `%s = $1`: %s", tc.indice, tc.coluna, def)
			}
			if !regexp.MustCompile(`\(` + tc.coluna + `\)`).MatchString(def) {
				t.Errorf("%s não indexa a coluna %s: %s", tc.indice, tc.coluna, def)
			}
			// Único: dois produtos com o mesmo código fariam o leitor abrir o
			// item errado, e a busca exata devolveria uma lista em vez do item.
			if !strings.Contains(def, "UNIQUE") {
				t.Errorf("%s deveria ser UNIQUE: %s", tc.indice, def)
			}
		})
	}
}
