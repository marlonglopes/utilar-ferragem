import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeAll } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { LaraBubble } from '@/components/assistant/LaraBubble'

// Sem VITE_ASSISTANT_URL (vite.config zera em teste) → mock cliente da Lara.
beforeAll(() => {})

function renderBubble() {
  return render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <LaraBubble />
    </MemoryRouter>
  )
}

describe('LaraBubble', () => {
  it('mostra o botão flutuante e abre o painel', () => {
    renderBubble()
    const open = screen.getByRole('button', { name: /abrir assistente lara/i })
    fireEvent.click(open)
    expect(screen.getByText('Lara')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/pergunte à lara/i)).toBeInTheDocument()
  })

  it('responde uma saudação (mock cliente)', async () => {
    renderBubble()
    fireEvent.click(screen.getByRole('button', { name: /abrir assistente lara/i }))
    const input = screen.getByPlaceholderText(/pergunte à lara/i)
    fireEvent.change(input, { target: { value: 'oi' } })
    fireEvent.click(screen.getByRole('button', { name: 'Enviar' }))

    await waitFor(() => {
      expect(screen.getByText(/sou a Lara/i)).toBeInTheDocument()
    })
  })
})
