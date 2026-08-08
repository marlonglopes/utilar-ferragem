import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { AnnouncementBanner } from '@/components/layout/AnnouncementBanner'
import type { StoreSettings } from '@/lib/storeSettingsApi'

// Mocka o hook: o banner é lógica de apresentação pura sobre o settings.
const mockData = vi.fn<() => { data: StoreSettings | undefined }>()
vi.mock('@/hooks/useStoreSettings', () => ({
  useStoreSettings: () => mockData(),
}))

function set(s: Partial<StoreSettings['announcement']> | null) {
  mockData.mockReturnValue({
    data: s ? { announcement: { enabled: false, message: '', level: 'info', ...s } } : undefined,
  })
}

describe('AnnouncementBanner', () => {
  beforeEach(() => mockData.mockReset())

  it('não renderiza nada quando desligado', () => {
    set({ enabled: false, message: 'oi' })
    const { container } = render(<AnnouncementBanner />)
    expect(container).toBeEmptyDOMElement()
  })

  // Regressão: ligado com mensagem só de espaço não pode virar barra em branco.
  it('não renderiza com mensagem vazia mesmo ligado', () => {
    set({ enabled: true, message: '   ' })
    const { container } = render(<AnnouncementBanner />)
    expect(container).toBeEmptyDOMElement()
  })

  it('mostra a mensagem quando ligado', () => {
    set({ enabled: true, message: 'Fechados no feriado', level: 'warning' })
    render(<AnnouncementBanner />)
    expect(screen.getByText(/fechados no feriado/i)).toBeInTheDocument()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('não quebra quando o dado ainda não chegou', () => {
    set(null)
    const { container } = render(<AnnouncementBanner />)
    expect(container).toBeEmptyDOMElement()
  })
})
