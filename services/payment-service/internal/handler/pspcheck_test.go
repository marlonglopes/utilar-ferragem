package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// gwFake devolve o erro que o teste mandar, para simular cada resposta do PSP.
type gwFake struct {
	nome string
	err  error
}

func (g gwFake) Name() string { return g.nome }
func (g gwFake) GetPayment(context.Context, string) (*psp.GetResult, error) {
	return nil, g.err
}
func (g gwFake) CreatePayment(context.Context, psp.CreateRequest) (*psp.CreateResult, error) {
	return nil, nil
}
func (g gwFake) VerifyWebhook([]byte, http.Header) error             { return nil }
func (g gwFake) ParseWebhookEvent([]byte) (*psp.WebhookEvent, error) { return nil, nil }

// A regressão que motivou o pacote: a chave do Stripe expirou, o /health
// continuou respondendo "ok" porque só pingava o banco, e a falha só apareceu na
// PRIMEIRA VENDA — 502 na cara do cliente. Ninguém olhando o painel saberia que
// a loja tinha parado de vender.
func TestPSPCheck_ChaveExpiradaMarcaDegradado(t *testing.T) {
	gw := gwFake{nome: "stripe", err: errors.New(
		`psp: upstream error: {"code":"api_key_expired","status":401}`)}

	est := NewPSPCheck(gw, 0).Verify(context.Background())

	if est.OK {
		t.Fatal("credencial expirada passou como válida — o painel continuaria verde " +
			"enquanto toda venda falha")
	}
	if est.Motivo == "" {
		t.Error("sem motivo registrado: quem for investigar não sabe o que corrigir")
	}
}

// O ponto sutil da verificação: "não encontrado" é RESULTADO BOM. O PSP só chega
// a procurar o id depois de autenticar — então ErrNotFound prova que a
// credencial foi aceita. Tratar isso como falha deixaria o serviço eternamente
// degradado com a chave correta.
func TestPSPCheck_NotFoundSignificaCredencialBoa(t *testing.T) {
	est := NewPSPCheck(gwFake{nome: "appmax-v1", err: psp.ErrNotFound}, 0).
		Verify(context.Background())

	if !est.OK {
		t.Fatal("ErrNotFound foi lido como credencial ruim; é o oposto — " +
			"o PSP autenticou e só não achou o id da sonda")
	}
}

// Requisição recusada também prova autenticação: o PSP leu o pedido e respondeu.
func TestPSPCheck_RequisicaoInvalidaTambemProvaAutenticacao(t *testing.T) {
	est := NewPSPCheck(gwFake{nome: "stripe", err: psp.ErrInvalidRequest}, 0).
		Verify(context.Background())
	if !est.OK {
		t.Error("ErrInvalidRequest indica que o PSP respondeu — a credencial serve")
	}
}

// Antes da primeira verificação o estado não pode nascer vermelho: o serviço
// acabou de subir e ainda não perguntou nada. Nascer degradado geraria alarme
// em todo deploy.
func TestPSPCheck_EstadoInicialNaoAlarmaFalso(t *testing.T) {
	est := NewPSPCheck(gwFake{nome: "stripe"}, 0).Estado()
	if !est.OK {
		t.Error("estado inicial nasceu degradado — alarme falso a cada deploy")
	}
}

// O /health precisa dizer "degraded", e NÃO 503: derrubar o serviço tiraria o
// pod do balanceador sem resolver nada, porque o problema é configuração e não
// capacidade. O serviço ainda atende consulta de pagamento e webhook.
func TestPSPCheck_EstadoSerializaParaOPainel(t *testing.T) {
	c := NewPSPCheck(gwFake{nome: "stripe", err: errors.New("401 api_key_expired")}, 0)
	c.Verify(context.Background())

	b, err := json.Marshal(c.Estado())
	if err != nil {
		t.Fatalf("estado não serializa: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, campo := range []string{"ok", "provider", "motivo", "verifiedAt"} {
		if _, existe := m[campo]; !existe {
			t.Errorf("campo %q ausente no payload do /health", campo)
		}
	}
}
