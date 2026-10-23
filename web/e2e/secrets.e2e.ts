import { expect, test } from '@playwright/test';

// Plan v0.2, tranche 4: the Secrets page (spec v0.2 §5.9). The inventory
// gives names only; one value is revealed by POST /env/reveal, by full ID,
// and forgotten on leaving the page; nothing goes to a browser store.
// scripts/e2e.sh gives hublot-e2e DB_PASSWORD=s3cr3t-e2e and GREETING=bonjour.
test('secrets : inventaire sans valeurs, révélation unitaire oubliée', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const inventories: string[] = [];
	const reveals: string[] = [];
	page.on('response', async (r) => {
		const u = new URL(r.url());
		if (u.pathname === '/api/secrets') inventories.push(await r.text().catch(() => ''));
		if (u.pathname.endsWith('/env/reveal')) reveals.push(`${r.request().method()} ${u.pathname}`);
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');
	const stores = () => page.evaluate(() => JSON.stringify({ ...localStorage }) + JSON.stringify({ ...sessionStorage }));

	await page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('link', { name: 'Secrets' }).click();
	await expect(page).toHaveURL(/\/secrets$/);
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Secrets');
	const fold = page.getByRole('button', { name: 'hublot-e2e', exact: true });
	await expect(fold).toHaveAttribute('aria-expanded', 'false');
	await expect.poll(() => inventories.length).toBe(1);
	for (const body of inventories) {
		expect(body).not.toContain('s3cr3t');
		expect(body).not.toContain('bonjour');
		expect(body).toContain('"DB_PASSWORD"');
	}

	// Unfolded: the name, masked; a plain variable is not listed.
	await fold.click();
	await expect(fold).toHaveAttribute('aria-expanded', 'true');
	const row = page.locator('li.row', { has: fold });
	await expect(row.locator('code.vname')).toHaveText(['DB_PASSWORD']);
	await expect(row).toContainText('••••••');
	await expect(page.locator('body')).not.toContainText('s3cr3t');

	// Reveal: by full ID, journaled by the agent; Hide forgets it.
	await row.getByRole('button', { name: 'Révéler' }).click();
	await expect(row.locator('code.value')).toHaveText('s3cr3t-e2e');
	expect(reveals).toHaveLength(1);
	expect(reveals[0]).toMatch(/^POST \/api\/containers\/[0-9a-f]{64}\/env\/reveal$/);
	expect(await stores()).not.toContain('s3cr3t');
	expect(page.url()).not.toContain('s3cr3t');
	await row.getByRole('button', { name: 'Masquer' }).click();
	await expect(row.locator('code.value')).toHaveCount(0);

	// Folding the row, then leaving the page, forget the value.
	await row.getByRole('button', { name: 'Révéler' }).click();
	await expect(row.locator('code.value')).toHaveText('s3cr3t-e2e');
	await fold.click();
	await fold.click();
	await expect(row.locator('code.value')).toHaveCount(0);
	await row.getByRole('button', { name: 'Révéler' }).click();
	await expect(row.locator('code.value')).toHaveText('s3cr3t-e2e');
	await page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('link', { name: 'Conteneurs' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Conteneurs');
	await page.goBack();
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Secrets');
	await expect(page.locator('body')).not.toContainText('s3cr3t');
	expect(await stores()).not.toContain('s3cr3t');

	// The project filter lives in the URL, as on the Containers page.
	await page.getByRole('combobox', { name: 'Projet' }).selectOption({ label: 'Sans projet' });
	await expect(page).toHaveURL(/[?&]project=-/);
	await expect(page.getByRole('button', { name: 'hublot-e2e', exact: true })).toBeVisible();
});
