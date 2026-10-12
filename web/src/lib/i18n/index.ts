import { fr, type Key } from './fr';
import { en } from './en';

export type { Key };
export type Lang = 'fr' | 'en';

// Error codes of spec §6, plus client-side "network" and "unknown".
export const API_ERROR_CODES = [
	'unauthorized', 'invalid_name', 'forbidden_origin', 'rate_limited', 'not_found', 'read_only',
	'conflict', 'container_changed', 'indexing', 'too_large', 'not_regular', 'forbidden_path',
	'link_in_path', 'docker_unavailable', 'logs_unavailable', 'internal', 'network', 'unknown'
] as const;

const dicts: Record<Lang, Record<Key, string>> = { fr, en };

export function detectLang(navLang: string | undefined): Lang {
	return navLang?.toLowerCase().startsWith('fr') ? 'fr' : 'en';
}

const lang: Lang = typeof navigator === 'undefined' ? 'fr' : detectLang(navigator.language);

// Screen readers and hyphenation follow <html lang>; app.html ships a static default.
if (typeof document !== 'undefined') document.documentElement.lang = lang;

export function t(key: Key): string {
	return dicts[lang][key];
}

export function errorMessage(code: string): string {
	const key = `error.${code}`;
	return key in fr ? t(key as Key) : t('error.unknown');
}
