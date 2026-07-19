import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'
import { I18nextProvider } from 'react-i18next'
import { HelmetProvider } from 'react-helmet-async'
import i18n from '@/i18n'
import CategoryPage from '@/pages/category/CategoryPage'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

/** Espelha a URL atual no DOM para podermos afirmar sobre o query string. */
function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname + location.search}</div>
}

function renderCategory(initialUrl = '/categoria/ferramentas') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <HelmetProvider>
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter
            initialEntries={[initialUrl]}
            future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
          >
            <LocationProbe />
            <Routes>
              <Route path="/categoria/:slug" element={<CategoryPage />} />
              <Route path="/404" element={<p>pagina 404</p>} />
            </Routes>
          </MemoryRouter>
        </QueryClientProvider>
      </I18nextProvider>
    </HelmetProvider>
  )
}

describe('CategoryPage', () => {
  it('renderiza o título da categoria e a lista de produtos', async () => {
    renderCategory()

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/ferramentas/i)
    await waitFor(() => {
      expect(screen.getByText(/produtos?$/i)).toBeInTheDocument()
    })
  })

  it('redireciona para /404 quando o slug não existe na taxonomia', () => {
    renderCategory('/categoria/slug-inexistente')
    expect(screen.getByText('pagina 404')).toBeInTheDocument()
  })

  it('mostra o FacetSidebar funcional, sem o grupo de categorias', async () => {
    renderCategory()

    // "Somente em estoque" vem do FacetSidebar real; a antiga FilterSidebar
    // morta não tinha esse controle.
    await waitFor(() => {
      expect(
        screen.getByRole('checkbox', { name: /somente em estoque/i })
      ).toBeInTheDocument()
    })

    // O grupo de categorias fica escondido: na CategoryPage a categoria é a rota.
    expect(screen.queryByText(/todas as categorias/i)).not.toBeInTheDocument()
  })

  it('marcar "somente em estoque" escreve o filtro na URL', async () => {
    renderCategory()

    const checkbox = await screen.findByRole('checkbox', { name: /somente em estoque/i })
    expect(checkbox).not.toBeChecked()

    await userEvent.click(checkbox)

    // O filtro precisa viver na URL para a listagem ser compartilhável —
    // antes ele era um checkbox sem `checked` nem `onChange`, puramente decorativo.
    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent('em_estoque=true')
    })
    expect(checkbox).toBeChecked()
  })

  it('trocar a ordenação escreve `ordem` na URL', async () => {
    renderCategory()

    const sort = screen.getByRole('combobox', { name: /ordenar por/i })
    await userEvent.selectOptions(sort, 'price_asc')

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent('ordem=price_asc')
    })
  })

  it('lê os filtros já presentes na URL ao montar', async () => {
    renderCategory('/categoria/ferramentas?em_estoque=true&ordem=price_desc')

    const checkbox = await screen.findByRole('checkbox', { name: /somente em estoque/i })
    expect(checkbox).toBeChecked()
    expect(screen.getByRole('combobox', { name: /ordenar por/i })).toHaveValue('price_desc')
  })

  it('a marca é single-select de verdade (radio), não checkbox mentiroso', async () => {
    renderCategory()

    // O FacetSidebar só renderiza o grupo de marcas quando as facets carregam.
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: /todas as marcas/i })).toBeInTheDocument()
    })

    const allBrands = screen.getByRole('radio', { name: /todas as marcas/i })
    expect(allBrands).toHaveAttribute('type', 'radio')
    expect(allBrands).toBeChecked()
  })
})
