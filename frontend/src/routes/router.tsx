import { createBrowserRouter, Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import RequireAuth from './RequireAuth'
import AppLayout from '../components/layout/AppLayout'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import { useAuthStore } from '../features/auth/useAuthStore'

function HomePage() {
  const user = useAuthStore((state) => state.user)
  return <p className="text-gray-700">Bem-vindo, {user?.name}. A lista de boards chega na próxima fase.</p>
}

function RedirectIfAuthenticated({ children }: { children: ReactNode }) {
  const status = useAuthStore((state) => state.status)
  if (status === 'authenticated') {
    return <Navigate to="/" replace />
  }
  return <>{children}</>
}

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <RedirectIfAuthenticated>
        <LoginPage />
      </RedirectIfAuthenticated>
    ),
  },
  {
    path: '/register',
    element: (
      <RedirectIfAuthenticated>
        <RegisterPage />
      </RedirectIfAuthenticated>
    ),
  },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <AppLayout />,
        children: [{ path: '/', element: <HomePage /> }],
      },
    ],
  },
])
