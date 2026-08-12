import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext } from '@dnd-kit/sortable'
import CardItem from './CardItem'
import type { Card } from '../../api/cards'

const card: Card = {
  id: 'card-1',
  columnId: 'col-1',
  title: 'Write tests',
  description: 'Cover the happy path and edge cases in detail',
  position: 0,
  assigneeId: null,
  dueDate: null,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

// Without an activationConstraint, dnd-kit's PointerSensor treats every pointerdown as an
// immediate drag start and swallows the subsequent click event (it registers a document-level
// click listener that stops propagation once a drag has begun). BoardPage configures a 5px
// activation distance for the same reason (see Step 5) — mirror that here so a plain click
// (no pointer movement) still reaches CardItem's onClick, matching real usage.
function Wrapper({ card, onClick }: { card: Card; onClick: () => void }) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  return (
    <DndContext sensors={sensors}>
      <SortableContext items={[card.id]}>
        <CardItem card={card} onClick={onClick} />
      </SortableContext>
    </DndContext>
  )
}

function renderCard(card: Card, onClick: () => void) {
  return render(<Wrapper card={card} onClick={onClick} />)
}

describe('CardItem', () => {
  it('renders the title and a truncated description', () => {
    renderCard(card, vi.fn())

    expect(screen.getByText('Write tests')).toBeInTheDocument()
    expect(screen.getByText(/cover the happy path/i)).toBeInTheDocument()
  })

  it('calls onClick when clicked', async () => {
    const onClick = vi.fn()
    renderCard(card, onClick)

    await userEvent.click(screen.getByText('Write tests'))

    expect(onClick).toHaveBeenCalled()
  })

  it('shows a due date badge when the card has one', () => {
    renderCard({ ...card, dueDate: '2026-09-01T00:00:00Z' }, vi.fn())

    expect(screen.getByText(/2026-09-01/)).toBeInTheDocument()
  })
})
