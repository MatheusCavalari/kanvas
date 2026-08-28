import { useState, type FormEvent } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listComments, createComment, updateComment, deleteComment } from '../../api/comments'
import { commentKeys } from '../../lib/queryKeys'
import { useAuthStore } from '../auth/useAuthStore'

interface CommentSectionProps {
  cardId: string
  boardId: string
}

export default function CommentSection({ cardId }: CommentSectionProps) {
  const queryClient = useQueryClient()
  const currentUserId = useAuthStore((state) => state.user?.id)
  const [body, setBody] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingBody, setEditingBody] = useState('')

  const {
    data,
    isPending,
    isError,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: commentKeys.card(cardId),
    queryFn: ({ pageParam }) => listComments(cardId, pageParam ?? undefined),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
  })

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: commentKeys.card(cardId) })
  }

  const createMutation = useMutation({
    mutationFn: (text: string) => createComment(cardId, text),
    onSuccess: () => {
      invalidate()
      setBody('')
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ commentId, text }: { commentId: string; text: string }) => updateComment(commentId, text),
    onSuccess: () => {
      invalidate()
      setEditingId(null)
      setEditingBody('')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (commentId: string) => deleteComment(commentId),
    onSuccess: () => invalidate(),
  })

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!body.trim()) return
    createMutation.mutate(body)
  }

  function handleEditSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!editingId || !editingBody.trim()) return
    updateMutation.mutate({ commentId: editingId, text: editingBody })
  }

  const comments = data?.pages.flatMap((page) => page.comments) ?? []

  return (
    <div>
      <h3 className="text-sm font-medium text-gray-700">Comentários</h3>

      {isPending && <p className="mt-2 text-sm text-gray-500">Carregando...</p>}
      {isError && <p className="mt-2 text-sm text-red-700">Não foi possível carregar os comentários.</p>}

      <ul className="mt-2 space-y-2">
        {comments.map((comment) => (
          <li key={comment.id} className="rounded border border-gray-100 px-3 py-2">
            {editingId === comment.id ? (
              <form onSubmit={handleEditSubmit} className="space-y-2">
                <textarea
                  value={editingBody}
                  onChange={(event) => setEditingBody(event.target.value)}
                  rows={2}
                  className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
                  autoFocus
                />
                <div className="flex gap-2">
                  <button
                    type="submit"
                    disabled={updateMutation.isPending}
                    className="rounded bg-blue-600 px-2 py-1 text-xs text-white disabled:opacity-50"
                  >
                    Salvar
                  </button>
                  <button
                    type="button"
                    onClick={() => setEditingId(null)}
                    className="rounded px-2 py-1 text-xs text-gray-600 hover:bg-gray-100"
                  >
                    Cancelar
                  </button>
                </div>
              </form>
            ) : (
              <>
                <p className="whitespace-pre-wrap text-sm text-gray-800">{comment.body}</p>
                {comment.authorId === currentUserId && (
                  <div className="mt-1 flex gap-2">
                    <button
                      type="button"
                      onClick={() => {
                        setEditingId(comment.id)
                        setEditingBody(comment.body)
                      }}
                      className="text-xs text-blue-700 hover:underline"
                    >
                      Editar
                    </button>
                    <button
                      type="button"
                      onClick={() => deleteMutation.mutate(comment.id)}
                      className="text-xs text-red-700 hover:underline"
                    >
                      Excluir
                    </button>
                  </div>
                )}
              </>
            )}
          </li>
        ))}
        {!isPending && comments.length === 0 && (
          <li className="text-sm text-gray-500">Nenhum comentário ainda.</li>
        )}
      </ul>

      {hasNextPage && (
        <button
          type="button"
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="mt-2 text-sm text-blue-700 hover:underline disabled:opacity-50"
        >
          {isFetchingNextPage ? 'Carregando...' : 'Carregar mais'}
        </button>
      )}

      <form onSubmit={handleSubmit} className="mt-3 space-y-2">
        <label htmlFor="new-comment" className="sr-only">
          Novo comentário
        </label>
        <textarea
          id="new-comment"
          value={body}
          onChange={(event) => setBody(event.target.value)}
          rows={2}
          placeholder="Escreva um comentário..."
          className="w-full rounded border border-gray-300 px-3 py-2 text-sm"
        />
        <button
          type="submit"
          disabled={createMutation.isPending || !body.trim()}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white disabled:opacity-50"
        >
          Comentar
        </button>
        {createMutation.isError && (
          <p className="text-sm text-red-700">Não foi possível enviar o comentário.</p>
        )}
      </form>
    </div>
  )
}
