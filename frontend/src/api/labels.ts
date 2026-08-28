import { apiFetch } from './client'

export interface Label {
  id: string
  boardId: string
  name: string
  color: string
  createdAt: string
}

interface LabelBody {
  id: string
  board_id: string
  name: string
  color: string
  created_at: string
}

function toLabel(body: LabelBody): Label {
  return { id: body.id, boardId: body.board_id, name: body.name, color: body.color, createdAt: body.created_at }
}

export async function createLabel(boardId: string, name: string, color: string): Promise<Label> {
  return toLabel(await apiFetch<LabelBody>(`/boards/${boardId}/labels`, { method: 'POST', body: { name, color } }))
}

export async function listLabels(boardId: string): Promise<Label[]> {
  const bodies = await apiFetch<LabelBody[]>(`/boards/${boardId}/labels`)
  return bodies.map(toLabel)
}

export async function updateLabel(boardId: string, labelId: string, name: string, color: string): Promise<Label> {
  return toLabel(await apiFetch<LabelBody>(`/boards/${boardId}/labels/${labelId}`, { method: 'PATCH', body: { name, color } }))
}

export async function deleteLabel(boardId: string, labelId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/labels/${labelId}`, { method: 'DELETE' })
}

export async function attachLabel(cardId: string, labelId: string): Promise<void> {
  await apiFetch<void>(`/cards/${cardId}/labels`, { method: 'POST', body: { label_id: labelId } })
}

export async function detachLabel(cardId: string, labelId: string): Promise<void> {
  await apiFetch<void>(`/cards/${cardId}/labels/${labelId}`, { method: 'DELETE' })
}

export async function listCardLabels(cardId: string): Promise<Label[]> {
  const bodies = await apiFetch<LabelBody[]>(`/cards/${cardId}/labels`)
  return bodies.map(toLabel)
}
