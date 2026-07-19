package knowledge_test

import (
	"strings"
	"testing"

	"github.com/utilar/assistant-service/internal/knowledge"
)

// A base tem que CARREGAR E VALIDAR no boot. Se este teste quebra, o serviço
// não sobe — que é exatamente o comportamento desejado: coeficiente sem fonte
// ou material órfão precisa falhar alto, não passar batido para o cliente.
func TestLoad_BaseValidaNoBoot(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatalf("base de conhecimento não validou no boot: %v", err)
	}
	if len(kb.Servicos()) == 0 {
		t.Fatal("nenhum serviço carregado")
	}
}

// Cobertura mínima exigida pelo escopo do produto. Se alguém remover um serviço
// da base, isto acusa.
func TestLoad_CoberturaDeServicos(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatal(err)
	}
	obrigatorios := []string{
		"alvenaria", "contrapiso", "chapisco", "reboco",
		"assentar-piso", "revestir-parede-ceramica",
		"pintura-interna", "pintura-externa", "textura",
		"eletrica-basica", "hidraulica-basica",
		"telhado", "impermeabilizacao", "drywall", "gesso-liso",
		"concretagem-simples",
	}
	for _, id := range obrigatorios {
		if _, ok := kb.Servico(id); !ok {
			t.Errorf("serviço obrigatório ausente da base: %q", id)
		}
	}
}

// Regra inegociável: TODO coeficiente carrega fonte. Sem fonte a Alice não
// consegue dizer de onde tirou o número, e número sem procedência é chute.
func TestLoad_TodoCoeficienteTemFonte(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range kb.Servicos() {
		if s.Fonte.Ref == "" || s.Fonte.Nota == "" {
			t.Errorf("serviço %s sem fonte completa", s.ID)
		}
		for _, c := range s.Consumos {
			if c.Coef.Fonte.Ref == "" {
				t.Errorf("%s/%s: coeficiente sem fonte", s.ID, c.MaterialID)
			}
			if c.Coef.Fonte.Nota == "" {
				t.Errorf("%s/%s: fonte sem nota explicando o que cobre", s.ID, c.MaterialID)
			}
			if c.Coef.Min <= 0 || c.Coef.Max < c.Coef.Min {
				t.Errorf("%s/%s: faixa inválida %v–%v", s.ID, c.MaterialID, c.Coef.Min, c.Coef.Max)
			}
		}
	}
}

// Anti-"inventar norma": só é NBR/NR o que está marcado como tipo=norma, e todo
// tipo=norma tem que citar algo que se pareça com norma. O Load já valida isso;
// aqui garantimos que nenhuma nota de tipo não-normativo cite NBR de contrabando.
func TestLoad_NaoInventaNorma(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatal(err)
	}
	check := func(where string, s knowledge.Source) {
		if s.Tipo == knowledge.KindNorma {
			u := strings.ToUpper(s.Ref)
			if !strings.Contains(u, "NBR") && !strings.Contains(u, "NR-") {
				t.Errorf("%s: tipo=norma mas ref %q não é norma", where, s.Ref)
			}
		}
		if s.Tipo == knowledge.KindMercado && strings.Contains(strings.ToUpper(s.Ref), "NBR") {
			t.Errorf("%s: ref de mercado citando NBR: %q", where, s.Ref)
		}
	}
	for _, s := range kb.Servicos() {
		check("servico "+s.ID, s.Fonte)
		for _, c := range s.Consumos {
			check(s.ID+"/"+c.MaterialID, c.Coef.Fonte)
		}
	}
}

// Cada material referenciado por um serviço tem que existir e ser comprável:
// sem unidade de venda não dá para arredondar, e sem termo de busca a Alice não
// acha o produto real no catálogo.
func TestLoad_MateriaisSaoCompraveis(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range kb.Servicos() {
		for _, c := range s.Consumos {
			m, ok := kb.Material(c.MaterialID)
			if !ok {
				t.Fatalf("%s referencia material inexistente %q", s.ID, c.MaterialID)
			}
			if m.ConteudoVend <= 0 {
				t.Errorf("material %s sem conteudo_venda", m.ID)
			}
			if m.BuscaCatalogo == "" {
				t.Errorf("material %s sem busca_catalogo", m.ID)
			}
		}
	}
}

func TestResolveServico_PorAliasENome(t *testing.T) {
	kb, err := knowledge.Load()
	if err != nil {
		t.Fatal(err)
	}
	casos := map[string]string{
		"alvenaria":           "alvenaria",
		"levantar parede":     "alvenaria",
		"muro":                "alvenaria",
		"contrapiso":          "contrapiso",
		"assentar piso":       "assentar-piso",
		"porcelanato":         "assentar-piso",
		"azulejo":             "revestir-parede-ceramica",
		"pintar":              "pintura-interna",
		"drywall":             "drywall",
		"impermeabilizar":     "impermeabilizacao",
		"telhado":             "telhado",
		"CALÇADA":             "concretagem-simples",
		"instalação elétrica": "eletrica-basica",
		"encanamento":         "hidraulica-basica",
	}
	for termo, want := range casos {
		s, ok := kb.ResolveServico(termo)
		if !ok {
			t.Errorf("não resolveu %q", termo)
			continue
		}
		if s.ID != want {
			t.Errorf("%q → %q, esperava %q", termo, s.ID, want)
		}
	}
}

func TestResolveServico_TermoDesconhecido(t *testing.T) {
	kb, _ := knowledge.Load()
	if s, ok := kb.ResolveServico("montar foguete espacial"); ok {
		t.Errorf("não deveria resolver termo fora da base, veio %q", s.ID)
	}
	if _, ok := kb.ResolveServico(""); ok {
		t.Error("string vazia não deveria resolver")
	}
}

func TestConversao_DiretaEInversa(t *testing.T) {
	kb, _ := knowledge.Load()

	c, ok := kb.Conversao("saco de cimento", "kg")
	if !ok || c.Fator != 50 {
		t.Fatalf("saco de cimento → kg deveria ser 50, veio %v (ok=%v)", c.Fator, ok)
	}

	inv, ok := kb.Conversao("kg", "saco de cimento")
	if !ok {
		t.Fatal("conversão inversa deveria existir")
	}
	if got := 1 / inv.Fator; got != 50 {
		t.Errorf("inversa inconsistente: 1/%v = %v, esperava 50", inv.Fator, got)
	}
}

func TestConversao_Desconhecida(t *testing.T) {
	kb, _ := knowledge.Load()
	if _, ok := kb.Conversao("jubileu", "kg"); ok {
		t.Error("unidade inventada não pode converter")
	}
	if len(kb.UnidadesConhecidas()) == 0 {
		t.Error("deveria listar as unidades conhecidas para dar erro útil")
	}
}
