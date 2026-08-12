import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import RegisterPage from './RegisterPage'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

describe('RegisterPage', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'unauthenticated', register: vi.fn() })
  })

  it('requires name, email and password before submitting', async () => {
    const register = vi.fn()
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    expect(register).not.toHaveBeenCalled()
  })

  it('calls register with the entered data on submit', async () => {
    const register = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/nome/i), 'Ada Lovelace')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    await waitFor(() =>
      expect(register).toHaveBeenCalledWith('Ada Lovelace', 'ada@example.com', 'password123'),
    )
  })

  it('shows the backend error message when registration fails', async () => {
    const register = vi.fn().mockRejectedValue(new ApiError(409, 'email_taken', 'email already registered'))
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/nome/i), 'Ada Lovelace')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    expect(await screen.findByText('email already registered')).toBeInTheDocument()
  })
})
