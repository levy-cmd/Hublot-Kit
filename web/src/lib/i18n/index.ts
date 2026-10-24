// Dictionaries split by module (plan v0.2, tranche 1): <module>.fr.ts and
// <module>.en.ts, each exporting `default` as a flat { key: text } object.
// A tranche adds its own pair; nothing here changes. i18n.test.ts checks
// that both languages have the same keys, that no key is defined twice, and
// that every literal key used by the code exists.

export type Lang = 'fr' | 'en';
export type Key = string;
type Dict = Record<string, string>;

const files = import.meta.glob<{ default: Dict }>('./*.{fr,en}.ts', { eager: true });

export const modules: Record<Lang, Record<string, Dict>> = { fr: {}, en: {} };
for (const [path, mod] of Object.entries(files)) {
	const m = /^\.\/(.+)\.(fr|en)\.ts$/.exec(path);
	if (m) modules[m[2] as Lang][m[1]] = mod.default;
}

export const dicts: Record<Lang, Dict> = {
	fr: Object.assign({}, ...Object.values(modules.fr)),
	en: Object.assign({}, ...Object.values(modules.en))
};

// Error codes of spec §6 and v0.2 §6, plus client-side "network" and "unknown".
export const API_ERROR_CODES = [
	'unauthorized', 'invalid_name', 'forbidden_origin', 'rate_limited', 'not_found', 'read_only',
	'conflict', 'container_changed', 'indexing', 'too_large', 'not_regular', 'forbidden_path',
	'link_in_path', 'docker_unavailable', 'logs_unavailable', 'internal', 'invalid_query',
	'unsupported_image', 'network', 'unknown'
] as const;

export function detectLang(navLang: string | undefined): Lang {
	return navLang?.toLowerCase().startsWith('fr') ? 'fr' : 'en';
}

export const lang: Lang = typeof navigator === 'undefined' ? 'fr' : detectLang(navigator.language);

// Screen readers and hyphenation follow <html lang>; app.html ships a static default.
if (typeof document !== 'undefined') document.documentElement.lang = lang;

/** The text of key in language l; the key itself if it is missing. */
export function tl(l: Lang, key: Key): string {
	return dicts[l][key] ?? key;
}

export function t(key: Key): string {
	return tl(lang, key);
}

/** t(key) with {name} placeholders replaced. */
export function tf(key: Key, vars: Record<string, string | number>): string {
	return t(key).replace(/\{(\w+)\}/g, (all, k: string) => (k in vars ? String(vars[k]) : all));
}

export function errorMessage(code: string): string {
	const key = `error.${code}`;
	return key in dicts.fr ? t(key) : t('error.unknown');
}
