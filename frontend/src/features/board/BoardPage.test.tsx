import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BoardPage from './BoardPage'
import * as columnsApi from '../../api/columns'

vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, listColumns: vi.fn(), createColumn: vi.fn() }
})

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
})
