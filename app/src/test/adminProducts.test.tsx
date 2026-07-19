import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import AdminProductsPage from '@/pages/admin/ProductsPage'
import AdminProductEditPage from '@/pages/admin/ProductEditPage'
import AdminProductNewPage from '@/pages/admin/ProductNewPage'
import { ProductImageManager } from '@/components/admin/products/ProductImageManager'
import { MarginReadout } from '@/components/admin/products/productPrimitives'
import { AdminRoute } from '@/components/admin/AdminRoute'
import { useAuthStore, type User } from '@/store/authStore'
import { resetMockProducts } from '@/lib/adminProductsMock'
import { diffInput, toForm } from '@/lib/adminProductForm'
import {
  byteSavings,
  filterProducts,
  formatBytes,
  hasVariants,
  imageOrderPayload,
  imageSrc,
  isPriceBelowCost,
  marginFraction,
  marginPercent,
  moveImage,
  productMarginSeverity,
  promoteCover,
  readMargin,
  rejectLabel,
  sortProducts,
  unitProfit,
} from '@/lib/adminProductFormat'
import type { AdminProductRow, ProductImageRecord } from '@/lib/adminProductTypes'

/**
 * Cobertura da gestão de produtos no painel.
 *
 * O foco não é "o componente renderiza". É que as DEFESAS funcionem:
 *
 * - a margem está certa, inclusive quando o preço fica abaixo do custo e
 *   quando não há custo cadastrado (que não é margem zero);
 * - a lista filtra de verdade, e ordenar por margem não empurra produto sem
 *   custo para o topo do prejuízo;
 * - o upload que recusou arquivos diz QUAIS e POR QUÊ, casando por código;
 * - reordenar e definir capa produzem a lista que o servidor espera;
 * - custo não vaza para `localStorage` nem para a URL;
 * - o guard de papel continua barrando quem não é admin.
 */

function renderPage(ui: ReactElement, route = '/admin/produtos') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[route]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/admin/produtos" element={ui} />
          <Route path="/admin/produtos/novo" element={<AdminProductNewPage />} />
          <Route path="/admin/produtos/:id" element={<AdminProductEditPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  resetMockProducts()
})

// ===========================================================================
// 1. Margem — o número que evita cadastrar produto no prejuízo
// ===========================================================================

describe('Margem', () => {
  it('calcula a margem sobre o preço, como o backend', () => {
    // (100 - 70) / 100 = 0,30
    expect(marginFraction(100, 70)).toBeCloseTo(0.3, 10)
    expect(marginPercent(100, 70)).toBeCloseTo(30, 10)
    expect(unitProfit(100, 70)).toBeCloseTo(30, 10)
  })

  it('devolve null sem custo cadastrado — ausência de dado NÃO é margem zero', () => {
    // Se isto virasse 0, a tela diria "este produto não dá lucro" sobre um
    // produto do qual simplesmente não se sabe o custo.
    expect(marginFraction(100, null)).toBeNull()
    expect(marginFraction(100, undefined)).toBeNull()
    expect(unitProfit(100, null)).toBeNull()
    expect(readMargin(100, null).unknown).toBe(true)
  })

  it('devolve null com preço zero ou negativo, em vez de dividir por zero', () => {
    expect(marginFraction(0, 10)).toBeNull()
    expect(marginFraction(-5, 10)).toBeNull()
  })

  it('devolve margem NEGATIVA quando o preço fica abaixo do custo', () => {
    // (10 - 25) / 10 = -1,5 → 150% de prejuízo sobre o preço.
    expect(marginFraction(10, 25)).toBeCloseTo(-1.5, 10)
    expect(unitProfit(10, 25)).toBeCloseTo(-15, 10)
  })

  it('classifica a severidade nos cortes de ferragem, com negativo em crítico', () => {
    expect(productMarginSeverity(-0.5)).toBe('critical')
    expect(productMarginSeverity(0)).toBe('critical')
    expect(productMarginSeverity(0.11)).toBe('critical')
    expect(productMarginSeverity(0.12)).toBe('warn')
    expect(productMarginSeverity(0.21)).toBe('warn')
    expect(productMarginSeverity(0.22)).toBe('ok')
    expect(productMarginSeverity(null)).toBeNull()
  })
})

describe('Preço abaixo do custo', () => {
  it('acusa quando o custo supera o preço', () => {
    expect(isPriceBelowCost(10, 25)).toBe(true)
    expect(readMargin(10, 25).belowCost).toBe(true)
  })

  it('NÃO acusa venda exatamente no custo — margem zero é decisão comercial', () => {
    expect(isPriceBelowCost(30, 30)).toBe(false)
  })

  it('tolera meio centavo, como o holdIfPriceBelowCost da importação', () => {
    // A importação não retém diferenças ≤ 0,005; a edição não pode ser mais
    // rígida, senão o mesmo dado passaria por um caminho e travaria no outro.
    expect(isPriceBelowCost(10, 10.004)).toBe(false)
    expect(isPriceBelowCost(10, 10.02)).toBe(true)
  })

  it('não acusa nada quando o custo não existe', () => {
    expect(isPriceBelowCost(10, null)).toBe(false)
    expect(isPriceBelowCost(10, undefined)).toBe(false)
  })

  it('mostra a margem ao vivo e alerta em vermelho abaixo do custo', () => {
    const { rerender } = render(<MarginReadout price={100} cost={70} />)
    expect(screen.getByTestId('margin-value')).toHaveTextContent('30,0%')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()

    rerender(<MarginReadout price={10} cost={25} />)
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/preço abaixo do custo/i)
    // Diz o tamanho do prejuízo por unidade, não só que existe.
    expect(alert).toHaveTextContent(/15,00/)
  })

  it('convida a preencher o custo em vez de exibir 0%', () => {
    render(<MarginReadout price={100} cost={null} />)
    expect(screen.getByTestId('margin-value')).toHaveTextContent('—')
    expect(screen.getByText(/preencha o/i)).toBeInTheDocument()
  })
})

// ===========================================================================
// 2. Lista — filtro, ordenação, render
// ===========================================================================

const ROWS: AdminProductRow[] = [
  {
    id: '1', slug: 'furadeira', sku: 'UTL-1', name: 'Furadeira Bosch', category: 'ferramentas',
    brand: 'Bosch', price: 300, cost: 200, stock: 5, unitOfMeasure: 'un', status: 'published',
  },
  {
    id: '2', slug: 'cimento', sku: 'UTL-2', name: 'Cimento Votoran', category: 'construcao',
    brand: 'Votoran', price: 40, cost: 38, stock: 100, unitOfMeasure: 'sc', status: 'draft',
  },
  {
    id: '3', slug: 'cabo', sku: 'UTL-3', name: 'Cabo Flexível', category: 'eletrica',
    brand: 'Sil', price: 250, cost: null, stock: 60, unitOfMeasure: 'rl', status: 'published',
  },
  {
    id: '4', slug: 'tinta', sku: 'UTL-4', name: 'Tinta Suvinil', category: 'pintura',
    brand: 'Suvinil', price: 100, cost: 130, stock: 8, unitOfMeasure: 'un', status: 'archived',
  },
]

describe('Lista — filtro e ordenação', () => {
  it('filtra por categoria', () => {
    expect(filterProducts(ROWS, '', 'eletrica', '').map((r) => r.id)).toEqual(['3'])
  })

  it('filtra por situação — é como se acha o que a planilha deixou em rascunho', () => {
    expect(filterProducts(ROWS, '', '', 'draft').map((r) => r.id)).toEqual(['2'])
  })

  it('busca por nome, SKU e marca, ignorando acento e caixa', () => {
    expect(filterProducts(ROWS, 'FLEXIVEL', '', '').map((r) => r.id)).toEqual(['3'])
    expect(filterProducts(ROWS, 'utl-4', '', '').map((r) => r.id)).toEqual(['4'])
    expect(filterProducts(ROWS, 'bosch', '', '').map((r) => r.id)).toEqual(['1'])
  })

  it('combina busca com categoria e situação', () => {
    expect(filterProducts(ROWS, 'cimento', 'construcao', 'draft').map((r) => r.id)).toEqual(['2'])
    expect(filterProducts(ROWS, 'cimento', 'eletrica', '')).toEqual([])
  })

  it('ordena por margem e empurra produto SEM custo para o fim, nos dois sentidos', () => {
    // Sem esta regra, o produto sem custo apareceria como o de pior margem da
    // loja — e o dono agiria sobre um problema que não existe.
    const asc = sortProducts(ROWS, 'margin', 'asc').map((r) => r.id)
    const desc = sortProducts(ROWS, 'margin', 'desc').map((r) => r.id)
    expect(asc[asc.length - 1]).toBe('3')
    expect(desc[desc.length - 1]).toBe('3')
    // Pior margem primeiro no ascendente: a tinta (-30%).
    expect(asc[0]).toBe('4')
  })

  it('ordena por preço e por nome', () => {
    expect(sortProducts(ROWS, 'price', 'asc').map((r) => r.id)).toEqual(['2', '4', '3', '1'])
    expect(sortProducts(ROWS, 'name', 'asc')[0].name).toBe('Cabo Flexível')
  })
})

describe('Lista — tela', () => {
  it('renderiza as colunas de custo e margem e avisa que é demonstração', async () => {
    renderPage(<AdminProductsPage />)
    expect(
      await screen.findByRole('heading', { level: 1, name: /produtos/i }),
    ).toBeInTheDocument()
    expect(screen.getByText(/modo demonstração/i)).toBeInTheDocument()

    const table = await screen.findByRole('table')
    expect(within(table).getByRole('columnheader', { name: /custo/i })).toBeInTheDocument()
    expect(within(table).getByRole('columnheader', { name: /margem/i })).toBeInTheDocument()
  })

  it('filtra a tabela pela busca', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />)
    await screen.findByRole('table')

    await user.type(screen.getByLabelText(/buscar/i), 'Cimento')

    await waitFor(() => {
      const links = screen.getAllByRole('link', { name: /cimento/i })
      expect(links.length).toBeGreaterThan(0)
    })
    expect(screen.queryByRole('link', { name: /furadeira/i })).not.toBeInTheDocument()
  })

  it('filtra por situação e mostra só rascunhos', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />)
    await screen.findByRole('table')

    await user.selectOptions(screen.getByLabelText(/situação/i), 'draft')

    await waitFor(() => {
      const body = screen.getByRole('table').querySelector('tbody') as HTMLElement
      const pills = within(body).getAllByText(/rascunho/i)
      expect(pills.length).toBeGreaterThan(0)
      expect(within(body).queryByText('Publicado')).not.toBeInTheDocument()
    })
  })

  it('oferece a ação em lote só depois de selecionar, e publica a seleção', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />)
    await screen.findByRole('table')

    expect(screen.queryByRole('button', { name: /^publicar$/i })).not.toBeInTheDocument()

    // A primeira linha da tabela — não o "selecionar todos" do cabeçalho.
    const rowBoxes = screen.getAllByRole('checkbox', { name: /^selecionar (?!todos)/i })
    await user.click(rowBoxes[0])

    const publish = await screen.findByRole('button', { name: /^publicar$/i })
    await user.click(publish)

    // O resumo é por item: falha parcial é o caso comum quando o lote é feito
    // com um PATCH por produto.
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(/atualizado/i)
    })
  })

  it('a tabela larga rola no próprio contêiner, nunca no body', async () => {
    renderPage(<AdminProductsPage />)
    const table = await screen.findByRole('table')
    const scroller = table.closest('.overflow-x-auto')
    expect(scroller).not.toBeNull()
    expect(scroller?.className).toContain('overscroll-x-contain')
  })
})

// ===========================================================================
// 3. Custo é dado sensível
// ===========================================================================

describe('Custo não vaza', () => {
  it('não escreve nada em localStorage ao listar produtos com custo', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    renderPage(<AdminProductsPage />)
    await screen.findByRole('table')

    // Qualquer gravação com uma chave relacionada a produto/custo é falha.
    const written = setItem.mock.calls.map(([k, v]) => `${String(k)}:${String(v)}`)
    expect(written.some((s) => /cost|custo|margin|margem|produto/i.test(s))).toBe(false)
    setItem.mockRestore()
  })

  it('não coloca valor de custo na URL — ordenar por margem não carrega número', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />)
    const table = await screen.findByRole('table')

    await user.click(within(table).getByRole('button', { name: /margem/i }))

    await waitFor(() => {
      const search = window.location.search + window.location.hash
      expect(search).not.toMatch(/\d+[.,]\d{2}/)
    })
  })
})

// ===========================================================================
// 4. Imagens — variantes, ordem, capa
// ===========================================================================

const own = (id: string, sortOrder: number): ProductImageRecord => ({
  id,
  url: `/media/${id}-large.jpg`,
  alt: `foto ${id}`,
  sortOrder,
  variants: {
    thumb: `/media/${id}-thumb.jpg`,
    medium: `/media/${id}-medium.jpg`,
    large: `/media/${id}-large.jpg`,
  },
  width: 1600,
  height: 1600,
  originalBytes: 4_000_000,
  bytes: 200_000,
})

const external = (id: string, sortOrder: number): ProductImageRecord => ({
  id,
  url: `https://upload.wikimedia.org/${id}.jpg`,
  alt: `externa ${id}`,
  sortOrder,
})

describe('Imagens — imagem externa não pode quebrar', () => {
  it('distingue imagem própria de externa pela presença de variants', () => {
    expect(hasVariants(own('a', 0))).toBe(true)
    expect(hasVariants(external('b', 1))).toBe(false)
  })

  it('escolhe a variante do contexto, e cai na url original quando não há variante', () => {
    expect(imageSrc(own('a', 0), 'thumb')).toBe('/media/a-thumb.jpg')
    expect(imageSrc(own('a', 0), 'large')).toBe('/media/a-large.jpg')
    // Sem variants: usa `url` como está, sem acessar `variants.thumb`.
    expect(imageSrc(external('b', 0), 'thumb')).toBe('https://upload.wikimedia.org/b.jpg')
  })
})

describe('Imagens — reordenação e capa', () => {
  const list = [own('a', 0), own('b', 1), own('c', 2), own('d', 3)]

  it('move uma imagem e renumera o sortOrder sem deixar buraco', () => {
    const moved = moveImage(list, 2, 0)
    expect(moved.map((i) => i.id)).toEqual(['c', 'a', 'b', 'd'])
    expect(moved.map((i) => i.sortOrder)).toEqual([0, 1, 2, 3])
  })

  it('a primeira posição É a capa depois de mover', () => {
    expect(moveImage(list, 3, 0)[0].id).toBe('d')
  })

  it('ignora índice inválido em vez de embaralhar — arrastar cancelado não reordena', () => {
    expect(moveImage(list, 0, 0)).toBe(list)
    expect(moveImage(list, -1, 2)).toBe(list)
    expect(moveImage(list, 1, 99)).toBe(list)
  })

  it('promove a capa preservando a ordem relativa das demais', () => {
    const promoted = promoteCover(list, 'c')
    expect(promoted.map((i) => i.id)).toEqual(['c', 'a', 'b', 'd'])
    // Promover a que já é capa não faz nada.
    expect(promoteCover(list, 'a')).toBe(list)
  })

  it('monta o corpo de PUT .../images/order com a lista completa na ordem', () => {
    expect(imageOrderPayload(promoteCover(list, 'd'))).toEqual(['d', 'a', 'b', 'c'])
  })

  it('reordena pela tela e mantém a capa na primeira posição', async () => {
    const user = userEvent.setup()
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <ProductImageManager productId="1" />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const grid = await screen.findByTestId('image-grid')
    const before = within(grid).getAllByRole('listitem')
    expect(before[0]).toHaveTextContent(/capa/i)

    // Botão explícito de capa na segunda imagem — o caminho que funciona no
    // celular, onde arrastar é impreciso.
    await user.click(screen.getByRole('button', { name: /definir .* como capa/i }))

    await waitFor(() => {
      const after = within(screen.getByTestId('image-grid')).getAllByRole('listitem')
      expect(after[0]).toHaveTextContent(/capa/i)
      expect(after[0].getAttribute('data-testid')).toBe('image-card-img-1b')
    })
  })
})

describe('Imagens — tamanho antes/depois', () => {
  it('formata bytes em unidade legível', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(4_821_994)).toBe('4,6 MB')
    expect(formatBytes(null)).toBe('—')
  })

  it('calcula a economia da normalização', () => {
    expect(byteSavings(4_000_000, 200_000)).toBeCloseTo(0.95, 5)
  })

  it('cala sobre economia quando o arquivo cresceu ou falta o original', () => {
    expect(byteSavings(100_000, 200_000)).toBeNull()
    expect(byteSavings(undefined, 200_000)).toBeNull()
  })

  it('mostra o antes/depois só para imagem própria', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          {/* produto 9 do mock: uma imagem própria + uma externa */}
          <ProductImageManager productId="9" />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const grid = await screen.findByTestId('image-grid')
    expect(within(grid).getByText(/externa/i)).toBeInTheDocument()
    expect(within(grid).getByText(/sem variantes/i)).toBeInTheDocument()
    // A própria mostra a conversão de tamanho; a externa não promete nada.
    expect(within(grid).getByText(/→/)).toBeInTheDocument()
  })
})

// ===========================================================================
// 5. Upload — motivo POR ARQUIVO
// ===========================================================================

describe('Upload — recusa por arquivo', () => {
  it('traduz cada código estável para um motivo acionável', () => {
    expect(rejectLabel('not_an_image')).toMatch(/não é uma imagem/i)
    expect(rejectLabel('file_too_large')).toMatch(/12 MB/)
    expect(rejectLabel('image_too_small')).toMatch(/200 px/)
    expect(rejectLabel('image_too_large')).toMatch(/50 MP|12\.000/)
    expect(rejectLabel('corrupt_image')).toMatch(/corrompido/i)
    expect(rejectLabel('processing_timeout')).toMatch(/tempo limite/i)
    expect(rejectLabel('storage_error')).toMatch(/armazenamento/i)
  })

  it('casa pelo CÓDIGO, nunca pela mensagem do servidor', () => {
    // Mensagem diferente, código igual → mesmo rótulo. É o que impede a tela de
    // quebrar quando o backend reescreve um texto.
    expect(rejectLabel('not_an_image', 'qualquer texto novo do servidor')).toBe(
      rejectLabel('not_an_image', 'outro texto totalmente diferente'),
    )
  })

  it('não some com o arquivo quando o código é desconhecido', () => {
    expect(rejectLabel('codigo_que_nao_existe', 'motivo cru')).toBe('motivo cru')
    expect(rejectLabel(undefined)).toMatch(/não identificado/i)
  })

  it('mostra o nome e o motivo de CADA arquivo recusado, não só um total', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <ProductImageManager productId="2" />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    await screen.findByTestId('image-dropzone')

    const boa = new File(['x'], 'furadeira.jpg', { type: 'image/jpeg' })
    const ruim = new File(['x'], 'planilha.pdf', { type: 'application/pdf' })
    const gigante = new File([new Uint8Array(13 * 1024 * 1024)], 'enorme.jpg', {
      type: 'image/jpeg',
    })

    const input = screen.getByLabelText(/escolher imagens do produto/i)
    fireEvent.change(input, { target: { files: [boa, ruim, gigante] } })

    const panel = await screen.findByTestId('image-rejections')

    // O resumo é honesto: diz quantas entraram E quantas foram recusadas.
    expect(panel).toHaveTextContent(/1 imagem entrou/i)
    expect(panel).toHaveTextContent(/2 foram recusadas/i)

    // E, principalmente, diz QUAIS e POR QUÊ — arquivo por arquivo.
    expect(within(panel).getByText('planilha.pdf')).toBeInTheDocument()
    expect(panel).toHaveTextContent(/não é uma imagem/i)
    expect(within(panel).getByText('enorme.jpg')).toBeInTheDocument()
    expect(panel).toHaveTextContent(/12 MB/)

    // O arquivo bom não aparece na lista de recusas.
    expect(within(panel).queryByText('furadeira.jpg')).not.toBeInTheDocument()
  })

  it('mostra estado vazio útil quando o produto não tem foto', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
          <ProductImageManager productId="2" />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/nenhuma imagem ainda/i)).toBeInTheDocument()
    expect(screen.getByText(/antes de publicar/i)).toBeInTheDocument()
  })
})

// ===========================================================================
// 6. Formulário — PATCH parcial
// ===========================================================================

describe('Formulário — envia só o que mudou', () => {
  const inicial = toForm({
    name: 'Furadeira',
    price: 300,
    cost: 200,
    category: 'ferramentas',
    stock: 5,
    status: 'published',
    ncm: '8467.21.00',
  })

  it('não envia campo nenhum quando nada mudou', () => {
    expect(diffInput(inicial, inicial)).toEqual({})
  })

  it('envia apenas o campo alterado — não reescreve o fiscal ao mexer no preço', () => {
    const out = diffInput(inicial, { ...inicial, price: '350' })
    expect(out).toEqual({ price: 350 })
    expect(out).not.toHaveProperty('ncm')
    expect(out).not.toHaveProperty('status')
  })

  it('trata custo apagado como null (apagar), e não como undefined (não mexer)', () => {
    // É a única forma de a tela desfazer um custo cadastrado por engano.
    expect(diffInput(inicial, { ...inicial, cost: '' })).toEqual({ cost: null })
  })

  it('converte vírgula decimal, que é como se digita preço em pt-BR', () => {
    expect(diffInput(inicial, { ...inicial, price: '349,90' })).toEqual({ price: 349.9 })
  })

  it('serializa a ficha técnica como objeto', () => {
    const out = diffInput(inicial, { ...inicial, specs: '{"Potência":"650 W"}' })
    expect(out.specs).toEqual({ 'Potência': '650 W' })
  })
})

describe('Formulário — edição', () => {
  it('carrega o produto, calcula a margem ao vivo e bloqueia salvar abaixo do custo', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />, '/admin/produtos/1')

    await screen.findByLabelText(/preço de venda/i)

    const price = screen.getByLabelText(/preço de venda/i)
    const cost = screen.getByLabelText(/^custo/i)

    await user.clear(cost)
    await user.type(cost, '100')
    await user.clear(price)
    await user.type(price, '200')

    await waitFor(() => {
      expect(screen.getByTestId('margin-value')).toHaveTextContent('50,0%')
    })

    // Agora inverte: preço abaixo do custo.
    await user.clear(price)
    await user.type(price, '50')

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(/preço abaixo do custo/i)

    // O botão fica desabilitado até a confirmação explícita — a edição manual
    // não pode ser mais permissiva que a importação, que retém esse caso.
    const submit = screen.getByRole('button', { name: /salvar alterações/i })
    expect(submit).toBeDisabled()

    await user.click(screen.getByRole('checkbox', { name: /intencional/i }))
    expect(submit).toBeEnabled()
  })

  it('recusa salvar sem nome e sem preço válido', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />, '/admin/produtos/1')

    const name = await screen.findByLabelText(/^nome/i)
    await user.clear(name)
    await user.click(screen.getByRole('button', { name: /salvar alterações/i }))

    expect(await screen.findByText(/o nome é obrigatório/i)).toBeInTheDocument()
  })

  it('mostra a galeria de imagens junto da edição', async () => {
    renderPage(<AdminProductsPage />, '/admin/produtos/1')
    expect(await screen.findByTestId('image-dropzone')).toBeInTheDocument()
    expect(screen.getByText(/a primeira imagem é a capa/i)).toBeInTheDocument()
  })
})

describe('Formulário — criação', () => {
  it('entra como rascunho e não deixa escolher a situação', async () => {
    renderPage(<AdminProductsPage />, '/admin/produtos/novo')
    expect(
      await screen.findByRole('heading', { level: 1, name: /novo produto/i }),
    ).toBeInTheDocument()
    // O campo existe para dar contexto, mas está travado: produto novo não vai
    // para a vitrine sem alguém publicar de propósito.
    expect(screen.getByLabelText(/situação/i)).toBeDisabled()
    expect(screen.getByRole('button', { name: /criar como rascunho/i })).toBeInTheDocument()
  })

  it('cria o produto e leva para a edição, onde estão as fotos', async () => {
    const user = userEvent.setup()
    renderPage(<AdminProductsPage />, '/admin/produtos/novo')

    await user.type(await screen.findByLabelText(/^nome/i), 'Martelo Unha 27mm')
    await user.type(screen.getByLabelText(/preço de venda/i), '39.90')
    await user.click(screen.getByRole('button', { name: /criar como rascunho/i }))

    // A navegação vai para a edição do item recém-criado.
    expect(await screen.findByTestId('image-dropzone')).toBeInTheDocument()
  })
})

// ===========================================================================
// 7. Guard de papel
// ===========================================================================

function setUser(role: User['role'] | null) {
  useAuthStore.setState({
    user: role === null ? null : { id: 'u1', email: 'a@b.c', name: 'Teste', role, token: 't' },
  })
}

describe('Guard de papel nas rotas de produto', () => {
  afterEach(() => {
    vi.resetModules()
    vi.unstubAllEnvs()
    setUser(null)
  })

  it('em modo mock libera, para a tela ser demonstrável sem backend', () => {
    render(
      <MemoryRouter
        initialEntries={['/admin/produtos']}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/admin/produtos"
            element={
              <AdminRoute>
                <div>gestão de produtos</div>
              </AdminRoute>
            }
          />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('gestão de produtos')).toBeInTheDocument()
  })

  /**
   * Em produção com auth real, só `admin` vê a tela — e isso importa mais aqui
   * do que nas outras telas do painel, porque esta exibe CUSTO. Como
   * `isAuthEnabled` é derivado de `import.meta.env` no carregamento do módulo,
   * o modo restritivo exige remontar o grafo de módulos.
   */
  async function loadStrictGuard(role: User['role'] | null) {
    vi.resetModules()
    vi.stubEnv('DEV', false)
    vi.doMock('@/lib/api', () => ({
      isAuthEnabled: true,
      isApiEnabled: true,
      isOrderEnabled: true,
      isCatalogEnabled: true,
    }))
    const mod = await import('@/components/admin/AdminRoute')
    const { useAuthStore: fresh } = await import('@/store/authStore')
    fresh.setState({
      user: role === null ? null : { id: 'u1', email: 'a@b.c', name: 'Teste', role, token: 't' },
    })
    return mod.AdminRoute
  }

  function renderWith(Guard: typeof AdminRoute) {
    return render(
      <MemoryRouter
        initialEntries={['/admin/produtos']}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/admin/produtos"
            element={
              <Guard>
                <div>gestão de produtos</div>
              </Guard>
            }
          />
          <Route path="/entrar" element={<div>tela de login</div>} />
        </Routes>
      </MemoryRouter>,
    )
  }

  it('manda anônimo para o login', async () => {
    renderWith(await loadStrictGuard(null))
    expect(screen.getByText('tela de login')).toBeInTheDocument()
    expect(screen.queryByText('gestão de produtos')).not.toBeInTheDocument()
  })

  it('barra customer — custo não pode aparecer para cliente', async () => {
    renderWith(await loadStrictGuard('customer'))
    expect(screen.getByRole('heading', { name: /acesso restrito/i })).toBeInTheDocument()
    expect(screen.queryByText('gestão de produtos')).not.toBeInTheDocument()
  })

  it('barra store_operator — operador de balcão não escreve no catálogo', async () => {
    renderWith(await loadStrictGuard('store_operator' as User['role']))
    expect(screen.getByRole('heading', { name: /acesso restrito/i })).toBeInTheDocument()
  })

  it('deixa admin passar', async () => {
    renderWith(await loadStrictGuard('admin'))
    expect(screen.getByText('gestão de produtos')).toBeInTheDocument()
  })
})
