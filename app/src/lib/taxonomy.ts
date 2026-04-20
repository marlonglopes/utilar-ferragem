export interface TaxonomyNode {
  slug: string
  labelKey: string
  icon: string
}

export const TOP_LEVEL_CATEGORIES: TaxonomyNode[] = [
  { slug: 'ferramentas', labelKey: 'taxonomy.ferramentas', icon: '⚒' },
  { slug: 'construcao', labelKey: 'taxonomy.construcao', icon: '◫' },
  { slug: 'eletrica', labelKey: 'taxonomy.eletrica', icon: '⚡' },
  { slug: 'hidraulica', labelKey: 'taxonomy.hidraulica', icon: '◡' },
  { slug: 'pintura', labelKey: 'taxonomy.pintura', icon: '▥' },
  { slug: 'jardim', labelKey: 'taxonomy.jardim', icon: '❀' },
  { slug: 'seguranca', labelKey: 'taxonomy.seguranca', icon: '⚠' },
  { slug: 'fixacao', labelKey: 'taxonomy.fixacao', icon: '▣' },
]
