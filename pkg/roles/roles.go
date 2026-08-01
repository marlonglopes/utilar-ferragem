// Package roles nomeia os papéis de usuário compartilhados entre os serviços.
//
// POR QUÊ existir: cada serviço decide SOZINHO quais papéis entram em cada
// rota (a autorização é local — ver os grupos /admin de cada main.go). O que
// NÃO pode divergir é a GRAFIA do papel: se o catalog escrevesse "almoxarife"
// e o order "almoxarrife", um token válido num serviço tomaria 403 no outro —
// um bug fantasma difícil de achar. Aqui mora só o vocabulário; a política de
// quem-pode-o-quê continua em cada serviço.
//
// ⚠️ `Vendas` é o vendedor INTERNO da loja (PDV + pedidos + catálogo, vê custo
// pra negociar). NÃO é `Seller` (lojista anunciante do marketplace) nem
// `StoreOperator` (o papel do balcão). Confundir dá PDV a todo anunciante.
//
// `service` NÃO está aqui de propósito: identidade de máquina não é papel de
// usuário (auditoria A1), e mora em pkg/servicetoken.
package roles

const (
	Customer      = "customer"
	Seller        = "seller"
	Admin         = "admin"
	StoreOperator = "store_operator"

	// Personas de operação do backoffice (2026-07).
	Contador   = "contador"   // contábil/faturamento; leitura no resto; NÃO vê custo
	Vendas     = "vendas"     // vendedor interno; catálogo/pedidos/PDV; vê custo
	Almoxarife = "almoxarife" // estoque/separação/despacho/devolução; NÃO vê custo

	// Operator é o papel legado que o order-service já aceitava nas rotas de
	// operação (quem embala). Mantido para não quebrar integrações existentes.
	Operator = "operator"
)

// IsKnown responde se `r` é um papel de usuário reconhecido. `service` é
// recusado de propósito (identidade de máquina, não de usuário — A1).
func IsKnown(r string) bool {
	switch r {
	case Customer, Seller, Admin, StoreOperator, Contador, Vendas, Almoxarife, Operator:
		return true
	}
	return false
}
