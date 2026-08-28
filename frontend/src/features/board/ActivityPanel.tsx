import { useEffect, useRef } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { activityKeys } from '../../lib/queryKeys'
import { listActivity, type ActivityEntry } from '../../api/activity'

interface ActivityPanelProps {
  boardId: string
  onClose: () => void
}

const ACTION_ICONS: Record<string, string> = {
  'column.created': '➕',
  'column.updated': '✏️',
  'column.deleted': '🗑️',
  'card.created': '📝',
  'card.updated': '✏️',
  'card.deleted': '🗑️',
  'card.moved': '↔️',
  'comment.created': '💬',
  'comment.updated': '💬',
  'comment.deleted': '💬',
  'label.created': '🏷️',
  'label.updated': '🏷️',
  'label.deleted': '🏷️',
  'member.invited': '👤',
  'member.removed': '👤',
}

const ACTION_LABELS: Record<string, string> = {
  'column.created': 'criou a coluna',
  'column.updated': 'atualizou a coluna',
  'column.deleted': 'removeu a coluna',
  'card.created': 'criou o cartão',
  'card.updated': 'atualizou o cartão',
  'card.deleted': 'removeu o cartão',
  'card.moved': 'moveu o cartão',
  'comment.created': 'comentou no cartão',
  'comment.updated': 'editou um comentário no cartão',
  'comment.deleted': 'removeu um comentário do cartão',
  'label.created': 'criou a etiqueta',
  'label.updated': 'atualizou a etiqueta',
  'label.deleted': 'removeu a etiqueta',
  'member.invited': 'convidou um membro para o cartão',
  'member.removed': 'removeu um membro do cartão',
}

function describeAction(entry: ActivityEntry): string {
  const label = ACTION_LABELS[entry.action] ?? entry.action
  return `${label} ${entry.entityType}`
}

function formatRelativeTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const diffMs = Date.now() - date.getTime()
  const diffSec = Math.round(diffMs / 1000)
  if (diffSec < 60) return 'agora mesmo'
  const diffMin = Math.round(diffSec / 60)
  if (diffMin < 60) return `há ${diffMin} min`
  const diffHour = Math.round(diffMin / 60)
  if (diffHour < 24) return `há ${diffHour} h`
  const diffDay = Math.round(diffHour / 24)
  return `há ${diffDay} d`
}

const SCROLL_THRESHOLD_PX = 100

export default function ActivityPanel({ boardId, onClose }: ActivityPanelProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } = useInfiniteQuery({
    queryKey: activityKeys.board(boardId),
    queryFn: ({ pageParam }: { pageParam: string | undefined }) => listActivity(boardId, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: Boolean(boardId),
  })

  const entries = data?.pages.flatMap((page) => page.entries) ?? []

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  function handleScroll() {
    const el = scrollRef.current
    if (!el || isFetchingNextPage || !hasNextPage) return
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    if (distanceFromBottom < SCROLL_THRESHOLD_PX) {
      void fetchNextPage()
    }
  }

  return (
    <div
      data-testid="activity-backdrop"
      className="fixed inset-0 z-50 flex justify-end bg-black/40"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Atividade"
        className="flex h-full w-full max-w-sm flex-col bg-white shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-gray-100 p-4">
          <h2 className="text-lg font-semibold text-gray-900">Atividade</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Fechar"
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>

        <div ref={scrollRef} onScroll={handleScroll} className="flex-1 space-y-3 overflow-y-auto p-4">
          {isPending && <p className="text-sm text-gray-500">Carregando...</p>}
          {!isPending && entries.length === 0 && (
            <p className="text-sm text-gray-500">Nenhuma atividade ainda.</p>
          )}
          {entries.map((entry) => (
            <div key={entry.id} className="flex items-start gap-2 text-sm">
              <span aria-hidden="true">{ACTION_ICONS[entry.action] ?? '•'}</span>
              <div>
                <p className="text-gray-900">{describeAction(entry)}</p>
                <p className="text-xs text-gray-500">{formatRelativeTime(entry.createdAt)}</p>
              </div>
            </div>
          ))}
          {isFetchingNextPage && <p className="text-xs text-gray-400">Carregando mais...</p>}
        </div>
      </div>
    </div>
  )
}
