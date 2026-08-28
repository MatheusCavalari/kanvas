import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listLabels, listCardLabels, attachLabel, detachLabel } from '../../api/labels'
import { boardKeys, cardLabelKeys } from '../../lib/queryKeys'
import LabelChip from './LabelChip'

interface LabelPickerProps {
  cardId: string
  boardId: string
}

export default function LabelPicker({ cardId, boardId }: LabelPickerProps) {
  const queryClient = useQueryClient()
  const [isOpen, setIsOpen] = useState(false)

  const { data: boardLabels } = useQuery({
    queryKey: boardKeys.labels(boardId),
    queryFn: () => listLabels(boardId),
  })

  const { data: cardLabels } = useQuery({
    queryKey: cardLabelKeys.card(cardId),
    queryFn: () => listCardLabels(cardId),
  })

  const attachMutation = useMutation({
    mutationFn: (labelId: string) => attachLabel(cardId, labelId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cardLabelKeys.card(cardId) }),
  })

  const detachMutation = useMutation({
    mutationFn: (labelId: string) => detachLabel(cardId, labelId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cardLabelKeys.card(cardId) }),
  })

  const attachedIds = new Set((cardLabels ?? []).map((l) => l.id))

  function toggle(labelId: string) {
    if (attachedIds.has(labelId)) {
      detachMutation.mutate(labelId)
    } else {
      attachMutation.mutate(labelId)
    }
  }

  return (
    <div>
      <label className="block text-sm font-medium text-gray-700">Etiquetas</label>
      <div className="mt-1 flex flex-wrap items-center gap-1.5">
        {(cardLabels ?? []).map((label) => (
          <LabelChip key={label.id} name={label.name} color={label.color} onRemove={() => detachMutation.mutate(label.id)} />
        ))}
        <button
          type="button"
          onClick={() => setIsOpen((open) => !open)}
          className="rounded border border-dashed border-gray-300 px-2 py-0.5 text-xs text-gray-500 hover:border-gray-400"
        >
          + Etiqueta
        </button>
      </div>

      {isOpen && (
        <ul className="mt-2 max-h-40 space-y-1 overflow-y-auto rounded border border-gray-200 p-2">
          {(boardLabels ?? []).length === 0 && (
            <li className="text-xs text-gray-500">Nenhuma etiqueta neste board.</li>
          )}
          {boardLabels?.map((label) => (
            <li key={label.id}>
              <label className="flex cursor-pointer items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={attachedIds.has(label.id)}
                  onChange={() => toggle(label.id)}
                />
                <LabelChip name={label.name} color={label.color} />
              </label>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
