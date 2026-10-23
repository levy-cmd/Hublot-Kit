import { expect, test } from '@playwright/test';

// Plan v0.2, tranche 6: the files of an image (spec v0.2 §5.5), read from
// its layers by the agent, without a container. Images → alpine:3 → Files
// → /etc/os-release; read only: no upload, no download, no edit.
test('images : fichiers en lecture seule', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const asked: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (u.pathname.startsWith('/api/')) asked.push(`${r.method()} ${u.pathname}`);
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');

	await page.goto('/images');
	await page.getByRole('link', { name: 'alpine:3', exact: true }).click();
	await expect(page).toHaveURL(/\/images\/[0-9a-f]{64}$/);
	// The detail opens without any export: the files are read on the tab only.
	expect(asked.filter((a) => a.includes('/fs'))).toEqual([]);

	await page.getByRole('navigation', { name: "Vues de l'image" }).getByRole('link', { name: 'Fichiers' }).click();
	await expect(page).toHaveURL(/\/images\/[0-9a-f]{64}\?tab=files$/);
	const files = page.getByRole('region', { name: 'Fichiers' });
	await expect(files).toContainText('Lecture seule');
	await files.getByRole('button', { name: 'etc/', exact: true }).click({ timeout: 30_000 });
	await files.getByRole('button', { name: 'os-release', exact: true }).click();
	// A link in Alpine (/etc/os-release -> ../usr/lib/os-release): its
	// target, extracted from its layer.
	await expect(page).toHaveURL(/file=%2F[a-z%2F]*os-release/);
	await expect(files.locator('pre.text')).toContainText('Alpine Linux', { timeout: 30_000 });

	// Read only (Q63): no upload, no download, no edit, no Diff.
	for (const name of ['Envoyer un fichier ici', 'Télécharger', 'Télécharger ce dossier (.tar)', 'Modifier']) {
		await expect(files.getByRole('button', { name, exact: true })).toHaveCount(0);
	}
	await expect(files.getByRole('button', { name: 'Diff', exact: true })).toHaveCount(0);

	// The palette searches the files of the image open, in its index.
	await page.keyboard.press('Control+k');
	await page.getByRole('combobox').fill('os-rel');
	const hits = page.getByRole('group', { name: /^Fichiers de alpine:3/ });
	await expect(hits.getByRole('option', { name: '/etc/os-release' })).toBeVisible();
	await page.keyboard.press('Escape');

	// Only reads, by the full ID: never a write, never a name.
	for (const a of asked.filter((x) => x.includes('/api/images/'))) {
		expect(a).toMatch(/^(GET|HEAD) \/api\/images\/[0-9a-f]{64}(\/fs(\/file)?)?$|^POST \/api\/images\/[0-9a-f]{64}\/fs\/search$/);
	}
});
