import type { NavItem } from '../nav.ts';

// Opens the live logs panel of the chosen container (spec v0.2 §3).
export default {
	id: 'logs',
	href: '/?panel=logs',
	icon: 'logs',
	labelKey: 'nav.logs',
	order: 50,
	panel: true
} satisfies NavItem;
