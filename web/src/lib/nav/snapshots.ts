import type { NavItem } from '../nav.ts';

// Spec v0.2 §3, §5.7: after Secrets, before Settings.
export default {
	id: 'snapshots',
	href: '/snapshots',
	icon: 'camera',
	labelKey: 'nav.snapshots',
	order: 70
} satisfies NavItem;
