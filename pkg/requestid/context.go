package requestid

import "context"

// ctxKey é privado: ninguém sobrescreve o request_id por engano com uma string
// key qualquer colidindo com a nossa.
type ctxKey struct{}

// NewContext devolve um ctx carregando o request_id.
//
// O PORQUÊ (observabilidade): o request_id nascia no middleware HTTP e morria
// ali — os clients service-to-service (payment→order, payment→auth,
// order→catalog) abriam uma request NOVA, sem header nenhum. Resultado: um
// pedido que atravessa 3 serviços vira 3 traços desconexos e ninguém consegue
// seguir um checkout ponta a ponta.
//
// Colocando o id no context.Context, o Transport de pkg/httpclient injeta o
// header sozinho em toda chamada feita com NewRequestWithContext — que é o
// padrão de todos os clients do repo. Zero mudança nos call sites.
func NewContext(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext lê o request_id; "" se ausente.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// FromContextOrNew nunca devolve vazio — útil em jobs de background (drainer,
// reconciliação) que não vêm de uma request HTTP mas precisam de correlação.
func FromContextOrNew(ctx context.Context) string {
	if id := FromContext(ctx); id != "" {
		return id
	}
	return New()
}
