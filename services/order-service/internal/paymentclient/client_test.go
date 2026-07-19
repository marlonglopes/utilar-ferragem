package paymentclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/utilar/order-service/internal/paymentclient"
	"github.com/utilar/pkg/servicetoken"
)

const segredoServico = "segredo-de-servico-com-mais-de-32-chars!!"

func fato() paymentclient.ExternalSettlement {
	return paymentclient.ExternalSettlement{
		OrderID: "3f8b1d2e-0000-4000-8000-000000000001", AmountBRL: 189.90,
		NSU: "004417", StoreID: "loja-centro", SettledBy: "op-a",
		OccurredAt: time.Now().UTC(),
	}
}

// O lançamento contábil viaja com identidade de SERVIÇO, assinada com o
// SERVICE_JWT_SECRET — nunca com o JWT do usuário que apertou o botão. Ver
// pkg/servicetoken: o poder de emitir identidade de máquina é restrito.
func TestLancamentoViajaComTokenDeServico(t *testing.T) {
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := paymentclient.New(srv.URL, segredoServico).
		PostExternalSettlement(context.Background(), fato()); err != nil {
		t.Fatalf("post: %v", err)
	}
	if path != "/internal/v1/ledger/external-settlement" {
		t.Errorf("path = %q", path)
	}
	tok := strings.TrimPrefix(auth, "Bearer ")
	sub, err := servicetoken.Parse(tok, segredoServico)
	if err != nil {
		t.Fatalf("token enviado não é um token de serviço válido: %v", err)
	}
	if sub != "order-service" {
		t.Errorf("sub = %q, esperado order-service", sub)
	}
}

// 200 é o "já estava lançado" do outro lado (idempotência), 201 é o "lancei
// agora". Os dois são SUCESSO: em ambos o fato está no livro exatamente uma
// vez. Tratar 200 como erro faria o handler registrar falso alarme de
// lançamento perdido em todo retry.
func TestDuplicataDoOutroLadoEhSucesso(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		if err := paymentclient.New(srv.URL, segredoServico).
			PostExternalSettlement(context.Background(), fato()); err != nil {
			t.Errorf("status %d deveria ser sucesso: %v", status, err)
		}
		srv.Close()
	}
}

func TestErrosSaoTipados(t *testing.T) {
	casos := []struct {
		status int
		quer   error
	}{
		{http.StatusConflict, paymentclient.ErrPeriodClosed},
		{http.StatusBadRequest, paymentclient.ErrRejected},
		{http.StatusUnauthorized, paymentclient.ErrRejected},
		{http.StatusInternalServerError, paymentclient.ErrUpstream},
		{http.StatusBadGateway, paymentclient.ErrUpstream},
	}
	for _, tc := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		err := paymentclient.New(srv.URL, segredoServico).
			PostExternalSettlement(context.Background(), fato())
		if !errors.Is(err, tc.quer) {
			t.Errorf("status %d: err = %v, esperado %v", tc.status, err, tc.quer)
		}
		srv.Close()
	}
}

// FAIL-CLOSED explícito: sem segredo de serviço o cliente não tenta e não
// finge que deu certo. O handler precisa saber que o lançamento não saiu para
// gravar o rastro de reprocessamento.
func TestSemSegredoOuURLFalhaExplicito(t *testing.T) {
	if err := paymentclient.New("http://x", "").
		PostExternalSettlement(context.Background(), fato()); !errors.Is(err, paymentclient.ErrNotConfigured) {
		t.Errorf("sem segredo: err = %v", err)
	}
	if err := paymentclient.New("", segredoServico).
		PostExternalSettlement(context.Background(), fato()); !errors.Is(err, paymentclient.ErrNotConfigured) {
		t.Errorf("sem URL: err = %v", err)
	}
}

// O request_id correlaciona a trilha de auditoria do balcão (order-service) com
// o lançamento no livro (payment-service). Sem ele, investigar uma liquidação
// suspeita exige casar as duas pontas por horário.
func TestRequestIDEhPropagado(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Request-Id")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	ctx := paymentclient.WithRequestID(context.Background(), "req-abc")
	if err := paymentclient.New(srv.URL, segredoServico).PostExternalSettlement(ctx, fato()); err != nil {
		t.Fatalf("post: %v", err)
	}
	if got != "req-abc" {
		t.Errorf("X-Request-Id = %q, esperado req-abc", got)
	}
}
