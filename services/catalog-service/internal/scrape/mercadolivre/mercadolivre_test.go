package mercadolivre

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/utilar/catalog-service/internal/scrape"
)

// Resposta-fixture de GET /sites/MLB/search (subconjunto real). Sem rede.
const fixtureBusca = `{
  "results": [
    {
      "id": "MLB111", "title": "Dobradiça 3\" Zincada com Anel", "price": 12.90,
      "thumbnail": "https://http2.mlstatic.com/D_111-O.jpg",
      "permalink": "https://produto.mercadolivre.com.br/MLB-111",
      "category_id": "MLB1234",
      "attributes": [{"id":"GTIN","value_name":"7891234000019"}]
    },
    {
      "id": "MLB222", "title": "Fechadura Externa Rolete", "price": 89.90,
      "thumbnail": "",
      "permalink": "https://produto.mercadolivre.com.br/MLB-222",
      "category_id": "MLB5678",
      "attributes": []
    }
  ]
}`

func TestMapResults_Fixture(t *testing.T) {
	var r mlSearchResp
	if err := json.Unmarshal([]byte(fixtureBusca), &r); err != nil {
		t.Fatalf("json: %v", err)
	}
	got := mapResults(r)
	if len(got) != 2 {
		t.Fatalf("mapeou %d, quero 2", len(got))
	}

	// 1º: completo (nome, preço, imagem, GTIN como código).
	d := got[0]
	if d.Nome != `Dobradiça 3" Zincada com Anel` {
		t.Errorf("nome = %q", d.Nome)
	}
	if d.Preco == nil || *d.Preco != 12.90 {
		t.Errorf("preço = %v, quero 12.90", d.Preco)
	}
	if d.ImagemURLOriginal == nil || *d.ImagemURLOriginal == "" {
		t.Error("esperava imagem (thumbnail)")
	}
	if d.CodigoFabricante == nil || *d.CodigoFabricante != "7891234000019" {
		t.Errorf("código (GTIN) = %v", d.CodigoFabricante)
	}

	// 2º: SEM thumbnail => sem imagem no schema.
	if got[1].ImagemURLOriginal != nil {
		t.Error("resultado sem thumbnail não deveria ter imagem")
	}
}

// Integração com o pipeline: o item SEM imagem é DESCARTADO do lote (regra do
// dono) mas SINALIZADO — nunca some em silêncio.
func TestAssemble_ExigeImagem(t *testing.T) {
	var r mlSearchResp
	_ = json.Unmarshal([]byte(fixtureBusca), &r)
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

	batch := scrape.Assemble("mercadolivre", mapResults(r), now, now)

	// Só a dobradiça (tem imagem) entra; a fechadura (sem thumbnail) fica de fora.
	if len(batch.Produtos) != 1 {
		t.Fatalf("Produtos = %d, quero 1 (só o que tem imagem)", len(batch.Produtos))
	}
	if batch.Produtos[0].CategoriaNormalizada != "fixacao" {
		t.Errorf("dobradiça normalizou para %q, quero fixacao", batch.Produtos[0].CategoriaNormalizada)
	}
	if len(batch.Report.Sinalizados) != 1 {
		t.Errorf("Sinalizados = %d, quero 1 (a fechadura sem imagem)", len(batch.Report.Sinalizados))
	}
}
