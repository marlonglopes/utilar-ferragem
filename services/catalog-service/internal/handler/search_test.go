package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Testes de INTEGRAÇÃO da busca textual (migration 014: tsvector + GIN com a
// configuração `utilar_pt`, que tem unaccent no dicionário).
//
// Rodam contra o Postgres de dev (:5436) com o seed real de 400 produtos.
// Skipam se o banco não estiver disponível — ver setupTestDB em product_test.go.

type buscaResultado struct {
	Data []struct {
		Name    string  `json:"name"`
		SKU     *string `json:"sku"`
		Barcode *string `json:"barcode"`
		Slug    string  `json:"slug"`
	} `json:"data"`
	Meta struct {
		Total       int    `json:"total"`
		Approximate bool   `json:"approximate"`
		Suggestion  string `json:"suggestion"`
	} `json:"meta"`
}

func buscar(t *testing.T, r *gin.Engine, q string, extra ...string) buscaResultado {
	t.Helper()
	u := "/api/v1/products?per_page=100&q=" + url.QueryEscape(q)
	for _, e := range extra {
		u += "&" + e
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, u, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("busca %q: status %d (esperado 200) — %s", q, w.Code, w.Body.String())
	}
	var out buscaResultado
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("busca %q: json inválido: %v", q, err)
	}
	return out
}

// achou diz se algum resultado tem `sub` no nome (case/acento-insensível o
// bastante pro propósito do teste).
func achou(res buscaResultado, sub string) bool {
	for _, p := range res.Data {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func searchRouter(t *testing.T) (*sql.DB, *gin.Engine) {
	t.Helper()
	db := setupTestDB(t)
	return db, setupRouter(db)
}

// ============================================================================
// Acento, plural, radical — o que o tsvector com unaccent tem que resolver
// ============================================================================

// REGRESSÃO: com o ILIKE antigo, "eletrica" NÃO achava "elétrica" — o cliente
// digita sem acento (é o normal no celular) e a loja respondia vazio.
// Só funciona porque `utilar_pt` tem unaccent no DICIONÁRIO; foi por isso que
// a coluna gerada pôde ficar IMMUTABLE (unaccent() na expressão seria rejeitado).
func TestBusca_AcentoOmitidoAcha(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	casos := []struct{ termo, esperado string }{
		{"eletrica", "létric"},     // Elétrica
		{"ceramico", "erâmic"},     // Cerâmico
		{"sifao", "ifã"},           // Sifão
		{"mascara", "áscara"},      // Máscara
		{"acrilica", "crílic"},     // Acrílica
		{"descartavel", "scartáv"}, // Descartável
	}
	for _, c := range casos {
		res := buscar(t, r, c.termo)
		if !achou(res, c.esperado) {
			t.Errorf("q=%q não achou nada com %q (total=%d) — unaccent não está atuando",
				c.termo, c.esperado, res.Meta.Total)
		}
	}
}

// REGRESSÃO: o ILIKE não tinha noção de plural. "parafusos" não achava
// "Parafuso". O stemmer português resolve — mas só se estiver na config certa.
func TestBusca_PluralAchaSingular(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	for _, termo := range []string{"parafusos", "furadeiras", "tijolos", "disjuntores"} {
		res := buscar(t, r, termo)
		if res.Meta.Total == 0 {
			t.Errorf("q=%q devolveu 0 — o stemmer não está reduzindo o plural", termo)
		}
		if res.Meta.Approximate {
			t.Errorf("q=%q caiu na busca APROXIMADA; plural é caso do stemmer, não de similaridade", termo)
		}
	}
}

// O caso que o dono citou: "acrilico" (masculino, sem acento) tem que achar
// "Acrílica" (feminino, com acento). São DOIS mecanismos ao mesmo tempo —
// unaccent e stemmer — e é o erro que o cliente comete de verdade.
func TestBusca_GeneroTrocadoESemAcento(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	for _, termo := range []string{"acrilico", "acrilica"} {
		res := buscar(t, r, termo)
		if !achou(res, "crílic") {
			t.Errorf("q=%q não achou nenhum produto acrílico (total=%d)", termo, res.Meta.Total)
		}
	}
}

// ============================================================================
// Palavra no MEIO do nome e da descrição
// ============================================================================

// REGRESSÃO: o dono pediu explicitamente — "a pesquisa pode incluir palavras
// que estão no meio da descrição, como acrilico".
//
// ⚠️ Isto NÃO é o mesmo que o prefixo `:*`, que só casa o INÍCIO da palavra.
// Aqui a palavra inteira está no meio da frase, e quem resolve é o tsvector,
// que indexa todos os lemas do texto — não a posição deles.
func TestBusca_PalavraNoMeioDoNome(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	casos := []struct{ termo, esperado string }{
		{"galvanizado", "Galvanizado"},   // Arame Galvanizado — 2ª palavra
		{"bipolar", "Bipolar"},           // Disjuntor Bipolar — 2ª palavra
		{"zincada", "Zincada"},           // Arruela de Pressão Zincada — 4ª palavra
		{"refletivo", "Refletivo"},       // Colete Refletivo — 2ª palavra
		{"autobrocante", "Autobrocante"}, // Parafuso Autobrocante — 2ª palavra
		{"descartavel", "Descartável"},   // Máscara Descartável — 2ª palavra, sem acento
	}
	for _, c := range casos {
		res := buscar(t, r, c.termo)
		if !achou(res, c.esperado) {
			t.Errorf("q=%q não achou %q — palavra do meio do nome tem que casar (total=%d)",
				c.termo, c.esperado, res.Meta.Total)
		}
		if res.Meta.Approximate {
			t.Errorf("q=%q caiu no APROXIMADO; é casamento exato de lema, tem que sair no passo 1", c.termo)
		}
	}
}

// ============================================================================
// Relevância
// ============================================================================

// REGRESSÃO: a busca antiga devolvia em ordem de CADASTRO — ou seja, sem
// ordem nenhuma. Nome (peso A) tem que ganhar de descrição (peso C).
//
// E o outro lado, que é o que o dono pediu junto: quem só menciona o termo na
// DESCRIÇÃO ainda tem que APARECER, depois. Peso C não pode virar exclusão.
func TestBusca_NomeGanhaDeDescricao(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	const termoRaro = "zzrelev"
	limpar := func() {
		_, _ = db.Exec(`DELETE FROM products WHERE slug IN ('zz-rank-nome','zz-rank-desc')`)
	}
	limpar()
	defer limpar()

	seller := ""
	if err := db.QueryRow(`SELECT id FROM sellers LIMIT 1`).Scan(&seller); err != nil {
		t.Skipf("sem sellers: %v", err)
	}
	ins := `INSERT INTO products (slug,name,category_id,seller_id,price,currency,icon,stock,status,description)
	        VALUES ($1,$2,'ferramentas',$3,10,'BRL','⚒',5,'published',$4)`
	if _, err := db.Exec(ins, "zz-rank-nome", "Ferramenta "+termoRaro+" Profissional", seller, "sem o termo aqui"); err != nil {
		t.Skipf("não consegui semear (schema pode exigir colunas extras): %v", err)
	}
	if _, err := db.Exec(ins, "zz-rank-desc", "Ferramenta Comum de Bancada", seller, "usada com "+termoRaro+" no processo"); err != nil {
		t.Skipf("não consegui semear: %v", err)
	}

	res := buscar(t, r, termoRaro)
	if res.Meta.Total < 2 {
		t.Fatalf("esperava os 2 produtos semeados, veio total=%d", res.Meta.Total)
	}
	// O de DESCRIÇÃO tem que estar presente...
	if !achou(res, "Ferramenta Comum de Bancada") {
		t.Error("produto que cita o termo só na DESCRIÇÃO sumiu — peso C não pode excluir")
	}
	// ...mas o de NOME tem que vir primeiro.
	if res.Data[0].Slug != "zz-rank-nome" {
		t.Errorf("relevância errada: o 1º resultado foi %q, esperava o que tem o termo no NOME",
			res.Data[0].Slug)
	}
}

// ============================================================================
// Prefixo / autocomplete
// ============================================================================

// Quem digita "furad" na caixa de busca tem que ver "Furadeira" antes de
// terminar de digitar — é o `:*` no último termo da tsquery.
func TestBusca_PrefixoParaAutocomplete(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	casos := []struct{ termo, esperado string }{
		{"furad", "Furadeira"},
		{"argamas", "Argamassa"},
		{"disjunt", "Disjuntor"},
		{"parafu", "Parafuso"},
	}
	for _, c := range casos {
		res := buscar(t, r, c.termo)
		if !achou(res, c.esperado) {
			t.Errorf("q=%q (autocomplete) não achou %q — o sufixo `:*` não está atuando (total=%d)",
				c.termo, c.esperado, res.Meta.Total)
		}
	}
}

// ⚠️ O CASO MAIS TRAIÇOEIRO DA BUSCA: prefixo CURTO cujo acento vem DEPOIS do
// ponto de corte. "acri" tem que achar "Acrílica".
//
// Por que é diferente de acento omitido e de plural: aqui o `:*` casa por
// PREFIXO do lexema. Se o termo da consulta e o vetor indexado não passarem
// pelo MESMO unaccent, `acri:*` não casa com um lexema ainda acentuado —
// diverge já no 4º caractere.
//
// E o modo de falha é SILENCIOSO: dá pra ter unaccent no vetor e esquecer na
// consulta (ou o contrário) e ficar com "palavra inteira funciona, prefixo
// não". Ninguém percebe até um cliente reclamar que o autocomplete não acha.
//
// É justamente isto que a decisão de arquitetura garante: como o unaccent está
// no DICIONÁRIO da configuração `utilar_pt`, ele é aplicado pelos dois lados
// por construção — o to_tsvector da coluna e o to_tsquery da consulta usam a
// mesma config. Com um wrapper IMMUTABLE sobre unaccent() (a alternativa
// descartada) seria possível aplicar em um lado só e ficar com este bug.
func TestBusca_PrefixoCurtoComAcentoDepoisDoCorte(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	casos := []struct{ termo, esperado string }{
		{"acri", "crílic"}, // acento no 4º caractere, DEPOIS do corte
		{"acril", "crílic"},
		{"acrili", "crílic"},
		{"eletr", "létric"},   // Elétrico
		{"masc", "áscara"},    // Máscara
		{"lumin", "uminári"},  // Luminária
		{"cerami", "erâmic"},  // Cerâmico
		{"asfalt", "sfáltic"}, // Asfáltica
		// NOTA: "hidra" → "Hidráulico" ficou DELIBERADAMENTE de fora. O seed
		// de dev não tem nenhum produto com "Hidráulico" acentuado no nome —
		// a categoria hidráulica é grafada sem acento. O mecanismo funciona
		// (verificado à mão: to_tsvector('utilar_pt','Hidráulico') = 'hidraul',
		// casado por 'hidra':*), mas um caso de teste que passa por ausência
		// de dado não cobre nada e vira falso conforto.
	}
	for _, c := range casos {
		res := buscar(t, r, c.termo)
		if !achou(res, c.esperado) {
			t.Errorf("q=%q (prefixo curto) não achou %q (total=%d) — "+
				"o unaccent não está sendo aplicado nos DOIS lados (consulta e vetor)",
				c.termo, c.esperado, res.Meta.Total)
		}
	}
}

// O inverso do teste acima: digitar COM acento não pode regredir. Quem escreve
// certo tem que continuar achando pelo menos tanto quanto quem escreve errado.
func TestRegressao_ComAcentoNaoAchaMenosQueSemAcento(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	pares := []struct{ comAcento, semAcento string }{
		{"acrílica", "acrilica"},
		{"elétrica", "eletrica"},
		{"cerâmico", "ceramico"},
		{"máscara", "mascara"},
	}
	for _, p := range pares {
		com := buscar(t, r, p.comAcento)
		sem := buscar(t, r, p.semAcento)
		if com.Meta.Total == 0 {
			t.Errorf("q=%q (grafia CORRETA) devolveu 0 — escrever certo não pode achar nada", p.comAcento)
		}
		if com.Meta.Total != sem.Meta.Total {
			t.Errorf("q=%q deu %d e q=%q deu %d — com e sem acento têm que convergir para o mesmo lexema",
				p.comAcento, com.Meta.Total, p.semAcento, sem.Meta.Total)
		}
	}
}

// ============================================================================
// Erro de grafia — a cascata para similaridade
// ============================================================================

// REGRESSÃO (pedido do dono): "pesquisas a produtos com erro de grafia devem
// ser resolvidos e achar sugestões na lista". O cliente digita no celular e o
// vendedor digita com pressa no balcão — errar é o caso comum, não a exceção.
//
// ⚠️ `similarity()` (o `%` do pg_trgm) NÃO resolveria nenhum destes: medido em
// 0,219 / 0,205 / 0,135, todos abaixo do corte de 0,3, porque ele divide pelos
// trigramas do nome INTEIRO. Quem resolve é strict_word_similarity, que compara
// com a melhor extensão de palavras do alvo.
func TestBusca_ErroDeGrafiaAchaOProduto(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	// "Veda Calha" não existe no seed de dev — semeio pra que o caso do dono
	// seja realmente exercitado, e não passe por acidente (ou falhe por
	// ausência de dado, que é pior: parece bug de busca e é falta de produto).
	seller := ""
	if err := db.QueryRow(`SELECT id FROM sellers LIMIT 1`).Scan(&seller); err == nil {
		_, _ = db.Exec(`INSERT INTO products (slug,name,category_id,seller_id,price,currency,icon,stock,status,description)
		                VALUES ('zz-veda-calha','Veda Calha Cinza 400g','construcao',$1,29.9,'BRL','⚒',10,'published','Vedante para calha.')
		                ON CONFLICT (slug) DO NOTHING`, seller)
		defer func() { _, _ = db.Exec(`DELETE FROM products WHERE slug='zz-veda-calha'`) }()
	}

	// ⚠️ Cada caso é anotado com o ESTÁGIO da cascata que o resolve, porque
	// isso é resultado de medição e é contraintuitivo:
	//
	// Vários "erros de grafia" são resolvidos já no estágio EXATO, porque o
	// stemmer português corta o sufixo errado junto com o certo — "furadera"
	// vira o lema 'furad', e o prefixo `furad:*` casa 'furadeir'. Não é sorte,
	// é o stemmer fazendo o trabalho antes da similaridade precisar entrar.
	//
	// O que o teste garante é o RESULTADO (achou o produto), não o caminho —
	// travar o caminho tornaria o teste frágil a qualquer ajuste do stemmer.
	casos := []struct{ errado, esperado string }{
		{"furadera", "Furadeira"},            // resolvido no estágio 2 (prefixo do lema)
		{"argamasa", "Argamassa"},            // resolvido no estágio 2
		{"vuradeira", "Furadeira"},           // letra adjacente no teclado → estágio 3
		{"cimeto", "Cimento"},                // estágio 3
		{"tijollo", "Tijolo"},                // estágio 3
		{"paraffuso", "Parafuso"},            // estágio 3
		{"disjutor", "Disjuntor"},            // estágio 3
		{"vedacalha", "Veda Calha"},          // estágio 3 (palavras coladas)
		{"veda calha", "Veda Calha"},         // grafia separada
		{"disjuntor bipolat 40a", "Bipolar"}, // multi-termo com 1 palavra errada
	}
	for _, c := range casos {
		res := buscar(t, r, c.errado)
		if res.Meta.Total == 0 {
			t.Errorf("q=%q devolveu 0 resultados — erro de grafia não foi corrigido", c.errado)
			continue
		}
		if !achou(res, c.esperado) {
			t.Errorf("q=%q achou %d resultados mas nenhum contém %q", c.errado, res.Meta.Total, c.esperado)
		}
	}
}

// REGRESSÃO DO LIMIAR (0,42). O valor foi escolhido por varredura, e a
// varredura mostrou dois precipícios muito próximos: em 0,45 o recall cai
// (perde "tomda"), em 0,38 começa a entrar lixo. Este teste trava os DOIS
// lados, porque mexer no limiar sem medir é o jeito mais fácil de estragar a
// busca achando que está melhorando.
func TestRegressao_LimiarDoFuzzyNaoPodeSerMexidoSemMedir(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	// Lado do RECALL: "tomda" mede 0,444. Subir o limiar para 0,45 (ou para o
	// default 0,5 do Postgres) perde este caso.
	if res := buscar(t, r, "tomda"); !achou(res, "Tomada") {
		t.Errorf("q=\"tomda\" não achou Tomada (total=%d) — limiar subiu demais? "+
			"o caso mede 0,444 e o corte é 0,42", res.Meta.Total)
	}

	// Lado da PRECISÃO: termo sem sentido nenhum não pode trazer produto.
	// A 0,38 a varredura já devolvia 6 resultados irrelevantes; a 0,35, trinta e oito.
	for _, lixo := range []string{"xyzqwkjhgf", "zzzzzz", "qwertyui", "bananarama", "foobarbaz"} {
		if res := buscar(t, r, lixo); res.Meta.Total != 0 {
			t.Errorf("q=%q (sem sentido) trouxe %d produtos — limiar caiu demais, "+
				"a busca aproximada virou gerador de lixo", lixo, res.Meta.Total)
		}
	}
}

// O payload PRECISA distinguir exato de aproximado. Sem isso o frontend finge
// que achou o que o usuário pediu, e o cliente compra o item errado achando
// que buscou certo.
func TestBusca_PayloadMarcaResultadoAproximadoESugereTermo(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	// "vuradeira" (e não "furadera"): a primeira letra errada impede o stemmer
	// de salvar o caso no estágio 2, então ele chega MESMO no estágio 3 — que
	// é o que este teste precisa exercitar.
	res := buscar(t, r, "vuradeira")
	if !res.Meta.Approximate {
		t.Fatalf("meta.approximate deveria ser true — o resultado veio da busca por similaridade (total=%d)",
			res.Meta.Total)
	}
	if res.Meta.Suggestion == "" {
		t.Error("meta.suggestion vazia — o front não tem como dizer 'mostrando resultados para furadeira'")
	} else if !strings.Contains(strings.ToLower(res.Meta.Suggestion), "furadeira") {
		t.Errorf("meta.suggestion = %q, esperava conter 'furadeira'", res.Meta.Suggestion)
	}
}

// ⚠️ O TESTE MAIS IMPORTANTE DESTE ARQUIVO.
//
// Falso positivo do fuzzy é PIOR que o bug original: uma busca que já
// funcionava passar a devolver lixo aproximado destrói a confiança na loja
// inteira. A cascata só pode cair na similaridade quando o passo exato veio
// VAZIO — nunca "sempre".
func TestRegressao_BuscaCorretaNuncaViraAproximada(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	corretas := []string{
		"furadeira", "parafuso", "cimento", "argamassa", "tijolo",
		"disjuntor", "tinta acrilica", "arame galvanizado", "massa acrilica",
	}
	for _, termo := range corretas {
		res := buscar(t, r, termo)
		if res.Meta.Total == 0 {
			t.Errorf("q=%q (termo CORRETO) devolveu 0 resultados", termo)
			continue
		}
		if res.Meta.Approximate {
			t.Errorf("q=%q (termo CORRETO) foi marcado como APROXIMADO — a cascata caiu no fuzzy sem precisar", termo)
		}
		if res.Meta.Suggestion != "" {
			t.Errorf("q=%q (termo CORRETO) veio com sugestão %q — 'você quis dizer' num termo certo faz a loja parecer quebrada",
				termo, res.Meta.Suggestion)
		}
	}
}

// Termo que não é erro de grafia de nada: tem que devolver vazio, e vazio NÃO
// é resultado aproximado. Marcar como aproximado uma lista vazia seria mentir.
func TestBusca_TermoInexistenteDevolveVazioSemMarcarAproximado(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	res := buscar(t, r, "xyzqwkjhgf")
	if res.Meta.Total != 0 {
		t.Errorf("termo sem sentido devolveu %d resultados", res.Meta.Total)
	}
	if res.Meta.Approximate {
		t.Error("lista vazia não pode vir marcada como aproximada — não houve aproximação nenhuma")
	}
}

// ============================================================================
// Balcão / PDV — NÃO PODE REGREDIR
// ============================================================================

// ⚠️ Código de barras é EXATO ou nada. Fuzzy aqui venderia o produto errado,
// com o preço errado, para um cliente que está no caixa. O leitor não erra a
// digitação — se não bateu, é outro produto.
func TestRegressao_CodigoDeBarrasNuncaEhAproximado(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	var barcode string
	err := db.QueryRow(`SELECT barcode FROM products WHERE barcode IS NOT NULL AND status='published' LIMIT 1`).Scan(&barcode)
	if err != nil {
		t.Skipf("sem produto com código de barras no seed: %v", err)
	}

	// Exato: acha exatamente 1.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?barcode="+barcode, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("barcode exato: status %d", w.Code)
	}
	var exato buscaResultado
	_ = json.Unmarshal(w.Body.Bytes(), &exato)
	if exato.Meta.Total != 1 {
		t.Errorf("barcode exato %q devolveu %d produtos, esperava 1", barcode, exato.Meta.Total)
	}

	// Um dígito trocado: NADA. Nunca "o mais parecido".
	errado := []byte(barcode)
	if errado[len(errado)-1] == '9' {
		errado[len(errado)-1] = '8'
	} else {
		errado[len(errado)-1]++
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?barcode="+string(errado), nil))
	var quase buscaResultado
	_ = json.Unmarshal(w.Body.Bytes(), &quase)
	if quase.Meta.Total != 0 {
		t.Errorf("código de barras com 1 dígito trocado devolveu %d produtos — TEM que ser 0. "+
			"Aproximar código de barras vende o produto errado no caixa.", quase.Meta.Total)
	}
	if quase.Meta.Approximate {
		t.Error("busca por código de barras NUNCA pode cair na cascata aproximada")
	}
}

// SKU exato pelo parâmetro dedicado continua sendo lookup de índice único.
func TestRegressao_SKUExatoContinuaExato(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	var sku string
	if err := db.QueryRow(`SELECT sku FROM products WHERE sku IS NOT NULL AND status='published' LIMIT 1`).Scan(&sku); err != nil {
		t.Skipf("sem SKU no seed: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products?sku="+url.QueryEscape(sku), nil))
	var res buscaResultado
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Meta.Total != 1 {
		t.Errorf("sku=%q devolveu %d, esperava exatamente 1", sku, res.Meta.Total)
	}

	// E pelo `q` geral, por PREFIXO — o vendedor digita o começo do código.
	if len(sku) > 4 {
		res := buscar(t, r, sku[:len(sku)-2])
		if res.Meta.Total == 0 {
			t.Errorf("prefixo de SKU %q não achou nada pelo `q` geral", sku[:len(sku)-2])
		}
	}
}

// ============================================================================
// Entrada hostil ponta a ponta — não pode virar 500 nem travar
// ============================================================================

// REGRESSÃO (audit CT1-C1, superfície nova): com tsvector o risco deixou de ser
// wildcard no pg_trgm e passou a ser SINTAXE — `to_tsquery` lança exceção com
// `&&&`, e exceção no banco vira 500. `websearch_to_tsquery` é tolerante, e o
// ramo de prefixo só recebe tokens que nós geramos.
//
// O teste prova as DUAS coisas: não vira 500 E não trava.
func TestSegurancaBusca_EntradaHostilNaoVira500NemTrava(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	hostis := []string{
		"&&&", "|||", "!!!", "((((((((((((((((", "a & b | !c", "<->",
		"%_%_%_%_%_%_%_%_%_%_%_%", // payload de ReDoS do pg_trgm
		`\`, `\\\\\\\\`, `''''''`, `";--`,
		"'; DROP TABLE products; --",
		"1' OR '1'='1",
		strings.Repeat("a", 5000),         // termo gigante (cap de 100 runes atua)
		strings.Repeat("furadeira ", 200), // muitos termos (cap de 8 atua)
		strings.Repeat("&|!()", 400),
		"furadeira:*:*:*:*:*",
		"\x00\x01\x02\x03",
		"日本語のテスト",
		"—–…«»§¶",
		"-", "--", "\"", "\"\"\"", "\"aberta sem fechar",
	}

	for _, h := range hostis {
		inicio := time.Now()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
			"/api/v1/products?q="+url.QueryEscape(h), nil))

		if w.Code != http.StatusOK {
			t.Errorf("q=%.40q virou status %d (esperado 200) — %s",
				h, w.Code, w.Body.String()[:min(200, w.Body.Len())])
		}
		// Não travou: o pior caso legítimo (fuzzy em 400 produtos) fica muito
		// abaixo disso. Um ReDoS estouraria com folga.
		if d := time.Since(inicio); d > 5*time.Second {
			t.Errorf("q=%.40q levou %v — busca travando é DoS", h, d)
		}
	}
}

// Busca vazia = listagem normal. Não pode virar erro nem sumir com a vitrine.
func TestBusca_TermoVazioDevolveListagemNormal(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	for _, u := range []string{
		"/api/v1/products",
		"/api/v1/products?q=",
		"/api/v1/products?q=%20%20%20",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, u, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", u, w.Code)
		}
		var res buscaResultado
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Meta.Total == 0 {
			t.Errorf("%s: busca vazia devolveu vitrine vazia", u)
		}
		if res.Meta.Approximate {
			t.Errorf("%s: busca sem termo não pode ser aproximada", u)
		}
	}
}

// ============================================================================
// Vendedor desnormalizado — a consistência do search_vector
// ============================================================================

// REGRESSÃO: `seller_name_cache` é denormalização, e denormalização
// desatualizada não dá erro — dá RESULTADO ERRADO. Se o gatilho de propagação
// sumir, a busca por fornecedor continua achando pelo nome ANTIGO da loja
// (que não existe mais) e não acha pelo novo, silenciosamente.
func TestRegressao_RenomearVendedorAtualizaOVetorDeBusca(t *testing.T) {
	db, r := searchRouter(t)
	defer db.Close()

	var id, original string
	if err := db.QueryRow(`SELECT s.id, s.name FROM sellers s
	                       JOIN products p ON p.seller_id = s.id AND p.status='published'
	                       LIMIT 1`).Scan(&id, &original); err != nil {
		t.Skipf("sem vendedor com produto publicado: %v", err)
	}
	defer func() { _, _ = db.Exec(`UPDATE sellers SET name=$1 WHERE id=$2`, original, id) }()

	const novo = "Zzferramentaria Renomeada Teste"
	if _, err := db.Exec(`UPDATE sellers SET name=$1 WHERE id=$2`, novo, id); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	// O nome NOVO tem que achar os produtos dele.
	if res := buscar(t, r, "Zzferramentaria"); res.Meta.Total == 0 {
		t.Error("depois de renomear o vendedor, a busca pelo nome NOVO não acha nada — " +
			"o gatilho trg_sellers_name_to_products não propagou para search_vector")
	}

	// E o cache tem que refletir o novo nome em todos os produtos.
	var desatualizados int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE seller_id=$1 AND seller_name_cache IS DISTINCT FROM $2`,
		id, novo).Scan(&desatualizados); err != nil {
		t.Fatalf("conferir cache: %v", err)
	}
	if desatualizados != 0 {
		t.Errorf("%d produtos ficaram com seller_name_cache desatualizado", desatualizados)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
