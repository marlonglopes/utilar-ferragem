package ledger

import (
	"strings"
	"testing"
)

// REGRESSÃO: separador decimal do CSV é VÍRGULA (Excel pt-BR) e o do OFX é
// PONTO (padrão americano). Trocar os dois é o erro clássico e produz um
// extrato com valores mil vezes maiores no sistema do contador.
func TestSeparadorDecimalCSVeOFXNaoSeMisturam(t *testing.T) {
	if got := reaisBR(128450); got != "1284,50" {
		t.Errorf("CSV precisa de vírgula decimal: %q", got)
	}
	if got := ofxAmount(128450); got != "1284.50" {
		t.Errorf("OFX precisa de ponto decimal: %q", got)
	}
}

// Sem separador de milhar: "1.284,50" quebra a importação de vários sistemas
// contábeis brasileiros que leem o ponto como decimal.
func TestCSVNaoUsaSeparadorDeMilhar(t *testing.T) {
	if got := reaisBR(123456789); strings.Contains(got, ".") {
		t.Fatalf("valor com separador de milhar: %q", got)
	}
}

func TestFormatacaoDeValoresNegativosEZero(t *testing.T) {
	casos := []struct {
		c        Cents
		csv, ofx string
	}{
		{0, "0,00", "0.00"},
		{5, "0,05", "0.05"},
		{-3490, "-34,90", "-34.90"},
		{100, "1,00", "1.00"},
	}
	for _, tc := range casos {
		if got := reaisBR(tc.c); got != tc.csv {
			t.Errorf("reaisBR(%d) = %q, esperado %q", tc.c, got, tc.csv)
		}
		if got := ofxAmount(tc.c); got != tc.ofx {
			t.Errorf("ofxAmount(%d) = %q, esperado %q", tc.c, got, tc.ofx)
		}
	}
}

// OFX 1.0.2 é SGML: uma descrição com "<" quebraria o parser do sistema do
// contador — e um `<` vindo de descrição controlada pelo usuário seria injeção
// de tags no arquivo entregue.
func TestOFXEscapaCaracteresDeMarcacao(t *testing.T) {
	got := ofxEscape(`Pedido <script>alerta & "cia"`)
	for _, proibido := range []string{"<script>", " & "} {
		if strings.Contains(got, proibido) {
			t.Errorf("caractere de marcação não escapado em %q", got)
		}
	}
	if !strings.Contains(got, "&lt;") || !strings.Contains(got, "&amp;") {
		t.Errorf("escape não aplicado: %q", got)
	}
}

func TestOFXEscapaQuebraDeLinha(t *testing.T) {
	// SGML do OFX 1.0.2 é sensível a quebra de linha dentro de tag.
	if strings.Contains(ofxEscape("linha1\nlinha2"), "\n") {
		t.Fatal("quebra de linha sobreviveu ao escape")
	}
}

func TestTraducoesParaOContador(t *testing.T) {
	if naturezaPT(KindSale) != "Venda" || naturezaPT(KindPSPFee) != "Taxa do gateway" {
		t.Error("natureza do lançamento tem que sair em português")
	}
	if metodoPT("card") != "Cartao de credito" {
		t.Error("forma de pagamento tem que sair em português")
	}
	if tipoContaPT("liability") != "Passivo" {
		t.Error("tipo de conta tem que sair em português")
	}
	// Valor desconhecido não pode virar string vazia — o contador precisa ver
	// que existe algo que ele não reconhece, não uma célula em branco.
	if naturezaPT("kind_novo") != "kind_novo" {
		t.Error("kind desconhecido deve sair cru, não vazio")
	}
}

func TestPlanoDeContasEhConsistente(t *testing.T) {
	vistos := map[Account]bool{}
	for _, m := range ChartOfAccounts {
		if vistos[m.Code] {
			t.Errorf("conta duplicada no plano: %s", m.Code)
		}
		vistos[m.Code] = true
		if m.Name == "" || m.Type == "" {
			t.Errorf("conta %s incompleta", m.Code)
		}
		if m.NormalSide != Debit && m.NormalSide != Credit {
			t.Errorf("conta %s com natureza inválida %q", m.Code, m.NormalSide)
		}
		// Ativo e despesa são devedoras; passivo e receita, credoras — exceto
		// as REDUTORAS de receita (estornos, chargebacks), que são devedoras
		// de propósito.
		switch m.Type {
		case "asset", "expense":
			if m.NormalSide != Debit {
				t.Errorf("%s (%s) deveria ser devedora", m.Code, m.Type)
			}
		case "liability":
			if m.NormalSide != Credit {
				t.Errorf("%s (passivo) deveria ser credora", m.Code)
			}
		}
	}
	if _, ok := MetaOf("9.9.9"); ok {
		t.Error("MetaOf devolveu conta inexistente")
	}
}
