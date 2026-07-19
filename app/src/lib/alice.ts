// Cliente da assistente Alice ✨. Base URL independente (VITE_ASSISTANT_URL);
// vazio = mock leve no cliente (o assistant-service tem o modo mock próprio, mas
// aqui garantimos que a bolha funcione mesmo sem backend).
const ASSISTANT_URL = import.meta.env.VITE_ASSISTANT_URL ?? ''
export const isLaraEnabled = ASSISTANT_URL !== ''

export interface LaraProduct {
  id: string
  slug: string
  name: string
  price: number
  stock: number
  brand?: string | null
  category: string
}

/** Modo da conversa. `vendedor` é o balcão (vê custo); `cliente` é a loja. */
export type LaraMode = 'cliente' | 'vendedor'

/**
 * Produto casado a uma linha da lista de materiais.
 *
 * Espelha o subconjunto do `catalog.Product` (Go) que o cálculo consegue casar.
 * Os campos extras são opcionais porque o backend pode não trazê-los; sem eles
 * o carrinho cai em valores neutros — nunca em preço inventado.
 */
export interface MaterialProduto {
  slug: string
  nome: string
  preco: number
  estoque: number
  id?: string
  icon?: string
  sellerId?: string
  sellerName?: string
}

/**
 * Uma linha da lista de materiais.
 *
 * Espelha `calc.Item` (services/assistant-service/internal/calc/calc.go),
 * já em camelCase. `produto` é opcional: o cálculo dá a QUANTIDADE mesmo quando
 * não existe produto correspondente no catálogo — e nesse caso não há preço.
 */
export interface MaterialItem {
  materialId: string
  nome: string
  /** Consumo na unidade em que a obra consome (kg, m³, un…). */
  quantidade: number
  unidBase: string
  /** Quanto COMPRAR: embalagens inteiras (3,7 sacos são 4). */
  embalagens: number
  unidVenda: string
  /** Memória de cálculo em pt-BR, conferível à mão. */
  memoria: string
  /** Fonte do coeficiente. */
  fonte: string
  coefMin: number
  coefMax: number
  coefUnid: string
  /** Motivo de a faixa ser larga, quando houver. */
  observacao?: string
  produto?: MaterialProduto
}

/**
 * Sugestão de complemento. O backend SEMPRE manda o "por que" — sugestão sem
 * motivo não é renderizada (ver ComplementSuggestion.tsx).
 */
export interface LaraComplemento {
  produto: LaraProduct
  motivo: string
  /** `tecnica` = exigência do serviço; `co-compra` = padrão agregado de pedidos. */
  origem?: 'tecnica' | 'co-compra'
}

export interface LaraResult {
  reply: string
  products: LaraProduct[]
  model: string
  mode: LaraMode
  /**
   * Avisos de SEGURANÇA anexados pelo SERVIDOR (o modelo não os escreve e não
   * consegue suprimi-los). Precisam ser destacados, nunca diluídos no texto.
   */
  avisos?: string[]
  /** A resposta se apoiou em alguma ferramenta? */
  fundamentado?: boolean
  /**
   * ATENÇÃO — CONTRATO PARCIAL: hoje o assistant-service devolve a lista de
   * materiais DENTRO do texto de `reply`, não como campo estruturado do
   * `alice.Result`. Este campo é a antecipação desse contrato: trate sempre
   * como "pode não vir". Quando o backend passar a emitir `materiais`, o front
   * já renderiza a tabela; até lá, só o mock o preenche.
   */
  materiais?: MaterialItem[]
  /** Mesma ressalva de `materiais`: opcional, pode não vir. */
  complementos?: LaraComplemento[]
}

export interface LaraTurn {
  role: 'user' | 'assistant'
  text: string
}

/**
 * Token JWT do usuário. Fonte: o mesmo storage persistido pelo authStore
 * (zustand/persist, chave `utilar-auth`). Lido do localStorage em vez de
 * importar o store para manter esta camada livre de React.
 * É ele que habilita o modo vendedor no backend.
 */
function authToken(): string | null {
  try {
    const raw = localStorage.getItem('utilar-auth')
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return null
    const state = (parsed as { state?: { user?: { token?: unknown } } }).state
    const token = state?.user?.token
    return typeof token === 'string' && token !== '' ? token : null
  } catch {
    return null
  }
}

export async function sendToLara(message: string, history: LaraTurn[]): Promise<LaraResult> {
  if (!isLaraEnabled) {
    return mockReply(message)
  }
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = authToken()
  if (token) headers.Authorization = `Bearer ${token}`

  const res = await fetch(`${ASSISTANT_URL}/api/v1/assistant/chat`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ message, history }),
  })
  if (!res.ok) throw new Error(`alice ${res.status}`)
  const data = (await res.json()) as Partial<LaraResult>
  return {
    reply: data.reply ?? '',
    products: data.products ?? [],
    model: data.model ?? 'desconhecido',
    mode: data.mode === 'vendedor' ? 'vendedor' : 'cliente',
    avisos: data.avisos ?? [],
    fundamentado: data.fundamentado ?? false,
    materiais: data.materiais,
    complementos: data.complementos,
  }
}

// ---------------------------------------------------------------------------
// Mock cliente (sem backend). Determinístico — os testes dependem dele.
// ---------------------------------------------------------------------------

const AVISO_ESTRUTURAL =
  '⚠️ Atenção: isso envolve elemento ESTRUTURAL. Eu consigo explicar e listar material, ' +
  'mas NÃO posso dimensionar — bitola de ferro, seção de viga e profundidade de fundação são ' +
  'atribuição legal de um engenheiro civil ou arquiteto. Procure um profissional antes de executar.'

/** Lista de materiais de exemplo (contrapiso de 20 m²), usada pelo mock. */
export const MOCK_MATERIAIS: MaterialItem[] = [
  {
    materialId: 'cimento-cp2',
    nome: 'Cimento CP-II 50 kg',
    quantidade: 168,
    unidBase: 'kg',
    embalagens: 4,
    unidVenda: 'saco 50 kg',
    memoria: '20 m² × 0,05 m de espessura = 1 m³ × 168 kg/m³ = 168 kg → 4 sacos 50 kg',
    fonte: 'Tabela de traços — NBR 12655 (referência de consumo)',
    coefMin: 150,
    coefMax: 190,
    coefUnid: 'kg/m³',
    observacao: 'A faixa varia com o traço e a umidade da areia.',
    produto: {
      slug: 'cimento-cp2-50kg',
      nome: 'Cimento CP-II-Z 32 50 kg',
      preco: 42.9,
      estoque: 120,
      id: 'prod-cimento',
      sellerId: 'utilar',
      sellerName: 'UtiLar Ferragem',
      icon: '🧱',
    },
  },
  {
    materialId: 'areia-media',
    nome: 'Areia média',
    quantidade: 0.75,
    unidBase: 'm³',
    embalagens: 1,
    unidVenda: 'm³',
    memoria: '20 m² × 0,05 m = 1 m³ × 0,75 m³/m³ de traço = 0,75 m³',
    fonte: 'Tabela de traços — consumo corrente de obra',
    coefMin: 0.7,
    coefMax: 0.85,
    coefUnid: 'm³/m³',
  },
]

const MOCK_COMPLEMENTOS: LaraComplemento[] = [
  {
    produto: {
      id: 'prod-desempenadeira',
      slug: 'desempenadeira-aco',
      name: 'Desempenadeira de aço 12x25 cm',
      price: 34.5,
      stock: 18,
      category: 'ferramentas',
    },
    motivo: 'Contrapiso exige desempenadeira para o acabamento — sem ela a superfície fica irregular.',
    origem: 'tecnica',
  },
]

function mockReply(message: string): LaraResult {
  const q = message.toLowerCase()
  const greeting = /\b(oi|olá|ola|bom dia|boa tarde|boa noite|ajuda)\b/.test(q)
  const calculo = /(calcul|quanto|quantos|quantas|contrapiso|m²|m2|parede|piso|reboco)/.test(q)
  const estrutural = /(viga|pilar|laje|funda[çc][ãa]o|sapata|arrimo|estrutural)/.test(q)

  const avisos = estrutural ? [AVISO_ESTRUTURAL] : []

  if (calculo) {
    return {
      reply:
        'Fiz a conta para 20 m² de contrapiso com 5 cm de espessura. A lista de material está abaixo — ' +
        'abra a memória de cálculo para conferir de onde veio cada número.',
      products: [],
      model: 'mock-client',
      mode: 'cliente',
      avisos,
      fundamentado: true,
      materiais: MOCK_MATERIAIS,
      complementos: MOCK_COMPLEMENTOS,
    }
  }

  const reply = greeting
    ? 'Oi! Eu sou a Alice ✨, sua ajudante aqui da UtiLar Ferragem. Posso achar ferramentas e materiais, comparar preços e estoque, e montar a lista pra sua obra. O que você procura?'
    : 'Posso te ajudar a encontrar ferramentas e materiais. Me diga o que você precisa — por exemplo "furadeira", "cimento" ou uma categoria como elétrica.'
  return {
    reply,
    products: [],
    model: 'mock-client',
    mode: 'cliente',
    avisos,
    fundamentado: false,
  }
}
