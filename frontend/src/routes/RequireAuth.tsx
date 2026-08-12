import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '../features/auth/useAuthStore'

export default function RequireAuth() {
  const status = useAuthStore((state) => state.status)
  const location = useLocation()

  if (status === 'idle') {
    return null
  }

  if (status === 'unauthenticated') {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <Outlet />
}
