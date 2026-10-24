import type { NavItem } from '../nav.ts';

// Opens the "Diff with the image" panel of the chosen container (spec v0.2 §3).
export default {
	id: 'diff',
	href: '/?panel=diff',
	icon: 'diff',
	labelKey: 'nav.diff',
	order: 30,
	panel: true
} satisfies NavItem;
