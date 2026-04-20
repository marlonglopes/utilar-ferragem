export interface ProductImage {
  url: string
  alt: string
}

export interface Product {
  id: string
  name: string
  slug: string
  category: string
  price: number
  originalPrice?: number
  currency: 'BRL'
  imageUrl?: string
  images?: ProductImage[]
  icon: string
  seller: string
  sellerId?: string
  sellerRating?: number
  sellerReviewCount?: number
  stock: number
  rating: number
  reviewCount: number
  cashbackAmount?: number
  badge?: 'discount' | 'free_shipping' | 'last_units'
  badgeLabel?: string
  installments?: number
  description?: string
  specs?: Record<string, string>
}

export interface ProductsParams {
  category?: string
  page?: number
  per_page?: number
  sort?: 'relevance' | 'price_asc' | 'price_desc' | 'newest' | 'top_rated'
  q?: string
}

export interface ProductsResponse {
  data: Product[]
  meta: {
    page: number
    per_page: number
    total: number
    total_pages: number
  }
}
