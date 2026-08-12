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

  it('does not append a duplicate card on a card.created echo for an id already in the cache', () => {
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
            title: 'Already here',
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
    ws.emit('card.created', {
      id: 'card-1',
      column_id: 'col-1',
      title: 'Already here',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(1)
  })

  it('does not append a duplicate column on a column.created echo for an id already in the cache', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-2', boardId: 'board-1', title: 'Done', position: 1, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

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
  })

  it('updates a card in place on card.updated', () => {
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
            title: 'Old title',
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
    ws.emit('card.updated', {
      id: 'card-1',
      column_id: 'col-1',
      title: 'New title',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(1)
    expect(data?.[0].cards[0].title).toBe('New title')
  })

  it('renames a column on column.updated', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('column.updated', {
      id: 'col-1',
      board_id: 'board-1',
      title: 'Doing',
      position: 0,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].title).toBe('Doing')
  })

  it('removes a column on column.deleted', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
      { id: 'col-2', boardId: 'board-1', title: 'Done', position: 1, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('column.deleted', { id: 'col-1', board_id: 'board-1' })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data).toHaveLength(1)
    expect(data?.[0].id).toBe('col-2')
  })

  it('reorders columns on column.reordered', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
      { id: 'col-2', boardId: 'board-1', title: 'Doing', position: 1, createdAt: '', updatedAt: '', cards: [] },
      { id: 'col-3', boardId: 'board-1', title: 'Done', position: 2, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('column.reordered', { board_id: 'board-1', column_ids: ['col-3', 'col-1', 'col-2'] })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.map((c) => c.id)).toEqual(['col-3', 'col-1', 'col-2'])
  })

  it('moves a card to a new column and position on card.moved', () => {
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
            title: 'Move me',
            description: '',
            position: 0,
            assigneeId: null,
            dueDate: null,
            createdAt: '',
            updatedAt: '',
          },
        ],
      },
      {
        id: 'col-2',
        boardId: 'board-1',
        title: 'Doing',
        position: 1,
        createdAt: '',
        updatedAt: '',
        cards: [
          {
            id: 'card-2',
            columnId: 'col-2',
            title: 'Already there',
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
    ws.emit('card.moved', {
      id: 'card-1',
      column_id: 'col-2',
      title: 'Move me',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(0)
    expect(data?.[1].cards.map((c) => c.id)).toEqual(['card-1', 'card-2'])
  })

  it('clamps an out-of-range position on card.moved to the end of the target column', () => {
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
            title: 'Move me',
            description: '',
            position: 0,
            assigneeId: null,
            dueDate: null,
            createdAt: '',
            updatedAt: '',
          },
        ],
      },
      {
        id: 'col-2',
        boardId: 'board-1',
        title: 'Doing',
        position: 1,
        createdAt: '',
        updatedAt: '',
        cards: [
          {
            id: 'card-2',
            columnId: 'col-2',
            title: 'Already there',
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
    ws.emit('card.moved', {
      id: 'card-1',
      column_id: 'col-2',
      title: 'Move me',
      description: '',
      position: 999,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[1].cards.map((c) => c.id)).toEqual(['card-2', 'card-1'])
  })

  it('ignores a malformed JSON frame without throwing or corrupting the cache', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]

    expect(() => ws.onmessage?.({ data: '{not valid json' })).not.toThrow()

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data).toEqual(initial)
  })

  it('invalidates the columns query on reconnect (onopen after a prior close)', () => {
    vi.useFakeTimers()
    try {
      queryClient.setQueryData(boardKeys.columns('board-1'), [] as ColumnWithCards[])
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      renderHook(() => useBoardRealtime('board-1'), { wrapper })
      const first = FakeWebSocket.instances[0]

      // Initial connection opening should not trigger a resync invalidation.
      first.onopen?.()
      expect(invalidateSpy).not.toHaveBeenCalled()

      // Simulate a drop; the hook schedules a reconnect via setTimeout.
      first.onclose?.()
      vi.runOnlyPendingTimers()
      expect(FakeWebSocket.instances).toHaveLength(2)
      const second = FakeWebSocket.instances[1]
      second.onopen?.()

      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: boardKeys.columns('board-1') })
    } finally {
      vi.useRealTimers()
    }
  })

  it('closes the socket on unmount', () => {
    const { unmount } = renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]

    unmount()

    expect(ws.closed).toBe(true)
  })
})
