package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Passo 2 do fluxo (mapeamento de colunas) e a listagem de lotes, contra banco
// de verdade. Ver docs/ingestao-de-produtos.md.
func TestImport_SugestaoDeColunasComConfianca(t *testing.T) {
	db := setupTestDB(t)
	r := importRouter(db)

	csv := "CODIGO;DESCRICAO DO PRODUTO;VLR VENDA;VLR CUSTO;ESTOQUE;UNIDADE;COD_ERP_ANTIGO\n" +
		"ABC;Cimento;\"R$ 42,90\";\"R$ 31,50\";10;SC;zzz\n"

	w := uploadFile(t, r, "/api/v1/admin/import/suggest", "forn.csv", []byte(csv), true)
	if w.Code != http.StatusOK {
		t.Fatalf("suggest: %d — %s", w.Code, w.Body)
	}

	var out struct {
		Columns []struct {
			Column     string `json:"column"`
			Field      string `json:"field"`
			Confidence string `json:"confidence"`
			Recognized bool   `json:"recognized"`
		} `json:"columns"`
		Summary struct {
			Recognized int      `json:"recognized"`
			Ignored    []string `json:"ignored"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	byCol := map[string]string{}
	conf := map[string]string{}
	for _, c := range out.Columns {
		byCol[c.Column] = c.Field
		conf[c.Column] = c.Confidence
	}
	if byCol["VLR VENDA"] != "price" || byCol["VLR CUSTO"] != "cost" {
		t.Errorf("mapeamento errado: VLR VENDA=%q VLR CUSTO=%q", byCol["VLR VENDA"], byCol["VLR CUSTO"])
	}
	if byCol["CODIGO"] != "sku" || byCol["DESCRICAO DO PRODUTO"] != "name" {
		t.Errorf("CODIGO=%q DESCRICAO=%q", byCol["CODIGO"], byCol["DESCRICAO DO PRODUTO"])
	}
	if conf["VLR VENDA"] == "" {
		t.Error("sem grau de confiança")
	}
	if len(out.Summary.Ignored) != 1 || out.Summary.Ignored[0] != "COD_ERP_ANTIGO" {
		t.Errorf("coluna desconhecida deveria ser reportada como ignorada: %v", out.Summary.Ignored)
	}
}

func TestImport_ListagemDeLotesComStatus(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-listagem", `{}`)
	csv := "SKU,NOME,CATEGORIA,PRECO\n" + importSKUPrefix + "L,Item,fixacao,\"R$ 5,00\"\n"
	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID, "teste-list.csv", []byte(csv), true)
	batchID := jsonField(t, w.Body.Bytes(), "batchId")

	w = adminJSON(t, r, http.MethodGet, "/api/v1/admin/import/batches?status=validated", "")
	if w.Code != http.StatusOK {
		t.Fatalf("listagem: %d — %s", w.Code, w.Body)
	}

	var out struct {
		Data []struct {
			ID      string                       `json:"id"`
			Status  string                       `json:"status"`
			Profile string                       `json:"profile"`
			Summary struct{ Total, Creates int } `json:"summary"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	found := false
	for _, b := range out.Data {
		if b.ID == batchID {
			found = true
			if b.Status != "validated" || b.Profile != "teste-listagem" || b.Summary.Creates != 1 {
				t.Errorf("lote mal reportado: %+v", b)
			}
		}
	}
	if !found {
		t.Fatalf("lote %s não apareceu na listagem", batchID)
	}
}
