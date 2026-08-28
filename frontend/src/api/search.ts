import { apiFetch } from './client'

export interface SearchCardResult {
  id: string
  title: string
  columnId: string
  rank: number
}

export interface SearchCommentResult {
  id: string
  cardId: string
  bodyExcerpt: string
  rank: number
}

export interface SearchResults {
  cards: SearchCardResult[]
  comments: SearchCommentResult[]
}

interface SearchCardResultBody {
  id: string
  title: string
  column_id: string
  rank: number
}

interface SearchCommentResultBody {
  id: string
  card_id: string
  body_excerpt: string
  rank: number
}

interface SearchResponseBody {
  cards?: SearchCardResultBody[]
  comments?: SearchCommentResultBody[]
}

export type SearchType = 'cards' | 'comments' | 'all'

export async function search(boardId: string, q: string, type: SearchType = 'all'): Promise<SearchResults> {
  const params = new URLSearchParams({ q, type })
  const body = await apiFetch<SearchResponseBody>(`/boards/${boardId}/search?${params.toString()}`)
  return {
    cards: (body.cards ?? []).map((c) => ({ id: c.id, title: c.title, columnId: c.column_id, rank: c.rank })),
    comments: (body.comments ?? []).map((c) => ({
      id: c.id,
      cardId: c.card_id,
      bodyExcerpt: c.body_excerpt,
      rank: c.rank,
    })),
  }
}
