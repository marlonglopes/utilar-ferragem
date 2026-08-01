package handler

import "github.com/utilar/pkg/roles"

// Conjuntos de papéis das rotas de admin do catálogo, em um lugar só (lidos por
// main.go e pelo teste de autorização — não podem divergir).
var (
	// Gestão de catálogo (produto, categoria, import, imagens, trilha, review,
	// price-tiers/atributos). Inclui `vendas` — o vendedor interno mantém o
	// catálogo e VÊ custo/margem pra negociar. É AQUI que mora a rota que
	// devolve `cost`; por isso contador e almoxarife (que não veem custo) NÃO
	// entram neste conjunto — a proteção é não conceder a rota, não filtrar o
	// campo.
	CatalogAdminRoles = []string{roles.Admin, roles.Vendas}

	// Observabilidade agregada dos serviços (saúde/métricas, sem custo). O
	// contador acompanha a saúde da operação; não precisa e não deve mexer no
	// catálogo, então fica num grupo separado do CatalogAdminRoles.
	CatalogObsRoles = []string{roles.Admin, roles.Contador}
)
