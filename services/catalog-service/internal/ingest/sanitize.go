package ingest

import "strings"

// Defesa contra CSV / Formula Injection (CWE-1236).
//
// O ATAQUE: o fornecedor manda uma planilha com a célula
//
//	=cmd|'/c calc'!A1
//
// Nada acontece na nossa importação — guardamos a string e seguimos. O estrago
// vem DEPOIS: alguém exporta o catálogo pra CSV, abre no Excel, e o Excel
// executa aquilo como fórmula. Variantes conhecidas usam DDE (`=cmd|...`),
// `=HYPERLINK(...)` pra exfiltrar dado via URL, e `=WEBSERVICE(...)` pra
// vazar o conteúdo da planilha inteira pra um servidor do atacante.
//
// Por isso o saneamento acontece na ENTRADA, não na saída: o dado fica limpo
// no banco e qualquer consumidor futuro (export CSV, relatório, integração com
// ERP, download do admin) herda a proteção sem precisar lembrar dela. Sanear
// só na saída significa que a primeira rota de export escrita por alguém que
// não leu este comentário reabre o buraco.
//
// A escolha de PREFIXAR com apóstrofo em vez de rejeitar a linha: existe
// produto legítimo cujo nome começa com "-" ("-30% OFF") ou "+" ("+Cola
// Extra"). Rejeitar perderia dado real; o apóstrofo neutraliza a fórmula e o
// Excel não o exibe ao reabrir.

// formulaTriggers — os caracteres que fazem o Excel/LibreOffice/Sheets tratar
// a célula como fórmula.
//
// `\t`, `\r` e `\n` estão na lista porque o Excel os ignora como espaço em
// branco à esquerda: "\t=cmd|..." ainda é executado como fórmula, e um filtro
// que só olha o primeiro byte deixa passar.
const formulaTriggers = "=+-@\t\r\n"

// SanitizeCell neutraliza fórmula em texto vindo de arquivo. Idempotente:
// aplicar duas vezes não acumula apóstrofos — importante porque o mesmo valor
// passa pelo staging e pelo mapeamento.
func SanitizeCell(s string) string {
	if s == "" {
		return s
	}
	// Já saneado numa passagem anterior.
	if strings.HasPrefix(s, "'") {
		return s
	}
	if strings.ContainsRune(formulaTriggers, rune(s[0])) {
		return "'" + s
	}
	return s
}

// IsFormula diz se a célula PARECE uma tentativa de fórmula — usado pra emitir
// aviso na revisão humana, não só sanear em silêncio. Uma planilha com muitas
// células assim não é descuido de formatação: é alguém testando o sistema.
func IsFormula(s string) bool {
	t := strings.TrimLeft(s, " \t\r\n")
	if t == "" {
		return false
	}
	if !strings.ContainsRune("=+-@", rune(t[0])) {
		return false
	}
	// "-30%" e "+5" são valores, não fórmula. Fórmula tem função ou referência.
	upper := strings.ToUpper(t)
	for _, sig := range []string{"CMD|", "DDE", "HYPERLINK(", "WEBSERVICE(", "IMPORTXML(",
		"IMPORTDATA(", "IMPORTRANGE(", "MSEXCEL|", "EXEC(", "CALL(", "!A1"} {
		if strings.Contains(upper, sig) {
			return true
		}
	}
	// "=ALGO(" genérico.
	return strings.HasPrefix(t, "=") && strings.Contains(t, "(")
}
