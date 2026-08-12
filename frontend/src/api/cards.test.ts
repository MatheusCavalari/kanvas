import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createCard, updateCard, deleteCard, moveCard } from './cards'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const cardBody = {
  id: 'card-1',
  column_id: 'col-1',
  title: 'Write tests',
  description: 'Cover the happy path',
  position: 0,
  assignee_id: null,
  due_date: null,
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
}

describe('cards API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('createCard posts to /cards with column_id, title, and description', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    const result = await createCard('col-1', 'Write tests', 'Cover the happy path')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards', {
      method: 'POST',
      body: { column_id: 'col-1', title: 'Write tests', description: 'Cover the happy path' },
    })
    expect(result).toEqual({
      id: 'card-1',
      columnId: 'col-1',
      title: 'Write tests',
      description: 'Cover the happy path',
      position: 0,
      assigneeId: null,
      dueDate: null,
      createdAt: '2026-08-12T00:00:00Z',
      updatedAt: '2026-08-12T00:00:00Z',
    })
  })

  it('createCard defaults description to an empty string', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await createCard('col-1', 'Write tests')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards', {
      method: 'POST',
      body: { column_id: 'col-1', title: 'Write tests', description: '' },
    })
  })

  it('updateCard patches /cards/{id} with the full editable field set', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await updateCard('card-1', 'New title', 'New description', 'user-2', '2026-09-01T00:00:00Z')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1', {
      method: 'PATCH',
      body: {
        title: 'New title',
        description: 'New description',
        assignee_id: 'user-2',
        due_date: '2026-09-01T00:00:00Z',
      },
    })
  })

  it('deleteCard deletes /cards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteCard('card-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1', { method: 'DELETE' })
  })

  it('moveCard patches /cards/{id}/move with the target column and position', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await moveCard('card-1', 'col-2', 3)

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1/move', {
      method: 'PATCH',
      body: { column_id: 'col-2', position: 3 },
    })
  })
})
