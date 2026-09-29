import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.goto('/')
})

test('edits grammar and inspects first failing parser stack', async ({ page }) => {
  await expect(page.getByRole('heading', { name: '预测分析表教学台' })).toBeVisible()

  await page.getByTestId('start-input').fill('E')
  await page.getByTestId('productions-input').fill('E->TR\nR->pTR\nR->\nT->i\nT->oEc')
  await page.getByTestId('tokens-input').fill('i p o')
  await page.getByTestId('analyze-button').click()

  await expect(page.getByTestId('server-request-id')).toHaveText(await page.getByTestId('active-request-id').innerText())
  await expect(page.getByRole('heading', { name: '词序列拒绝' })).toBeVisible()
  await expect(page.locator('tr.failed')).toBeVisible()
  await expect(page.locator('tr.failed td.mono').nth(0)).toHaveText('EcR$')
  await expect(page.locator('tr.failed td.mono').nth(1)).toHaveText('$')

  await page.getByRole('button', { name: 'FOLLOW 冲突' }).click()
  await page.getByTestId('analyze-button').click()
  await expect(page.getByTestId('conflict-cell')).toHaveText('LL(1) 冲突：M[A, a]')
  await expect(page.getByTestId('parse-steps')).toHaveCount(0)
})
