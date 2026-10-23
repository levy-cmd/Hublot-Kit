// The providers of the palette, one file per module in this folder (plan
// v0.2: a tranche adds its own, images.ts with the Images page), plus the
// pages of the side bar. Each provider returns what the browser already has.
import { t } from '../i18n/index.ts';
import { nav } from '../nav.ts';
import type { PaletteItem } from '../palette.ts';

const files = import.meta.glob<{ default: () => PaletteItem[] }>(['./*.ts', '!./index.ts', '!./*.test.ts'], { eager: true });

/** The pages: side bar entries that lead to a page (not a panel, not the palette itself). */
function pages(): PaletteItem[] {
	return nav.filter((n) => !n.panel && !n.action).map((n) => ({ kind: 'page', label: t(n.labelKey), href: n.href }));
}

export function paletteItems(): PaletteItem[] {
	return [...Object.values(files).flatMap((m) => m.default()), ...pages()];
}
