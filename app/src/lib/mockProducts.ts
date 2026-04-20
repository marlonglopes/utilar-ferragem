import type { Product, ProductsParams, ProductsResponse } from '@/types/product'

export const MOCK_PRODUCTS: Product[] = [
  // ferramentas
  { id: '1', name: 'Furadeira de Impacto Bosch GSB 13 RE 650W 127V', slug: 'furadeira-bosch-gsb-13-re', category: 'ferramentas', price: 329, originalPrice: 389, currency: 'BRL', icon: '⚒', seller: 'Ferragem Silva', stock: 42, rating: 5, reviewCount: 142, cashbackAmount: 24.9, badge: 'discount', badgeLabel: '-15%', installments: 12 },
  { id: '2', name: 'Parafusadeira Makita DF333DWYE 12V Bivolt', slug: 'parafusadeira-makita-df333', category: 'ferramentas', price: 459, currency: 'BRL', icon: '⚒', seller: 'Ferragem Silva', stock: 18, rating: 5, reviewCount: 201, installments: 12 },
  { id: '3', name: 'Furadeira de Impacto Bosch GSB 16 RE 750W Bivolt', slug: 'furadeira-bosch-gsb-16-re', category: 'ferramentas', price: 419, currency: 'BRL', icon: '⚒', seller: 'Pro Tools BR', stock: 27, rating: 5, reviewCount: 214, badge: 'free_shipping', badgeLabel: 'Frete grátis' },
  { id: '4', name: 'Martelete Bosch GBH 2-24 D SDS Plus 790W Bivolt', slug: 'martelete-bosch-gbh-2-24', category: 'ferramentas', price: 1089, currency: 'BRL', icon: '⚒', seller: 'Ferragem Silva', stock: 9, rating: 5, reviewCount: 87, cashbackAmount: 32.67, installments: 12 },
  { id: '5', name: 'Esmerilhadeira Bosch GWS 700 4.1/2" 127V', slug: 'esmerilhadeira-bosch-gws-700', category: 'ferramentas', price: 289, currency: 'BRL', icon: '⚒', seller: 'Casa & Obra', stock: 3, rating: 4, reviewCount: 63, badge: 'last_units', badgeLabel: 'Últimas 3' },
  { id: '6', name: 'Lixadeira Orbital Bosch GSS 140 180W Bivolt', slug: 'lixadeira-bosch-gss-140', category: 'ferramentas', price: 499, currency: 'BRL', icon: '⚒', seller: 'Pro Tools BR', stock: 15, rating: 4, reviewCount: 54, cashbackAmount: 24.95 },
  { id: '7', name: 'Serra Tico-Tico Bosch GST 650 500W Bivolt', slug: 'serra-tico-tico-bosch-gst-650', category: 'ferramentas', price: 389, currency: 'BRL', icon: '⚒', seller: 'Ferragem Silva', stock: 21, rating: 5, reviewCount: 112 },
  { id: '8', name: 'Rompedor Bosch GSH 5 CE SDS Max 1100W Bivolt', slug: 'rompedor-bosch-gsh-5-ce', category: 'ferramentas', price: 2799, originalPrice: 3499, currency: 'BRL', icon: '⚒', seller: 'Pro Tools BR', stock: 6, rating: 5, reviewCount: 29, cashbackAmount: 83.97, badge: 'discount', badgeLabel: '-20%', installments: 12 },

  // construcao
  { id: '9', name: 'Cimento CP II-E-32 Votoran 50kg', slug: 'cimento-votoran-50kg', category: 'construcao', price: 42.9, currency: 'BRL', icon: '◫', seller: 'Casa & Obra', stock: 200, rating: 4, reviewCount: 87 },
  { id: '10', name: 'Argamassa AC-II Quartzolit 20kg', slug: 'argamassa-quartzolit-20kg', category: 'construcao', price: 28.5, currency: 'BRL', icon: '◫', seller: 'Casa & Obra', stock: 150, rating: 4, reviewCount: 43 },
  { id: '11', name: 'Tijolo Cerâmico 9 Furos 9x14x19cm (cento)', slug: 'tijolo-ceramico-9-furos', category: 'construcao', price: 89, currency: 'BRL', icon: '◫', seller: 'Material Braz', stock: 80, rating: 4, reviewCount: 31, badge: 'free_shipping', badgeLabel: 'Frete grátis' },
  { id: '12', name: 'Tela Soldada Galvanizada 1x25m Fio 1,5mm', slug: 'tela-soldada-galvanizada', category: 'construcao', price: 149, currency: 'BRL', icon: '◫', seller: 'Material Braz', stock: 35, rating: 4, reviewCount: 19 },

  // eletrica
  { id: '13', name: 'Cabo Flexível 2,5mm² 100m Rolo Azul Sil', slug: 'cabo-flexivel-2-5mm-100m', category: 'eletrica', price: 249, currency: 'BRL', icon: '⚡', seller: 'Elétrica Costa', stock: 60, rating: 5, reviewCount: 203, cashbackAmount: 12.5, badge: 'free_shipping', badgeLabel: 'Frete grátis' },
  { id: '14', name: 'Disjuntor Bipolar 20A Schneider Domae', slug: 'disjuntor-bipolar-20a-schneider', category: 'eletrica', price: 34.9, currency: 'BRL', icon: '⚡', seller: 'Elétrica Costa', stock: 120, rating: 5, reviewCount: 88 },
  { id: '15', name: 'Tomada 2P+T 10A Tramontina Liz Branca', slug: 'tomada-tramontina-liz-10a', category: 'eletrica', price: 12.9, currency: 'BRL', icon: '⚡', seller: 'Materiais SP', stock: 300, rating: 4, reviewCount: 162 },
  { id: '16', name: 'Quadro de Distribuição 12 Disjuntores Embutir', slug: 'quadro-distribuicao-12-disjuntores', category: 'eletrica', price: 89.9, currency: 'BRL', icon: '⚡', seller: 'Elétrica Costa', stock: 44, rating: 4, reviewCount: 57, installments: 3 },

  // hidraulica
  { id: '17', name: 'Kit Tubo PVC Soldável 25mm 6m + 10 Conexões', slug: 'kit-pvc-soldavel-25mm', category: 'hidraulica', price: 89.9, currency: 'BRL', icon: '◡', seller: 'Hidro Total', stock: 55, rating: 4, reviewCount: 54 },
  { id: '18', name: 'Registro de Gaveta Bronze 3/4" Deca', slug: 'registro-gaveta-deca-3-4', category: 'hidraulica', price: 38.5, currency: 'BRL', icon: '◡', seller: 'Hidro Total', stock: 90, rating: 5, reviewCount: 76 },
  { id: '19', name: 'Caixa d\'Água Polietileno 500L Fortlev', slug: 'caixa-dagua-500l-fortlev', category: 'hidraulica', price: 379, currency: 'BRL', icon: '◡', seller: 'Hidro Total', stock: 12, rating: 5, reviewCount: 34, badge: 'free_shipping', badgeLabel: 'Frete grátis', installments: 10 },

  // pintura
  { id: '20', name: 'Tinta Acrílica Suvinil Fosco Premium 18L Branco', slug: 'tinta-suvinil-fosco-18l', category: 'pintura', price: 279, currency: 'BRL', icon: '▥', seller: 'Tintas Rio', stock: 38, rating: 5, reviewCount: 318, cashbackAmount: 25 },
  { id: '21', name: 'Rolo de Lã 23cm Tigre Cabo 15cm', slug: 'rolo-la-23cm-tigre', category: 'pintura', price: 18.9, currency: 'BRL', icon: '▥', seller: 'Tintas Rio', stock: 200, rating: 4, reviewCount: 94 },
  { id: '22', name: 'Massa Corrida PVA 25kg Suvinil', slug: 'massa-corrida-pva-25kg-suvinil', category: 'pintura', price: 89, currency: 'BRL', icon: '▥', seller: 'Tintas Rio', stock: 50, rating: 4, reviewCount: 47 },

  // jardim
  { id: '23', name: 'Mangueira de Jardim 50m 1/2" Tramontina', slug: 'mangueira-jardim-50m-tramontina', category: 'jardim', price: 79.9, currency: 'BRL', icon: '❀', seller: 'Verde Vida', stock: 45, rating: 4, reviewCount: 83 },
  { id: '24', name: 'Enxada Reta 1400g Tramontina com Cabo', slug: 'enxada-reta-tramontina-1400g', category: 'jardim', price: 49.9, currency: 'BRL', icon: '❀', seller: 'Verde Vida', stock: 70, rating: 5, reviewCount: 61 },

  // seguranca
  { id: '25', name: 'Capacete de Segurança Classe B Branco CA 31469', slug: 'capacete-seguranca-classe-b', category: 'seguranca', price: 29.9, currency: 'BRL', icon: '⚠', seller: 'EPI Pro', stock: 150, rating: 5, reviewCount: 72, cashbackAmount: 6, badge: 'last_units', badgeLabel: 'Últimas' },
  { id: '26', name: 'Luva de Segurança Vaqueta Tamanho M Par', slug: 'luva-seguranca-vaqueta-m', category: 'seguranca', price: 14.9, currency: 'BRL', icon: '⚠', seller: 'EPI Pro', stock: 300, rating: 4, reviewCount: 108 },
  { id: '27', name: 'Óculos de Proteção Ampla Visão Incolor 3M', slug: 'oculos-protecao-3m-incolor', category: 'seguranca', price: 19.9, currency: 'BRL', icon: '⚠', seller: 'EPI Pro', stock: 200, rating: 5, reviewCount: 95 },

  // fixacao
  { id: '28', name: 'Kit Parafusos Autoatarraxantes Sortidos 500pç', slug: 'kit-parafusos-autoatarraxantes-500', category: 'fixacao', price: 34.9, currency: 'BRL', icon: '▣', seller: 'Parafusos SP', stock: 120, rating: 4, reviewCount: 96 },
  { id: '29', name: 'Bucha Nylon S8 com Parafuso Kit 100pç', slug: 'bucha-nylon-s8-kit-100', category: 'fixacao', price: 22.9, currency: 'BRL', icon: '▣', seller: 'Parafusos SP', stock: 400, rating: 5, reviewCount: 134 },
  { id: '30', name: 'Prego com Cabeça 17x27 1kg Gerdau', slug: 'prego-com-cabeca-17x27-1kg', category: 'fixacao', price: 9.9, currency: 'BRL', icon: '▣', seller: 'Parafusos SP', stock: 500, rating: 4, reviewCount: 52 },
  { id: '31', name: 'Fita Veda Rosca 18mm x 50m Tigre', slug: 'fita-veda-rosca-18x50m', category: 'fixacao', price: 7.5, currency: 'BRL', icon: '▣', seller: 'Materiais SP', stock: 600, rating: 5, reviewCount: 187 },
]

export function getMockProducts(params: ProductsParams): Promise<ProductsResponse> {
  const perPage = params.per_page ?? 24
  const page = params.page ?? 1

  let filtered = [...MOCK_PRODUCTS]

  if (params.category) {
    filtered = filtered.filter((p) => p.category === params.category)
  }

  if (params.q) {
    const q = params.q.toLowerCase()
    filtered = filtered.filter(
      (p) => p.name.toLowerCase().includes(q) || p.seller.toLowerCase().includes(q)
    )
  }

  if (params.sort === 'price_asc') filtered.sort((a, b) => a.price - b.price)
  else if (params.sort === 'price_desc') filtered.sort((a, b) => b.price - a.price)
  else if (params.sort === 'top_rated') filtered.sort((a, b) => b.rating - a.rating || b.reviewCount - a.reviewCount)

  const total = filtered.length
  const totalPages = Math.ceil(total / perPage)
  const start = (page - 1) * perPage
  const data = filtered.slice(start, start + perPage)

  return Promise.resolve({ data, meta: { page, per_page: perPage, total, total_pages: totalPages } })
}
