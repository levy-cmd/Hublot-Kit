// Entries of the side bar (spec v0.2 §3), one file per module in ./nav/,
// assembled here and sorted by order. A tranche adds its own file when its
// page exists: no dead link (nav.test.ts).
import type { IconName } from './icons/icons.ts';
import type { Key } from './i18n/index.ts';

export type NavItem = {
	id: string;
	href: string;
	icon: IconName;
	labelKey: Key;
	order: number;
	count?: () => number | null;
	/** The entry opens a panel of the chosen container: the link carries ?c=. */
	panel?: boolean;
	/** The entry opens the global search instead of a page (spec v0.2 §3, §5.8). */
	action?: 'palette';
};

const files = import.meta.glob<{ default: NavItem }>('./nav/*.ts', { eager: true });

export const nav: NavItem[] = Object.values(files)
	.map((m) => m.default)
	.sort((a, b) => a.order - b.order);

/**
 * The entry of the page at url: same path (a container page /c/<id> counts
 * as the list), every parameter of its link present, the most specific one
 * winning. A panel entry ("Diff with the image", "Logs") only opens a panel
 * of the Containers page: it is never the current page (review of tranche 2).
 */
export function currentNav(url: { pathname: string; searchParams: { get(k: string): string | null } }): string | null {
	const path = url.pathname.startsWith('/c/') ? '/' : url.pathname;
	let best: NavItem | null = null;
	let bestScore = -1;
	for (const item of nav) {
		if (item.panel || item.action) continue;
		const u = new URL(item.href, 'http://x');
		if (u.pathname !== path) continue;
		const params = [...u.searchParams];
		if (!params.every(([k, v]) => url.searchParams.get(k) === v)) continue;
		if (params.length > bestScore) {
			best = item;
			bestScore = params.length;
		}
	}
	return best?.id ?? null;
}

/** The link of an entry; a panel entry names the chosen container, if any. */
export function navHref(item: NavItem, container: string | null): string {
	if (!item.panel || !container) return item.href;
	const url = new URL(item.href, 'http://x');
	const q = new URLSearchParams({ c: container });
	for (const [k, v] of url.searchParams) q.set(k, v);
	return `${url.pathname}?${q}`;
}
