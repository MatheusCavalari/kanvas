import { describe, expect, it, vi, beforeEach } from 'vitest'
import { listColumns, createColumn, renameColumn, deleteColumn, reorderColumns } from './columns'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const columnBody = {
  id: 'col-1',
  board_id: 'board-1',
  title: 'To do',
  position: 0,
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
  cards: [
    {
      id: 'card-1',
      column_id: 'col-1',
      title: 'Write tests',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '2026-08-12T00:00:00Z',
      updated_at: '2026-08-12T00:00:00Z',
    },
  ],
}

describe('columns API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('listColumns fetches /boards/{id}/columns and maps embedded cards', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([columnBody])

    const result = await listColumns('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns')
    expect(result).toEqual([
      {
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
      },
    ])
  })

  it('listColumns defaults cards to an empty array when omitted', async () => {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { cards: _cards, ...withoutCards } = columnBody
    vi.mocked(client.apiFetch).mockResolvedValue([withoutCards])

    const result = await listColumns('board-1')

    expect(result[0].cards).toEqual([])
  })

  it('createColumn posts to /boards/{id}/columns with the title', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(columnBody)

    await createColumn('board-1', 'To do')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns', {
      method: 'POST',
      body: { title: 'To do' },
    })
  })

  it('renameColumn patches /boards/{id}/columns/{columnId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(columnBody)

    await renameColumn('board-1', 'col-1', 'Doing')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/col-1', {
      method: 'PATCH',
      body: { title: 'Doing' },
    })
  })

  it('deleteColumn deletes /boards/{id}/columns/{columnId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteColumn('board-1', 'col-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/col-1', { method: 'DELETE' })
  })

  it('reorderColumns patches /boards/{id}/columns/reorder with the full ordering', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await reorderColumns('board-1', ['col-2', 'col-1'])

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/reorder', {
      method: 'PATCH',
      body: { column_ids: ['col-2', 'col-1'] },
    })
  })
})
