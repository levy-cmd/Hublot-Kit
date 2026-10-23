// Dark (default) or light. Without a stored choice the theme is dark, even
// when the system asks for light (spec v0.2, Q64). The toggle stores the
// other one. The bootstrap script of app.html applies a stored "light"
// before the first paint: keep THEME_KEY in step with it.
export type Theme = 'light' | 'dark';

export const THEME_KEY = 'hublot-theme';

export const parseStored = (v: string | null): Theme | null => (v === 'light' || v === 'dark' ? v : null);

// The system's preference is still passed (signature of "Quai", plan v0.2),
// and ignored on purpose: Q64.
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export const effectiveTheme = (stored: Theme | null, prefersDark?: boolean): Theme => stored ?? 'dark';

export const toggled = (t: Theme): Theme => (t === 'dark' ? 'light' : 'dark');

function readStored(): Theme | null {
	try {
		return parseStored(localStorage.getItem(THEME_KEY));
	} catch {
		return null; // storage blocked: dark
	}
}

export function currentTheme(): Theme {
	return effectiveTheme(readStored());
}

export function setTheme(t: Theme): void {
	if (t === 'light') document.documentElement.dataset.theme = 'light';
	else delete document.documentElement.dataset.theme;
	try {
		localStorage.setItem(THEME_KEY, t);
	} catch {
		// not remembered, still applied
	}
}
