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
};

const files = import.meta.glob<{ default: NavItem }>('./nav/*.ts', { eager: true });

export const nav: NavItem[] = Object.values(files)
	.map((m) => m.default)
	.sort((a, b) => a.order - b.order);

/** The link of an entry; a panel entry names the chosen container, if any. */
export function navHref(item: NavItem, container: string | null): string {
	if (!item.panel || !container) return item.href;
	const url = new URL(item.href, 'http://x');
	const q = new URLSearchParams({ c: container });
	for (const [k, v] of url.searchParams) q.set(k, v);
	return `${url.pathname}?${q}`;
}
