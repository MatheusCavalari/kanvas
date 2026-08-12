import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateCard, deleteCard, type Card } from '../../api/cards'
import { boardKeys } from '../../lib/queryKeys'
import Modal from '../../components/ui/Modal'

interface CardDetailModalProps {
  card: Card
  boardId: string
  onClose: () => void
}

export default function CardDetailModal({ card, boardId, onClose }: CardDetailModalProps) {
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(card.title)
  const [description, setDescription] = useState(card.description)

  const updateMutation = useMutation({
    mutationFn: () => updateCard(card.id, title, description, card.assigneeId, card.dueDate),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
      onClose()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteCard(card.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
      onClose()
    },
  })

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    updateMutation.mutate()
  }

  return (
    <Modal title="Detalhes do card" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        {(updateMutation.isError || deleteMutation.isError) && (
          <p className="rounded bg-red-50 p-2 text-sm text-red-700">Não foi possível salvar as alterações.</p>
        )}
        <div>
          <label htmlFor="card-title" className="block text-sm font-medium text-gray-700">
            Título
          </label>
          <input
            id="card-title"
            type="text"
            required
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="card-description" className="block text-sm font-medium text-gray-700">
            Descrição
          </label>
          <textarea
            id="card-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={4}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="flex items-center justify-between">
          <button
            type="button"
            onClick={() => deleteMutation.mutate()}
            disabled={deleteMutation.isPending}
            className="rounded border border-red-300 px-3 py-1.5 text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
          >
            Excluir
          </button>
          <button
            type="submit"
            disabled={updateMutation.isPending}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white disabled:opacity-50"
          >
            Salvar
          </button>
        </div>
      </form>
    </Modal>
  )
}
