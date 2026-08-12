import { describe, expect, it, vi, beforeEach } from 'vitest'
import { listBoards, createBoard, renameBoard, deleteBoard, listMembers, inviteMember, removeMember } from './boards'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const boardBody = {
  id: 'board-1',
  name: 'Sprint Board',
  owner_id: 'user-1',
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
}

const memberBody = {
  user_id: 'user-2',
  role: 'member',
  name: 'Ada Lovelace',
  email: 'ada@example.com',
  created_at: '2026-08-12T00:00:00Z',
}

describe('boards API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('listBoards fetches /boards and maps the response', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([boardBody])

    const result = await listBoards()

    expect(client.apiFetch).toHaveBeenCalledWith('/boards')
    expect(result).toEqual([
      { id: 'board-1', name: 'Sprint Board', ownerId: 'user-1', createdAt: '2026-08-12T00:00:00Z', updatedAt: '2026-08-12T00:00:00Z' },
    ])
  })

  it('createBoard posts to /boards with the name', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(boardBody)

    await createBoard('Sprint Board')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards', { method: 'POST', body: { name: 'Sprint Board' } })
  })

  it('renameBoard patches /boards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(boardBody)

    await renameBoard('board-1', 'New name')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1', { method: 'PATCH', body: { name: 'New name' } })
  })

  it('deleteBoard deletes /boards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteBoard('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1', { method: 'DELETE' })
  })

  it('listMembers fetches /boards/{id}/members and maps the response', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([memberBody])

    const result = await listMembers('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members')
    expect(result).toEqual([
      { userId: 'user-2', role: 'member', name: 'Ada Lovelace', email: 'ada@example.com', createdAt: '2026-08-12T00:00:00Z' },
    ])
  })

  it('inviteMember posts to /boards/{id}/members with the email', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(memberBody)

    await inviteMember('board-1', 'ada@example.com')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members', {
      method: 'POST',
      body: { email: 'ada@example.com' },
    })
  })

  it('removeMember deletes /boards/{id}/members/{userId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await removeMember('board-1', 'user-2')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members/user-2', { method: 'DELETE' })
  })
})
