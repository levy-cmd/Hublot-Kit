<script lang="ts">
	import { onMount } from 'svelte';
	import { getMeta, type Meta } from '#lib/api.ts';
	import { t } from '#lib/i18n/index.ts';

	let meta = $state<Meta | null>(null);
	onMount(async () => {
		meta = await getMeta().catch(() => null);
	});
</script>

<main>
	<h1>{t('servers.title')}</h1>
	<ul>
		<li class="server">
			<strong>{t('servers.current')}</strong>
			<span>{location.host}</span>
			{#if meta}<span>{t('servers.version')} {meta.version}</span>{/if}
		</li>
	</ul>
	<button type="button" disabled title={t('servers.soon')}>
		{t('servers.add')} — {t('servers.soon')}
	</button>
</main>

<style>
	main { padding: 1rem; }
	ul { list-style: none; padding: 0; }
	.server { display: flex; gap: 1rem; padding: 0.75rem; border: 1px solid #ddd; border-radius: 0.5rem; margin-bottom: 1rem; flex-wrap: wrap; }
</style>
