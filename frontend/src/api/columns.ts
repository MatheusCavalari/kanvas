import { apiFetch } from './client'
import { toCard, type Card, type CardBody } from './cards'

export interface Column {
  id: string
  boardId: string
  title: string
  position: number
  createdAt: string
  updatedAt: string
}

export interface ColumnWithCards extends Column {
  cards: Card[]
}

interface ColumnBody {
  id: string
  board_id: string
  title: string
  position: number
  created_at: string
  updated_at: string
  cards?: CardBody[]
}

function toColumn(body: ColumnBody): Column {
  return {
    id: body.id,
    boardId: body.board_id,
    title: body.title,
    position: body.position,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

function toColumnWithCards(body: ColumnBody): ColumnWithCards {
  return { ...toColumn(body), cards: (body.cards ?? []).map(toCard) }
}

export async function listColumns(boardId: string): Promise<ColumnWithCards[]> {
  const body = await apiFetch<ColumnBody[]>(`/boards/${boardId}/columns`)
  return body.map(toColumnWithCards)
}

export async function createColumn(boardId: string, title: string): Promise<Column> {
  const body = await apiFetch<ColumnBody>(`/boards/${boardId}/columns`, {
    method: 'POST',
    body: { title },
  })
  return toColumn(body)
}

export async function renameColumn(boardId: string, columnId: string, title: string): Promise<Column> {
  const body = await apiFetch<ColumnBody>(`/boards/${boardId}/columns/${columnId}`, {
    method: 'PATCH',
    body: { title },
  })
  return toColumn(body)
}

export async function deleteColumn(boardId: string, columnId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/columns/${columnId}`, { method: 'DELETE' })
}

export async function reorderColumns(boardId: string, columnIds: string[]): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/columns/reorder`, {
    method: 'PATCH',
    body: { column_ids: columnIds },
  })
}
