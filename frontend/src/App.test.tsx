import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import App from './App'
import { useAuthStore } from './features/auth/useAuthStore'

describe('App', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle', restoreSession: vi.fn() })
  })

  it('shows a loading state while session restore is in flight', () => {
    render(<App />)
    expect(screen.getByText('Carregando...')).toBeInTheDocument()
  })

  it('renders the router content once session restore resolves', async () => {
    useAuthStore.setState({
      restoreSession: vi.fn().mockImplementation(async () => {
        useAuthStore.setState({ user: null, status: 'unauthenticated' })
      }),
    })

    render(<App />)

    await waitFor(() => expect(screen.queryByText('Carregando...')).not.toBeInTheDocument())
  })
})
