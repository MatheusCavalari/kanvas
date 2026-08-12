import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listColumns, createColumn } from '../../api/columns'
import Column from './Column'

export default function BoardPage() {
  const { boardId } = useParams<{ boardId: string }>()
  const queryClient = useQueryClient()
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
    <div className="flex gap-4 overflow-x-auto pb-4">
      {columns.map((column) => (
        <Column key={column.id} column={column} boardId={boardId} />
      ))}

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
  )
}
