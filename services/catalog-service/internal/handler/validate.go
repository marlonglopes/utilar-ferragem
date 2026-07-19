package handler

import (
	"fmt"
	"regexp"
	"strings"
)

// Validação de entrada das rotas de escrita de catálogo.
//
// PORQUÊ existir separado dos CHECKs do banco: o CHECK é a última linha de
// defesa e devolve um erro de driver ("violates check constraint
// products_ncm_format") que não diz ao lojista o que ele digitou errado. Aqui
// a mensagem é acionável e vem no envelope {error,code,requestId}.

var (
	// GTIN-8/12/13/14. O modo de falha real é o Excel transformar o EAN em
	// notação científica ("7.89123E+12") — que aqui é rejeitado com nome.
	barcodeRe = regexp.MustCompile(`^[0-9]{8,14}$`)
	ncmRe     = regexp.MustCompile(`^[0-9]{8}$`)
	cfopRe    = regexp.MustCompile(`^[0-9]{4}$`)
	cestRe    = regexp.MustCompile(`^[0-9]{7}$`)
)

// unitsOfMeasure — unidades aceitas. Lista fechada de propósito: sem ela, a
// mesma grandeza vira "sc", "SC", "saco" e "Saco" no mesmo catálogo e nenhum
// relatório fecha. Ampliar é editar aqui (e é para ser uma decisão, não um
// efeito colateral de importação).
var unitsOfMeasure = map[string]string{
	"un":  "unidade",
	"pc":  "peça",
	"cx":  "caixa",
	"sc":  "saco",
	"br":  "barra",
	"rl":  "rolo",
	"jg":  "jogo",
	"par": "par",
	"cto": "cento",
	"mlh": "milheiro",
	"m":   "metro",
	"m2":  "metro quadrado",
	"m3":  "metro cúbico",
	"kg":  "quilograma",
	"l":   "litro",
	"lt":  "lata",
	"gl":  "galão",
}

// normalizeUnit aceita "SC", "Sc", " sc " e devolve "sc". Erro se desconhecida.
func normalizeUnit(s string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(s))
	if u == "" {
		return "", fmt.Errorf("unitOfMeasure cannot be empty")
	}
	if _, ok := unitsOfMeasure[u]; !ok {
		return "", fmt.Errorf("unitOfMeasure %q is not a known unit", s)
	}
	return u, nil
}

// validateBarcode devolve o código normalizado (sem espaços/hífens de digitação).
func validateBarcode(s string) (string, error) {
	b := strings.NewReplacer(" ", "", "-", "", ".", "").Replace(strings.TrimSpace(s))
	if b == "" {
		return "", nil // limpar o código é uma operação válida
	}
	if !barcodeRe.MatchString(b) {
		return "", fmt.Errorf("barcode %q must be 8 to 14 digits (GTIN-8/12/13/14)", s)
	}
	return b, nil
}

// validateFiscal checa NCM/CFOP/CEST/origem. Ponteiro nil = campo não enviado.
func validateFiscal(ncm, cfop, cest *string, origem *int) error {
	check := func(name string, v *string, re *regexp.Regexp, digits int) error {
		if v == nil || *v == "" {
			return nil
		}
		if !re.MatchString(strings.TrimSpace(*v)) {
			return fmt.Errorf("%s %q must be exactly %d digits", name, *v, digits)
		}
		return nil
	}
	if err := check("ncm", ncm, ncmRe, 8); err != nil {
		return err
	}
	if err := check("cfop", cfop, cfopRe, 4); err != nil {
		return err
	}
	if err := check("cest", cest, cestRe, 7); err != nil {
		return err
	}
	// Origem da mercadoria (tabela ICMS da NF-e): 0 a 8.
	if origem != nil && (*origem < 0 || *origem > 8) {
		return fmt.Errorf("origem %d must be between 0 and 8", *origem)
	}
	return nil
}

// validateMoney rejeita valores negativos e absurdos.
//
// O teto existe por causa do erro de vírgula na importação: "1.234,56" lido
// como "123456" passa em qualquer validação de "não-negativo" e vira um
// produto de R$ 123 mil na vitrine. maxMoney é folgado o bastante pra
// equipamento pesado e apertado o bastante pra pegar 3+ ordens de grandeza.
const maxMoney = 1_000_000.0

func validateMoney(field string, v float64) error {
	if v < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	if v > maxMoney {
		return fmt.Errorf("%s of %.2f exceeds the maximum of %.2f — check for a decimal separator mistake", field, v, maxMoney)
	}
	return nil
}

// validateQtyStep: passo zero ou negativo trava o seletor de quantidade do
// frontend em loop (incremento infinito) e torna impossível fechar o carrinho.
func validateQtyStep(v float64) error {
	if v <= 0 {
		return fmt.Errorf("qtyStep must be > 0")
	}
	if v > 100000 {
		return fmt.Errorf("qtyStep of %g is implausible", v)
	}
	return nil
}

func validateDimension(field string, v float64) error {
	if v < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	return nil
}
