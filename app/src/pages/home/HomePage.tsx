export default function HomePage() {
  return (
    <main className="min-h-screen bg-white flex flex-col items-center justify-center gap-4">
      <div className="flex items-center gap-3">
        <span className="w-10 h-10 rounded-lg bg-brand-blue flex items-center justify-center text-white font-display font-black text-xl italic">
          U
        </span>
        <div className="flex flex-col leading-none">
          <span className="font-display font-black text-2xl tracking-tight">
            <span className="text-gray-900">Uti</span>
            <span className="text-brand-blue">Lar</span>
          </span>
          <span className="text-[10px] font-bold tracking-widest uppercase text-gray-400">
            Solução em Ferragem
          </span>
        </div>
      </div>
      <p className="text-gray-500 text-sm">Sprint 01 — scaffold ✓</p>
    </main>
  )
}
