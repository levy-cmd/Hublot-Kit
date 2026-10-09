import { expect, test } from '@playwright/test';

// Spec §10: login → list → open a file → see the diff. scripts/e2e.sh starts
// the agent and the stopped container hublot-e2e, whose /root/note.txt was
// added after the image: this also runs tarindex end to end.
test('connexion, liste, fichier, diff', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await page.getByLabel('Afficher les conteneurs arrêtés').check(); // hublot-e2e is stopped
	await page.getByRole('link', { name: 'hublot-e2e', exact: true }).click();
	await page.getByRole('button', { name: 'root/', exact: true }).click(); // after the index is built
	await page.getByRole('button', { name: 'note.txt', exact: true }).click();
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	await expect(page.getByRole('row', { name: /note\.txt/ }).locator('abbr.mark.added')).toBeVisible();
	await page.getByRole('link', { name: 'Changements', exact: true }).click();
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toContainText('ajouté');

	// A link of the Changes tab leads to the file (?path=).
	await page.getByRole('link', { name: '/root/note.txt', exact: true }).click();
	await expect(page).toHaveURL(/file=%2Froot%2Fnote\.txt/);
	await expect(page.locator('.preview')).toContainText('bonjour hublot');

	// 390 px wide: no horizontal scroll, on the file view and on the Changes tab.
	await page.setViewportSize({ width: 390, height: 800 });
	const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
	expect(await overflow()).toBeLessThanOrEqual(0);
	await page.getByRole('link', { name: 'Changements', exact: true }).click();
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toBeVisible();
	expect(await overflow()).toBeLessThanOrEqual(0);
});
