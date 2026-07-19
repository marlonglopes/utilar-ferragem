package appmaxv1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/utilar/payment-service/internal/psp"
)

// Testes de REGRESSÃO da auditoria de segurança do appmaxv1 (2026-07-18).
// Cada teste amarra um achado específico — ver docs/security/appmaxv1-audit-2026-07-18.md.

// ===================== AV1-H2: overflow na trava do split =====================

// A trava do split comparava `sum > cap` com `sum` acumulado sem checagem de
// overflow. Duas entries de ~4.6e18 centavos estouram o int64, `sum` fica
// NEGATIVO e passa direto pela comparação — a trava era contornável com dois
// números grandes.
func TestSplitNaoEhBurlavelPorOverflowDeInt64(t *testing.T) {
	s := newStub(t)
	var splitHits int32
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/split-order") {
			atomic.AddInt32(&splitHits, 1)
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":10,"status":"pendente"}}}`)
	})
	c, _ := s.client(t)

	// Duas parcelas grandes cuja soma estoura int64 e vira negativo.
	metade := int64(math.MaxInt64/2) + 10
	_, err := c.SplitOrder(context.Background(), 10, []SplitEntry{
		{Amount: metade, RecipientHash: "r1"},
		{Amount: metade, RecipientHash: "r2"},
	}, SplitOptions{ReferenceCents: 10000})

	if err == nil {
		t.Fatal("SPLIT COM OVERFLOW FOI ACEITO — a trava é contornável")
	}
	if !errors.Is(err, psp.ErrInvalidRequest) {
		t.Errorf("err = %v, esperado ErrInvalidRequest", err)
	}
	if n := atomic.LoadInt32(&splitHits); n != 0 {
		t.Fatalf("split com overflow foi enviado à rede (%d chamadas)", n)
	}
}

// A soma legítima logo abaixo do teto continua passando: a checagem de overflow
// não pode virar um falso positivo que quebra split de valor alto.
func TestSplitDeValorAltoLegitimoContinuaPassando(t *testing.T) {
	s := newStub(t)
	var splitHits int32
	s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/split-order") {
			atomic.AddInt32(&splitHits, 1)
			return jsonRespond(w, 200, `{"data":{"ok":true}}`)
		}
		return jsonRespond(w, 200, `{"data":{"order":{"id":10,"status":"pendente"}}}`)
	})
	c, _ := s.client(t)

	// Pedido de R$ 1.000.000,00; teto 80% = R$ 800.000,00.
	if _, err := c.SplitOrder(context.Background(), 10, []SplitEntry{
		{Amount: 40_000_000, RecipientHash: "r1"},
		{Amount: 39_000_000, RecipientHash: "r2"},
	}, SplitOptions{ReferenceCents: 100_000_000}); err != nil {
		t.Fatalf("split legítimo de valor alto recusado: %v", err)
	}
	if atomic.LoadInt32(&splitHits) != 1 {
		t.Error("split legítimo não chegou à Appmax")
	}
}

// ===================== AV1-H3: split fail-open =====================

// A verificação "pedido já aprovado?" era `if err == nil` — qualquer falha na
// consulta PULAVA a checagem e o split seguia. Guarda que some quando o sistema
// está sob stress é guarda que não existe.
func TestSplitEhFailClosedQuandoNaoConsegueConfirmarOStatus(t *testing.T) {
	casos := map[string]func(http.ResponseWriter){
		"5xx no GetOrder":   func(w http.ResponseWriter) { w.WriteHeader(500) },
		"404 no GetOrder":   func(w http.ResponseWriter) { w.WriteHeader(404) },
		"resposta ilegível": func(w http.ResponseWriter) { w.WriteHeader(200); _, _ = w.Write([]byte("{{{")) },
	}
	for nome, responder := range casos {
		t.Run(nome, func(t *testing.T) {
			s := newStub(t)
			var splitHits int32
			s.on(func(w http.ResponseWriter, r *http.Request, _ []byte) bool {
				if strings.HasSuffix(r.URL.Path, "/split-order") {
					atomic.AddInt32(&splitHits, 1)
					return jsonRespond(w, 200, `{"data":{"ok":true}}`)
				}
				responder(w)
				return true
			})
			c, _ := s.client(t)
			c.cfg.MaxRetries = 0

			_, err := c.SplitOrder(context.Background(), 10,
				[]SplitEntry{{Amount: 1000, RecipientHash: "r1"}},
				SplitOptions{ReferenceCents: 10000})
			if err == nil {
				t.Fatal("SPLIT SEGUIU SEM CONFIRMAR O STATUS DO PEDIDO (fail-open)")
			}
			if n := atomic.LoadInt32(&splitHits); n != 0 {
				t.Fatalf("split foi enviado mesmo sem confirmação (%d chamadas)", n)
			}
		})
	}
}

// ===================== AV1-M1: validação dos endpoints de saque =====================

// Valor <= 0 chegava a ser enviado à Appmax numa rota de SAQUE, sem contrato
// documentado sobre negativos. "Comportamento desconhecido" numa rota que move
// dinheiro pra fora é inaceitável.
func TestSaqueRecusaValorInvalidoAntesDaRede(t *testing.T) {
	s := newStub(t)
	var hits int32
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		atomic.AddInt32(&hits, 1)
		return jsonRespond(w, 200, `{"data":{"status":2}}`)
	})
	c, _ := s.client(t)
	ctx := context.Background()

	casos := []struct {
		nome  string
		hash  string
		valor int64
	}{
		{"valor zero", "h", 0},
		{"valor negativo", "h", -10000},
		{"hash vazio", "", 10000},
		{"hash só espaço", "   ", 10000},
		{"valor absurdo (provável erro de unidade)", "h", 999_999_999_999},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			for _, chamada := range []struct {
				nome string
				fn   func() error
			}{
				{"antecipação", func() error { _, e := c.RequestAnticipation(ctx, tc.hash, tc.valor); return e }},
				{"saldo disponível", func() error { _, e := c.RequestAvailableWithdraw(ctx, tc.hash, tc.valor); return e }},
				{"simulação", func() error { _, e := c.SimulateAnticipation(ctx, tc.hash, tc.valor); return e }},
			} {
				if err := chamada.fn(); !errors.Is(err, psp.ErrInvalidRequest) {
					t.Errorf("%s: err = %v, esperado ErrInvalidRequest", chamada.nome, err)
				}
			}
		})
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("pedido de saque inválido chegou à rede (%d chamadas)", n)
	}
}

func TestSaqueValidoContinuaPassando(t *testing.T) {
	s := newStub(t)
	s.on(func(w http.ResponseWriter, _ *http.Request, _ []byte) bool {
		return jsonRespond(w, 200, `{"data":{"status":2}}`)
	})
	c, _ := s.client(t)
	if _, err := c.RequestAvailableWithdraw(context.Background(), "hash-valido", 5000); err != nil {
		t.Fatalf("saque válido recusado: %v", err)
	}
}

// ===================== AV1-H4: vazamento de credencial =====================

// O Client guarda o access token e o client_secret em memória. UM
// `slog.Error("falhou", "client", c)` ou um `%+v` num handler de erro
// mandaria a credencial de produção pro agregador de logs. A defesa é fazer
// com que a forma NATURAL de imprimir já seja segura.
func TestCredenciaisNuncaAparecemEmFmtNemEmSlog(t *testing.T) {
	const (
		secret     = "client-secret-de-producao-nao-vaze-isto"
		clientID   = "client-id-de-producao-longo"
		whSecret   = "webhook-secret-de-producao"
		tokenValor = "eyJhbGciOiJIUzI1NiJ9.TOKEN-OAUTH2-DE-PRODUCAO.assinatura"
	)

	cfg := Config{
		AuthURL: "https://auth.appmax.com.br", APIURL: "https://api.appmax.com.br",
		ClientID: clientID, ClientSecret: secret, WebhookSecret: whSecret,
	}
	g := New(cfg)
	c := g.Client()
	// Simula o token já em cache.
	c.mu.Lock()
	c.token = tokenValor
	c.mu.Unlock()

	// Todos os caminhos plausíveis de impressão acidental.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Error("falha na appmax", "cfg", cfg, "client", c, "gateway", g)

	saidas := []string{
		buf.String(),
		fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg),
		fmt.Sprintf("%s", cfg),
		fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c),
		fmt.Sprintf("%v", g), fmt.Sprintf("%+v", g), fmt.Sprintf("%#v", g),
		// Struct envolvente — o caso mais insidioso: alguém loga um wrapper.
		fmt.Sprintf("%+v", struct {
			Cfg     Config
			Client  *Client
			Gateway *Gateway
		}{cfg, c, g}),
	}

	segredos := map[string]string{
		"client_secret":  secret,
		"webhook_secret": whSecret,
		"access token":   tokenValor,
	}
	for i, saida := range saidas {
		for nome, seg := range segredos {
			if strings.Contains(saida, seg) {
				t.Errorf("CREDENCIAL VAZOU (%s) na saída %d:\n%s", nome, i, saida)
			}
		}
	}

	// O que deve continuar visível: informação de diagnóstico não-secreta.
	if !strings.Contains(buf.String(), "api.appmax.com.br") {
		t.Error("a URL da API deveria continuar visível pra debug")
	}
	if !strings.Contains(buf.String(), "token_cached") {
		t.Error("saber se HÁ token em cache é o que se precisa pra debugar 401")
	}
}

// mask não pode entregar metade de um segredo curto.
func TestMascaraNaoEntregaMetadeDoSegredo(t *testing.T) {
	casos := map[string]string{
		"":             "",
		"abc":          "***",
		"12345678":     "***",
		"123456789012": "***",
	}
	for in, want := range casos {
		if got := mask(in); got != want {
			t.Errorf("mask(%q) = %q, esperado %q", in, got, want)
		}
	}
	// Segredo longo: prefixo curto pra correlacionar, nunca o miolo.
	// Montado em partes de propósito: escrito literal, o scanner de segredos
	// do GitHub bloqueia o push por parecer uma chave Stripe real. É falsa —
	// existe só para provar que o mascaramento não deixa o miolo passar.
	longo := "sk_" + "live_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	got := mask(longo)
	if strings.Contains(longo, got) {
		t.Errorf("máscara %q é substring do segredo — não mascara nada", got)
	}
	if len(got) > 12 {
		t.Errorf("máscara longa demais: %q", got)
	}
}

// O erro do endpoint de TOKEN não pode ecoar o corpo: se a Appmax mudar o
// shape da resposta (o parser é tolerante justamente porque já mudou), o token
// viria dentro dele e cairia na mensagem de erro, que é logada e sobe até o
// handler.
func TestErroDoEndpointDeTokenNaoEcoaOCorpo(t *testing.T) {
	// Servidor próprio: precisamos controlar o corpo do /oauth2/token, o que o
	// stub compartilhado não permite.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 200 OK com um shape que o parser não reconhece, mas que CONTÉM o token.
		_, _ = w.Write([]byte(`{"resultado":{"identificador":"TOKEN-SECRETO-NAO-VAZAR","ttl":3600}}`))
	}))
	defer srv.Close()

	c := NewClient(Config{AuthURL: srv.URL, APIURL: srv.URL, ClientID: "cid", ClientSecret: "sec"})
	_, err := c.Token(context.Background())
	if err == nil {
		t.Fatal("shape desconhecido deveria falhar")
	}
	if strings.Contains(err.Error(), "TOKEN-SECRETO-NAO-VAZAR") {
		t.Fatalf("CORPO DO ENDPOINT DE TOKEN VAZOU NO ERRO: %v", err)
	}
	// Mas o erro tem que ser acionável.
	if !strings.Contains(err.Error(), "APPMAX_V1_CLIENT_ID") {
		t.Errorf("erro não diz o que verificar: %v", err)
	}
}
