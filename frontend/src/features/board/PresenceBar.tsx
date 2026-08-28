import { useQuery } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listPresence } from '../../api/presence'

interface PresenceBarProps {
  boardId: string
}

function initials(name: string): string {
  return name
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}

export default function PresenceBar({ boardId }: PresenceBarProps) {
  const { data: users } = useQuery({
    queryKey: boardKeys.presence(boardId),
    queryFn: () => listPresence(boardId),
    enabled: Boolean(boardId),
  })

  if (!users || users.length === 0) {
    return null
  }

  return (
    <div className="flex items-center -space-x-2" aria-label="Usuários online">
      {users.map((user) => (
        <div
          key={user.userId}
          title={user.name}
          className="relative flex h-8 w-8 items-center justify-center rounded-full border-2 border-white bg-blue-600 text-xs font-medium text-white"
        >
          {initials(user.name)}
          <span
            aria-hidden="true"
            className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-white bg-green-500"
          />
        </div>
      ))}
    </div>
  )
}
