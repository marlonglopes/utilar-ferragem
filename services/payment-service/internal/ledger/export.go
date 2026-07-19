package ledger

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Exportação para o contador.
//
// O QUE UM CONTADOR BRASILEIRO REALMENTE PRECISA (e por que o CSV "óbvio" não serve):
//
//  1. SEPARADOR `;` e DECIMAL COM VÍRGULA. O Excel em pt-BR abre CSV com
//     vírgula-decimal e separador ponto-e-vírgula. Entregar "1234.56" com
//     separador vírgula faz o Excel ler R$ 1.234,56 como duas colunas — e o
//     contador vai conferir 300 linhas na mão antes de descobrir.
//  2. BOM UTF-8. Sem ele o Excel abre "Cartão" como "CartÃ£o".
//  3. DATA dd/mm/aaaa.
//  4. DÉBITO e CRÉDITO em COLUNAS SEPARADAS, não uma coluna com sinal. É como
//     o razão é lido e como os sistemas contábeis (Domínio, Alterdata) importam.
//  5. HISTÓRICO em português e o DOCUMENTO de origem — sem isso o lançamento
//     não é conciliável com a nota/pedido e o contador devolve o arquivo.
//  6. OFX pra bater com o extrato: é o formato que os sistemas contábeis
//     importam nativamente pra conciliação bancária.

const (
	csvSep     = ';'
	utf8BOM    = "\uFEFF"
	dateBR     = "02/01/2006"
	dateTimeBR = "02/01/2006 15:04:05"
)

// reaisBR formata centavos como "1234,56" (sem separador de milhar: milhar com
// ponto quebra a importação de vários sistemas contábeis).
func reaisBR(c Cents) string {
	neg := ""
	v := int64(c)
	if v < 0 {
		neg, v = "-", -v
	}
	return fmt.Sprintf("%s%d,%02d", neg, v/100, v%100)
}

// ExportRazaoCSV escreve o livro razão da janela [from, to).
func (r *Reports) ExportRazaoCSV(ctx context.Context, w io.Writer, from, to time.Time) error {
	entries, err := r.Entries(ctx, from, to, 50000)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma = csvSep
	defer cw.Flush()

	if err := cw.Write([]string{
		"Data", "Hora", "Lancamento", "Historico", "Natureza",
		"Conta", "Nome da Conta", "Debito", "Credito",
		"Documento", "Tipo do Documento", "Forma de Pagamento", "Vendedor", "Request ID",
	}); err != nil {
		return err
	}

	for _, e := range entries {
		debito, credito := "", ""
		if e.Side == Debit {
			debito = reaisBR(e.AmountCents)
		} else {
			credito = reaisBR(e.AmountCents)
		}
		historico := e.Description
		if e.Memo != "" {
			historico = historico + " — " + e.Memo
		}
		if err := cw.Write([]string{
			e.OccurredAt.UTC().Format(dateBR),
			e.OccurredAt.UTC().Format("15:04:05"),
			e.TxID,
			historico,
			naturezaPT(e.Kind),
			string(e.Account),
			e.AccountName,
			debito,
			credito,
			e.SourceID,
			string(e.SourceType),
			metodoPT(e.Method),
			e.SellerID,
			e.RequestID,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ExportBalanceteCSV escreve o balancete da janela.
func (r *Reports) ExportBalanceteCSV(ctx context.Context, w io.Writer, from, to time.Time) error {
	tb, err := r.TrialBalance(ctx, from, to)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma = csvSep
	defer cw.Flush()

	if err := cw.Write([]string{"Conta", "Nome da Conta", "Natureza", "Debitos", "Creditos", "Saldo"}); err != nil {
		return err
	}
	for _, l := range tb.Lines {
		if err := cw.Write([]string{
			string(l.Account), l.Name, tipoContaPT(l.Type),
			reaisBR(l.DebitsCents), reaisBR(l.CreditsCents), reaisBR(l.BalanceCents),
		}); err != nil {
			return err
		}
	}
	// A linha de totais é o que o contador confere primeiro: se débitos !=
	// créditos, o arquivo inteiro é suspeito e ele nem começa a importar.
	if err := cw.Write([]string{"", "TOTAL", "", reaisBR(tb.TotalDebits), reaisBR(tb.TotalCredits), ""}); err != nil {
		return err
	}
	if !tb.Balanced {
		if err := cw.Write([]string{"", "ATENCAO: BALANCETE NAO FECHA", "", "", "",
			reaisBR(tb.TotalDebits - tb.TotalCredits)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func naturezaPT(k Kind) string {
	switch k {
	case KindSale:
		return "Venda"
	case KindExternalSale:
		return "Venda externa (maquininha)"
	case KindPSPFee:
		return "Taxa do gateway"
	case KindRefund:
		return "Estorno"
	case KindChargeback:
		return "Chargeback"
	case KindSellerSplit:
		return "Repasse a vendedor"
	case KindSellerWithdrawal:
		return "Saque do vendedor"
	case KindPayout:
		return "Transferencia PSP-Banco"
	case KindAnticipationFee:
		return "Taxa de antecipacao"
	case KindReversal:
		return "Estorno de lancamento"
	case KindAdjustment:
		return "Ajuste"
	default:
		return string(k)
	}
}

func metodoPT(m string) string {
	switch m {
	case "pix":
		return "PIX"
	case "boleto":
		return "Boleto"
	case "card":
		return "Cartao de credito"
	case MethodExternal:
		// Rótulo explícito no CSV do contador: "Cartao de credito" aqui faria
		// a maquininha da loja se confundir com o cartão processado pelo PSP,
		// que é exatamente o erro que esta feature existe pra corrigir.
		return "Maquininha da loja (externo)"
	default:
		return m
	}
}

func tipoContaPT(t string) string {
	switch t {
	case "asset":
		return "Ativo"
	case "liability":
		return "Passivo"
	case "equity":
		return "Patrimonio Liquido"
	case "revenue":
		return "Receita"
	case "expense":
		return "Despesa"
	default:
		return t
	}
}

// ===================== OFX =====================

// OFXOptions parametriza o extrato.
type OFXOptions struct {
	BankID  string  // código do banco (ex "0000" quando é conta PSP, não banco)
	AcctID  string  // identificação da conta no extrato
	Account Account // conta do razão a exportar (default: caixa no PSP)
}

// ExportOFX escreve um extrato OFX 1.0.2 (SGML) da conta de caixa.
//
// POR QUE OFX 1.0.2 (SGML) E NÃO OFX 2 (XML): os sistemas contábeis brasileiros
// (Domínio, Alterdata, Questor) importam o 1.0.2. O 2.x é XML bem-formado e
// tecnicamente melhor, e é rejeitado por metade deles. Interoperabilidade
// ganha de elegância aqui.
//
// Convenção de sinal do OFX: TRNAMT positivo = entrou na conta; negativo =
// saiu. Como exportamos a ótica da conta de CAIXA (ativo, natureza devedora),
// débito = entrada (positivo) e crédito = saída (negativo).
func (r *Reports) ExportOFX(ctx context.Context, w io.Writer, from, to time.Time, opt OFXOptions) error {
	if opt.Account == "" {
		opt.Account = AcctCaixaPSP
	}
	if opt.BankID == "" {
		opt.BankID = "0000"
	}
	if opt.AcctID == "" {
		opt.AcctID = "UTILAR-PSP"
	}

	entries, err := r.Entries(ctx, from, to, 50000)
	if err != nil {
		return err
	}

	var b strings.Builder
	now := time.Now().UTC()
	b.WriteString("OFXHEADER:100\r\nDATA:OFXSGML\r\nVERSION:102\r\nSECURITY:NONE\r\n")
	b.WriteString("ENCODING:UTF-8\r\nCHARSET:NONE\r\nCOMPRESSION:NONE\r\n")
	b.WriteString("OLDFILEUID:NONE\r\nNEWFILEUID:NONE\r\n\r\n")
	b.WriteString("<OFX>\r\n<SIGNONMSGSRSV1><SONRS><STATUS><CODE>0<SEVERITY>INFO</STATUS>\r\n")
	b.WriteString("<DTSERVER>" + ofxTime(now) + "<LANGUAGE>POR</SONRS></SIGNONMSGSRSV1>\r\n")
	b.WriteString("<BANKMSGSRSV1><STMTTRNRS><TRNUID>1<STATUS><CODE>0<SEVERITY>INFO</STATUS>\r\n")
	b.WriteString("<STMTRS><CURDEF>BRL\r\n")
	b.WriteString("<BANKACCTFROM><BANKID>" + ofxEscape(opt.BankID) + "<ACCTID>" + ofxEscape(opt.AcctID) + "<ACCTTYPE>CHECKING</BANKACCTFROM>\r\n")
	b.WriteString("<BANKTRANLIST><DTSTART>" + ofxTime(from) + "<DTEND>" + ofxTime(to) + "\r\n")

	var saldo Cents
	for _, e := range entries {
		if e.Account != opt.Account {
			continue
		}
		amount := e.AmountCents
		trnType := "CREDIT"
		if e.Side == Credit { // saída de caixa
			amount = -amount
			trnType = "DEBIT"
		}
		saldo += amount

		b.WriteString("<STMTTRN>")
		b.WriteString("<TRNTYPE>" + trnType)
		b.WriteString("<DTPOSTED>" + ofxTime(e.OccurredAt))
		b.WriteString("<TRNAMT>" + ofxAmount(amount))
		// FITID tem que ser único e ESTÁVEL: reimportar o mesmo período não pode
		// duplicar lançamento no sistema do contador. tx-id + conta + lado é
		// determinístico e sobrevive a reexport.
		b.WriteString("<FITID>" + ofxEscape(e.TxID+"-"+string(e.Account)+"-"+string(e.Side)))
		b.WriteString("<MEMO>" + ofxEscape(truncateRunes(naturezaPT(e.Kind)+": "+e.Description, 250)))
		b.WriteString("</STMTTRN>\r\n")
	}

	b.WriteString("</BANKTRANLIST>\r\n")
	b.WriteString("<LEDGERBAL><BALAMT>" + ofxAmount(saldo) + "<DTASOF>" + ofxTime(to) + "</LEDGERBAL>\r\n")
	b.WriteString("</STMTRS></STMTTRNRS></BANKMSGSRSV1>\r\n</OFX>\r\n")

	_, err = io.WriteString(w, b.String())
	return err
}

// ofxTime formata no padrão OFX: YYYYMMDDHHMMSS[+0:UTC].
func ofxTime(t time.Time) string {
	return t.UTC().Format("20060102150405") + "[0:GMT]"
}

// ofxAmount usa PONTO decimal — OFX é padrão americano, ao contrário do CSV.
// Trocar os dois é o erro clássico e produz um extrato que importa valores
// mil vezes maiores.
func ofxAmount(c Cents) string {
	neg := ""
	v := int64(c)
	if v < 0 {
		neg, v = "-", -v
	}
	return neg + strconv.FormatInt(v/100, 10) + "." + fmt.Sprintf("%02d", v%100)
}

// ofxEscape neutraliza `<`, `>` e `&`: OFX 1.0.2 é SGML e uma descrição com
// "<" quebraria o parser do sistema do contador.
func ofxEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return strings.ReplaceAll(s, "\n", " ")
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
