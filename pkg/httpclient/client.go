// Package httpclient fornece um *http.Client com configuração defensiva
// pra clients service-to-service.
//
// Defaults do `http.DefaultTransport` são otimistas:
//   - 100 idle conns globais (pool grande, mas sem cap por host)
//   - 90s idle timeout (segura conn ociosa por muito tempo)
//   - sem dial timeout, sem TLS timeout
//
// Quando um upstream fica lento (MP, order-service, catalog), conexões
// empilham até saturar o file descriptor. Esta config (L-PAYMENT-1):
//   - timeout total agressivo (configurável; default 5s)
//   - timeout de dial 1s (recusa rápido se host está down)
//   - 10 conns por host (cap explícito)
//   - 30s idle timeout (libera conexões mais cedo)
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// DefaultTimeout é o timeout total padrão por request (request + body).
const DefaultTimeout = 5 * time.Second

// New cria um *http.Client com transport defensivo.
// timeout: timeout total da request (zero = DefaultTimeout).
func New(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: defaultTransport(),
	}
}

func defaultTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   1 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       50,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}
