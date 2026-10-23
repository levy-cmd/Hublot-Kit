import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// Spec v0.2 §4.2: every pair of the table is recomputed from app.css with the
// relative luminance of WCAG 2.x. A token changed below its threshold fails
// `make test`.
const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8');

function block(selector: string): Record<string, string> {
	const start = css.indexOf(`${selector} {`);
	if (start < 0) throw new Error(`${selector} absent from app.css`);
	const body = css.slice(start, css.indexOf('}', start));
	const out: Record<string, string> = {};
	for (const m of body.matchAll(/(--[a-z-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g)) out[m[1]] = m[2];
	return out;
}

const dark = block(':root');
const light = { ...dark, ...block(":root[data-theme='light']") };

function luminance(hex: string): number {
	const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
	const [r, g, b] = c.map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function ratio(fg: string, bg: string): number {
	if (!fg || !bg) throw new Error(`token missing: ${fg} on ${bg}`);
	const [a, b] = [luminance(fg), luminance(bg)].sort((x, y) => y - x);
	return (a + 0.05) / (b + 0.05);
}

// [foreground, background, minimum]: text 4.5, non-text information 3.
const TEXT = 4.5;
const UI = 3;
const PAIRS: [string, string, number][] = [
	...['--fond', '--surface', '--releve', '--selection'].flatMap((bg): [string, string, number][] => [
		['--texte', bg, TEXT],
		['--texte-doux', bg, TEXT],
		['--emeraude-texte', bg, TEXT],
		['--focus', bg, UI]
	]),
	['--sur-emeraude', '--emeraude', TEXT],
	['--sur-ambre', '--ambre', TEXT],
	...['--surface', '--releve', '--selection'].flatMap((bg): [string, string, number][] => [
		['--ambre-texte', bg, TEXT],
		['--rouge-texte', bg, TEXT]
	]),
	['--texte', '--diff-ajout', TEXT],
	['--texte', '--diff-suppr', TEXT],
	['--emeraude-texte', '--diff-ajout', TEXT],
	['--rouge-texte', '--diff-suppr', TEXT],
	['--trait-fort', '--fond', UI],
	['--trait-fort', '--surface', UI],
	['--trait-fort', '--releve', UI]
];

describe('contrast of the tokens (spec v0.2 §4.2)', () => {
	it('reads both themes from app.css', () => {
		expect(Object.keys(dark).length).toBeGreaterThanOrEqual(18);
		expect(light['--fond']).not.toBe(dark['--fond']);
	});

	it.each(PAIRS)('%s on %s ≥ %d in both themes', (fg, bg, min) => {
		for (const theme of [dark, light]) expect(ratio(theme[fg], theme[bg])).toBeGreaterThanOrEqual(min);
	});

	it('emerald fill on a dark surface carries information (≥ 3)', () => {
		expect(ratio(dark['--emeraude'], dark['--surface'])).toBeGreaterThanOrEqual(UI);
	});

	it('never uses the emerald fill alone on a light surface', () => {
		expect(ratio(light['--emeraude'], light['--surface'])).toBeLessThan(3); // documented exception
	});

	it('keeps the same emerald fill in both themes', () => {
		expect(light['--emeraude']).toBe(dark['--emeraude']);
	});

	it('matches a ratio of the spec table', () => {
		expect(ratio(dark['--texte'], dark['--fond'])).toBeCloseTo(15.39, 1);
		expect(ratio(light['--emeraude-texte'], light['--selection'])).toBeCloseTo(5.36, 1);
	});
});
