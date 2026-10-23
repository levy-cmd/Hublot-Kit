import { expect, test } from '@playwright/test';

// Plan v0.2, tranche 5: the Images pages (spec v0.2 §5.4). Side bar →
// Images → alpine:3 → its detail, by its full ID, with the container
// hublot-e2e; the image hublot-e2e-img of scripts/e2e.sh shows its build
// argument, variable and label masked; a name in the address is never sent.
test('images : liste, détail par ID, secrets masqués', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	const bodies: string[] = [];
	const asked: string[] = [];
	page.on('response', async (r) => {
		const u = new URL(r.url());
		if (u.pathname.startsWith('/api/images')) {
			asked.push(u.pathname);
			bodies.push(await r.text().catch(() => ''));
		}
	});
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await expect(page.locator('.topbar')).toContainText('Connecté');

	// The entry of the side bar, with its counter.
	const entry = page.getByRole('navigation', { name: 'Navigation principale' }).getByRole('link', { name: /^Images/ });
	await expect(entry.locator('.count')).toHaveText(/^\d+$/);
	await entry.click();
	await expect(page).toHaveURL(/\/images$/);
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('Images');

	// alpine:3 → its detail, by its full ID; hublot-e2e runs it.
	await page.getByRole('link', { name: 'alpine:3', exact: true }).click();
	await expect(page).toHaveURL(/\/images\/[0-9a-f]{64}$/);
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('alpine:3');
	await expect(page.getByRole('region', { name: 'Conteneurs de cette image' })).toContainText('hublot-e2e');
	await expect(page.getByRole('region', { name: 'Couches' })).toContainText(/\d+ couches/);

	// The image with witnesses: names shown, values masked.
	await page.getByRole('link', { name: 'Toutes les images' }).click();
	await page.getByRole('link', { name: 'hublot-e2e-img:latest', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1 })).toHaveText('hublot-e2e-img:latest');
	const config = page.getByRole('region', { name: 'Configuration' });
	await expect(config.locator('code.vname', { hasText: 'API_KEY' })).toBeVisible();
	await expect(config).toContainText('bonjour-img');
	await expect(config).toContainText('org.opencontainers.image.title');
	await expect(config).toContainText('1 autre label masqué');
	// The RUN that reads GH_PAT changes no file: 0 bytes under overlay2 (CI),
	// 4 KiB under the containerd store. Unfold the entries without files first.
	await page.getByRole('button', { name: /Montrer les \d+ entrées sans fichier/ }).click();
	const layers = page.getByRole('region', { name: 'Couches' });
	await expect(layers).toContainText('RUN |1 GH_PAT=•••••• /bin/sh -c');
	await expect(layers).toContainText('ARG GH_PAT=••••••');
	await expect(layers).toContainText('LABEL ••••••');
	await expect(layers).toContainText('API_KEY=••••••');
	await expect(page.locator('body')).not.toContainText('s3cr3t');

	// A name in the address: refused by the page, never sent to the agent.
	const before = asked.length;
	await page.goto('/images/alpine:3');
	await expect(page.getByRole('alert')).toHaveText('Introuvable.');
	expect(asked.slice(before).filter((p) => p.startsWith('/api/images/'))).toEqual([]);

	expect(bodies.length).toBeGreaterThan(0);
	for (const b of bodies) expect(b).not.toContain('s3cr3t');
	for (const p of asked.filter((x) => x.startsWith('/api/images/'))) expect(p).toMatch(/^\/api\/images\/[0-9a-f]{64}$/);
});
