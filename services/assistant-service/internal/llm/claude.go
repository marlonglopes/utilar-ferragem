package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/utilar/pkg/httpclient"
)

const messagesURL = "https://api.anthropic.com/v1/messages"

// Claude implementa LLM via a Messages API (raw HTTP — consistente com os
// clients de PSP do Utilar). Ref: platform.claude.com/docs (Messages API).
type Claude struct {
	apiKey    string
	model     string
	maxTokens int
	http      *http.Client
}

// requestTimeout — teto total de uma chamada à Messages API.
const requestTimeout = 60 * time.Second

// NewClaude monta o client com um transport DEDICADO.
//
// pkg/httpclient é calibrado pra chamadas service-to-service (upstreams em
// milissegundos) e traz ResponseHeaderTimeout: 5s. A Messages API sem streaming
// só manda o header de resposta quando a geração termina — com Opus e 1024
// tokens de saída isso passa de 5s rotineiramente, então o transport
// compartilhado abortaria requests perfeitamente saudáveis, e o Timeout de 60s
// do client nunca chegaria a valer.
//
// O pkg é compartilhado por 4 serviços e não pode ser afrouxado por causa deste
// caso; então partimos do transport dele (dial/TLS/pool defensivos, que
// continuam corretos) e sobrescrevemos só o ResponseHeaderTimeout.
func NewClaude(apiKey, model string) *Claude {
	c := httpclient.New(requestTimeout)
	if tr, ok := c.Transport.(*http.Transport); ok {
		t := tr.Clone() // Clone: não mutar um transport que o pkg possa vir a reusar
		t.ResponseHeaderTimeout = requestTimeout
		c.Transport = t
	}
	return &Claude{apiKey: apiKey, model: model, maxTokens: 1024, http: c}
}

func (c *Claude) Name() string { return c.model }

// -- wire types (Messages API) ----------------------------------------------

type wireContent struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type wireMessage struct {
	Role    string        `json:"role"`
	Content []wireContent `json:"content"`
}

type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

func (c *Claude) Complete(ctx context.Context, system string, tools []Tool, msgs []Message) (*Response, error) {
	body := map[string]any{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"system":     system,
		"messages":   toWireMessages(msgs),
	}
	if len(tools) > 0 {
		wt := make([]wireTool, len(tools))
		for i, t := range tools {
			wt[i] = wireTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
		}
		body["tools"] = wt
	}

	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, raw)
	}

	var out struct {
		Content    []wireContent `json:"content"`
		StopReason string        `json:"stop_reason"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	blocks := make([]Block, 0, len(out.Content))
	for _, wc := range out.Content {
		switch wc.Type {
		case "text":
			blocks = append(blocks, Block{Type: "text", Text: wc.Text})
		case "tool_use":
			blocks = append(blocks, Block{Type: "tool_use", ToolUseID: wc.ID, ToolName: wc.Name, ToolInput: wc.Input})
		}
	}
	return &Response{Blocks: blocks, StopReason: out.StopReason}, nil
}

func toWireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: string(m.Role)}
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				wm.Content = append(wm.Content, wireContent{Type: "text", Text: b.Text})
			case "tool_use":
				wm.Content = append(wm.Content, wireContent{Type: "tool_use", ID: b.ToolUseID, Name: b.ToolName, Input: b.ToolInput})
			case "tool_result":
				wm.Content = append(wm.Content, wireContent{Type: "tool_result", ToolUseID: b.ToolUseID, Content: b.Text, IsError: b.IsError})
			}
		}
		out = append(out, wm)
	}
	return out
}

var _ LLM = (*Claude)(nil)
