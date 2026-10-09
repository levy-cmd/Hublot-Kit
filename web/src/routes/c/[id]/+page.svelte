<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, listContainers, type Container } from '#lib/api.ts';
	import { stateLabel } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import FileBrowser from '#lib/components/FileBrowser.svelte';

	let id = $derived(page.params.id ?? '');
	let container = $state<Container | null>(null);
	let error = $state('');
	let reloads = $state(0); // bumped by the Reload button

	// The list gives the name, image and state; GET /api/containers/{id}
	// arrives with the X-ray tab (phase Observation).
	$effect(() => {
		const wanted = id;
		void reloads;
		let current = true;
		error = '';
		container = null;
		listContainers()
			.then((list) => {
				if (!current) return;
				container = list.find((c) => c.id === wanted || c.name === wanted) ?? null;
				if (!container) error = errorMessage('not_found');
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
		<h1>{container.name} <small class="state {container.state}">{stateLabel(container.state)}</small></h1>
		<p class="image">{container.image}</p>
		<nav class="tabs">
			<span class="tab active" aria-current="page">{t('container.tab.files')}</span>
			<span class="tab soon">{t('container.tab.changes')} ({t('container.soon')})</span>
			<span class="tab soon">{t('container.tab.logs')} ({t('container.soon')})</span>
			<span class="tab soon">{t('container.tab.xray')} ({t('container.soon')})</span>
		</nav>
		{#if container.state === 'running' || container.state === 'paused'}
			<FileBrowser id={container.id} />
		{:else}
			<p>{t('container.stoppedSoon')}</p>
			<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
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
	.tab.soon { opacity: 0.5; }
	.state.running { color: #1b7f3b; }
	.state.exited, .state.dead { color: #b00020; }
	[role='alert'] { color: #b00020; }
</style>
