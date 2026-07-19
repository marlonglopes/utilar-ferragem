package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// chain monta uma cadeia válida de n registros — o ponto de partida de todo
// teste de adulteração abaixo.
func chain(t *testing.T, n int) []Record {
	t.Helper()
	base := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	out := make([]Record, 0, n)
	prev := GenesisHash
	for i := 0; i < n; i++ {
		r := Record{
			Seq:        int64(i + 1),
			OccurredAt: base.Add(time.Duration(i) * time.Minute),
			Service:    "payment-service",
			ActorID:    "user-42",
			ActorRole:  "customer",
			ActorIP:    "203.0.113.7",
			EntityType: "payment",
			EntityID:   "pay-" + string(rune('a'+i)),
			Action:     string(ActionUpdate),
			NewValue:   json.RawMessage(`{"status":"confirmed"}`),
			RequestID:  "01J0000000000000000000000" + string(rune('0'+i)),
			PrevHash:   prev,
		}
		r.Hash = ComputeHash(r)
		prev = r.Hash
		out = append(out, r)
	}
	return out
}

func TestVerifyChainAceitaCadeiaIntacta(t *testing.T) {
	if err := VerifyChain(chain(t, 10), GenesisHash); err != nil {
		t.Fatalf("cadeia íntegra recusada: %v", err)
	}
}

func TestComputeHashEhDeterministicoEHex64(t *testing.T) {
	r := chain(t, 1)[0]
	h1, h2 := ComputeHash(r), ComputeHash(r)
	if h1 != h2 {
		t.Fatalf("hash não determinístico: %s != %s", h1, h2)
	}
	if len(h1) != 64 || strings.ToLower(h1) != h1 {
		t.Fatalf("hash fora do formato hex-64 minúsculo: %q", h1)
	}
}

// O hash tem que mudar se QUALQUER campo mudar. Um campo esquecido no
// canonical() é um buraco por onde passa adulteração invisível.
func TestHashCobreTodosOsCamposDoRegistro(t *testing.T) {
	base := chain(t, 1)[0]
	mutacoes := map[string]func(*Record){
		"Seq":            func(r *Record) { r.Seq = 99 },
		"OccurredAt":     func(r *Record) { r.OccurredAt = r.OccurredAt.Add(time.Second) },
		"Service":        func(r *Record) { r.Service = "auth-service" },
		"ActorID":        func(r *Record) { r.ActorID = "user-43" },
		"ActorRole":      func(r *Record) { r.ActorRole = "admin" },
		"ActorIP":        func(r *Record) { r.ActorIP = "10.0.0.1" },
		"ActorUserAgent": func(r *Record) { r.ActorUserAgent = "curl/8" },
		"EntityType":     func(r *Record) { r.EntityType = "order" },
		"EntityID":       func(r *Record) { r.EntityID = "outro" },
		"Action":         func(r *Record) { r.Action = "delete" },
		"OldValue":       func(r *Record) { r.OldValue = json.RawMessage(`{"a":1}`) },
		"NewValue":       func(r *Record) { r.NewValue = json.RawMessage(`{"status":"failed"}`) },
		"RequestID":      func(r *Record) { r.RequestID = "outro-req" },
		"PrevHash":       func(r *Record) { r.PrevHash = strings.Repeat("f", 64) },
	}
	original := ComputeHash(base)
	for campo, mutar := range mutacoes {
		r := base
		mutar(&r)
		if got := ComputeHash(r); got == original {
			t.Errorf("campo %s não entra no hash — adulteração passaria despercebida", campo)
		}
	}
}

// Length-prefix no canonical: dois registros com campos "deslocados" não podem
// colidir. Sem o prefixo, ("ab","c") e ("a","bc") gerariam o mesmo hash.
func TestCanonicalNaoEhAmbiguo(t *testing.T) {
	a := Record{Seq: 1, PrevHash: GenesisHash, EntityType: "ab", EntityID: "c", Action: "x"}
	b := Record{Seq: 1, PrevHash: GenesisHash, EntityType: "a", EntityID: "bc", Action: "x"}
	if ComputeHash(a) == ComputeHash(b) {
		t.Fatal("colisão por ambiguidade de serialização — canonical precisa de length-prefix")
	}
}

// ===== detecção de adulteração =====

func TestVerifyChainDetectaAdulteracaoDeConteudo(t *testing.T) {
	recs := chain(t, 5)
	// Atacante edita o valor do registro do meio e deixa o hash antigo.
	recs[2].NewValue = json.RawMessage(`{"status":"confirmed","amount_cents":1}`)

	var ce *ChainError
	err := VerifyChain(recs, GenesisHash)
	if !errors.As(err, &ce) {
		t.Fatalf("adulteração não detectada: %v", err)
	}
	if ce.Kind != "hash_mismatch" || ce.Seq != 3 {
		t.Fatalf("apontou o lugar errado: kind=%s seq=%d", ce.Kind, ce.Seq)
	}
}

// O caso esperto: o atacante recomputa o hash do registro que alterou. A cadeia
// quebra no elo SEGUINTE, porque o prev_hash dele ficou órfão.
func TestVerifyChainDetectaAdulteracaoComHashRecomputado(t *testing.T) {
	recs := chain(t, 5)
	recs[2].NewValue = json.RawMessage(`{"status":"confirmed","amount_cents":1}`)
	recs[2].Hash = ComputeHash(recs[2]) // recomputa só o dele

	var ce *ChainError
	err := VerifyChain(recs, GenesisHash)
	if !errors.As(err, &ce) {
		t.Fatalf("adulteração com re-hash não detectada: %v", err)
	}
	if ce.Kind != "prev_mismatch" || ce.Seq != 4 {
		t.Fatalf("esperava prev_mismatch no seq=4, veio kind=%s seq=%d", ce.Kind, ce.Seq)
	}
}

func TestVerifyChainDetectaRegistroApagado(t *testing.T) {
	recs := chain(t, 5)
	recs = append(recs[:2], recs[3:]...) // some com o seq=3

	var ce *ChainError
	if err := VerifyChain(recs, GenesisHash); !errors.As(err, &ce) {
		t.Fatalf("deleção não detectada: %v", err)
	} else if ce.Kind != "seq_gap" {
		t.Fatalf("esperava seq_gap, veio %s", ce.Kind)
	}
}

func TestVerifyChainDetectaReordenacao(t *testing.T) {
	recs := chain(t, 5)
	recs[1], recs[2] = recs[2], recs[1]
	if err := VerifyChain(recs, GenesisHash); err == nil {
		t.Fatal("reordenação não detectada")
	}
}

func TestVerifyChainDetectaGenesisForjado(t *testing.T) {
	recs := chain(t, 3)
	recs[0].PrevHash = strings.Repeat("a", 64)
	recs[0].Hash = ComputeHash(recs[0])
	if err := VerifyChain(recs, GenesisHash); err == nil {
		t.Fatal("cadeia com genesis forjado foi aceita")
	}
}

func TestVerifyChainAceitaJanelaParcial(t *testing.T) {
	recs := chain(t, 10)
	// Verificação paginada: começa no meio passando o hash do anterior.
	if err := VerifyChain(recs[4:], recs[3].Hash); err != nil {
		t.Fatalf("janela parcial válida recusada: %v", err)
	}
	if err := VerifyChain(recs[4:], GenesisHash); err == nil {
		t.Fatal("janela parcial com âncora errada foi aceita")
	}
}
