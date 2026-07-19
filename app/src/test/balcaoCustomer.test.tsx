import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { CustomerBlock } from '@/components/balcao/CustomerBlock'
import type { BalcaoCustomer } from '@/store/balcaoStore'

/**
 * Identificação do cliente no balcão.
 *
 * O contrato do auth-service é "um ou nenhum": documento é chave exata e um
 * desconhecido devolve 404, não lista vazia. Então 404 NÃO é erro na UI — é o
 * caminho normal para abrir o cadastro rápido. É isso que estes testes travam,
 * junto com a obrigatoriedade do telefone (a Appmax recusa a cobrança sem o
 * celular do pagador, 403 confirmado em sandbox).
 *
 * Em modo mock o hook consulta um diretório local; `MOCK_DOC` existe lá,
 * qualquer outro documento cai no caminho "não encontrado".
 */

const MOCK_DOC = '12345678909'
/** CPF válido em formato, ausente do diretório de demonstração. */
const UNKNOWN_DOC = '52998224725'

function setup(customer: BalcaoCustomer | null = null) {
  const onChange = vi.fn()
  render(<CustomerBlock customer={customer} onChange={onChange} />)
  return { onChange, user: userEvent.setup() }
}

async function openSearch(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: /identificar cliente/i }))
  return screen.getByLabelText(/cpf \/ cnpj/i)
}

describe('CustomerBlock — busca por documento', () => {
  it('o botão principal abre a BUSCA, não o cadastro', async () => {
    const { user } = setup()
    await openSearch(user)

    expect(screen.getByRole('heading', { name: /buscar cliente/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^buscar$/i })).toBeInTheDocument()
    // O cadastro ainda não apareceu: nome/telefone só depois da consulta.
    expect(screen.queryByLabelText(/^nome$/i)).not.toBeInTheDocument()
  })

  it('documento encontrado seleciona o cliente sem redigitar nada', async () => {
    const { user, onChange } = setup()
    const input = await openSearch(user)

    await user.type(input, MOCK_DOC)
    await user.click(screen.getByRole('button', { name: /^buscar$/i }))

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith(
        expect.objectContaining({ document: MOCK_DOC, id: expect.any(String) }),
      )
    })
    // Cliente vindo da base tem `id` — é ele que vai como `customerId` no pedido.
    expect(onChange.mock.calls[0][0].id).toBeTruthy()
  })

  it('404 (não encontrado) abre o cadastro rápido com o documento preenchido', async () => {
    const { user, onChange } = setup()
    const input = await openSearch(user)

    await user.type(input, UNKNOWN_DOC)
    await user.click(screen.getByRole('button', { name: /^buscar$/i }))

    expect(await screen.findByText(/cliente não encontrado/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /cadastro rápido/i })).toBeInTheDocument()
    // Documento preservado: o vendedor não redigita o que acabou de ler.
    expect(screen.getByLabelText(/cpf \/ cnpj/i)).toHaveValue('529.982.247-25')
    // Não encontrar não é selecionar ninguém.
    expect(onChange).not.toHaveBeenCalled()
  })

  it('recusa documento com tamanho inválido antes de consultar', async () => {
    const { user, onChange } = setup()
    const input = await openSearch(user)

    await user.type(input, '123')
    await user.click(screen.getByRole('button', { name: /^buscar$/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/cpf \(11 dígitos\) ou cnpj/i)
    expect(onChange).not.toHaveBeenCalled()
  })
})

describe('CustomerBlock — cadastro rápido', () => {
  async function reachForm(user: ReturnType<typeof userEvent.setup>, doc: string) {
    const input = await openSearch(user)
    await user.type(input, doc)
    await user.click(screen.getByRole('button', { name: /^buscar$/i }))
    await screen.findByRole('heading', { name: /cadastro rápido/i })
  }

  it('exige telefone — a operadora recusa a cobrança sem o celular do pagador', async () => {
    const { user, onChange } = setup()
    await reachForm(user, UNKNOWN_DOC)

    await user.type(screen.getByLabelText(/^nome$/i), 'Obra do Zé')
    await user.click(screen.getByRole('button', { name: /salvar cliente/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/telefone é obrigatório/i)
    expect(onChange).not.toHaveBeenCalled()
  })

  it('exige nome', async () => {
    const { user, onChange } = setup()
    await reachForm(user, UNKNOWN_DOC)

    await user.type(screen.getByLabelText(/telefone/i), '11988887777')
    await user.click(screen.getByRole('button', { name: /salvar cliente/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/informe o nome/i)
    expect(onChange).not.toHaveBeenCalled()
  })

  it('cadastra e seleciona o cliente com documento e telefone só em dígitos', async () => {
    const { user, onChange } = setup()
    // Documento próprio deste caso: o diretório de demonstração é mutável e um
    // cadastro de outro teste tornaria este "encontrado" em vez de novo.
    await reachForm(user, '11144477735')

    await user.type(screen.getByLabelText(/^nome$/i), 'Construtora Aurora')
    await user.type(screen.getByLabelText(/telefone/i), '11988887777')
    await user.click(screen.getByRole('button', { name: /salvar cliente/i }))

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'Construtora Aurora',
          document: '11144477735',
          phone: '11988887777',
        }),
      )
    })
  })

  it('dá para pular a busca e cadastrar direto', async () => {
    const { user } = setup()
    await openSearch(user)

    await user.click(screen.getByRole('button', { name: /cadastrar sem buscar/i }))
    expect(screen.getByRole('heading', { name: /cadastro rápido/i })).toBeInTheDocument()
  })
})
