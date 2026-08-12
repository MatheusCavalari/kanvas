import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BoardListPage from './BoardListPage'
import * as boardsApi from '../../api/boards'

vi.mock('../../api/boards')

function renderWithProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <BoardListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const board = {
  id: 'board-1',
  name: 'Sprint Board',
  ownerId: 'user-1',
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

describe('BoardListPage', () => {
  beforeEach(() => {
    vi.mocked(boardsApi.listBoards).mockReset()
    vi.mocked(boardsApi.createBoard).mockReset()
  })

  it('renders the fetched boards', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValue([board])
    renderWithProviders()

    expect(await screen.findByText('Sprint Board')).toBeInTheDocument()
  })

  it('shows an empty state when there are no boards', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValue([])
    renderWithProviders()

    expect(await screen.findByText(/nenhum board ainda/i)).toBeInTheDocument()
  })

  it('shows a retry option when the fetch fails', async () => {
    vi.mocked(boardsApi.listBoards).mockRejectedValue(new Error('network down'))
    renderWithProviders()

    expect(await screen.findByRole('button', { name: /tentar novamente/i })).toBeInTheDocument()
  })

  it('creates a board and adds it to the list', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValueOnce([]).mockResolvedValueOnce([board])
    vi.mocked(boardsApi.createBoard).mockResolvedValue(board)
    renderWithProviders()

    await screen.findByText(/nenhum board ainda/i)

    await userEvent.click(screen.getByRole('button', { name: /novo board/i }))
    await userEvent.type(screen.getByLabelText(/nome/i), 'Sprint Board')
    await userEvent.click(screen.getByRole('button', { name: /criar/i }))

    await waitFor(() => expect(boardsApi.createBoard).toHaveBeenCalledWith('Sprint Board'))
    expect(await screen.findByText('Sprint Board')).toBeInTheDocument()
  })
})
