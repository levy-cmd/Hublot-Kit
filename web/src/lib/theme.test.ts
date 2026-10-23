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

	it('follows the system when nothing is stored', () => {
		expect(effectiveTheme(null, true)).toBe('dark');
		expect(effectiveTheme(null, false)).toBe('light');
	});

	it('lets the stored choice win over the system', () => {
		expect(effectiveTheme('light', true)).toBe('light');
		expect(effectiveTheme('dark', false)).toBe('dark');
	});

	it('toggles between the two themes', () => {
		expect(toggled('light')).toBe('dark');
		expect(toggled('dark')).toBe('light');
	});

	it('uses the key read by the bootstrap script of app.html', () => {
		expect(THEME_KEY).toBe('hublot-theme');
	});
});
