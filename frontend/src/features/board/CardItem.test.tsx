import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

describe('CardItem', () => {
  it('renders the title and a truncated description', () => {
    render(<CardItem card={card} onClick={vi.fn()} />)

    expect(screen.getByText('Write tests')).toBeInTheDocument()
    expect(screen.getByText(/cover the happy path/i)).toBeInTheDocument()
  })

  it('calls onClick when clicked', async () => {
    const onClick = vi.fn()
    render(<CardItem card={card} onClick={onClick} />)

    await userEvent.click(screen.getByText('Write tests'))

    expect(onClick).toHaveBeenCalled()
  })

  it('shows a due date badge when the card has one', () => {
    render(<CardItem card={{ ...card, dueDate: '2026-09-01T00:00:00Z' }} onClick={vi.fn()} />)

    expect(screen.getByText(/2026-09-01/)).toBeInTheDocument()
  })
})
