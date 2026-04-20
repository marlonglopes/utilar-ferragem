import { Link } from 'react-router-dom'

export default function HomePage() {
  return (
    <div className="flex flex-col items-center justify-center py-24 gap-4">
      <p className="text-gray-500 text-sm">
        Sprint 02 — design system ✓{' '}
        <Link to="/_dev/ui" className="text-brand-orange underline font-medium">
          Ver componentes →
        </Link>
      </p>
    </div>
  )
}
