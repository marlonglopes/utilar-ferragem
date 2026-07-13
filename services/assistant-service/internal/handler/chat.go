package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/assistant-service/internal/lara"
	"github.com/utilar/assistant-service/internal/llm"
)

// maxMessage — teto do texto do usuário (anti-abuso / custo).
const maxMessage = 2000

type ChatHandler struct {
	engine *lara.Engine
}

func NewChatHandler(engine *lara.Engine) *ChatHandler {
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

// Chat handles POST /api/v1/assistant/chat
func (h *ChatHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}
	if len(req.Message) > maxMessage {
		req.Message = req.Message[:maxMessage]
	}

	history := make([]llm.Message, 0, len(req.History))
	for _, t := range req.History {
		role := llm.RoleUser
		if t.Role == "assistant" {
			role = llm.RoleAssistant
		}
		if txt := strings.TrimSpace(t.Text); txt != "" {
			history = append(history, llm.Message{Role: role, Blocks: []llm.Block{llm.Text(txt)}})
		}
	}

	res, err := h.engine.Chat(c.Request.Context(), history, req.Message)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "assistant unavailable"})
		return
	}
	c.JSON(http.StatusOK, res)
}
