import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement, ReactNode } from 'react'
import { useBoardRealtime } from './useBoardRealtime'
import { boardKeys } from '../../lib/queryKeys'
import type { ColumnWithCards } from '../../api/columns'

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, getAccessToken: vi.fn(() => 'test-token') }
})

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
    this.onclose?.()
  }

  emit(type: string, data: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type, board_id: 'board-1', data }) })
  }
}

describe('useBoardRealtime', () => {
  let queryClient: QueryClient
  let wrapper: ({ children }: { children: ReactNode }) => ReactElement

  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.stubGlobal('WebSocket', FakeWebSocket)
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    wrapper = ({ children }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens a WebSocket to the board endpoint with the access token', () => {
    renderHook(() => useBoardRealtime('board-1'), { wrapper })

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/boards/board-1/ws?token=test-token')
  })

  it('appends a card on card.created', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('card.created', {
      id: 'card-1',
      column_id: 'col-1',
      title: 'New card',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(1)
    expect(data?.[0].cards[0].title).toBe('New card')
  })

  it('removes a card on card.deleted', () => {
    const initial: ColumnWithCards[] = [
      {
        id: 'col-1',
        boardId: 'board-1',
        title: 'To do',
        position: 0,
        createdAt: '',
        updatedAt: '',
        cards: [
          {
            id: 'card-1',
            columnId: 'col-1',
            title: 'Old card',
            description: '',
            position: 0,
            assigneeId: null,
            dueDate: null,
            createdAt: '',
            updatedAt: '',
          },
        ],
      },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('card.deleted', { id: 'card-1', column_id: 'col-1' })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(0)
  })

  it('appends a column on column.created', () => {
    queryClient.setQueryData(boardKeys.columns('board-1'), [] as ColumnWithCards[])

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('column.created', {
      id: 'col-2',
      board_id: 'board-1',
      title: 'Done',
      position: 1,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data).toHaveLength(1)
    expect(data?.[0].title).toBe('Done')
  })

  it('closes the socket on unmount', () => {
    const { unmount } = renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]

    unmount()

    expect(ws.closed).toBe(true)
  })
})
