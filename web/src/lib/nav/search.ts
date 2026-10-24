import type { NavItem } from '../nav.ts';

// "Search" opens the palette (spec v0.2 §3, §5.8): no page of its own.
export default {
	id: 'search',
	href: '/',
	icon: 'search',
	labelKey: 'nav.search',
	order: 40,
	action: 'palette'
} satisfies NavItem;
