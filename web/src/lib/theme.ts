// Light ("Quai") or dark ("quai de nuit"). Without a stored choice the
// system decides (prefers-color-scheme); the toggle stores the opposite.
// The bootstrap script of app.html applies the stored choice before the
// first paint: keep THEME_KEY in step with it.
export type Theme = 'light' | 'dark';

export const THEME_KEY = 'hublot-theme';

export const parseStored = (v: string | null): Theme | null => (v === 'light' || v === 'dark' ? v : null);

export const effectiveTheme = (stored: Theme | null, prefersDark: boolean): Theme =>
	stored ?? (prefersDark ? 'dark' : 'light');

export const toggled = (t: Theme): Theme => (t === 'dark' ? 'light' : 'dark');

function readStored(): Theme | null {
	try {
		return parseStored(localStorage.getItem(THEME_KEY));
	} catch {
		return null; // storage blocked: follow the system
	}
}

export function currentTheme(): Theme {
	return effectiveTheme(readStored(), matchMedia('(prefers-color-scheme: dark)').matches);
}

export function setTheme(t: Theme): void {
	document.documentElement.dataset.theme = t;
	try {
		localStorage.setItem(THEME_KEY, t);
	} catch {
		// not remembered, still applied
	}
}
