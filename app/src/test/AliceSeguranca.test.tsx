import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { AvisoSeguranca, AvisosSeguranca } from '@/components/assistant/AvisoSeguranca'
import { inferirGravidade } from '@/components/assistant/helpers'
import { AliceMessage, ModoBadge } from '@/components/assistant/AliceMessage'
import { MemoryRouter } from 'react-router-dom'

const AVISO_ESTRUTURAL =
  '⚠️ Atenção: isso envolve elemento ESTRUTURAL. Não posso dimensionar — procure um engenheiro civil.'

const AVISO_ALTURA = 'Trabalho em altura exige cinto e ponto de ancoragem. Use EPI.'

describe('AvisoSeguranca', () => {
  it('renderiza com role="alert" — é o que garante que não passe despercebido', () => {
    render(<AvisoSeguranca aviso={AVISO_ESTRUTURAL} />)
    const alerta = screen.getByRole('alert')
    expect(alerta).toBeInTheDocument()
    expect(alerta).toHaveTextContent(/engenheiro civil/i)
  })

  it('usa gravidade alta (vermelho) para risco estrutural e média (âmbar) para o resto', () => {
    expect(inferirGravidade(AVISO_ESTRUTURAL)).toBe('alta')
    expect(inferirGravidade(AVISO_ALTURA)).toBe('media')

    const { unmount } = render(<AvisoSeguranca aviso={AVISO_ESTRUTURAL} />)
    expect(screen.getByRole('alert')).toHaveAttribute('data-gravidade', 'alta')
    expect(screen.getByRole('alert').className).toMatch(/red/)
    unmount()

    render(<AvisoSeguranca aviso={AVISO_ALTURA} />)
    expect(screen.getByRole('alert')).toHaveAttribute('data-gravidade', 'media')
    expect(screen.getByRole('alert').className).toMatch(/amber/)
  })

  it('renderiza um alerta por aviso e nada quando não há avisos', () => {
    const { unmount } = render(<AvisosSeguranca avisos={[AVISO_ESTRUTURAL, AVISO_ALTURA]} />)
    expect(screen.getAllByRole('alert')).toHaveLength(2)
    unmount()

    const { container } = render(<AvisosSeguranca avisos={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('AliceMessage — avisos no topo', () => {
  function renderMsg(node: React.ReactElement) {
    return render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        {node}
      </MemoryRouter>
    )
  }

  it('o aviso vem ANTES do texto da resposta e não se dilui nele', () => {
    const { container } = renderMsg(
      <AliceMessage
        message={{
          role: 'assistant',
          text: 'A viga tem 4 metros de vão. O material da fôrma é este.',
          avisos: [AVISO_ESTRUTURAL],
        }}
      />
    )

    const alerta = screen.getByRole('alert')
    const texto = screen.getByText(/4 metros de vão/i)

    // O aviso é um elemento próprio, não parte do balão de texto.
    expect(texto).not.toContainElement(alerta)
    // E está posicionado antes dele no DOM.
    const pos = alerta.compareDocumentPosition(texto)
    expect(pos & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(container.querySelectorAll('[role="alert"]')).toHaveLength(1)
  })

  it('mensagem do usuário nunca carrega aviso de segurança', () => {
    renderMsg(<AliceMessage message={{ role: 'user', text: 'e a viga?', avisos: [AVISO_ESTRUTURAL] }} />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})

describe('ModoBadge', () => {
  it('aparece só no modo vendedor', () => {
    const { unmount } = render(<ModoBadge mode="vendedor" />)
    expect(screen.getByText('balcão')).toBeInTheDocument()
    unmount()

    const cliente = render(<ModoBadge mode="cliente" />)
    expect(screen.queryByText('balcão')).not.toBeInTheDocument()
    cliente.unmount()

    const { container } = render(<ModoBadge />)
    expect(container).toBeEmptyDOMElement()
  })
})
