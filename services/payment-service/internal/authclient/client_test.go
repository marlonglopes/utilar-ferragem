package authclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utilar/payment-service/internal/authclient"
)

func TestMe_Success(t *testing.T) {
	cpf := "12345678900"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("auth header missing")
		}
		_ = json.NewEncoder(w).Encode(authclient.User{
			ID: "u-1", Email: "a@b.com", Name: "Joao", CPF: &cpf,
		})
	}))
	defer srv.Close()

	u, err := authclient.New(srv.URL).Me(context.Background(), "t")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Joao" || u.CPF == nil || *u.CPF != cpf {
		t.Errorf("user inesperado: %+v", u)
	}
}

func TestMe_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := authclient.New(srv.URL).Me(context.Background(), "t")
	if !errors.Is(err, authclient.ErrUnauthorized) {
		t.Errorf("esperado ErrUnauthorized, got %v", err)
	}
}

func TestMe_EmptyJWT(t *testing.T) {
	_, err := authclient.New("http://unused").Me(context.Background(), "")
	if !errors.Is(err, authclient.ErrUnauthorized) {
		t.Errorf("esperado ErrUnauthorized, got %v", err)
	}
}
