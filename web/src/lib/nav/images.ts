import { imageList } from '../imageList.svelte.ts';
import type { NavItem } from '../nav.ts';

// Spec v0.2 §3, §5.4: after Containers, with its counter.
export default {
	id: 'images',
	href: '/images',
	icon: 'image',
	labelKey: 'nav.images',
	order: 20,
	count: () => {
		imageList.ensure();
		return imageList.items?.length ?? null;
	}
} satisfies NavItem;
