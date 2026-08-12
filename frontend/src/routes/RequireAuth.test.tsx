import { describe, expect, it, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import { useAuthStore } from '../features/auth/useAuthStore'

function renderWithAuth(status: 'idle' | 'authenticated' | 'unauthenticated') {
  useAuthStore.setState({ status })
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route element={<RequireAuth />}>
          <Route path="/" element={<p>Protected content</p>} />
        </Route>
        <Route path="/login" element={<p>Login page</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('RequireAuth', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
  })

  it('renders the protected content when authenticated', () => {
    renderWithAuth('authenticated')
    expect(screen.getByText('Protected content')).toBeInTheDocument()
  })

  it('redirects to /login when unauthenticated', () => {
    renderWithAuth('unauthenticated')
    expect(screen.getByText('Login page')).toBeInTheDocument()
  })

  it('renders nothing (no redirect yet) while status is idle', () => {
    renderWithAuth('idle')
    expect(screen.queryByText('Protected content')).not.toBeInTheDocument()
    expect(screen.queryByText('Login page')).not.toBeInTheDocument()
  })
})
