import { useEffect, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { search } from '../../api/search'
import { boardKeys } from '../../lib/queryKeys'

interface SearchBarProps {
  boardId: string
}

const DEBOUNCE_MS = 300

/**
 * Renders a `ts_headline` excerpt that may contain `<b>`/`</b>` highlight
 * markers. Never uses dangerouslySetInnerHTML — every other character
 * (including any other tag the backend might emit) is rendered as plain
 * text via JSX, which React escapes automatically.
 */
function renderExcerpt(excerpt: string): ReactNode {
  const parts = excerpt.split(/(<b>|<\/b>)/g)
  let bold = false
  return parts.map((part, index) => {
    if (part === '<b>') {
      bold = true
      return null
    }
    if (part === '</b>') {
      bold = false
      return null
    }
    if (!part) return null
    return bold ? <b key={index}>{part}</b> : <span key={index}>{part}</span>
  })
}

function focusCard(cardId: string) {
  const el = document.querySelector<HTMLElement>(`[data-card-id="${cardId}"]`)
  el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el?.focus()
}

export default function SearchBar({ boardId }: SearchBarProps) {
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query.trim()), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [query])

  const enabled = debouncedQuery.length >= 2

  const { data, isFetching } = useQuery({
    queryKey: boardKeys.search(boardId, debouncedQuery),
    queryFn: () => search(boardId, debouncedQuery),
    enabled,
  })

  const showDropdown = isOpen && enabled

  return (
    <div className="relative w-64">
      <label htmlFor="board-search" className="sr-only">
        Buscar no board
      </label>
      <input
        id="board-search"
        type="search"
        value={query}
        onChange={(event) => {
          setQuery(event.target.value)
          setIsOpen(true)
        }}
        onFocus={() => setIsOpen(true)}
        onBlur={() => setTimeout(() => setIsOpen(false), 150)}
        placeholder="Buscar cards e comentários..."
        className="w-full rounded border border-gray-300 px-3 py-1.5 text-sm"
      />

      {showDropdown && (
        <div className="absolute right-0 top-full z-20 mt-1 max-h-80 w-80 overflow-y-auto rounded border border-gray-200 bg-white p-2 shadow-lg">
          {isFetching && <p className="p-2 text-sm text-gray-500">Buscando...</p>}
          {!isFetching && (data?.cards.length ?? 0) === 0 && (data?.comments.length ?? 0) === 0 && (
            <p className="p-2 text-sm text-gray-500">Nenhum resultado.</p>
          )}

          {(data?.cards.length ?? 0) > 0 && (
            <div className="mb-2">
              <p className="px-2 text-xs font-semibold uppercase text-gray-400">Cards</p>
              <ul>
                {data?.cards.map((result) => (
                  <li key={result.id}>
                    <button
                      type="button"
                      onMouseDown={() => focusCard(result.id)}
                      className="block w-full rounded px-2 py-1.5 text-left text-sm text-gray-800 hover:bg-gray-100"
                    >
                      {result.title}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {(data?.comments.length ?? 0) > 0 && (
            <div>
              <p className="px-2 text-xs font-semibold uppercase text-gray-400">Comentários</p>
              <ul>
                {data?.comments.map((result) => (
                  <li key={result.id}>
                    <button
                      type="button"
                      onMouseDown={() => focusCard(result.cardId)}
                      className="block w-full rounded px-2 py-1.5 text-left text-sm text-gray-800 hover:bg-gray-100"
                    >
                      {renderExcerpt(result.bodyExcerpt)}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
