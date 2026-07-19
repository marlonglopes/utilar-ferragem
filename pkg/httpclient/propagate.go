package httpclient

import (
	"net/http"

	"github.com/utilar/pkg/requestid"
)

// propagateTransport injeta X-Request-Id em toda request cujo context carrega
// um (ver requestid.NewContext). É o elo que faltava pra correlação ponta a
// ponta entre os serviços.
//
// Regras:
//   - NÃO sobrescreve um X-Request-Id já setado à mão pelo chamador.
//   - Clona a request antes de mexer no header: um RoundTripper NÃO pode mutar
//     a *http.Request que recebeu (contrato do net/http; mutar quebra retry e
//     é data race quando a mesma request é reusada).
//   - Silencioso quando não há id no context — não inventa um. Um id gerado
//     aqui não estaria em nenhum log a montante e só poluiria a correlação.
type propagateTransport struct {
	base http.RoundTripper
}

func (t *propagateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	id := requestid.FromContext(req.Context())
	if id == "" || req.Header.Get(requestid.HeaderName) != "" {
		return base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header.Set(requestid.HeaderName, id)
	return base.RoundTrip(clone)
}

// WithRequestIDPropagation embrulha um transport existente. Exposto pra quem
// monta o próprio *http.Client (SDK de PSP, por exemplo) e quer o mesmo
// comportamento sem passar por New().
func WithRequestIDPropagation(base http.RoundTripper) http.RoundTripper {
	return &propagateTransport{base: base}
}
