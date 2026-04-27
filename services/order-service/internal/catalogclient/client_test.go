package catalogclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utilar/order-service/internal/catalogclient"
)

func TestGetByID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/products/by-id/p-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "p-123",
			"name":  "Furadeira Bosch",
			"price": 299.90,
			"stock": 12,
		})
	}))
	defer srv.Close()

	c := catalogclient.New(srv.URL)
	p, err := c.GetByID(context.Background(), "p-123")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if p.ID != "p-123" || p.Price != 299.90 || p.Stock != 12 {
		t.Errorf("produto inesperado: %+v", p)
	}
}

func TestGetByID_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := catalogclient.New(srv.URL)
	_, err := c.GetByID(context.Background(), "p-x")
	if !errors.Is(err, catalogclient.ErrNotFound) {
		t.Errorf("esperado ErrNotFound, got %v", err)
	}
}

func TestGetByID_5xxIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := catalogclient.New(srv.URL)
	_, err := c.GetByID(context.Background(), "p-x")
	if !errors.Is(err, catalogclient.ErrUpstream) {
		t.Errorf("esperado ErrUpstream, got %v", err)
	}
}

func TestGetByID_EmptyIDRejected(t *testing.T) {
	c := catalogclient.New("http://unused")
	_, err := c.GetByID(context.Background(), "")
	if !errors.Is(err, catalogclient.ErrUpstream) {
		t.Errorf("esperado ErrUpstream para id vazio, got %v", err)
	}
}
