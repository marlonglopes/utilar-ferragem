package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMaskIPv4ReduzAoPrefixo24(t *testing.T) {
	casos := map[string]string{
		"192.168.1.37":    "192.168.1.0/24",
		"203.0.113.7":     "203.0.113.0/24",
		"8.8.8.8":         "8.8.8.0/24",
		"10.0.0.255":      "10.0.0.0/24",
		"127.0.0.1":       "127.0.0.0/24",
		"203.0.113.0/24":  IPUnparsed, // já mascarado não é IP: não re-mascaramos silenciosamente
		"  189.12.34.56 ": "189.12.34.0/24",
	}
	for in, want := range casos {
		if got := MaskIP(in); got != want {
			t.Errorf("MaskIP(%q) = %q, esperado %q", in, got, want)
		}
	}
}

func TestMaskIPv6ReduzAoPrefixo48(t *testing.T) {
	casos := map[string]string{
		"2001:db8:abcd:1234::1":                   "2001:db8:abcd::/48",
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334": "2001:db8:85a3::/48",
		"::1":                         "::/48",
		"fe80::1%eth0":                "fe80::/48", // zona de interface é local, não identifica
		"[2001:db8:abcd:1234::1]:443": "2001:db8:abcd::/48",
	}
	for in, want := range casos {
		if got := MaskIP(in); got != want {
			t.Errorf("MaskIP(%q) = %q, esperado %q", in, got, want)
		}
	}
}

// IPv4 mapeado em IPv6 é IPv4 na prática. Mascarar como /48 deixaria os quatro
// octetos visíveis dentro do prefixo — o vazamento que a função existe pra evitar.
func TestMaskIPv4MapeadoEmV6VaiPara24(t *testing.T) {
	if got := MaskIP("::ffff:203.0.113.7"); got != "203.0.113.0/24" {
		t.Fatalf("IPv4-mapeado = %q, esperado 203.0.113.0/24", got)
	}
}

func TestMaskIPFormatosDeProxy(t *testing.T) {
	if got := MaskIP("203.0.113.7:54321"); got != "203.0.113.0/24" {
		t.Errorf("host:port = %q", got)
	}
	if got := MaskIP(""); got != "" {
		t.Errorf(`vazio deve continuar vazio, veio %q`, got)
	}
	// Fail-closed: o que não entendemos vira sentinela, nunca volta cru.
	for _, lixo := range []string{"unknown", "não-é-ip", "999.999.999.999", "<script>"} {
		if got := MaskIP(lixo); got != IPUnparsed {
			t.Errorf("MaskIP(%q) = %q, esperado %q", lixo, got, IPUnparsed)
		}
	}
}

// Este é o teste que FALHA se alguém desligar o mascaramento na gravação.
// Varre a saída de MaskIP com IsFullIP: se o valor gravado ainda for um
// endereço individual parseável, o mascaramento não aconteceu.
func TestNenhumIPCompletoSobreviveAoMascaramento(t *testing.T) {
	entradas := []string{
		"192.168.1.37", "203.0.113.7", "8.8.8.8", "1.2.3.4",
		"2001:db8:abcd:1234::1", "fe80::1%eth0", "::ffff:203.0.113.7",
		"::1", "203.0.113.7:54321", "[2001:db8::1]:443",
	}
	for _, in := range entradas {
		got := MaskIP(in)
		if IsFullIP(got) {
			t.Errorf("VAZAMENTO: MaskIP(%q) = %q, que ainda é um IP completo", in, got)
		}
		if got == in {
			t.Errorf("VAZAMENTO: MaskIP(%q) devolveu a entrada intacta", in)
		}
	}
}

// A blindagem acima só vale se IsFullIP realmente reconhecer um IP completo —
// senão o teste passaria por incompetência do detector.
func TestIsFullIPReconheceEnderecoIndividual(t *testing.T) {
	for _, s := range []string{"192.168.1.37", "2001:db8::1", "203.0.113.7:80"} {
		if !IsFullIP(s) {
			t.Errorf("IsFullIP(%q) = false, deveria reconhecer IP completo", s)
		}
	}
	for _, s := range []string{"", "192.168.1.0/24", "2001:db8::/48", IPUnparsed} {
		if IsFullIP(s) {
			t.Errorf("IsFullIP(%q) = true, não é endereço individual", s)
		}
	}
}

// ---------- compatibilidade da cadeia ----------

// legacyRecord monta um registro no formato ANTIGO: IP completo na coluna e
// hash calculado sobre ele. É o que existe hoje no banco de produção.
func legacyRecord(seq int64, prev string, ip string) Record {
	r := Record{
		Seq:            seq,
		OccurredAt:     time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute),
		Service:        "payment-service",
		ActorID:        "user-42",
		ActorRole:      "admin",
		ActorIP:        ip,
		ActorUserAgent: "Mozilla/5.0",
		EntityType:     "payment",
		EntityID:       "pay-1",
		Action:         string(ActionUpdate),
		NewValue:       json.RawMessage(`{"status":"confirmed"}`),
		RequestID:      "req-1",
		PrevHash:       prev,
	}
	r.Hash = ComputeHash(r)
	return r
}

// O ponto central da mudança: registro gravado ANTES do mascaramento continua
// verificável. Se alguém mover MaskIP para dentro de canonical() ou para o
// caminho de leitura, este teste quebra — e é exatamente o que deve acontecer,
// porque nesse cenário a trilha inteira anterior à mudança apareceria como
// adulterada.
func TestRegistroLegadoComIPCompletoContinuaVerificavel(t *testing.T) {
	legado := legacyRecord(1, GenesisHash, "203.0.113.7")
	if err := VerifyChain([]Record{legado}, GenesisHash); err != nil {
		t.Fatalf("registro legado deixou de verificar: %v", err)
	}
	if !IsFullIP(legado.ActorIP) {
		t.Fatal("o teste perdeu o sentido: o registro legado precisa ter IP completo")
	}
}

// Cadeia mista — metade antiga (IP completo), metade nova (prefixo). É o
// estado real do banco no dia seguinte ao deploy, e tem que verificar inteira.
func TestCadeiaMistaLegadoMaisMascaradoVerifica(t *testing.T) {
	var recs []Record
	prev := GenesisHash

	for seq := int64(1); seq <= 3; seq++ {
		r := legacyRecord(seq, prev, "203.0.113.7")
		prev = r.Hash
		recs = append(recs, r)
	}
	for seq := int64(4); seq <= 6; seq++ {
		r := legacyRecord(seq, prev, MaskIP("203.0.113.7"))
		prev = r.Hash
		recs = append(recs, r)
	}

	if err := VerifyChain(recs, GenesisHash); err != nil {
		t.Fatalf("cadeia mista deveria verificar: %v", err)
	}
	if !IsFullIP(recs[0].ActorIP) {
		t.Error("os 3 primeiros deveriam estar no formato legado")
	}
	if IsFullIP(recs[3].ActorIP) {
		t.Error("os 3 últimos deveriam estar mascarados")
	}
}

// O IP continua fazendo parte do hash: trocar o prefixo de um registro
// mascarado tem que ser detectado. Se o mascaramento tivesse tirado o IP do
// canonical, adulterar a origem de um acesso ficaria invisível.
func TestAdulterarPrefixoMascaradoQuebraACadeia(t *testing.T) {
	recs := []Record{legacyRecord(1, GenesisHash, MaskIP("203.0.113.7"))}
	recs[0].ActorIP = MaskIP("198.51.100.7") // "veio de outra rede"

	err := VerifyChain(recs, GenesisHash)
	if err == nil {
		t.Fatal("adulteração do prefixo passou despercebida")
	}
	var ce *ChainError
	if !asChainError(err, &ce) || ce.Kind != "hash_mismatch" {
		t.Fatalf("esperado hash_mismatch, veio %v", err)
	}
}

func asChainError(err error, target **ChainError) bool {
	ce, ok := err.(*ChainError)
	if ok {
		*target = ce
	}
	return ok
}

// Mascarar é idempotente do ponto de vista do hash: mesmo IP → mesmo prefixo →
// mesmo hash. Determinismo é pré-requisito da verificação.
func TestMaskIPDeterministico(t *testing.T) {
	a := legacyRecord(1, GenesisHash, MaskIP("192.168.1.37"))
	b := legacyRecord(1, GenesisHash, MaskIP("192.168.1.200"))
	if a.Hash != b.Hash {
		t.Fatal("dois IPs da mesma /24 deveriam produzir o mesmo registro canônico")
	}

	c := legacyRecord(1, GenesisHash, MaskIP("192.168.2.37"))
	if a.Hash == c.Hash {
		t.Fatal("/24 diferentes não podem colidir")
	}
}
