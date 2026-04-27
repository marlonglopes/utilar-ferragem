package orderclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utilar/payment-service/internal/orderclient"
)

func TestGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/orders/abc" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-jwt" {
			t.Errorf("Authorization header missing/wrong")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":            "abc",
			"userId":        "user-123",
			"status":        "pending_payment",
			"total":         199.90,
			"paymentMethod": "pix",
		})
	}))
	defer srv.Close()

	c := orderclient.New(srv.URL)
	order, err := c.Get(context.Background(), "abc", "test-jwt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.ID != "abc" || order.Total != 199.90 || order.UserID != "user-123" {
		t.Errorf("unexpected order: %+v", order)
	}
}

func TestGet_404IsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := orderclient.New(srv.URL)
	_, err := c.Get(context.Background(), "abc", "test-jwt")
	if !errors.Is(err, orderclient.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGet_401IsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := orderclient.New(srv.URL)
	_, err := c.Get(context.Background(), "abc", "test-jwt")
	if !errors.Is(err, orderclient.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestGet_5xxIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := orderclient.New(srv.URL)
	_, err := c.Get(context.Background(), "abc", "test-jwt")
	if !errors.Is(err, orderclient.ErrUpstream) {
		t.Errorf("expected ErrUpstream, got %v", err)
	}
}

func TestGet_EmptyJWTRejected(t *testing.T) {
	c := orderclient.New("http://unused")
	_, err := c.Get(context.Background(), "abc", "")
	if !errors.Is(err, orderclient.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for empty JWT, got %v", err)
	}
}

func TestGet_EmptyOrderIDRejected(t *testing.T) {
	c := orderclient.New("http://unused")
	_, err := c.Get(context.Background(), "", "jwt")
	if !errors.Is(err, orderclient.ErrUpstream) {
		t.Errorf("expected ErrUpstream for empty orderID, got %v", err)
	}
}

func TestGet_PropagatesJWT(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"id": "x", "userId": "u", "status": "pending_payment", "total": 10.0})
	}))
	defer srv.Close()

	c := orderclient.New(srv.URL)
	_, err := c.Get(context.Background(), "x", "specific-jwt-value")
	if err != nil {
		t.Fatal(err)
	}
	if capturedAuth != "Bearer specific-jwt-value" {
		t.Errorf("Authorization not propagated: got %q", capturedAuth)
	}
}
