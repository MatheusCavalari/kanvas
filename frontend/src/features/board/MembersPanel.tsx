import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listMembers, inviteMember, removeMember } from '../../api/boards'
import Modal from '../../components/ui/Modal'

interface MembersPanelProps {
  boardId: string
  currentUserId: string
  onClose: () => void
}

export default function MembersPanel({ boardId, currentUserId, onClose }: MembersPanelProps) {
  const queryClient = useQueryClient()
  const [email, setEmail] = useState('')

  const { data: members, isPending } = useQuery({
    queryKey: boardKeys.members(boardId),
    queryFn: () => listMembers(boardId),
  })

  const currentMember = members?.find((m) => m.userId === currentUserId)
  const isOwner = currentMember?.role === 'owner'

  const inviteMutation = useMutation({
    mutationFn: (inviteEmail: string) => inviteMember(boardId, inviteEmail),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.members(boardId) })
      setEmail('')
    },
  })

  const removeMutation = useMutation({
    mutationFn: (userId: string) => removeMember(boardId, userId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.members(boardId) }),
  })

  function handleInviteSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    inviteMutation.mutate(email)
  }

  return (
    <Modal title="Membros" onClose={onClose}>
      {isPending ? (
        <p className="text-sm text-gray-500">Carregando...</p>
      ) : (
        <ul className="space-y-2">
          {members?.map((m) => (
            <li key={m.userId} className="flex items-center justify-between rounded border border-gray-100 px-3 py-2">
              <div>
                <p className="text-sm font-medium text-gray-900">{m.name}</p>
                <p className="text-xs text-gray-500">
                  <span>{m.email}</span> · <span>{m.role === 'owner' ? 'dono' : 'membro'}</span>
                </p>
              </div>
              {isOwner && m.role !== 'owner' && (
                <button
                  type="button"
                  onClick={() => removeMutation.mutate(m.userId)}
                  aria-label={`Remover ${m.name}`}
                  className="text-sm text-red-700 hover:underline"
                >
                  Remover
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {isOwner && (
        <form onSubmit={handleInviteSubmit} className="mt-4 flex items-end gap-2">
          <div className="flex-1">
            <label htmlFor="invite-email" className="block text-sm font-medium text-gray-700">
              E-mail
            </label>
            <input
              id="invite-email"
              type="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
            />
          </div>
          <button
            type="submit"
            disabled={inviteMutation.isPending}
            className="rounded bg-blue-600 px-3 py-2 text-sm text-white disabled:opacity-50"
          >
            Convidar
          </button>
        </form>
      )}
      {inviteMutation.isError && (
        <p className="mt-2 text-sm text-red-700">Não foi possível convidar esse e-mail.</p>
      )}
    </Modal>
  )
}
