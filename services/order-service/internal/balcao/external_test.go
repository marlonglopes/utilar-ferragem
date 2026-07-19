package balcao_test

import (
	"errors"
	"testing"

	"github.com/utilar/order-service/internal/balcao"
)

// ============================================================================
// Liquidação externa: quem pode dizer "pagou" sem dinheiro nenhum ter entrado.
//
// Estes testes rodam sem banco, sem HTTP e sem auth-service. É de propósito:
// são a barreira que separa "venda registrada" de "mercadoria entregue de
// graça", e barreira que só é testada quando o Postgres está de pé é barreira
// que alguém pula no dia de pressa.
// ============================================================================

func balcaoOrder() balcao.OrderRef {
	return balcao.OrderRef{
		OwnerUserID:    "op-a",
		Channel:        balcao.ChannelBalcao,
		StoreID:        "loja-a",
		OperatorID:     "op-a",
		ApprovalStatus: balcao.ApprovalNotRequired,
	}
}

func operator(store string) balcao.Actor {
	return balcao.Actor{UserID: "op-a", Role: balcao.RoleStoreOperator, StoreID: store}
}

// ---------------------------------------------------------------------------
// O TESTE QUE IMPORTA: cliente e anônimo são recusados.
// ---------------------------------------------------------------------------

func TestRegression_ClienteNaoLiquidaOProprioPedido(t *testing.T) {
	// O cenário: o cliente da loja ONLINE descobre a rota e chama com o próprio
	// token, no próprio pedido, do qual ele é o dono. Se isto passar, qualquer
	// pessoa com uma conta leva mercadoria de graça.
	cliente := balcao.Actor{UserID: "cli-1", Role: balcao.RoleCustomer}

	proprioPedido := balcaoOrder()
	proprioPedido.OwnerUserID = "cli-1"

	if err := balcao.CanSettleExternal(cliente, proprioPedido); !errors.Is(err, balcao.ErrNotSettler) {
		t.Fatalf("cliente liquidou o PRÓPRIO pedido — mercadoria sai de graça. err = %v", err)
	}
}

func TestRegression_AnonimoNaoLiquida(t *testing.T) {
	// Ator zerado é o que sobra quando o middleware de autenticação é
	// contornado, ou quando um handler futuro esquece de exigir usuário.
	// O default do switch de papéis precisa ser RECUSAR.
	if err := balcao.CanSettleExternal(balcao.Actor{}, balcaoOrder()); !errors.Is(err, balcao.ErrNotSettler) {
		t.Fatalf("ator anônimo (papel vazio) liquidou pedido: err = %v", err)
	}
}

func TestRegression_PapeisSemPoderDeLiquidar(t *testing.T) {
	// Nenhum destes papéis pode declarar dinheiro recebido. `seller` merece
	// destaque: lojista do marketplace NÃO é vendedor de balcão, e liberar para
	// ele seria deixar cada lojista dar baixa nas próprias vendas.
	for _, role := range []string{
		balcao.RoleCustomer, balcao.RoleSeller, balcao.RoleService,
		"", " ", "Admin", "administrator", "store_operator ", "papel_novo_do_futuro",
	} {
		t.Run("papel="+role, func(t *testing.T) {
			a := balcao.Actor{UserID: "x", Role: role, StoreID: "loja-a"}
			if err := balcao.CanSettleExternal(a, balcaoOrder()); err == nil {
				t.Fatalf("papel %q liquidou pedido externamente", role)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Escopo de loja
// ---------------------------------------------------------------------------

func TestOperadorLiquidaSomenteNaPropriaLoja(t *testing.T) {
	if err := balcao.CanSettleExternal(operator("loja-a"), balcaoOrder()); err != nil {
		t.Fatalf("operador da própria loja deveria liquidar: %v", err)
	}
	err := balcao.CanSettleExternal(operator("loja-b"), balcaoOrder())
	if !errors.Is(err, balcao.ErrForeignStore) {
		t.Fatalf("operador de outra loja liquidou venda alheia: err = %v", err)
	}
}

func TestOperadorSemVinculoNaoLiquida(t *testing.T) {
	// Vínculo revogado depois da emissão do token: fail-closed, sem loja não há
	// liquidação. O token velho não pode continuar valendo como passe da filial.
	err := balcao.CanSettleExternal(operator(""), balcaoOrder())
	if !errors.Is(err, balcao.ErrNoStoreBinding) {
		t.Fatalf("operador sem loja liquidou: err = %v", err)
	}
}

func TestAdminLiquidaEmQualquerLoja(t *testing.T) {
	admin := balcao.Actor{UserID: "adm", Role: balcao.RoleAdmin}
	if err := balcao.CanSettleExternal(admin, balcaoOrder()); err != nil {
		t.Fatalf("admin deveria liquidar em qualquer loja: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pedido web nunca é liquidável por fora
// ---------------------------------------------------------------------------

func TestRegression_PedidoWebNuncaEhLiquidadoPorFora(t *testing.T) {
	web := balcaoOrder()
	web.Channel = balcao.ChannelWeb

	// Nem operador, nem admin: a venda do site se paga pelo PSP. Esta checagem
	// vem antes da de papel de propósito — se um dia a regra de papel for
	// afrouxada, o pedido web continua exigindo dinheiro de verdade.
	for _, a := range []balcao.Actor{
		operator("loja-a"),
		{UserID: "adm", Role: balcao.RoleAdmin},
	} {
		if err := balcao.CanSettleExternal(a, web); !errors.Is(err, balcao.ErrNotBalcaoOrder) {
			t.Fatalf("papel %q liquidou pedido WEB por fora: err = %v", a.Role, err)
		}
	}

	// Canal vazio (pedido histórico, anterior à migration do balcão) conta como
	// web — nunca como balcão.
	semCanal := balcaoOrder()
	semCanal.Channel = ""
	if err := balcao.CanSettleExternal(operator("loja-a"), semCanal); !errors.Is(err, balcao.ErrNotBalcaoOrder) {
		t.Fatalf("pedido sem canal (histórico) foi tratado como balcão: err = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Desconto pendente/recusado trava a liquidação
// ---------------------------------------------------------------------------

func TestDescontoPendenteOuRecusadoBloqueiaLiquidacao(t *testing.T) {
	// Sem isto, a fila de aprovação vira decoração: bastaria dar 40% e cobrar
	// na maquininha antes de o gerente ver.
	pend := balcaoOrder()
	pend.ApprovalStatus = balcao.ApprovalPending
	if err := balcao.CanSettleExternal(operator("loja-a"), pend); !errors.Is(err, balcao.ErrApprovalPending) {
		t.Errorf("desconto pendente foi liquidado: err = %v", err)
	}

	rej := balcaoOrder()
	rej.ApprovalStatus = balcao.ApprovalRejected
	if err := balcao.CanSettleExternal(operator("loja-a"), rej); !errors.Is(err, balcao.ErrApprovalRejected) {
		t.Errorf("desconto RECUSADO foi liquidado: err = %v", err)
	}

	apr := balcaoOrder()
	apr.ApprovalStatus = balcao.ApprovalApproved
	if err := balcao.CanSettleExternal(operator("loja-a"), apr); err != nil {
		t.Errorf("desconto aprovado deveria liquidar: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NSU
// ---------------------------------------------------------------------------

func TestNormalizeNSU(t *testing.T) {
	casos := []struct {
		entrada string
		quer    string
		erro    bool
	}{
		{"004417", "004417", false},
		{"  004417  ", "004417", false},
		{"0044-17", "004417", false},
		{"00 44 17", "004417", false},
		{"ab12cd", "AB12CD", false}, // maiúsculas: busca por igualdade exata
		{"", "", true},              // sem NSU não há conciliação possível
		{"   ", "", true},
		{"12", "", true}, // curto demais para ser um NSU real
		{"004417; DROP TABLE orders", "", true},
		{"004417\n004418", "", true},
		{"ÇÃO", "", true},
		{"123456789012345678901234567890123456", "", true}, // longo demais
	}
	for _, tc := range casos {
		got, err := balcao.NormalizeNSU(tc.entrada)
		if tc.erro {
			if err == nil {
				t.Errorf("NormalizeNSU(%q) deveria falhar, veio %q", tc.entrada, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeNSU(%q): %v", tc.entrada, err)
		}
		if got != tc.quer {
			t.Errorf("NormalizeNSU(%q) = %q, quero %q", tc.entrada, got, tc.quer)
		}
	}
}

// ---------------------------------------------------------------------------
// Idempotência
// ---------------------------------------------------------------------------

func TestIdempotenciaDaLiquidacao(t *testing.T) {
	// Ainda não liquidado: segue.
	settled, err := balcao.CheckSettlementIdempotency("", "004417")
	if settled || err != nil {
		t.Errorf("pedido novo: settled=%v err=%v", settled, err)
	}

	// Retry com o MESMO NSU: no-op silencioso. É o caso real — a rede caiu
	// depois de gravar, o PDV tentou de novo. Não pode virar segundo
	// lançamento contábil.
	settled, err = balcao.CheckSettlementIdempotency("004417", "004417")
	if !settled || err != nil {
		t.Errorf("retry com mesmo NSU: settled=%v err=%v", settled, err)
	}

	// O NSU normalizado é o que entra na comparação — "0044-17" e "004417" são
	// o mesmo comprovante e não podem virar duas liquidações.
	norm, _ := balcao.NormalizeNSU("0044-17")
	settled, err = balcao.CheckSettlementIdempotency("004417", norm)
	if !settled || err != nil {
		t.Errorf("mesmo NSU com formatação diferente virou nova liquidação: settled=%v err=%v", settled, err)
	}

	// NSU DIFERENTE no mesmo pedido: dois comprovantes para uma venda só —
	// possível cobrança em duplicidade no cartão do cliente. Recusa, e o NSU
	// original (a prova do primeiro comprovante) nunca é sobrescrito.
	settled, err = balcao.CheckSettlementIdempotency("004417", "009999")
	if !settled || !errors.Is(err, balcao.ErrNSUMismatch) {
		t.Errorf("segundo NSU no mesmo pedido: settled=%v err=%v", settled, err)
	}
}

// ---------------------------------------------------------------------------
// Bandeira
// ---------------------------------------------------------------------------

func TestNormalizeBrand(t *testing.T) {
	if b, err := balcao.NormalizeBrand("  VISA "); err != nil || b != "visa" {
		t.Errorf("NormalizeBrand(VISA) = %q, %v", b, err)
	}
	// Vazio é aceito: nem todo comprovante traz bandeira, e o NSU é o que
	// realmente amarra a venda ao extrato.
	if b, err := balcao.NormalizeBrand(""); err != nil || b != "" {
		t.Errorf("bandeira vazia deveria ser aceita, veio %q, %v", b, err)
	}
	// Desconhecida falha AGORA, e não no fechamento do mês com a coluna de
	// bandeira cheia de variação de digitação.
	if _, err := balcao.NormalizeBrand("vsia"); err == nil {
		t.Error("bandeira desconhecida deveria falhar")
	}
}
