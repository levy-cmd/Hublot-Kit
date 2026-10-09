<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { ApiError, listContainers, type Container } from '#lib/api.ts';
	import { filterContainers, formatPorts, stateLabel } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let all = $state<Container[]>([]);
	let query = $state('');
	let showAll = $state(false);
	let error = $state('');
	let loading = $state(false);
	let shown = $derived(filterContainers(all, query, showAll));

	async function load() {
		loading = true;
		error = '';
		try {
			all = await listContainers();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) return goto('/login');
			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<main>
	<h1>{t('containers.title')}</h1>
	<div class="tools">
		<input type="search" aria-label={t('containers.search')} placeholder={t('containers.search')} bind:value={query} />
		<label><input type="checkbox" bind:checked={showAll} /> {t('containers.showAll')}</label>
		<button type="button" onclick={load} disabled={loading}>{t('containers.refresh')}</button>
	</div>

	{#if error}<p role="alert">{error}</p>{/if}

	{#if error}
		<!-- error shown above -->
	{:else if shown.length === 0 && loading}
		<p>{t('containers.loading')}</p>
	{:else if shown.length === 0}
		<p>{t('containers.empty')}</p>
	{:else}
		<div class="scroll">
			<table>
				<thead>
					<tr>
						<th>{t('containers.col.name')}</th>
						<th>{t('containers.col.image')}</th>
						<th>{t('containers.col.state')}</th>
						<th>{t('containers.col.status')}</th>
						<th>{t('containers.col.ports')}</th>
					</tr>
				</thead>
				<tbody>
					{#each shown as c (c.id)}
						<tr>
							<td><a href={`/c/${c.id}`}>{c.name}</a></td>
							<td>{c.image}</td>
							<td><span class="state {c.state}">{stateLabel(c.state)}</span></td>
							<td>{c.status}</td>
							<td>{formatPorts(c.ports)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</main>

<style>
	main { padding: 1rem; }
	.tools { display: flex; gap: 1rem; flex-wrap: wrap; align-items: center; margin-bottom: 1rem; }
	.scroll { overflow-x: auto; }
	table { border-collapse: collapse; width: 100%; }
	th, td { text-align: left; padding: 0.4rem 0.6rem; border-bottom: 1px solid #eee; white-space: nowrap; }
	.state.running { color: #1b7f3b; }
	.state.exited, .state.dead { color: #b00020; }
	[role='alert'] { color: #b00020; }
</style>
