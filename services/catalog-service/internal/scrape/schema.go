// Package scrape coleta, normaliza e estrutura dados de produtos de FERRAGEM
// (dobradiças, fechaduras, puxadores, parafusos, buchas, cadeados, trilhos,
// corrediças e correlatos de fixação/marcenaria/serralheria) a partir de
// catálogos públicos de distribuidores, para ALIMENTAR o fluxo de ingestão do
// catálogo — nunca escrevendo direto na base de produção.
//
// Princípios (herdados da spec do agente e do docs/ingestao-de-produtos.md):
//   - Escopo estrito: produtos de ferragem, não um scraper genérico.
//   - Ética/legal NÃO negociável: respeita robots.txt/ToS, rate limit por
//     domínio, User-Agent identificável, nunca burla CAPTCHA/login/paywall.
//   - Saída é um LOTE para revisão humana (draft), nunca publica sozinho.
//   - Imagem/texto têm direito autoral: coletamos a URL de origem; re-hospedar
//     e reexibir publicamente é decisão humana, sinalizada, não do robô.
package scrape

import "time"

// ScrapedProduct é o formato ÚNICO de saída, independente da fonte. Espelha o
// schema da spec (chaves em pt-BR/snake_case no JSON). Campos opcionais são
// ponteiros: distinguir "ausente" de "vazio" importa para a validação.
type ScrapedProduct struct {
	Fonte                string   `json:"fonte"`
	URLOrigem            string   `json:"url_origem"`
	CodigoFabricante     *string  `json:"codigo_fabricante"`
	Nome                 string   `json:"nome"`
	CategoriaBruta       string   `json:"categoria_bruta"`
	CategoriaNormalizada string   `json:"categoria_normalizada"`
	Descricao            *string  `json:"descricao"`
	Preco                *float64 `json:"preco"`
	Moeda                string   `json:"moeda"` // sempre "BRL"
	ImagemURLOriginal    *string  `json:"imagem_url_original"`
	UnidadeVenda         *string  `json:"unidade_venda"` // UN, CX, KG…
	ColetadoEm           time.Time `json:"coletado_em"`
}

// Flag marca um item que PASSA para revisão humana (não é descartado em
// silêncio) — tipicamente por falta de campo obrigatório.
type Flag struct {
	URLOrigem string   `json:"url_origem"`
	Nome      string   `json:"nome"`
	Motivos   []string `json:"motivos"` // ex: "sem nome", "sem imagem", "sem categoria"
}

// RunError é uma falha localizada (uma URL) que não interrompe o lote inteiro.
type RunError struct {
	URLOrigem string `json:"url_origem"`
	Erro      string `json:"erro"`
}

// Report é o que se reporta ao final de cada execução. `novos`/`atualizados`
// NÃO vivem aqui de propósito: essa decisão é do IMPORT (compara com a base,
// entra como draft), não do scrape. Aqui contamos o que a coleta viu.
type Report struct {
	Fonte            string     `json:"fonte"`
	IniciadoEm       time.Time  `json:"iniciado_em"`
	FinalizadoEm     time.Time  `json:"finalizado_em"`
	URLsDescobertas  int        `json:"urls_descobertas"`
	Coletados        int        `json:"coletados"`
	Duplicatas       int        `json:"duplicatas_removidas"`
	Sinalizados      []Flag     `json:"sinalizados_para_revisao"`
	Erros            []RunError `json:"erros"`
	DominiosPausados []string   `json:"dominios_pausados"` // robots.txt ou 429/bloqueio
}

// Batch é o arquivo de saída pronto para revisão + import.
type Batch struct {
	Report   Report           `json:"report"`
	Produtos []ScrapedProduct `json:"produtos"`
}
