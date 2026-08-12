import { test, expect } from '@playwright/test'
import { uniqueEmail } from './helpers'

test('user can register, log out, and log back in', async ({ page }) => {
  const email = uniqueEmail('auth')
  const password = 'password123'

  await page.goto('/register')
  await page.getByLabel('Nome').fill('Ada Lovelace')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Criar conta' }).click()

  await expect(page.getByRole('heading', { name: 'Seus boards' })).toBeVisible()

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login/)

  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()

  await expect(page.getByRole('heading', { name: 'Seus boards' })).toBeVisible()
})
