import { describe, expect, it } from 'vitest';
import { fr } from './fr';
import { en } from './en';
import { API_ERROR_CODES, detectLang, errorMessage } from './index';

describe('i18n', () => {
	it('fr and en have exactly the same keys', () => {
		expect(Object.keys(en).sort()).toEqual(Object.keys(fr).sort());
	});

	it('every API error code of the spec has a translation', () => {
		for (const code of API_ERROR_CODES) {
			expect(fr).toHaveProperty(`error.${code}`);
		}
	});

	it('unknown codes fall back to a generic message', () => {
		expect(errorMessage('nope')).toBe(errorMessage('unknown'));
	});

	it('detects the language', () => {
		expect(detectLang('fr-FR')).toBe('fr');
		expect(detectLang('en-US')).toBe('en');
		expect(detectLang(undefined)).toBe('en');
	});
});
