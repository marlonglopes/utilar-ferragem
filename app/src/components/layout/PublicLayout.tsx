import { Outlet } from 'react-router-dom'
import { Navbar } from './Navbar'
import { CategoryRail } from './CategoryRail'
import { Footer } from './Footer'

export function PublicLayout() {
  return (
    <div className="min-h-screen flex flex-col">
      <Navbar />
      <CategoryRail />
      <main className="flex-1">
        <Outlet />
      </main>
      <Footer />
    </div>
  )
}
