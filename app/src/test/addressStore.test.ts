import { describe, it, expect, beforeEach } from 'vitest'
import { useAddressStore, type AddressInput } from '@/store/addressStore'

function makeAddr(overrides: Partial<AddressInput> = {}): AddressInput {
  return {
    label: 'Casa',
    cep: '01310-100',
    street: 'Av Paulista',
    number: '1000',
    complement: '',
    neighborhood: 'Bela Vista',
    city: 'São Paulo',
    state: 'SP',
    ...overrides,
  }
}

beforeEach(() => {
  useAddressStore.setState({ addresses: [] })
})

describe('addressStore.addAddress', () => {
  it('adiciona o primeiro endereço e o marca como padrão', () => {
    useAddressStore.getState().addAddress(makeAddr())
    const list = useAddressStore.getState().addresses
    expect(list).toHaveLength(1)
    expect(list[0].isDefault).toBe(true)
  })

  it('não marca o segundo endereço como padrão por omissão', () => {
    useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    useAddressStore.getState().addAddress(makeAddr({ label: 'B' }))
    const list = useAddressStore.getState().addresses
    expect(list).toHaveLength(2)
    expect(list[0].isDefault).toBe(true)
    expect(list[1].isDefault).toBe(false)
  })

  it('respeita makeDefault=true e desmarca os anteriores', () => {
    useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    useAddressStore.getState().addAddress(makeAddr({ label: 'B' }), true)
    const list = useAddressStore.getState().addresses
    expect(list.find((a) => a.label === 'A')?.isDefault).toBe(false)
    expect(list.find((a) => a.label === 'B')?.isDefault).toBe(true)
  })

  it('retorna o endereço criado com id gerado', () => {
    const created = useAddressStore.getState().addAddress(makeAddr())
    expect(created.id).toBeDefined()
    expect(created.isDefault).toBe(true)
  })
})

describe('addressStore.updateAddress', () => {
  it('atualiza os campos sem mudar id nem isDefault', () => {
    const created = useAddressStore.getState().addAddress(makeAddr())
    useAddressStore.getState().updateAddress(created.id, makeAddr({ street: 'Nova Rua' }))
    const found = useAddressStore.getState().addresses.find((a) => a.id === created.id)!
    expect(found.street).toBe('Nova Rua')
    expect(found.isDefault).toBe(true)
  })
})

describe('addressStore.removeAddress', () => {
  it('remove o endereço pelo id', () => {
    const a = useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    useAddressStore.getState().addAddress(makeAddr({ label: 'B' }))
    useAddressStore.getState().removeAddress(a.id)
    expect(useAddressStore.getState().addresses).toHaveLength(1)
    expect(useAddressStore.getState().addresses[0].label).toBe('B')
  })

  it('promove outro endereço a padrão ao remover o padrão atual', () => {
    const a = useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    useAddressStore.getState().addAddress(makeAddr({ label: 'B' }))
    useAddressStore.getState().removeAddress(a.id)
    const list = useAddressStore.getState().addresses
    expect(list).toHaveLength(1)
    expect(list[0].isDefault).toBe(true)
  })

  it('lista vazia após remover o único endereço', () => {
    const a = useAddressStore.getState().addAddress(makeAddr())
    useAddressStore.getState().removeAddress(a.id)
    expect(useAddressStore.getState().addresses).toHaveLength(0)
  })
})

describe('addressStore.setDefault', () => {
  it('marca apenas o id indicado como padrão', () => {
    const a = useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    const b = useAddressStore.getState().addAddress(makeAddr({ label: 'B' }))
    useAddressStore.getState().setDefault(b.id)
    const list = useAddressStore.getState().addresses
    expect(list.find((x) => x.id === a.id)?.isDefault).toBe(false)
    expect(list.find((x) => x.id === b.id)?.isDefault).toBe(true)
  })
})

describe('addressStore.getDefault', () => {
  it('retorna null quando lista vazia', () => {
    expect(useAddressStore.getState().getDefault()).toBeNull()
  })

  it('retorna o endereço marcado como padrão', () => {
    useAddressStore.getState().addAddress(makeAddr({ label: 'A' }))
    const b = useAddressStore.getState().addAddress(makeAddr({ label: 'B' }), true)
    expect(useAddressStore.getState().getDefault()?.id).toBe(b.id)
  })
})
