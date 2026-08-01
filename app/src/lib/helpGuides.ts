// Guias de OPERAÇÃO (equipe): como usar as features do balcão e do admin.
// Separado do FAQ do cliente (institutional/HelpPage). Conteúdo estruturado em
// blocos simples (sem markdown/dep) — renderizado por GuidePage. Deep-linkável:
// as telas do painel/balcão apontam pra cá com /ajuda/operacao/<slug>.

export type Block = { h: string } | { p: string } | { steps: string[] } | { tip: string }

export interface Guide {
  slug: string
  title: string
  category: string
  summary: string
  blocks: Block[]
}

export const GUIDE_CATEGORIES = [
  'Balcão (PDV)',
  'Catálogo',
  'Pedidos e entrega',
  'Estoque',
  'Financeiro',
  'Acesso e auditoria',
] as const

export const GUIDES: Guide[] = [
  // ---------------- Balcão ----------------
  {
    slug: 'balcao-fazer-venda',
    title: 'Fazer uma venda no balcão',
    category: 'Balcão (PDV)',
    summary: 'Montar a comanda, adicionar itens e fechar a venda no tablet.',
    blocks: [
      {
        p: 'O balcão (PDV) é feito para o tablet, na tela `/balcao`. Cada atendimento é uma comanda.',
      },
      {
        steps: [
          'Abra uma comanda (o botão "Nova comanda").',
          'Busque o produto por nome, SKU ou código de barras e adicione ao carrinho.',
          'Ajuste a quantidade de cada item.',
          '(Opcional) Vincule um cliente pelo CPF/CNPJ, ou cadastre na hora.',
          'Escolha a forma de pagamento e finalize.',
        ],
      },
      {
        tip: 'Vários atendimentos ao mesmo tempo? Cada um vira uma aba de comanda — troca com um toque.',
      },
    ],
  },
  {
    slug: 'balcao-desconto-aprovacao',
    title: 'Desconto e a fila de aprovação',
    category: 'Balcão (PDV)',
    summary: 'Como o teto de desconto por cargo funciona e quando vai pro gerente.',
    blocks: [
      {
        p: 'Cada operador tem um TETO de desconto pelo cargo. Dentro do teto, o desconto é aplicado na hora. Acima do teto, a venda vai para a fila de aprovação do gerente.',
      },
      {
        steps: [
          'Aplique o desconto no item ou na comanda.',
          'Se passar do seu teto, a venda entra em "Aguardando aprovação".',
          'Um supervisor/gerente abre `/balcao/aprovacoes`, revisa e aprova ou recusa (recusa exige motivo).',
          'Aprovado, o operador finaliza a venda normalmente.',
        ],
      },
      { tip: 'Ninguém aprova o próprio desconto — é regra travada no servidor, não só na tela.' },
    ],
  },
  {
    slug: 'balcao-cliente-rapido',
    title: 'Cadastrar um cliente rápido',
    category: 'Balcão (PDV)',
    summary: 'Vincular ou criar o cliente sem sair da comanda.',
    blocks: [
      {
        steps: [
          'No bloco do cliente, busque pelo CPF/CNPJ.',
          'Se já existir, é só selecionar.',
          'Se não, preencha nome + documento + telefone e cadastre — o cliente já fica vinculado à comanda.',
        ],
      },
      {
        tip: 'Cliente vinculado é o que permite emitir a nota no nome dele e ele ver a compra na conta depois.',
      },
    ],
  },

  // ---------------- Catálogo ----------------
  {
    slug: 'produto-publicar-editar',
    title: 'Publicar e editar um produto',
    category: 'Catálogo',
    summary: 'Tirar do rascunho, ajustar preço/estoque e colocar na vitrine.',
    blocks: [
      {
        p: 'Produto importado entra como RASCUNHO de propósito — publicar é decisão humana. Vá em Admin → Produtos.',
      },
      {
        steps: [
          'Ache o produto (busca por nome ou SKU; filtre por situação "rascunho").',
          'Abra para editar: confira nome, preço, categoria e estoque.',
          'Só o admin/vendas vê e edita o CUSTO (para conferir a margem — a tela avisa se o preço ficar abaixo do custo).',
          'Salve e mude a situação para "publicado" para aparecer na vitrine.',
        ],
      },
      {
        tip: 'Publicou sem foto? Tudo bem — o produto aparece com "Sem foto ainda" (melhor que foto errada). Suba as fotos depois pelo guia de imagens em lote.',
      },
    ],
  },
  {
    slug: 'imagens-em-lote',
    title: 'Subir fotos em lote (por SKU)',
    category: 'Catálogo',
    summary: 'Dar foto a dezenas/centenas de produtos de uma vez.',
    blocks: [
      {
        p: 'Em Admin → Imagens em lote. A ideia: você fotografa cada produto e nomeia o arquivo pelo SKU; o sistema casa a foto ao produto pelo código — nunca pelo palpite.',
      },
      {
        steps: [
          'Fotografe o produto (celular serve; o sistema corta em quadrado e ajusta sozinho).',
          'Nomeie cada arquivo pelo SKU: 6320.jpg. Mais de uma foto do mesmo produto? 6320-2.jpg, 6320-3.jpg.',
          'Arraste todos os arquivos para a área da tela (ou clique, ou cole com Ctrl+V).',
          'Confira o que casou; os "sem produto" são nomes que não bateram com nenhum SKU.',
          'Clique em "Enviar" — sobe em paralelo, com barra de progresso e opção de tentar de novo no que falhar.',
        ],
      },
      {
        tip: 'Não precisa fazer tudo de uma vez. Comece pelos campeões de venda. Foto errada é pior que sem foto.',
      },
    ],
  },
  {
    slug: 'categorias',
    title: 'Organizar categorias',
    category: 'Catálogo',
    summary: 'Criar, renomear e excluir categorias do catálogo.',
    blocks: [
      {
        p: 'Admin → Categorias. O identificador (slug) é fixo depois de criado — é a chave dos produtos.',
      },
      {
        steps: [
          'Para criar: informe o identificador (ex.: fechaduras), o nome e um ícone opcional.',
          'Para renomear: edite o nome na linha (o identificador não muda).',
          'Para excluir: só é possível se não houver produto na categoria.',
        ],
      },
    ],
  },

  // ---------------- Pedidos e entrega ----------------
  {
    slug: 'pedidos-fulfillment',
    title: 'Separar, despachar e entregar pedidos',
    category: 'Pedidos e entrega',
    summary: 'Tocar o pedido online da loja: da separação à entrega.',
    blocks: [
      { p: 'Admin → Pedidos. Cada pedido pago segue um fluxo até a entrega.' },
      {
        steps: [
          'Pedido "pago" → marque "Em separação" quando começar a separar.',
          '"Em separação" → "Despachado" quando sair para entrega/transportadora.',
          '"Despachado" → "Entregue" quando o cliente receber.',
          'Precisa cancelar? Dá para cancelar até a separação (o estoque volta).',
        ],
      },
      { tip: 'O contador vê a lista (faturamento), mas não muda status — só quem opera.' },
    ],
  },
  {
    slug: 'devolucoes',
    title: 'Aprovar, receber e estornar devoluções',
    category: 'Pedidos e entrega',
    summary: 'A fila de devolução da loja (CDC), passo a passo.',
    blocks: [
      { p: 'Admin → Devoluções. O fluxo separa o físico (mercadoria) do dinheiro (estorno).' },
      {
        steps: [
          'Devolução "solicitada" → Aprovar ou Recusar (recusa exige motivo).',
          'Aprovada → quando a mercadoria chegar, clique em "Receber" (aí o estoque volta).',
          'Recebida → "Estornar" devolve o dinheiro. Só o admin faz o estorno.',
        ],
      },
      {
        tip: 'Quem confere a mercadoria (almoxarife/vendas) não estorna. Dinheiro saindo é decisão do dono.',
      },
    ],
  },
  {
    slug: 'frete',
    title: 'Configurar o frete por CEP',
    category: 'Pedidos e entrega',
    summary: 'Editar as faixas de frete que o cliente paga no checkout.',
    blocks: [
      { p: 'Admin → Frete. É o valor que o cliente paga — só o admin mexe.' },
      {
        steps: [
          'Cada faixa é: zona + intervalo de CEP + serviço (padrão/expressa) + custo base + custo por item + prazo + valor de frete grátis.',
          'Para adicionar: preencha o formulário e clique em "Adicionar faixa".',
          'Para corrigir: clique em "Editar" na linha.',
        ],
      },
      {
        tip: 'A tela avisa em vermelho se detectar faixa de CEP de São Paulo — o padrão inicial veio de SP e precisa ser trocado pelos CEPs reais da loja.',
      },
    ],
  },

  // ---------------- Estoque ----------------
  {
    slug: 'estoque-ajuste',
    title: 'Ajustar estoque com motivo',
    category: 'Estoque',
    summary: 'Dar entrada, baixa e acompanhar o histórico e o alerta de baixo.',
    blocks: [
      {
        p: 'Admin → Estoque. O ajuste é RELATIVO (você lança a diferença), com motivo obrigatório — nada de sobrescrever número sem rastro.',
      },
      {
        steps: [
          'Ache o produto (os de estoque baixo já aparecem no topo, com alerta).',
          'Clique em "Gerenciar".',
          'Escolha Entrada (+) ou Baixa (−), a quantidade e o motivo (recebimento, avaria, contagem…).',
          'Aplique. O histórico do produto fica ao lado, com quem fez e quando.',
        ],
      },
      { tip: 'Use o filtro "só estoque baixo" para saber o que precisa repor.' },
    ],
  },

  // ---------------- Financeiro ----------------
  {
    slug: 'financeiro-contabil',
    title: 'Livro contábil e conciliação',
    category: 'Financeiro',
    summary: 'Faturamento, período e bater as vendas com o gateway (contador).',
    blocks: [
      {
        p: 'Admin → Auditoria contábil. É o livro em partidas dobradas: cada centavo entra com contrapartida. Acesso do admin e do contador.',
      },
      {
        steps: [
          'Escolha o período no seletor.',
          'Veja o resumo (faturamento, por método) e o balancete.',
          'Rode a conciliação para comparar o que o sistema registrou com o que o gateway (PSP) reporta — divergências ficam listadas.',
          'Exporte em CSV/OFX quando precisar mandar pro contador.',
        ],
      },
      {
        tip: 'Config de pagamento (qual gateway está ativo e a saúde da credencial) fica em Admin → Pagamento — sem expor segredo.',
      },
    ],
  },

  // ---------------- Acesso e auditoria ----------------
  {
    slug: 'personas-acesso',
    title: 'Papéis: quem vê e faz o quê',
    category: 'Acesso e auditoria',
    summary: 'As personas do painel (contador, vendas, almoxarife) e como atribuir.',
    blocks: [
      { p: 'O painel mostra para cada pessoa só o que ela pode usar. Os papéis:' },
      {
        steps: [
          'Admin: tudo.',
          'Contador: contábil, trilhas e relatórios; leitura no resto; NÃO vê custo.',
          'Vendas: catálogo, pedidos, estoque, imagens, balcão; vê custo (para negociar margem).',
          'Almoxarife: pedidos (separação/despacho), estoque e devolução física; NÃO vê custo.',
        ],
      },
      {
        p: 'Para dar um papel a alguém: Admin → Operadores (para o balcão) ou a atribuição de papel do usuário. Cada troca de papel fica registrada na auditoria.',
      },
      { tip: 'Custo/margem é o dado mais sensível: só admin e vendas veem, em qualquer tela.' },
    ],
  },
  {
    slug: 'auditoria-atividade',
    title: 'Auditoria: quem fez o quê, quando',
    category: 'Acesso e auditoria',
    summary: 'A trilha unificada de tudo que acontece no painel.',
    blocks: [
      {
        p: 'Admin → Atividade. Junta as três frentes numa linha do tempo só: catálogo (produto/preço/estoque), staff (operador, mudança de papel) e operação (venda de balcão, desconto, devolução, estorno).',
      },
      {
        steps: [
          'Filtre por serviço, ação, ator ou período.',
          'Cada linha mostra quem fez, o que mudou (de → para) e quando.',
        ],
      },
      {
        tip: 'A trilha é imutável (encadeada por hash) — serve para investigar qualquer coisa com confiança.',
      },
    ],
  },
]

export function guidesByCategory(): Record<string, Guide[]> {
  const out: Record<string, Guide[]> = {}
  for (const cat of GUIDE_CATEGORIES) out[cat] = []
  for (const g of GUIDES) (out[g.category] ??= []).push(g)
  return out
}

export function findGuide(slug: string): Guide | undefined {
  return GUIDES.find((g) => g.slug === slug)
}
