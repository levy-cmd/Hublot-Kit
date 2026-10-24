import type { NavItem } from '../nav.ts';

// Spec v0.2 §3, §5.9: after Logs, before Snapshots.
export default {
	id: 'secrets',
	href: '/secrets',
	icon: 'lock',
	labelKey: 'nav.secrets',
	order: 60
} satisfies NavItem;
