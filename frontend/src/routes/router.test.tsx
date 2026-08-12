import { describe, expect, it, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider, type InitialEntry } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { routes } from './router'
import { useAuthStore } from '../features/auth/useAuthStore'
import * as boardsApi from '../api/boards'

vi.mock('../api/boards')

// The real route table plus a stand-in destination route, so a redirect to
// a deep link (e.g. /boards/42) has somewhere to land — /boards/* itself
// doesn't exist yet (it's a future phase).
const testRoutes = [...routes, { path: '/boards/42', element: <p>Board 42</p> }]

function renderAt(initialEntries: InitialEntry[]) {
  const router = createMemoryRouter(testRoutes, { initialEntries })
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

describe('router', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: { id: '1', name: 'Ada', email: 'ada@example.com' }, status: 'idle' })
    vi.mocked(boardsApi.listBoards).mockReset()
    vi.mocked(boardsApi.listBoards).mockResolvedValue([])
  })

  it('redirects an already-authenticated user hitting /login to the preserved deep-link destination', () => {
    useAuthStore.setState({ status: 'authenticated' })
    renderAt([{ pathname: '/login', state: { from: { pathname: '/boards/42' } } }])

    expect(screen.getByText('Board 42')).toBeInTheDocument()
  })

  it('redirects an already-authenticated user hitting /login with no preserved destination to /', async () => {
    useAuthStore.setState({ status: 'authenticated' })
    renderAt(['/login'])

    expect(await screen.findByText('Seus boards')).toBeInTheDocument()
  })
})
