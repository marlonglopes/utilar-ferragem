// Configuração da loja — aviso da vitrine editável pelo dono.
//
// Leitura PÚBLICA (a home lê pra desenhar o banner) via catalogGet; escrita
// ADMIN via adminSend. Mock quando VITE_CATALOG_URL vazio.
import { catalogGet, isCatalogEnabled } from '@/lib/api'
import { adminSend } from '@/lib/adminApi'

const CATALOG_URL = import.meta.env.VITE_CATALOG_URL ?? ''

export type AnnouncementLevel = 'info' | 'warning' | 'success'

export interface Announcement {
  enabled: boolean
  message: string
  level: AnnouncementLevel
}

export interface StoreSettings {
  announcement: Announcement
}

const DEFAULT: StoreSettings = {
  announcement: { enabled: false, message: '', level: 'info' },
}

// Lida pela vitrine (pública). Nunca lança pro chamador quebrar o layout: em
// erro/mock devolve o default desligado — um soluço no aviso não pode derrubar a
// home.
export async function fetchStoreSettings(): Promise<StoreSettings> {
  if (!isCatalogEnabled) return DEFAULT
  try {
    return await catalogGet<StoreSettings>('/api/v1/store/settings')
  } catch {
    return DEFAULT
  }
}

// Escrita admin. Em mock devolve o próprio valor (a tela reflete a mudança sem
// backend, pra demonstração).
export async function updateAnnouncement(a: Announcement): Promise<StoreSettings> {
  if (!isCatalogEnabled) return { announcement: a }
  return adminSend<StoreSettings>(CATALOG_URL, '/api/v1/admin/store/settings', 'PUT', a)
}

export const isStoreSettingsAdminEnabled = CATALOG_URL !== ''
