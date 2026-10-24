import { readdirSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { currentNav, nav, navHref } from './nav.ts';
import { dicts } from './i18n/index.ts';

// Spec v0.2 §3: an entry appears once its page exists; no dead link.
describe('nav', () => {
	it('lists the entries delivered so far, in order', () => {
		expect(nav.map((n) => n.id)).toEqual(['containers', 'images', 'diff', 'search', 'logs', 'secrets', 'snapshots', 'settings']);
	});

	it('leads only to pages that exist', () => {
		const routes = new Set(['/', ...readdirSync(new URL('../routes', import.meta.url)).map((d) => `/${d}`)]);
		for (const n of nav) expect(routes).toContain(new URL(n.href, 'http://x').pathname);
	});

	it('has a label in both languages', () => {
		for (const n of nav) {
			expect(dicts.fr).toHaveProperty(n.labelKey);
			expect(dicts.en).toHaveProperty(n.labelKey);
		}
	});

	// Review of tranche 2: "Diff with the image" and "Logs" open a panel of the
	// Containers page; they are never the current page, even with ?panel=.
	it('marks the page, never a panel shortcut, as current', () => {
		const at = (u: string) => currentNav(new URL(u, 'http://x'));
		expect(at('/')).toBe('containers');
		expect(at('/?c=abc&panel=diff')).toBe('containers');
		expect(at('/?panel=logs')).toBe('containers');
		expect(at('/c/abc?tab=diff')).toBe('containers');
		expect(at('/settings')).toBe('settings');
		expect(at('/servers')).toBeNull();
		// "Search" opens the palette (tranche 3): never the current page.
		expect(nav.find((n) => n.id === 'search')?.action).toBe('palette');
	});

	it('opens the panel of the chosen container', () => {
		const diff = nav.find((n) => n.id === 'diff')!;
		expect(navHref(diff, 'abc')).toBe('/?c=abc&panel=diff');
		expect(navHref(diff, null)).toBe('/?panel=diff');
		const settings = nav.find((n) => n.id === 'settings')!;
		expect(navHref(settings, 'abc')).toBe('/settings');
	});
});
