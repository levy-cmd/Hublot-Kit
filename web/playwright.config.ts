import { defineConfig, devices } from '@playwright/test';

// The single journey of spec §10, run by scripts/e2e.sh against a real agent.
// *.e2e.ts, never *.spec.ts: Vitest would pick those up.
export default defineConfig({
	testDir: 'e2e',
	testMatch: '*.e2e.ts',
	timeout: 60_000,
	expect: { timeout: 15_000 },
	retries: 0,
	reporter: 'list',
	use: { baseURL: process.env.HUBLOT_URL ?? 'http://127.0.0.1:7879', locale: 'fr-FR', trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], locale: 'fr-FR' } }]
});
