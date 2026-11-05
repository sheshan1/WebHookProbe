import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests',
  // The application intentionally admits one active API run per process.
  workers: 1,
  timeout: 30_000,
  use: { baseURL: process.env.TEST_BASE_URL ?? 'http://127.0.0.1:5173', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  webServer: process.env.TEST_BASE_URL ? undefined : { command: 'npm run dev', url: 'http://127.0.0.1:5173', reuseExistingServer: !process.env.CI },
});
