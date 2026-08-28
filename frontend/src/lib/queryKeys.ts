export const boardKeys = {
  all: ['boards'] as const,
  list: () => [...boardKeys.all, 'list'] as const,
  detail: (boardId: string) => [...boardKeys.all, boardId] as const,
  columns: (boardId: string) => [...boardKeys.all, boardId, 'columns'] as const,
  members: (boardId: string) => [...boardKeys.all, boardId, 'members'] as const,
  labels: (boardId: string) => [...boardKeys.all, boardId, 'labels'] as const,
  search: (boardId: string, q: string) => [...boardKeys.all, boardId, 'search', q] as const,
  presence: (boardId: string) => [...boardKeys.all, boardId, 'presence'] as const,
}

export const commentKeys = {
  all: ['comments'] as const,
  card: (cardId: string) => [...commentKeys.all, cardId] as const,
}

export const cardLabelKeys = {
  all: ['cardLabels'] as const,
  card: (cardId: string) => [...cardLabelKeys.all, cardId] as const,
}

export const activityKeys = {
  all: ['activity'] as const,
  board: (boardId: string) => [...activityKeys.all, boardId] as const,
}
