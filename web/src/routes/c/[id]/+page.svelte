<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { ApiError, getContainer, type Radiography } from '#lib/api.ts';
	import { stateLabel } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import ChangesList from '#lib/components/ChangesList.svelte';
	import FileBrowser from '#lib/components/FileBrowser.svelte';
	import LogsView from '#lib/components/LogsView.svelte';
	import XrayView from '#lib/components/XrayView.svelte';
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
	<p><a href="/">← {t('nav.containers')}</a></p>
	{#if error}
		<p role="alert">{error}</p>
		<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
	{/if}
	{#if container}
		<h1>{container.name} <small class="state {container.state.status}">{stateLabel(container.state.status)}</small></h1>
		<p class="image">{container.image}</p>
		<nav class="tabs">
			<a class="tab" class:active={tab === 'files'} aria-current={tab === 'files' ? 'page' : undefined} href="?">{t('container.tab.files')}</a>
			<a class="tab" class:active={tab === 'changes'} aria-current={tab === 'changes' ? 'page' : undefined} href="?tab=changes">{t('container.tab.changes')}</a>
			<a class="tab" class:active={tab === 'logs'} aria-current={tab === 'logs' ? 'page' : undefined} href="?tab=logs">{t('container.tab.logs')}</a>
			<a class="tab" class:active={tab === 'xray'} aria-current={tab === 'xray' ? 'page' : undefined} href="?tab=xray">{t('container.tab.xray')}</a>
		</nav>
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
	{/if}
</main>

<style>
	main { padding: 1rem; }
	h1 { margin-bottom: 0.25rem; word-break: break-all; }
	h1 small { font-size: 0.9rem; font-weight: normal; }
	.image { margin-top: 0; opacity: 0.8; }
	.tabs { display: flex; flex-wrap: wrap; gap: 0.5rem; margin: 1rem 0; border-bottom: 1px solid #ddd; }
	.tab { padding: 0.4rem 0.8rem; }
	.tab.active { border-bottom: 2px solid currentColor; font-weight: 600; }
	.state.running { color: #1b7f3b; }
	.state.exited, .state.dead { color: #b00020; }
	[role='alert'] { color: #b00020; }
</style>
