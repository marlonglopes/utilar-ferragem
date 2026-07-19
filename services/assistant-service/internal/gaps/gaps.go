// Package gaps registra as perguntas que a Alice NÃO conseguiu responder.
//
// Por que isto existe: o pedido original era que a Alice "sempre respondesse a
// contento". Otimizar literalmente para isso produz alucinação — um modelo
// sempre consegue produzir um parágrafo plausível. O objetivo correto é
// responder muito bem o que está fundamentado e ADMITIR CLARAMENTE o que não
// está, oferecendo buscar ou encaminhar.
//
// Admitir sem registrar, porém, é desperdício: a lacuna some. Este registro
// fecha o ciclo — o que a Alice não soube hoje vira a fila do que ingerir
// amanhã. É assim que a cobertura cresce por dado, e não por invenção.
//
// LGPD: só o TEMA agregado é guardado, com contador. Sem id de usuário, sem IP,
// sem texto livre do cliente (que poderia trazer nome, telefone ou endereço).
package gaps

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxTemas limita a memória do registro. Sem teto, um atacante inflaria o mapa
// mandando perguntas aleatórias — o endpoint é público.
const MaxTemas = 500

// maxTamanhoTema corta o tema. Texto longo é onde dado pessoal se esconde.
const maxTamanhoTema = 120

// Lacuna é um tema sem resposta, agregado.
type Lacuna struct {
	Tema        string    `json:"tema"`
	Ocorrencias int       `json:"ocorrencias"`
	PrimeiraEm  time.Time `json:"primeira_em"`
	UltimaEm    time.Time `json:"ultima_em"`
	Motivo      string    `json:"motivo,omitempty"`
}

type Registro struct {
	mu      sync.Mutex
	lacunas map[string]*Lacuna
}

func New() *Registro { return &Registro{lacunas: map[string]*Lacuna{}} }

// Registrar anota um tema sem resposta.
//
// O tema passa por normalização e truncagem ANTES de ser guardado. Se algo que
// pareça dado pessoal escapar (e-mail, telefone, CPF), a entrada é descartada:
// perder um registro de lacuna é irrelevante, guardar dado pessoal não é.
func (r *Registro) Registrar(tema, motivo string) {
	t := normalizar(tema)
	if t == "" || contemDadoPessoal(t) {
		return
	}
	if len(t) > maxTamanhoTema {
		t = t[:maxTamanhoTema]
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if l, ok := r.lacunas[t]; ok {
		l.Ocorrencias++
		l.UltimaEm = time.Now().UTC()
		return
	}
	if len(r.lacunas) >= MaxTemas {
		return // teto atingido: para de crescer em vez de virar vetor de memória
	}
	agora := time.Now().UTC()
	r.lacunas[t] = &Lacuna{
		Tema: t, Ocorrencias: 1, PrimeiraEm: agora, UltimaEm: agora,
		Motivo: normalizar(motivo),
	}
}

// Top devolve as lacunas mais frequentes — a fila de prioridade de ingestão.
func (r *Registro) Top(n int) []Lacuna {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Lacuna, 0, len(r.lacunas))
	for _, l := range r.lacunas {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ocorrencias != out[j].Ocorrencias {
			return out[i].Ocorrencias > out[j].Ocorrencias
		}
		return out[i].Tema < out[j].Tema
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (r *Registro) Total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lacunas)
}

func normalizar(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// contemDadoPessoal é uma heurística conservadora. Prefere descartar demais a
// guardar de menos: o valor de uma lacuna individual é baixo, o custo de reter
// dado pessoal é alto.
func contemDadoPessoal(s string) bool {
	if strings.Contains(s, "@") {
		return true
	}
	digitos := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digitos++
		}
	}
	// 8+ dígitos cobre telefone, CPF e CEP. Medidas de obra ("parede de 10 m²")
	// ficam bem abaixo disso.
	return digitos >= 8
}
