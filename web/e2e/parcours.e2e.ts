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
	const logsCalls: string[] = [];
	page.on('request', (r) => {
		if (/\/api\/containers\/[^/]+\/logs(\?|$)/.test(r.url())) logsCalls.push(r.url());
	});
	await page.goto(`/?c=${agentId}&panel=logs`);
	await expect(page.getByRole('log')).toBeVisible();
	await expect(page.getByRole('log').locator('.line').first()).toBeVisible(); // the agent's own start lines
	await expect.poll(openStreams).toBe(1);
	const bottom = page.getByRole('navigation', { name: 'Panneaux du conteneur' });
	await bottom.getByRole('link', { name: "Diff avec l'image" }).click();
	await expect(page).toHaveURL(/panel=diff/);
	await expect(page.getByRole('log')).toBeHidden(); // mounted, out of sight
	await expect.poll(openStreams).toBe(0);
	await bottom.getByRole('link', { name: 'Logs en direct' }).click();
	await expect.poll(openStreams).toBe(1);
	// The stream resumes after the last line received, it does not start over.
	expect(logsCalls.length).toBeGreaterThanOrEqual(2);
	expect(logsCalls.at(-1)).toMatch(/[?&]after=[^&]+/);
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

	// Diff panel: the added file, its content alone (tranche 7), and its
	// link back to the file.
	const panels = page.getByRole('navigation', { name: 'Panneaux du conteneur' });
	await panels.getByRole('link', { name: "Diff avec l'image" }).click();
	await expect(page).toHaveURL(/panel=diff/);
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toContainText('ajouté');
	await page.getByRole('link', { name: '/root/note.txt', exact: true }).click();
	await expect(page).toHaveURL(/diff=%2Froot%2Fnote\.txt/);
	await expect(page).toHaveURL(/change=added/);
	const content = page.locator('.p-content');
	await expect(content).toContainText("seule la version du conteneur existe");
	await expect(content.locator('.cm-content')).toHaveText('bonjour hublot');
	await content.getByRole('link', { name: "Ouvrir dans l'explorateur" }).click();
	await expect(page).toHaveURL(/file=%2Froot%2Fnote\.txt/);
	await expect(page).toHaveURL(/panel=files/);
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	expect(new URL(page.url()).searchParams.get('c')).toBe(id);

	// Content diff of a modified file (tranche 7, spec v0.2 §5.6):
	// hublot-e2e-motd replaced the first line of /etc/motd and added one at
	// the end, and the tag that started it
	// was removed (scripts/e2e.sh, Review Focus 5): the image is read by its
	// ID. The image answers 202 until released: its progress shows.
	const motd = await page.evaluate(() =>
		fetch('/api/containers')
			.then((r) => r.json())
			.then((l: { id: string; name: string }[]) => l.find((c) => c.name === 'hublot-e2e-motd')?.id)
	);
	expect(motd).toMatch(/^[0-9a-f]{64}$/);
	const radio = await page.evaluate((id) => fetch(`/api/containers/${id}`).then((r) => r.json()), motd);
	expect(radio.image).toBe('hublot-e2e-motd'); // a name Docker no longer knows
	const imageCalls: string[] = [];
	let extracting = true;
	const imageFile = /\/api\/images\/[^/]+\/fs\/file\?/;
	// A barrier, through the same route: the page's requests are routed in
	// the order they are sent, so once it has come back, every request the
	// page sent before it has been counted (no fixed wait, whatever the load).
	const barrierPath = '/api/images/barrier/fs/file';
	const settled = () =>
		page.evaluate((p) => fetch(`${p}?path=%2F`).then((r) => r.text()).then(() => undefined, () => undefined), barrierPath);
	// The page's clock is Playwright's: it flows as usual, and runFor below
	// fires its timers without waiting for them.
	await page.clock.install();
	await page.route(imageFile, (route) => {
		const path = new URL(route.request().url()).pathname;
		if (path === barrierPath) return route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
		imageCalls.push(path);
		if (!extracting) return route.continue();
		return route.fulfill({
			status: 202,
			contentType: 'application/json',
			body: JSON.stringify({ error: 'indexing', detail: 'extracting', progress: 0.42 })
		});
	});
	await page.goto(`/?c=${motd}&panel=diff`);
	await page.getByRole('link', { name: '/etc/motd', exact: true }).click();
	await expect(content).toContainText("Lecture du fichier dans l'image : 42 %");
	// Review of tranche 7 (Codex): under 90 rem, the diff hidden behind
	// another panel stops asking the agent; shown again, it asks again.
	await panels.getByRole('link', { name: 'Logs en direct' }).click();
	await expect(page).toHaveURL(/panel=logs/);
	await expect(content).toBeHidden();
	await settled(); // a request sent just before the panel was hidden is counted
	const asked = imageCalls.length;
	// Five retries' worth of the page's time (files.ts pollDelay, 2 s), one
	// step at a time: a request a timer sends is counted (barrier) before the
	// next step, so a retry it would schedule is fired by the next one.
	for (let i = 0; i < 5; i++) {
		await page.clock.runFor(3000);
		await settled();
	}
	expect(imageCalls.length).toBe(asked);
	await panels.getByRole('link', { name: "Diff avec l'image" }).click();
	await expect.poll(() => imageCalls.length).toBeGreaterThan(asked);
	extracting = false;
	const added = content.locator('.cm-changedLine');
	await expect(added).toHaveCount(2, { timeout: 30_000 });
	await expect(added).toHaveText(['Bienvenue sur Hublot', /^ligne ajoutée par hublot /]);
	// Review of tranche 7: the U+202E of that line is shown, not obeyed.
	await expect(added.locator('.cm-specialChar')).toHaveCount(1);
	await expect(content.locator('.cm-deletedLine')).toHaveCount(1); // the image's first line
	// Line numbers of each side: an added line has none in the image, the
	// removed one none in the container.
	const lines = await content.locator('.cm-content .cm-line').count();
	const [imageNums, containerNums] = await content
		.locator('.hk-num')
		.evaluateAll((gs) => gs.map((g) => [...g.querySelectorAll('.cm-gutterElement')].map((e) => e.textContent).filter(Boolean)));
	const top = (l: (string | null)[]) => Math.max(...l.map(Number).filter((n) => !Number.isNaN(n)));
	expect(imageNums.filter((n) => n === '+')).toHaveLength(2);
	expect(containerNums.filter((n) => n === '−')).toHaveLength(1);
	expect(imageNums.slice(0, 3)).toContain('1'); // the removed line, above the first
	expect(top(imageNums)).toBe(lines - 1);
	expect(top(containerNums)).toBe(lines);
	expect(imageCalls.length).toBeGreaterThanOrEqual(2);
	for (const p of imageCalls) expect(p).toMatch(/^\/api\/images\/[0-9a-f]{64}\/fs\/file$/);
	expect(imageCalls[0].split('/')[3]).toBe(radio.imageId.replace(/^sha256:/, ''));
	await page.unroute(imageFile);

	// The numbers RENDERED in the two gutters, row by row (review after
	// tranche 10: shifted after a removed block). Each row of the view is
	// matched with the number drawn at its height in each gutter; the
	// number must point to a line of the same text, in the image (left) or
	// in the container (right).
	const hex = radio.imageId.replace(/^sha256:/, '');
	const fileText = (url: string) =>
		page.evaluate(async (u) => {
			for (;;) {
				const r = await fetch(u);
				if (r.status === 200) return r.text();
				if (r.status !== 202) throw new Error(`${u}: ${r.status}`);
				await new Promise((ok) => setTimeout(ok, 500));
			}
		}, url);
	const cases: [string, number][] = [
		['/etc/protocols', 3], // in the middle
		['/etc/passwd', 2], // at the start
		['/etc/group', 2], // at the end
		['/etc/inittab', 4], // two blocks, one line apart
		['/etc/profile', 1] // a removal, then an addition
	];
	for (const [path, removedLines] of cases) {
		await page.goto(`/?c=${motd}&panel=diff`);
		await page.getByRole('link', { name: path, exact: true }).click();
		await expect(content.locator('.cm-deletedLine')).toHaveCount(removedLines, { timeout: 30_000 });
		const q = `?path=${encodeURIComponent(path)}`;
		const imageLines = (await fileText(`/api/images/${hex}/fs/file${q}`)).split('\n');
		const containerLines = (await fileText(`/api/containers/${motd}/fs/file${q}`)).split('\n');
		const view = await content.evaluate((root) => {
			const top = (e: Element) => Math.round(e.getBoundingClientRect().top);
			const rows = [...root.querySelectorAll('.cm-line, .cm-deletedLine')].map((e) => ({
				top: top(e),
				text: e.textContent ?? '',
				removed: e.classList.contains('cm-deletedLine')
			}));
			const cells = [...root.querySelectorAll('.hk-num')].map((g) =>
				[...g.querySelectorAll('.cm-gutterElement')]
					.filter((el) => (el as HTMLElement).style.visibility !== 'hidden')
					.flatMap((el) => [...el.querySelectorAll(':scope > div > div')])
					.map((d) => ({ top: top(d), text: d.textContent ?? '' }))
			);
			return { rows, cells };
		});
		expect(view.cells).toHaveLength(2);
		const at = (cells: { top: number; text: string }[], y: number) => cells.find((c) => Math.abs(c.top - y) <= 2)?.text;
		const seen = view.rows.map((r) => {
			const a = at(view.cells[0], r.top);
			const b = at(view.cells[1], r.top);
			const image = a && a !== '+' ? imageLines[Number(a) - 1] : null;
			const container = b && b !== '−' ? containerLines[Number(b) - 1] : null;
			return { row: r.text, removed: r.removed, a, b, image, container };
		});
		const wrong = seen.filter(
			(s) =>
				s.a === undefined ||
				s.b === undefined ||
				(s.removed ? s.b !== '−' || s.image !== s.row : s.container !== s.row || (s.a !== '+' && s.image !== s.row))
		);
		expect(wrong, `${path}: ${JSON.stringify(wrong.slice(0, 3))}`).toEqual([]);
	}

	// Another container drops the diff shown.
	const other = await page.getByRole('link', { name: 'hublot-e2e', exact: true }).first().getAttribute('href');
	expect(other).not.toMatch(/[?&](diff|change)=/);

	// The full page keeps working (links of v0.1, phone).
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
