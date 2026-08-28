import { useEffect } from 'react'
import { useQueryClient, type InfiniteData } from '@tanstack/react-query'
import { boardKeys, cardLabelKeys, commentKeys, activityKeys } from '../../lib/queryKeys'
import { getAccessToken } from '../../api/client'
import { env } from '../../lib/env'
import { toCard, type CardBody, type Card } from '../../api/cards'
import type { ColumnWithCards } from '../../api/columns'
import { toActivityEntry, type ActivityBody, type ActivityEntry } from '../../api/activity'
import type { PresenceUser } from '../../api/presence'

interface RealtimeEvent {
  type: string
  board_id: string
  data: unknown
}

interface ColumnEventPayload {
  id: string
  board_id: string
  title: string
  position: number
  created_at: string
  updated_at: string
}

interface ColumnDeletedPayload {
  id: string
  board_id: string
}

interface ColumnsReorderedPayload {
  board_id: string
  column_ids: string[]
}

interface CardDeletedPayload {
  id: string
  column_id: string
}

interface CardLabelPayload {
  card_id: string
  label_id: string
}

interface CommentPayload {
  id: string
  card_id: string
}

interface PresencePayload {
  user_id: string
  name: string
}

type ActivityPage = { entries: ActivityEntry[]; nextCursor: string | null }

const RECONNECT_DELAYS_MS = [1000, 2000, 4000, 8000, 10000]
const PING_INTERVAL_MS = 30000

function wsURL(boardId: string): string {
  const httpURL = new URL(`${env.API_URL}/boards/${boardId}/ws`)
  const wsProtocol = httpURL.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = getAccessToken() ?? ''
  return `${wsProtocol}//${httpURL.host}/boards/${boardId}/ws?token=${encodeURIComponent(token)}`
}

export function useBoardRealtime(boardId: string): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    let socket: WebSocket | null = null
    let reconnectAttempt = 0
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let pingTimer: ReturnType<typeof setInterval> | null = null
    let stopped = false

    function patchColumns(updater: (columns: ColumnWithCards[]) => ColumnWithCards[]) {
      queryClient.setQueryData<ColumnWithCards[]>(boardKeys.columns(boardId), (current) =>
        updater(current ?? []),
      )
    }

    function handleEvent(event: RealtimeEvent) {
      switch (event.type) {
        case 'column.created': {
          const payload = event.data as ColumnEventPayload
          patchColumns((columns) => {
            if (columns.some((column) => column.id === payload.id)) return columns
            return [
              ...columns,
              {
                id: payload.id,
                boardId: payload.board_id,
                title: payload.title,
                position: payload.position,
                createdAt: payload.created_at,
                updatedAt: payload.updated_at,
                cards: [],
              },
            ]
          })
          break
        }
        case 'column.updated': {
          const payload = event.data as ColumnEventPayload
          patchColumns((columns) =>
            columns.map((column) =>
              column.id === payload.id ? { ...column, title: payload.title } : column,
            ),
          )
          break
        }
        case 'column.deleted': {
          const payload = event.data as ColumnDeletedPayload
          patchColumns((columns) => columns.filter((column) => column.id !== payload.id))
          break
        }
        case 'column.reordered': {
          const payload = event.data as ColumnsReorderedPayload
          patchColumns((columns) => {
            const byId = new Map(columns.map((column) => [column.id, column]))
            return payload.column_ids
              .map((id) => byId.get(id))
              .filter((column): column is ColumnWithCards => column !== undefined)
          })
          break
        }
        case 'card.created': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) => {
            if (columns.some((column) => column.cards.some((c) => c.id === card.id))) {
              return columns
            }
            return columns.map((column) =>
              column.id === card.columnId ? { ...column, cards: [...column.cards, card] } : column,
            )
          })
          break
        }
        case 'card.updated': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) =>
            columns.map((column) =>
              column.id === card.columnId
                ? { ...column, cards: column.cards.map((c) => (c.id === card.id ? card : c)) }
                : column,
            ),
          )
          break
        }
        case 'card.deleted': {
          const payload = event.data as CardDeletedPayload
          patchColumns((columns) =>
            columns.map((column) => ({
              ...column,
              cards: column.cards.filter((c) => c.id !== payload.id),
            })),
          )
          break
        }
        case 'card.moved': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) => {
            const withoutCard = columns.map((column) => ({
              ...column,
              cards: column.cards.filter((c) => c.id !== card.id),
            }))
            return withoutCard.map((column) =>
              column.id === card.columnId
                ? { ...column, cards: insertAt(column.cards, card, card.position) }
                : column,
            )
          })
          break
        }
        case 'label.created':
        case 'label.updated':
        case 'label.deleted': {
          queryClient.invalidateQueries({ queryKey: boardKeys.labels(boardId) })
          break
        }
        case 'card.label_added':
        case 'card.label_removed': {
          const payload = event.data as CardLabelPayload
          queryClient.invalidateQueries({ queryKey: cardLabelKeys.card(payload.card_id) })
          break
        }
        case 'comment.created':
        case 'comment.updated':
        case 'comment.deleted': {
          const payload = event.data as CommentPayload
          queryClient.invalidateQueries({ queryKey: commentKeys.card(payload.card_id) })
          break
        }
        case 'presence.joined': {
          const payload = event.data as PresencePayload
          queryClient.setQueryData<PresenceUser[]>(boardKeys.presence(boardId), (current) => {
            const list = current ?? []
            if (list.some((user) => user.userId === payload.user_id)) return list
            return [...list, { userId: payload.user_id, name: payload.name }]
          })
          break
        }
        case 'presence.left': {
          const payload = event.data as PresencePayload
          queryClient.setQueryData<PresenceUser[]>(boardKeys.presence(boardId), (current) =>
            (current ?? []).filter((user) => user.userId !== payload.user_id),
          )
          break
        }
        case 'activity.created': {
          const entry = toActivityEntry(event.data as ActivityBody)
          queryClient.setQueryData<InfiniteData<ActivityPage>>(activityKeys.board(boardId), (current) => {
            if (!current) return current
            const [firstPage, ...restPages] = current.pages
            if (firstPage.entries.some((existing) => existing.id === entry.id)) return current
            return {
              ...current,
              pages: [{ ...firstPage, entries: [entry, ...firstPage.entries] }, ...restPages],
            }
          })
          break
        }
        default:
          break
      }
    }

    function connect() {
      if (stopped) return
      socket = new WebSocket(wsURL(boardId))
      socket.onmessage = (message) => {
        try {
          handleEvent(JSON.parse(message.data as string) as RealtimeEvent)
        } catch {
          // Ignore malformed frames rather than crashing the socket handler.
        }
      }
      socket.onopen = () => {
        const isReconnect = reconnectAttempt > 0
        reconnectAttempt = 0
        if (isReconnect) {
          void queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
        }
      }
      socket.onclose = () => {
        if (stopped) return
        const delay = RECONNECT_DELAYS_MS[Math.min(reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)]
        reconnectAttempt += 1
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

    pingTimer = setInterval(() => {
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'ping' }))
      }
    }, PING_INTERVAL_MS)

    return () => {
      stopped = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (pingTimer) clearInterval(pingTimer)
      socket?.close()
    }
  }, [boardId, queryClient])
}

function insertAt(cards: Card[], card: Card, index: number): Card[] {
  const clamped = Math.max(0, Math.min(index, cards.length))
  const next = [...cards]
  next.splice(clamped, 0, card)
  return next
}
