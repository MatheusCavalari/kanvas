import { apiFetch } from './client'

export interface Card {
  id: string
  columnId: string
  title: string
  description: string
  position: number
  assigneeId: string | null
  dueDate: string | null
  createdAt: string
  updatedAt: string
}

export interface CardBody {
  id: string
  column_id: string
  title: string
  description: string
  position: number
  assignee_id: string | null
  due_date: string | null
  created_at: string
  updated_at: string
}

export function toCard(body: CardBody): Card {
  return {
    id: body.id,
    columnId: body.column_id,
    title: body.title,
    description: body.description,
    position: body.position,
    assigneeId: body.assignee_id,
    dueDate: body.due_date,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

export async function createCard(columnId: string, title: string, description = ''): Promise<Card> {
  const body = await apiFetch<CardBody>('/cards', {
    method: 'POST',
    body: { column_id: columnId, title, description },
  })
  return toCard(body)
}

export async function updateCard(
  cardId: string,
  title: string,
  description: string,
  assigneeId: string | null,
  dueDate: string | null,
): Promise<Card> {
  const body = await apiFetch<CardBody>(`/cards/${cardId}`, {
    method: 'PATCH',
    body: { title, description, assignee_id: assigneeId, due_date: dueDate },
  })
  return toCard(body)
}

export async function deleteCard(cardId: string): Promise<void> {
  await apiFetch<void>(`/cards/${cardId}`, { method: 'DELETE' })
}

export async function moveCard(cardId: string, columnId: string, position: number): Promise<Card> {
  const body = await apiFetch<CardBody>(`/cards/${cardId}/move`, {
    method: 'PATCH',
    body: { column_id: columnId, position },
  })
  return toCard(body)
}
