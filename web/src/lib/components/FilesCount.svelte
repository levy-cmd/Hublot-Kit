<script lang="ts">
	import { ApiError, type Figures } from '#lib/api.ts';
	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import { countFiles, percent, type FilesCount } from '#lib/snapshots.ts';

	// The "Files" figure (spec v0.2 §5.3): at no cost when the agent knows it
	// (index already built, last snapshot, count kept), else a "Count" button
	// that starts the bounded count after a confirmation of its cost.
	let {
		id,
		name,
		running,
		files,
		loaded,
		when
	}: {
		id: string | null; // the full ID
		name: string;
		running: boolean;
		files: Figures['files'] | undefined;
		loaded: boolean;
		when: (iso: string) => string;
	} = $props();

	let counted = $state<FilesCount | null>(null);
	let asking = $state(false);
	let busy = $state<number | null>(null); // progress while counting
	let failed = $state('');
	let timer: ReturnType<typeof setTimeout> | null = null;
	let alive = true;

	let shown = $derived(files ?? counted);

	$effect(() => {
		void id; // another container: start again
		counted = null;
		busy = null;
		failed = '';
		return () => {
			if (timer) clearTimeout(timer);
		};
	});
	$effect(() => () => (alive = false));

	async function step(target: string) {
		try {
			const r = await countFiles(target);
			if (!alive || target !== id) return;
			counted = r;
			busy = null;
		} catch (e) {
			if (!alive || target !== id) return;
			if (e instanceof ApiError && e.code === 'indexing') {
				busy = e.progress ?? 0;
				timer = setTimeout(() => step(target), 1000);
				return;
			}
			busy = null;
			failed = tf('snapshots.count.failed', { error: errorMessage(e instanceof ApiError ? e.code : 'unknown') });
		}
	}

	function start() {
		asking = false;
		failed = '';
		if (!id) return;
		busy = 0;
		step(id);
	}
</script>

<dd
	title={!shown
		? t('snapshots.filesNone')
		: shown.source === 'index'
			? tf('container.figure.filesAt', { at: when(shown.at) })
			: tf(shown.source === 'snapshot' ? 'snapshots.filesAt.snapshot' : 'snapshots.filesAt.count', { at: when(shown.at) })}
>
	{#if shown}
		{shown.count.toLocaleString(lang)}
	{:else if busy !== null}
		<span class="sub" role="status">{tf('snapshots.count.busy', { p: percent(busy) })}</span>
	{:else if loaded && id}
		<button type="button" class="btn small" onclick={() => (asking = true)}>{t('snapshots.count')}</button>
	{:else}
		—
	{/if}
	{#if failed}<span class="sub" role="alert">{failed}</span>{/if}
</dd>

{#if asking}
	<ConfirmDialog
		title={t('snapshots.count.title')}
		container={name}
		path="/"
		details={[t(running ? 'snapshots.count.running' : 'snapshots.count.stopped')]}
		onconfirm={start}
		oncancel={() => (asking = false)}
	/>
{/if}
