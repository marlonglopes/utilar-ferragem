import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, beforeEach, beforeAll, vi } from 'vitest'

// O canvas real (editFile) é coberto no e2e imageEditor.spec.ts; aqui testamos a
// fiação da UI com editFile mockado.
vi.mock('@/lib/imageEditor', () => ({ editFile: vi.fn() }))

import { editFile } from '@/lib/imageEditor'
import { ImageEditorModal } from '@/components/admin/products/ImageEditorModal'

const mockedEdit = vi.mocked(editFile)

beforeAll(() => {
  URL.createObjectURL = vi.fn(() => 'blob:mock')
  URL.revokeObjectURL = vi.fn()
})
beforeEach(() => mockedEdit.mockReset())

function aFile() {
  return new File(['x'], 'foto.jpg', { type: 'image/jpeg' })
}

describe('ImageEditorModal', () => {
  it('gira à direita e aplica: editFile recebe rotation=90; onApply recebe o editado', async () => {
    const edited = new File(['y'], 'foto.jpg', { type: 'image/jpeg' })
    mockedEdit.mockResolvedValueOnce(edited)
    const onApply = vi.fn()

    render(<ImageEditorModal file={aFile()} open onApply={onApply} onCancel={() => {}} />)

    expect(screen.getByTestId('rotation-label')).toHaveTextContent(/sem rotação/i)
    fireEvent.click(screen.getByRole('button', { name: /girar à direita/i }))
    expect(screen.getByTestId('rotation-label')).toHaveTextContent(/90/)

    fireEvent.click(screen.getByRole('button', { name: /usar imagem/i }))
    await waitFor(() => expect(onApply).toHaveBeenCalledWith(edited))
    // recorte inteiro → crop undefined; rotação 90.
    expect(mockedEdit).toHaveBeenCalledWith(expect.any(File), { rotation: 90, crop: undefined })
  })

  it('duas voltas à esquerda = -180', async () => {
    mockedEdit.mockResolvedValueOnce(aFile())
    render(<ImageEditorModal file={aFile()} open onApply={() => {}} onCancel={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /girar à esquerda/i }))
    fireEvent.click(screen.getByRole('button', { name: /girar à esquerda/i }))
    fireEvent.click(screen.getByRole('button', { name: /usar imagem/i }))
    await waitFor(() =>
      expect(mockedEdit).toHaveBeenCalledWith(expect.any(File), { rotation: -180, crop: undefined })
    )
  })

  it('cancelar chama onCancel e não processa', () => {
    const onCancel = vi.fn()
    render(<ImageEditorModal file={aFile()} open onApply={() => {}} onCancel={onCancel} />)
    fireEvent.click(screen.getByRole('button', { name: /cancelar/i }))
    expect(onCancel).toHaveBeenCalled()
    expect(mockedEdit).not.toHaveBeenCalled()
  })
})
