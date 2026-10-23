<script lang="ts">
	// The porthole ring: the mark of Hublot-Kit. Eight rivets cut into the
	// ring, the glass in the selection green.
	let { size = 28 }: { size?: number } = $props();
	const rivets = Array.from({ length: 8 }, (_, i) => {
		const a = (i * Math.PI) / 4 + Math.PI / 8;
		return { x: 16 + 12.25 * Math.cos(a), y: 16 + 12.25 * Math.sin(a) };
	});
</script>

<svg class="ring" width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
	<circle cx="16" cy="16" r="12.25" fill="none" stroke="var(--emeraude-texte)" stroke-width="5" />
	<circle cx="16" cy="16" r="7.4" fill="var(--selection)" stroke="var(--emeraude-texte)" stroke-width="1" />
	<path d="M11.8 14.2a4.6 4.6 0 0 1 3.4-3" fill="none" stroke="var(--emeraude-texte)" stroke-width="1.4" stroke-linecap="round" />
	{#each rivets as r, i (i)}
		<circle cx={r.x} cy={r.y} r="1" fill="var(--surface)" />
	{/each}
</svg>

<style>
	.ring {
		flex: none;
		display: block;
	}
</style>
