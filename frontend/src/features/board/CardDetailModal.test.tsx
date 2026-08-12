import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import CardDetailModal from './CardDetailModal'
import * as cardsApi from '../../api/cards'
import type { Card } from '../../api/cards'

vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, updateCard: vi.fn(), deleteCard: vi.fn() }
})

const card: Card = {
  id: 'card-1',
  columnId: 'col-1',
  title: 'Write tests',
  description: 'Original description',
  position: 0,
  assigneeId: null,
  dueDate: null,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

function renderWithProviders(onClose = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return { queryClient, ...render(
    <QueryClientProvider client={queryClient}>
      <CardDetailModal card={card} boardId="board-1" onClose={onClose} />
    </QueryClientProvider>,
  ) }
}

describe('CardDetailModal', () => {
  beforeEach(() => {
    vi.mocked(cardsApi.updateCard).mockReset()
    vi.mocked(cardsApi.deleteCard).mockReset()
  })

  it('shows the current title and description', () => {
    renderWithProviders()

    expect(screen.getByDisplayValue('Write tests')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Original description')).toBeInTheDocument()
  })

  it('saves edits via updateCard', async () => {
    vi.mocked(cardsApi.updateCard).mockResolvedValue({ ...card, title: 'Updated title' })
    const onClose = vi.fn()
    renderWithProviders(onClose)

    await userEvent.clear(screen.getByLabelText(/título/i))
    await userEvent.type(screen.getByLabelText(/título/i), 'Updated title')
    await userEvent.click(screen.getByRole('button', { name: /salvar/i }))

    await waitFor(() =>
      expect(cardsApi.updateCard).toHaveBeenCalledWith('card-1', 'Updated title', 'Original description', null, null),
    )
    expect(onClose).toHaveBeenCalled()
  })

  it('deletes the card via deleteCard', async () => {
    vi.mocked(cardsApi.deleteCard).mockResolvedValue(undefined)
    const onClose = vi.fn()
    renderWithProviders(onClose)

    await userEvent.click(screen.getByRole('button', { name: /excluir/i }))

    await waitFor(() => expect(cardsApi.deleteCard).toHaveBeenCalledWith('card-1'))
    expect(onClose).toHaveBeenCalled()
  })
})
