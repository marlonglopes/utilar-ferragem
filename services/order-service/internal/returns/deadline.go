package returns

import "time"

// ============================================================================
// Contagem do prazo — a parte que mais dá briga no Procon
// ============================================================================

// BasisSource identifica DE ONDE saiu a data base do prazo. Vai congelada no
// registro da devolução: é a prova de como a loja chegou ao prazo que aplicou.
type BasisSource string

const (
	// BasisDelivered — o pedido tem data de entrega. Caso correto e desejado.
	BasisDelivered BasisSource = "delivered_at"

	// BasisNotDelivered — o pedido ainda não foi entregue.
	//
	// O art. 49 conta "da assinatura OU do ato de recebimento". Antes do
	// recebimento o prazo sequer começou a correr, então a janela está ABERTA.
	// O cliente pode desistir de uma compra que ainda está a caminho — e isso é
	// o cenário mais comum de arrependimento que existe.
	BasisNotDelivered BasisSource = "not_delivered"

	// ⚠️ BasisUnknown — O CASO CHATO E REAL: o pedido está marcado como
	// entregue mas NÃO tem `delivered_at`. Acontece com pedido migrado, com
	// pedido cujo status foi corrigido à mão, e com venda de balcão (retirada
	// no ato, sem etapa de entrega registrada).
	//
	// POLÍTICA: sem data de recebimento, a loja NÃO PODE alegar prazo vencido.
	// O ônus de provar QUANDO o produto foi entregue é do fornecedor — é ele
	// quem tem o comprovante da transportadora. Na dúvida, a janela fica
	// ABERTA e o pedido de devolução segue.
	//
	// A alternativa (contar do `created_at`, que é a data mais antiga
	// disponível) parece conservadora mas é o contrário: encurta o prazo do
	// consumidor usando uma data que a lei não manda usar, e a loja perde a
	// discussão no primeiro processo. Errar a favor do consumidor custa uma
	// devolução; errar contra custa uma multa e o processo.
	//
	// O registro fica marcado com este basis_source justamente para que o
	// relatório de operação encontre os pedidos sem data de entrega e o
	// problema seja consertado na origem, e não na régua do prazo.
	BasisUnknown BasisSource = "unknown"

	// BasisBalcao — venda de balcão. A entrega é no ato, então a data base é a
	// do pagamento. Aqui não há dúvida sobre quando o cliente recebeu: ele saiu
	// da loja com o produto.
	BasisBalcao BasisSource = "balcao_pickup"
)

// Deadline é o prazo resolvido, com a data base e a origem dela — tudo
// congelado no ato do pedido de devolução.
//
// PORQUÊ congelar: um `delivered_at` corrigido meses depois mudaria
// retroativamente se a devolução foi tempestiva, e a prova de que a loja agiu
// certo evaporaria junto.
type Deadline struct {
	// Basis é a data da qual o prazo foi contado. Zero quando desconhecida.
	Basis time.Time
	// Source diz de onde Basis saiu.
	Source BasisSource
	// RegretAt é o fim dos 7 dias do art. 49. Zero quando não há data base.
	RegretAt time.Time
	// DefectAt é o fim dos 90 dias do art. 26.
	DefectAt time.Time

	// RegretOpen — dentro dos 7 dias (ou sem data base para alegar o
	// contrário). É este campo que decide se o pedido é arrependimento
	// (incondicional) ou vício (com análise).
	RegretOpen bool
	// DefectOpen — dentro dos 90 dias.
	DefectOpen bool
}

// ResolveDeadline calcula o prazo aplicável ao pedido.
//
// A data base é escolhida nesta ordem, e a ordem É a regra:
//
//  1. venda de balcão            → paid_at (entrega no ato)
//  2. delivered_at preenchido    → delivered_at (o que o art. 49 manda)
//  3. pedido ainda não entregue  → janela ABERTA (o prazo nem começou)
//  4. entregue mas sem data      → janela ABERTA (a loja não pode alegar
//     vencimento de um prazo cuja contagem ela mesma não registrou)
func ResolveDeadline(o OrderRef, now time.Time) Deadline {
	now = now.UTC()

	// Caso 1 — balcão: o cliente saiu da loja com o produto.
	if o.Channel == "balcao" {
		if o.PaidAt != nil {
			return windowFrom(o.PaidAt.UTC(), BasisBalcao, now)
		}
		// Balcão sem paid_at é dado inconsistente; cai na regra do desconhecido.
		return openWindow(BasisUnknown, now)
	}

	// Caso 2 — o caminho correto.
	if o.DeliveredAt != nil {
		return windowFrom(o.DeliveredAt.UTC(), BasisDelivered, now)
	}

	// Caso 3 — ainda a caminho. O prazo do art. 49 nem começou a correr.
	if o.Status != "delivered" {
		return openWindow(BasisNotDelivered, now)
	}

	// Caso 4 — marcado como entregue, sem data. Ver o comentário de
	// BasisUnknown: a janela fica aberta e o registro fica sinalizado.
	return openWindow(BasisUnknown, now)
}

// windowFrom monta a janela a partir de uma data base conhecida.
func windowFrom(basis time.Time, src BasisSource, now time.Time) Deadline {
	regretAt := basis.AddDate(0, 0, RegretDays)
	defectAt := basis.AddDate(0, 0, DefectDaysDurable)
	return Deadline{
		Basis:    basis,
		Source:   src,
		RegretAt: regretAt,
		DefectAt: defectAt,
		// !After e não Before: o dia 7 INTEIRO ainda é dentro do prazo. Prazo
		// em dias que expira no instante exato da entrega + 168h tiraria do
		// consumidor as horas finais do sétimo dia — e é exatamente o tipo de
		// arredondamento que se perde no Procon.
		RegretOpen: !now.After(regretAt),
		DefectOpen: !now.After(defectAt),
	}
}

// openWindow é a janela sem data base: tudo aberto.
func openWindow(src BasisSource, now time.Time) Deadline {
	return Deadline{
		Source:     src,
		RegretOpen: true,
		DefectOpen: true,
	}
}

// NeedsOperationalReview diz se este pedido de devolução deve aparecer no
// relatório de pendências operacionais.
//
// `unknown` significa que existe um pedido marcado como entregue sem data de
// entrega. A devolução segue (a favor do consumidor), mas o buraco de dado
// precisa ser consertado NA ORIGEM — senão a loja fica permanentemente sem
// poder aplicar prazo nenhum.
func (d Deadline) NeedsOperationalReview() bool {
	return d.Source == BasisUnknown
}
