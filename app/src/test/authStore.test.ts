import { describe, it, expect, beforeEach } from 'vitest'
import { useAuthStore } from '@/store/authStore'
import type { User } from '@/store/authStore'

const mockUser: User = {
  id: 'user-1',
  email: 'joao@example.com',
  name: 'João Silva',
  role: 'customer',
  token: 'jwt-token-abc',
}

beforeEach(() => {
  useAuthStore.setState({ user: null })
})

describe('authStore.setUser / clearUser', () => {
  it('sets user correctly', () => {
    useAuthStore.getState().setUser(mockUser)
    expect(useAuthStore.getState().user).toEqual(mockUser)
  })

  it('clearUser removes the user', () => {
    useAuthStore.getState().setUser(mockUser)
    useAuthStore.getState().clearUser()
    expect(useAuthStore.getState().user).toBeNull()
  })
})

describe('authStore.logout', () => {
  it('logs out and clears user', () => {
    useAuthStore.getState().setUser(mockUser)
    useAuthStore.getState().logout()
    expect(useAuthStore.getState().user).toBeNull()
  })
})

describe('authStore.token', () => {
  it('returns null when not logged in', () => {
    expect(useAuthStore.getState().token()).toBeNull()
  })

  it('returns the JWT when logged in', () => {
    useAuthStore.getState().setUser(mockUser)
    expect(useAuthStore.getState().token()).toBe('jwt-token-abc')
  })
})

describe('authStore.isLoggedIn', () => {
  it('returns false when no user', () => {
    expect(useAuthStore.getState().isLoggedIn()).toBe(false)
  })

  it('returns true when user is set', () => {
    useAuthStore.getState().setUser(mockUser)
    expect(useAuthStore.getState().isLoggedIn()).toBe(true)
  })
})

describe('authStore.isCustomer', () => {
  it('returns false when not logged in', () => {
    expect(useAuthStore.getState().isCustomer()).toBe(false)
  })

  it('returns true for customer role', () => {
    useAuthStore.getState().setUser(mockUser)
    expect(useAuthStore.getState().isCustomer()).toBe(true)
  })

  it('returns false for seller role', () => {
    useAuthStore.getState().setUser({ ...mockUser, role: 'seller' })
    expect(useAuthStore.getState().isCustomer()).toBe(false)
  })

  it('returns false for admin role', () => {
    useAuthStore.getState().setUser({ ...mockUser, role: 'admin' })
    expect(useAuthStore.getState().isCustomer()).toBe(false)
  })
})
