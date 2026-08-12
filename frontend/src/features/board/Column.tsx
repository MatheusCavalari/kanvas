import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { renameColumn, deleteColumn, type ColumnWithCards } from '../../api/columns'
import { createCard } from '../../api/cards'
import { boardKeys } from '../../lib/queryKeys'
import CardItem from './CardItem'
import CardDetailModal from './CardDetailModal'

interface ColumnProps {
  column: ColumnWithCards
  boardId: string
}

export default function Column({ column, boardId }: ColumnProps) {
  const queryClient = useQueryClient()
  const [menuOpen, setMenuOpen] = useState(false)
  const [isRenaming, setIsRenaming] = useState(false)
  const [title, setTitle] = useState(column.title)
  const [isAddingCard, setIsAddingCard] = useState(false)
  const [newCardTitle, setNewCardTitle] = useState('')
  const [selectedCardId, setSelectedCardId] = useState<string | null>(null)

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
  }

  const renameMutation = useMutation({
    mutationFn: (newTitle: string) => renameColumn(boardId, column.id, newTitle),
    onSuccess: () => {
      invalidate()
      setIsRenaming(false)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteColumn(boardId, column.id),
    onSuccess: invalidate,
  })

  const createCardMutation = useMutation({
    mutationFn: (cardTitle: string) => createCard(column.id, cardTitle),
    onSuccess: () => {
      invalidate()
      setIsAddingCard(false)
      setNewCardTitle('')
    },
  })

  function handleRenameSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    renameMutation.mutate(title)
  }

  function handleAddCardSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createCardMutation.mutate(newCardTitle)
  }

  const selectedCard = column.cards.find((c) => c.id === selectedCardId) ?? null

  return (
    <div className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">
      <div className="mb-2 flex items-center justify-between">
        {isRenaming ? (
          <form onSubmit={handleRenameSubmit} className="flex-1">
            <input
              type="text"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              onBlur={() => renameMutation.mutate(title)}
              autoFocus
              className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
            />
          </form>
        ) : (
          <h3 className="font-medium text-gray-900">{column.title}</h3>
        )}
        <div className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen((open) => !open)}
            aria-label="Opções da coluna"
            className="px-1 text-gray-500 hover:text-gray-800"
          >
            ⋯
          </button>
          {menuOpen && (
            <div className="absolute right-0 z-10 mt-1 w-36 rounded border border-gray-200 bg-white shadow">
              <button
                type="button"
                onClick={() => {
                  setIsRenaming(true)
                  setMenuOpen(false)
                }}
                className="block w-full px-3 py-2 text-left text-sm hover:bg-gray-50"
              >
                Renomear
              </button>
              <button
                type="button"
                onClick={() => {
                  deleteMutation.mutate()
                  setMenuOpen(false)
                }}
                className="block w-full px-3 py-2 text-left text-sm text-red-700 hover:bg-red-50"
              >
                Excluir coluna
              </button>
            </div>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        {column.cards.map((card) => (
          <CardItem key={card.id} card={card} onClick={() => setSelectedCardId(card.id)} />
        ))}
      </div>

      {isAddingCard ? (
        <form onSubmit={handleAddCardSubmit} className="mt-2 space-y-2">
          <label htmlFor={`new-card-${column.id}`} className="sr-only">
            Título do card
          </label>
          <input
            id={`new-card-${column.id}`}
            type="text"
            required
            value={newCardTitle}
            onChange={(event) => setNewCardTitle(event.target.value)}
            className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
            autoFocus
          />
          <div className="flex gap-2">
            <button
              type="submit"
              disabled={createCardMutation.isPending}
              className="rounded bg-blue-600 px-3 py-1 text-sm text-white disabled:opacity-50"
            >
              Adicionar
            </button>
            <button
              type="button"
              onClick={() => setIsAddingCard(false)}
              className="rounded px-3 py-1 text-sm text-gray-600 hover:bg-gray-200"
            >
              Cancelar
            </button>
          </div>
        </form>
      ) : (
        <button
          type="button"
          onClick={() => setIsAddingCard(true)}
          className="mt-2 rounded px-2 py-1 text-left text-sm text-gray-600 hover:bg-gray-200"
        >
          + Adicionar card
        </button>
      )}

      {selectedCard && (
        <CardDetailModal card={selectedCard} boardId={boardId} onClose={() => setSelectedCardId(null)} />
      )}
    </div>
  )
}
