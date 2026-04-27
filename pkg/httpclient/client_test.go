package httpclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/utilar/pkg/httpclient"
)

func TestNew_TimeoutFunciona(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := httpclient.New(200 * time.Millisecond)
	start := time.Now()
	_, err := c.Get(srv.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("esperado erro de timeout, got nil")
	}
	if elapsed > 1*time.Second {
		t.Errorf("timeout não respeitado: levou %v (cap=200ms)", elapsed)
	}
}

func TestNew_DefaultTimeoutQuandoZero(t *testing.T) {
	c := httpclient.New(0)
	if c.Timeout != httpclient.DefaultTimeout {
		t.Errorf("Timeout=%v, esperado default %v", c.Timeout, httpclient.DefaultTimeout)
	}
}

func TestNew_TransportCustomizado(t *testing.T) {
	c := httpclient.New(time.Second)
	if c.Transport == http.DefaultTransport {
		t.Error("usando DefaultTransport — esperado custom com pools/timeouts")
	}
}
