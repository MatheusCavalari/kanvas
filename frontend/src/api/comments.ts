import { apiFetch } from './client'

export interface Comment {
  id: string
  cardId: string
  authorId: string
  body: string
  createdAt: string
  updatedAt: string
}

export interface CommentPage {
  comments: Comment[]
  nextCursor: string | null
}

interface CommentBody {
  id: string
  card_id: string
  author_id: string
  body: string
  created_at: string
  updated_at: string
}

interface ListCommentsBody {
  comments: CommentBody[]
  next_cursor?: string
}

function toComment(body: CommentBody): Comment {
  return {
    id: body.id,
    cardId: body.card_id,
    authorId: body.author_id,
    body: body.body,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

export async function listComments(cardId: string, cursor?: string, limit?: number): Promise<CommentPage> {
  const params = new URLSearchParams()
  if (cursor) params.set('cursor', cursor)
  if (limit) params.set('limit', String(limit))
  const query = params.toString()
  const body = await apiFetch<ListCommentsBody>(`/cards/${cardId}/comments${query ? `?${query}` : ''}`)
  return {
    comments: body.comments.map(toComment),
    nextCursor: body.next_cursor ?? null,
  }
}

export async function createComment(cardId: string, body: string): Promise<Comment> {
  return toComment(await apiFetch<CommentBody>(`/cards/${cardId}/comments`, { method: 'POST', body: { body } }))
}

export async function updateComment(commentId: string, body: string): Promise<Comment> {
  return toComment(await apiFetch<CommentBody>(`/comments/${commentId}`, { method: 'PATCH', body: { body } }))
}

export async function deleteComment(commentId: string): Promise<void> {
  await apiFetch<void>(`/comments/${commentId}`, { method: 'DELETE' })
}
