import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BoardPage from './BoardPage'
import * as columnsApi from '../../api/columns'
import * as boardsApi from '../../api/boards'
import { useAuthStore } from '../auth/useAuthStore'

vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, listColumns: vi.fn(), createColumn: vi.fn(), reorderColumns: vi.fn() }
})
vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, moveCard: vi.fn() }
})
vi.mock('../../api/boards', async () => {
  const actual = await vi.importActual<typeof import('../../api/boards')>('../../api/boards')
  return { ...actual, listMembers: vi.fn().mockResolvedValue([]) }
})

class FakeWebSocket {
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
  }

  close() {
    this.closed = true
    this.onclose?.()
  }
}

function renderWithProviders(boardId = 'board-1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/boards/${boardId}`]}>
        <Routes>
          <Route path="/boards/:boardId" element={<BoardPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('BoardPage', () => {
  beforeEach(() => {
    vi.mocked(columnsApi.listColumns).mockReset()
    vi.mocked(columnsApi.createColumn).mockReset()
    vi.mocked(boardsApi.listMembers).mockReset()
    vi.mocked(boardsApi.listMembers).mockResolvedValue([])
    vi.stubGlobal('WebSocket', FakeWebSocket)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the fetched columns', async () => {
    vi.mocked(columnsApi.listColumns).mockResolvedValue([
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ])

    renderWithProviders()

    expect(await screen.findByText('To do')).toBeInTheDocument()
  })

  it('shows a retry option when the fetch fails', async () => {
    vi.mocked(columnsApi.listColumns).mockRejectedValue(new Error('network down'))

    renderWithProviders()

    expect(await screen.findByRole('button', { name: /tentar novamente/i })).toBeInTheDocument()
  })

  it('creates a column via the add-column form', async () => {
    vi.mocked(columnsApi.listColumns).mockResolvedValue([])
    vi.mocked(columnsApi.createColumn).mockResolvedValue({
      id: 'col-1',
      boardId: 'board-1',
      title: 'Backlog',
      position: 0,
      createdAt: '',
      updatedAt: '',
    })

    renderWithProviders()
    await waitFor(() => expect(columnsApi.listColumns).toHaveBeenCalled())

    await userEvent.click(await screen.findByRole('button', { name: /adicionar coluna/i }))
    await userEvent.type(screen.getByLabelText(/título da coluna/i), 'Backlog')
    await userEvent.click(screen.getByRole('button', { name: /^adicionar$/i }))

    await waitFor(() => expect(columnsApi.createColumn).toHaveBeenCalledWith('board-1', 'Backlog'))
  })

  it('opens the members panel from the header button', async () => {
    useAuthStore.setState({ user: { id: 'user-1', name: 'Owner', email: 'owner@example.com' }, status: 'authenticated' })
    vi.mocked(columnsApi.listColumns).mockResolvedValue([])

    renderWithProviders()
    await waitFor(() => expect(columnsApi.listColumns).toHaveBeenCalled())

    await userEvent.click(await screen.findByRole('button', { name: /membros/i }))

    expect(await screen.findByRole('dialog', { name: /membros/i })).toBeInTheDocument()
  })
})
