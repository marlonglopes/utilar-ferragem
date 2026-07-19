import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import AdminImportPage from '@/pages/admin/ImportPage'
import { ImportDiff } from '@/components/admin/import/ImportDiff'
import { ImportResult } from '@/components/admin/import/ImportApprove'
import {
  confidenceSeverity,
  formatPriceDelta,
  priceChangePct,
  priceChangeSeverity,
  summaryCounters,
  validateImportFile,
  MAX_UPLOAD_BYTES,
} from '@/lib/adminImportFormat'
import { parseCsv, parseMoneyBR, suggestColumns } from '@/lib/adminImportMock'
import type { ImportPlan } from '@/lib/adminImportTypes'

/**
 * Cobertura da tela de importação de produtos.
 *
 * O foco não é "o componente renderiza": é que as DEFESAS da tela funcionem —
 * confiança baixa destacada, variação grande de preço destacada, upload
 * inválido recusado com mensagem útil, e o resultado do commit nunca
 * confundido com a previsão do dry-run.
 */

function renderPage(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={['/admin/importar']}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        {ui}
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

/** A planilha-exemplo real, com as linhas problemáticas plantadas. */
const CSV_FORNECEDOR = [
  'CODIGO;DESCRICAO DO PRODUTO;GRUPO;MARCA;UN;VLR VENDA;VLR CUSTO;ESTOQUE;COD_ERP_ANTIGO',
  'FORN-0001;Cimento CP-II-E-32 saco 50kg;construcao;Votoran;saco;R$ 34,90;R$ 27,10;240;',
  'FORN-0002;Argamassa AC-III 20kg;construcao;Quartzolit;saco;28,40;21,90;180;',
  'FORN-0014;Categoria que não existe;departamento-inventado;Marca Y;un;R$ 10,00;R$ 5,00;5;',
  ';Linha sem código;ferramentas;Marca Z;un;R$ 99,00;R$ 70,00;3;',
  'FORN-0017;Cimento CP-V ARI saco 50kg;construcao;Votoran;saco;R$ 1,23;R$ 31,40;80;',
].join('\n')

function csvFile(content = CSV_FORNECEDOR, name = 'exemplo-fornecedor.csv'): File {
  return new File([content], name, { type: 'text/csv' })
}

// ---------------------------------------------------------------------------
// Os quatro passos
// ---------------------------------------------------------------------------

describe('Importação — os quatro passos', () => {
  it('passo 1: mostra a área de envio e o aviso de modo demonstração', () => {
    renderPage(<AdminImportPage />)
    expect(screen.getByRole('heading', { level: 1, name: /importar produtos/i })).toBeInTheDocument()
    expect(screen.getByText(/arraste a planilha aqui/i)).toBeInTheDocument()
    expect(screen.getByText(/\.csv, \.xlsx, \.json/i)).toBeInTheDocument()
    // Sem este aviso alguém acredita ter importado de verdade num ambiente sem backend.
    expect(screen.getByText(/modo demonstração/i)).toBeInTheDocument()
  })

  it('passo 2: detecta as colunas da planilha e sugere o mapeamento', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<AdminImportPage />)
    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    await user.upload(input, csvFile())

    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /conferir o mapeamento/i })).toBeInTheDocument(),
    )
    expect(screen.getByTestId('mapping-row-CODIGO')).toBeInTheDocument()
    expect(screen.getByTestId('mapping-row-VLR VENDA')).toBeInTheDocument()
    // A amostra de dados embaixo da coluna é o que desempata preço × custo.
    expect(screen.getByText('R$ 34,90')).toBeInTheDocument()
    expect(screen.getByText('R$ 27,10')).toBeInTheDocument()
  })

  it('passo 3: simula e mostra os contadores e o número da linha na planilha', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<AdminImportPage />)
    await user.upload(container.querySelector('input[type="file"]') as HTMLInputElement, csvFile())
    await waitFor(() => expect(screen.getByTestId('dryrun-button')).toBeInTheDocument())
    await user.click(screen.getByTestId('dryrun-button'))

    await waitFor(() => expect(screen.getByTestId('counter-creates')).toBeInTheDocument())
    // Nada foi escrito — dito com todas as letras.
    expect(screen.getByText(/isto é uma simulação/i)).toBeInTheDocument()
    // A linha SEM código é rejeitada, e o número é o da planilha (cabeçalho = 1).
    expect(screen.getByTestId('diff-row-5')).toBeInTheDocument()
    expect(screen.getByText(/SKU ausente/i)).toBeInTheDocument()
    // Categoria inexistente também é rejeitada, com o motivo legível.
    expect(screen.getByText(/categoria "departamento-inventado" não existe/i)).toBeInTheDocument()
  })

  it('passo 4: aprova e mostra o RESULTADO real, sempre como rascunho', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<AdminImportPage />)
    await user.upload(container.querySelector('input[type="file"]') as HTMLInputElement, csvFile())
    await waitFor(() => expect(screen.getByTestId('dryrun-button')).toBeInTheDocument())
    await user.click(screen.getByTestId('dryrun-button'))
    await waitFor(() => expect(screen.getByTestId('approve-button')).toBeInTheDocument())

    // O aviso do rascunho aparece ANTES de aprovar, não só depois.
    expect(screen.getAllByText(/entram como rascunho/i).length).toBeGreaterThan(0)

    await user.click(screen.getByTestId('approve-button'))
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /importação aplicada/i })).toBeInTheDocument(),
    )
    expect(screen.getByTestId('result-created')).toBeInTheDocument()
    expect(screen.getByTestId('result-failed')).toBeInTheDocument()
    expect(screen.getAllByText(/como rascunho/i).length).toBeGreaterThan(0)
  })

  it('mostra o histórico de lotes, marcando o que foi conferido e nunca aplicado', async () => {
    renderPage(<AdminImportPage />)
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /histórico de importações/i })).toBeInTheDocument(),
    )
    expect(await screen.findByText('tabela-fornecedor-julho.csv')).toBeInTheDocument()
    // O estado mais traiçoeiro do histórico precisa ser nomeado, não inferido.
    expect(screen.getByText(/conferido, não aplicado/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Erro de upload
// ---------------------------------------------------------------------------

describe('Upload é entrada hostil', () => {
  it('recusa extensão não aceita com mensagem que diz o que fazer', async () => {
    renderPage(<AdminImportPage />)
    const pdf = new File(['%PDF-1.4'], 'catalogo.pdf', { type: 'application/pdf' })
    // Entra por ARRASTAR-E-SOLTAR de propósito: o `accept` do input é filtro de
    // conveniência do navegador e não vale no drop. É o caminho por onde um
    // arquivo errado realmente chega, e é a validação da tela que tem de barrá-lo.
    fireEvent.drop(screen.getByTestId('import-dropzone'), { dataTransfer: { files: [pdf] } })

    const alert = await screen.findByRole('alert')
    expect(within(alert).getByText(/arquivo não aceito/i)).toBeInTheDocument()
    expect(within(alert).getByText(/catalogo\.pdf/)).toBeInTheDocument()
    // Recusou e NÃO avançou de passo.
    expect(screen.queryByRole('heading', { name: /conferir o mapeamento/i })).not.toBeInTheDocument()
  })

  it('valida extensão, arquivo vazio e tamanho antes de gastar rede', () => {
    expect(validateImportFile(new File(['x'], 'a.pdf'))).toMatch(/formato não aceito/i)
    expect(validateImportFile(new File([], 'a.csv'))).toMatch(/vazio/i)
    expect(validateImportFile(new File(['abc'], 'a.csv'))).toBeNull()
    expect(validateImportFile(new File(['abc'], 'A.XLSX'))).toBeNull()

    const huge = new File(['x'], 'grande.csv')
    Object.defineProperty(huge, 'size', { value: MAX_UPLOAD_BYTES + 1 })
    expect(validateImportFile(huge)).toMatch(/excede o limite/i)
  })
})

// ---------------------------------------------------------------------------
// Confiança do mapeamento
// ---------------------------------------------------------------------------

describe('Destaque de confiança baixa', () => {
  it('trata palpite fraco como o caso mais alarmante da tela', () => {
    // `exact` é discreto de propósito: destacar tudo é não destacar nada.
    expect(confidenceSeverity('exact', true)).toBeNull()
    expect(confidenceSeverity('high', true)).toBe('warn')
    expect(confidenceSeverity('low', true)).toBe('critical')
    expect(confidenceSeverity(undefined, false)).toBe('warn')
  })

  it('marca a coluna não reconhecida na tela e a deixa ignorada por padrão', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<AdminImportPage />)
    await user.upload(container.querySelector('input[type="file"]') as HTMLInputElement, csvFile())
    await waitFor(() => expect(screen.getByTestId('mapping-row-COD_ERP_ANTIGO')).toBeInTheDocument())

    const row = screen.getByTestId('mapping-row-COD_ERP_ANTIGO')
    expect(row).toHaveAttribute('data-confidence', 'unrecognized')
    expect(within(row).getByText(/não reconhecida/i)).toBeInTheDocument()
    // Coluna desconhecida NUNCA é chutada num campo — entra como ignorada.
    expect(within(row).getByRole('combobox')).toHaveValue('')
  })

  it('avisa quantas colunas precisam de conferência humana', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<AdminImportPage />)
    await user.upload(container.querySelector('input[type="file"]') as HTMLInputElement, csvFile())
    await waitFor(() => expect(screen.getByText(/precisam de conferência/i)).toBeInTheDocument())
  })

  it('sugere pelo alias e não deixa a ordem das colunas decidir a qualidade', () => {
    const cols = suggestColumns(['CODIGO', 'VLR VENDA', 'VLR CUSTO', 'COD_ERP_ANTIGO'])
    const byName = Object.fromEntries(cols.map((c) => [c.column, c]))
    expect(byName['VLR VENDA'].field).toBe('price')
    expect(byName['VLR CUSTO'].field).toBe('cost')
    expect(byName['COD_ERP_ANTIGO'].recognized).toBe(false)
    // Preço e custo nunca podem cair no mesmo campo.
    expect(byName['VLR VENDA'].field).not.toBe(byName['VLR CUSTO'].field)
  })
})

// ---------------------------------------------------------------------------
// Variação de preço — o motivo de existir a revisão
// ---------------------------------------------------------------------------

describe('Destaque de variação grande de preço', () => {
  it('classifica o erro de vírgula como crítico', () => {
    // 1.234,56 lido como 1,23 — o modo de falha mais caro do catálogo.
    expect(priceChangeSeverity(1234.56, 1.23)).toBe('critical')
    expect(priceChangePct(1234.56, 1.23)).toBeLessThan(-99)
    expect(formatPriceDelta(1234.56, 1.23)).toBe('−99,9%')
  })

  it('gradua queda pequena, queda grande e subida absurda', () => {
    expect(priceChangeSeverity(100, 98)).toBeNull()
    expect(priceChangeSeverity(100, 85)).toBe('warn')
    expect(priceChangeSeverity(100, 60)).toBe('critical')
    expect(priceChangeSeverity(100, 150)).toBe('warn')
    expect(priceChangeSeverity(100, 500)).toBe('critical')
    expect(priceChangeSeverity(100, 100)).toBeNull()
    expect(priceChangeSeverity(undefined, 10)).toBeNull()
    // Preço anterior zero não tem variação relativa definida — não inventa.
    expect(priceChangeSeverity(0, 10)).toBeNull()
  })

  it('mostra de → para com o alerta na linha do diff', () => {
    const plan: ImportPlan = {
      batchId: 'b1',
      status: 'validated',
      dryRun: true,
      summary: { total: 2, creates: 0, updates: 1, skips: 0, reviews: 1, rejects: 0, toArchive: 0 },
      rows: [
        { rowNumber: 7, action: 'update', sku: 'A-1', oldPrice: 1234.56, newPrice: 1.23 },
        { rowNumber: 8, action: 'update', sku: 'A-2', oldPrice: 100, newPrice: 98 },
      ],
    }
    render(<ImportDiff plan={plan} />)

    const alarming = screen.getByTestId('diff-row-7')
    expect(within(alarming).getByTestId('price-alert')).toHaveTextContent('−99,9%')
    expect(within(alarming).getByText('R$ 1.234,56')).toBeInTheDocument()
    expect(within(alarming).getByText('R$ 1,23')).toBeInTheDocument()

    // Variação pequena não recebe alarme — senão o alarme perde o significado.
    expect(within(screen.getByTestId('diff-row-8')).queryByTestId('price-alert')).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// Contadores
// ---------------------------------------------------------------------------

describe('Formatação dos contadores', () => {
  it('nomeia as seis classificações e atribui severidade só quando há o que ver', () => {
    const counters = summaryCounters({
      total: 500,
      creates: 40,
      updates: 400,
      skips: 30,
      reviews: 12,
      rejects: 18,
      toArchive: 0,
    })
    const byKey = Object.fromEntries(counters.map((c) => [c.key, c]))
    expect(counters).toHaveLength(6)
    expect(byKey.creates.severity).toBe('ok')
    expect(byKey.rejects.severity).toBe('critical')
    expect(byKey.reviews.severity).toBe('warn')
    expect(byKey.updates.severity).toBeNull()
    // Arquivar zero não alarma, mas o contador continua visível: ensina que existe.
    expect(byKey.toArchive.severity).toBeNull()
  })

  it('formata os contadores com separador de milhar no diff', () => {
    const plan: ImportPlan = {
      batchId: 'b2',
      status: 'validated',
      dryRun: true,
      summary: { total: 4200, creates: 1234, updates: 2966, skips: 0, reviews: 0, rejects: 0, toArchive: 0 },
      rows: [],
    }
    render(<ImportDiff plan={plan} />)
    expect(within(screen.getByTestId('counter-creates')).getByText('1.234')).toBeInTheDocument()
    expect(within(screen.getByTestId('counter-updates')).getByText('2.966')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Commit parcial
// ---------------------------------------------------------------------------

describe('Commit parcial', () => {
  it('mostra o que entrou e o que não entrou quando há falha no meio', () => {
    render(
      <ImportResult
        batchId="b3"
        plan={null}
        error={null}
        onRestart={() => {}}
        result={{
          created: 30,
          updated: 12,
          skipped: 0,
          archived: 0,
          rejected: 4,
          held: 2,
          failed: 7,
          errors: [{ field: 'price', message: 'linha 91: violação de restrição' }],
        }}
      />,
    )
    expect(screen.getByRole('heading', { name: /aplicada parcialmente/i })).toBeInTheDocument()
    expect(within(screen.getByTestId('result-created')).getByText('30')).toBeInTheDocument()
    expect(within(screen.getByTestId('result-failed')).getByText('7')).toBeInTheDocument()
    expect(screen.getByText(/uma linha por transação/i)).toBeInTheDocument()
    expect(screen.getByText(/violação de restrição/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// Leitura da planilha em modo demonstração
// ---------------------------------------------------------------------------

describe('Leitura da planilha brasileira', () => {
  it('detecta o ponto e vírgula e remove o BOM do Excel', () => {
    const table = parseCsv(`\ufeffA;B\n1;2\n`)
    expect(table.header).toEqual(['A', 'B'])
    expect(table.rows).toEqual([['1', '2']])
  })

  it('não deixa a polegada no nome do produto engolir as colunas seguintes', () => {
    // `3/8"` é o caso COMUM num catálogo de ferragem. Tratando a aspa como
    // delimitador, o resto da linha some e o preço vira "ausente" — uma linha
    // boa rejeitada, que é pior que um erro visível.
    const table = parseCsv('CODIGO;NOME;VLR VENDA\nFORN-3;Vergalhão CA-50 3/8" barra 12m;R$ 47,50')
    expect(table.rows[0]).toEqual(['FORN-3', 'Vergalhão CA-50 3/8" barra 12m', 'R$ 47,50'])
    // Aspas que ABREM o campo continuam valendo como delimitador de verdade.
    expect(parseCsv('A;B\n"x;y";2').rows[0]).toEqual(['x;y', '2'])
  })

  it('converte dinheiro brasileiro e distingue vazio de zero', () => {
    expect(parseMoneyBR('R$ 1.234,56')).toBe(1234.56)
    expect(parseMoneyBR('28,40')).toBe(28.4)
    expect(parseMoneyBR('')).toBeNull()
    expect(parseMoneyBR('-')).toBeNull()
    expect(parseMoneyBR('#REF!')).toBeNull()
    expect(parseMoneyBR('0')).toBe(0)
  })
})
