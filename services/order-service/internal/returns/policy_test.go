package returns_test

import (
	"errors"
	"testing"
	"time"

	"github.com/utilar/order-service/internal/returns"
)

// ============================================================================
// Regras de devolução — testes puros, rodam sempre, sem banco.
//
// O que está sendo travado aqui é OBRIGAÇÃO LEGAL (CDC arts. 26 e 49), não
// política comercial. Um teste que quebra aqui é a loja deixando de cumprir a
// lei, não um detalhe de UX.
// ============================================================================

var entrega = time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

// pedidoEntregue monta um pedido web entregue, com 10 unidades de um item e 2
// de outro.
func pedidoEntregue() (returns.OrderRef, map[string]returns.OrderItemRef) {
	d := entrega
	p := entrega.Add(-72 * time.Hour)
	o := returns.OrderRef{
		ID: "ord-1", UserID: "cliente-1", Status: "delivered", Channel: "web",
		PaidAt: &p, DeliveredAt: &d, ShippingCost: 25.00,
	}
	items := map[string]returns.OrderItemRef{
		"item-a": {ID: "item-a", ProductID: "prod-a", Quantity: 10, UnitPrice: 30.00},
		"item-b": {ID: "item-b", ProductID: "prod-b", Quantity: 2, UnitPrice: 100.00},
	}
	return o, items
}

func pedirDevolucao(o returns.OrderRef, items map[string]returns.OrderItemRef,
	wanted []returns.RequestedItem, now time.Time) (returns.Decision, error) {
	return returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "cliente-1", Role: returns.RoleCustomer},
		Order: o, Items: items, Wanted: wanted, Now: now,
	})
}

// ============================================================================
// PRAZO DE 7 DIAS — a borda
// ============================================================================

// TestArrependimentoNoDia7AindaVale.
//
// Modo de falha que previne: usar `now.Before(deadline)` em vez de
// `!now.After(deadline)` tira do consumidor as horas finais do sétimo dia. É
// exatamente o tipo de arredondamento que a loja perde no Procon.
func TestArrependimentoNoDia7AindaVale(t *testing.T) {
	o, items := pedidoEntregue()

	// Dia 7, quase no fim: entrega + 7 dias menos 1 minuto.
	dia7 := entrega.AddDate(0, 0, 7).Add(-time.Minute)

	d, err := pedirDevolucao(o, items, []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}}, dia7)
	if err != nil {
		t.Fatalf("dia 7: err = %v — o prazo legal foi encurtado", err)
	}
	if d.Kind != returns.KindRegret {
		t.Fatalf("kind = %v, esperado arrependimento (art. 49) dentro dos 7 dias", d.Kind)
	}
	if !d.AutoApproved {
		t.Fatal("arrependimento não foi deferido automaticamente — direito incondicional " +
			"não pode ir para fila de análise")
	}
}

// TestArrependimentoNoDia8ViraVicio — passado o prazo do art. 49, o caminho
// não deixa de existir: vira vício do produto (art. 26), COM análise e COM
// exigência de motivo.
func TestArrependimentoNoDia8ViraVicio(t *testing.T) {
	o, items := pedidoEntregue()
	dia8 := entrega.AddDate(0, 0, 8)

	// Sem motivo: agora é exigido, porque não é mais arrependimento.
	_, err := pedirDevolucao(o, items, []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}}, dia8)
	if !errors.Is(err, returns.ErrReasonRequired) {
		t.Fatalf("dia 8 sem motivo: err = %v, esperado ErrReasonRequired", err)
	}

	// Com motivo: aceito como vício, e SEM aprovação automática.
	d, err := returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "cliente-1", Role: returns.RoleCustomer},
		Order: o, Items: items,
		Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		Reason: "a bateria não carrega", Now: dia8,
	})
	if err != nil {
		t.Fatalf("dia 8 com motivo: err = %v", err)
	}
	if d.Kind != returns.KindDefect {
		t.Fatalf("kind = %v, esperado vício (art. 26) fora dos 7 dias", d.Kind)
	}
	if d.AutoApproved {
		t.Fatal("vício foi aprovado automaticamente — este caminho EXIGE análise")
	}
	// E o frete não volta no vício por esta regra (só no arrependimento total).
	if d.ShippingRefund != 0 {
		t.Fatalf("shippingRefund = %v, esperado 0 fora do arrependimento", d.ShippingRefund)
	}
}

// TestArrependimentoNoInstanteExatoDoLimite — a fronteira exata.
func TestArrependimentoNoInstanteExatoDoLimite(t *testing.T) {
	o, items := pedidoEntregue()
	limite := entrega.AddDate(0, 0, 7)

	if d, err := pedirDevolucao(o, items, []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}}, limite); err != nil {
		t.Fatalf("no instante do limite: err = %v", err)
	} else if d.Kind != returns.KindRegret {
		t.Fatalf("kind = %v no instante exato do limite, esperado arrependimento", d.Kind)
	}

	// Um segundo depois já é vício.
	d, err := returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "cliente-1", Role: returns.RoleCustomer},
		Order: o, Items: items,
		Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		Reason: "veio quebrado", Now: limite.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if d.Kind != returns.KindDefect {
		t.Fatalf("kind = %v 1s após o limite, esperado vício", d.Kind)
	}
}

// TestForaDosDoisPrazosNaoTemCaminho — depois de 90 dias, nem vício.
func TestForaDosDoisPrazosNaoTemCaminho(t *testing.T) {
	o, items := pedidoEntregue()
	_, err := returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "cliente-1", Role: returns.RoleCustomer},
		Order: o, Items: items,
		Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		Reason: "parou de funcionar", Now: entrega.AddDate(0, 0, 91),
	})
	if !errors.Is(err, returns.ErrDefectWindowExpired) {
		t.Fatalf("err = %v, esperado ErrDefectWindowExpired", err)
	}
}

// ============================================================================
// PEDIDO SEM DATA DE ENTREGA — o caso chato e real
// ============================================================================

// TestRegression_PedidoSemDataDeEntregaNaoPerdeOPrazo.
//
// Modo de falha que previne: contar o prazo do `created_at` quando falta
// `delivered_at` encurta o prazo do consumidor usando uma data que a lei não
// manda usar. O ônus de provar QUANDO entregou é do fornecedor — quem tem o
// comprovante da transportadora é ele. Sem a data, a loja não pode alegar
// vencimento.
func TestRegression_PedidoSemDataDeEntregaNaoPerdeOPrazo(t *testing.T) {
	o, items := pedidoEntregue()
	o.DeliveredAt = nil // marcado como entregue, sem data (migração / correção manual)

	// Seis meses depois. Se o prazo fosse contado de qualquer data disponível,
	// aqui já teria vencido tudo.
	d, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		entrega.AddDate(0, 6, 0))
	if err != nil {
		t.Fatalf("sem data de entrega: err = %v — a loja alegou prazo que não pode provar", err)
	}
	if d.Kind != returns.KindRegret {
		t.Fatalf("kind = %v, esperado arrependimento: sem data base o prazo não pode ser dado por vencido", d.Kind)
	}
	if d.Deadline.Source != returns.BasisUnknown {
		t.Fatalf("basisSource = %v, esperado 'unknown'", d.Deadline.Source)
	}
	// E o caso fica sinalizado para o relatório operacional: o buraco de dado
	// tem que ser consertado na ORIGEM, não na régua do prazo.
	if !d.Deadline.NeedsOperationalReview() {
		t.Fatal("o pedido sem data de entrega não foi sinalizado para revisão operacional")
	}
}

// TestPedidoAindaNaoEntregueTemJanelaAberta — o art. 49 conta do recebimento;
// antes dele o prazo nem começou. Desistir de compra a caminho é o cenário mais
// comum de arrependimento que existe.
func TestPedidoAindaNaoEntregueTemJanelaAberta(t *testing.T) {
	o, items := pedidoEntregue()
	o.Status = "shipped"
	o.DeliveredAt = nil

	d, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 10}, {OrderItemID: "item-b", Quantity: 2}},
		entrega.AddDate(0, 0, 30))
	if err != nil {
		t.Fatalf("pedido a caminho: err = %v", err)
	}
	if d.Deadline.Source != returns.BasisNotDelivered {
		t.Fatalf("basisSource = %v, esperado 'not_delivered'", d.Deadline.Source)
	}
	if !d.AutoApproved {
		t.Fatal("desistência de pedido a caminho não foi deferida automaticamente")
	}
}

// TestVendaDeBalcaoContaDoPagamento — na retirada no ato, não há dúvida sobre
// quando o cliente recebeu: ele saiu da loja com o produto.
func TestVendaDeBalcaoContaDoPagamento(t *testing.T) {
	o, items := pedidoEntregue()
	o.Channel = "balcao"
	o.DeliveredAt = nil
	pago := entrega
	o.PaidAt = &pago

	d, err := pedirDevolucao(o, items, []returns.RequestedItem{{OrderItemID: "item-b", Quantity: 1}},
		entrega.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("balcão: err = %v", err)
	}
	if d.Deadline.Source != returns.BasisBalcao {
		t.Fatalf("basisSource = %v, esperado 'balcao_pickup'", d.Deadline.Source)
	}
}

// ============================================================================
// DEVOLUÇÃO PARCIAL
// ============================================================================

// TestDevolucaoParcialDeItemEspecifico — o cliente compra 10 e devolve 1.
// Sem isto, a única opção seria devolver o pedido inteiro.
func TestDevolucaoParcialDeItemEspecifico(t *testing.T) {
	o, items := pedidoEntregue()

	d, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		entrega.AddDate(0, 0, 2))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(d.Items) != 1 || d.Items[0].Quantity != 1 {
		t.Fatalf("itens = %+v, esperado 1 unidade do item-a", d.Items)
	}
	if d.ItemsAmount != 30.00 {
		t.Fatalf("itemsAmount = %v, esperado 30.00 (1 × 30,00)", d.ItemsAmount)
	}
	if d.IsFullReturn {
		t.Fatal("devolução de 1 de 12 unidades foi classificada como TOTAL")
	}
	// ⚠️ Frete NÃO volta na parcial: a entrega dos demais itens de fato
	// aconteceu. Devolver o frete inteiro daria à loja um prejuízo que a lei
	// não impõe.
	if d.ShippingRefund != 0 {
		t.Fatalf("shippingRefund = %v, esperado 0 na devolução parcial", d.ShippingRefund)
	}
	if d.TotalRefund != 30.00 {
		t.Fatalf("totalRefund = %v, esperado 30.00", d.TotalRefund)
	}
}

// TestDevolucaoTotalDevolveOFrete — no arrependimento total, o art. 49
// (parágrafo único) manda devolver os valores pagos, e a jurisprudência
// consolidada inclui o frete.
func TestDevolucaoTotalDevolveOFrete(t *testing.T) {
	o, items := pedidoEntregue()

	d, err := pedirDevolucao(o, items, []returns.RequestedItem{
		{OrderItemID: "item-a", Quantity: 10},
		{OrderItemID: "item-b", Quantity: 2},
	}, entrega.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !d.IsFullReturn {
		t.Fatal("devolução de todas as unidades não foi classificada como total")
	}
	if d.ItemsAmount != 500.00 { // 10×30 + 2×100
		t.Fatalf("itemsAmount = %v, esperado 500.00", d.ItemsAmount)
	}
	if d.ShippingRefund != 25.00 {
		t.Fatalf("shippingRefund = %v, esperado 25.00", d.ShippingRefund)
	}
	if d.TotalRefund != 525.00 {
		t.Fatalf("totalRefund = %v, esperado 525.00", d.TotalRefund)
	}
}

// TestRegression_NaoDevolveMaisDoQueComprou.
//
// Modo de falha que previne: dez pedidos de devolução de 1 unidade cada, de um
// item comprado 1 vez, estornariam 10 vezes o valor. Dinheiro saindo sem
// contrapartida.
func TestRegression_NaoDevolveMaisDoQueComprou(t *testing.T) {
	o, items := pedidoEntregue()

	_, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-b", Quantity: 3}}, // comprou 2
		entrega.AddDate(0, 0, 1))
	if !errors.Is(err, returns.ErrQuantityExceeded) {
		t.Fatalf("err = %v, esperado ErrQuantityExceeded", err)
	}
}

// TestRegression_ItemRepetidoNoPedidoESomadoAntesDeValidar.
//
// Modo de falha que previne: mandar o mesmo item duas vezes com quantidade 1
// cada, num item comprado 1 vez. Validar linha a linha deixaria as duas
// passarem. Mesmo bug que checkStock já resolve na criação do pedido.
func TestRegression_ItemRepetidoNoPedidoESomadoAntesDeValidar(t *testing.T) {
	o, items := pedidoEntregue()

	_, err := pedirDevolucao(o, items, []returns.RequestedItem{
		{OrderItemID: "item-b", Quantity: 1},
		{OrderItemID: "item-b", Quantity: 1},
		{OrderItemID: "item-b", Quantity: 1}, // total 3, comprou 2
	}, entrega.AddDate(0, 0, 1))
	if !errors.Is(err, returns.ErrQuantityExceeded) {
		t.Fatalf("err = %v, esperado ErrQuantityExceeded para item repetido somado", err)
	}
}

// TestDescontaOQueJaFoiDevolvidoAntes — comprou 10, devolveu 3 na semana
// passada, só pode devolver 7 agora.
func TestDescontaOQueJaFoiDevolvidoAntes(t *testing.T) {
	o, items := pedidoEntregue()
	it := items["item-a"]
	it.AlreadyReturned = 3
	items["item-a"] = it

	if _, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 8}},
		entrega.AddDate(0, 0, 1)); !errors.Is(err, returns.ErrQuantityExceeded) {
		t.Fatalf("err = %v, esperado ErrQuantityExceeded (só restam 7)", err)
	}

	if _, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 7}},
		entrega.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("devolver as 7 restantes: err = %v", err)
	}
}

// TestSaldoRestanteContaComoDevolucaoTotal — devolveu 3 de 10 antes; devolver
// os 7 restantes junto com o outro item esgota o pedido. Importa para a trava
// de split e para o frete.
func TestSaldoRestanteContaComoDevolucaoTotal(t *testing.T) {
	o, items := pedidoEntregue()
	a := items["item-a"]
	a.AlreadyReturned = 3
	items["item-a"] = a

	d, err := pedirDevolucao(o, items, []returns.RequestedItem{
		{OrderItemID: "item-a", Quantity: 7},
		{OrderItemID: "item-b", Quantity: 2},
	}, entrega.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !d.IsFullReturn {
		t.Fatal("esgotar o saldo devolvível não foi classificado como devolução total")
	}
}

func TestItemDeOutroPedidoERecusado(t *testing.T) {
	o, items := pedidoEntregue()
	_, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-de-outro-pedido", Quantity: 1}},
		entrega.AddDate(0, 0, 1))
	if !errors.Is(err, returns.ErrItemNotInOrder) {
		t.Fatalf("err = %v, esperado ErrItemNotInOrder", err)
	}
}

func TestDevolucaoSemItemERecusada(t *testing.T) {
	o, items := pedidoEntregue()
	if _, err := pedirDevolucao(o, items, nil, entrega.AddDate(0, 0, 1)); !errors.Is(err, returns.ErrNoItems) {
		t.Fatalf("err = %v, esperado ErrNoItems", err)
	}
}

// ============================================================================
// AUTORIZAÇÃO — o cliente só devolve o PRÓPRIO pedido
// ============================================================================

// TestRegression_ClienteNaoDevolvePedidoDeOutro.
//
// Modo de falha que previne: IDOR na rota de devolução. Sem esta checagem,
// qualquer cliente autenticado abriria devolução (e portanto ESTORNO) sobre o
// pedido de qualquer outro — dinheiro saindo para a conta errada.
func TestRegression_ClienteNaoDevolvePedidoDeOutro(t *testing.T) {
	o, items := pedidoEntregue() // dono: cliente-1

	_, err := returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "cliente-2-invasor", Role: returns.RoleCustomer},
		Order: o, Items: items,
		Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		Now:    entrega.AddDate(0, 0, 1),
	})
	if !errors.Is(err, returns.ErrNotOwner) {
		t.Fatalf("err = %v, esperado ErrNotOwner — IDOR na rota de devolução", err)
	}
}

// TestAtendenteAbreDevolucaoEmNomeDoCliente — fluxo real do SAC e do balcão.
func TestAtendenteAbreDevolucaoEmNomeDoCliente(t *testing.T) {
	o, items := pedidoEntregue()

	for _, papel := range []string{returns.RoleAdmin, returns.RoleOperator, returns.RoleStoreOperator} {
		if _, err := returns.Evaluate(returns.Request{
			Actor: returns.Actor{UserID: "atendente-9", Role: papel},
			Order: o, Items: items,
			Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
			Now:    entrega.AddDate(0, 0, 1),
		}); err != nil {
			t.Fatalf("papel %s: err = %v", papel, err)
		}
	}
}

// TestRegression_SellerNaoEAtendente.
//
// Modo de falha que previne: `seller` é LOJISTA do marketplace, não atendente
// da loja. Confundir os dois já custou caro no PDV (ver CLAUDE.md); aqui daria
// a todo anunciante o poder de abrir devolução sobre pedido alheio.
func TestRegression_SellerNaoEAtendente(t *testing.T) {
	o, items := pedidoEntregue()

	_, err := returns.Evaluate(returns.Request{
		Actor: returns.Actor{UserID: "lojista-7", Role: "seller"},
		Order: o, Items: items,
		Wanted: []returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		Now:    entrega.AddDate(0, 0, 1),
	})
	if !errors.Is(err, returns.ErrNotOwner) {
		t.Fatalf("err = %v, esperado ErrNotOwner — seller foi tratado como atendente", err)
	}
}

// TestClienteNaoDecideAPropriaDevolucao — aprovar a própria devolução é o
// equivalente exato de aprovar o próprio desconto no balcão.
func TestClienteNaoDecideAPropriaDevolucao(t *testing.T) {
	if returns.CanDecide(returns.RoleCustomer) {
		t.Fatal("cliente pode decidir sobre devolução — aprovaria o próprio estorno")
	}
	if returns.CanDecide("seller") {
		t.Fatal("seller pode decidir sobre devolução")
	}
	for _, papel := range []string{returns.RoleAdmin, returns.RoleOperator, returns.RoleStoreOperator} {
		if !returns.CanDecide(papel) {
			t.Fatalf("papel %s não pode decidir, mas deveria", papel)
		}
	}
}

// ============================================================================
// ESTADO DO PEDIDO
// ============================================================================

func TestPedidoNaoPagoOuCanceladoNaoTemDevolucao(t *testing.T) {
	o, items := pedidoEntregue()
	for _, st := range []string{"pending_payment", "cancelled"} {
		o.Status = st
		_, err := pedirDevolucao(o, items,
			[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
			entrega.AddDate(0, 0, 1))
		if !errors.Is(err, returns.ErrOrderNotReturnable) {
			t.Fatalf("status %s: err = %v, esperado ErrOrderNotReturnable", st, err)
		}
	}
}

// ============================================================================
// SPLIT DE PAGAMENTO — a trava do PSP
// ============================================================================

// TestRegression_SplitRecusaDevolucaoParcialAntesDeChegarNoPSP.
//
// Modo de falha que previne: a Appmax só aceita estorno TOTAL em pedido com
// Payment Split (docs/appmax-v1-appstore.md § 5). Sem esta trava, a devolução
// parcial seria aceita aqui, o cliente seria avisado de que o dinheiro está
// voltando, o produto seria devolvido, o estoque reposto — e a chamada só
// falharia lá no PSP. Produto devolvido e dinheiro preso.
func TestRegression_SplitRecusaDevolucaoParcialAntesDeChegarNoPSP(t *testing.T) {
	o, items := pedidoEntregue()
	o.PaymentSplit = true

	_, err := pedirDevolucao(o, items,
		[]returns.RequestedItem{{OrderItemID: "item-a", Quantity: 1}},
		entrega.AddDate(0, 0, 1))
	if !errors.Is(err, returns.ErrSplitPartialRefund) {
		t.Fatalf("err = %v, esperado ErrSplitPartialRefund", err)
	}
}

// TestSplitAceitaDevolucaoTotal — a trava é sobre a PARCIAL. Total pode.
func TestSplitAceitaDevolucaoTotal(t *testing.T) {
	o, items := pedidoEntregue()
	o.PaymentSplit = true

	if _, err := pedirDevolucao(o, items, []returns.RequestedItem{
		{OrderItemID: "item-a", Quantity: 10},
		{OrderItemID: "item-b", Quantity: 2},
	}, entrega.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("devolução total com split: err = %v", err)
	}
}
