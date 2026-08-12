import type { Card } from '../../api/cards'

interface CardItemProps {
  card: Card
  onClick: () => void
}

export default function CardItem({ card, onClick }: CardItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full rounded border border-gray-200 bg-white p-3 text-left shadow-sm hover:border-blue-400"
    >
      <p className="font-medium text-gray-900">{card.title}</p>
      {card.description && (
        <p className="mt-1 line-clamp-2 text-sm text-gray-600">{card.description}</p>
      )}
      {card.dueDate && (
        <span className="mt-2 inline-block rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
          {card.dueDate.slice(0, 10)}
        </span>
      )}
    </button>
  )
}
