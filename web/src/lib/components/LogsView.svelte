<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, getMeta, logsDownloadUrl, logsUrl, type LogLine } from '#lib/api.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import { appendLines, enqueue, filterLines, formatTs, parseCount, parseLine, streamEnd, type ShownLine } from '#lib/logs.ts';

	let { id }: { id: string } = $props();

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
	let atBottom = true; // the view follows the end unless the user scrolled up

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
		if (paused) {
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

	function onScroll() {
		if (box) atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 8;
	}

	$effect(() => {
		void shown.length;
		if (box && atBottom) box.scrollTop = box.scrollHeight;
	});
</script>

<section class="logs">
	<div class="tools">
		<input type="search" placeholder={t('logs.search')} aria-label={t('logs.search')} bind:value={query} />
		<label><input type="checkbox" bind:checked={stdout} /> stdout</label>
		<label><input type="checkbox" bind:checked={stderr} /> stderr</label>
		<label><input type="checkbox" bind:checked={timestamps} /> {t('logs.timestamps')}</label>
		{#if status === 'paused'}
			<button type="button" onclick={resume}>{t('logs.resume')}</button>
		{:else if status === 'live' || status === 'connecting' || status === 'reconnecting'}
			<button type="button" onclick={pause}>{t('logs.pause')}</button>
		{/if}
		{#if status !== 'error'}<a href={logsDownloadUrl(id)} download>{t('logs.download')}</a>{/if}
	</div>
	<p class="status" role="status">
		{#if status === 'connecting'}{t('logs.connecting')}
		{:else if status === 'live'}<span class="live">●</span> {t('logs.live')}
		{:else if status === 'reconnecting'}{t('logs.reconnecting')}
		{:else if status === 'paused'}{t('logs.paused')}
		{:else if status === 'ended'}{t('logs.ended')}{/if}
		· {shown.length} {t('logs.shown')}
	</p>
	{#if status === 'ended'}
		<button type="button" onclick={resume}>{t('logs.follow')}</button>
	{/if}
	{#if error}
		<p role="alert">{error}</p>
		<button type="button" onclick={resume}>{t('files.reload')}</button>
	{/if}
	{#if dropped}<p class="note">{dropped} {t('logs.dropped')}</p>{/if}
	{#if removed}<p class="note">{removed} {t('logs.removed')}</p>{/if}
	<div class="lines" bind:this={box} onscroll={onScroll}>
		{#if lines.length === 0 && status !== 'connecting'}
			<p class="note">{t('logs.empty')}</p>
		{:else if shown.length === 0 && lines.length > 0}
			<p class="note">{t('logs.noMatch')}</p>
		{/if}
		{#each shown as l (l.key)}
			<div class="line {l.stream}" class:nots={!timestamps}>
				{#if timestamps}<time datetime={l.ts}>{formatTs(l.ts)}</time>{/if}
				<span class="text">{l.line}</span>
				{#if l.truncated}<abbr class="cut" title={t('logs.truncated')}>…</abbr>{/if}
			</div>
		{/each}
	</div>
</section>

<style>
	.logs { min-width: 0; }
	.tools { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: center; }
	.tools input[type='search'] { flex: 1 1 12rem; min-width: 0; }
	.status { margin: 0.5rem 0; font-size: 0.9rem; }
	.live { color: var(--vert-texte); }
	.note { font-style: italic; }
	.lines {
		font-family: var(--mono); font-size: var(--t-xs); max-height: 70vh; overflow: auto;
		border: 1px solid var(--trait-fin); padding: 0.25rem 0.5rem; background: var(--fond-releve);
	}
	.line { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 0.75rem; white-space: pre-wrap; overflow-wrap: anywhere; }
	.line.nots { grid-template-columns: minmax(0, 1fr) auto; }
	.line time { color: var(--texte-doux); }
	.line.stderr .text { color: var(--rouille-texte); }
	.cut { color: var(--ambre-texte); text-decoration: none; }
	[role='alert'] { color: var(--rouille-texte); }
</style>
