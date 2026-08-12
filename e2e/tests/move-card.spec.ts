import { test, expect } from '@playwright/test'
import { registerAndLogin, createBoardAndOpen, addColumn, addCard, columnByTitle, dragElement } from './helpers'

test('user can drag a card to a different column and it persists after reload', async ({ page }) => {
  await registerAndLogin(page, 'move-card')
  await createBoardAndOpen(page, 'Kanban Board')

  await addColumn(page, 'To Do')
  await addColumn(page, 'Done')
  await addCard(page, 'To Do', 'Ship it')

  const card = page.getByRole('button', { name: 'Ship it' })
  const doneColumnCards = columnByTitle(page, 'Done').getByTestId('column-cards')

  await dragElement(page, card, doneColumnCards)

  await expect(doneColumnCards.getByRole('button', { name: 'Ship it' })).toBeVisible()

  await page.reload()

  await expect(columnByTitle(page, 'Done').getByTestId('column-cards').getByRole('button', { name: 'Ship it' })).toBeVisible()
  await expect(columnByTitle(page, 'To Do')).toBeVisible()
  await expect(columnByTitle(page, 'To Do').getByRole('button', { name: 'Ship it' })).not.toBeVisible()
})
