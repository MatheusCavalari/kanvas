import { apiFetch } from './client'

export interface Board {
  id: string
  name: string
  ownerId: string
  createdAt: string
  updatedAt: string
}

export interface Member {
  userId: string
  role: 'owner' | 'member'
  name: string
  email: string
  createdAt: string
}

interface BoardBody {
  id: string
  name: string
  owner_id: string
  created_at: string
  updated_at: string
}

interface MemberBody {
  user_id: string
  role: string
  name: string
  email: string
  created_at: string
}

function toBoard(body: BoardBody): Board {
  return {
    id: body.id,
    name: body.name,
    ownerId: body.owner_id,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

function toMember(body: MemberBody): Member {
  return {
    userId: body.user_id,
    role: body.role as Member['role'],
    name: body.name,
    email: body.email,
    createdAt: body.created_at,
  }
}

export async function listBoards(): Promise<Board[]> {
  const body = await apiFetch<BoardBody[]>('/boards')
  return body.map(toBoard)
}

export async function createBoard(name: string): Promise<Board> {
  const body = await apiFetch<BoardBody>('/boards', { method: 'POST', body: { name } })
  return toBoard(body)
}

export async function renameBoard(boardId: string, name: string): Promise<Board> {
  const body = await apiFetch<BoardBody>(`/boards/${boardId}`, { method: 'PATCH', body: { name } })
  return toBoard(body)
}

export async function deleteBoard(boardId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}`, { method: 'DELETE' })
}

export async function listMembers(boardId: string): Promise<Member[]> {
  const body = await apiFetch<MemberBody[]>(`/boards/${boardId}/members`)
  return body.map(toMember)
}

export async function inviteMember(boardId: string, email: string): Promise<Member> {
  const body = await apiFetch<MemberBody>(`/boards/${boardId}/members`, {
    method: 'POST',
    body: { email },
  })
  return toMember(body)
}

export async function removeMember(boardId: string, userId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/members/${userId}`, { method: 'DELETE' })
}
