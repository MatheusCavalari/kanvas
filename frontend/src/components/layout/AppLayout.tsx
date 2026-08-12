import { Outlet } from 'react-router-dom'
import { useAuthStore } from '../../features/auth/useAuthStore'

export default function AppLayout() {
  const user = useAuthStore((state) => state.user)
  const logout = useAuthStore((state) => state.logout)

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="flex items-center justify-between border-b bg-white px-6 py-3">
        <span className="font-semibold text-gray-900">Kanvas</span>
        <div className="flex items-center gap-4">
          {user && <span className="text-sm text-gray-700">{user.name}</span>}
          <button
            type="button"
            onClick={() => logout()}
            className="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-100"
          >
            Sair
          </button>
        </div>
      </header>
      <main className="p-6">
        <Outlet />
      </main>
    </div>
  )
}
