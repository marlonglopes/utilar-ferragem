// Package cashback tem as REGRAS PURAS do programa de cashback — sem banco, sem
// HTTP — pra serem testadas isoladamente e reaproveitadas no acúmulo (consumer de
// pagamento) e no resgate (criação de pedido).
//
// Cashback é DINHEIRO (dívida da loja com o cliente). As invariantes:
//   - O cliente NUNCA dita o valor: acúmulo e resgate são calculados no servidor
//     sobre valores autoritativos (o pago, o saldo, o subtotal do pedido).
//   - Acúmulo = % sobre o valor de mercadoria efetivamente pago (fora frete).
//   - Resgate = limitado ao MENOR entre (saldo, teto% do pedido, pedido).
//
// Dinheiro em float64 segue a convenção do resto do order-service (NUMERIC(12,2)
// no banco); round2 em toda saída pra não vazar meio centavo.
package cashback

import (
	"math"
	"time"
)

// Config são os parâmetros do programa, resolvidos do banco (singleton). Ficam
// aqui como struct pura pra regra e teste não dependerem de SQL.
type Config struct {
	// EarnRatePct — quanto do valor pago vira cashback (ex.: 5 = 5%).
	EarnRatePct float64
	// RedeemMaxPct — teto de resgate por pedido, em % do subtotal (ex.: 50).
	RedeemMaxPct float64
	// ValidityDays — validade do cashback acumulado, em dias.
	ValidityDays int
	// Active — programa ligado? Desligado: não acumula nem deixa resgatar.
	Active bool

	// MinEarnSubtotal — só acumula em compras de mercadoria acima deste valor.
	MinEarnSubtotal float64
	// MinRedeemSubtotal — só deixa resgatar em pedidos acima deste valor.
	MinRedeemSubtotal float64

	// Campanha: taxa turbinada entre datas. Quando now está na janela e a taxa é
	// > 0, ela SUBSTITUI EarnRatePct no acúmulo. Bounds nil = janela aberta
	// daquele lado (só início / só fim / sempre, se ambos nil).
	CampaignRatePct  float64
	CampaignStartsAt *time.Time
	CampaignEndsAt   *time.Time
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// campaignActive diz se a campanha vale em `now`: precisa de taxa > 0 e now dentro
// da janela (bounds nil são "aberto" naquele lado).
func campaignActive(cfg Config, now time.Time) bool {
	if cfg.CampaignRatePct <= 0 {
		return false
	}
	if cfg.CampaignStartsAt != nil && now.Before(*cfg.CampaignStartsAt) {
		return false
	}
	if cfg.CampaignEndsAt != nil && now.After(*cfg.CampaignEndsAt) {
		return false
	}
	return true
}

// EffectiveEarnRate é a taxa que vale AGORA: a da campanha, se ativa; senão a base.
func EffectiveEarnRate(cfg Config, now time.Time) float64 {
	if campaignActive(cfg, now) {
		return cfg.CampaignRatePct
	}
	return cfg.EarnRatePct
}

// Earn calcula o cashback a creditar sobre uma base (mercadoria paga, já líquida
// de desconto e de cashback resgatado — não se acumula cashback sobre cashback).
// Programa desligado, base não-positiva ou abaixo do mínimo de acúmulo → zero.
// A taxa é a vigente em `now` (respeita campanha).
func Earn(cfg Config, basis float64, now time.Time) float64 {
	if !cfg.Active || basis <= 0 || basis < cfg.MinEarnSubtotal {
		return 0
	}
	rate := EffectiveEarnRate(cfg, now)
	if rate <= 0 {
		return 0
	}
	return round2(basis * rate / 100)
}

// ItemLine é uma linha do pedido pro acúmulo por categoria.
type ItemLine struct {
	CategoryID string
	// LineTotal = preço unitário × quantidade (bruto da linha, antes de desconto).
	LineTotal float64
}

// EarnByItems acumula POR ITEM, aplicando a taxa da categoria de cada um (ou a
// taxa efetiva base quando a categoria não tem override).
//
// O acúmulo é sobre o que foi REALMENTE pago (basisNet = total − frete),
// distribuído entre as linhas na proporção do valor de cada uma. Assim
// desconto/cupom/cashback reduzem a base de TODAS as linhas proporcionalmente e
// nunca se acumula sobre o que não foi pago. Categoria com override não pega o
// turbo da campanha (o override é explícito); categorias sem override seguem a
// taxa efetiva (campanha, se ativa; senão a base).
func EarnByItems(cfg Config, items []ItemLine, basisNet float64, categoryRates map[string]float64, now time.Time) float64 {
	if !cfg.Active || basisNet <= 0 || basisNet < cfg.MinEarnSubtotal {
		return 0
	}
	effRate := EffectiveEarnRate(cfg, now)
	var gross float64
	for _, it := range items {
		if it.LineTotal > 0 {
			gross += it.LineTotal
		}
	}
	// Sem itens úteis: cai no acúmulo achatado pela taxa efetiva (compat/robustez).
	if gross <= 0 {
		if effRate <= 0 {
			return 0
		}
		return round2(basisNet * effRate / 100)
	}
	ratio := basisNet / gross
	var sum float64
	for _, it := range items {
		if it.LineTotal <= 0 {
			continue
		}
		rate := effRate
		if r, ok := categoryRates[it.CategoryID]; ok {
			rate = r
		}
		sum += it.LineTotal * ratio * rate / 100
	}
	return round2(sum)
}

// ClampRedeem devolve quanto do cashback pedido PODE ser resgatado neste pedido:
// o menor entre o pedido do cliente, o saldo disponível e o teto% do subtotal.
// Nunca negativo. Respeita o pedido mínimo pra resgatar. É o ponto que impede o
// cliente de "gastar" mais do que tem ou de zerar o pedido além do permitido.
func ClampRedeem(cfg Config, requested, balance, subtotal float64) float64 {
	if !cfg.Active || requested <= 0 || balance <= 0 || subtotal <= 0 {
		return 0
	}
	if subtotal < cfg.MinRedeemSubtotal {
		return 0
	}
	cap := subtotal * cfg.RedeemMaxPct / 100
	allowed := math.Min(requested, math.Min(balance, cap))
	if allowed < 0 {
		return 0
	}
	return round2(allowed)
}
