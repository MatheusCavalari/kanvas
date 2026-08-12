import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import LoginPage from './LoginPage'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

describe('LoginPage', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'unauthenticated', login: vi.fn() })
  })

  it('requires email and password before submitting', async () => {
    const login = vi.fn()
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(login).not.toHaveBeenCalled()
    expect(screen.getByLabelText(/e-mail/i)).toBeInvalid()
  })

  it('calls login with the entered credentials on submit', async () => {
    const login = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    await waitFor(() => expect(login).toHaveBeenCalledWith('ada@example.com', 'password123'))
  })

  it('redirects back to the originally-requested URL after login, when one was preserved', async () => {
    const login = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login })
    render(
      <MemoryRouter
        initialEntries={[{ pathname: '/login', state: { from: { pathname: '/boards/42' } } }]}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/boards/42" element={<p>Board 42</p>} />
        </Routes>
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(await screen.findByText('Board 42')).toBeInTheDocument()
  })

  it('shows the backend error message when login fails', async () => {
    const login = vi.fn().mockRejectedValue(new ApiError(401, 'invalid_credentials', 'invalid email or password'))
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'wrong-password')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(await screen.findByText('invalid email or password')).toBeInTheDocument()
  })
})
