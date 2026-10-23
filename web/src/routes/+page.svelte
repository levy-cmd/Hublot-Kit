<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import type { Container } from '#lib/api.ts';
	import { containerList } from '#lib/containerList.svelte.ts';
	import { countGroups, filterContainers, groupOf, STATES, stateLabel } from '#lib/containers.ts';
	import { errorMessage, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { withParams } from '#lib/url.ts';
	import BottomPanels, { type Panel } from '#lib/components/BottomPanels.svelte';
	import ContainerDetail, { type DetailTab } from '#lib/components/ContainerDetail.svelte';
	import ContainerTable from '#lib/components/ContainerTable.svelte';

	const readOnly = getContext<() => boolean>('readOnly');

	// State in the URL (spec v0.2 §3): ?state= (tab or Docker state), ?c= (the
	// chosen container), ?panel=, ?tab= (of the detail), ?path=&file= (files).
	// The search is not in the URL: a name can be sensitive.
	let query = $state('');
	let search = $state<HTMLInputElement>();
	let params = $derived(page.url.searchParams);
	let stateFilter = $derived(params.get('state') ?? '');
	let chosenId = $derived(params.get('c'));
	let panel = $derived<Panel>((['files', 'diff', 'logs'] as const).find((p) => p === params.get('panel')) ?? 'files');
	let detailTab = $derived<DetailTab>(params.get('tab') === 'secrets' ? 'secrets' : 'overview');

	let all = $derived(containerList.items ?? []);
	let shown = $derived(filterContainers(all, query, stateFilter));
	let counts = $derived(countGroups(all));
	let chosen = $derived(chosenId ? (all.find((c) => c.id === chosenId) ?? null) : null);
	let presentStates = $derived(STATES.filter((s) => all.some((c) => c.state === s)));
	let error = $derived(containerList.error ? errorMessage(containerList.error.code) : '');

	$effect(() => {
		if (containerList.error?.status === 401) goto('/login').catch(() => {});
	});

	// A phone shows the detail as a full page (spec v0.2 §3).
	let narrow = $state(false);
	$effect(() => {
		const mq = matchMedia('(max-width: 48rem)');
		narrow = mq.matches;
		const on = () => (narrow = mq.matches);
		mq.addEventListener('change', on);
		return () => mq.removeEventListener('change', on);
	});

	function hrefFor(c: Container, p?: Panel): string {
		if (narrow) return p ? `/c/${c.id}?tab=${p}` : `/c/${c.id}`;
		return withParams(params, { c: c.id, panel: p ?? (c.id === chosenId ? panel : null), path: null, file: null, tab: null });
	}

	const stateHref = (s: string) => withParams(params, { state: s || null });
	const TABS = [
		{ id: '', key: 'containers.tab.all', n: () => counts.all },
		{ id: 'running', key: 'containers.tab.running', n: () => counts.running },
		{ id: 'stopped', key: 'containers.tab.stopped', n: () => counts.stopped }
	];
	// The tab of a Docker state chosen in the Status list.
	let activeTab = $derived(stateFilter === '' || stateFilter === 'running' || stateFilter === 'stopped' ? stateFilter : groupOf(stateFilter));

	function pickState(e: Event & { currentTarget: HTMLSelectElement }) {
		goto(stateHref(e.currentTarget.value), { reset: false }).catch(() => {});
	}

	// "/" moves to the search, unless one is typing.
	function onkeydown(e: KeyboardEvent) {
		if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;
		const el = e.target as HTMLElement;
		if (el.closest('input, textarea, select, [contenteditable="true"], .cm-editor')) return;
		e.preventDefault();
		search?.focus();
	}
</script>

<svelte:window {onkeydown} />

<div class="page" data-sveltekit-reset="false">
	<div class="top" class:with-detail={chosen !== null}>
		<section class="list panel" aria-labelledby="list-title">
			<header class="head">
				<div>
					<h1 id="list-title">{t('containers.title')}</h1>
					<p class="sub">{t('containers.subtitle')}</p>
				</div>
				<div class="head-right">
					{#if containerList.items}
						<span class="total">
							{tf(counts.all === 1 ? 'containers.total.one' : 'containers.total.other', { n: counts.all })}
						</span>
					{/if}
					<button
						type="button"
						class="icon-btn"
						onclick={() => containerList.load()}
						disabled={containerList.loading}
						aria-label={t('containers.refresh')}
						title={t('containers.refresh')}
					>
						<Icon name="refresh" size={20} />
					</button>
				</div>
			</header>

			<nav class="tabs" aria-label={t('containers.tabs')}>
				{#each TABS as tab (tab.id)}
					<a href={stateHref(tab.id)} aria-current={activeTab === tab.id ? 'page' : undefined}>
						{t(tab.key)} <span class="count">{tab.n()}</span>
					</a>
				{/each}
			</nav>

			<div class="filters">
				<label class="search">
					<span class="sr-only">{t('containers.search')}</span>
					<Icon name="search" />
					<input type="search" bind:this={search} bind:value={query} placeholder={t('containers.search')} />
					<kbd aria-hidden="true">/</kbd>
				</label>
				<label class="status">
					<span class="sr-only">{t('containers.status')}</span>
					<select value={presentStates.includes(stateFilter) ? stateFilter : ''} onchange={pickState}>
						<option value="">{t('containers.status.any')}</option>
						{#each presentStates as s (s)}
							<option value={s}>{stateLabel(s)}</option>
						{/each}
					</select>
				</label>
			</div>

			{#if error}
				<div class="msg">
					<p role="alert">{error}</p>
					<button type="button" class="btn small" onclick={() => containerList.load()}>{t('files.reload')}</button>
				</div>
			{:else if !containerList.items}
				<p class="msg muted">{t('containers.loading')}</p>
			{:else if all.length === 0}
				<p class="msg muted">{t('containers.empty')}</p>
			{:else if shown.length === 0}
				<p class="msg muted">{t('containers.noMatch')}</p>
			{:else}
				<ContainerTable list={shown} chosen={chosenId} capped={chosen !== null} {hrefFor} />
			{/if}
		</section>

		{#if chosen}
			{#key chosen.id}
				<aside class="side panel">
					<ContainerDetail
						id={chosen.id}
						mode="side"
						tab={detailTab}
						readOnly={readOnly()}
						tabHref={(k) => withParams(params, { tab: k === 'overview' ? null : k })}
						panelHref={(p) => withParams(params, { panel: p })}
						closeHref={withParams(params, { c: null, panel: null, tab: null, path: null, file: null })}
					/>
				</aside>
			{/key}
		{/if}
	</div>

	{#if chosen}
		{#key chosen.id}
			<BottomPanels id={chosen.id} name={chosen.name} stopped={!['running', 'paused'].includes(chosen.state)} readOnly={readOnly()} {panel} />
		{/key}
	{:else if params.get('panel') && containerList.items}
		<p class="pick panel"><Icon name="info" size={20} /> {t('containers.pick')}</p>
	{/if}
</div>

<style>
	.page {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.top {
		display: grid;
		gap: 1rem;
		align-items: start;
		min-width: 0;
	}
	@media (min-width: 64rem) {
		.top.with-detail {
			grid-template-columns: minmax(0, 1fr) minmax(20rem, 24rem);
		}
	}
	.list {
		display: grid;
		gap: 1rem;
		padding: 1.25rem;
		min-width: 0;
	}
	.side {
		padding: 1.25rem;
		min-width: 0;
	}
	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
	}
	.sub {
		color: var(--texte-doux);
		margin-top: 0.35rem;
	}
	.head-right {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		white-space: nowrap;
	}
	.total {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-weight: 550;
		font-size: var(--t-s);
	}
	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
	}
	.search {
		flex: 1 1 18rem;
		position: relative;
		display: flex;
		align-items: center;
		color: var(--texte-doux);
	}
	.search :global(.icon) {
		position: absolute;
		left: 0.65rem;
		pointer-events: none;
	}
	.search input {
		width: 100%;
		padding-left: 2.1rem;
		padding-right: 2rem;
	}
	kbd {
		position: absolute;
		right: 0.55rem;
		font-size: var(--t-xs);
		padding: 0 0.35rem;
		border: 1px solid var(--trait);
		border-radius: 4px;
		color: var(--texte-doux);
	}
	.status select {
		min-width: 11rem;
	}
	.msg {
		display: flex;
		gap: 0.75rem;
		align-items: center;
		padding: 1rem 0;
	}
	.pick {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 1rem 1.25rem;
		color: var(--texte-doux);
	}
	@media (max-width: 48rem) {
		.list {
			padding: 1rem 0.75rem;
			gap: 0.75rem;
		}
		.head {
			flex-direction: column;
			gap: 0.5rem;
		}
		h1 {
			font-size: var(--t-xl);
		}
		.status,
		.status select {
			width: 100%;
		}
	}
</style>
