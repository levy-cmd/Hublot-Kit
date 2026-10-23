import { readdirSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { nav, navHref } from './nav.ts';
import { dicts } from './i18n/index.ts';

// Spec v0.2 §3: an entry appears once its page exists; no dead link.
describe('nav', () => {
	it('lists the entries of tranche 1, in order', () => {
		expect(nav.map((n) => n.id)).toEqual(['containers', 'diff', 'logs', 'settings']);
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

	it('opens the panel of the chosen container', () => {
		const diff = nav.find((n) => n.id === 'diff')!;
		expect(navHref(diff, 'abc')).toBe('/?c=abc&panel=diff');
		expect(navHref(diff, null)).toBe('/?panel=diff');
		const settings = nav.find((n) => n.id === 'settings')!;
		expect(navHref(settings, 'abc')).toBe('/settings');
	});
});
