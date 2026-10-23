<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onDestroy, onMount } from 'svelte';
	import { ApiError, revealEnv } from '#lib/api.ts';
	import { NO_PROJECT, stateLabel, stateTone } from '#lib/containers.ts';
	import { errorMessage, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import {
		addInvReveal, askInv, dropInvContainer, filterRows, forgetInv, getSecrets, invRevealed, noInvReveals,
		projectsOfRows, removeInvReveal, secretCount, sortRows, type SecretRow
	} from '#lib/inventory.ts';
	import { withParams } from '#lib/url.ts';

	// Spec v0.2 §5.9: names only. A revealed value lives in `reveals` only,
	// tied to its container: never in a browser store, the URL or the
	// console; folding the row, reloading, leaving the page and pagehide
	// forget it.
	let rows = $state<SecretRow[] | null>(null);
	let loading = $state(false);
	let error = $state('');
	let open = $state<Record<string, boolean>>({});
	let reveals = $state.raw(noInvReveals);
	let busy = $state(''); // "<id>/<name>" of the reveal in flight
	let revealError = $state<{ id: string; text: string } | null>(null);
	let ctrl: AbortController | null = null;

	let params = $derived(page.url.searchParams);
	let projects = $derived(projectsOfRows(rows ?? []));
	let projectFilter = $derived.by(() => {
		const p = params.get('project') ?? '';
		return p === NO_PROJECT || projects.includes(p) ? p : '';
	});
	let shown = $derived(sortRows(filterRows(rows ?? [], projectFilter)));
	let totals = $derived({
		secrets: shown.reduce((n, r) => n + secretCount(r), 0),
		containers: shown.filter((r) => secretCount(r) > 0).length,
		total: shown.length
	});

	async function load() {
		ctrl?.abort();
		ctrl = new AbortController();
		const signal = ctrl.signal;
		reveals = forgetInv(reveals);
		loading = true;
		error = '';
		try {
			const r = await getSecrets(signal);
			if (!signal.aborted) rows = r;
		} catch (e) {
			if (signal.aborted) return;
			if (e instanceof ApiError && e.status === 401) {
				goto('/login').catch(() => {});
				return;
			}
			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	onMount(() => {
		load();
		return () => ctrl?.abort();
	});
	onDestroy(() => (reveals = forgetInv(reveals)));

	function toggle(id: string) {
		if (open[id]) reveals = dropInvContainer(reveals, id);
		open[id] = !open[id];
	}

	async function reveal(id: string, name: string) {
		if (busy) return;
		busy = `${id}/${name}`;
		revealError = null;
		const asked = askInv(reveals, id); // before the await
		try {
			const value = await revealEnv(id, name);
			if (open[id]) reveals = addInvReveal(reveals, asked, name, value);
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				goto('/login').catch(() => {});
				return;
			}
			revealError = { id, text: errorMessage(e instanceof ApiError ? e.code : 'unknown') };
		} finally {
			busy = '';
		}
	}

	function pickProject(e: Event & { currentTarget: HTMLSelectElement }) {
		goto(withParams(params, { project: e.currentTarget.value || null }), { reset: false }).catch(() => {});
	}
</script>

<svelte:window onpagehide={() => (reveals = forgetInv(reveals))} />

<div class="page">
	<section class="panel inv" aria-labelledby="secrets-title">
		<header class="head">
			<div>
				<h1 id="secrets-title">{t('secrets.title')}</h1>
				<p class="sub">{t('secrets.subtitle')}</p>
			</div>
			<button
				type="button"
				class="icon-btn"
				onclick={load}
				disabled={loading}
				aria-label={t('secrets.refresh')}
				title={t('secrets.refresh')}
			>
				<Icon name="refresh" size={20} />
			</button>
		</header>

		<div class="filters">
			<label class="project">
				<span class="sr-only">{t('secrets.project')}</span>
				<select value={projectFilter} onchange={pickProject}>
					<option value="">{t('secrets.project.any')}</option>
					{#each projects as p (p)}
						<option value={p}>{p}</option>
					{/each}
					<option value={NO_PROJECT}>{t('secrets.project.none')}</option>
				</select>
			</label>
			{#if rows}
				<p class="summary" aria-live="polite">{tf('secrets.summary', totals)}</p>
			{/if}
		</div>
		<p class="note">{t('secrets.revealHint')}</p>

		{#if error}
			<p role="alert">{error}</p>
		{:else if !rows}
			<p class="muted" aria-live="polite">{t('secrets.loading')}</p>
		{:else if shown.length === 0}
			<p class="muted">{t('secrets.empty')}</p>
		{:else}
			<ul class="rows">
				{#each shown as r (r.id)}
					{@const n = secretCount(r)}
					{@const foldable = !r.unavailable && n + r.labels.length > 0}
					<li class="row" class:open={open[r.id]}>
						<div class="line">
							{#if foldable}
								<button
									type="button"
									class="fold"
									aria-expanded={open[r.id] ? 'true' : 'false'}
									aria-controls="inv-{r.id}"
									onclick={() => toggle(r.id)}
								>
									<span class="chev" aria-hidden="true">›</span>
									<span class="name">{r.name}</span>
								</button>
							{:else}
								<span class="fold static"><span class="chev" aria-hidden="true"></span><span class="name">{r.name}</span></span>
							{/if}
							<span class="meta">
								{#if r.project}<span class="proj mono">{r.project}{r.service ? ` / ${r.service}` : ''}</span>{/if}
								<span class="state tone-{stateTone(r.state)}"><span class="dot {stateTone(r.state)}"></span>{stateLabel(r.state)}</span>
							</span>
							<span class="counts">
								{#if r.unavailable}
									<span class="pill warn" title={t('secrets.unavailableHint')}>{t('secrets.unavailable')}</span>
								{:else}
									{#if r.env.length}<span class="pill ok">{tf('secrets.count.env', { n: r.env.length })}</span>{/if}
									{#if r.args.length}<span class="pill ok">{tf('secrets.count.args', { n: r.args.length })}</span>{/if}
									{#if r.labels.length}<span class="pill">{tf('secrets.count.labels', { n: r.labels.length })}</span>{/if}
									{#if n + r.labels.length === 0}<span class="muted small">{t('secrets.none')}</span>{/if}
								{/if}
							</span>
							<a class="icon-btn small" href="/c/{r.id}?tab=secrets" aria-label="{t('secrets.open')} {r.name}" title={t('secrets.open')}>
								<Icon name="expand" />
							</a>
						</div>
						{#if r.unavailable}
							<p class="note small">{t('secrets.unavailableHint')}</p>
						{/if}
						{#if foldable && open[r.id]}
							<div class="body" id="inv-{r.id}">
								{#if revealError?.id === r.id}<p role="alert">{revealError.text}</p>{/if}
								{#if r.env.length}
									<h2>{t('secrets.env')}</h2>
									<ul class="env">
										{#each r.env as name (name)}
											{@const value = invRevealed(reveals, r.id, name)}
											<li>
												<code class="vname">{name}</code>
												{#if value === null}
													<span class="masked">••••••<span class="sr-only"> ({t('secrets.masked')})</span></span>
													<button type="button" class="btn small" disabled={busy !== ''} onclick={() => reveal(r.id, name)}>
														{t('secrets.reveal')}
													</button>
												{:else}
													<code class="value">{value}</code>
													<button type="button" class="btn small" onclick={() => (reveals = removeInvReveal(reveals, r.id, name))}>
														{t('secrets.hide')}
													</button>
												{/if}
											</li>
										{/each}
									</ul>
								{/if}
								{#if r.args.length}
									<h2>{t('secrets.args')}</h2>
									<ul class="plain">
										{#each r.args as a, i (i)}<li><code>{a}</code></li>{/each}
									</ul>
									<p class="note small">{t('secrets.argsHint')}</p>
								{/if}
								{#if r.labels.length}
									<h2>{t('secrets.labels')}</h2>
									<ul class="chips">
										{#each r.labels as k (k)}<li><code>{k}</code></li>{/each}
									</ul>
									<p class="note small">{t('secrets.labelsHint')}</p>
								{/if}
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>

<style>
	.page {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.inv {
		display: grid;
		gap: 1rem;
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
		max-width: 48rem;
	}
	.filters {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem 1.25rem;
	}
	.project select {
		min-width: 12rem;
	}
	.summary {
		font-size: var(--t-s);
		font-weight: 550;
	}
	.rows {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--trait);
	}
	.row {
		border-bottom: 1px solid var(--trait);
	}
	.row.open {
		background: var(--releve);
	}
	.line {
		display: grid;
		grid-template-columns: minmax(10rem, 1.2fr) minmax(8rem, 1fr) minmax(0, 1.4fr) auto;
		align-items: center;
		gap: 0.75rem;
		padding: 0.55rem 0.5rem;
	}
	.fold {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		padding: 0.2rem 0;
		border: 0;
		background: none;
		color: var(--texte);
		font-weight: 600;
		text-align: left;
		cursor: pointer;
	}
	.fold.static {
		cursor: default;
	}
	.fold:hover .name {
		color: var(--emeraude-texte);
	}
	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.chev {
		display: inline-block;
		width: 0.8rem;
		color: var(--texte-doux);
		transition: transform var(--vite);
	}
	.open .chev {
		transform: rotate(90deg);
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.75rem;
		min-width: 0;
		font-size: var(--t-s);
	}
	.proj {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.counts {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		min-width: 0;
	}
	.small {
		font-size: var(--t-xs);
	}
	.row > .note {
		padding: 0 0.5rem 0.55rem 1.7rem;
	}
	.body {
		display: grid;
		gap: 0.5rem;
		padding: 0.25rem 0.75rem 1rem 1.7rem;
	}
	h2 {
		font-size: var(--t-s);
		color: var(--texte-doux);
		font-weight: 600;
		margin-top: 0.4rem;
	}
	ul.env,
	ul.plain,
	ul.chips {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.env li {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 0.75rem;
		align-items: center;
		padding: 0.35rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.env li:last-child {
		border-bottom: 0;
	}
	.vname {
		font-weight: 600;
	}
	.value {
		min-width: 0;
		flex: 1;
		overflow-wrap: anywhere;
	}
	.masked {
		color: var(--texte-doux);
		letter-spacing: 0.1em;
	}
	.plain li {
		padding: 0.15rem 0;
		overflow-wrap: anywhere;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.chips li {
		padding: 0.05rem 0.5rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--surface);
		font-size: var(--t-xs);
		overflow-wrap: anywhere;
	}
	@media (max-width: 48rem) {
		.inv {
			padding: 1rem 0.75rem;
		}
		.head {
			gap: 0.5rem;
		}
		h1 {
			font-size: var(--t-xl);
		}
		.line {
			grid-template-columns: minmax(0, 1fr) auto;
		}
		.line > a {
			grid-row: 1;
			grid-column: 2;
		}
		.meta,
		.counts {
			grid-column: 1 / -1;
			padding-left: 1.2rem;
		}
		.project,
		.project select {
			width: 100%;
		}
		.body,
		.row > .note {
			padding-left: 1.2rem;
		}
	}
</style>
