import { describe, expect, it, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'
import { useAuthStore } from './features/auth/useAuthStore'

describe('App', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
  })

  it('shows a loading state while session restore is in flight', () => {
    render(<App />)
    expect(screen.getByText('Carregando...')).toBeInTheDocument()
  })
})
