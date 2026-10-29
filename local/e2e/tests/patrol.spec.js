// private-isu の主な画面と操作を一通り巡回する。施策のあとに、ベンチでは気づきにくい表示崩れや 5xx を拾う。
// 問題が変わったら、このファイルを問題ごとに書き直す（ユーザーは初期データのもの。パスワードはアカウント名を2回）。
import { test, expect } from '@playwright/test';

const admin = 'mary';      // authority = 1
const normal = 'dorothy';  // authority = 0
const banned = 'diane';    // /initialize で del_flg = 1 になる

// 1x1 の PNG
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');

test.describe.configure({ mode: 'serial' });

// 初期実装からある既知のエラー。施策で増えたものだけを拾うため、ここに挙げたものは数えない
const knownErrors = [
  // 投稿が 0 件のページで timeago.render に空の NodeList を渡すと、timeago.js が NodeList 自体を要素として扱って落ちる
  'e.getAttribute is not a function',
];

// どのテストでも、5xx の応答とブラウザのエラーが無いことを確かめる
let problems;
test.beforeEach(async ({ page }) => {
  problems = [];
  page.on('response', (res) => { if (res.status() >= 500) problems.push(`${res.status()} ${res.url()}`); });
  page.on('pageerror', (err) => { if (!knownErrors.includes(err.message)) problems.push(`pageerror: ${err.message}`); });
  // HTTP の失敗は上の response で見るので、ブラウザが出す「Failed to load resource」は数えない（403 を確かめるテストもある）
  page.on('console', (msg) => {
    if (msg.type() === 'error' && !msg.text().startsWith('Failed to load resource')) problems.push(`console: ${msg.text()}`);
  });
});
test.afterEach(async () => {
  expect(problems, '5xx かブラウザのエラーがありました').toEqual([]);
});

async function login(page, name, password = name + name) {
  await page.goto('/login');
  await page.fill('input[name="account_name"]', name);
  await page.fill('input[name="password"]', password);
  await page.click('input[type="submit"]');
}

async function expectImagesLoaded(page) {
  await page.waitForLoadState('networkidle');
  const broken = await page.$$eval('img.isu-image', (imgs) => imgs.filter((i) => !i.complete || i.naturalWidth === 0).map((i) => i.src));
  expect(broken, '読み込めなかった画像').toEqual([]);
}

test('/initialize が 200 を返す', async ({ request }) => {
  const res = await request.get('/initialize', { timeout: 30_000 });
  expect(res.status()).toBe(200);
});

test('トップに投稿と画像が出て、もっと見るで増える', async ({ page }) => {
  const res = await page.goto('/');
  expect(res.status()).toBe(200);
  const posts = page.locator('.isu-post');
  await expect(posts.first()).toBeVisible();
  const before = await posts.count();
  expect(before).toBeGreaterThan(0);
  await expectImagesLoaded(page);
  await page.click('#isu-post-more-btn');
  await expect.poll(() => posts.count()).toBeGreaterThan(before);
});

test('投稿の詳細とユーザーページが出る', async ({ page }) => {
  await page.goto('/');
  const href = await page.locator('.isu-post-permalink').first().getAttribute('href');
  const res = await page.goto(href);
  expect(res.status()).toBe(200);
  await expect(page.locator('.isu-post-comment-count')).toBeVisible();
  await expectImagesLoaded(page);

  const user = await page.goto(`/@${admin}`);
  expect(user.status()).toBe(200);
  await expect(page.locator('.isu-user-account-name')).toContainText(admin);
});

test('間違ったパスワードと BAN されたユーザーはログインできない', async ({ page }) => {
  await login(page, normal, 'wrong-password');
  await expect(page.locator('#notice-message')).toBeVisible();
  await login(page, banned);
  await expect(page.locator('#notice-message')).toBeVisible();
});

test('ログインして画像を投稿し、コメントできる', async ({ page }) => {
  await login(page, normal);
  await expect(page.locator('.isu-account-name')).toHaveText(normal);

  const body = `patrol ${Date.now()}`;
  await page.setInputFiles('input[name="file"]', { name: 'patrol.png', mimeType: 'image/png', buffer: png });
  await page.fill('textarea[name="body"]', body);
  await page.click('form[action="/"] input[type="submit"]');
  await expect(page).toHaveURL(/\/posts\/\d+$/);
  await expect(page.locator('.isu-post-text')).toContainText(body);
  await expectImagesLoaded(page);

  const comment = `comment ${Date.now()}`;
  await page.fill('.isu-comment-form input[name="comment"]', comment);
  await page.click('.isu-comment-form input[type="submit"]');
  await expect(page.locator('.isu-comment-text').last()).toHaveText(comment);
});

test('一般ユーザーは管理画面に入れず、管理者は入れる', async ({ page }) => {
  await login(page, normal);
  const denied = await page.goto('/admin/banned');
  expect(denied.status()).toBe(403);

  await page.goto('/logout');
  await login(page, admin);
  const allowed = await page.goto('/admin/banned');
  expect(allowed.status()).toBe(200);
});

test('新しいユーザーを登録するとログイン状態になる', async ({ page }) => {
  const name = `patrol${Date.now() % 1_000_000}`;
  await page.goto('/register');
  await page.fill('input[name="account_name"]', name);
  await page.fill('input[name="password"]', `${name}_password`);
  await page.click('input[type="submit"]');
  await expect(page.locator('.isu-account-name')).toHaveText(name);
});
