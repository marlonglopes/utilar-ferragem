// Package shipping calcula frete no servidor.
//
// PORQUÊ existe: o handler de criação de pedido fazia
// `total := subtotal + req.ShippingCost` — o cliente ditava o próprio frete.
// Mandar `shippingCost: 0` funcionava. O cálculo agora vem daqui, de uma tabela
// no banco, e o valor do request é ignorado (ou validado, ver order.go).
//
// A função de cálculo é pura (tabela + entrada → opções) justamente pra ser
// testável sem banco: o teste monta as faixas em memória.
package shipping

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Rate é uma faixa da tabela de frete.
//
// MODELO: faixa de CEP × faixa de valor do carrinho. Não usamos peso porque o
// catálogo não tem coluna de peso hoje (products não tem `weight`) — inventar
// um peso default por item seria pior que usar o que temos. `CostPerItem`
// aproxima o efeito do volume: 20 sacos de cimento custam mais que 1 chave de
// fenda. Quando o catálogo ganhar peso, esta struct ganha WeightMin/WeightMax
// e o cálculo passa a considerá-lo sem mudar o contrato HTTP.
type Rate struct {
	ID           string
	ZoneName     string  // "Capital SP", "Grande SP", ...
	CEPStart     int     // 8 dígitos, inclusive
	CEPEnd       int     // 8 dígitos, inclusive
	ServiceCode  string  // "standard" | "express"
	ServiceName  string  // "Entrega padrão"
	BaseCost     float64 // custo fixo da faixa
	CostPerItem  float64 // acréscimo por unidade no carrinho
	DeliveryDays int     // prazo em dias úteis
	FreeAbove    float64 // subtotal a partir do qual o frete zera; 0 = nunca
	Active       bool
}

// Option é uma opção de frete devolvida ao frontend.
type Option struct {
	ServiceCode  string  `json:"serviceCode"`
	ServiceName  string  `json:"serviceName"`
	ZoneName     string  `json:"zoneName"`
	Cost         float64 `json:"cost"`
	DeliveryDays int     `json:"deliveryDays"`
	Free         bool    `json:"free"`
}

// Quote é a entrada do cálculo.
type Quote struct {
	CEP       string
	Subtotal  float64
	ItemCount int
}

var (
	// ErrInvalidCEP — CEP fora do formato de 8 dígitos.
	ErrInvalidCEP = errors.New("shipping: invalid CEP")
	// ErrNoCoverage — nenhuma faixa cobre esse CEP. Não entregamos lá.
	ErrNoCoverage = errors.New("shipping: no coverage for CEP")
)

var nonDigits = regexp.MustCompile(`\D`)

// NormalizeCEP tira hífen/espaço e valida 8 dígitos, devolvendo o valor
// numérico usado na comparação de faixas.
func NormalizeCEP(cep string) (int, error) {
	digits := nonDigits.ReplaceAllString(strings.TrimSpace(cep), "")
	if len(digits) != 8 {
		return 0, fmt.Errorf("%w: %q (expected 8 digits)", ErrInvalidCEP, cep)
	}
	n := 0
	for _, r := range digits {
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// Calculate devolve as opções de frete aplicáveis, ordenadas da mais barata
// para a mais cara (empate: prazo menor primeiro).
//
// Regras:
//   - só faixas ativas cujo intervalo de CEP contém o destino;
//   - custo = BaseCost + CostPerItem × itens;
//   - se FreeAbove > 0 e Subtotal >= FreeAbove, o custo zera (frete grátis);
//   - CEP sem cobertura → ErrNoCoverage (nunca "frete 0" silencioso).
//
// Valores são arredondados a 2 casas: o total do pedido é NUMERIC(12,2) no
// banco, e devolver 12.345000000001 na cotação faria o front mostrar um valor e
// o pedido gravar outro.
func Calculate(rates []Rate, q Quote) ([]Option, error) {
	cep, err := NormalizeCEP(q.CEP)
	if err != nil {
		return nil, err
	}
	if q.ItemCount < 0 {
		return nil, errors.New("shipping: itemCount must be >= 0")
	}
	if q.Subtotal < 0 {
		return nil, errors.New("shipping: subtotal must be >= 0")
	}

	// Dedup por serviço: se duas faixas cobrem o mesmo CEP com o mesmo
	// serviceCode (sobreposição na tabela), a mais barata vence. Sem isso a
	// cotação mostraria duas linhas "Entrega padrão" com preços diferentes.
	best := make(map[string]Option)
	for _, r := range rates {
		if !r.Active || cep < r.CEPStart || cep > r.CEPEnd {
			continue
		}

		cost := r.BaseCost + r.CostPerItem*float64(q.ItemCount)
		free := r.FreeAbove > 0 && q.Subtotal >= r.FreeAbove
		if free {
			cost = 0
		}
		if cost < 0 {
			cost = 0
		}

		opt := Option{
			ServiceCode:  r.ServiceCode,
			ServiceName:  r.ServiceName,
			ZoneName:     r.ZoneName,
			Cost:         round2(cost),
			DeliveryDays: r.DeliveryDays,
			Free:         free,
		}
		if prev, ok := best[r.ServiceCode]; !ok || opt.Cost < prev.Cost {
			best[r.ServiceCode] = opt
		}
	}

	if len(best) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoCoverage, q.CEP)
	}

	out := make([]Option, 0, len(best))
	for _, o := range best {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost < out[j].Cost
		}
		if out[i].DeliveryDays != out[j].DeliveryDays {
			return out[i].DeliveryDays < out[j].DeliveryDays
		}
		return out[i].ServiceCode < out[j].ServiceCode
	})
	return out, nil
}

// CostFor devolve o custo de um serviço específico. É o que o handler de
// criação de pedido usa: o cliente escolhe `shippingService`, o servidor
// precifica. Serviço desconhecido → erro, nunca fallback pra zero.
func CostFor(rates []Rate, q Quote, serviceCode string) (Option, error) {
	opts, err := Calculate(rates, q)
	if err != nil {
		return Option{}, err
	}
	if serviceCode == "" {
		// Sem escolha explícita, cobra a opção mais barata (já ordenada).
		return opts[0], nil
	}
	for _, o := range opts {
		if o.ServiceCode == serviceCode {
			return o, nil
		}
	}
	return Option{}, fmt.Errorf("shipping: service %q not available for CEP %s", serviceCode, q.CEP)
}

// round2 arredonda pra 2 casas (half away from zero), como o dinheiro espera.
func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}
