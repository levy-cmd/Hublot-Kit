import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { THEME_KEY, effectiveTheme, parseStored, toggled } from './theme.ts';

describe('theme', () => {
	it('keeps only a known stored value', () => {
		expect(parseStored('light')).toBe('light');
		expect(parseStored('dark')).toBe('dark');
		expect(parseStored(null)).toBeNull();
		expect(parseStored('auto')).toBeNull();
		expect(parseStored('<script>')).toBeNull();
	});

	it('is dark without a stored choice, whatever the system asks (Q64)', () => {
		expect(effectiveTheme(null, true)).toBe('dark');
		expect(effectiveTheme(null, false)).toBe('dark');
	});

	it('falls back to dark on an unreadable stored value', () => {
		expect(effectiveTheme(parseStored('auto'), false)).toBe('dark');
		expect(effectiveTheme(parseStored(''), false)).toBe('dark');
	});

	it('lets the stored choice win', () => {
		expect(effectiveTheme('light', true)).toBe('light');
		expect(effectiveTheme('dark', false)).toBe('dark');
	});

	it('toggles between the two themes', () => {
		expect(toggled('light')).toBe('dark');
		expect(toggled('dark')).toBe('light');
	});

	it('uses the key read by the bootstrap script of app.html', () => {
		expect(THEME_KEY).toBe('hublot-theme');
		const html = readFileSync(new URL('../app.html', import.meta.url), 'utf8');
		expect(html).toContain(`localStorage.getItem('${THEME_KEY}')`);
		expect(html).toContain('<meta name="color-scheme" content="dark light" />');
		// One inline script only (spec v0.2 §7): the agent hashes it for the CSP.
		expect(html.match(/<script/g)).toHaveLength(1);
	});
});
