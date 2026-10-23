<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, listChanges, type Change } from '#lib/api.ts';
	import { CHANGES_SHOWN, countChanges, kindLabel, kindLetter } from '#lib/changes.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let { id }: { id: string } = $props();

	let changes = $state<Change[] | null>(null);
	let error = $state('');
	let loading = $state(false);
	let reloads = $state(0); // bumped by the Reload button

	$effect(() => {
		const wanted = id;
		void reloads;
		let current = true;
		loading = true;
		error = '';
		listChanges(wanted)
			.then((l) => {
				if (current) changes = l ?? [];
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				changes = null;
				error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
		};
	});
</script>

<section class="changes">
	{#if error}
		<p role="alert">{error}</p>
		<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
	{:else if loading && !changes}
		<p>{t('files.loading')}</p>
	{:else if changes}
		{#if changes.length === 0}
			<p>{t('changes.empty')}</p>
		{:else}
			{@const n = countChanges(changes)}
			<p class="note">{t('changes.hint')}</p>
			<p>
				{t('changes.added')} : {n.added} · {t('changes.modified')} : {n.modified} · {t('changes.deleted')} : {n.deleted}
			</p>
			<ul>
				{#each changes.slice(0, CHANGES_SHOWN) as c, i (i)}
					<li>
						<abbr class="mark {c.kind}" title={kindLabel(c.kind)}>{kindLetter(c.kind)}</abbr>
						<span>{kindLabel(c.kind)}</span>
						{#if c.kind === 'deleted'}
							<s class="path">{c.path}</s>
						{:else}
							<a class="path" href={'?' + new URLSearchParams({ path: c.path })}>{c.path}</a>
						{/if}
					</li>
				{/each}
			</ul>
			{#if changes.length > CHANGES_SHOWN}
				<p class="note">{changes.length - CHANGES_SHOWN} {t('changes.more')}</p>
			{/if}
		{/if}
	{/if}
</section>

<style>
	ul { list-style: none; padding: 0; margin: 0; }
	li { display: grid; grid-template-columns: 1.5rem 5.5rem minmax(0, 1fr); gap: 0.25rem; padding: 0.15rem 0; border-bottom: 1px solid var(--trait-fin); }
	.path { word-break: break-all; }
	.note { font-style: italic; }
	.mark { font-weight: 700; text-decoration: none; }
	.added { color: var(--vert-texte); }
	.modified { color: var(--ambre-texte); }
	.deleted { color: var(--rouille-texte); }
	[role='alert'] { color: var(--rouille-texte); }
</style>
