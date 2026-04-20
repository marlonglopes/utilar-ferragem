export interface TaxonomyNode {
  slug: string
  labelKey: string
  icon: string
}

export const TOP_LEVEL_CATEGORIES: TaxonomyNode[] = [
  { slug: 'ferragens', labelKey: 'taxonomy.ferragens', icon: '🔩' },
  { slug: 'ferramentas', labelKey: 'taxonomy.ferramentas', icon: '🔧' },
  { slug: 'eletrica', labelKey: 'taxonomy.eletrica', icon: '⚡' },
  { slug: 'hidraulica', labelKey: 'taxonomy.hidraulica', icon: '🚿' },
  { slug: 'tintas', labelKey: 'taxonomy.tintas', icon: '🎨' },
  { slug: 'construcao', labelKey: 'taxonomy.construcao', icon: '🧱' },
  { slug: 'jardim', labelKey: 'taxonomy.jardim', icon: '🌿' },
  { slug: 'seguranca', labelKey: 'taxonomy.seguranca', icon: '🔒' },
]
