import { expect, test } from '@playwright/test';

// Plan v0.2, tranche 8: Snapshots → take → the list shows the photo, then
// delete it. The confirmation says the cost (Q57); no fingerprint nor key
// id ever reaches the page (spec v0.2 §5.7). hublot-e2e is the stopped
// container of scripts/e2e.sh: its photo is an export (or, when another
// journey has built its index, the photo of the metadata, offered first).
test('snapshots : prise après confirmation du coût, liste, suppression', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const lists: string[] = [];
	page.on('response', async (r) => {
		if (new URL(r.url()).pathname === '/api/snapshots') lists.push(await r.text().catch(() => ''));
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');

	await page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('link', { name: 'Snapshots' }).click();
	await expect(page).toHaveURL(/\/snapshots$/);
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Snapshots');
	await page.getByRole('combobox', { name: 'Conteneur' }).selectOption({ label: 'hublot-e2e' });

	const plan = page.locator('.plans button').first();
	await expect(plan).toBeEnabled();
	await plan.click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toContainText('hublot-e2e');
	await expect(dialog).toContainText(/verrouille|index déjà construit/);
	await dialog.getByRole('button', { name: 'Confirmer' }).click();

	const group = page.getByRole('region', { name: 'hublot-e2e' });
	await expect(group.locator('tbody tr')).toHaveCount(1);
	await expect(group.locator('tbody tr')).toContainText(/export|métadonnées/);

	for (const body of lists) {
		expect(body).not.toContain('"mac"');
		expect(body).not.toContain('keyId');
	}

	await group.getByRole('button', { name: 'Supprimer' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(group).toHaveCount(0);
});
