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

// Plan v0.2, tranche 9: a photo compared with the current container
// (nothing changed: no difference), then, the comparison and the match
// answered by the test, what the page shows of a modified file: the diff
// with the image when image-match is true, the current content alone
// otherwise (Q59: nothing of the old version is kept). hublot-e2e-motd
// changed /etc/motd of its image (scripts/e2e.sh). In this file, after the
// test above: one heavy job at a time for the agent.
test('snapshots : comparaison avec le conteneur actuel, diff seulement si l’image le prouve', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const bodies: string[] = [];
	page.on('response', async (r) => {
		if (new URL(r.url()).pathname.startsWith('/api/snapshots')) bodies.push(await r.text().catch(() => ''));
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');

	await page.goto('/snapshots');
	await page.getByRole('combobox', { name: 'Conteneur' }).selectOption({ label: 'hublot-e2e-motd' });
	const plan = page.locator('.plans button').first();
	await expect(plan).toBeEnabled();
	await plan.click();
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	const group = page.getByRole('region', { name: 'hublot-e2e-motd' });
	await expect(group.locator('tbody tr')).toHaveCount(1);

	await group.getByRole('link', { name: 'Comparer' }).click();
	await expect(page).toHaveURL(/compare=[0-9a-f-]{36}/);
	const section = page.getByRole('region', { name: 'Comparaison' });
	await section.getByRole('combobox', { name: 'Comparer avec' }).selectOption({ label: 'Le conteneur actuel' });
	const done = page.waitForResponse((r) => r.url().includes('with=current') && r.status() === 200, { timeout: 30_000 });
	await section.getByRole('button', { name: 'Comparer' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toContainText("rien n'est écrit");
	await dialog.getByRole('button', { name: 'Confirmer' }).click();
	await expect(page).toHaveURL(/with=current/);
	const real = await (await done).json();
	await expect(section).toContainText('Aucune différence.');
	expect(real.b.id).toBe('current');
	expect(real.contentCompared).toBe(real.a.source !== 'index');

	// The page as it shows a modified file, the agent's answers replaced.
	const answer = {
		...real,
		results: [
			{ path: '/etc', kind: 'meta', type: 'dir', fields: ['mtime'] },
			{ path: '/etc/motd', kind: 'modified', type: 'file', fields: [] },
			{ path: '/etc/os-release', kind: 'modified', type: 'file', fields: ['mtime'] }
		]
	};
	await page.route(/\/compare\?with=current/, (route) =>
		route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(answer) })
	);
	const matched: string[] = [];
	await page.route(/\/image-match\?/, (route) => {
		const p = new URL(route.request().url()).searchParams.get('path') ?? '';
		matched.push(p);
		return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ match: p === '/etc/motd' }) });
	});
	await page.reload();
	const results = section.locator('.results a');
	await expect(results).toHaveCount(2); // the directory whose date alone changed is hidden
	await section.getByRole('checkbox', { name: /Dossiers dont seule la date change/ }).check();
	await expect(results).toHaveCount(3);

	await results.filter({ hasText: '/etc/motd' }).click();
	await expect(page).toHaveURL(/path=%2Fetc%2Fmotd/);
	const content = section.locator('.content');
	await expect(content).toContainText("exactement celle de l'image");
	await expect(content.locator('.cm-changedLine')).toHaveCount(2, { timeout: 30_000 });
	await expect(content.locator('.cm-deletedLine')).toHaveCount(1);

	await results.filter({ hasText: '/etc/os-release' }).click();
	await expect(content).toContainText('Ancienne version non conservée');
	await expect(content).toContainText('Alpine');
	await expect(content.locator('.cm-changedLine')).toHaveCount(0);
	expect(matched).toEqual(['/etc/motd', '/etc/os-release']);
	await page.unroute(/\/compare\?with=current/);
	await page.unroute(/\/image-match\?/);

	for (const body of bodies) {
		expect(body).not.toContain('"mac"');
		expect(body).not.toContain('keyId');
	}

	await group.getByRole('button', { name: 'Supprimer' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(group).toHaveCount(0);
});
