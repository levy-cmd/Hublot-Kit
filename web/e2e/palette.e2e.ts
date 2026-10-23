import { expect, test } from '@playwright/test';

// Plan v0.2, tranche 3: the global search (spec v0.2 §5.8). Ctrl+K →
// container name → Enter; Escape gives the focus back; files only in the
// index already built of the container open (Q61); nothing kept.
test('recherche globale : Ctrl+K, conteneur, fichier', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const searches: string[] = [];
	const badSearches: string[] = [];
	// Every call to a file, search or figures route, to check the ID it names.
	const byId: string[] = [];
	page.on('request', (r) => {
		const u = new URL(r.url());
		if (u.pathname.endsWith('/fs/search')) {
			searches.push(r.url());
			// Review of tranche 3 (Codex): a POST, the query in its body only.
			if (r.method() !== 'POST' || u.search !== '') badSearches.push(`${r.method()} ${r.url()}`);
		}
		const m = /^\/api\/containers\/([^/]+)\/(fs|figures)/.exec(u.pathname);
		if (m) byId.push(m[1]);
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');
	// localStorage holds the theme only; sessionStorage, SvelteKit's history
	// (scroll positions): the palette writes in neither.
	const local = () => page.evaluate(() => JSON.stringify({ ...localStorage }));
	const session = () => page.evaluate(() => JSON.stringify({ ...sessionStorage }));
	const before = await local();

	// Ctrl+K from the field of the top bar; Escape gives the focus back to it.
	const field = page.locator('.topbar .search');
	await field.focus();
	await page.keyboard.press('Control+K');
	const dialog = page.getByRole('dialog', { name: 'Recherche globale' });
	const input = dialog.getByRole('combobox');
	await expect(input).toBeFocused();
	await page.keyboard.press('Tab');
	await page.keyboard.press('Tab');
	await expect(input).toBeFocused(); // the focus stays in the palette
	await page.keyboard.press('Escape');
	await expect(dialog).toBeHidden();
	await expect(field).toBeFocused();

	// Container name → Enter: the container is chosen; the query is in no URL
	// of the page.
	const url = page.url();
	await page.keyboard.press('Control+K');
	await input.fill('hublot-e2e');
	await expect(dialog.getByRole('option').first()).toContainText('hublot-e2e');
	expect(page.url()).toBe(url);
	await page.keyboard.press('Enter');
	await expect(dialog).toBeHidden();
	await expect(page).toHaveURL(/[?&]c=[0-9a-f]{64}/);
	await expect(page.locator('.detail h2')).toHaveText('hublot-e2e');
	expect(page.url()).not.toContain('q=');
	expect(searches).toEqual([]); // no container open while typing: no file search

	// Files of the open container: once the explorer has built its index.
	await page.getByRole('button', { name: 'root/', exact: true }).click();
	await page.keyboard.press('Control+K');
	await input.fill('note.tx');
	const hit = dialog.getByRole('option', { name: /\/root\/note\.txt/ });
	await expect(hit).toBeVisible();
	// The section is a named group: a screen reader says "Fichiers de …".
	await expect(dialog.getByRole('group', { name: 'Fichiers de hublot-e2e' })).toContainText('/root/note.txt');
	while ((await hit.getAttribute('aria-selected')) !== 'true') await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await expect(page).toHaveURL(/file=%2Froot%2Fnote\.txt/);
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	expect(searches.length).toBeGreaterThan(0);

	// Review of tranche 3 (Codex): a /c/<short ID> link; the file, search and
	// figures routes still get the full ID, the only one the agent matches
	// with a build in progress.
	const id = new URL(page.url()).searchParams.get('c')!;
	await page.goto(`/c/${id.slice(0, 12)}?tab=files`);
	await expect(page.getByRole('button', { name: 'root/', exact: true })).toBeVisible();
	await page.keyboard.press('Control+K');
	await input.fill('note.tx');
	await expect(dialog.getByRole('option', { name: /\/root\/note\.txt/ })).toBeVisible();
	await page.keyboard.press('Escape');
	expect(byId.length).toBeGreaterThan(0);
	expect(byId.filter((x) => x !== id)).toEqual([]);
	expect(badSearches).toEqual([]);

	// A running container (the agent) has no index: the explorer is offered,
	// never a file.
	await page.goto('/');
	await expect(page.locator('.topbar .search')).toBeVisible(); // the palette comes with the top bar
	await page.keyboard.press('Control+K');
	await input.fill('hublot-e2e-agent');
	await expect(dialog.getByRole('option').first()).toContainText('hublot-e2e-agent'); // the list is loaded again
	await page.keyboard.press('Enter');
	await expect(page.locator('.detail h2')).toHaveText('hublot-e2e-agent');
	await page.keyboard.press('Control+K');
	await input.fill('etc');
	await expect(dialog.getByRole('option', { name: /Ouvrir l’explorateur/ })).toBeVisible();
	await expect(dialog.getByRole('option', { name: /^\/etc/ })).toHaveCount(0);
	await page.keyboard.press('Escape');

	// The side bar entry opens it too; no query was stored.
	await page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('button', { name: 'Recherche' }).click();
	await expect(input).toBeFocused();
	await page.keyboard.press('Escape');
	expect(await local()).toBe(before);
	for (const q of ['hublot-e2e', 'note.tx']) expect(await session()).not.toContain(q);
});
