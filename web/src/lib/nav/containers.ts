import { containerList } from '../containerList.svelte.ts';
import type { NavItem } from '../nav.ts';

export default {
	id: 'containers',
	href: '/',
	icon: 'container',
	labelKey: 'nav.containers',
	order: 10,
	count: () => containerList.items?.length ?? null
} satisfies NavItem;
