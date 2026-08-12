import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MembersPanel from './MembersPanel'
import * as boardsApi from '../../api/boards'

vi.mock('../../api/boards', async () => {
  const actual = await vi.importActual<typeof import('../../api/boards')>('../../api/boards')
  return { ...actual, listMembers: vi.fn(), inviteMember: vi.fn(), removeMember: vi.fn() }
})

const owner = { userId: 'user-1', role: 'owner' as const, name: 'Owner Person', email: 'owner@example.com', createdAt: '' }
const member = { userId: 'user-2', role: 'member' as const, name: 'Ada Lovelace', email: 'ada@example.com', createdAt: '' }

function renderWithProviders(currentUserId = 'user-1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MembersPanel boardId="board-1" currentUserId={currentUserId} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('MembersPanel', () => {
  beforeEach(() => {
    vi.mocked(boardsApi.listMembers).mockReset()
    vi.mocked(boardsApi.inviteMember).mockReset()
    vi.mocked(boardsApi.removeMember).mockReset()
  })

  it('renders each member with name, email, and role', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders()

    expect(await screen.findByText('Owner Person')).toBeInTheDocument()
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
  })

  it('lets the owner invite a member by email', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner]);
    vi.mocked(boardsApi.inviteMember).mockResolvedValue(member)
    renderWithProviders('user-1')

    await screen.findByText('Owner Person')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.click(screen.getByRole('button', { name: /convidar/i }))

    await waitFor(() => expect(boardsApi.inviteMember).toHaveBeenCalledWith('board-1', 'ada@example.com'))
  })

  it('does not show the invite form to a non-owner', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders('user-2')

    await screen.findByText('Owner Person')
    expect(screen.queryByLabelText(/e-mail/i)).not.toBeInTheDocument()
  })

  it('lets the owner remove a non-owner member', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    vi.mocked(boardsApi.removeMember).mockResolvedValue(undefined)
    renderWithProviders('user-1')

    await screen.findByText('Ada Lovelace')
    await userEvent.click(screen.getByRole('button', { name: /remover ada lovelace/i }))

    await waitFor(() => expect(boardsApi.removeMember).toHaveBeenCalledWith('board-1', 'user-2'))
  })

  it('does not show a remove control for the owner row', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders('user-1')

    await screen.findByText('Owner Person')
    expect(screen.queryByRole('button', { name: /remover owner person/i })).not.toBeInTheDocument()
  })
})
