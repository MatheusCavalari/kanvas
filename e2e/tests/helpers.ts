import type { Locator, Page } from '@playwright/test'

export function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 100000)}@example.com`
}

/**
 * Registers a fresh user via the UI, landing on the board list. Returns the
 * generated email in case a test needs to log back in with it later.
 */
export async function registerAndLogin(page: Page, namePrefix: string): Promise<string> {
  const email = uniqueEmail(namePrefix)
  await page.goto('/register')
  await page.getByLabel('Nome').fill(`${namePrefix} User`)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('password123')
  await page.getByRole('button', { name: 'Criar conta' }).click()
  await page.getByRole('heading', { name: 'Seus boards' }).waitFor()
  return email
}

export async function createBoardAndOpen(page: Page, name: string): Promise<void> {
  await page.getByRole('button', { name: 'Novo board' }).click()
  await page.getByLabel('Nome').fill(name)
  await page.getByRole('button', { name: 'Criar' }).click()
  await page.getByRole('link', { name }).click()
}

export async function addColumn(page: Page, title: string): Promise<void> {
  await page.getByRole('button', { name: '+ Adicionar coluna' }).click()
  await page.getByLabel('Título da coluna').fill(title)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()
}

export function columnByTitle(page: Page, title: string): Locator {
  return page.getByTestId('column').filter({ hasText: title })
}

export async function addCard(page: Page, columnTitle: string, cardTitle: string): Promise<void> {
  const column = columnByTitle(page, columnTitle)
  await column.getByRole('button', { name: '+ Adicionar card' }).click()
  await page.getByLabel('Título do card').fill(cardTitle)
  await column.getByRole('button', { name: 'Adicionar', exact: true }).click()
}

/**
 * Drags `source` onto `target` using raw pointer events — this app's
 * drag-and-drop (@dnd-kit) listens for pointer events, not native HTML5
 * drag-and-drop, so Locator.dragTo() (which dispatches HTML5 DnD events)
 * does not work here.
 */
export async function dragElement(page: Page, source: Locator, target: Locator): Promise<void> {
  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()
  if (!sourceBox || !targetBox) {
    throw new Error('drag source or target is not visible')
  }

  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2

  await page.mouse.move(startX, startY)
  await page.mouse.down()
  // Exceed dnd-kit's PointerSensor activation distance (5px) before the
  // real move, otherwise the drag never activates.
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  await page.mouse.up()
}
