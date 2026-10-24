<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, getMeta, logsDownloadUrl, logsUrl, type LogLine } from '#lib/api.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import {
		appendLines, enqueue, filterLines, formatTs, levelOf, parseCount, parseLine, streamEnd, type ShownLine
	} from '#lib/logs.ts';

	// live false: the view is mounted but out of sight (a hidden bottom
	// panel). Its stream is closed, its lines kept; it resumes after the last
	// line received when live again.
	let { id, live = true }: { id: string; live?: boolean } = $props();

	type Status = 'connecting' | 'live' | 'reconnecting' | 'paused' | 'ended' | 'error';
	let status = $state<Status>('connecting');
	let error = $state('');
	let lines = $state.raw<ShownLine[]>([]); // always replaced whole
	let dropped = $state(0); // announced by the agent (rate)
	let removed = $state(0); // left the view (MAX_LINES)
	let query = $state('');
	let stdout = $state(true);
	let stderr = $state(true);
	let timestamps = $state(true);
	let paused = $state(false);
	let restarts = $state(0); // bumped by Resume / Follow again / Reload
	let shown = $derived(filterLines(lines, { query, stdout, stderr }));
	let box = $state<HTMLElement | null>(null);

	let lastId = ''; // ID of the last line received: where a resumption starts
	let streamOf = ''; // container the lines belong to
	let pending: LogLine[] = [];
	let frame = 0;

	// Lines arrive by hundreds: they are added once per animation frame.
	function flush() {
		frame = 0;
		const r = appendLines(lines, pending);
		pending = [];
		lines = r.lines;
		removed += r.removed;
	}

	$effect(() => {
		const wanted = id;
		void restarts;
		if (streamOf !== wanted) {
			// Another container: nothing of the previous one stays.
			streamOf = wanted;
			lastId = '';
			pending = [];
			lines = [];
			dropped = 0;
			removed = 0;
		}
		if (paused || !live) {
			status = 'paused'; // the stream is closed; lastId keeps the place
			return;
		}
		let current = true; // stale-response guard: false once this stream is replaced
		status = 'connecting';
		error = '';
		const es = new EventSource(logsUrl(wanted, lastId ? { after: lastId } : {}));
		const stop = (next: Status) => {
			es.close();
			status = next;
		};
		es.onopen = () => {
			if (current) status = 'live';
		};
		es.onmessage = (e: MessageEvent<string>) => {
			if (!current) return;
			const l = parseLine(e.data);
			if (!l) return;
			if (e.lastEventId) lastId = e.lastEventId;
			const cut = enqueue(pending, l); // bounded while the tab is hidden
			if (cut) removed += cut;
			if (!frame) frame = requestAnimationFrame(flush);
		};
		es.addEventListener('dropped', (e) => {
			if (current) dropped += parseCount((e as MessageEvent<string>).data);
		});
		for (const name of ['end', 'failure', 'evicted'] as const) {
			es.addEventListener(name, (e) => {
				if (!current) return;
				const r = streamEnd(name, (e as MessageEvent<string>).data);
				stop(r.status);
				if (r.login) goto('/login').catch(() => {});
				if (r.code) error = r.code === 'evicted' ? t('logs.evicted') : errorMessage(r.code);
			});
		}
		es.onerror = () => {
			if (!current) return;
			if (es.readyState === EventSource.CONNECTING) {
				status = 'reconnecting'; // the browser retries with Last-Event-ID
				return;
			}
			if (status === 'ended' || status === 'error') return;
			stop('error');
			// An HTTP error (401 after the session expired): EventSource cannot
			// read it, /api/meta tells.
			getMeta()
				.then(() => {
					if (current) error = errorMessage('network');
				})
				.catch((err) => {
					if (!current) return;
					if (err instanceof ApiError && err.status === 401) goto('/login').catch(() => {});
					else error = errorMessage(err instanceof ApiError ? err.code : 'unknown');
				});
		};
		return () => {
			current = false;
			es.close(); // leaving the tab frees the connection (6 per host in HTTP/1.1)
			if (frame) cancelAnimationFrame(frame);
			frame = 0;
			if (pending.length) flush(); // a new container resets them right after
		};
	});

	function pause() {
		paused = true;
	}

	function resume() {
		paused = false;
		restarts++;
	}

	// Auto-scroll (spec v0.2 §3): the view follows the end of the stream.
	// Scrolling up turns it off, the switch turns it back on; neither pauses
	// the stream.
	let follow = $state(true);

	function onScroll() {
		if (box) follow = box.scrollHeight - box.scrollTop - box.clientHeight < 8;
	}

	$effect(() => {
		void shown.length;
		if (box && follow) box.scrollTop = box.scrollHeight;
	});

	function setFollow(on: boolean) {
		follow = on;
		if (on && box) box.scrollTop = box.scrollHeight;
	}
</script>

<section class="logs">
	<div class="tools">
		<input type="search" placeholder={t('logs.search')} aria-label={t('logs.search')} bind:value={query} />
		<div class="checks" role="group" aria-label={t('logs.streams')}>
			<label><input type="checkbox" bind:checked={stdout} /> stdout</label>
			<label><input type="checkbox" bind:checked={stderr} /> stderr</label>
			<label><input type="checkbox" bind:checked={timestamps} /> {t('logs.timestamps')}</label>
		</div>
	</div>
	<div class="status">
		<p role="status">
			{#if status === 'connecting'}{t('logs.connecting')}
			{:else if status === 'live'}<span class="dot ok"></span> {t('logs.live')}
			{:else if status === 'reconnecting'}{t('logs.reconnecting')}
			{:else if status === 'paused'}{t('logs.paused')}
			{:else if status === 'ended'}{t('logs.ended')}{/if}
			<span class="muted">{shown.length} {t('logs.shown')}</span>
		</p>
		{#if status === 'paused'}
			<button type="button" class="btn small" onclick={resume}>{t('logs.resume')}</button>
		{:else if status === 'live' || status === 'connecting' || status === 'reconnecting'}
			<button type="button" class="btn small" onclick={pause}>{t('logs.pause')}</button>
		{:else if status === 'ended'}
			<button type="button" class="btn small" onclick={resume}>{t('logs.follow')}</button>
		{/if}
	</div>
	{#if error}
		<div class="error">
			<p role="alert">{error}</p>
			<button type="button" class="btn small" onclick={resume}>{t('files.reload')}</button>
		</div>
	{/if}
	{#if dropped}<p class="note">{dropped} {t('logs.dropped')}</p>{/if}
	{#if removed}<p class="note">{removed} {t('logs.removed')}</p>{/if}
	<!-- A scrolling region must take the focus, or the keyboard cannot scroll it. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div class="lines" bind:this={box} onscroll={onScroll} tabindex="0" role="log" aria-label={t('panels.logs')}>
		{#if lines.length === 0 && status !== 'connecting'}
			<p class="note">{t('logs.empty')}</p>
		{:else if shown.length === 0 && lines.length > 0}
			<p class="note">{t('logs.noMatch')}</p>
		{/if}
		{#each shown as l (l.key)}
			{@const lv = levelOf(l.line)}
			<div class="line {l.stream}" class:nots={!timestamps}>
				{#if timestamps}<time datetime={l.ts}>{formatTs(l.ts)}</time>{/if}
				<span class="level {lv ?? 'stream'}" title={t('logs.levelHint')}>{lv ?? l.stream}</span>
				<span class="text">{l.line}{#if l.truncated}<abbr class="cut" title={t('logs.truncated')}>…</abbr>{/if}</span>
			</div>
		{/each}
	</div>
	<div class="foot">
		<label class="switch" title={t('logs.autoscrollHint')}>
			<input type="checkbox" role="switch" checked={follow} onchange={(e) => setFollow(e.currentTarget.checked)} />
			<span class="track" aria-hidden="true"></span>
			{t('logs.autoscroll')}
		</label>
		{#if status !== 'error'}
			<a class="btn small" href={logsDownloadUrl(id)} download><Icon name="download" /> {t('logs.download')}</a>
		{/if}
	</div>
</section>

<style>
	.logs {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		min-width: 0;
		min-height: 0;
		height: 100%;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 0.75rem;
		align-items: center;
	}
	.tools input[type='search'] {
		flex: 1 1 10rem;
		min-height: 2rem;
	}
	.checks {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 0.75rem;
		font-size: var(--t-s);
		color: var(--texte-doux);
	}
	.checks label {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
	}
	.status {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		font-size: var(--t-s);
	}
	.status p {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}
	.error {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.lines {
		flex: 1;
		min-height: 12rem;
		overflow: auto;
		font-family: var(--mono);
		font-size: var(--t-xs);
		line-height: 1.6;
		padding: 0.5rem 0.6rem;
		background: var(--releve);
		border-radius: var(--rayon);
	}
	.line {
		display: grid;
		grid-template-columns: auto 3.25rem minmax(0, 1fr);
		gap: 0.75rem;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.line.nots {
		grid-template-columns: 3.25rem minmax(0, 1fr);
	}
	/* A narrow panel or a phone: the text takes its own line under the time
	   and the level. */
	.logs {
		container-type: inline-size;
	}
	@container (max-width: 34rem) {
		.line,
		.line.nots {
			grid-template-columns: auto minmax(0, 1fr);
			gap: 0 0.75rem;
			padding: 0.2rem 0;
			border-bottom: 1px solid var(--trait);
		}
		.line .text {
			grid-column: 1 / -1;
		}
	}
	time {
		color: var(--texte-doux);
	}
	.level {
		text-transform: uppercase;
		font-weight: 600;
		color: var(--texte-doux);
		cursor: help;
	}
	.level.error {
		color: var(--rouge-texte);
	}
	.level.warn {
		color: var(--ambre-texte);
	}
	.level.info {
		color: var(--emeraude-texte);
	}
	.level.stream {
		font-weight: 400;
		text-transform: none;
	}
	.cut {
		color: var(--ambre-texte);
		text-decoration: none;
	}
	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.switch {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-size: var(--t-s);
		cursor: pointer;
	}
	.switch input {
		position: absolute;
		opacity: 0;
		width: 2.25rem;
		height: 1.25rem;
		margin: 0;
		cursor: pointer;
	}
	.track {
		position: relative;
		width: 2.25rem;
		height: 1.25rem;
		border-radius: 999px;
		background: var(--releve);
		border: 1px solid var(--trait-fort);
		transition: background-color var(--vite);
	}
	.track::after {
		content: '';
		position: absolute;
		top: 2px;
		left: 2px;
		width: 0.875rem;
		height: 0.875rem;
		border-radius: 999px;
		background: var(--texte-doux);
		transition: transform var(--vite);
	}
	.switch input:checked + .track {
		background: var(--emeraude);
		border-color: var(--emeraude);
	}
	.switch input:checked + .track::after {
		transform: translateX(1rem);
		background: var(--sur-emeraude);
	}
	.switch input:focus-visible + .track {
		outline: 2px solid var(--focus);
		outline-offset: 2px;
	}
</style>
