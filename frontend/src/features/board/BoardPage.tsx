import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  closestCorners,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { boardKeys } from '../../lib/queryKeys'
import { listColumns, createColumn, reorderColumns, type ColumnWithCards } from '../../api/columns'
import { moveCard } from '../../api/cards'
import Column from './Column'
import CardItem from './CardItem'
import { useBoardRealtime } from './useBoardRealtime'

export default function BoardPage() {
  const { boardId } = useParams<{ boardId: string }>()
  const queryClient = useQueryClient()
  useBoardRealtime(boardId ?? '')
  const [isAddingColumn, setIsAddingColumn] = useState(false)
  const [newColumnTitle, setNewColumnTitle] = useState('')

  const { data: columns, isPending, isError, refetch } = useQuery({
    queryKey: boardKeys.columns(boardId ?? ''),
    queryFn: () => listColumns(boardId ?? ''),
    enabled: Boolean(boardId),
  })

  const createColumnMutation = useMutation({
    mutationFn: (title: string) => createColumn(boardId ?? '', title),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId ?? '') })
      setIsAddingColumn(false)
      setNewColumnTitle('')
    },
  })

  function handleAddColumnSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createColumnMutation.mutate(newColumnTitle)
  }

  const [activeCard, setActiveCard] = useState<ColumnWithCards['cards'][number] | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  const moveCardMutation = useMutation({
    mutationFn: ({ cardId, targetColumnId, position }: { cardId: string; targetColumnId: string; position: number }) =>
      moveCard(cardId, targetColumnId, position),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId ?? '') }),
  })

  const reorderColumnsMutation = useMutation({
    mutationFn: (columnIds: string[]) => reorderColumns(boardId ?? '', columnIds),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId ?? '') }),
  })

  function handleDragStart(event: DragStartEvent) {
    if (event.active.data.current?.type !== 'card') return
    const card = columns?.flatMap((c) => c.cards).find((c) => c.id === event.active.id)
    setActiveCard(card ?? null)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveCard(null)
    const { active, over } = event
    if (!over || !columns) return

    if (active.data.current?.type === 'column') {
      const oldIndex = columns.findIndex((c) => c.id === active.id)
      const newIndex = columns.findIndex((c) => c.id === over.id)
      if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) return
      const reordered = arrayMove(columns, oldIndex, newIndex)
      reorderColumnsMutation.mutate(reordered.map((c) => c.id))
      return
    }

    if (active.data.current?.type === 'card') {
      const cardId = String(active.id)
      const overData = over.data.current as { type: string; columnId?: string } | undefined
      const targetColumnId =
        overData?.type === 'column'
          ? overData.columnId!
          : (columns.find((c) => c.cards.some((card) => card.id === over.id))?.id ?? null)
      if (!targetColumnId) return

      const targetColumn = columns.find((c) => c.id === targetColumnId)
      if (!targetColumn) return

      const overIndex = targetColumn.cards.findIndex((card) => card.id === over.id)
      const position = overIndex === -1 ? targetColumn.cards.length : overIndex

      moveCardMutation.mutate({ cardId, targetColumnId, position })
    }
  }

  if (!boardId) {
    return null
  }

  if (isPending) {
    return (
      <div className="flex gap-4">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-64 w-72 shrink-0 animate-pulse rounded-lg bg-gray-200" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-red-700">Não foi possível carregar as colunas.</p>
        <button
          type="button"
          onClick={() => refetch()}
          className="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-100"
        >
          Tentar novamente
        </button>
      </div>
    )
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex gap-4 overflow-x-auto pb-4">
        <SortableContext items={columns.map((c) => c.id)} strategy={horizontalListSortingStrategy}>
          {columns.map((column) => (
            <Column key={column.id} column={column} boardId={boardId} />
          ))}
        </SortableContext>

        <div className="w-72 shrink-0">
          {isAddingColumn ? (
            <form onSubmit={handleAddColumnSubmit} className="space-y-2 rounded-lg bg-gray-100 p-3">
              <label htmlFor="new-column-title" className="sr-only">
                Título da coluna
              </label>
              <input
                id="new-column-title"
                type="text"
                required
                value={newColumnTitle}
                onChange={(event) => setNewColumnTitle(event.target.value)}
                className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
                autoFocus
              />
              <div className="flex gap-2">
                <button
                  type="submit"
                  disabled={createColumnMutation.isPending}
                  className="rounded bg-blue-600 px-3 py-1 text-sm text-white disabled:opacity-50"
                >
                  Adicionar
                </button>
                <button
                  type="button"
                  onClick={() => setIsAddingColumn(false)}
                  className="rounded px-3 py-1 text-sm text-gray-600 hover:bg-gray-200"
                >
                  Cancelar
                </button>
              </div>
            </form>
          ) : (
            <button
              type="button"
              onClick={() => setIsAddingColumn(true)}
              className="w-full rounded-lg border-2 border-dashed border-gray-300 p-3 text-sm text-gray-500 hover:border-gray-400"
            >
              + Adicionar coluna
            </button>
          )}
        </div>
      </div>

      <DragOverlay>{activeCard && <CardItem card={activeCard} onClick={() => {}} />}</DragOverlay>
    </DndContext>
  )
}
