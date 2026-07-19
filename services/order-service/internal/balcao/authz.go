// Package balcao concentra as REGRAS de autorização e de desconto da venda de
// balcão, como funções puras.
//
// PORQUÊ funções puras num pacote separado, e não `if`s dentro do handler:
// estas três regras são o coração da segurança do PDV —
//
//  1. um operador só cria pedido da PRÓPRIA loja;
//  2. um operador só enxerga pedidos da PRÓPRIA loja;
//  3. ninguém aprova o PRÓPRIO desconto.
//
// Regra enterrada em handler só é testável com banco de pé, e teste que precisa
// de banco é teste que alguém pula. Aqui elas são testáveis sem nada, e os
// testes de regressão em authz_test.go rodam sempre.
package balcao

import (
	"errors"
	"fmt"
)

// Papéis conhecidos pelo order-service (espelham o enum do auth-service).
const (
	RoleCustomer      = "customer"
	RoleSeller        = "seller"
	RoleAdmin         = "admin"
	RoleStoreOperator = "store_operator"
	RoleService       = "service"
)

// Canais de venda.
const (
	ChannelWeb    = "web"
	ChannelBalcao = "balcao"
)

// Status de aprovação.
const (
	ApprovalNotRequired = "not_required"
	ApprovalPending     = "pending"
	ApprovalApproved    = "approved"
	ApprovalRejected    = "rejected"
)

// Erros de autorização. São sentinelas para o handler traduzir em HTTP sem
// reinterpretar string de mensagem.
var (
	// ErrNotOperator — o papel não tem acesso ao PDV. Vale especialmente para
	// `seller`: lojista do marketplace NÃO é vendedor de balcão.
	ErrNotOperator = errors.New("balcao: user is not a store operator")
	// ErrNoStoreBinding — operador sem loja no token (vínculo desativado ou
	// token emitido antes do vínculo). Fail-closed: sem loja, sem venda.
	ErrNoStoreBinding = errors.New("balcao: operator has no store binding")
	// ErrForeignStore — tentativa de agir sobre outra loja.
	ErrForeignStore = errors.New("balcao: operator cannot act on another store")
	// ErrSelfApproval — quem vendeu não homologa o próprio desconto.
	ErrSelfApproval = errors.New("balcao: operator cannot approve their own discount")
	// ErrNotApprover — cargo sem poder de aprovação (operador/supervisor).
	ErrNotApprover = errors.New("balcao: level cannot approve discounts")
	// ErrNotOwner — cliente tentando ler pedido de outro cliente (IDOR).
	ErrNotOwner = errors.New("balcao: order does not belong to user")
	// ErrNothingToApprove — pedido que não está pendente.
	ErrNothingToApprove = errors.New("balcao: order is not pending approval")
)

// Actor é quem está fazendo a requisição.
//
// DiscountCeilingPct e CanApprove NÃO vêm do JWT: vêm da consulta ao
// auth-service no momento da decisão. Ver o comentário de Claims em
// auth-service/internal/auth/jwt.go sobre por que dinheiro não viaja em token
// de 15 minutos.
type Actor struct {
	UserID string
	Role   string
	// StoreID do vínculo — do token (escopo) e confirmado pelo auth-service.
	StoreID string
	Level   string
	// DiscountCeilingPct é o teto autoritativo, já resolvido (override do
	// indivíduo ou o do cargo).
	DiscountCeilingPct float64
	CanApprove         bool
}

// OrderRef é o subset do pedido que importa para autorizar. Deliberadamente
// pequeno: nenhuma decisão de acesso deve depender de campo que não esteja aqui.
type OrderRef struct {
	OwnerUserID    string
	Channel        string
	StoreID        string
	OperatorID     string
	ApprovalStatus string
}

// -- Regra 1: criar pedido só da própria loja --------------------------------

// CanCreateBalcaoOrder autoriza a criação de uma venda de balcão na loja
// `targetStoreID`.
//
// `targetStoreID` vazio significa "a loja do operador" — o caso normal, em que
// o cliente do PDV nem manda o campo. Quando o cliente MANDA uma loja, ela tem
// que bater com a do vínculo: aceitar a loja do request seria deixar o operador
// escolher em nome de quem vende (e a qual gerente a aprovação vai cair).
//
// Admin passa em qualquer loja, mas precisa nomear a loja explicitamente — não
// existe "loja implícita" para quem não tem vínculo.
func CanCreateBalcaoOrder(a Actor, targetStoreID string) (storeID string, err error) {
	if a.Role == RoleAdmin {
		if targetStoreID == "" {
			return "", ErrNoStoreBinding
		}
		return targetStoreID, nil
	}
	if a.Role != RoleStoreOperator {
		return "", ErrNotOperator
	}
	if a.StoreID == "" {
		return "", ErrNoStoreBinding
	}
	if targetStoreID != "" && targetStoreID != a.StoreID {
		return "", fmt.Errorf("%w: operator store %s, requested %s", ErrForeignStore, a.StoreID, targetStoreID)
	}
	return a.StoreID, nil
}

// -- Regra 2: ler pedido só da própria loja ----------------------------------

// CanViewOrder decide se `a` pode ler `o`.
//
// A ordem dos casos importa e é a parte sutil desta mudança. Antes do balcão,
// TODA leitura era `WHERE user_id = $2` — o escopo do cliente era a query. Ao
// introduzir pedido criado por terceiro, essa proteção muda de forma, e o
// risco é afrouxar o caso do cliente junto. Por isso:
//
//   - cliente continua vendo APENAS os pedidos onde ele é o dono. Nada aqui
//     amplia o que um `customer` enxerga — nem os pedidos de balcão em que ele
//     é o comprador identificado, se não for o user_id do pedido;
//   - operador vê pedidos de BALCÃO da própria loja (e não os pedidos web dos
//     clientes, que continuam privados);
//   - admin vê tudo.
func CanViewOrder(a Actor, o OrderRef) error {
	switch a.Role {
	case RoleAdmin, RoleService:
		return nil

	case RoleStoreOperator:
		// O operador é também um cliente: os pedidos web dele continuam dele.
		if o.OwnerUserID != "" && o.OwnerUserID == a.UserID {
			return nil
		}
		if o.Channel != ChannelBalcao {
			return ErrNotOwner
		}
		if a.StoreID == "" {
			return ErrNoStoreBinding
		}
		if o.StoreID != a.StoreID {
			return ErrForeignStore
		}
		return nil

	default:
		// customer, seller e qualquer papel futuro: escopo de dono, sem exceção.
		if o.OwnerUserID == "" || o.OwnerUserID != a.UserID {
			return ErrNotOwner
		}
		return nil
	}
}

// -- Regra 3: não aprovar o próprio desconto ---------------------------------

// CanApproveOrder decide se `a` pode aprovar/recusar o desconto de `o`.
//
// A checagem de auto-aprovação vem ANTES da de cargo de propósito: um gerente
// que vende no balcão (acontece o tempo todo em loja pequena) tem
// CanApprove=true e passaria pela checagem de cargo sem problema. É exatamente
// o caso perigoso — dar 40% e homologar a si mesmo em dois cliques.
func CanApproveOrder(a Actor, o OrderRef) error {
	if o.Channel != ChannelBalcao {
		return ErrNothingToApprove
	}
	if o.ApprovalStatus != ApprovalPending {
		return ErrNothingToApprove
	}

	// Nem o admin aprova o próprio desconto: se o admin vendeu, outra pessoa
	// homologa. Separação de funções não tem exceção por cargo.
	if o.OperatorID != "" && o.OperatorID == a.UserID {
		return ErrSelfApproval
	}

	if a.Role == RoleAdmin {
		return nil
	}
	if a.Role != RoleStoreOperator {
		return ErrNotOperator
	}
	if a.StoreID == "" {
		return ErrNoStoreBinding
	}
	if o.StoreID != a.StoreID {
		return ErrForeignStore
	}
	if !a.CanApprove {
		return ErrNotApprover
	}
	return nil
}

// -- Desconto: recalculado no servidor ---------------------------------------

// Discount é o resultado autoritativo do desconto de um pedido.
type Discount struct {
	Pct    float64
	Amount float64
	// Total já líquido (subtotal - Amount), sem frete.
	NetSubtotal float64
	// Status de aprovação com que o pedido nasce.
	ApprovalStatus string
	// Capped indica que o pct pedido foi maior que 100 e foi truncado — sinal
	// de bug de frontend ou tamper, vai para o log e para a auditoria.
	Capped bool
}

// ResolveDiscount recalcula o desconto no servidor.
//
// O cliente do PDV manda apenas a PORCENTAGEM pretendida; o valor em reais é
// sempre derivado aqui, do subtotal que o servidor já calculou com os preços
// autoritativos do catalog. É a mesma política que já vale para preço de item e
// frete: o cliente escolhe a intenção, o servidor produz o número.
//
// Acima do teto do cargo NÃO recusa — marca `pending`. O cliente está no caixa
// com a mercadoria na mão; travar a venda empurraria o vendedor para o desconto
// "por fora" (dar um item de brinde), que não deixa rastro nenhum. Deixar
// passar e registrar é mais auditável que proibir e ser contornado.
func ResolveDiscount(subtotal, requestedPct, ceilingPct float64) Discount {
	pct := requestedPct
	capped := false
	if pct < 0 {
		pct, capped = 0, true
	}
	if pct > 100 {
		pct, capped = 100, true
	}
	if ceilingPct < 0 {
		ceilingPct = 0
	}

	amount := Round2(subtotal * pct / 100)
	if amount > subtotal {
		amount = subtotal
	}

	status := ApprovalNotRequired
	if pct > 0 && pct > ceilingPct {
		status = ApprovalPending
	}

	return Discount{
		Pct:            Round2(pct),
		Amount:         amount,
		NetSubtotal:    Round2(subtotal - amount),
		ApprovalStatus: status,
		Capped:         capped,
	}
}

// Round2 arredonda valores monetários para 2 casas.
func Round2(v float64) float64 {
	if v < 0 {
		return -Round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}
