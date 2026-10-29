// make patrol（scripts/patrol.sh）から BASE_URL を受け取る。結果とスクリーンショットは test-results/ に出る
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  // ログインや投稿で状態を作りながら順に進むので、並列にしない
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    screenshot: 'on',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
