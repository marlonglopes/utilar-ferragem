package balcao

import (
	"errors"
	"testing"
)

// ============================================================================
// Testes de REGRESSÃO das três regras que sustentam a segurança do PDV.
//
// Rodam sem banco, sem HTTP e sem catalog — de propósito. Um teste que precisa
// de infra é um teste que alguém desliga na sexta-feira, e estas três regras
// são justamente as que não podem parar de ser verificadas.
// ============================================================================

func operatorAt(store string) Actor {
	return Actor{UserID: "op-1", Role: RoleStoreOperator, StoreID: store, Level: "operator", DiscountCeilingPct: 12}
}

func managerAt(store, userID string) Actor {
	return Actor{UserID: userID, Role: RoleStoreOperator, StoreID: store, Level: "manager",
		DiscountCeilingPct: 100, CanApprove: true}
}

// ---------------------------------------------------------------------------
// Regra 1 — operador só cria pedido da própria loja
// ---------------------------------------------------------------------------

func TestRegression_OperatorCannotCreateOrderForAnotherStore(t *testing.T) {
	op := operatorAt("loja-A")

	if _, err := CanCreateBalcaoOrder(op, "loja-B"); !errors.Is(err, ErrForeignStore) {
		t.Fatalf("operador da loja-A criando pedido na loja-B deveria falhar com ErrForeignStore, veio: %v", err)
	}

	// A loja do próprio vínculo continua funcionando.
	storeID, err := CanCreateBalcaoOrder(op, "loja-A")
	if err != nil || storeID != "loja-A" {
		t.Fatalf("operador deveria poder vender na própria loja: store=%q err=%v", storeID, err)
	}

	// Omitir a loja usa a do vínculo — nunca a do request.
	storeID, err = CanCreateBalcaoOrder(op, "")
	if err != nil || storeID != "loja-A" {
		t.Fatalf("loja omitida deveria cair na loja do vínculo: store=%q err=%v", storeID, err)
	}
}

func TestRegression_MarketplaceSellerIsNotStoreOperator(t *testing.T) {
	// O bug que o papel novo existe para impedir: `seller` = lojista que anuncia
	// no site. Se um dia alguém "simplificar" reusando seller para o PDV, este
	// teste quebra.
	seller := Actor{UserID: "s-1", Role: RoleSeller, StoreID: "loja-A"}
	if _, err := CanCreateBalcaoOrder(seller, "loja-A"); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("seller do marketplace não pode vender no balcão, veio: %v", err)
	}

	customer := Actor{UserID: "c-1", Role: RoleCustomer}
	if _, err := CanCreateBalcaoOrder(customer, "loja-A"); !errors.Is(err, ErrNotOperator) {
		t.Fatalf("customer não pode vender no balcão, veio: %v", err)
	}
}

func TestRegression_OperatorWithoutStoreBindingCannotSell(t *testing.T) {
	// Fail-closed: token de operador sem store_id (vínculo desativado depois da
	// emissão) não vende em lugar nenhum, nem "na loja que ele mandar".
	orphan := Actor{UserID: "op-x", Role: RoleStoreOperator}
	if _, err := CanCreateBalcaoOrder(orphan, ""); !errors.Is(err, ErrNoStoreBinding) {
		t.Fatalf("operador sem vínculo deveria falhar: %v", err)
	}
	if _, err := CanCreateBalcaoOrder(orphan, "loja-A"); !errors.Is(err, ErrNoStoreBinding) {
		t.Fatalf("operador sem vínculo não pode escolher loja pelo request: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Regra 2 — operador só vê pedidos da própria loja / cliente só vê os dele
// ---------------------------------------------------------------------------

func TestRegression_OperatorCannotViewOrderFromAnotherStore(t *testing.T) {
	op := operatorAt("loja-A")
	foreign := OrderRef{Channel: ChannelBalcao, StoreID: "loja-B", OperatorID: "op-9", OwnerUserID: "cli-9"}

	if err := CanViewOrder(op, foreign); !errors.Is(err, ErrForeignStore) {
		t.Fatalf("operador não pode ler pedido de outra loja, veio: %v", err)
	}

	own := OrderRef{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-9", OwnerUserID: "cli-9"}
	if err := CanViewOrder(op, own); err != nil {
		t.Fatalf("operador deveria ler pedido da própria loja (inclusive de outro vendedor): %v", err)
	}
}

func TestRegression_CustomerScopeIsNotWidenedByBalcao(t *testing.T) {
	// A mudança de `channel` não pode afrouxar o escopo do cliente comum. Antes,
	// toda query filtrava por user_id; se a autorização passar a olhar loja, o
	// cliente NÃO pode herdar visibilidade de loja nenhuma.
	cust := Actor{UserID: "cli-1", Role: RoleCustomer}

	cases := []struct {
		name  string
		order OrderRef
	}{
		{"pedido web de outro cliente", OrderRef{Channel: ChannelWeb, OwnerUserID: "cli-2"}},
		{"pedido de balcão de outro cliente", OrderRef{Channel: ChannelBalcao, StoreID: "loja-A", OwnerUserID: "cli-2", OperatorID: "op-1"}},
		{"pedido de balcão sem dono", OrderRef{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-1"}},
	}
	for _, tc := range cases {
		if err := CanViewOrder(cust, tc.order); !errors.Is(err, ErrNotOwner) {
			t.Errorf("%s: cliente não pode ler, veio: %v", tc.name, err)
		}
	}

	// E o pedido dele continua acessível.
	mine := OrderRef{Channel: ChannelWeb, OwnerUserID: "cli-1"}
	if err := CanViewOrder(cust, mine); err != nil {
		t.Fatalf("cliente deveria ler o próprio pedido: %v", err)
	}
}

func TestRegression_OperatorCannotViewWebOrdersOfCustomers(t *testing.T) {
	// Escopo de loja vale para vendas de balcão. O pedido web de um cliente
	// qualquer não vira visível só porque quem pergunta é operador.
	op := operatorAt("loja-A")
	webOrder := OrderRef{Channel: ChannelWeb, OwnerUserID: "cli-7", StoreID: ""}
	if err := CanViewOrder(op, webOrder); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("operador não pode ler pedido web de cliente, veio: %v", err)
	}

	// Mas os pedidos web DELE continuam dele.
	own := OrderRef{Channel: ChannelWeb, OwnerUserID: op.UserID}
	if err := CanViewOrder(op, own); err != nil {
		t.Fatalf("operador deveria ler o próprio pedido web: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Regra 3 — ninguém aprova o próprio desconto
// ---------------------------------------------------------------------------

func TestRegression_OperatorCannotApproveOwnDiscount(t *testing.T) {
	// O caso perigoso: o GERENTE que vendeu. Ele tem cargo de aprovador e está
	// na loja certa — só a checagem de identidade o impede.
	mgr := managerAt("loja-A", "mgr-1")
	ownSale := OrderRef{
		Channel: ChannelBalcao, StoreID: "loja-A",
		OperatorID: "mgr-1", ApprovalStatus: ApprovalPending,
	}
	if err := CanApproveOrder(mgr, ownSale); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("gerente não pode aprovar o próprio desconto, veio: %v", err)
	}

	// A venda de outra pessoa, na mesma loja, ele aprova.
	otherSale := OrderRef{
		Channel: ChannelBalcao, StoreID: "loja-A",
		OperatorID: "op-2", ApprovalStatus: ApprovalPending,
	}
	if err := CanApproveOrder(mgr, otherSale); err != nil {
		t.Fatalf("gerente deveria aprovar venda de outro operador: %v", err)
	}
}

func TestRegression_AdminCannotApproveOwnDiscountEither(t *testing.T) {
	// Separação de funções não abre exceção por cargo alto.
	admin := Actor{UserID: "adm-1", Role: RoleAdmin}
	ownSale := OrderRef{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "adm-1", ApprovalStatus: ApprovalPending}
	if err := CanApproveOrder(admin, ownSale); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("admin não pode aprovar a própria venda, veio: %v", err)
	}
}

func TestRegression_NonManagerCannotApprove(t *testing.T) {
	op := operatorAt("loja-A") // CanApprove=false
	sale := OrderRef{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-2", ApprovalStatus: ApprovalPending}
	if err := CanApproveOrder(op, sale); !errors.Is(err, ErrNotApprover) {
		t.Fatalf("operador comum não aprova desconto, veio: %v", err)
	}
}

func TestRegression_ManagerCannotApproveAnotherStore(t *testing.T) {
	mgr := managerAt("loja-A", "mgr-1")
	sale := OrderRef{Channel: ChannelBalcao, StoreID: "loja-B", OperatorID: "op-2", ApprovalStatus: ApprovalPending}
	if err := CanApproveOrder(mgr, sale); !errors.Is(err, ErrForeignStore) {
		t.Fatalf("gerente não aprova venda de outra loja, veio: %v", err)
	}
}

func TestApproveRequiresPendingOrder(t *testing.T) {
	mgr := managerAt("loja-A", "mgr-1")
	cases := []OrderRef{
		{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-2", ApprovalStatus: ApprovalApproved},
		{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-2", ApprovalStatus: ApprovalRejected},
		{Channel: ChannelBalcao, StoreID: "loja-A", OperatorID: "op-2", ApprovalStatus: ApprovalNotRequired},
		{Channel: ChannelWeb, OwnerUserID: "cli-1", ApprovalStatus: ApprovalPending},
	}
	for i, o := range cases {
		if err := CanApproveOrder(mgr, o); !errors.Is(err, ErrNothingToApprove) {
			t.Errorf("caso %d: esperado ErrNothingToApprove, veio %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Desconto — o servidor recalcula, o cliente não dita
// ---------------------------------------------------------------------------

func TestResolveDiscount_ServerRecalculatesAmount(t *testing.T) {
	// Mesmo que o PDV mande outro valor em reais, o que vale é o derivado do
	// subtotal autoritativo.
	d := ResolveDiscount(1000, 10, 12)
	if d.Amount != 100 {
		t.Errorf("desconto de 10%% sobre 1000 deveria ser 100, veio %v", d.Amount)
	}
	if d.NetSubtotal != 900 {
		t.Errorf("líquido deveria ser 900, veio %v", d.NetSubtotal)
	}
	if d.ApprovalStatus != ApprovalNotRequired {
		t.Errorf("10%% dentro do teto de 12%% não precisa de aprovação, veio %q", d.ApprovalStatus)
	}
}

func TestRegression_DiscountAboveCeilingGoesToApprovalQueue(t *testing.T) {
	d := ResolveDiscount(1000, 18, 12)
	if d.ApprovalStatus != ApprovalPending {
		t.Fatalf("18%% acima do teto de 12%% deveria ficar pendente, veio %q", d.ApprovalStatus)
	}
	// A venda não é bloqueada: o valor sai calculado do mesmo jeito.
	if d.Amount != 180 || d.NetSubtotal != 820 {
		t.Fatalf("desconto acima do teto continua sendo aplicado: amount=%v net=%v", d.Amount, d.NetSubtotal)
	}
}

func TestResolveDiscount_ExactlyAtCeilingDoesNotNeedApproval(t *testing.T) {
	// Borda: teto é "até", não "abaixo de". 12% com teto 12% é venda normal.
	if d := ResolveDiscount(100, 12, 12); d.ApprovalStatus != ApprovalNotRequired {
		t.Fatalf("desconto igual ao teto não deveria pedir aprovação, veio %q", d.ApprovalStatus)
	}
}

func TestResolveDiscount_ClampsOutOfRangeInput(t *testing.T) {
	if d := ResolveDiscount(100, -5, 12); d.Pct != 0 || d.Amount != 0 || !d.Capped {
		t.Errorf("pct negativo deveria virar 0 e marcar Capped: %+v", d)
	}
	if d := ResolveDiscount(100, 250, 12); d.Pct != 100 || d.Amount != 100 || !d.Capped {
		t.Errorf("pct > 100 deveria ser truncado em 100: %+v", d)
	}
	// Desconto nunca deixa o total negativo.
	if d := ResolveDiscount(100, 100, 100); d.NetSubtotal != 0 {
		t.Errorf("100%% de desconto deveria zerar, não negativar: %+v", d)
	}
}

func TestRegression_ZeroCeilingSendsEveryDiscountToApproval(t *testing.T) {
	// Fail-closed: quando o teto não pôde ser resolvido (auth-service fora do
	// ar), o handler usa teto 0 — e aí QUALQUER desconto vira pendente em vez
	// de passar direto.
	d := ResolveDiscount(1000, 1, 0)
	if d.ApprovalStatus != ApprovalPending {
		t.Fatalf("com teto 0, todo desconto deveria ficar pendente, veio %q", d.ApprovalStatus)
	}
	// Sem desconto continua sendo venda normal, mesmo com teto 0.
	if d := ResolveDiscount(1000, 0, 0); d.ApprovalStatus != ApprovalNotRequired {
		t.Fatalf("venda sem desconto não pode cair na fila de aprovação: %q", d.ApprovalStatus)
	}
}
