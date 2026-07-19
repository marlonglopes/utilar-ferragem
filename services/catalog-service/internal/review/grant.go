// Package review contém as regras de avaliação que não dependem de HTTP nem de
// banco: verificação do comprovante de compra e triagem de moderação.
//
// Está fora de `handler` de propósito. As duas coisas aqui são as que decidem
// se uma avaliação é legítima, e regra de legitimidade testada só por
// httptest.Recorder é regra testada de longe.
package review

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// COMPRA VERIFICADA — o contrato com o order-service
// ============================================================================
//
// O PROBLEMA: só quem comprou pode avaliar, mas o pedido vive no
// order-service, com banco próprio, e não existe acesso cruzado a banco entre
// serviços (é regra de arquitetura, não limitação técnica). O catalog-service
// não tem como perguntar "este usuário comprou este produto?" olhando os
// próprios dados... quase.
//
// A SOLUÇÃO, em duas provas independentes que a aplicação exige JUNTAS:
//
//	PROVA 1 (esta arquivo) — "purchase grant": um JWT curto que o order-service
//	  emite, a pedido do cliente autenticado, dizendo "o usuário U comprou o
//	  produto P no pedido O". Assinado com o SERVICE_JWT_SECRET, que só os
//	  serviços têm (a Alice/assistant NÃO tem — ver pkg/servicetoken).
//
//	PROVA 2 (handler) — existência local de uma `stock_reservations` com
//	  status='committed' para aquele (order_id, product_id). Esse dado já está
//	  no banco do catálogo porque foi o próprio catálogo que baixou o estoque.
//
// POR QUE AS DUAS. Sozinha, a prova 1 significa "confio inteiramente no
// order-service": um bug de autorização lá dentro (devolver grant para pedido
// de outra pessoa, ou para item que não estava no pedido) vira avaliação falsa
// aqui, sem nenhuma chance de detecção. Sozinha, a prova 2 não amarra o pedido
// a NINGUÉM — o catálogo sabe que o pedido O levou o produto P, mas não sabe de
// quem é O, então qualquer usuário que descobrisse um order_id poderia avaliar
// em nome dele. Cada serviço conhece metade do fato; exigir as duas metades é o
// que fecha a lacuna sem criar acesso cruzado a banco.
//
// POR QUE NÃO UMA CHAMADA HTTP SÍNCRONA ao order-service em cada POST de
// avaliação: colocaria o order-service no caminho crítico de uma escrita do
// cliente (mais uma dependência para cair), exigiria um cliente HTTP com
// timeout/retry, e não seria mais seguro — a resposta dele seria confiada
// exatamente do mesmo jeito que o grant assinado é. O grant tem a vantagem de
// ser verificável offline e de expirar.
//
// ⚠️ O QUE ISTO NÃO RESOLVE: quem comprometer o order-service consegue emitir
// grants — a mesma limitação que pkg/servicetoken documenta para os tokens de
// serviço, e que só a assinatura assimétrica elimina de verdade. A prova 2
// reduz o estrago: mesmo com o segredo, só dá para forjar avaliação de pedidos
// que REALMENTE confirmaram aquele produto.

const (
	// GrantIssuer identifica quem emite o comprovante (claim `iss`).
	// Deliberadamente diferente de servicetoken.Issuer: um token de serviço
	// genérico (que autoriza chamar as rotas internas) NÃO pode ser reaproveitado
	// como comprovante de compra. São autorizações de escopos diferentes e a
	// checagem de `iss` é o que impede a confusão.
	GrantIssuer = "utilar-order"

	// GrantAudience amarra o comprovante a este uso (claim `aud`). Um grant
	// emitido para outra finalidade não passa aqui.
	GrantAudience = "utilar-catalog-reviews"

	// GrantMaxTTL é o teto de validade aceito. O grant é buscado quando o
	// cliente ABRE o formulário de avaliação e usado quando ele envia — 15
	// minutos cobrem escrever um texto com folga. Mais que isso vira credencial
	// de longa duração dentro do navegador, que é o que não queremos.
	GrantMaxTTL = 15 * time.Minute
)

var (
	// ErrGrantInvalid — assinatura, algoritmo, emissor, audiência ou expiração.
	// Erro ÚNICO de propósito: distinguir "assinatura errada" de "expirado" na
	// resposta ao cliente só ajudaria quem está sondando.
	ErrGrantInvalid = errors.New("review: comprovante de compra inválido")

	// ErrGrantTTLTooLong — grant válido, mas com validade acima do teto.
	// Recusado mesmo assim: aceitar significaria deixar o emissor escolher por
	// quanto tempo a credencial vive.
	ErrGrantTTLTooLong = errors.New("review: comprovante com validade acima do permitido")

	// ErrGrantMismatch — o grant é válido, mas é de outro usuário ou de outro
	// produto. É o erro que pega o "peguei o grant do produto A e mandei no
	// endpoint do produto B".
	ErrGrantMismatch = errors.New("review: comprovante não corresponde ao usuário ou produto")
)

// Grant é o conteúdo verificado do comprovante.
type Grant struct {
	UserID    string // claim `sub` — o comprador
	ProductID string // claim `pid`
	OrderID   string // claim `oid`

	// Name é o nome do comprador (claim `nm`), OPCIONAL.
	//
	// Vem do grant e não do corpo do POST por um motivo específico: o nome
	// exibido junto de uma avaliação é atribuição de autoria. Se ele viesse do
	// cliente, qualquer comprador poderia assinar "Utilar Ferragem" ou o nome
	// de um concorrente. O order-service conhece o titular do pedido; é a fonte
	// certa. Quando ausente (order-service antigo), cai para "Cliente" — nunca
	// para um texto escolhido por quem está avaliando.
	Name string
}

// ParseGrant valida assinatura e claims estruturais do comprovante.
//
// NÃO confere se o grant é do usuário/produto certos — isso é Match, chamado
// pelo handler, que é quem sabe qual usuário está autenticado e qual produto a
// rota endereça. Separado porque são erros com respostas HTTP diferentes (401
// vs 403) e porque juntar as duas coisas numa função só produz assinatura com
// cinco parâmetros que ninguém confere na ordem certa.
func ParseGrant(tokenStr, serviceSecret string) (Grant, error) {
	if serviceSecret == "" {
		// Segredo vazio + HS256 = qualquer um assina. Falhar alto é a única
		// opção segura (mesma postura de servicetoken.ErrNoSecret).
		return Grant{}, ErrGrantInvalid
	}

	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		// Lock estrito no algoritmo: sem isto, `alg: none` ou uma troca para
		// RS256 com a chave pública como segredo HMAC viram token válido.
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("algoritmo inesperado")
		}
		return []byte(serviceSecret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(GrantIssuer),
		jwt.WithAudience(GrantAudience),
		// Sem tolerância de relógio: os serviços rodam na mesma infra e a
		// janela de 15 min já é folga suficiente.
		jwt.WithExpirationRequired(),
	)
	if err != nil || !tok.Valid {
		return Grant{}, ErrGrantInvalid
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return Grant{}, ErrGrantInvalid
	}

	g := Grant{
		UserID:    claimString(claims, "sub"),
		ProductID: claimString(claims, "pid"),
		OrderID:   claimString(claims, "oid"),
		Name:      claimString(claims, "nm"),
	}
	if g.UserID == "" || g.ProductID == "" || g.OrderID == "" {
		return Grant{}, ErrGrantInvalid
	}

	// Teto de validade. `exp` já foi verificado pela lib; o que falta é impedir
	// um grant com validade de um ano.
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return Grant{}, ErrGrantInvalid
	}
	iat, err := claims.GetIssuedAt()
	if err != nil || iat == nil {
		// `iat` obrigatório: sem ele não dá para saber a validade CONTRATADA,
		// só a restante — e um grant de um ano recém-emitido passaria.
		return Grant{}, ErrGrantInvalid
	}
	if exp.Sub(iat.Time) > GrantMaxTTL {
		return Grant{}, ErrGrantTTLTooLong
	}

	return g, nil
}

// Match confere que o comprovante é DAQUELE usuário e DAQUELE produto.
func (g Grant) Match(userID, productID string) error {
	if g.UserID != userID || g.ProductID != productID {
		return fmt.Errorf("%w", ErrGrantMismatch)
	}
	return nil
}

func claimString(c jwt.MapClaims, k string) string {
	v, _ := c[k].(string)
	return v
}

// IssueGrantForTest emite um comprovante válido. Existe para os testes deste
// serviço poderem exercitar o caminho feliz sem subir o order-service — e para
// servir de especificação executável do formato ao implementador do outro lado
// (ver docs/reviews-e-recomendacao.md).
//
// ⚠️ Não é usada em nenhum caminho de produção do catalog-service: quem emite
// comprovante é o order-service. Se um dia aparecer chamada a isto fora de
// teste, é bug de segurança — o catálogo estaria atestando compras sozinho.
func IssueGrantForTest(serviceSecret, userID, productID, orderID, name string, ttl time.Duration) (string, error) {
	if serviceSecret == "" {
		return "", ErrGrantInvalid
	}
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": GrantIssuer,
		"aud": GrantAudience,
		"sub": userID,
		"pid": productID,
		"oid": orderID,
		"nm":  name,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	})
	return t.SignedString([]byte(serviceSecret))
}
