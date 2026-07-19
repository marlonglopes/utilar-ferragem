package catalogclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/pkg/servicetoken"
)

// A1 (auditoria 2026-07-18) — lado EMISSOR: o order-service assina o token das
// rotas internas do catálogo com o SERVICE_JWT_SECRET. Este teste captura o
// token que sai na rede e verifica com qual segredo ele foi assinado.

const (
	segredoServico = "segredo-de-servico-com-mais-de-32-chars"
	segredoUsuario = "segredo-de-usuario-com-mais-de-32-chars"
)

func TestReserveAssinaComOSegredoDeServico(t *testing.T) {
	var capturado string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturado = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := catalogclient.NewWithSecret(srv.URL, segredoServico)
	err := c.Reserve(context.Background(), "pedido-1",
		[]catalogclient.ReservationItem{{ProductID: "p1", Quantity: 1}}, 0)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if capturado == "" {
		t.Fatal("nenhum Authorization enviado")
	}

	sub, err := servicetoken.Parse(capturado, segredoServico)
	if err != nil {
		t.Fatalf("token emitido não valida com o segredo de serviço: %v", err)
	}
	if sub != "order-service" {
		t.Fatalf("sub = %q", sub)
	}

	// E o ponto do A1: o token NÃO pode ser verificável com o segredo de usuário.
	if _, err := servicetoken.Parse(capturado, segredoUsuario); err == nil {
		t.Fatal("token de serviço valida com o segredo de USUÁRIO — os segredos não estão separados")
	}
}

// Sem o segredo de serviço, a reserva falha com erro explícito em vez de pular
// silenciosamente o controle de estoque.
func TestReserveSemSegredoFalhaExplicito(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("não deveria ter chegado ao catalog-service sem segredo")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	err := catalogclient.New(srv.URL).Reserve(context.Background(), "pedido-1",
		[]catalogclient.ReservationItem{{ProductID: "p1", Quantity: 1}}, time.Minute)
	if err == nil {
		t.Fatal("esperava erro por falta de SERVICE_JWT_SECRET")
	}
	if !strings.Contains(err.Error(), "SERVICE_JWT_SECRET") {
		t.Fatalf("erro deveria citar a variável ausente: %v", err)
	}
}
