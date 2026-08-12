import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext } from '@dnd-kit/sortable'
import Column from './Column'
import * as columnsApi from '../../api/columns'
import * as cardsApi from '../../api/cards'
import type { ColumnWithCards } from '../../api/columns'

vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, renameColumn: vi.fn(), deleteColumn: vi.fn() }
})
vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, createCard: vi.fn() }
})

const column: ColumnWithCards = {
  id: 'col-1',
  boardId: 'board-1',
  title: 'To do',
  position: 0,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
  cards: [
    {
      id: 'card-1',
      columnId: 'col-1',
      title: 'Write tests',
      description: '',
      position: 0,
      assigneeId: null,
      dueDate: null,
      createdAt: '2026-08-12T00:00:00Z',
      updatedAt: '2026-08-12T00:00:00Z',
    },
  ],
}

// Column's header row (rename/options controls) now carries dnd-kit's drag listeners, spread
// via useSortable so the header can act as a column-reorder handle. Without an
// activationConstraint, PointerSensor treats every pointerdown as an immediate drag start and
// swallows the subsequent click — see the matching note in CardItem.test.tsx. Configure the same
// 5px activation distance BoardPage uses in real usage (Step 5) so button clicks in tests behave
// like real clicks.
function Wrapper() {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  return (
    <DndContext sensors={sensors}>
      <SortableContext items={[column.id]}>
        <Column column={column} boardId="board-1" />
      </SortableContext>
    </DndContext>
  )
}

function renderWithProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <Wrapper />
    </QueryClientProvider>,
  )
}

describe('Column', () => {
  beforeEach(() => {
    vi.mocked(columnsApi.renameColumn).mockReset()
    vi.mocked(columnsApi.deleteColumn).mockReset()
    vi.mocked(cardsApi.createCard).mockReset()
  })

  it('renders the column title and its cards', () => {
    renderWithProviders()

    expect(screen.getByText('To do')).toBeInTheDocument()
    expect(screen.getByText('Write tests')).toBeInTheDocument()
  })

  it('creates a card via the add-card form', async () => {
    vi.mocked(cardsApi.createCard).mockResolvedValue({ ...column.cards[0], id: 'card-2', title: 'New card' })
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /adicionar card/i }))
    await userEvent.type(screen.getByLabelText(/título do card/i), 'New card')
    await userEvent.click(screen.getByRole('button', { name: /^adicionar$/i }))

    await waitFor(() => expect(cardsApi.createCard).toHaveBeenCalledWith('col-1', 'New card'))
  })

  it('renames the column via the menu', async () => {
    vi.mocked(columnsApi.renameColumn).mockResolvedValue({ ...column, title: 'Doing' })
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /opções da coluna/i }))
    // The column header row is now a dnd-kit drag handle (role="button" via useSortable's
    // attributes), so its accessible name includes the menu's text once open ("Renomear",
    // "Excluir coluna"). Use an exact name match to target only the menu item, not the header.
    await userEvent.click(screen.getByRole('button', { name: 'Renomear' }))
    const input = screen.getByDisplayValue('To do')
    await userEvent.clear(input)
    await userEvent.type(input, 'Doing{Enter}')

    await waitFor(() => expect(columnsApi.renameColumn).toHaveBeenCalledWith('board-1', 'col-1', 'Doing'))
  })

  it('deletes the column via the menu', async () => {
    vi.mocked(columnsApi.deleteColumn).mockResolvedValue(undefined)
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /opções da coluna/i }))
    // Same exact-name rationale as the rename test above.
    await userEvent.click(screen.getByRole('button', { name: 'Excluir coluna' }))

    await waitFor(() => expect(columnsApi.deleteColumn).toHaveBeenCalledWith('board-1', 'col-1'))
  })
})
