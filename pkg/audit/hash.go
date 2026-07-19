package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GenesisHash é o prev_hash do primeiro registro da cadeia. 64 zeros em vez de
// string vazia pra que TODO registro tenha um prev_hash de 64 chars — assim uma
// linha com prev_hash vazio é imediatamente reconhecível como adulteração.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// canonical serializa o registro de forma NÃO-AMBÍGUA pra hashing.
//
// O PORQUÊ do length-prefix: concatenar campos com separador permite forjar
// dois registros diferentes com a mesma serialização ("ab|c" vs "a|bc"). Com
// `len:valor` isso é impossível — a fronteira de cada campo é explícita.
//
// Timestamp em UTC RFC3339Nano: se o servidor mudar de timezone, o hash de um
// registro antigo tem que continuar batendo.
func canonical(r Record) string {
	var b strings.Builder
	write := func(s string) {
		b.WriteString(strconv.Itoa(len(s)))
		b.WriteByte(':')
		b.WriteString(s)
		b.WriteByte('\n')
	}
	write(strconv.FormatInt(r.Seq, 10))
	write(r.PrevHash)
	write(r.OccurredAt.UTC().Truncate(TimePrecision).Format(time.RFC3339Nano))
	write(r.Service)
	write(r.ActorID)
	write(r.ActorRole)
	write(r.ActorIP)
	write(r.ActorUserAgent)
	write(r.EntityType)
	write(r.EntityID)
	write(r.Action)
	write(r.RequestID)
	// Os payloads entram como digest, não inline: mantém o canonical curto e
	// evita que um JSON gigante domine o custo de verificação da cadeia.
	write(digest(r.OldValue))
	write(digest(r.NewValue))
	return b.String()
}

// digest resume um payload JSON pro canonical.
//
// O PORQUÊ DA CANONICALIZAÇÃO: o hash é calculado ANTES do INSERT e recalculado
// DEPOIS do SELECT. O Postgres não devolve JSONB byte-a-byte igual ao que
// recebeu — ele reordena chaves, remove espaços e normaliza números. Hashear os
// bytes crus faria TODO registro parecer adulterado ao ser relido, e a
// verificação da cadeia viraria ruído que ninguém olha (que é pior que não ter
// verificação nenhuma).
//
// A saída é o digest da forma canônica: json.Marshal de map[string]any já
// ordena as chaves, então ida e volta pelo banco produz o mesmo canonical.
func digest(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(CanonicalJSON(b))
	return hex.EncodeToString(sum[:])
}

// CanonicalJSON normaliza JSON: chaves ordenadas, sem espaço supérfluo.
// JSON inválido volta cru — nunca falhamos a gravação da trilha por causa
// disso; o pior caso é o digest ficar sensível a formatação.
func CanonicalJSON(b []byte) []byte {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return b
	}
	out, err := json.Marshal(v)
	if err != nil {
		return b
	}
	return out
}

// TimePrecision é a precisão à qual truncamos OccurredAt antes de hashear.
//
// TIMESTAMPTZ do Postgres guarda MICROSSEGUNDOS. time.Now() no Go tem
// nanossegundos. Sem truncar, o hash calculado na escrita nunca bate com o
// recalculado na leitura — os últimos 3 dígitos somem no banco.
const TimePrecision = time.Microsecond

// ComputeHash devolve o hash do registro (hex, 64 chars).
// Determinístico: mesmo registro → mesmo hash, sempre.
func ComputeHash(r Record) string {
	sum := sha256.Sum256([]byte(canonical(r)))
	return hex.EncodeToString(sum[:])
}

// ChainError descreve exatamente ONDE a cadeia quebrou. Detalhe importa:
// numa auditoria de verdade a pergunta é "qual registro foi mexido", não
// "a cadeia está ok?".
type ChainError struct {
	Index    int    // posição no slice verificado
	Seq      int64  // seq do registro problemático
	Kind     string // "hash_mismatch" | "prev_mismatch" | "seq_gap" | "seq_order"
	Expected string
	Got      string
}

func (e *ChainError) Error() string {
	return fmt.Sprintf("audit: cadeia quebrada no seq=%d (índice %d): %s — esperado %q, encontrado %q",
		e.Seq, e.Index, e.Kind, e.Expected, e.Got)
}

// VerifyChain valida uma sequência CONTÍGUA de registros, em ordem crescente
// de seq. Detecta:
//
//   - adulteração de conteúdo (hash recomputado não bate com o armazenado)
//   - reescrita de elo (prev_hash não aponta pro hash do anterior)
//   - deleção (gap na numeração — por isso seq é atribuído por nós, não por
//     BIGSERIAL: sequence do Postgres tem gap natural em rollback e a gente
//     não conseguiria distinguir gap-legítimo de linha apagada)
//
// `expectedPrev` é o hash do registro imediatamente anterior ao primeiro do
// slice; passe GenesisHash quando o slice começa no início da cadeia.
func VerifyChain(records []Record, expectedPrev string) error {
	if expectedPrev == "" {
		expectedPrev = GenesisHash
	}
	prev := expectedPrev
	var prevSeq int64 = -1

	for i, r := range records {
		if prevSeq >= 0 {
			if r.Seq <= prevSeq {
				return &ChainError{Index: i, Seq: r.Seq, Kind: "seq_order",
					Expected: "> " + strconv.FormatInt(prevSeq, 10),
					Got:      strconv.FormatInt(r.Seq, 10)}
			}
			if r.Seq != prevSeq+1 {
				return &ChainError{Index: i, Seq: r.Seq, Kind: "seq_gap",
					Expected: strconv.FormatInt(prevSeq+1, 10),
					Got:      strconv.FormatInt(r.Seq, 10)}
			}
		}
		if r.PrevHash != prev {
			return &ChainError{Index: i, Seq: r.Seq, Kind: "prev_mismatch",
				Expected: prev, Got: r.PrevHash}
		}
		want := ComputeHash(r)
		if want != r.Hash {
			return &ChainError{Index: i, Seq: r.Seq, Kind: "hash_mismatch",
				Expected: want, Got: r.Hash}
		}
		prev = r.Hash
		prevSeq = r.Seq
	}
	return nil
}
