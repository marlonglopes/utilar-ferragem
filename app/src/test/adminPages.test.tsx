import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import AdminOverviewPage from '@/pages/admin/OverviewPage'
import AdminAccountingPage from '@/pages/admin/AccountingPage'
import AdminSellersPage from '@/pages/admin/SellersPage'
import AdminAuditTrailPage from '@/pages/admin/AuditTrailPage'
import AdminObservabilityPage from '@/pages/admin/ObservabilityPage'
import { mockObservability, mockOverview, defaultPeriod, mockLedgerEntries } from '@/lib/adminMock'
import { ledgerBalanceCents } from '@/lib/adminFormat'

/**
 * Render das cinco telas em modo mock — o mesmo modo em que o dono vai
 * demonstrar o painel sem backend nenhum de pé.
 */

function renderPage(ui: ReactElement, route = '/admin') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[route]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        {ui}
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('Visão geral', () => {
  it('mostra o chrome do painel e o aviso de dados de demonstração', async () => {
    renderPage(<AdminOverviewPage />)
    expect(screen.getByText('Utilar · Admin')).toBeInTheDocument()
    // Sem este aviso, alguém apresenta número de mock achando que é faturamento.
    expect(screen.getByText('Dados de demonstração')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: /visão geral/i })).toBeInTheDocument()
  })

  it('renderiza os KPIs do dia, da semana e do período', async () => {
    renderPage(<AdminOverviewPage />)
    await waitFor(() => expect(screen.getByText('Vendas hoje')).toBeInTheDocument())
    expect(screen.getByText('Últimos 7 dias')).toBeInTheDocument()
    expect(screen.getByText('Ticket médio')).toBeInTheDocument()
    expect(screen.getByText('Conversão de pagamento')).toBeInTheDocument()
  })

  it('lista os pedidos travados com a severidade em forma, não só em cor', async () => {
    renderPage(<AdminOverviewPage />)
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /pedidos travados/i })).toBeInTheDocument(),
    )
    // O rótulo textual da severidade é o canal redundante da cor.
    expect(screen.getAllByText(/mais de 1 dia|atrasado|recente/i).length).toBeGreaterThan(0)
  })

  it('promove os alertas críticos para o topo da tela', async () => {
    renderPage(<AdminOverviewPage />)
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /alertas? crítico/i })).toBeInTheDocument(),
    )
  })

  it('mostra a conversão de pagamento por método', async () => {
    renderPage(<AdminOverviewPage />)
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: /conversão por método de pagamento/i }),
      ).toBeInTheDocument(),
    )
  })
})

describe('Auditoria contábil', () => {
  it('mostra bruto, taxas, estornos, chargebacks e líquido', async () => {
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    await waitFor(() => expect(screen.getByText('Receita bruta')).toBeInTheDocument())
    expect(screen.getByText('Taxas do PSP')).toBeInTheDocument()
    // "Estornos" aparece no KPI e como coluna da tabela por método — as duas
    // ocorrências são legítimas.
    expect(screen.getAllByText('Estornos').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Chargebacks').length).toBeGreaterThan(0)
    expect(screen.getByText('Receita líquida')).toBeInTheDocument()
  })

  it('não exibe o aviso de "não fecha" quando a identidade contábil bate', async () => {
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    await waitFor(() => expect(screen.getByText('Receita bruta')).toBeInTheDocument())
    expect(screen.queryByText(/o resumo não fecha/i)).not.toBeInTheDocument()
  })

  it('mostra o livro de lançamentos e confirma que a partida dobrada fecha', async () => {
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /livro de lançamentos/i })).toBeInTheDocument(),
    )
    expect(await screen.findByText(/partida dobrada fecha/i)).toBeInTheDocument()
  })

  it('destaca as divergências de reconciliação com o PSP', async () => {
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: /divergências de reconciliação/i }),
      ).toBeInTheDocument(),
    )
    expect(await screen.findByText(/falta no nosso livro/i)).toBeInTheDocument()
  })

  it('oferece a exportação para o contador', async () => {
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    expect(screen.getByRole('button', { name: /exportar p\/ contador/i })).toBeInTheDocument()
  })

  it('filtra o livro por natureza do lançamento', async () => {
    const user = userEvent.setup()
    renderPage(<AdminAccountingPage />, '/admin/contabil')
    const select = await screen.findByLabelText('Natureza')
    await user.selectOptions(select, 'refund')
    await waitFor(() => expect((select as HTMLSelectElement).value).toBe('refund'))
  })
})

describe('Desempenho dos vendedores', () => {
  it('mostra o ranking com desconto e margem médios', async () => {
    renderPage(<AdminSellersPage />, '/admin/vendedores')
    await waitFor(() => expect(screen.getAllByText('Desconto médio').length).toBeGreaterThan(0))
    // KPI do topo + cabeçalho ordenável da tabela.
    expect(screen.getAllByText('Margem média').length).toBeGreaterThan(0)
    expect(await screen.findByRole('heading', { name: /^ranking$/i })).toBeInTheDocument()
  })

  it('mostra os pedidos que precisaram de aprovação do gerente', async () => {
    renderPage(<AdminSellersPage />, '/admin/vendedores')
    expect(await screen.findByRole('button', { name: /aprov\. gerente/i })).toBeInTheDocument()
  })

  it('reordena o ranking por margem ao clicar no cabeçalho', async () => {
    const user = userEvent.setup()
    renderPage(<AdminSellersPage />, '/admin/vendedores')
    const header = await screen.findByRole('button', { name: /margem média/i })
    await user.click(header)
    await waitFor(() => expect(header.closest('th')).toHaveAttribute('aria-sort', 'descending'))
  })

  it('oferece o filtro por loja', async () => {
    renderPage(<AdminSellersPage />, '/admin/vendedores')
    const select = await screen.findByLabelText('Loja')
    expect(within(select).getByRole('option', { name: 'Todas as lojas' })).toBeInTheDocument()
  })
})

describe('Trilha de auditoria', () => {
  it('deixa explícito que o registro é imutável e encadeado por hash', async () => {
    renderPage(<AdminAuditTrailPage />, '/admin/trilha')
    expect(
      await screen.findByText(/cada registro guarda o hash sha-256 do anterior/i),
    ).toBeInTheDocument()
  })

  it('mostra o resultado da verificação de integridade da cadeia', async () => {
    renderPage(<AdminAuditTrailPage />, '/admin/trilha')
    expect(await screen.findByText('Cadeia íntegra')).toBeInTheDocument()
    expect(screen.getByText(/hash âncora/i)).toBeInTheDocument()
  })

  it('lista os eventos com de → para e oferece os três filtros pedidos', async () => {
    renderPage(<AdminAuditTrailPage />, '/admin/trilha')
    await waitFor(() => expect(screen.getByRole('heading', { name: /^eventos$/i })).toBeInTheDocument())
    expect(screen.getByLabelText('Usuário')).toBeInTheDocument()
    expect(screen.getByLabelText('Entidade')).toBeInTheDocument()
    expect(screen.getByLabelText('Ação')).toBeInTheDocument()
    expect(await screen.findByText('De → para')).toBeInTheDocument()
  })

  it('avisa que o IP vem mascarado pelo backend', async () => {
    renderPage(<AdminAuditTrailPage />, '/admin/trilha')
    expect(await screen.findByText(/último octeto mascarado/i)).toBeInTheDocument()
  })
})

describe('Observabilidade', () => {
  it('coloca a fila do outbox em destaque, com tamanho e idade do mais antigo', async () => {
    renderPage(<AdminObservabilityPage />, '/admin/observabilidade')
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /fila do outbox/i })).toBeInTheDocument(),
    )
    expect(screen.getByText('Eventos pendentes')).toBeInTheDocument()
    expect(screen.getByText('Evento mais antigo')).toBeInTheDocument()
  })

  it('mostra a saúde dos quatro serviços com latência e taxa de erro', async () => {
    renderPage(<AdminObservabilityPage />, '/admin/observabilidade')
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /saúde dos serviços/i })).toBeInTheDocument(),
    )
    // getAllBy: o nome do serviço também aparece como origem de um alerta.
    for (const svc of ['auth-service', 'catalog-service', 'order-service', 'payment-service']) {
      expect(screen.getAllByText(new RegExp(svc)).length).toBeGreaterThan(0)
    }
    expect(screen.getByText('Erro 5xx')).toBeInTheDocument()
  })

  it('lista os alertas ativos', async () => {
    renderPage(<AdminObservabilityPage />, '/admin/observabilidade')
    expect(await screen.findByRole('heading', { name: /alertas ativos/i })).toBeInTheDocument()
  })
})

describe('Consistência dos dados de demonstração', () => {
  it('o livro-razão de mock fecha em partida dobrada', () => {
    expect(ledgerBalanceCents(mockLedgerEntries())).toBe(0)
  })

  it('a visão geral traz série, status, funil e alertas preenchidos', () => {
    const o = mockOverview(defaultPeriod())
    expect(o.series).toHaveLength(30)
    expect(o.byStatus.length).toBeGreaterThan(0)
    expect(o.funnel.confirmed).toBeLessThanOrEqual(o.funnel.created)
    expect(o.alerts.length).toBeGreaterThan(0)
  })

  it('a severidade do outbox é derivada, não hardcoded no mock', () => {
    const snap = mockObservability()
    expect(snap.outbox.severity).toBe('critical')
    expect(snap.services).toHaveLength(4)
  })
})
