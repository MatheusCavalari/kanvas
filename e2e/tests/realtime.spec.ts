import { test, expect } from '@playwright/test'
import { registerAndLogin, createBoardAndOpen, addColumn, addCard, columnByTitle } from './helpers'

test('a card created in one tab appears in another tab viewing the same board', async ({ context }) => {
  const ownerPage = await context.newPage()
  await registerAndLogin(ownerPage, 'realtime')
  await createBoardAndOpen(ownerPage, 'Realtime Board')
  await addColumn(ownerPage, 'Inbox')

  const boardUrl = ownerPage.url()

  // A second tab in the SAME browser context: it shares the httpOnly
  // refresh-token cookie, so it restores the same session on load — this
  // is what makes it "a second tab", not a second user.
  const viewerPage = await context.newPage()
  await viewerPage.goto(boardUrl)
  await expect(viewerPage.getByRole('button', { name: '+ Adicionar coluna' })).toBeVisible()

  await addCard(ownerPage, 'Inbox', 'Realtime card')

  await expect
    .poll(
      async () => columnByTitle(viewerPage, 'Inbox').getByRole('button', { name: 'Realtime card' }).count(),
      { timeout: 10000 },
    )
    .toBeGreaterThan(0)
})
