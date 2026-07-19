package review

import "testing"

// A política de moderação é "publica, mas segura o suspeito". Os dois lados
// erram caro e por motivos opostos:
//
//   • deixar passar spam → texto de venda de concorrente na página do produto;
//   • segurar demais → avaliação técnica honesta some, e como a fila é revisada
//     devagar numa loja sem equipe, "some" na prática quer dizer "some".
//
// Por isso os falsos positivos têm tantos casos quanto os verdadeiros.
func TestClassify(t *testing.T) {
	casos := []struct {
		nome  string
		title string
		body  string
		want  string
	}{
		// --- deve PUBLICAR -------------------------------------------------
		{"só estrela, sem texto", "", "", StatusPublished},
		{"elogio comum", "Ótima", "Furadeira boa, pegou bem no concreto.", StatusPublished},
		{"crítica dura", "Não recomendo", "Veio amassado e o motor esquenta muito.", StatusPublished},
		{"reação curta em caixa alta", "", "PÉSSIMO", StatusPublished},
		// ⚠️ Estes são os falsos positivos que custam caro: avaliação TÉCNICA é a
		// mais útil da loja, e ela é cheia de número e sigla. Se a triagem
		// mandar isto para a fila, ela está punindo exatamente o bom conteúdo.
		{"specs elétricas", "", "Usei com cabo 2,5mm² 750V, aguentou bem.", StatusPublished},
		{"modelo com números", "", "A GSB 13 RE 650W é melhor que a de 500W.", StatusPublished},
		{"medidas", "", "Cortei 2,5 m de cano de 3/4 sem problema.", StatusPublished},
		{"sigla em caixa alta no meio", "", "A argamassa AC-III colou muito melhor que a AC-II aqui em casa.", StatusPublished},

		// --- deve ir para a FILA -------------------------------------------
		{"link http", "", "veja em http://outraloja.com", StatusPending},
		{"domínio sem protocolo", "", "compre em melhorpreco.com.br", StatusPending},
		{"e-mail", "", "me chama em joao@gmail.com", StatusPending},
		{"telefone formatado", "", "faço mais barato, (11) 91234-5678", StatusPending},
		{"telefone colado", "", "chama 11912345678", StatusPending},
		{"convite a contato", "", "me chama no zap que eu faço melhor", StatusPending},
		{"parágrafo gritado", "", "PRODUTO HORRIVEL NAO COMPREM DE JEITO NENHUM", StatusPending},
		{"repetição", "", "boooooooom demais", StatusPending},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			got := Classify(tc.title, tc.body)
			if got.Status != tc.want {
				t.Fatalf("Classify(%q, %q).Status = %q (nota: %q), want %q",
					tc.title, tc.body, got.Status, got.Note, tc.want)
			}
			if got.Status == StatusPending && got.Note == "" {
				t.Fatal("pendência sem nota — o admin precisa saber o que o robô viu")
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	casos := map[string]string{
		"Marlon Gomes Lopes": "Marlon L.",
		"Ana Silva":          "Ana S.",
		"Ana":                "Ana",
		"":                   "Cliente",
		"   ":                "Cliente",
	}
	for in, want := range casos {
		if got := DisplayName(in); got != want {
			t.Errorf("DisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateText(t *testing.T) {
	longo := make([]rune, MaxBodyLen+1)
	for i := range longo {
		longo[i] = 'a'
	}
	if ValidateText("ok", "ok") != "" {
		t.Error("texto normal foi rejeitado")
	}
	if ValidateText("", string(longo)) == "" {
		t.Error("corpo acima do limite passou — o CHECK do banco viraria 500 em vez de 400")
	}
}
