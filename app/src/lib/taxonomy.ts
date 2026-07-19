export interface TaxonomyNode {
  slug: string
  labelKey: string
  icon: string
}

// O namespace vem embutido no labelKey de propósito.
//
// PORQUÊ: as traduções de categoria moram em common.json, mas vários
// consumidores declaram useTranslation(['catalog','common']) — o que faz o
// namespace PADRÃO virar `catalog`. Sem o prefixo, t('taxonomy.ferramentas')
// procura em catalog.json, não acha, e o i18next devolve a própria chave:
// o usuário via `taxonomy.ferramentas` escrito na tela do filtro de busca.
//
// Com o namespace na chave, o consumidor não precisa lembrar de nada.
export const TOP_LEVEL_CATEGORIES: TaxonomyNode[] = [
  { slug: 'ferramentas', labelKey: 'common:taxonomy.ferramentas', icon: '⚒' },
  { slug: 'construcao', labelKey: 'common:taxonomy.construcao', icon: '◫' },
  { slug: 'eletrica', labelKey: 'common:taxonomy.eletrica', icon: '⚡' },
  { slug: 'hidraulica', labelKey: 'common:taxonomy.hidraulica', icon: '◡' },
  { slug: 'pintura', labelKey: 'common:taxonomy.pintura', icon: '▥' },
  { slug: 'jardim', labelKey: 'common:taxonomy.jardim', icon: '❀' },
  { slug: 'seguranca', labelKey: 'common:taxonomy.seguranca', icon: '⚠' },
  { slug: 'fixacao', labelKey: 'common:taxonomy.fixacao', icon: '▣' },
]
