import { useEffect } from 'react'
import { RouterProvider } from 'react-router-dom'
import { router } from './routes/router'
import { useAuthStore } from './features/auth/useAuthStore'
import { setUnauthorizedHandler } from './api/client'

export default function App() {
  const restoreSession = useAuthStore((state) => state.restoreSession)
  const status = useAuthStore((state) => state.status)

  useEffect(() => {
    setUnauthorizedHandler(() => {
      useAuthStore.setState({ user: null, status: 'unauthenticated' })
    })
    restoreSession()
  }, [restoreSession])

  if (status === 'idle') {
    return (
      <div className="flex min-h-screen items-center justify-center text-gray-500">
        Carregando...
      </div>
    )
  }

  return <RouterProvider router={router} />
}
