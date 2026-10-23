<script lang="ts">
	import { ICONS, type IconName } from './icons.ts';

	// Decorative: the text next to it (or the button's label) says what it is.
	let { name, size = 16, filled = false }: { name: IconName; size?: 16 | 20; filled?: boolean } = $props();
</script>

<svg
	class="icon"
	width={size}
	height={size}
	viewBox="0 0 20 20"
	fill={filled ? 'currentColor' : 'none'}
	stroke="currentColor"
	stroke-width="1.5"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	focusable="false"
>
	<path d={ICONS[name]} vector-effect="non-scaling-stroke" />
</svg>

<style>
	.icon {
		flex: none;
		display: block;
	}
</style>
