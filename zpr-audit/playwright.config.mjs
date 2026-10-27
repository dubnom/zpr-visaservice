import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.mjs',
  use: { baseURL: 'http://127.0.0.1:49283' },
  webServer: {
    command: 'python3 -m http.server 49283 --bind 127.0.0.1 --directory public',
    url: 'http://127.0.0.1:49283',
    reuseExistingServer: false,
  },
});
