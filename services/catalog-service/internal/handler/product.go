package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/utilar/catalog-service/internal/model"
	"github.com/utilar/catalog-service/internal/storage"
)

// slugLookupMinElapsed é o tempo mínimo de resposta de GetBySlug e Related.
// Mitiga timing attack que distinguiria slug existente vs inexistente
// (CT1-H4). Valor pequeno o suficiente pra não impactar UX humana.
const slugLookupMinElapsed = 50 * time.Millisecond

// padToMinElapsed bloqueia até que pelo menos `min` tenha decorrido desde
// `start`. Usado pra normalizar tempo de respostas sensíveis a timing.
func padToMinElapsed(start time.Time, min time.Duration) {
	if elapsed := time.Since(start); elapsed < min {
		time.Sleep(min - elapsed)
	}
}

type ProductHandler struct {
	db *sql.DB
	// media traduz a CHAVE lógica gravada em product_images.variants na URL
	// pública. É o único ponto de leitura que sabe onde a mídia mora — por isso
	// migrar disco→S3/CDN é trocar o resolver, não reescrever a tabela.
	media storage.URLResolver
}

func NewProductHandler(db *sql.DB) *ProductHandler {
	// Default "/media": é onde o driver local publica. Mantém a assinatura de
	// antes funcionando (vários testes chamam assim) sem precisar de storage.
	return &ProductHandler{db: db, media: storage.PrefixResolver("/media")}
}

// WithMedia troca o resolver de URL de mídia. O main injeta o do storage ativo.
func (h *ProductHandler) WithMedia(r storage.URLResolver) *ProductHandler {
	if r != nil {
		h.media = r
	}
	return h
}

// imageVariants converte o JSONB de variantes (chaves lógicas) no bloco de URLs
// públicas do payload.
//
// Devolve nil para imagem EXTERNA (as 288 fotos CC0 do Wikimedia, sem
// variantes) — e é essa ausência que o frontend usa pra distinguir os dois
// tipos: com `variants`, escolhe o tamanho pelo contexto; sem, usa `url`.
func (h *ProductHandler) imageVariants(raw []byte) *model.ImageVariants {
	if len(raw) == 0 {
		return nil
	}
	var keys map[string]string
	if err := json.Unmarshal(raw, &keys); err != nil || len(keys) == 0 {
		return nil
	}
	return &model.ImageVariants{
		Thumb:  h.media.URL(keys["thumb"]),
		Medium: h.media.URL(keys["medium"]),
		Large:  h.media.URL(keys["large"]),
	}
}

// productColumns é a projeção PÚBLICA de produto — a única usada pelas rotas
// abertas.
//
// ⚠️ `p.cost` NÃO ESTÁ AQUI E NÃO PODE ENTRAR. Custo é informação sensível de
// negócio (quem vê o custo sabe até onde a loja pode baixar o preço) e só sai
// pelas rotas de admin, que têm projeção própria em admin_product.go.
// O teste TestPublicAPI_NuncaVazaCusto cobre isso.
//
// Constante única em vez de a mesma lista repetida em quatro queries: era o que
// já acontecia, e uma coluna nova exigia lembrar de editar os quatro lugares
// (com `scanProduct` quebrando silenciosamente na ordem errada).
const productColumns = `
	  p.id, p.slug, p.name, p.category_id, p.price, p.original_price, p.currency, p.icon, p.brand,
	  s.name, s.id, s.rating, s.review_count,
	  p.stock, p.rating, p.review_count, p.cashback_amount, p.badge::text, p.badge_label, p.installments,
	  p.description, p.specs, p.created_at, p.updated_at,
	  p.sku, p.barcode, p.unit_of_measure, p.qty_step,
	  p.weight_kg, p.length_cm, p.width_cm, p.height_cm,
	  p.rating_bayes`

// List GET /api/v1/products
// Query params: category, q, brand, price_min, price_max, in_stock, sort, page, per_page
func (h *ProductHandler) List(c *gin.Context) {
	params := parseProductsQuery(c)

	// CASCATA DE BUSCA — a ordem é a feature, não um detalhe.
	//
	//   1. tsvector exato   (lema + acento + plural, com relevância por peso)
	//   2. prefixo `:*`     (autocomplete, no MESMO passo — é um OR na tsquery)
	//   3. similaridade     (erro de grafia) ← SÓ se 1 e 2 vierem vazios
	//
	// O gatilho do passo 3 é "não achei NADA", nunca "sempre". Rodar
	// similaridade junto com a busca boa traria lixo aproximado misturado com
	// resultado exato e destruiria a busca que já funciona — falso positivo
	// aqui é pior que o bug original.
	var q dbQueryer = h.db

	f := buildProductFilters(params, searchFullText, scopeList)
	total, err := h.countProducts(c, q, f)
	if err != nil {
		return
	}

	approximate := false
	if total == 0 && params.Q != "" {
		// A busca aproximada roda numa TRANSAÇÃO porque precisa de
		// `SET LOCAL pg_trgm.strict_word_similarity_threshold` (ver
		// fuzzyThreshold). `SET LOCAL` é desfeito no COMMIT/ROLLBACK — um
		// `SET` normal contaminaria a conexão do POOL e mudaria o limiar de
		// todas as buscas seguintes que pegassem aquela conexão. Esse tipo de
		// vazamento de estado por pool é bug intermitente e irreproduzível.
		tx, err := h.db.Begin()
		if err != nil {
			DBError(c, err)
			return
		}
		defer func() { _ = tx.Rollback() }() // só leitura: rollback sempre

		if _, err := tx.Exec(
			"SET LOCAL pg_trgm.strict_word_similarity_threshold = " + fuzzyThreshold); err != nil {
			DBError(c, err)
			return
		}

		fz := buildProductFilters(params, searchFuzzy, scopeList)
		fzTotal, err := h.countProducts(c, tx, fz)
		if err != nil {
			return
		}
		// Só troca se a aproximada achou algo. Se ela também vier vazia,
		// devolvemos a busca exata vazia — sem marcar como aproximado, porque
		// não houve aproximação nenhuma.
		if fzTotal > 0 {
			f, total, approximate, q = fz, fzTotal, true, tx
		}
	}

	orderBy := productOrderBy(params, f)
	whereSQL := strings.Join(f.where, " AND ")

	// Page
	args := append(f.args, params.PerPage, (params.Page-1)*params.PerPage)

	// O JOIN com sellers continua para PROJETAR s.name/s.rating no payload — mas
	// ele saiu do WHERE. Era `s.name ILIKE ...` dentro do OR que impedia
	// qualquer índice em `products` de ser usado; agora o nome do vendedor mora
	// no search_vector (peso D) e o predicado é de uma tabela só.
	//
	// #nosec G202 — whereSQL/orderBy são construídos só de literais hardcoded
	// (`p.brand = $N`, `p.price ASC`, etc.) com placeholders posicionais.
	// orderBy passa por whitelist em parseProductsQuery (CT1-M3); valores
	// entram via `args`. Atacante não controla os fragmentos de SQL.
	querySQL := `
		SELECT ` + productColumns + `
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE ` + whereSQL + `
		ORDER BY ` + orderBy + `
		LIMIT $` + strconv.Itoa(f.idx) + ` OFFSET $` + strconv.Itoa(f.idx+1)

	rows, err := q.Query(querySQL, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	products := make([]model.Product, 0)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			DBError(c, err)
			return
		}
		products = append(products, p)
	}
	// rows.Err() distingue "acabaram os produtos" de "a varredura morreu no
	// meio" — sem isso a vitrine mostraria uma página truncada como completa.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	// Capa de cada produto, em uma query só (ver loadThumbnails).
	h.loadThumbnails(c, products)

	meta := model.Meta{
		Page: params.Page, PerPage: params.PerPage,
		Total: total, TotalPages: (total + params.PerPage - 1) / params.PerPage,
	}
	if approximate {
		meta.Approximate = true
		// Sugestão calculada em Go, sobre os nomes que ACABARAM de vir — zero
		// consulta a mais. Ver suggestFromNames.
		meta.Suggestion = suggestFromNames(params.Q, products)
	}

	c.JSON(http.StatusOK, model.ProductsResponse{Data: products, Meta: meta})
}

// dbQueryer é o mínimo que List precisa, satisfeito tanto por *sql.DB quanto
// por *sql.Tx. Existe porque a busca aproximada roda em transação (SET LOCAL do
// limiar) e a exata não — sem a interface seria o corpo de List duplicado.
type dbQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// countProducts roda o COUNT do filtro. Em erro já responde 500 e devolve err —
// quem chama só precisa dar `return`.
//
// Note que NÃO há JOIN com sellers: o predicado de busca virou uma coluna de
// `products` (search_vector), e era exatamente o `s.name` dentro do OR que
// obrigava o JOIN e cegava o planejador.
func (h *ProductHandler) countProducts(c *gin.Context, q dbQueryer, f productFilters) (int, error) {
	var total int
	// #nosec G202 — ver comentário em List: fragmentos hardcoded, valores em args.
	countSQL := "SELECT count(*) FROM products p WHERE " + strings.Join(f.where, " AND ")
	if err := q.QueryRow(countSQL, f.args...).Scan(&total); err != nil {
		DBError(c, err)
		return 0, err
	}
	return total, nil
}

// productOrderBy resolve a ordenação. É whitelist — valor não reconhecido cai
// no default (audit CT1-M3).
//
// MUDANÇA DE COMPORTAMENTO: com termo de busca e sem `sort` explícito, o
// default passa a ser RELEVÂNCIA, não data de cadastro. Ordenar resultado de
// busca por data de cadastro é o mesmo que não ordenar. `sort=newest` continua
// entregando data — quem pede explicitamente recebe o que pediu.
func productOrderBy(params productsQuery, f productFilters) string {
	switch params.Sort {
	case "price_asc":
		return "p.price ASC"
	case "price_desc":
		return "p.price DESC"
	case "top_rated":
		// ⚠️ ORDENA PELA MÉDIA BAYESIANA, não pela média pura (migration 015).
		//
		// `p.rating DESC` colocava no topo o produto com UMA avaliação 5★, à
		// frente de um 4,8★ com 400 — ou seja, "mais bem avaliado" ordenava, na
		// prática, por "menos avaliado". `rating_bayes` puxa a nota de quem tem
		// pouca amostra na direção de uma nota neutra, então nota alta COM
		// volume vence nota alta de uma pessoa só.
		//
		// `p.id ASC` no fim dá ordem TOTAL: sem ele, produtos empatados (e no
		// começo da loja todos empatam em 0) podem trocar de posição entre a
		// página 1 e a 2, e o usuário vê o mesmo item duas vezes.
		return "p.rating_bayes DESC, p.review_count DESC, p.id ASC"
	case "newest":
		return "p.created_at DESC"
	}
	if f.rankExpr == "" {
		return "p.created_at DESC"
	}
	// Desempate ESTÁVEL: sem `p.id` no fim, dois produtos com o mesmo rank e o
	// mesmo created_at podem trocar de lugar entre a página 1 e a 2, e o
	// usuário vê o mesmo item duas vezes (ou nenhuma). Paginação sem ordem
	// total é bug de resultado, não de performance.
	return f.rankExpr + " DESC, p.created_at DESC, p.id ASC"
}

// -- construção do filtro de busca -------------------------------------------

// searchStrategy escolhe COMO o termo `q` vira predicado.
type searchStrategy int

const (
	// searchFullText: tsvector + GIN. O caminho normal.
	searchFullText searchStrategy = iota
	// searchFuzzy: similaridade trigram no nome. Só quando o full-text não
	// achou nada — cobre erro de digitação.
	searchFuzzy
)

// tsConfig é a configuração de busca criada na migration 014: `portuguese`
// com `unaccent` no dicionário. É por causa dela que "eletrica" acha
// "elétrica" E que a coluna gerada pôde ser IMMUTABLE (ver o cabeçalho da
// migration — `unaccent()` chamado direto na expressão seria rejeitado).
const tsConfig = "utilar_pt"

// productFilters é o WHERE montado, mais o que o ORDER BY precisa saber.
type productFilters struct {
	where []string
	args  []any
	idx   int // próximo placeholder livre
	// rankExpr é o fragmento SQL de relevância (vazio quando não há busca).
	rankExpr string
}

// filterScope diz QUAIS filtros entram no WHERE.
type filterScope int

const (
	// scopeList: todos os filtros. É a listagem que o usuário vê.
	scopeList filterScope = iota
	// scopeFacets: só categoria, busca e atributos.
	//
	// ⚠️ NÃO ACRESCENTE brand/price/in_stock AQUI. Faceta descreve o universo
	// ANTES do refinamento: se a contagem de marcas já viesse filtrada pela
	// marca selecionada, a lista de marcas colapsaria pra uma só e o usuário
	// não teria como trocar de marca sem limpar o filtro. Mesma coisa pro
	// slider de preço, que viraria a faixa que ele já escolheu.
	scopeFacets
)

// buildProductFilters monta o WHERE compartilhado por List, Facets e pelo
// COUNT. Ter um builder só é o que garante que a contagem e a página falam da
// MESMA busca — quando eram dois trechos de código, divergiam (o predicado de
// Facets não olhava o vendedor e o de List olhava).
func buildProductFilters(params productsQuery, strategy searchStrategy, scope filterScope) productFilters {
	f := productFilters{where: []string{"p.status = 'published'"}, args: []any{}, idx: 1}

	if params.Category != "" {
		f.where = append(f.where, fmt.Sprintf("p.category_id = $%d", f.idx))
		f.args = append(f.args, params.Category)
		f.idx++
	}
	if scope == scopeList {
		// Lookup exato de scanner/balcão. Vem ANTES do `q` porque é a leitura
		// mais quente do PDV e precisa cair direto no índice único, sem ILIKE e
		// sem tsvector. NÃO REGREDIR: é o caminho do leitor de código de barras.
		if params.SKU != "" {
			f.where = append(f.where, fmt.Sprintf("p.sku = $%d", f.idx))
			f.args = append(f.args, params.SKU)
			f.idx++
		}
		if params.Barcode != "" {
			f.where = append(f.where, fmt.Sprintf("p.barcode = $%d", f.idx))
			f.args = append(f.args, params.Barcode)
			f.idx++
		}
	}
	if params.Q != "" {
		f.appendSearch(params.Q, strategy)
	}
	if scope == scopeList {
		if params.Brand != "" {
			f.where = append(f.where, fmt.Sprintf("p.brand = $%d", f.idx))
			f.args = append(f.args, params.Brand)
			f.idx++
		}
		if params.PriceMin != nil {
			f.where = append(f.where, fmt.Sprintf("p.price >= $%d", f.idx))
			f.args = append(f.args, *params.PriceMin)
			f.idx++
		}
		if params.PriceMax != nil {
			f.where = append(f.where, fmt.Sprintf("p.price <= $%d", f.idx))
			f.args = append(f.args, *params.PriceMax)
			f.idx++
		}
		if params.InStock {
			f.where = append(f.where, "p.stock > 0")
		}
	}

	// Facetas técnicas: cada filtro de atributo vira um EXISTS independente,
	// que é o que dá semântica de AND entre grandezas diferentes ("bitola 2,5
	// E cor azul"). Um JOIN único não conseguiria — uma linha de
	// product_attributes tem uma chave só.
	f.where, f.args, f.idx = appendAttrFilters(f.where, f.args, f.idx, params)
	return f
}

// appendSearch acrescenta o predicado do termo de busca.
//
// Formato do ramo full-text:
//
//	p.search_vector @@ (websearch_to_tsquery(cfg,$a) || to_tsquery(cfg,$b))
//	  OR p.sku ILIKE $c ESCAPE '\'
//	  OR p.barcode = $d
//
// Todas as colunas são de `products`. É essa a correção: o antigo `s.name`
// dentro do OR vinha da tabela do JOIN e impedia o planejador de usar
// QUALQUER índice em products. O vendedor agora está no search_vector (peso D).
//
// ⚠️ `p.sku ILIKE`, NUNCA `COALESCE(p.sku,”) ILIKE`. Isto foi MEDIDO em 150k:
// o COALESCE (que estava no código antigo) é uma EXPRESSÃO, não uma coluna, e
// nenhum índice a serve. Um só ramo não-indexável derruba o OR inteiro —
// mesmo com o GIN pronto o plano voltava pra Parallel Seq Scan:
//
//	com COALESCE:  Parallel Seq Scan   124,9 ms / 12.132 buffers
//	sem COALESCE:  BitmapOr (3 índices)  5,2 ms /    360 buffers
//
// A semântica é idêntica aqui: `NULL ILIKE 'x%'` é NULL, e num OR do WHERE
// NULL e FALSE dão no mesmo (a linha não entra). A única divergência seria com
// padrão vazio, e `q` nunca é vazio neste ramo (guardado por `params.Q != ""`).
func (f *productFilters) appendSearch(q string, strategy searchStrategy) {
	esc := escapeLikePattern(q)

	if strategy == searchFuzzy {
		// `%>>` é o operador de STRICT WORD SIMILARITY do pg_trgm — não é
		// wildcard de LIKE, não tem metacaractere pra escapar e o custo é
		// linear no tamanho da string. Não há superfície de ReDoS aqui.
		// Servido pelo GIN trigram que já existia e nunca era usado
		// (idx_products_name_trgm) — a infraestrutura estava pronta, só o `OR`
		// cruzado impedia.
		//
		// ⚠️ `similarity()` (o `%` comum) NÃO SERVE aqui, e isso foi MEDIDO:
		// ele divide pelos trigramas do nome INTEIRO, então uma palavra errada
		// contra um nome longo afunda:
		//
		//   similarity('furadera', 'Furadeira de Impacto 1/2" 750W')  = 0,219
		//   similarity('argamasa', 'Argamassa Colante AC-I ... 20kg') = 0,205
		//   similarity('cimeto',   'Cimento Portland CP II-32 ...')   = 0,135
		//
		// Todos ABAIXO do corte default de 0,3 — ou seja, o `%` não corrigiria
		// NENHUM erro de grafia real. strict_word_similarity compara com a
		// melhor extensão de PALAVRAS do alvo e resolve os três (0,583 / 0,727
		// / 0,500).
		//
		// ⚠️ SKU continua ILIKE por PREFIXO e o CÓDIGO DE BARRAS FICA DE FORA
		// da busca aproximada, de propósito: bipar um item e receber "o mais
		// parecido" venderia o produto errado, com o preço errado. Leitor de
		// código exige correspondência exata ou nada.
		f.where = append(f.where, fmt.Sprintf(
			"(p.name %%>> $%d OR p.sku ILIKE $%d ESCAPE '\\')", f.idx, f.idx+1))
		f.args = append(f.args, q, esc+"%")
		f.rankExpr = fmt.Sprintf("strict_word_similarity($%d, p.name)", f.idx)
		f.idx += 2
		return
	}

	web, prefix := buildTsqueryInputs(q)
	tsq := fmt.Sprintf("(websearch_to_tsquery('%s', $%d) || to_tsquery('%s', $%d))",
		tsConfig, f.idx, tsConfig, f.idx+1)

	// SKU continua na busca geral por PREFIXO (`ABC%`), não por `%ABC%`: código
	// de produto é lido/digitado da esquerda pra direita, e o infixo daria
	// resultado lixo ("12" casando com metade do catálogo) além de ser mais
	// caro. O SKU também está no vetor (peso B), mas o vetor não faz prefixo
	// de token com hífen — é o ILIKE que atende o vendedor digitando o começo
	// do código no balcão.
	f.where = append(f.where, fmt.Sprintf(
		"(p.search_vector @@ %s OR p.sku ILIKE $%d ESCAPE '\\' OR p.barcode = $%d)",
		tsq, f.idx+2, f.idx+3))
	f.args = append(f.args, web, prefix, esc+"%", q)

	// Match de CÓDIGO ganha de qualquer relevância textual: quem digitou o SKU
	// no balcão quer aquele produto, não o mais bem ranqueado que menciona o
	// código na descrição. Sem esse degrau o item exato caía no fim da lista
	// com rank 0 (não está no vetor por prefixo).
	//
	// ⚠️ `ts_rank`, NÃO `ts_rank_cd` — decisão MEDIDA em 150k produtos:
	//
	//	termo                       ts_rank_cd   ts_rank
	//	"argamassa colante" (294)      32,8 ms    4,5 ms
	//	"acrilico"       (24.458)     379,6 ms   29,2 ms   ← 13x
	//
	// O que o `_cd` acrescenta é COVER DENSITY: o quão PRÓXIMOS os termos da
	// busca aparecem dentro do documento. Isso vale em texto corrido longo.
	// Nome de produto tem 3 a 8 palavras — os termos estão sempre próximos, o
	// sinal é praticamente constante, e estaríamos pagando 13x por ele.
	//
	// A ordenação por PESO (que é a relevância que importa aqui) é idêntica
	// nos dois. Verificado com o caso que o dono pediu — termo no nome tem que
	// ganhar de termo só na descrição:
	//
	//	"Massa Acrílica Premium 18L" (termo no NOME, peso A)   ts_rank 0,608
	//	"Rolo de Espuma" (termo só na DESCRIÇÃO, peso C)       ts_rank 0,122
	//
	// Mesma proporção 5:1 do ts_rank_cd. E note que o peso C NÃO zera: quem
	// só menciona o termo na descrição continua aparecendo, depois — que é
	// exatamente o comportamento pedido.
	f.rankExpr = fmt.Sprintf(
		"(CASE WHEN p.sku ILIKE $%d ESCAPE '\\' OR p.barcode = $%d THEN 1 ELSE 0 END) DESC, ts_rank(p.search_vector, %s)",
		f.idx+2, f.idx+3, tsq)
	f.idx += 4
}

// -- saneamento do termo de busca (audit CT1-C1, estendido pro tsquery) -------

// maxSearchWords limita quantos termos entram na tsquery de prefixo.
//
// PORQUÊ: cada termo é um ramo `&` a mais no plano. `q` já vem truncado em 100
// runes por parseProductsQuery (CT1-M1), então isto é a segunda camada — mas é
// a que impede uma string de 100 caracteres de espaço-separados virar uma
// tsquery de 50 ramos com prefixo, que é cara mesmo com índice.
const maxSearchWords = 8

// buildTsqueryInputs traduz o termo do usuário nas DUAS entradas de tsquery.
//
// ⚠️ SEGURANÇA — a superfície do CT1-C1 mudou de lugar, NÃO sumiu.
//
// Com ILIKE o risco era wildcard injection (`%_%_%_%`) forçando ReDoS no
// pg_trgm; por isso existe (e continua existindo) escapeLikePattern — o ramo do
// SKU ainda é ILIKE. Com tsquery o risco é outro:
//
//   - `to_tsquery` LANÇA EXCEÇÃO com sintaxe inválida (`&&&`, `((`, `!`), e
//     exceção no banco vira **500 na busca**: derrubar a vitrine com uma query
//     string é DoS de graça.
//   - `websearch_to_tsquery` é TOLERANTE — nunca lança, devolve tsquery vazia.
//     Foi a razão principal de escolhê-lo pro texto livre do usuário.
//
// Daí a divisão:
//
//	web    — o termo CRU vai pro websearch_to_tsquery, que aceita aspas e
//	         `-negação` (o que o usuário já espera de uma caixa de busca) e
//	         engole qualquer lixo sem exceção.
//	prefix — entrada do to_tsquery, montada SÓ com tokens alfanuméricos que
//	         NÓS geramos. O texto do usuário nunca chega cru no to_tsquery,
//	         então não existe sintaxe pra quebrar.
//
// prefix é "" quando o usuário usou operadores (aspas ou negação): aí ele está
// sendo específico de propósito, e OR-ar um ramo de prefixo por cima desfaria a
// negação que ele pediu. `to_tsquery(cfg, ”)` devolve tsquery vazia (com
// NOTICE, sem erro), e `x || ”::tsquery` = x — então o SQL não precisa de
// ramo condicional.
func buildTsqueryInputs(q string) (web, prefix string) {
	return q, tsPrefixQuery(q)
}

// tsPrefixQuery monta `termo1 & termo2 & ultimo:*` para autocomplete —
// quem digita "furad" tem que achar "furadeira".
func tsPrefixQuery(q string) string {
	// Operadores do websearch: respeitá-los significa NÃO adicionar o ramo de
	// prefixo, que é um OR e afrouxaria a busca que o usuário restringiu.
	if strings.Contains(q, `"`) || strings.HasPrefix(q, "-") || strings.Contains(q, " -") {
		return ""
	}

	words := make([]string, 0, maxSearchWords)
	for _, field := range strings.Fields(q) {
		// Só letra e dígito sobrevivem. É isto que torna impossível uma
		// tsquery malformada: `&`, `|`, `!`, `(`, `)`, `:`, `*`, aspas e
		// qualquer pontuação são descartados ANTES de virar entrada do
		// to_tsquery. Unicode incluso — "válvula" continua sendo uma palavra.
		var b strings.Builder
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		if w := b.String(); w != "" {
			words = append(words, w)
			if len(words) == maxSearchWords {
				break
			}
		}
	}
	if len(words) == 0 {
		return ""
	}

	// Só o ÚLTIMO termo ganha `:*`. É o que o usuário está digitando agora; os
	// anteriores ele já terminou. Prefixar todos multiplicaria o resultado sem
	// melhorar a intenção ("furadeira imp" traria toda palavra começada em "f").
	words[len(words)-1] += ":*"
	return strings.Join(words, " & ")
}

// -- limiar da busca aproximada ----------------------------------------------

// fuzzyThreshold é o corte de `strict_word_similarity` da busca por
// similaridade. CALIBRADO POR VARREDURA, não chutado — um limiar chutado é pior
// que não ter busca aproximada, porque destrói a confiança na busca.
//
// Método: varri os limiares candidatos contra 12 erros de grafia reais (padrões
// de quem digita em português: letra dobrada, sílaba comida, tecla vizinha) e 7
// termos sem sentido, medindo os DOIS lados ao mesmo tempo — o que se acha e o
// que se traz de lixo junto:
//
//	limiar | acertos | perdidos | ruído | RESULTADOS IRRELEVANTES
//	  0,35 |    11   |    1     |   0   |   38   ← inunda de lixo
//	  0,38 |    11   |    1     |   0   |    6   ← começa o lixo
//	  0,40 |    11   |    1     |   0   |    0
//	  0,42 |    11   |    1     |   0   |    0   ← ESCOLHIDO (meio do platô)
//	  0,44 |    11   |    1     |   0   |    0
//	  0,45 |    10   |    2     |   0   |    0   ← perde 'tomda' (0,444)
//	  0,50 |    10   |    2     |   0   |    0
//
// O platô seguro é [0,40 ; 0,44]: recall máximo com ZERO resultado irrelevante.
// Fora dele há dois precipícios — em 0,38 o lixo aparece, em 0,45 o recall cai.
// 0,42 é o MEIO do platô, e é meio de propósito: escolher uma das pontas seria
// ajustar o número à amostra, e a próxima palavra do catálogo derrubaria a
// escolha para um dos lados.
//
// ⚠️ O 1 caso perdido é 'luninaria' → 'Luminária' (troca m→n no meio da
// palavra), que mede **0,250** — abaixo até do pior falso-positivo conhecido
// ('cimento' → 'Cimeira', 0,333). Não dá pra alcançá-lo sem descer a 0,35 e
// aceitar 38 resultados irrelevantes. É limite conhecido e ACEITO: uma
// substituição de letra no meio de uma palavra longa quebra três trigramas de
// uma vez. Perder um caso raro vale mais que estragar a busca de todo mundo.
//
// String (e não float) porque vai concatenada num `SET LOCAL`, que não aceita
// placeholder. É constante hardcoded, nunca entrada de usuário.
const fuzzyThreshold = "0.42"

// -- sugestão de termo ("você quis dizer?") ----------------------------------

// suggestFromNames devolve o termo corrigido pro "Mostrando resultados para
// **furadeira**", ou "" se não houver correção que valha a pena mostrar.
//
// PORQUÊ EM GO, e não numa consulta: os nomes dos produtos aproximados JÁ estão
// em memória — foram a resposta que acabamos de montar. Uma consulta a mais
// (varrer o catálogo atrás da palavra mais parecida) custaria uma ida ao banco
// pra produzir um enfeite de UI. Aqui custa zero I/O e é testável sem banco.
//
// Corrige PALAVRA A PALAVRA: em "furadera bosch" só "furadera" está errado, e
// sugerir a frase inteira do produto ("Furadeira de Impacto 1/2\" 750W") não é
// uma sugestão de busca, é um título.
func suggestFromNames(q string, products []model.Product) string {
	origem := strings.Fields(q)
	if len(origem) == 0 || len(products) == 0 {
		return ""
	}

	// Vocabulário: todas as palavras dos nomes devolvidos, normalizadas.
	vocab := make([]string, 0, 64)
	vistas := make(map[string]bool, 64)
	for _, p := range products {
		for _, w := range strings.Fields(p.Name) {
			w = normalizeWord(w)
			if len(w) < 3 || vistas[w] {
				continue
			}
			vistas[w] = true
			vocab = append(vocab, w)
		}
	}

	corrigidas := make([]string, len(origem))
	mudou := false
	for i, orig := range origem {
		alvo := normalizeWord(orig)
		corrigidas[i] = orig
		if len(alvo) < 3 {
			continue // "de", "x", "40" não se corrigem
		}
		melhor, melhorSim := "", fuzzyThresholdFloat
		for _, cand := range vocab {
			if s := trigramSimilarity(alvo, cand); s > melhorSim {
				melhor, melhorSim = cand, s
			}
		}
		// `melhor != alvo` evita sugerir exatamente o que o usuário digitou —
		// "Mostrando resultados para **cimento**" quando ele digitou "cimento"
		// é ruído que faz a loja parecer quebrada.
		if melhor != "" && melhor != alvo {
			corrigidas[i] = melhor
			mudou = true
		}
	}
	if !mudou {
		return ""
	}
	return strings.Join(corrigidas, " ")
}

// fuzzyThresholdFloat espelha fuzzyThreshold pro lado Go. Manter os dois em
// sincronia é proposital: a sugestão exibida tem que usar o MESMO corte que
// selecionou os resultados, senão a loja mostra "você quis dizer X" e devolve
// uma lista que não tem nada de X.
const fuzzyThresholdFloat = 0.42

// normalizeWord tira acento, pontuação e caixa — "Acrílica" e "acrilico"
// precisam ser comparáveis, que é justamente o erro que o cliente comete.
func normalizeWord(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if d, ok := deaccent[r]; ok {
			b.WriteRune(d)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// deaccent cobre o que aparece em português. Tabela explícita em vez de
// golang.org/x/text/unicode/norm pra não adicionar dependência ao módulo por
// causa de um enfeite de UI.
var deaccent = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'ê': 'e', 'è': 'e', 'ë': 'e',
	'í': 'i', 'î': 'i', 'ì': 'i', 'ï': 'i',
	'ó': 'o', 'õ': 'o', 'ô': 'o', 'ò': 'o', 'ö': 'o',
	'ú': 'u', 'û': 'u', 'ù': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// trigramSimilarity reproduz o `similarity()` do pg_trgm: Jaccard sobre os
// conjuntos de trigramas, com a mesma convenção de padding (dois espaços na
// frente, um atrás). Só é usada pra ESCOLHER O TEXTO da sugestão — quem
// seleciona as linhas continua sendo o Postgres, com o índice.
func trigramSimilarity(a, b string) float64 {
	ta, tb := trigrams(a), trigrams(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	uni := len(ta) + len(tb) - inter
	if uni == 0 {
		return 0
	}
	return float64(inter) / float64(uni)
}

func trigrams(s string) map[string]bool {
	r := []rune("  " + s + " ")
	out := make(map[string]bool, len(r))
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}

// GetBySlug GET /api/v1/products/:slug
//
// L-CATALOG-2: gera ETag derivado do `updated_at` do produto. Se o cliente
// envia `If-None-Match` que bate, retorna 304 Not Modified — economiza
// bandwidth em browsers/CDN com produto não-modificado.
func (h *ProductHandler) GetBySlug(c *gin.Context) {
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	slug := c.Param("slug")

	row := h.db.QueryRow(`
		SELECT `+productColumns+`
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.slug = $1 AND p.status = 'published'
	`, slug)
	p, err := scanProduct(row)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	h.loadImages(c, &p)
	h.enrichDetail(c, &p)

	if respondWithETag(c, p.UpdatedAt) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// GetByID GET /api/v1/products/by-id/:id
//
// Endpoint usado por outros serviços (order-service) pra resolver um produto
// pelo seu UUID — o frontend mantém ID em vez de slug nos itens do carrinho.
// Retorna o mesmo payload que GetBySlug. O timing pad também aplica aqui pra
// não vazar enumeração de IDs (CT1-H4 generalizado).
func (h *ProductHandler) GetByID(c *gin.Context) {
	start := time.Now()
	defer padToMinElapsed(start, slugLookupMinElapsed)

	id := c.Param("id")

	row := h.db.QueryRow(`
		SELECT `+productColumns+`
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.id = $1 AND p.status = 'published'
	`, id)
	p, err := scanProduct(row)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	h.loadImages(c, &p)
	h.enrichDetail(c, &p)

	if respondWithETag(c, p.UpdatedAt) {
		return
	}
	c.JSON(http.StatusOK, p)
}

// Facets GET /api/v1/products/facets?category=...&q=...
func (h *ProductHandler) Facets(c *gin.Context) {
	params := parseProductsQuery(c)

	// MESMO builder de List: antes eram dois predicados de busca escritos à mão
	// em lugares diferentes, e eles já divergiam (o de Facets não olhava o
	// vendedor). Faceta que não descreve o resultado exibido é pior que faceta
	// nenhuma — o usuário filtra por uma marca que a lista não tem.
	f := buildProductFilters(params, searchFullText, scopeFacets)
	args := f.args
	whereSQL := strings.Join(f.where, " AND ")

	// Brands with counts.
	// #nosec G202 — whereSQL é construído só de literais hardcoded (`p.brand = $N`,
	// `p.category_id = $N`, etc) com placeholders posicionais; valores entram via `args`,
	// nunca em string concat. Atacante não controla os fragmentos de SQL.
	brandSQL := `
		SELECT p.brand, count(*) AS cnt
		FROM products p
		WHERE ` + whereSQL + ` AND p.brand IS NOT NULL
		GROUP BY p.brand ORDER BY cnt DESC, p.brand ASC
	`
	rows, err := h.db.Query(brandSQL, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	brands := make([]model.BrandFacet, 0)
	for rows.Next() {
		var b model.BrandFacet
		if err := rows.Scan(&b.Value, &b.Count); err != nil {
			DBError(c, err)
			return
		}
		brands = append(brands, b)
	}
	// rows.Err() distingue "não há marcas" de "a varredura morreu no meio" —
	// sem isso a vitrine mostraria uma lista de facetas truncada como se fosse
	// completa, e o usuário filtraria por um universo que não existe.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	var priceMin, priceMax sql.NullFloat64
	priceSQL := "SELECT MIN(p.price), MAX(p.price) FROM products p WHERE " + whereSQL
	if err := h.db.QueryRow(priceSQL, args...).Scan(&priceMin, &priceMax); err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.Facets{
		Brands:     brands,
		PriceMin:   priceMin.Float64,
		PriceMax:   priceMax.Float64,
		Attributes: h.attributeFacets(c, params, whereSQL, args),
	})
}

// attributeFacets monta as facetas técnicas (bitola, tensão, potência) do
// resultado atual.
//
// SÓ COM CATEGORIA: atributo é definido por categoria no registry. Facetar
// "potência" sobre o catálogo inteiro juntaria furadeira com saco de cimento e
// devolveria um slider sem sentido — além de varrer a tabela toda.
//
// Numéricos viram min/max (slider); textuais viram valores com contagem
// (checkbox). Cada um é uma query só, agrupada — não uma query por atributo.
func (h *ProductHandler) attributeFacets(c *gin.Context, params productsQuery, whereSQL string, args []any) []model.AttributeFacet {
	out := make([]model.AttributeFacet, 0)
	if params.Category == "" {
		return out
	}

	// `p` continua na cláusula porque whereSQL referencia p.*.
	//
	// O JOIN com `sellers` FOI REMOVIDO: ele só existia porque o whereSQL antigo
	// citava `s.name` na busca por `q` — e era justamente esse `s.name` dentro
	// do OR que impedia o planejador de usar índice em products. Agora o nome do
	// vendedor mora em p.search_vector (peso D) e o predicado é de uma tabela só.
	//
	// #nosec G202 — whereSQL vem do mesmo builder das outras queries: literais
	// hardcoded com placeholders posicionais.
	base := `
		FROM product_attributes pa
		JOIN products p ON p.id = pa.product_id
		JOIN category_attributes ca ON ca.key = pa.key AND ca.category_id = p.category_id
		WHERE ` + whereSQL + ` AND ca.filterable = true`

	rows, err := h.db.Query(`
		SELECT ca.key, ca.label, ca.data_type, ca.unit,
		       pa.value_text,
		       MIN(pa.value_num), MAX(pa.value_num),
		       count(*), MIN(ca.sort_order)
		`+base+`
		GROUP BY ca.key, ca.label, ca.data_type, ca.unit, pa.value_text
		ORDER BY MIN(ca.sort_order) ASC, ca.key ASC, count(*) DESC`, args...)
	if err != nil {
		// Faceta é enriquecimento: sem ela a busca ainda funciona. Não vale um
		// 500 na vitrine inteira.
		slog.Error("facets.attributes_failed",
			"category", params.Category,
			"request_id", c.GetString("request_id"), "error", err.Error())
		return out
	}
	defer rows.Close()

	byKey := map[string]*model.AttributeFacet{}
	order := []string{}

	for rows.Next() {
		var (
			key, label, dataType string
			unit, valueText      *string
			min, max             *float64
			count, sortOrder     int
		)
		if err := rows.Scan(&key, &label, &dataType, &unit, &valueText, &min, &max, &count, &sortOrder); err != nil {
			slog.Error("facets.attributes_scan_failed", "request_id", c.GetString("request_id"), "error", err.Error())
			return out
		}

		f, ok := byKey[key]
		if !ok {
			f = &model.AttributeFacet{Key: key, Label: label, DataType: dataType, Unit: unit}
			byKey[key] = f
			order = append(order, key)
		}

		if dataType == "number" {
			if min != nil && (f.Min == nil || *min < *f.Min) {
				f.Min = min
			}
			if max != nil && (f.Max == nil || *max > *f.Max) {
				f.Max = max
			}
			continue
		}
		if valueText != nil {
			f.Values = append(f.Values, model.AttributeValueFacet{Value: *valueText, Count: count})
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("facets.attributes_iter_failed", "request_id", c.GetString("request_id"), "error", err.Error())
		return out
	}

	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// `Related` MUDOU DE ARQUIVO: virou RecommendationHandler.Related, em
// recommendation.go.
//
// O que havia aqui era `mesma categoria ORDER BY rating DESC LIMIT 4` — que
// devolvia os mesmos 4 produtos para toda a categoria, ordenados por um número
// que o seed inventou. Aquilo continua existindo, mas como TERCEIRA opção e
// marcado no payload como fallback, atrás de co-compra agregada e de regra
// técnica de complemento. Ver o cabeçalho de recommendation.go.

// -- helpers -----------------------------------------------------------------

// CT1-M1: caps por filtro pra prevenir DoS via query string absurda
// (ILIKE em string de 100KB queima CPU mesmo com escape).
const (
	maxFilterQ        = 100 // termo de busca (typed)
	maxFilterCategory = 64  // slug
	maxFilterBrand    = 64  // nome de marca
	maxFilterSort     = 32  // string de sort (já tem whitelist)
	maxFilterCode     = 64  // SKU / código de barras (EAN tem 14; SKU de fornecedor é maior)
	maxFilterAttr     = 128 // par `chave:valor` de faceta técnica
)

// sanitizeText remove o que o Postgres NÃO ACEITA em texto, antes que vire
// argumento de consulta.
//
// 🐛 BUG REAL, pego por teste de entrada hostil: `?q=%00` (byte nulo) fazia o
// driver devolver `pq: invalid byte sequence for encoding "UTF8": 0x00` e a
// busca respondia **500**. Ou seja: derrubar a vitrine custava uma query
// string de 5 caracteres. O problema é anterior ao tsvector — o ILIKE tinha
// exatamente a mesma falha, ela só nunca havia sido testada.
//
// Também caem os demais caracteres de controle e os runes inválidos de UTF-8
// (que viram RuneError na decodificação e chegariam malformados ao banco).
// Nada disso é digitável numa caixa de busca: só aparece em ataque.
func sanitizeText(s string) string {
	limpo := strings.Map(func(r rune) rune {
		if r == utf8.RuneError || r == 0 || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(limpo)
}

// truncateRunes corta a string mantendo os primeiros N runes (não bytes).
// Evita explodir caracteres UTF-8 multibyte no meio.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

type productsQuery struct {
	Category string
	Q        string
	Brand    string
	SKU      string
	Barcode  string
	PriceMin *float64
	PriceMax *float64
	InStock  bool
	Sort     string
	Page     int
	PerPage  int
	Attrs    []attrFilter
}

// attrFilter é um filtro por atributo tipado (`?attr=potencia_w:650`).
// Num != nil quando o valor parseou como número — aí a comparação vai contra
// `value_num` e não contra a representação textual (2.5 ≠ "2.500000").
type attrFilter struct {
	Key string
	Str string
	Num *float64
	Op  string // "eq" | "gte" | "lte"
}

// attrKeyRe: chave de atributo é identificador do nosso registry, não texto
// livre do usuário. Restringir aqui evita que qualquer coisa vire argumento de
// query e limita o custo de uma query maliciosa.
var attrKeyRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// maxAttrFilters: cada filtro vira um EXISTS. Sem teto, `?attr=` repetido 500
// vezes monta um plano absurdo — mesmo problema de DoS que CT1-M1 tratou.
const maxAttrFilters = 8

// parseAttrFilters lê os parâmetros `attr`, `attr_min` e `attr_max`, todos no
// formato `chave:valor` e todos repetíveis.
//
//	?attr=cor:Azul&attr_min=potencia_w:700
//
// Formato inválido é IGNORADO, não vira 400: filtro de faceta chega de link
// compartilhado e de botão de UI antiga, e derrubar a busca inteira por um
// parâmetro obsoleto é pior que ignorá-lo.
func parseAttrFilters(c *gin.Context) []attrFilter {
	out := make([]attrFilter, 0, 4)

	collect := func(param, op string) {
		for _, raw := range c.QueryArray(param) {
			if len(out) >= maxAttrFilters {
				return
			}
			key, val, ok := strings.Cut(truncateRunes(raw, maxFilterAttr), ":")
			if !ok || !attrKeyRe.MatchString(key) || val == "" {
				continue
			}
			f := attrFilter{Key: key, Str: val, Op: op}
			if n, err := strconv.ParseFloat(val, 64); err == nil {
				f.Num = &n
			}
			// Faixa numérica só faz sentido com número. `attr_min=cor:Azul` é
			// um erro de quem chamou, não um filtro textual disfarçado.
			if op != "eq" && f.Num == nil {
				continue
			}
			out = append(out, f)
		}
	}

	collect("attr", "eq")
	collect("attr_min", "gte")
	collect("attr_max", "lte")
	return out
}

// appendAttrFilters traduz os filtros de atributo em EXISTS correlacionados.
// Devolve where/args/idx atualizados pra encaixar no builder de List/Facets.
func appendAttrFilters(where []string, args []any, idx int, params productsQuery) ([]string, []any, int) {
	for _, f := range params.Attrs {
		var cond string
		switch f.Op {
		case "gte":
			cond = fmt.Sprintf("pa.value_num >= $%d", idx+1)
			args = append(args, f.Key, *f.Num)
		case "lte":
			cond = fmt.Sprintf("pa.value_num <= $%d", idx+1)
			args = append(args, f.Key, *f.Num)
		default:
			if f.Num != nil {
				// Casa pelos dois lados: "2.5" tanto como número quanto como
				// texto, porque o mesmo atributo pode estar cadastrado como
				// number numa categoria e text noutra.
				cond = fmt.Sprintf("(pa.value_num = $%d OR pa.value_text = $%d::text)", idx+1, idx+2)
				args = append(args, f.Key, *f.Num, f.Str)
			} else {
				cond = fmt.Sprintf("pa.value_text = $%d", idx+1)
				args = append(args, f.Key, f.Str)
			}
		}

		// #nosec G202 — `cond` é literal hardcoded com placeholders posicionais;
		// chave e valor entram por `args`.
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM product_attributes pa WHERE pa.product_id = p.id AND pa.key = $%d AND %s)",
			idx, cond))

		idx += 2
		if f.Op == "eq" && f.Num != nil {
			idx++ // o caso numérico consome um placeholder extra (num + texto)
		}
	}
	return where, args, idx
}

func parseProductsQuery(c *gin.Context) productsQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "24"))
	if perPage < 1 || perPage > 100 {
		perPage = 24
	}

	// CT1-M1: trunca cada filtro pra um cap sensato. Cap > limite humano (q=100
	// chars já é uma frase longa) mas << que payload abusivo.
	// sanitizeText ANTES do truncate em todo filtro textual: byte nulo e
	// caractere de controle viram erro do driver e 500 (ver sanitizeText).
	q := productsQuery{
		Category: truncateRunes(sanitizeText(c.Query("category")), maxFilterCategory),
		Q:        truncateRunes(sanitizeText(c.Query("q")), maxFilterQ),
		Brand:    truncateRunes(sanitizeText(c.Query("brand")), maxFilterBrand),
		InStock:  c.Query("in_stock") == "true",
		Sort:     truncateRunes(c.Query("sort"), maxFilterSort),
		Page:     page,
		PerPage:  perPage,
		// SKU e barcode são lookup EXATO (`=`), nunca ILIKE: é leitura de
		// scanner e digitação de código no balcão — precisa bater o índice
		// único e voltar em microssegundos, não varrer trigrama.
		SKU:     truncateRunes(sanitizeText(c.Query("sku")), maxFilterCode),
		Barcode: truncateRunes(sanitizeText(c.Query("barcode")), maxFilterCode),
		Attrs:   parseAttrFilters(c),
	}

	// price_min/max só aceitos se não-negativos (audit CT1-H3) — preço negativo
	// causa lógica de negócio surpresa.
	if v := c.Query("price_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			q.PriceMin = &f
		}
	}
	if v := c.Query("price_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			q.PriceMax = &f
		}
	}
	return q
}

// escapeLikePattern escapa os metacaracteres do LIKE/ILIKE (`%`, `_`, `\`).
// Usado junto com `ESCAPE '\'` no SQL pra prevenir ReDoS via wildcard injection
// no pg_trgm (audit CT1-C1).
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

type scanner interface {
	Scan(dest ...any) error
}

// scanProduct lê a projeção `productColumns` — a ordem dos destinos TEM que
// bater com a ordem das colunas lá.
func scanProduct(row scanner) (model.Product, error) {
	var p model.Product
	err := row.Scan(productScanTargets(&p)...)
	return p, err
}

// productScanTargets é a lista de destinos de Scan correspondente, NA ORDEM, à
// projeção `productColumns`.
//
// Extraída de scanProduct quando as consultas de recomendação passaram a
// carregar colunas EXTRAS depois da projeção pública (o número de pedidos que
// sustenta a co-compra, a nota da regra técnica): elas precisam da mesma lista
// com um sufixo. Duplicar a ordem num segundo lugar era garantir que a próxima
// coluna nova entrasse só num dos dois — e o modo de falha de scan fora de
// ordem é dado ERRADO no card, não erro.
func productScanTargets(p *model.Product) []any {
	return []any{
		&p.ID, &p.Slug, &p.Name, &p.Category, &p.Price, &p.OriginalPrice, &p.Currency, &p.Icon, &p.Brand,
		&p.Seller, &p.SellerID, &p.SellerRating, &p.SellerReviewCt,
		&p.Stock, &p.Rating, &p.ReviewCount, &p.CashbackAmount, &p.Badge, &p.BadgeLabel, &p.Installments,
		&p.Description, &p.Specs, &p.CreatedAt, &p.UpdatedAt,
		&p.SKU, &p.Barcode, &p.UnitOfMeasure, &p.QtyStep,
		&p.WeightKg, &p.LengthCm, &p.WidthCm, &p.HeightCm,
		&p.RatingScore,
	}
}

// adminProductScanTargets é o equivalente para a projeção de ADMIN
// (`productColumns` + custo + fiscais + status), usada por admin_catalog.go e
// admin_product_list.go.
//
// 🐛 BUG REAL que motivou a extração: as duas rotas de admin repetiam a lista de
// 32 destinos À MÃO. Ao acrescentar uma coluna em `productColumns`, o `Scan`
// público foi atualizado e os dois de admin não — resultado:
// `sql: expected 41 destination arguments in Scan, not 40`, e a tela de gestão
// de produtos abrindo com 500. Foi barato porque a contagem não bateu; o modo
// de falha CARO da mesma classe é uma coluna nova do MESMO tipo entrar na
// posição errada, e aí não há erro nenhum — só custo aparecendo no campo de
// preço. Uma lista, um lugar.
func adminProductScanTargets(p *model.AdminProduct, cost **float64) []any {
	return append(productScanTargets(&p.Product),
		cost, &p.SupplierID, &p.SupplierSKU, &p.NCM, &p.CFOP, &p.CEST, &p.Origem, &p.Status)
}

// loadTiers carrega as faixas de atacado de um produto.
//
// Só usada no DETALHE, nunca na listagem: 24 cards × 1 query cada seria N+1 por
// uma informação que o card não mostra. Quando a vitrine quiser exibir "a
// partir de R$ X" no card, o caminho é um LEFT JOIN LATERAL na query de
// listagem, não este helper em loop.
func loadTiers(db *sql.DB, productID string) ([]model.PriceTier, error) {
	rows, err := db.Query(`
		SELECT min_qty, price FROM product_price_tiers
		WHERE product_id = $1 ORDER BY min_qty ASC
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.PriceTier
	for rows.Next() {
		var t model.PriceTier
		if err := rows.Scan(&t.MinQty, &t.Price); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loadAttributes junta os valores tipados do produto com o registry da
// categoria (label/unidade). Atributo gravado que não está mais no registry
// fica de fora — o registry é a fonte da verdade do que a UI sabe exibir.
func loadAttributes(db *sql.DB, productID, categoryID string) ([]model.ProductAttribute, error) {
	rows, err := db.Query(`
		SELECT ca.key, ca.label, ca.data_type, ca.unit, pa.value_num, pa.value_text, pa.value_bool
		FROM product_attributes pa
		JOIN category_attributes ca ON ca.key = pa.key AND ca.category_id = $2
		WHERE pa.product_id = $1
		ORDER BY ca.sort_order ASC, ca.key ASC
	`, productID, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ProductAttribute
	for rows.Next() {
		var a model.ProductAttribute
		if err := rows.Scan(&a.Key, &a.Label, &a.DataType, &a.Unit, &a.ValueNum, &a.ValueText, &a.ValueBool); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// enrichDetail anexa faixas de preço e atributos ao produto do detalhe.
//
// Falha aqui NÃO derruba a resposta: o produto ainda é vendável sem faixa de
// atacado e sem ficha técnica estruturada. Logamos e seguimos — devolver 500
// numa página de produto por causa de um atributo seria trocar uma degradação
// invisível por uma venda perdida.
func (h *ProductHandler) enrichDetail(c *gin.Context, p *model.Product) {
	if tiers, err := loadTiers(h.db, p.ID); err != nil {
		logDetailEnrichFailure(c, p.ID, "price_tiers", err)
	} else {
		p.PriceTiers = tiers
	}
	if attrs, err := loadAttributes(h.db, p.ID, p.Category); err != nil {
		logDetailEnrichFailure(c, p.ID, "attributes", err)
	} else {
		p.Attributes = attrs
	}
}

// loadImages substitui o padrão anterior (erro do Query ignorado e rows.Err()
// nunca checado), que podia devolver uma galeria truncada como se fosse a
// galeria completa.
func (h *ProductHandler) loadImages(c *gin.Context, p *model.Product) {
	rows, err := h.db.Query(`
		SELECT id, url, alt, variants FROM product_images WHERE product_id=$1 ORDER BY sort_order ASC
	`, p.ID)
	if err != nil {
		logDetailEnrichFailure(c, p.ID, "images", err)
		return
	}
	defer rows.Close()

	var imgs []model.ProductImage
	for rows.Next() {
		var im model.ProductImage
		var rawVariants []byte
		if err := rows.Scan(&im.ID, &im.URL, &im.Alt, &rawVariants); err != nil {
			logDetailEnrichFailure(c, p.ID, "images", err)
			return
		}
		// Imagem própria: `url` do banco guarda a CHAVE, não uma URL. Resolver
		// aqui é o que mantém o banco livre de URL absoluta. Imagem externa já
		// vem com http(s) e o resolver a devolve intacta.
		im.Variants = h.imageVariants(rawVariants)
		if im.Variants != nil {
			// O detalhe usa a média no carrossel; o zoom pede a grande via
			// `variants.large`. Mandar a grande em `url` faria o slide baixar
			// 1600px que a tela não usa.
			im.URL = im.Variants.Medium
		}
		imgs = append(imgs, im)
	}
	if err := rows.Err(); err != nil {
		logDetailEnrichFailure(c, p.ID, "images", err)
		return
	}
	p.Images = imgs
}

// loadThumbnails preenche a imagem de capa dos produtos de UMA página de
// listagem, em uma única query.
//
// PORQUÊ existe: a vitrine (List) não devolvia imagem nenhuma — só o detalhe
// devolvia. O card do produto caía no emoji da categoria, e ninguém compra uma
// furadeira de R$ 429 olhando "⚒". Foto na vitrine é o que vende.
//
// PORQUÊ em lote: chamar loadImages por produto seria N+1 (100 itens = 100
// queries). Aqui é uma query com `= ANY($1)`, e o resultado é distribuído em
// memória.
//
// Só a capa (menor sort_order) entra: a galeria completa continua exclusiva do
// detalhe. Mandar 5 imagens de 100 produtos inflaria a resposta da listagem sem
// que o card use nenhuma delas.
//
// Falha aqui é degradação, não erro: sem foto o card cai no ícone, que é o
// comportamento de antes. Listagem inteira não pode falhar por causa da capa.
func (h *ProductHandler) loadThumbnails(c *gin.Context, products []model.Product) {
	if len(products) == 0 {
		return
	}

	ids := make([]string, 0, len(products))
	for i := range products {
		ids = append(ids, products[i].ID)
	}

	rows, err := h.db.Query(`
		SELECT DISTINCT ON (product_id) product_id, url, alt, variants
		FROM product_images
		WHERE product_id = ANY($1)
		ORDER BY product_id, sort_order ASC
	`, pq.Array(ids))
	if err != nil {
		slog.Error("product.list_thumbnails_failed",
			"request_id", c.GetString("request_id"), "error", err.Error())
		return
	}
	defer rows.Close()

	capas := make(map[string]model.ProductImage, len(products))
	for rows.Next() {
		var pid string
		var im model.ProductImage
		var rawVariants []byte
		if err := rows.Scan(&pid, &im.URL, &im.Alt, &rawVariants); err != nil {
			slog.Error("product.list_thumbnails_failed",
				"request_id", c.GetString("request_id"), "error", err.Error())
			return
		}
		// A VITRINE SERVE A MINIATURA. Este `im.URL = Thumb` é a linha que
		// decide se a listagem no celular carrega em 1s ou em 20s: são ~20
		// cards por página, e servir a imagem de zoom em cada um são megabytes
		// para desenhar 150px de tela.
		im.Variants = h.imageVariants(rawVariants)
		if im.Variants != nil {
			im.URL = im.Variants.Thumb
		}
		capas[pid] = im
	}
	if err := rows.Err(); err != nil {
		slog.Error("product.list_thumbnails_failed",
			"request_id", c.GetString("request_id"), "error", err.Error())
		return
	}

	for i := range products {
		if im, ok := capas[products[i].ID]; ok {
			products[i].Images = []model.ProductImage{im}
		}
	}
}

func logDetailEnrichFailure(c *gin.Context, productID, part string, err error) {
	slog.Error("product.detail_enrich_failed",
		"product_id", productID, "part", part,
		"request_id", c.GetString("request_id"), "error", err.Error())
}
