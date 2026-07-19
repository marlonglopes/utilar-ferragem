package ingest

import "strings"

// Normalização de unidade de medida.
//
// PORQUÊ separado da lista do handler: o handler valida entrada de ADMIN, onde
// "unidade desconhecida" deve ser erro — o operador digitou errado e precisa
// corrigir. Aqui é entrada de ARQUIVO, onde a mesma grandeza chega como "UN",
// "un", "Unid.", "UNIDADE" e "PC" na mesma planilha, e rejeitar cada variação
// reprovaria metade do catálogo por um detalhe de digitação do fornecedor.
//
// A lista de destino é a mesma de handler/validate.go (`unitsOfMeasure`) —
// tem que continuar sendo, senão o importador grava uma unidade que a rota
// admin recusa e o produto fica ineditável. O teste
// TestUnidadesCanonicasBatemComOHandler trava esse acoplamento.

// unitAliases mapeia o que aparece em arquivo → a unidade canônica da casa.
//
// As entradas em MAIÚSCULA sem acento vêm do vocabulário real do SINAPI
// (o arquivo oficial é todo em ASCII maiúsculo). `SC25KG` é o caso curioso:
// o SINAPI embute o peso na unidade — saco de 25 kg. Mapeamos pra "sc" e o
// peso vira `weight_kg`, que é onde ele serve pra calcular frete.
var unitAliases = map[string]string{
	// unidade
	"un": "un", "und": "un", "unid": "un", "unidade": "un", "uni": "un", "ea": "un",
	// peça
	"pc": "pc", "pç": "pc", "peca": "pc", "peça": "pc", "pcs": "pc", "pe": "pc",
	// caixa
	"cx": "cx", "caixa": "cx", "cxa": "cx", "box": "cx",
	// conjunto / jogo — SINAPI usa CJ
	"cj": "jg", "conj": "jg", "conjunto": "jg", "jg": "jg", "jogo": "jg", "kit": "jg",
	// saco
	"sc": "sc", "saco": "sc", "sac": "sc", "sc25kg": "sc", "sc50kg": "sc", "sc20kg": "sc",
	// barra
	"br": "br", "barra": "br", "bar": "br", "vara": "br",
	// rolo
	"rl": "rl", "rolo": "rl", "rol": "rl", "bobina": "rl",
	// par
	"par": "par", "pr": "par",
	// cento / milheiro
	"cto": "cto", "cento": "cto", "ct": "cto", "c/100": "cto",
	"mlh": "mlh", "milheiro": "mlh", "mil": "mlh", "mi": "mlh",
	// comprimento / área / volume
	"m": "m", "mt": "m", "metro": "m", "ml": "m", "m linear": "m", "100m": "m",
	"m2": "m2", "m²": "m2", "metro quadrado": "m2", "mq": "m2",
	"m3": "m3", "m³": "m3", "metro cubico": "m3", "metro cúbico": "m3", "mc": "m3",
	// massa
	"kg": "kg", "quilo": "kg", "quilograma": "kg", "kilo": "kg", "k": "kg",
	"t": "kg", "ton": "kg", "tonelada": "kg", // convertido: ver ConvertQuantity
	// volume líquido
	"l": "l", "lt": "l", "litro": "l", "lts": "l", "310ml": "un",
	// embalagens
	"lata": "lt", "gl": "gl", "galao": "gl", "galão": "gl", "balde": "gl",
}

// laborUnits — unidades que denunciam MÃO DE OBRA ou locação, não material.
//
// Isto importa muito na ingestão do SINAPI: a tabela de insumos traz pedreiro
// (H = hora), locação de equipamento (MES, DIA, UNXMES, M2XMES) e energia
// (KWH) misturados com cimento e areia. Nada disso é item de prateleira de
// ferragem, e importar "SERVENTE COM ENCARGOS COMPLEMENTARES - HORA" como
// produto colocaria mão de obra na vitrine.
var laborUnits = map[string]bool{
	"h": true, "hora": true, "hr": true,
	"mes": true, "mês": true, "dia": true, "diaria": true,
	"unxmes": true, "mxmes": true, "m2xmes": true, "m3xmes": true,
	"kwh": true, "cv": true, "hp": true,
}

// NormalizeUnit devolve a unidade canônica. `ok=false` quando não reconhecida —
// o chamador decide entre avisar e assumir "un" (importação) ou rejeitar
// (entrada admin).
func NormalizeUnit(s string) (string, bool) {
	u := strings.ToLower(strings.TrimSpace(CleanText(s)))
	u = strings.TrimSuffix(u, ".")
	if u == "" {
		return "", false
	}
	if v, ok := unitAliases[u]; ok {
		return v, true
	}
	// Sem acento, como o SINAPI grava.
	if v, ok := unitAliases[strings.ToLower(stripAccents(u))]; ok {
		return v, true
	}
	return "", false
}

// IsLaborUnit diz se a unidade indica serviço/mão de obra/locação.
func IsLaborUnit(s string) bool {
	u := strings.ToLower(stripAccents(strings.TrimSpace(CleanText(s))))
	return laborUnits[u]
}

// UnitWeightKg extrai o peso embutido na unidade do SINAPI ("SC25KG" → 25).
// Devolve 0 quando não há peso embutido.
func UnitWeightKg(s string) float64 {
	u := strings.ToUpper(strings.TrimSpace(CleanText(s)))
	if !strings.HasPrefix(u, "SC") || !strings.HasSuffix(u, "KG") {
		return 0
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(u, "SC"), "KG")
	if v, ok, _ := ParseNumber("", mid); ok && v > 0 && v < 10000 {
		return v
	}
	return 0
}

// ConvertQuantity ajusta a quantidade quando a unidade de origem é um MÚLTIPLO
// da canônica. Tonelada → quilo é o caso real: o SINAPI cota aço em T e a
// ferragem vende em KG; sem converter, "2 T" viraria "2 kg" — erro de mil vezes
// no estoque, silencioso, e só descoberto na primeira venda.
func ConvertQuantity(fromUnit string, qty float64) (float64, string, bool) {
	u := strings.ToLower(stripAccents(strings.TrimSpace(CleanText(fromUnit))))
	switch u {
	case "t", "ton", "tonelada":
		return qty * 1000, "kg", true
	case "100m":
		return qty * 100, "m", true
	}
	return qty, "", false
}
