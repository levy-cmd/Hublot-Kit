import { expect, test } from '@playwright/test';

// Spec §10, extended by plan v0.2 (tranche 1): login → list → choose a row →
// file system panel → preview → diff panel. scripts/e2e.sh starts the agent
// and the stopped container hublot-e2e, whose /root/note.txt was added after
// the image: this also runs tarindex end to end.
test('connexion, liste, panneaux fichiers et diff', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	// Every EventSource of the page, to count the open log streams.
	await page.addInitScript(() => {
		const Native = window.EventSource;
		const all: EventSource[] = [];
		(window as unknown as { openStreams: () => number }).openStreams = () =>
			all.filter((s) => s.readyState !== Native.CLOSED).length;
		window.EventSource = class extends Native {
			constructor(url: string | URL, init?: EventSourceInit) {
				super(url, init);
				all.push(this);
			}
		};
	});
	const openStreams = () => page.evaluate(() => (window as unknown as { openStreams: () => number }).openStreams());
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();

	// Dark without a stored choice (Q64).
	await expect(page.locator('html')).not.toHaveAttribute('data-theme', 'light');
	// The real version and Docker's state in the top bar (spec v0.2 §3).
	await expect(page.locator('.topbar')).toContainText('Connecté');

	// Review of tranche 1: under 90 rem (1280 px here), opening the logs panel
	// of a stopped container mounts no hidden file system panel, so no
	// export of the container starts.
	const id = await page.evaluate(() =>
		fetch('/api/containers')
			.then((r) => r.json())
			.then((l: { id: string; name: string }[]) => l.find((c) => c.name === 'hublot-e2e')?.id)
	);
	expect(id).toMatch(/^[0-9a-f]{64}$/);
	const fsCalls: string[] = [];
	page.on('request', (r) => {
		if (/\/api\/containers\/[^/]+\/fs/.test(r.url())) fsCalls.push(r.url());
	});
	await page.goto(`/?c=${id}&panel=logs`);
	await expect(page.getByRole('log')).toBeVisible();
	await page.waitForTimeout(2000);
	expect(fsCalls).toEqual([]);

	// Review of tranche 1 (Codex): under 90 rem, a logs panel opened then
	// hidden stays mounted, but closes its stream; shown again, it reopens it.
	// On the running agent, whose stream stays open (a stopped container's
	// ends by itself).
	const agentId = await page.evaluate(() =>
		fetch('/api/containers')
			.then((r) => r.json())
			.then((l: { id: string; name: string }[]) => l.find((c) => c.name === 'hublot-e2e-agent')?.id)
	);
	expect(agentId).toMatch(/^[0-9a-f]{64}$/);
	await page.goto(`/?c=${agentId}&panel=logs`);
	await expect(page.getByRole('log')).toBeVisible();
	await expect.poll(openStreams).toBe(1);
	const bottom = page.getByRole('navigation', { name: 'Panneaux du conteneur' });
	await bottom.getByRole('link', { name: "Diff avec l'image" }).click();
	await expect(page).toHaveURL(/panel=diff/);
	await expect(page.getByRole('log')).toBeHidden(); // mounted, out of sight
	await expect.poll(openStreams).toBe(0);
	await bottom.getByRole('link', { name: 'Logs en direct' }).click();
	await expect.poll(openStreams).toBe(1);
	await page.goto('/');

	// hublot-e2e is stopped: the "Stopped" tab lists it (?state=stopped).
	await page.getByRole('navigation', { name: 'Filtrer par état' }).getByRole('link', { name: /Arrêtés/ }).click();
	await expect(page).toHaveURL(/state=stopped/);
	await page.getByRole('link', { name: 'hublot-e2e', exact: true }).click();
	await expect(page).toHaveURL(/[?&]c=[0-9a-f]{64}/);
	await expect(page.locator('.detail h2')).toHaveText('hublot-e2e');

	// File system panel (the default), then the content panel.
	await page.getByRole('button', { name: 'root/', exact: true }).click(); // after the index is built
	await page.getByRole('button', { name: 'note.txt', exact: true }).click();
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	await expect(page.getByRole('row', { name: /note\.txt/ }).locator('abbr.mark.added')).toBeVisible();

	// Diff panel: the added file, and its link back to the file.
	const panels = page.getByRole('navigation', { name: 'Panneaux du conteneur' });
	await panels.getByRole('link', { name: "Diff avec l'image" }).click();
	await expect(page).toHaveURL(/panel=diff/);
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toContainText('ajouté');
	await page.getByRole('link', { name: '/root/note.txt', exact: true }).click();
	await expect(page).toHaveURL(/file=%2Froot%2Fnote\.txt/);
	await expect(page).toHaveURL(/panel=files/);
	await expect(page.locator('.preview')).toContainText('bonjour hublot');

	// The full page keeps working (links of v0.1, phone).
	expect(new URL(page.url()).searchParams.get('c')).toBe(id);
	await page.goto(`/c/${id}?tab=diff`);
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toBeVisible();

	// 390 px wide: no horizontal scroll, on the list, the file view and the diff.
	await page.setViewportSize({ width: 390, height: 800 });
	const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
	expect(await overflow()).toBeLessThanOrEqual(0);
	await page.goto(`/c/${id}?tab=files&path=%2Froot%2Fnote.txt`);
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	expect(await overflow()).toBeLessThanOrEqual(0);
	await page.goto('/');
	await expect(page.getByRole('link', { name: 'hublot-e2e', exact: true })).toBeVisible();
	expect(await overflow()).toBeLessThanOrEqual(0);
});
