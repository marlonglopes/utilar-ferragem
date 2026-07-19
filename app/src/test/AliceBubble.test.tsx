import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { AliceBubble } from '@/components/assistant/AliceBubble'

// Sem VITE_ASSISTANT_URL (vite.config zera em teste) → mock cliente da Alice.
beforeAll(() => {})

function renderBubble() {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <AliceBubble />
    </MemoryRouter>
  )
}

describe('AliceBubble', () => {
  it('mostra o botão flutuante e abre o painel', () => {
    renderBubble()
    const open = screen.getByRole('button', { name: /abrir assistente alice/i })
    fireEvent.click(open)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/pergunte à alice/i)).toBeInTheDocument()
  })

  it('responde uma saudação (mock cliente)', async () => {
    renderBubble()
    fireEvent.click(screen.getByRole('button', { name: /abrir assistente alice/i }))
    const input = screen.getByPlaceholderText(/pergunte à alice/i)
    fireEvent.change(input, { target: { value: 'oi' } })
    fireEvent.click(screen.getByRole('button', { name: 'Enviar' }))

    await waitFor(() => {
      expect(screen.getByText(/sou a Alice/i)).toBeInTheDocument()
    })
  })
})
