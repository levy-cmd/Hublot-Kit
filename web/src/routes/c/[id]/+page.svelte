<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { ApiError, getContainer, type Radiography } from '#lib/api.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import ChangesList from '#lib/components/ChangesList.svelte';
	import FileBrowser from '#lib/components/FileBrowser.svelte';
	import LogsView from '#lib/components/LogsView.svelte';
	import XrayView from '#lib/components/XrayView.svelte';
	import Plaque from '#lib/components/Plaque.svelte';
	import { isStopped } from '#lib/xray.ts';

	let id = $derived(page.params.id ?? '');
	let container = $state<Radiography | null>(null);
	let error = $state('');
	let reloads = $state(0); // bumped by the Reload button
	// From the layout's /api/meta: true until it answers (spec §9).
	const readOnly = getContext<() => boolean>('readOnly');
	const TABS = ['files', 'changes', 'logs', 'xray'] as const;
	let tab = $derived(TABS.find((x) => x === page.url.searchParams.get('tab')) ?? 'files');

	// GET /api/containers/{id} (F7): the header and the X-ray tab, secrets
	// already masked by the agent.
	$effect(() => {
		const wanted = id;
		void reloads;
		let current = true;
		error = '';
		container = null;
		getContainer(wanted)
			.then((c) => {
				if (current) container = c;
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			});
		return () => {
			current = false;
		};
	});
</script>

<main>
	{#if error}
		<div class="erreur">
			<p role="alert">{error}</p>
			<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
		</div>
	{/if}
	{#if container}
		<Plaque id={container.id} name={container.name} image={container.image} state={container.state.status} />
		<nav class="tabs" aria-label={t('container.tabs')}>
			<a class="tab" aria-current={tab === 'files' ? 'page' : undefined} href="?">{t('container.tab.files')}</a>
			<a class="tab" aria-current={tab === 'changes' ? 'page' : undefined} href="?tab=changes">{t('container.tab.changes')}</a>
			<a class="tab" aria-current={tab === 'logs' ? 'page' : undefined} href="?tab=logs">{t('container.tab.logs')}</a>
			<a class="tab" aria-current={tab === 'xray' ? 'page' : undefined} href="?tab=xray">{t('container.tab.xray')}</a>
		</nav>
		<div class="vue">
			{#if tab === 'changes'}
				<ChangesList id={container.id} />
			{:else if tab === 'logs'}
				<LogsView id={container.id} />
			{:else if tab === 'xray'}
				<XrayView radio={container} onreload={() => reloads++} />
			{:else}
				<FileBrowser
					id={container.id}
					name={container.name}
					readOnly={readOnly()}
					stopped={isStopped(container.state)}
				/>
			{/if}
		</div>
	{/if}
</main>

<style>
	main {
		padding: var(--e5) var(--gouttiere) var(--e7);
	}
	.erreur {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--e3);
	}
	.erreur p {
		margin: 0;
	}
	.tabs {
		display: flex;
		flex-wrap: wrap;
		margin-top: var(--e3);
		border-bottom: 1px solid var(--encre);
	}
	.tab {
		display: inline-flex;
		align-items: center;
		min-height: 2.5rem;
		padding: 0 var(--e4);
		font-size: var(--t-s);
		font-weight: 600;
		text-decoration: none;
	}
	.tab:hover {
		background: var(--survol);
	}
	/* l'onglet ouvert : bande jaune, comme la ligne choisie de la liste */
	.tab[aria-current='page'] {
		background: var(--jaune);
		color: var(--sur-jaune);
	}
	.vue {
		margin-top: var(--e5);
	}

	@media (max-width: 40rem) {
		.tab {
			padding: 0 var(--e3);
		}
	}
</style>
