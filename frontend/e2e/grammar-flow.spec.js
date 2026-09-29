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

test('named terminals stay atomic and the example reaches accept', async ({ page }) => {
  await page.goto('/')

  await page.getByRole('button', { name: '多字符终结符' }).click()
  await page.getByTestId('analyze-button').click()

  await expect(page.getByRole('heading', { name: '词序列接受' })).toBeVisible()

  // 表头只能有 id、plus、$ 三列，不能拆出 i、d 这样的单字母列。
  const headers = await page.locator('.table-panel thead th').allInnerTexts()
  expect(headers).toEqual(['', 'id', 'plus', '$'])

  // 栈回放末步为接受，且具名终结符在栈/剩余输入中以 {id} 整体出现。
  const rows = page.locator('[data-testid="parse-steps"] tbody tr')
  await expect(rows.last()).toContainText('接受')
  await expect(page.locator('[data-testid="parse-steps"] tbody tr').first()).toContainText('{id}')

  // 同一文法下带花括号输入 {id} 也应被规范为 id 并接受。
  await page.getByTestId('tokens-input').fill('{id} {plus} {id}')
  await page.getByTestId('analyze-button').click()
  await expect(page.getByRole('heading', { name: '词序列接受' })).toBeVisible()
})

test('malformed brace productions return a stable error banner', async ({ page }) => {
  await page.goto('/')

  await page.getByTestId('productions-input').fill('S->{id')
  await page.getByTestId('tokens-input').fill('')
  await page.getByTestId('analyze-button').click()

  const banner = page.getByTestId('request-error')
  await expect(banner).toBeVisible()
  await expect(banner).toContainText('花括号')
})
