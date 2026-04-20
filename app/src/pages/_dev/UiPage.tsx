import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Button,
  Input,
  Select,
  Checkbox,
  Radio,
  Card,
  CardHeader,
  CardTitle,
  CardBody,
  CardFooter,
  Badge,
  Tag,
  Modal,
  Drawer,
  Skeleton,
  Pagination,
  Breadcrumb,
  useToast,
} from '@/components/ui'
import { LocaleSwitcher } from '@/components/common/LocaleSwitcher'
import { Search, Star } from 'lucide-react'

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-12">
      <h2 className="text-lg font-display font-semibold text-gray-900 mb-6 pb-2 border-b border-gray-200">
        {title}
      </h2>
      {children}
    </section>
  )
}

function Row({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-wrap items-center gap-3 mb-4">{children}</div>
}

export default function UiPage() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const [modalOpen, setModalOpen] = useState(false)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [drawerSide, setDrawerSide] = useState<'left' | 'right' | 'bottom'>('right')
  const [page, setPage] = useState(3)
  const [checked, setChecked] = useState(false)
  const [radio, setRadio] = useState('a')

  return (
    <div className="container py-10">
      <div className="flex items-center justify-between mb-10">
        <div>
          <h1 className="text-2xl font-display font-black text-gray-900">
            Design System — Referência
          </h1>
          <p className="text-sm text-gray-500 mt-1">Sprint 02 — todos os primitivos em todas as variantes</p>
        </div>
        <LocaleSwitcher />
      </div>

      <Section title="Button">
        <Row>
          <Button variant="primary">Primary</Button>
          <Button variant="secondary">Secondary</Button>
          <Button variant="ghost">Ghost</Button>
          <Button variant="danger">Danger</Button>
        </Row>
        <Row>
          <Button size="sm">Small</Button>
          <Button size="md">Medium</Button>
          <Button size="lg">Large</Button>
        </Row>
        <Row>
          <Button loading>Loading</Button>
          <Button disabled>Disabled</Button>
          <Button fullWidth>Full width</Button>
        </Row>
      </Section>

      <Section title="Input">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 max-w-xl">
          <Input label="Label padrão" placeholder="Placeholder..." />
          <Input label="Com ícone esquerdo" leftIcon={<Search className="h-4 w-4" />} placeholder={t('searchPlaceholder')} />
          <Input label="Com erro" error="Campo obrigatório" defaultValue="valor inválido" />
          <Input label="Com dica" hint="Texto de ajuda abaixo" placeholder="Opcional..." />
          <Input label="Desabilitado" disabled value="Não editável" readOnly />
          <Input label="Com ícone direito" rightIcon={<Star className="h-4 w-4" />} placeholder="Avaliação..." />
        </div>
      </Section>

      <Section title="Select">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 max-w-xl">
          <Select
            label="Ordenar por"
            options={[
              { value: 'relevance', label: 'Relevância' },
              { value: 'price_asc', label: 'Menor preço' },
              { value: 'price_desc', label: 'Maior preço' },
            ]}
            placeholder="Escolher..."
          />
          <Select
            label="Com erro"
            error="Seleção obrigatória"
            options={[{ value: 'a', label: 'Opção A' }]}
          />
        </div>
      </Section>

      <Section title="Checkbox e Radio">
        <Row>
          <Checkbox
            label="Marcar"
            checked={checked}
            onChange={(e) => setChecked(e.target.checked)}
          />
          <Checkbox label="Marcado" checked readOnly />
          <Checkbox label="Desabilitado" disabled />
          <Checkbox label="Com erro" error="Aceite obrigatório" />
        </Row>
        <Row>
          <Radio label="Opção A" name="demo" value="a" checked={radio === 'a'} onChange={() => setRadio('a')} />
          <Radio label="Opção B" name="demo" value="b" checked={radio === 'b'} onChange={() => setRadio('b')} />
          <Radio label="Desabilitado" disabled />
        </Row>
      </Section>

      <Section title="Badge">
        <Row>
          <Badge>Default</Badge>
          <Badge variant="success">Success</Badge>
          <Badge variant="warning">Warning</Badge>
          <Badge variant="danger">Danger</Badge>
          <Badge variant="info">Info</Badge>
          <Badge variant="orange">Em estoque</Badge>
          <Badge variant="blue">Destaque</Badge>
        </Row>
      </Section>

      <Section title="Tag">
        <Row>
          <Tag>Ferramentas</Tag>
          <Tag removable onRemove={() => {}}>Removível</Tag>
          <Tag onRemove={() => {}}>Com handler</Tag>
        </Row>
      </Section>

      <Section title="Card">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 max-w-2xl">
          <Card>
            <CardHeader><CardTitle>Título do card</CardTitle></CardHeader>
            <CardBody><p className="text-sm text-gray-600">Conteúdo do card com texto de exemplo.</p></CardBody>
            <CardFooter>
              <Button size="sm">Ação</Button>
              <Button size="sm" variant="ghost">Cancelar</Button>
            </CardFooter>
          </Card>
          <Card shadow="md">
            <CardBody><p className="text-sm text-gray-600">Shadow md, padding padrão.</p></CardBody>
          </Card>
          <Card border={false} shadow="none" className="bg-brand-blue-light">
            <CardBody><p className="text-sm text-brand-blue font-medium">Sem borda, sem sombra.</p></CardBody>
          </Card>
        </div>
      </Section>

      <Section title="Skeleton">
        <div className="flex flex-col gap-4 max-w-xs">
          <div className="flex items-center gap-3">
            <Skeleton variant="circle" className="h-10 w-10" />
            <div className="flex-1">
              <Skeleton variant="text" className="mb-1" />
              <Skeleton variant="text" className="w-2/3" />
            </div>
          </div>
          <Skeleton className="h-32 w-full" />
          <Skeleton variant="text" lines={3} />
        </div>
      </Section>

      <Section title="Breadcrumb">
        <Breadcrumb
          items={[
            { label: 'Início', href: '/' },
            { label: 'Ferramentas', href: '/categoria/ferramentas' },
            { label: 'Furadeiras' },
          ]}
        />
      </Section>

      <Section title="Pagination">
        <Pagination page={page} totalPages={10} onPageChange={setPage} />
      </Section>

      <Section title="Modal">
        <Button onClick={() => setModalOpen(true)}>Abrir Modal</Button>
        <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Título do modal">
          <p className="text-sm text-gray-700 mb-4">
            Conteúdo do modal. Pressione <kbd className="rounded bg-gray-100 px-1 py-0.5 text-xs">Esc</kbd> ou clique fora para fechar.
          </p>
          <div className="flex gap-2 justify-end">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancelar</Button>
            <Button onClick={() => setModalOpen(false)}>Confirmar</Button>
          </div>
        </Modal>
      </Section>

      <Section title="Drawer">
        <Row>
          {(['right', 'left', 'bottom'] as const).map((side) => (
            <Button
              key={side}
              variant="secondary"
              onClick={() => { setDrawerSide(side); setDrawerOpen(true) }}
            >
              Drawer {side}
            </Button>
          ))}
        </Row>
        <Drawer
          open={drawerOpen}
          onClose={() => setDrawerOpen(false)}
          side={drawerSide}
          title={`Drawer — ${drawerSide}`}
        >
          <p className="text-sm text-gray-700">Conteúdo do drawer abre pelo lado <strong>{drawerSide}</strong>.</p>
        </Drawer>
      </Section>

      <Section title="Toast">
        <Row>
          {(['success', 'error', 'info', 'warning'] as const).map((v) => (
            <Button
              key={v}
              variant="ghost"
              onClick={() => toast(`Toast de ${v}`, v)}
            >
              {v}
            </Button>
          ))}
        </Row>
      </Section>

      <Section title="Locale Switcher">
        <LocaleSwitcher />
        <p className="mt-2 text-sm text-gray-500">{t('tagline')}</p>
      </Section>

      <Section title="Tipografia">
        <div className="space-y-3">
          <p className="font-display font-black text-3xl text-gray-900">Archivo Black — display</p>
          <p className="font-sans text-base text-gray-700">Inter Regular — corpo de texto</p>
          <p className="font-sans font-semibold text-base text-gray-900">Inter SemiBold — labels</p>
          <code className="font-mono text-sm bg-gray-100 px-2 py-1 rounded">JetBrains Mono — código</code>
          <div className="flex items-center gap-2">
            <Badge variant="orange" className="text-xs">R$ 149,90</Badge>
            <span className="text-brand-orange font-bold text-xl">R$ 149,90</span>
          </div>
        </div>
      </Section>

      <Section title="Paleta de cores">
        <div className="flex flex-wrap gap-3">
          {[
            { name: 'Orange', bg: 'bg-brand-orange' },
            { name: 'Orange Dark', bg: 'bg-brand-orange-dark' },
            { name: 'Orange Light', bg: 'bg-brand-orange-light' },
            { name: 'Blue', bg: 'bg-brand-blue' },
            { name: 'Blue Dark', bg: 'bg-brand-blue-dark' },
            { name: 'Blue Light', bg: 'bg-brand-blue-light' },
            { name: 'Gold', bg: 'bg-brand-gold' },
          ].map(({ name, bg }) => (
            <div key={name} className="flex flex-col items-center gap-1">
              <div className={`w-12 h-12 rounded-lg ${bg} border border-black/10`} />
              <span className="text-xs text-gray-600">{name}</span>
            </div>
          ))}
        </div>
      </Section>
    </div>
  )
}
