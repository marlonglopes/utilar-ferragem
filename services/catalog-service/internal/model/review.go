package model

import "time"

// Review é a projeção PÚBLICA de uma avaliação.
//
// ⚠️ `AuthorUserID` NÃO ESTÁ AQUI e não pode entrar. O identificador do cliente
// é dado pessoal e não tem nenhuma utilidade para a vitrine — o que a vitrine
// mostra é `AuthorName`, já reduzido a "Primeiro N." pela aplicação. Mesma
// lógica estrutural do `cost` em Product: o campo sensível não existe na struct
// que é serializada em rota aberta, então não vaza por esquecimento de
// `omitempty` nem por SELECT copiado.
//
// `OrderID` também fica de fora: provar que a compra existe é trabalho do
// servidor; expor QUAL pedido gerou a avaliação entregaria um identificador de
// transação a qualquer visitante.
type Review struct {
	ID     string  `json:"id"`
	Rating int     `json:"rating"`
	Title  *string `json:"title,omitempty"`
	Body   *string `json:"body,omitempty"`
	// AuthorName é o nome de exibição minimizado ("Marlon G.").
	AuthorName string `json:"authorName"`
	// VerifiedPurchase é sempre true — não existe caminho de escrita que crie
	// avaliação sem pedido confirmado. O campo é explícito no payload porque é
	// a informação que dá credibilidade ao número, e o frontend precisa poder
	// exibir o selo sem inferir.
	VerifiedPurchase bool      `json:"verifiedPurchase"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`

	// Status só é preenchido nas rotas do PRÓPRIO autor e do admin — na
	// listagem pública é sempre "published" e sai omitido. É o que permite ao
	// cliente saber que a avaliação dele foi para a fila de moderação em vez de
	// simplesmente não aparecer (que ele leria como bug).
	Status         string  `json:"status,omitempty"`
	ModerationNote *string `json:"moderationNote,omitempty"`
	// ProductID/AuthorUserID só aparecem na fila de moderação do admin, onde a
	// tela precisa saber de que produto é cada linha.
	ProductID    string `json:"productId,omitempty"`
	AuthorUserID string `json:"authorUserId,omitempty"`
}

// ReviewSummary é o bloco agregado que acompanha a listagem de avaliações.
type ReviewSummary struct {
	// Average é a média simples publicada — o número que o cliente reconhece.
	Average float64 `json:"average"`
	Count   int     `json:"count"`
	// Score é a média BAYESIANA (products.rating_bayes) — o número que ORDENA a
	// vitrine. Vem no payload para a UI poder explicar por que um 4,6★ com 200
	// avaliações aparece acima de um 5,0★ com uma.
	Score float64 `json:"score"`
	// Distribution é a contagem por nota, chave "1".."5". Serve para a barra de
	// distribuição; sem ela a UI teria que buscar todas as avaliações para
	// desenhar cinco barras.
	Distribution map[string]int `json:"distribution"`
}

// ReviewListResponse é o envelope de GET /products/:slug/reviews.
type ReviewListResponse struct {
	Data    []Review       `json:"data"`
	Meta    ReviewListMeta `json:"meta"`
	Summary ReviewSummary  `json:"summary"`
}

// ReviewListMeta — paginação.
type ReviewListMeta struct {
	Page    int    `json:"page"`
	PerPage int    `json:"perPage"`
	Total   int    `json:"total"`
	Sort    string `json:"sort"`
}

// -- recomendação -------------------------------------------------------------

// Motivos de recomendação. São valores ESTÁVEIS de contrato: o frontend decide
// o texto do cabeçalho do carrossel a partir daqui.
const (
	// ReasonCopurchase — co-compra agregada, acima do mínimo de ocorrências.
	ReasonCopurchase = "copurchase"
	// ReasonComplement — regra técnica (produto complementar por aplicação).
	ReasonComplement = "complement"
	// ReasonCategoryFallback — NÃO É RECOMENDAÇÃO. É "outros produtos desta
	// categoria", devolvido quando não há dado suficiente. Vem marcado
	// justamente para o frontend não anunciar como "quem comprou também levou".
	ReasonCategoryFallback = "category_fallback"
)

// RelatedReason explica POR QUE um produto foi recomendado.
type RelatedReason struct {
	Kind string `json:"kind"`
	// Label é o texto curto pronto para exibição (pt-BR).
	Label string `json:"label"`
	// Orders é o número de pedidos distintos que sustentam a co-compra.
	// Só preenchido em Kind=copurchase — é a evidência do número, e permite à
	// UI mostrar "12 clientes levaram junto" em vez de uma afirmação sem lastro.
	Orders int `json:"orders,omitempty"`
	// Note é a razão técnica da regra, em Kind=complement.
	Note *string `json:"note,omitempty"`
}

// RelatedProduct é um Product com o motivo anexado.
//
// Product é EMBUTIDO (sem tag de json), então os campos dele continuam saindo
// no mesmo nível do JSON de antes. Consumidor existente de
// `GET /products/:slug/related` não quebra; ganha um campo `reason` a mais.
type RelatedProduct struct {
	Product
	Reason RelatedReason `json:"reason"`
}

// RelatedMeta descreve a composição da lista devolvida.
type RelatedMeta struct {
	// Strategy: "copurchase" | "complement" | "mixed" | "category_fallback".
	Strategy string `json:"strategy"`
	// Fallback é true se QUALQUER item da lista veio do preenchimento por
	// categoria. É o sinalizador honesto pedido no contrato: com ele true, a UI
	// não deve chamar a seção de "recomendado para você".
	Fallback bool `json:"fallback"`
	// MinCopurchaseOrders é o limiar aplicado. Exposto para a decisão ser
	// auditável de fora, sem ler o código.
	MinCopurchaseOrders int `json:"minCopurchaseOrders"`
	Counts              struct {
		Copurchase int `json:"copurchase"`
		Complement int `json:"complement"`
		Fallback   int `json:"fallback"`
	} `json:"counts"`
}

// RelatedResponse é o envelope de GET /products/:slug/related.
type RelatedResponse struct {
	Data []RelatedProduct `json:"data"`
	Meta RelatedMeta      `json:"meta"`
}
