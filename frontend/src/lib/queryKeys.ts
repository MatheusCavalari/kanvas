export const boardKeys = {
  all: ['boards'] as const,
  list: () => [...boardKeys.all, 'list'] as const,
  detail: (boardId: string) => [...boardKeys.all, boardId] as const,
  columns: (boardId: string) => [...boardKeys.all, boardId, 'columns'] as const,
  members: (boardId: string) => [...boardKeys.all, boardId, 'members'] as const,
}
