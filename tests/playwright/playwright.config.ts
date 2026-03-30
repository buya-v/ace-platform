import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: '.',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: 'https://demo.garudax.asla.mn',
    ignoreHTTPSErrors: true,
    screenshot: 'on',
  },
  outputDir: './test-results',
  reporter: [['list'], ['html', { open: 'never' }]],
});
