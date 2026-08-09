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
