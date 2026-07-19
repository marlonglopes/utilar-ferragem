package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/assistant-service/internal/alice"
	"github.com/utilar/assistant-service/internal/llm"
)

// Tetos de custo do /chat. Cada request reenvia TODO o histórico pra API da
// Anthropic, e o engine roda até maxTurns (4) iterações de tool use — ou seja,
// o histórico é pago até 4x por request. Sem teto no `History`, o cap de
// `Message` era decorativo: bastava mandar a conversa inteira no campo vizinho.
const (
	// maxMessage — teto do texto do usuário, em RUNES (anti-abuso / custo).
	maxMessage = 2000

	// maxTurnText — teto por turno do histórico, em runes. Mais apertado que
	// maxMessage porque turnos antigos não precisam voltar inteiros.
	maxTurnText = 2000

	// maxHistoryTurns — quantos turnos do histórico chegam ao modelo.
	maxHistoryTurns = 20

	// maxHistoryBytes — teto agregado do histórico. Segura o caso "20 turnos de
	// 2000 runes cada", que sozinho já seria ~40k runes por chamada ao modelo.
	maxHistoryBytes = 16000
)

type ChatHandler struct {
	engine *alice.Engine
}

func NewChatHandler(engine *alice.Engine) *ChatHandler {
	return &ChatHandler{engine: engine}
}

type turn struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

type chatRequest struct {
	Message string `json:"message"`
	History []turn `json:"history"`
}

// truncateRunes corta a string mantendo os primeiros N runes (não bytes).
// Fatiar UTF-8 por byte (s[:n]) pode partir um caractere multibyte no meio e
// gerar texto inválido — mesma correção já aplicada no catalog-service.
func truncateRunes(s string, n int) string {
	if len(s) <= n { // len em bytes >= nº de runes: se cabe em bytes, cabe em runes
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// clampHistory aplica os tetos de turnos e de bytes agregados.
//
// Decisão: TRUNCAR mantendo os turnos MAIS RECENTES, em vez de rejeitar com
// erro. Conversa longa e legítima é o caso normal — o SPA acumula o histórico e
// não tem como saber onde fica o corte do servidor; devolver 400 no meio de uma
// conversa boa quebraria o produto por uma razão que é nossa, não do usuário.
// Janela deslizante é o comportamento padrão de chat e degrada suave: a Alice
// perde memória antiga, não a conversa.
//
// Nota de segurança: o cliente manda o histórico inteiro, inclusive turnos com
// role "assistant" — dá pra forjar falas da Alice e tentar sequestrar o escopo
// dela. Não dá pra impedir por completo sem sessão server-side (fora do escopo
// desta correção), mas o dano fica limitado: o systemPrompt é const no servidor
// e sempre reenviado (o cliente não o alcança), todo fato vem de tool use
// contra o catalog-service, e estes tetos limitam quanto texto injetado cabe.
func clampHistory(in []turn) []llm.Message {
	// Normaliza primeiro (trim + truncate por runes), descartando turnos vazios.
	// Roles fora de {user, assistant} viram "user" — nunca confiar no campo cru.
	norm := make([]turn, 0, len(in))
	for _, t := range in {
		txt := truncateRunes(strings.TrimSpace(t.Text), maxTurnText)
		if txt == "" {
			continue
		}
		role := "user"
		if t.Role == "assistant" {
			role = "assistant"
		}
		norm = append(norm, turn{Role: role, Text: txt})
	}

	// Do mais recente pro mais antigo, parando no primeiro teto atingido.
	start := len(norm)
	total := 0
	for i := len(norm) - 1; i >= 0; i-- {
		if len(norm)-i > maxHistoryTurns {
			break
		}
		if total+len(norm[i].Text) > maxHistoryBytes {
			break
		}
		total += len(norm[i].Text)
		start = i
	}

	out := make([]llm.Message, 0, len(norm)-start)
	for _, t := range norm[start:] {
		role := llm.RoleUser
		if t.Role == "assistant" {
			role = llm.RoleAssistant
		}
		out = append(out, llm.Message{Role: role, Blocks: []llm.Block{llm.Text(t.Text)}})
	}
	return out
}

// Chat handles POST /api/v1/assistant/chat
func (h *ChatHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Body acima de MaxRequestBytes cai aqui: http.MaxBytesReader corta a
		// leitura antes do parse, então nem chegamos a alocar o JSON gigante.
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}
	req.Message = truncateRunes(req.Message, maxMessage)

	history := clampHistory(req.History)

	// MODO: derivado EXCLUSIVAMENTE das claims validadas pelo OptionalAuth.
	//
	// Repare que `chatRequest` não tem campo `mode`, e isso é deliberado: se o
	// modo viesse do corpo, qualquer visitante do site público mandaria
	// {"mode":"vendedor"} e leria o custo e a margem da loja — o dado mais
	// sensível do negócio. O corpo da requisição é entrada do usuário; papel é
	// afirmação do servidor sobre quem ele é.
	mode := alice.ModeFromClaims(c.GetString("user_id") != "", c.GetString("user_role"))

	res, err := h.engine.Chat(c.Request.Context(), mode, history, req.Message)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "assistant unavailable"})
		return
	}
	c.JSON(http.StatusOK, res)
}
