package audit

import (
	"net"
	"net/netip"
	"strings"
)

// # Por que o IP é mascarado NA GRAVAÇÃO
//
// Endereço IP é dado pessoal sob a LGPD (art. 5º, I — dado relacionado a
// pessoa natural identificável): combinado com o horário, o provedor consegue
// apontar o assinante. Mascarar só na exibição é mitigação cosmética — o dado
// completo já teria cruzado a rede, entrado no banco e passado a existir em
// todo backup, dump e SELECT ad-hoc. A trilha é APPEND-ONLY: uma vez gravado
// errado, não há UPDATE que conserte (o trigger recusa). Ou nasce mascarado,
// ou fica para sempre.
//
// # Por que mascarar e não reter com prazo
//
// A alternativa seria guardar o IP completo com base legal (legítimo
// interesse), prazo de retenção e expurgo automático. Foi descartada por dois
// motivos:
//
//  1. Expurgo é impossível nesta tabela por construção. audit_log é
//     append-only garantido por trigger e sem GRANT de UPDATE/DELETE para a
//     app — exatamente a propriedade que dá valor à trilha. Um job de expurgo
//     exigiria abrir DELETE na tabela, o que destruiria a garantia que
//     justifica a trilha existir. Trocar integridade da auditoria por
//     retenção de IP é um péssimo negócio.
//  2. O prefixo preserva a utilidade forense real. A pergunta que se faz numa
//     investigação é "esses acessos vieram da mesma rede?" / "de que operadora
//     e região?" — e o /24 (IPv4) ou /48 (IPv6) responde as duas. O que se
//     perde é justamente a capacidade de individualizar o assinante, que é o
//     que a LGPD quer que a gente não guarde sem necessidade.
//
// Ou seja: minimização de dados (art. 6º, III) sem perda operacional.
//
// # Compatibilidade com a cadeia de hash — LEIA ANTES DE MEXER
//
// ActorIP entra no canonical() e portanto no hash. Isso NÃO quebra registros
// antigos porque o mascaramento acontece em UM único ponto: no RecordTx,
// ANTES de ComputeHash e antes do INSERT. O hash sempre foi e continua sendo
// calculado sobre exatamente o que está na coluna.
//
// Registro antigo tem "203.0.113.7" na coluna e o hash dele foi calculado com
// "203.0.113.7"; ao reler, VerifyChain recomputa a partir do valor lido e bate.
// Registro novo tem "203.0.113.0/24" na coluna e o hash foi calculado com
// "203.0.113.0/24"; também bate. A cadeia é heterogênea no formato do IP e
// íntegra mesmo assim.
//
// O que NÃO se pode fazer, nunca: aplicar MaskIP dentro de canonical(), no
// scanRecords ou em qualquer caminho de LEITURA. Isso normalizaria o valor
// antigo na hora de recomputar o hash, o hash deixaria de bater e a trilha
// inteira anterior a esta mudança apareceria como adulterada — um falso
// positivo em massa, que é o pior resultado possível: destrói a confiança na
// única ferramenta que detecta adulteração de verdade.

// Máscaras de rede aplicadas. /24 mantém a rede IPv4 (o que operadora/empresa
// aloca como bloco); /48 é o bloco tipicamente delegado a um assinante IPv6,
// e ir além disso (/56, /64) volta a individualizar.
const (
	maskBitsV4 = 24
	maskBitsV6 = 48
)

// IPUnparsed é o que gravamos quando não conseguimos interpretar o valor.
//
// Fail-CLOSED, ao contrário do resto do pacote: em caso de dúvida gravamos o
// sentinela e perdemos a informação, em vez de deixar passar cru. Um valor que
// não é IP válido pode ser um IP em formato inesperado (com porta, com zona,
// com lixo em volta) e devolver o original seria exatamente o vazamento que
// esta função existe para impedir.
const IPUnparsed = "unparsed"

// MaskIP reduz um endereço IP ao prefixo de rede: /24 em IPv4, /48 em IPv6.
// Devolve notação CIDR ("203.0.113.0/24", "2001:db8:abcd::/48") — canônica,
// reparseável por net.ParseCIDR e visivelmente NÃO é um endereço individual,
// o que evita que alguém mais adiante confunda o prefixo com o IP real.
//
// String vazia continua vazia: "não sabemos o IP" é informação legítima e não
// deve virar um prefixo falso.
func MaskIP(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	addr, ok := parseLoose(s)
	if !ok {
		return IPUnparsed
	}

	// IPv4 mapeado em IPv6 (::ffff:203.0.113.7) é um endereço IPv4 na prática;
	// mascarar como /48 deixaria os 4 octetos intactos dentro do prefixo.
	addr = addr.Unmap()

	bits := maskBitsV6
	if addr.Is4() {
		bits = maskBitsV4
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return IPUnparsed
	}
	return prefix.String()
}

// parseLoose aceita as formas em que um IP costuma chegar de proxy, de header
// e de RemoteAddr, porque qualquer uma delas não reconhecida vira IPUnparsed e
// perderíamos o dado forense por detalhe de formatação.
func parseLoose(s string) (netip.Addr, bool) {
	// Zona de link-local (fe80::1%eth0) — a zona é nome de interface local,
	// não identifica ninguém, e atrapalha o Prefix.
	if i := strings.IndexByte(s, '%'); i >= 0 {
		s = s[:i]
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return a, true
	}
	// "203.0.113.7:54321" ou "[2001:db8::1]:443" — RemoteAddr cru.
	if host, _, err := net.SplitHostPort(s); err == nil {
		if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return a, true
		}
	}
	// "[2001:db8::1]" sem porta.
	if t := strings.Trim(s, "[]"); t != s {
		if a, err := netip.ParseAddr(t); err == nil {
			return a, true
		}
	}
	return netip.Addr{}, false
}

// IsFullIP diz se a string é um endereço individual completo — isto é, o que
// NÃO pode existir na coluna actor_ip de registros novos.
//
// Existe para os testes e para auditoria de dados legados: rodar sobre um dump
// de audit_log mostra exatamente quantas linhas anteriores ao mascaramento
// ainda carregam IP completo. Não é usada no caminho de gravação — lá o que
// vale é MaskIP, que é incondicional.
func IsFullIP(s string) bool {
	_, ok := parseLoose(strings.TrimSpace(s))
	return ok
}
