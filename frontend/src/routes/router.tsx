import { createBrowserRouter, Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import type { RouteObject } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import AppLayout from '../components/layout/AppLayout'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import BoardListPage from '../features/boards/BoardListPage'
import { useAuthStore } from '../features/auth/useAuthStore'

function RedirectIfAuthenticated({ children }: { children: ReactNode }) {
  const status = useAuthStore((state) => state.status)
  const location = useLocation()
  if (status === 'authenticated') {
    const redirectTo = (location.state as { from?: { pathname: string } } | null)?.from?.pathname ?? '/'
    return <Navigate to={redirectTo} replace />
  }
  return <>{children}</>
}

export const routes: RouteObject[] = [
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
        children: [{ path: '/', element: <BoardListPage /> }],
      },
    ],
  },
]

export const router = createBrowserRouter(routes)
