import { apiFetch } from './client'

export interface ActivityEntry {
  id: string
  boardId: string
  actorId: string
  action: string
  entityType: string
  entityId: string
  createdAt: string
}

export interface ActivityBody {
  id: string
  board_id: string
  actor_id: string
  action: string
  entity_type: string
  entity_id: string
  created_at: string
}

export function toActivityEntry(body: ActivityBody): ActivityEntry {
  return {
    id: body.id,
    boardId: body.board_id,
    actorId: body.actor_id,
    action: body.action,
    entityType: body.entity_type,
    entityId: body.entity_id,
    createdAt: body.created_at,
  }
}

export async function listActivity(
  boardId: string,
  cursor?: string,
  limit = 50,
): Promise<{ entries: ActivityEntry[]; nextCursor: string | null }> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  const body = await apiFetch<{ entries: ActivityBody[]; next_cursor: string | null }>(
    `/boards/${boardId}/activity?${params}`,
  )
  return { entries: body.entries.map(toActivityEntry), nextCursor: body.next_cursor }
}
