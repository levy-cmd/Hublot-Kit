import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { logsLive, mountedPanels, PANELS, WIDE_QUERY } from './panels.ts';

// Review of tranche 1: under 110 rem, a hidden panel must not be mounted. A
// file system panel mounted out of sight would start the export of a
// stopped container (up to 4 GiB, 5 min, the container locked by dockerd).
describe('mountedPanels', () => {
	it('mounts the four panels side by side from 110 rem (spec v0.2 §3)', () => {
		expect([...mountedPanels(new Set(), 'logs', true)].sort()).toEqual([...PANELS].sort());
	});

	it('under 110 rem, mounts only the open panel', () => {
		expect([...mountedPanels(new Set(), 'logs', false)]).toEqual(['logs']);
		expect(mountedPanels(new Set(), 'logs', false).has('files')).toBe(false);
	});

	it('keeps a panel once opened (its index is not rebuilt)', () => {
		expect([...mountedPanels(new Set(['logs']), 'diff', false)].sort()).toEqual(['diff', 'logs']);
	});
});

// Review of tranche 1 (Codex), ruled for tranche 2: a logs panel opened,
// then hidden, stays mounted, but must not keep its stream open.
describe('logsLive', () => {
	it('under 110 rem, a hidden logs panel opens no stream', () => {
		expect(logsLive('diff', false)).toBe(false);
		expect(logsLive('files', false)).toBe(false);
		expect(logsLive('logs', false)).toBe(true);
	});

	it('from 110 rem, the logs panel is always in sight: its stream stays open', () => {
		for (const p of PANELS) expect(logsLive(p, true)).toBe(true);
	});
});

// Decision of 2026-10-25: at 1440 px the four panels side by side left the
// diff 217 px, its lines cut. The four columns start at 110 rem (1760 px);
// below, two columns (the current panel and the content, half each). The
// threshold is written twice, in the script (matchMedia) and in the style
// (@media): both are read here from BottomPanels.svelte, at 16 px a rem.
describe('threshold of the four columns', () => {
	const src = readFileSync(new URL('./components/BottomPanels.svelte', import.meta.url), 'utf8');
	const css = src.slice(src.indexOf('<style>'));
	const blocks = [...css.matchAll(/@media ([^{]+)\{/g)].map((m, i, all) => ({
		query: m[1].trim(),
		body: css.slice(m.index, all[i + 1]?.index ?? css.length)
	}));
	const query = (marker: string) => {
		const found = blocks.filter((b) => b.body.includes(marker));
		expect(found, marker).toHaveLength(1);
		return found[0].query;
	};
	const narrow = query('.panel:not(.current)'); // one panel at a time, tabs
	const wide = query('.switch'); // the four columns, tabs hidden
	const applies = (q: string, px: number) =>
		q.split(/\s+and\s+/).every((part) => {
			const m = /^\((min|max)-width:\s*([\d.]+)rem\)$/.exec(part.trim());
			if (!m) throw new Error(`requête inattendue : ${q}`);
			const limit = Number(m[2]) * 16;
			return m[1] === 'min' ? px >= limit : px <= limit;
		});

	it('two columns at 1024, 1440, 1600 and 1759 px', () => {
		for (const px of [1024, 1440, 1600, 1759]) {
			expect(applies(narrow, px), `${px} px`).toBe(true);
			expect(applies(wide, px), `${px} px`).toBe(false);
		}
	});

	it('four columns at 1760, 1800 and 2560 px', () => {
		for (const px of [1760, 1800, 2560]) {
			expect(applies(wide, px), `${px} px`).toBe(true);
			expect(applies(narrow, px), `${px} px`).toBe(false);
		}
	});

	it('the script and the style agree at every width', () => {
		expect(src.includes('matchMedia(WIDE_QUERY)'), 'matchMedia(WIDE_QUERY)').toBe(true);
		for (let px = 320; px <= 3840; px++) {
			expect(applies(wide, px), `${px} px`).toBe(applies(WIDE_QUERY, px));
			expect(applies(narrow, px), `${px} px`).toBe(!applies(WIDE_QUERY, px));
		}
	});
});
