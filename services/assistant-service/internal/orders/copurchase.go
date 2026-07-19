// Package orders lê PADRÕES AGREGADOS de co-compra dos pedidos reais da UtiLar,
// para a Alice poder dizer "quem levou X também levou Y".
//
// LGPD — a regra que molda todo este package:
//
// Comportamento de compra é dado pessoal. Saber que "o cliente Fulano comprou
// cimento e depois desempenadeira" é um perfil individual, e a Alice não tem
// nenhuma razão de negócio para vê-lo. O que ela precisa é da ESTATÍSTICA:
// "este par aparece junto em N pedidos". Por isso:
//
//   - Só entra par com pelo menos MinOcorrencias pedidos DISTINTOS. Abaixo
//     disso, o par pode identificar uma pessoa (se só um cliente comprou aquela
//     combinação incomum, a "estatística" é o histórico dele).
//   - Nunca trafega id de cliente, de pedido ou data. O contrato de resposta não
//     tem onde colocar isso, o que é proposital: não dá para vazar um campo que
//     não existe.
//   - A Alice cita o número de pedidos, nunca de quem vieram.
package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/utilar/pkg/httpclient"
)

// MinOcorrenciasPadrao é o piso de k-anonimato para um par virar sugestão.
// 5 pedidos distintos é conservador o bastante para que a sugestão não revele
// a compra de nenhum indivíduo, e baixo o bastante para o sinal existir num
// catálogo de nicho.
const MinOcorrenciasPadrao = 5

// Par é um padrão de co-compra já agregado. Repare no que NÃO tem aqui:
// nenhum identificador de cliente, de pedido ou de data.
type Par struct {
	Slug        string `json:"slug"`
	Nome        string `json:"nome"`
	Ocorrencias int    `json:"ocorrencias"`
}

// Client fala com o order-service.
//
// INTEGRAÇÃO PENDENTE: este cliente consome
//
//	GET /api/v1/internal/copurchase?slug=<slug>&min=<n>&limit=<n>
//
// que ainda NÃO existe no order-service (arquivo de outro agente — não editei).
// O endpoint deve rodar a agregação NO BANCO e devolver só {slug, nome,
// ocorrencias}, filtrando `HAVING COUNT(DISTINCT order_id) >= min`. Fazer a
// agregação aqui exigiria puxar itens de pedido crus para dentro do
// assistant-service, o que é exatamente o que a regra de LGPD acima proíbe.
//
// Enquanto o endpoint não existir, Disponivel() é false e a Alice simplesmente
// não oferece sugestão por co-compra — ela continua oferecendo a técnica, que
// vem da base de conhecimento. Degradar é correto; inventar co-compra não.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	minOcor int
}

func New(baseURL, serviceToken string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   serviceToken,
		http:    httpclient.New(5 * time.Second),
		minOcor: MinOcorrenciasPadrao,
	}
}

// Disponivel informa se dá para consultar co-compra. Sem URL configurada, a
// funcionalidade fica desligada em vez de falhar a cada pergunta.
func (c *Client) Disponivel() bool { return c != nil && c.baseURL != "" }

// CoCompras devolve os pares agregados de um produto.
func (c *Client) CoCompras(ctx context.Context, slug string, limite int) ([]Par, error) {
	if !c.Disponivel() {
		return nil, nil
	}
	if slug == "" {
		return nil, fmt.Errorf("slug vazio")
	}
	if limite <= 0 || limite > 10 {
		limite = 5
	}

	q := url.Values{}
	q.Set("slug", slug)
	q.Set("min", fmt.Sprint(c.minOcor))
	q.Set("limit", fmt.Sprint(limite))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/internal/copurchase?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("order-service copurchase → %d", resp.StatusCode)
	}

	var out struct {
		Data []Par `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	// Defesa em profundidade: mesmo que o endpoint um dia relaxe o filtro, o
	// piso de k-anonimato é reaplicado aqui. Confiar num filtro remoto para
	// proteger dado pessoal é confiar demais.
	filtrado := make([]Par, 0, len(out.Data))
	for _, p := range out.Data {
		if p.Ocorrencias >= c.minOcor && p.Slug != "" {
			filtrado = append(filtrado, p)
		}
	}
	return filtrado, nil
}
