import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect, beforeAll, vi } from 'vitest'
import BulkImagesPage from '@/pages/admin/BulkImagesPage'

// happy-dom não implementa object URLs; o componente cria/revoga previews.
beforeAll(() => {
  URL.createObjectURL = vi.fn(() => 'blob:mock')
  URL.revokeObjectURL = vi.fn()
})

function img(name: string) {
  return new File(['x'], name, { type: 'image/jpeg' })
}

describe('BulkImagesPage — solta uma pasta e casa por SKU', () => {
  it('numéricos casam, 2ª foto (-N) cai no SKU base, não-numérico vira "sem produto", não-imagem é ignorada', async () => {
    const { container } = render(
      <MemoryRouter>
        <BulkImagesPage />
      </MemoryRouter>
    )
    const input = container.querySelector('input[type="file"]') as HTMLInputElement

    // Uma "pasta": duas fotos casáveis (uma delas 2ª foto do produto), uma que
    // não casa, e um arquivo que nem é imagem.
    const files = [
      img('6320.jpg'),
      img('6321-2.jpg'), // candidatos [6321-2, 6321] → casa no 6321
      img('semproduto.jpg'), // SKU não-numérico → não casa no mock
      new File(['x'], 'notas.txt', { type: 'text/plain' }), // ignorado
    ]
    fireEvent.change(input, { target: { files } })

    // Espera o casamento (resolveBySku mock) terminar.
    await waitFor(() => expect(screen.getByText('Casados por SKU')).toBeInTheDocument())

    // 6320 e 6321 (do 6321-2) casaram e aparecem na grade.
    expect(screen.getByText('SKU 6320')).toBeInTheDocument()
    expect(screen.getByText('SKU 6321')).toBeInTheDocument()

    // O não-numérico foi para "Sem produto".
    expect(screen.getByText('Sem produto')).toBeInTheDocument()
    expect(screen.getByText('semproduto.jpg')).toBeInTheDocument()

    // O .txt não virou item nenhum (nem casado, nem sem produto).
    expect(screen.queryByText('notas.txt')).not.toBeInTheDocument()

    // Botão de enviar reflete a fila (2 casados pendentes).
    expect(screen.getByRole('button', { name: /Enviar 2 foto/ })).toBeInTheDocument()
  })

  it('arrastar uma PASTA nomeada pelo SKU casa pela pasta (foto com nome qualquer)', async () => {
    render(
      <MemoryRouter>
        <BulkImagesPage />
      </MemoryRouter>
    )
    const zone = screen.getByRole('button', { name: /Arraste imagens/i })

    // Pasta "6320" com uma foto cujo NOME não é o SKU — só a pasta identifica.
    const fileEntry = {
      isFile: true,
      isDirectory: false,
      name: 'IMG_1.jpg',
      file: (ok: (f: File) => void) => ok(new File(['x'], 'IMG_1.jpg', { type: 'image/jpeg' })),
    }
    const dirEntry = {
      isFile: false,
      isDirectory: true,
      name: '6320',
      createReader: () => {
        let served = false
        return {
          readEntries: (ok: (e: unknown[]) => void) => {
            if (served) return ok([])
            served = true
            ok([fileEntry])
          },
        }
      },
    }
    const dataTransfer = { items: [{ webkitGetAsEntry: () => dirEntry }], files: [] }

    fireEvent.drop(zone, { dataTransfer })

    await waitFor(() => expect(screen.getByText('Casados por SKU')).toBeInTheDocument())
    // Casou pela PASTA 6320 — a foto se chama IMG_1, que sozinha não casaria.
    expect(screen.getByText('SKU 6320')).toBeInTheDocument()
  })

  it('CORREÇÃO: pasta e arquivo em produtos diferentes → NÃO sobe no errado (ambíguo)', async () => {
    render(
      <MemoryRouter>
        <BulkImagesPage />
      </MemoryRouter>
    )
    const zone = screen.getByRole('button', { name: /Arraste imagens/i })

    // Pasta "6320" (produto) contendo "300.jpg" (também um produto, no mock).
    // Ambos batem em produtos DIFERENTES → é ambíguo → tem de ficar sem produto.
    const fileEntry = {
      isFile: true,
      isDirectory: false,
      name: '300.jpg',
      file: (ok: (f: File) => void) => ok(new File(['x'], '300.jpg', { type: 'image/jpeg' })),
    }
    const dirEntry = {
      isFile: false,
      isDirectory: true,
      name: '6320',
      createReader: () => {
        let served = false
        return {
          readEntries: (ok: (e: unknown[]) => void) => {
            if (served) return ok([])
            served = true
            ok([fileEntry])
          },
        }
      },
    }
    fireEvent.drop(zone, {
      dataTransfer: { items: [{ webkitGetAsEntry: () => dirEntry }], files: [] },
    })

    // Vai para "Sem produto", NUNCA casa (nem em 6320 nem em 300).
    await waitFor(() => expect(screen.getByText('Sem produto')).toBeInTheDocument())
    expect(screen.getByText('300.jpg')).toBeInTheDocument()
    expect(screen.queryByText('Casados por SKU')).not.toBeInTheDocument()
  })

  it('nenhuma imagem (só .txt) não cria itens', async () => {
    const { container } = render(
      <MemoryRouter>
        <BulkImagesPage />
      </MemoryRouter>
    )
    const input = container.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, {
      target: { files: [new File(['x'], 'a.txt', { type: 'text/plain' })] },
    })
    // Nada renderiza de casados/sem-produto.
    await waitFor(() => {
      expect(screen.queryByText('Casados por SKU')).not.toBeInTheDocument()
      expect(screen.queryByText('Sem produto')).not.toBeInTheDocument()
    })
  })
})
