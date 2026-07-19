import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeAll } from 'vitest'
import { I18nextProvider } from 'react-i18next'
import i18n from '@/i18n'
import { Input, Select, Checkbox, Modal, Drawer } from '@/components/ui'

beforeAll(async () => {
  await i18n.changeLanguage('pt-BR')
})

function wrapper({ children }: { children: React.ReactNode }) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
}

describe('Input — acessibilidade', () => {
  it('associa label e input mesmo sem id explícito', () => {
    render(<Input label="E-mail" />)
    expect(screen.getByLabelText('E-mail')).toBeInTheDocument()
  })

  it('gera ids únicos para dois campos com o mesmo label', () => {
    render(
      <>
        <Input label="E-mail" />
        <Input label="E-mail" />
      </>
    )
    const [a, b] = screen.getAllByLabelText('E-mail')
    expect(a.id).toBeTruthy()
    expect(b.id).toBeTruthy()
    // Antes do useId, ambos viravam id="e-mail" e o label apontava para o primeiro.
    expect(a.id).not.toBe(b.id)
  })

  it('marca aria-invalid e liga a mensagem de erro via aria-describedby', () => {
    render(<Input label="CPF" error="CPF inválido." />)
    const input = screen.getByLabelText('CPF')

    expect(input).toHaveAttribute('aria-invalid', 'true')

    const describedBy = input.getAttribute('aria-describedby')
    expect(describedBy).toBeTruthy()
    expect(document.getElementById(describedBy!)).toHaveTextContent('CPF inválido.')
    expect(screen.getByRole('alert')).toHaveTextContent('CPF inválido.')
  })

  it('não marca aria-invalid quando não há erro e expõe a dica', () => {
    render(<Input label="Senha" hint="Mínimo de 10 caracteres." />)
    const input = screen.getByLabelText('Senha')

    expect(input).not.toHaveAttribute('aria-invalid')
    const describedBy = input.getAttribute('aria-describedby')
    expect(document.getElementById(describedBy!)).toHaveTextContent('Mínimo de 10 caracteres.')
  })
})

describe('Select — acessibilidade', () => {
  const options = [
    { value: 'sp', label: 'São Paulo' },
    { value: 'rj', label: 'Rio de Janeiro' },
  ]

  it('associa label e expõe erro para leitor de tela', () => {
    render(<Select label="Estado" options={options} error="Selecione um estado." />)
    const select = screen.getByLabelText('Estado')

    expect(select).toHaveAttribute('aria-invalid', 'true')
    const describedBy = select.getAttribute('aria-describedby')
    expect(document.getElementById(describedBy!)).toHaveTextContent('Selecione um estado.')
  })

  it('gera ids únicos para selects homônimos', () => {
    render(
      <>
        <Select label="Estado" options={options} />
        <Select label="Estado" options={options} />
      </>
    )
    const [a, b] = screen.getAllByLabelText('Estado')
    expect(a.id).not.toBe(b.id)
  })
})

describe('Checkbox — acessibilidade', () => {
  it('associa label e expõe erro', () => {
    render(<Checkbox label="Aceito os termos" error="Você precisa aceitar." />)
    const box = screen.getByLabelText('Aceito os termos')

    expect(box).toHaveAttribute('aria-invalid', 'true')
    const describedBy = box.getAttribute('aria-describedby')
    expect(document.getElementById(describedBy!)).toHaveTextContent('Você precisa aceitar.')
  })

  it('gera ids únicos para checkboxes homônimos', () => {
    render(
      <>
        <Checkbox label="Aceito" />
        <Checkbox label="Aceito" />
      </>
    )
    const [a, b] = screen.getAllByLabelText('Aceito')
    expect(a.id).not.toBe(b.id)
  })
})

describe('Modal — foco e rotulagem', () => {
  it('rotula o diálogo com um id único por instância', () => {
    render(
      <Modal open onClose={() => {}} title="Confirmar pedido">
        <p>corpo</p>
      </Modal>,
      { wrapper }
    )

    const dialog = screen.getByRole('dialog')
    const labelledBy = dialog.getAttribute('aria-labelledby')
    expect(labelledBy).toBeTruthy()
    // O id era a string fixa "modal-title" — com dois modais, colidia.
    expect(labelledBy).not.toBe('modal-title')
    expect(document.getElementById(labelledBy!)).toHaveTextContent('Confirmar pedido')
  })

  it('move o foco para dentro do diálogo ao abrir', () => {
    render(
      <Modal open onClose={() => {}} title="Endereço">
        <button>Salvar</button>
      </Modal>,
      { wrapper }
    )

    const dialog = screen.getByRole('dialog')
    expect(dialog.contains(document.activeElement)).toBe(true)
  })

  it('devolve o foco ao elemento que abriu quando fecha', async () => {
    function Harness() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button onClick={() => setOpen(true)}>Abrir modal</button>
          <Modal open={open} onClose={() => setOpen(false)} title="Teste">
            <button onClick={() => setOpen(false)}>Fechar conteúdo</button>
          </Modal>
        </>
      )
    }

    render(<Harness />, { wrapper })

    const trigger = screen.getByRole('button', { name: 'Abrir modal' })
    await userEvent.click(trigger)
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Fechar conteúdo' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(document.activeElement).toBe(trigger)
  })

  it('fecha com Escape', async () => {
    let closed = false
    render(
      <Modal open onClose={() => { closed = true }} title="Teste">
        <p>corpo</p>
      </Modal>,
      { wrapper }
    )
    await userEvent.keyboard('{Escape}')
    expect(closed).toBe(true)
  })
})

describe('Drawer — foco e rotulagem', () => {
  it('rotula o painel e move o foco para dentro', () => {
    render(
      <Drawer open onClose={() => {}} title="Filtros">
        <button>Aplicar</button>
      </Drawer>,
      { wrapper }
    )

    const dialog = screen.getByRole('dialog')
    const labelledBy = dialog.getAttribute('aria-labelledby')
    expect(document.getElementById(labelledBy!)).toHaveTextContent('Filtros')
    expect(dialog.contains(document.activeElement)).toBe(true)
  })
})
