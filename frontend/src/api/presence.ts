import { apiFetch } from './client'

export interface PresenceUser {
  userId: string
  name: string
}

interface PresenceBody {
  user_id: string
  name: string
}

function toPresenceUser(body: PresenceBody): PresenceUser {
  return { userId: body.user_id, name: body.name }
}

export async function listPresence(boardId: string): Promise<PresenceUser[]> {
  const body = await apiFetch<PresenceBody[]>(`/boards/${boardId}/presence`)
  return body.map(toPresenceUser)
}
