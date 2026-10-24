<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { ApiError, getContainer, getFigures, type Figures, type Radiography } from '#lib/api.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import FilePreview from '#lib/components/FilePreview.svelte';
	import UnifiedDiff from '#lib/components/UnifiedDiff.svelte';
	import { containerList } from '#lib/containerList.svelte.ts';
	import { formatSize, imageFsErrorMessage, indexingPercent, pollIndexing } from '#lib/files.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import {
		compareCandidates, compareSnapshots, contentPlan, deleteSnapshot, dirTimeOnly, fieldLabel, filterResults,
		CHANGE_KINDS, findResult, groupSnapshots, imageMatch, kindCounts, listSnapshots, percent, planCost, snapshotPlans, sourceLabel,
		takeSnapshot, type CompareKind, type Comparison, type SnapshotList, type SnapshotPlan, type SnapshotRow
	} from '#lib/snapshots.ts';
	import { withParams } from '#lib/url.ts';

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

	// The comparison (spec v0.2 §5.7, Q59), in the address: ?compare=<sid>
	// &with=<sid2>|current&path=<a result>. Nothing of a content is kept:
	// between two photos no content is shown; against the current container
	// the current content, and a line diff only when image-match proves that
	// the old content was the image's version.
	let compareId = $derived(page.url.searchParams.get('compare'));
	let withId = $derived(page.url.searchParams.get('with'));
	let selPath = $derived(page.url.searchParams.get('path'));
	let aRow = $derived(list?.snapshots.find((r) => r.id === compareId) ?? null);
	let candidates = $derived(aRow && list ? compareCandidates(list.snapshots, aRow) : []);
	let choice = $state('');
	let confirmCurrent = $state<{ name: string; running: boolean } | null>(null);
	let choiceError = $state('');
	let cmp = $state<Comparison | null>(null);
	let cmpError = $state('');
	let cmpProgress = $state<number | null>(null);
	let shownKinds = $state<CompareKind[]>([...CHANGE_KINDS]);
	let showDirTime = $state(false);
	let limit = $state(500);
	let current = $derived(cmp?.b.id === 'current');
	let counts = $derived(cmp ? kindCounts(cmp.results, showDirTime) : null);
	let dirTimeCount = $derived(cmp ? cmp.results.filter(dirTimeOnly).length : 0);
	let shown = $derived(cmp ? filterResults(cmp.results, new Set(shownKinds), showDirTime) : []);
	let selected = $derived(cmp && selPath ? findResult(cmp, selPath) : null);
	let plan = $derived(selected ? contentPlan(selected, current) : 'none');

	const href = (set: Record<string, string | null>) => withParams(page.url.searchParams, set);
	const compareHref = (sid: string) => href({ compare: sid, with: null, path: null });

	$effect(() => {
		const a = compareId;
		const b = withId;
		cmp = null;
		cmpError = '';
		cmpProgress = null;
		limit = 500;
		if (!a || !b) return;
		const ctrl = new AbortController();
		const stop = pollIndexing(
			() => compareSnapshots(a, b, ctrl.signal),
			{
				indexing: (p) => (cmpProgress = indexingPercent(p)),
				result: (r) => {
					cmp = r;
					cmpProgress = null;
					// The agent's job is over: the list shows it no more.
					if (b === 'current') load();
				},
				error: (e) => {
					cmpProgress = null;
					if (unauthorized(e)) return;
					cmpError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
					if (e instanceof ApiError && e.detail) cmpError += ` (${e.detail})`;
				}
			},
			1000
		);
		return () => {
			stop();
			ctrl.abort();
		};
	});
	// Another photo compared: its own choice of B.
	$effect(() => {
		if (compareId !== undefined) {
			choice = '';
			choiceError = '';
		}
	});

	async function runCompare() {
		const a = aRow;
		choiceError = '';
		if (!a || !choice) return;
		if (choice !== 'current') {
			await goto(href({ compare: a.id, with: choice, path: null }));
			return;
		}
		try {
			// The cost said before the capture depends on the state now.
			const radio = await getContainer(a.containerId);
			confirmCurrent = { name: radio.name, running: radio.state.running };
		} catch (e) {
			if (unauthorized(e)) return;
			choiceError = e instanceof ApiError && e.code === 'not_found' ? t('snapshots.compare.gone') : errorMessage(e instanceof ApiError ? e.code : 'unknown');
		}
	}

	async function compareCurrent() {
		const a = aRow;
		confirmCurrent = null;
		if (a) await goto(href({ compare: a.id, with: 'current', path: null }));
	}

	// image-match of the result chosen, against the current container.
	let match = $state<{ path: string; progress: number | null; value: boolean | null; error: string } | null>(null);
	$effect(() => {
		const sid = compareId;
		const p = selPath;
		match = null;
		if (!sid || !p || plan !== 'match') return;
		match = { path: p, progress: null, value: null, error: '' };
		const ctrl = new AbortController();
		const set = (m: Partial<NonNullable<typeof match>>) => {
			if (match?.path === p) match = { ...match, ...m };
		};
		const stop = pollIndexing(() => imageMatch(sid, p, ctrl.signal), {
			indexing: (pr) => set({ progress: indexingPercent(pr) }),
			result: (v) => set({ value: v, progress: null }),
			error: (e) => {
				if (unauthorized(e)) return;
				set({ value: false, progress: null, error: imageFsErrorMessage(e instanceof ApiError ? e.code : 'unknown', true) });
			}
		});
		return () => {
			stop();
			ctrl.abort();
		};
	});

	function toggleKind(k: CompareKind, on: boolean) {
		shownKinds = on ? [...shownKinds, k] : shownKinds.filter((x) => x !== k);
		limit = 500;
	}

	const sideLabel = (s: Comparison['a']) => (s.id === 'current' ? t('snapshots.compare.current') : when(s.createdAt));

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
						: tf(c.kind === 'compare' ? 'snapshots.progress.compare' : 'snapshots.progress', {
								name: c.containerName ?? '',
								n: c.entries.toLocaleString(lang),
								p: percent(c.progress)
							})}
				</p>
				<progress max="100" value={percent(c.progress)}></progress>
			</div>
		{:else if lastFailed}
			<p role="alert">{tf('snapshots.last.error', { error: errorMessage(lastFailed.result) })}{lastFailed.detail ? ` (${lastFailed.detail})` : ''}</p>
		{/if}

		{#if compareId && aRow}
			<section class="compare" aria-labelledby="compare-title">
				<header class="chead">
					<div>
						<h2 id="compare-title">{t('snapshots.compare.title')}</h2>
						<p class="sub">{tf('snapshots.compare.a', { name: aRow.containerName, date: when(aRow.createdAt) })}</p>
					</div>
					<a class="icon-btn" href={href({ compare: null, with: null, path: null })} aria-label={t('snapshots.compare.close')} title={t('snapshots.compare.close')}>
						<Icon name="close" />
					</a>
				</header>
				<div class="pickb">
					<label>
						<span>{t('snapshots.compare.with')}</span>
						<select bind:value={choice}>
							<option value="">{t('snapshots.compare.pick')}</option>
							{#if !aRow.containerGone}<option value="current">{t('snapshots.compare.current')}</option>{/if}
							{#each candidates as r (r.id)}
								<option value={r.id}>{r.containerId === aRow.containerId ? when(r.createdAt) : tf('snapshots.compare.otherContainer', { date: when(r.createdAt) })}</option>
							{/each}
						</select>
					</label>
					<button type="button" class="btn primary" disabled={!choice || (choice === 'current' && !!list?.current)} onclick={runCompare}>
						<Icon name="diff" />
						{t('snapshots.compare.run')}
					</button>
				</div>
				{#if choiceError}<p role="alert">{choiceError}</p>{/if}

				{#if withId}
					{#if cmpError}
						<p role="alert">{cmpError}</p>
					{:else if cmpProgress !== null}
						<div class="progress" role="status">
							<p>{tf('snapshots.compare.progress', { p: cmpProgress })}</p>
							<progress max="100" value={cmpProgress}></progress>
						</div>
					{:else if !cmp}
						<p class="muted" aria-live="polite">{t('snapshots.compare.loading')}</p>
					{:else}
						<p class="sides">
							<span>{sideLabel(cmp.a)}</span>
							<span aria-hidden="true">→</span>
							<span>{sideLabel(cmp.b)}</span>
						</p>
						{#if !current}<p class="note">{t('snapshots.compare.noContent')}</p>{/if}
						{#if !cmp.contentCompared}<p class="note">{t('snapshots.compare.notCompared')}</p>{/if}
						{#if cmp.b.partial && current}<p class="note warn">{t('snapshots.compare.partial')}</p>{/if}
						{#if cmp.truncated}<p class="note warn">{tf('snapshots.compare.truncated', { n: cmp.results.length.toLocaleString(lang) })}</p>{/if}
						{#if cmp.results.length === 0 && cmp.unknownCount === 0}
							<p class="muted">{t('snapshots.compare.none')}</p>
						{:else}
							{#if cmp.results.length === 0}<p class="muted">{t('snapshots.compare.noChange')}</p>{/if}
							{#if cmp.unknownCount > 0}
								<!-- Review of tranche 9: the unknown apart, folded, never in the way of a change. -->
								<details class="unknown">
									<summary>{tf('snapshots.compare.unknown', { n: cmp.unknownCount.toLocaleString(lang) })}</summary>
									<ul>
										{#each cmp.unknown as r (r.path)}
											<li>
												<a href={href({ path: r.path })} aria-current={r.path === selPath ? 'true' : undefined} class="mono rpath">{r.path}</a>
											</li>
										{/each}
									</ul>
									{#if cmp.unknownCount > cmp.unknown.length}
										<p class="muted">{tf('snapshots.compare.unknownMore', { n: (cmp.unknownCount - cmp.unknown.length).toLocaleString(lang) })}</p>
									{/if}
								</details>
							{/if}
							{#if cmp.results.length > 0}
							<fieldset class="filters">
								<legend>{t('snapshots.compare.filters')}</legend>
								{#each CHANGE_KINDS as k (k)}
									<label class="kind-filter">
										<input type="checkbox" checked={shownKinds.includes(k)} onchange={(e) => toggleKind(k, e.currentTarget.checked)} />
										<span class="badge {k}">{t(`snapshots.kind.${k}`)}</span>
										<span class="count">{counts?.[k] ?? 0}</span>
									</label>
								{/each}
								{#if dirTimeCount > 0}
									<label class="kind-filter">
										<input type="checkbox" bind:checked={showDirTime} />
										{tf('snapshots.compare.dirTime', { n: dirTimeCount })}
									</label>
								{/if}
							</fieldset>
							{/if}
							<div class="cgrid">
								<div class="results">
									{#if shown.length === 0}
										{#if cmp.results.length > 0}<p class="muted">{t('snapshots.compare.noneShown')}</p>{/if}
									{:else}
										<ul aria-label={t('snapshots.compare.title')}>
											{#each shown.slice(0, limit) as r (r.path)}
												<li>
													<a href={href({ path: r.path })} aria-current={r.path === selPath ? 'true' : undefined} data-kind={r.kind}>
														<span class="badge {r.kind}">{t(`snapshots.kind.${r.kind}`)}</span>
														<span class="mono rpath">{r.path}</span>
														{#if r.fields.length}<span class="fields">{r.fields.map(fieldLabel).join(', ')}</span>{/if}
													</a>
												</li>
											{/each}
										</ul>
										{#if shown.length > limit}
											<button type="button" class="btn small" onclick={() => (limit += 500)}>
												{tf('snapshots.compare.more', { n: Math.min(500, shown.length - limit) })}
											</button>
										{/if}
									{/if}
								</div>
								<div class="content" aria-live="polite">
									{#if !selected}
										<p class="muted">{t('snapshots.compare.pickResult')}</p>
									{:else if plan === 'none'}
										<p class="muted">{selected.kind === 'modified' ? t('snapshots.compare.nothingKept') : t('snapshots.compare.noFile')}</p>
									{:else if plan === 'match' && match?.value === null}
										<p role="status">
											{match.progress === null
												? t('snapshots.compare.matching')
												: tf('snapshots.compare.matchingProgress', { p: match.progress })}
										</p>
									{:else if plan === 'match' && match?.value === true}
										<p class="note">{t('snapshots.compare.fromImage')}</p>
										{#key selected.path}
											<UnifiedDiff id={cmp.a.containerId} change={{ path: selected.path, kind: 'modified' }} stopped={cmp.b.state === 'stopped'} />
										{/key}
									{:else}
										{#if plan === 'match'}
											<p class="note">{t('snapshots.compare.oldNotKept')}</p>
											{#if match?.error}<p class="muted">{match.error}</p>{/if}
										{/if}
										{#key selected.path}
											<FilePreview id={cmp.a.containerId} name={cmp.b.containerName} path={selected.path} stopped={cmp.b.state === 'stopped'} readOnly />
										{/key}
									{/if}
								</div>
							</div>
						{/if}
					{/if}
				{/if}
			</section>
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
										<a class="btn small" href={compareHref(r.id)} aria-current={r.id === compareId ? 'true' : undefined}>{t('snapshots.compare')}</a>
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

{#if confirmCurrent && aRow}
	<ConfirmDialog
		title={t('snapshots.compare.confirm')}
		container={confirmCurrent.name}
		path="/"
		details={[...planCost(confirmCurrent.running ? 'running' : 'stopped', null, formatSize), t('snapshots.compare.confirmNote')]}
		onconfirm={compareCurrent}
		oncancel={() => (confirmCurrent = null)}
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
	.compare {
		display: grid;
		gap: 0.75rem;
		padding: 0.9rem 1rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		min-width: 0;
	}
	.chead {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 1rem;
	}
	.pickb {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 0.75rem;
	}
	.pickb label {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 0.75rem;
		font-weight: 550;
	}
	.pickb select {
		min-width: 14rem;
		max-width: 100%;
	}
	.sides {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		font-weight: 550;
	}
	.warn {
		color: var(--ambre-texte);
	}
	.filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem 1rem;
		border: 0;
		padding: 0;
		margin: 0;
	}
	.filters legend {
		font-weight: 600;
		color: var(--texte-doux);
		margin-bottom: 0.3rem;
	}
	.kind-filter {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		font-size: var(--t-s);
	}
	.count {
		color: var(--texte-doux);
		font-variant-numeric: tabular-nums;
	}
	.badge {
		display: inline-block;
		padding: 0.05rem 0.5rem;
		border-radius: 999px;
		font-size: var(--t-xs);
		font-weight: 600;
		background: var(--releve);
		color: var(--texte-doux);
		white-space: nowrap;
	}
	.badge.added {
		background: var(--diff-ajout);
		color: var(--emeraude-texte);
	}
	.badge.deleted {
		background: var(--diff-suppr);
		color: var(--rouge-texte);
	}
	.badge.modified,
	.badge.type {
		color: var(--ambre-texte);
	}
	.cgrid {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	@media (min-width: 70rem) {
		.cgrid {
			grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
		}
	}
	.results ul {
		list-style: none;
		margin: 0;
		padding: 0;
		max-height: 70vh;
		overflow: auto;
	}
	.results a {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.2rem 0.5rem;
		padding: 0.3rem 0.4rem;
		border-bottom: 1px solid var(--trait);
		color: inherit;
		text-decoration: none;
		min-width: 0;
	}
	.results a[aria-current] {
		background: var(--releve);
	}
	.rpath {
		font-size: var(--t-s);
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.fields {
		color: var(--texte-doux);
		font-size: var(--t-xs);
	}
	.content {
		min-width: 0;
	}
	.unknown summary {
		cursor: pointer;
		color: var(--texte-doux);
	}
	.unknown ul {
		list-style: none;
		margin: 0.4rem 0 0;
		padding: 0;
		max-height: 40vh;
		overflow: auto;
	}
	.unknown a {
		display: block;
		padding: 0.2rem 0.4rem;
		color: inherit;
		text-decoration: none;
		overflow-wrap: anywhere;
	}
	.unknown a[aria-current] {
		background: var(--releve);
	}
	.act .btn + .btn {
		margin-left: 0.3rem;
	}
	@media (max-width: 48rem) {
		.pickb select {
			width: 100%;
		}
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
