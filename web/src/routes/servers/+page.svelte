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
	<table>
		<tbody>
			<tr class="server">
				<th scope="row">{t('servers.current')}</th>
				<td class="mono">{location.host}</td>
				<td>{#if meta}{t('servers.version')} <span class="mono">{meta.version}</span>{/if}</td>
			</tr>
		</tbody>
	</table>
	<button type="button" disabled title={t('servers.soon')}>
		{t('servers.add')} ({t('servers.soon')})
	</button>
</main>

<style>
	main {
		padding: var(--e5) var(--gouttiere) var(--e7);
	}
	h1 {
		font-size: 2.25rem;
		font-stretch: 75%;
		font-weight: 800;
		margin-bottom: var(--e5);
	}
	table {
		border-collapse: collapse;
		margin-bottom: var(--e5);
		font-size: var(--t-s);
		border-top: 1px solid var(--encre);
	}
	th,
	td {
		text-align: left;
		padding: var(--e2) var(--e5) var(--e2) var(--e3);
		border-bottom: 1px solid var(--trait-fin);
	}
	th {
		font-weight: 600;
		border-left: 4px solid var(--vert);
	}
</style>
