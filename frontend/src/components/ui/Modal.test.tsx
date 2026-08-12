import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Modal from './Modal'

describe('Modal', () => {
  it('renders the title and children', () => {
    render(
      <Modal title="New board" onClose={vi.fn()}>
        <p>Form goes here</p>
      </Modal>,
    )

    expect(screen.getByText('New board')).toBeInTheDocument()
    expect(screen.getByText('Form goes here')).toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.click(screen.getByRole('button', { name: /fechar/i }))

    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when the backdrop is clicked', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.click(screen.getByTestId('modal-backdrop'))

    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when Escape is pressed', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.keyboard('{Escape}')

    expect(onClose).toHaveBeenCalled()
  })
})
