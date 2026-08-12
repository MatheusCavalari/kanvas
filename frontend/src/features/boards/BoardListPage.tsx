import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listBoards, createBoard } from '../../api/boards'
import Modal from '../../components/ui/Modal'

export default function BoardListPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [name, setName] = useState('')

  const { data: boards, isPending, isError, refetch } = useQuery({
    queryKey: boardKeys.list(),
    queryFn: listBoards,
  })

  const createMutation = useMutation({
    mutationFn: (boardName: string) => createBoard(boardName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.list() })
      setIsCreating(false)
      setName('')
    },
  })

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createMutation.mutate(name)
  }

  if (isPending) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-24 animate-pulse rounded-lg bg-gray-200" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-red-700">Não foi possível carregar os boards.</p>
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
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Seus boards</h1>
        <button
          type="button"
          onClick={() => setIsCreating(true)}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
        >
          Novo board
        </button>
      </div>

      {boards.length === 0 ? (
        <p className="text-sm text-gray-600">Nenhum board ainda — crie o primeiro.</p>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {boards.map((board) => (
            <Link
              key={board.id}
              to={`/boards/${board.id}`}
              className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm hover:border-blue-400 hover:shadow"
            >
              <h2 className="font-medium text-gray-900">{board.name}</h2>
            </Link>
          ))}
        </div>
      )}

      {isCreating && (
        <Modal title="Novo board" onClose={() => setIsCreating(false)}>
          <form onSubmit={handleSubmit} className="space-y-4">
            {createMutation.isError && (
              <p className="rounded bg-red-50 p-2 text-sm text-red-700">Não foi possível criar o board.</p>
            )}
            <div>
              <label htmlFor="board-name" className="block text-sm font-medium text-gray-700">
                Nome
              </label>
              <input
                id="board-name"
                type="text"
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
                className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
              />
            </div>
            <button
              type="submit"
              disabled={createMutation.isPending}
              className="w-full rounded bg-blue-600 py-2 text-white disabled:opacity-50"
            >
              Criar
            </button>
          </form>
        </Modal>
      )}
    </div>
  )
}
