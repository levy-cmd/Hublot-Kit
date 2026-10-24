<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { ApiError, getContainer, getFigures, type Figures, type Radiography } from '#lib/api.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import { containerList } from '#lib/containerList.svelte.ts';
	import { formatSize } from '#lib/files.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import {
		deleteSnapshot, groupSnapshots, listSnapshots, percent, planCost, snapshotPlans, sourceLabel, takeSnapshot,
		type SnapshotList, type SnapshotPlan, type SnapshotRow
	} from '#lib/snapshots.ts';

	// Spec v0.2 §5.7: nothing of a file's content, nor any fingerprint, ever
	// reaches the page. A photo is taken after a confirmation that says its
	// cost (Q57); the photo of the metadata comes first when the index of a
	// stopped container is already built.
	let list = $state<SnapshotList | null>(null);
	let error = $state('');
	let loading = $state(false);
	let picked = $state('');
	let target = $state<{ radio: Radiography; figures: Figures | null } | null>(null);
	let targetError = $state('');
	let confirm = $state<SnapshotPlan | null>(null);
	let removing = $state<SnapshotRow | null>(null);
	let actionError = $state('');
	let ctrl: AbortController | null = null;
	let poll: ReturnType<typeof setTimeout> | null = null;

	let groups = $derived(groupSnapshots(list?.snapshots ?? []));
	let plans = $derived(target ? snapshotPlans(target.radio.state.running, target.figures?.files) : []);
	let lastFailed = $derived(list?.last && list.last.result !== 'ok' ? list.last : null);

	function unauthorized(e: unknown): boolean {
		if (e instanceof ApiError && e.status === 401) {
			goto('/login').catch(() => {});
			return true;
		}
		return false;
	}

	async function load() {
		ctrl?.abort();
		ctrl = new AbortController();
		const signal = ctrl.signal;
		loading = true;
		try {
			const l = await listSnapshots(signal);
			if (signal.aborted) return;
			list = l;
			error = '';
			if (poll) clearTimeout(poll);
			poll = l.current ? setTimeout(load, 1000) : null;
		} catch (e) {
			if (signal.aborted || unauthorized(e)) return;
			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		} finally {
			if (!signal.aborted) loading = false;
		}
	}

	onMount(() => {
		load();
		if (!containerList.items) containerList.load();
		const c = page.url.searchParams.get('c');
		if (c) pick(c);
		return () => {
			ctrl?.abort();
			if (poll) clearTimeout(poll);
		};
	});

	let picking = 0;
	async function pick(ref: string) {
		const n = ++picking;
		picked = ref;
		target = null;
		targetError = '';
		if (!ref) return;
		try {
			const radio = await getContainer(ref);
			// By the full ID only (review of tranche 3): never a name or a short ID.
			const figures = await getFigures(radio.id).catch(() => null);
			if (n === picking) {
				target = { radio, figures };
				picked = radio.id;
			}
		} catch (e) {
			if (n !== picking || unauthorized(e)) return;
			targetError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		}
	}

	async function take(plan: SnapshotPlan) {
		const id = target?.radio.id;
		confirm = null;
		actionError = '';
		if (!id) return;
		try {
			await takeSnapshot(id, plan === 'index');
		} catch (e) {
			if (unauthorized(e)) return;
			actionError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			if (e instanceof ApiError && e.detail) actionError += ` (${e.detail})`;
		}
		await load();
	}

	async function remove(row: SnapshotRow) {
		removing = null;
		actionError = '';
		try {
			await deleteSnapshot(row.id);
		} catch (e) {
			if (unauthorized(e)) return;
			actionError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		}
		await load();
	}

	function when(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'medium' });
	}

	const containers = $derived(
		[...(containerList.items ?? [])].sort((a, b) => a.name.localeCompare(b.name))
	);
	const planLabel = (p: SnapshotPlan) => (p === 'index' ? t('snapshots.take.index') : plans.length > 1 ? t('snapshots.take.full') : t('snapshots.take'));
</script>

<div class="page">
	<section class="panel snaps" aria-labelledby="snapshots-title">
		<header class="head">
			<div>
				<h1 id="snapshots-title">{t('snapshots.title')}</h1>
				<p class="sub">{t('snapshots.subtitle')}</p>
				<p class="note">{t('snapshots.readOnly')}</p>
			</div>
			<button type="button" class="icon-btn" onclick={load} disabled={loading} aria-label={t('snapshots.refresh')} title={t('snapshots.refresh')}>
				<Icon name="refresh" size={20} />
			</button>
		</header>

		<div class="take">
			<label>
				<span>{t('snapshots.take.container')}</span>
				<select value={picked} onchange={(e) => pick(e.currentTarget.value)}>
					<option value="">{t('snapshots.take.pick')}</option>
					{#each containers as c (c.id)}
						<option value={c.id}>{c.name}</option>
					{/each}
					{#if picked && !containers.some((c) => c.id === picked) && target}
						<option value={picked}>{target.radio.name}</option>
					{/if}
				</select>
			</label>
			{#if picked && !target && !targetError}
				<p class="muted" aria-live="polite">{t('snapshots.take.loading')}</p>
			{/if}
			{#if targetError}<p role="alert">{targetError}</p>{/if}
			{#if target}
				<div class="plans">
					{#each plans as p, i (p)}
						<button type="button" class="btn" class:primary={i === 0} disabled={!!list?.current} onclick={() => (confirm = p)}>
							<Icon name="camera" />
							{planLabel(p)}
						</button>
					{/each}
				</div>
				{#if plans[0] === 'index'}<p class="note small">{t('snapshots.take.indexHint')}</p>{/if}
			{/if}
			{#if actionError}<p role="alert">{actionError}</p>{/if}
		</div>

		{#if list?.current}
			{@const c = list.current}
			<div class="progress" role="status">
				<p>
					{c.kind === 'count'
						? tf('snapshots.progress.count', { name: c.containerName ?? '', n: c.entries.toLocaleString(lang) })
						: tf('snapshots.progress', { name: c.containerName ?? '', n: c.entries.toLocaleString(lang), p: percent(c.progress) })}
				</p>
				<progress max="100" value={percent(c.progress)}></progress>
			</div>
		{:else if lastFailed}
			<p role="alert">{tf('snapshots.last.error', { error: errorMessage(lastFailed.result) })}{lastFailed.detail ? ` (${lastFailed.detail})` : ''}</p>
		{/if}

		{#if error}
			<p role="alert">{error}</p>
		{:else if !list}
			<p class="muted" aria-live="polite">{t('snapshots.loading')}</p>
		{:else if groups.length === 0}
			<p class="muted">{t('snapshots.empty')}</p>
		{:else}
			{#each groups as g (g.containerId)}
				<section class="group" aria-label={g.name}>
					<h2>
						<span class="name">{g.name}</span>
						{#if g.gone}<span class="pill warn">{t('snapshots.gone')}</span>{/if}
						<code class="mono id">{g.containerId.slice(0, 12)}</code>
					</h2>
					<table>
						<thead>
							<tr>
								<th scope="col">{t('snapshots.col.date')}</th>
								<th scope="col">{t('snapshots.col.source')}</th>
								<th scope="col">{t('snapshots.col.state')}</th>
								<th scope="col" class="num">{t('snapshots.col.entries')}</th>
								<th scope="col"><span class="sr-only">{t('snapshots.delete')}</span></th>
							</tr>
						</thead>
						<tbody>
							{#each g.rows as r (r.id)}
								<tr>
									<td>{when(r.createdAt)}</td>
									<td>{sourceLabel(r.source)}</td>
									<td>{t(`snapshots.state.${r.state}`)}</td>
									<td class="num">
										{r.entries.toLocaleString(lang)}
										{#if r.partial}<span class="pill warn" title={t('snapshots.partialHint')}>{t('snapshots.partial')}</span>{/if}
									</td>
									<td class="act">
										<button type="button" class="btn small" onclick={() => (removing = r)}>{t('snapshots.delete')}</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</section>
			{/each}
		{/if}
	</section>
</div>

{#if confirm && target}
	<ConfirmDialog
		title={t(`snapshots.confirm.${confirm}`)}
		container={target.radio.name}
		path="/"
		details={planCost(confirm, target.figures?.sizeRootFs, formatSize)}
		onconfirm={() => take(confirm!)}
		oncancel={() => (confirm = null)}
	/>
{/if}

{#if removing}
	<ConfirmDialog
		title={t('snapshots.delete.title')}
		container={removing.containerName}
		path={when(removing.createdAt)}
		details={[t('snapshots.delete.detail')]}
		onconfirm={() => remove(removing!)}
		oncancel={() => (removing = null)}
	/>
{/if}

<style>
	.page {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.snaps {
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
	.head .note {
		margin-top: 0.5rem;
	}
	.take {
		display: grid;
		gap: 0.6rem;
		padding: 0.9rem 1rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--releve);
	}
	.take label {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 0.75rem;
		font-weight: 550;
	}
	.take select {
		min-width: 14rem;
	}
	.plans {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
	.small {
		font-size: var(--t-xs);
	}
	.progress {
		display: grid;
		gap: 0.35rem;
	}
	progress {
		width: min(28rem, 100%);
		accent-color: var(--emeraude);
	}
	.group {
		display: grid;
		gap: 0.4rem;
	}
	h2 {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		font-size: var(--t-m);
	}
	.id {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--t-s);
	}
	th {
		text-align: left;
		font-weight: 600;
		color: var(--texte-doux);
		border-bottom: 1px solid var(--trait);
		padding: 0.35rem 0.5rem;
	}
	td {
		border-bottom: 1px solid var(--trait);
		padding: 0.4rem 0.5rem;
	}
	.num {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}
	.act {
		text-align: right;
		width: 1%;
		white-space: nowrap;
	}
	@media (max-width: 48rem) {
		.snaps {
			padding: 1rem 0.75rem;
		}
		h1 {
			font-size: var(--t-xl);
		}
		.take select {
			width: 100%;
		}
		th:nth-child(3),
		td:nth-child(3) {
			display: none;
		}
	}
</style>
