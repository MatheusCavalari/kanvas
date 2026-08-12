import { test, expect } from '@playwright/test'
import { registerAndLogin } from './helpers'

test('user can create a board and open its empty kanban view', async ({ page }) => {
  await registerAndLogin(page, 'create-board')

  await page.getByRole('button', { name: 'Novo board' }).click()
  await page.getByLabel('Nome').fill('Sprint Board')
  await page.getByRole('button', { name: 'Criar' }).click()

  const boardLink = page.getByRole('link', { name: 'Sprint Board' })
  await expect(boardLink).toBeVisible()
  await boardLink.click()

  await expect(page.getByRole('button', { name: '+ Adicionar coluna' })).toBeVisible()
})
