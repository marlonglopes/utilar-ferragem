package scrape

import (
	"bufio"
	"strings"
	"time"
)

// Robots é uma leitura mínima porém segura do robots.txt para o NOSSO agente.
// Regra: usa o grupo mais específico que se aplica (o nome do nosso bot; se não
// houver, "*"). Match por prefixo com suporte a `*` (curinga) e `$` (âncora de
// fim). Empate Allow x Disallow: o padrão MAIS LONGO vence (convenção Google).
// Na dúvida, respeita o Disallow — nunca "fail-open" para conteúdo protegido.
type Robots struct {
	disallow   []string
	allow      []string
	CrawlDelay time.Duration
	// unknown=true quando o robots.txt NÃO pôde ser lido por erro de rede (≠ 404).
	// 404 = sem robots = tudo permitido (convenção). Erro = política desconhecida
	// => o caller trata como bloqueado para aquele domínio.
	unknown bool
}

// RobotsUnknown marca um domínio cuja política não pôde ser lida (erro de rede).
func RobotsUnknown() *Robots { return &Robots{unknown: true} }

// IsUnknown indica robots.txt ilegível (≠ inexistente).
func (r *Robots) IsUnknown() bool { return r != nil && r.unknown }

// ParseRobots interpreta o corpo do robots.txt para o UA informado (ex.:
// "UtilarCatalogBot"). Corpo vazio (404) => permissivo.
func ParseRobots(body, ua string) *Robots {
	uaLower := strings.ToLower(ua)
	r := &Robots{}

	// Coletamos as regras dos grupos que se aplicam. Preferimos o grupo do
	// nosso UA; se não existir nenhum específico, usamos o do "*".
	type group struct {
		specific bool // casou o nosso UA pelo nome (não "*")
		applies  bool // este grupo se aplica a nós
		disallow []string
		allow    []string
		delay    time.Duration
	}
	var groups []group
	var cur *group
	appliesSpecific := false

	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, val, ok := splitDirective(line)
		if !ok {
			continue
		}
		switch key {
		case "user-agent":
			agent := strings.ToLower(val)
			// Novo grupo quando muda de contexto (linha user-agent após regras).
			if cur == nil || len(cur.disallow) > 0 || len(cur.allow) > 0 || cur.delay > 0 {
				groups = append(groups, group{})
				cur = &groups[len(groups)-1]
			}
			if agent == "*" || strings.Contains(uaLower, agent) || strings.Contains(agent, uaLower) {
				cur.applies = true
				if agent != "*" {
					cur.specific = true
					appliesSpecific = true
				}
			}
		case "disallow":
			if cur != nil && cur.applies {
				cur.disallow = append(cur.disallow, val)
			}
		case "allow":
			if cur != nil && cur.applies {
				cur.allow = append(cur.allow, val)
			}
		case "crawl-delay":
			if cur != nil && cur.applies {
				if d, err := time.ParseDuration(val + "s"); err == nil {
					cur.delay = d
				}
			}
		}
	}

	// Se há grupo específico do nosso UA, usa só ele(s); senão, os do "*".
	for i := range groups {
		g := &groups[i]
		if !g.applies {
			continue
		}
		if appliesSpecific && !g.specific {
			continue
		}
		r.disallow = append(r.disallow, g.disallow...)
		r.allow = append(r.allow, g.allow...)
		if g.delay > r.CrawlDelay {
			r.CrawlDelay = g.delay
		}
	}
	return r
}

// Allowed diz se o path pode ser rastreado. Empate: o padrão mais longo vence,
// Allow ganha do Disallow de mesmo tamanho. Sem regra que case => permitido.
func (r *Robots) Allowed(path string) bool {
	if r == nil {
		return true
	}
	if r.unknown {
		return false // política desconhecida => não arrisca
	}
	bestDis, bestAllow := -1, -1
	for _, p := range r.disallow {
		if p == "" {
			continue // "Disallow:" vazio = permite tudo, não conta
		}
		if pathMatches(p, path) && len(p) > bestDis {
			bestDis = len(p)
		}
	}
	for _, p := range r.allow {
		if pathMatches(p, path) && len(p) > bestAllow {
			bestAllow = len(p)
		}
	}
	if bestDis < 0 {
		return true
	}
	return bestAllow >= bestDis
}

// pathMatches trata `*` (qualquer sequência) e `$` (fim). Prefixo por padrão.
func pathMatches(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")

	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			// primeiro segmento tem que ser prefixo
			if !strings.HasPrefix(path[pos:], part) {
				return false
			}
			pos += len(part)
			continue
		}
		idx := strings.Index(path[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	if anchored {
		return pos == len(path)
	}
	return true
}

func splitDirective(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(line[:i]))
	val = strings.TrimSpace(line[i+1:])
	return key, val, true
}
