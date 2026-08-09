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

import "math"

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
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Earn calcula o cashback a creditar sobre uma base (mercadoria paga, já líquida
// de desconto e de cashback resgatado — não se acumula cashback sobre cashback).
// Programa desligado ou base não-positiva → zero.
func Earn(cfg Config, basis float64) float64 {
	if !cfg.Active || cfg.EarnRatePct <= 0 || basis <= 0 {
		return 0
	}
	return round2(basis * cfg.EarnRatePct / 100)
}

// ClampRedeem devolve quanto do cashback pedido PODE ser resgatado neste pedido:
// o menor entre o pedido do cliente, o saldo disponível e o teto% do subtotal.
// Nunca negativo. É o ponto que impede o cliente de "gastar" mais do que tem ou
// de zerar o pedido além do permitido.
func ClampRedeem(cfg Config, requested, balance, subtotal float64) float64 {
	if !cfg.Active || requested <= 0 || balance <= 0 || subtotal <= 0 {
		return 0
	}
	cap := subtotal * cfg.RedeemMaxPct / 100
	allowed := math.Min(requested, math.Min(balance, cap))
	if allowed < 0 {
		return 0
	}
	return round2(allowed)
}
