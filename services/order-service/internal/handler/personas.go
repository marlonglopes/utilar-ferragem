package handler

import "github.com/utilar/pkg/roles"

// Conjuntos de papéis das rotas de operação do painel, em UM lugar só, para
// que main.go (quem aplica) e o teste de autorização (quem verifica) leiam a
// mesma verdade — senão o teste checaria uma matriz e o servidor aplicaria
// outra. A política em si (quem entra onde) está explicada nos comentários do
// grupo em cmd/server/main.go.
var (
	// Ler pedido/devolução: inclui o contador (faturamento) e todo mundo que
	// opera. Leitura não expõe custo — o custo mora no catalog, não aqui.
	OpsReadRoles = []string{
		roles.Admin, roles.Operator, roles.Contador, roles.Vendas, roles.Almoxarife,
	}
	// Agir (separar/despachar/receber devolução física): SEM o contador, que é
	// read-only fora do contábil.
	OpsWriteRoles = []string{
		roles.Admin, roles.Operator, roles.Vendas, roles.Almoxarife,
	}
	// Reembolsar (dinheiro SAINDO): só o dono. Nem vendas, nem almoxarife, nem
	// contador cruzam esta linha.
	OpsRefundRoles = []string{roles.Admin}
)
