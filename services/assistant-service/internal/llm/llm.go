// Package llm abstrai o modelo por trás da Lara: uma rodada da Messages API
// (system + tools + histórico → blocos de resposta). O orquestrador (pkg lara)
// dirige o loop de tool use; assim o mock e o Claude real compartilham o contrato.
package llm

import (
	"context"
	"encoding/json"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Block é um bloco de conteúdo (text | tool_use | tool_result).
type Block struct {
	Type      string          // "text" | "tool_use" | "tool_result"
	Text      string          // text, ou o resultado (tool_result)
	ToolUseID string          // id do tool_use (e referência no tool_result)
	ToolName  string          // nome da tool (tool_use)
	ToolInput json.RawMessage // input da tool (tool_use)
	IsError   bool            // tool_result de erro
}

type Message struct {
	Role   Role
	Blocks []Block
}

// Tool é uma definição de ferramenta (custom tool da Messages API).
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Response struct {
	Blocks     []Block
	StopReason string
}

// LLM faz UMA rodada da Messages API. Retorna os blocos do assistant.
type LLM interface {
	Complete(ctx context.Context, system string, tools []Tool, msgs []Message) (*Response, error)
	// Name identifica o backend nos logs (ex: "claude-opus-4-8" | "mock").
	Name() string
}

// Helpers de construção de blocos.
func Text(s string) Block { return Block{Type: "text", Text: s} }
func ToolResult(id, s string, e bool) Block {
	return Block{Type: "tool_result", ToolUseID: id, Text: s, IsError: e}
}
