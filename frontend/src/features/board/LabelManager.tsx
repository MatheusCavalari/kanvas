import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listLabels, createLabel, updateLabel, deleteLabel, type Label } from '../../api/labels'
import { boardKeys } from '../../lib/queryKeys'
import Modal from '../../components/ui/Modal'
import LabelChip from './LabelChip'

interface LabelManagerProps {
  boardId: string
  onClose: () => void
}

const DEFAULT_COLOR = '#2563eb'

export default function LabelManager({ boardId, onClose }: LabelManagerProps) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [color, setColor] = useState(DEFAULT_COLOR)
  const [editingId, setEditingId] = useState<string | null>(null)

  const { data: labels, isPending } = useQuery({
    queryKey: boardKeys.labels(boardId),
    queryFn: () => listLabels(boardId),
  })

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: boardKeys.labels(boardId) })
  }

  const createMutation = useMutation({
    mutationFn: () => createLabel(boardId, name, color),
    onSuccess: () => {
      invalidate()
      resetForm()
    },
  })

  const updateMutation = useMutation({
    mutationFn: (labelId: string) => updateLabel(boardId, labelId, name, color),
    onSuccess: () => {
      invalidate()
      resetForm()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (labelId: string) => deleteLabel(boardId, labelId),
    onSuccess: () => invalidate(),
  })

  function resetForm() {
    setName('')
    setColor(DEFAULT_COLOR)
    setEditingId(null)
  }

  function startEdit(label: Label) {
    setEditingId(label.id)
    setName(label.name)
    setColor(label.color)
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (editingId) {
      updateMutation.mutate(editingId)
    } else {
      createMutation.mutate()
    }
  }

  const isSaving = createMutation.isPending || updateMutation.isPending

  return (
    <Modal title="Gerenciar etiquetas" onClose={onClose}>
      {isPending ? (
        <p className="text-sm text-gray-500">Carregando...</p>
      ) : (
        <ul className="space-y-2">
          {labels?.map((label) => (
            <li key={label.id} className="flex items-center justify-between rounded border border-gray-100 px-3 py-2">
              <LabelChip name={label.name} color={label.color} />
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => startEdit(label)}
                  className="text-sm text-blue-700 hover:underline"
                >
                  Editar
                </button>
                <button
                  type="button"
                  onClick={() => deleteMutation.mutate(label.id)}
                  className="text-sm text-red-700 hover:underline"
                >
                  Excluir
                </button>
              </div>
            </li>
          ))}
          {labels?.length === 0 && <li className="text-sm text-gray-500">Nenhuma etiqueta ainda.</li>}
        </ul>
      )}

      <form onSubmit={handleSubmit} className="mt-4 flex items-end gap-2">
        <div className="flex-1">
          <label htmlFor="label-name" className="block text-sm font-medium text-gray-700">
            Nome
          </label>
          <input
            id="label-name"
            type="text"
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="label-color" className="block text-sm font-medium text-gray-700">
            Cor
          </label>
          <input
            id="label-color"
            type="color"
            value={color}
            onChange={(event) => setColor(event.target.value)}
            className="mt-1 h-9 w-12 rounded border border-gray-300"
          />
        </div>
        <button
          type="submit"
          disabled={isSaving}
          className="rounded bg-blue-600 px-3 py-2 text-sm text-white disabled:opacity-50"
        >
          {editingId ? 'Salvar' : 'Adicionar'}
        </button>
        {editingId && (
          <button type="button" onClick={resetForm} className="rounded px-3 py-2 text-sm text-gray-600 hover:bg-gray-100">
            Cancelar
          </button>
        )}
      </form>
      {(createMutation.isError || updateMutation.isError || deleteMutation.isError) && (
        <p className="mt-2 text-sm text-red-700">Não foi possível salvar a etiqueta.</p>
      )}
    </Modal>
  )
}
