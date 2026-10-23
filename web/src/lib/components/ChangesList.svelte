<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, listChanges, type Change } from '#lib/api.ts';
	import { CHANGES_SHOWN, countChanges, kindLabel, kindLetter } from '#lib/changes.ts';
	import { errorMessage, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';

	// F5: the paths changed since the image. fileHref: the link that opens a
	// path in the file explorer (it keeps the page's other parameters).
	// diffHref, when given: the link that opens the content diff of a path
	// (F17), deleted ones included; selected, the path whose diff is shown.
	let {
		id,
		fileHref,
		diffHref,
		selected
	}: { id: string; fileHref: (path: string) => string; diffHref?: (c: Change) => string; selected?: string } = $props();

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

<!-- A long path breaks after a slash, not in the middle of a name. -->
{#snippet segments(path: string)}{#each path.split('/') as s, i (i)}{#if i > 0}/<wbr />{/if}{s}{/each}{/snippet}

<section class="changes">
	{#if error}
		<p role="alert">{error}</p>
		<button type="button" class="btn small" onclick={() => reloads++}>{t('files.reload')}</button>
	{:else if loading && !changes}
		<p class="muted">{t('files.loading')}</p>
	{:else if changes}
		{@const n = countChanges(changes)}
		<p class="counts">
			<span class="badge added">{t('changes.count.added')} <b>{n.added}</b></span>
			<span class="badge modified">{t('changes.count.modified')} <b>{n.modified}</b></span>
			<span class="badge deleted">{t('changes.count.deleted')} <b>{n.deleted}</b></span>
		</p>
		{#if changes.length === 0}
			<p class="muted">{t('changes.empty')}</p>
		{:else}
			<ul>
				{#each changes.slice(0, CHANGES_SHOWN) as c, i (i)}
					<li>
						<span class="icon {c.kind}"><Icon name="file" /></span>
						{#if diffHref}
							<a
								class="path mono"
								class:deleted={c.kind === 'deleted'}
								href={diffHref(c)}
								aria-current={selected === c.path ? 'true' : undefined}
								title={tf('diff.show', { path: c.path })}>{@render segments(c.path)}</a
							>
						{:else if c.kind === 'deleted'}
							<s class="path mono">{@render segments(c.path)}</s>
						{:else}
							<a class="path mono" href={fileHref(c.path)}>{@render segments(c.path)}</a>
						{/if}
						<span class="badge {c.kind}" title={kindLabel(c.kind)}><span class="long">{kindLabel(c.kind)}</span><span class="short" aria-hidden="true">{kindLetter(c.kind)}</span></span>
					</li>
				{/each}
			</ul>
			{#if changes.length > CHANGES_SHOWN}
				<p class="note">{changes.length - CHANGES_SHOWN} {t('changes.more')}</p>
			{/if}
			<p class="note">{t('changes.hint')}</p>
		{/if}
	{/if}
</section>

<style>
	.changes {
		display: grid;
		gap: 0.75rem;
		align-content: start;
		min-width: 0;
	}
	.counts {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.05rem 0.5rem;
		border-radius: 999px;
		font-size: var(--t-xs);
		font-weight: 600;
		white-space: nowrap;
	}
	.badge b {
		font-weight: 700;
	}
	.badge.added {
		background: var(--diff-ajout);
		color: var(--emeraude-texte);
	}
	.badge.modified {
		background: var(--releve);
		color: var(--ambre-texte);
	}
	.badge.deleted {
		background: var(--diff-suppr);
		color: var(--rouge-texte);
	}
	ul {
		list-style: none;
		padding: 0;
		margin: 0;
	}
	li {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0;
		border-bottom: 1px solid var(--trait);
	}
	.icon {
		color: var(--texte-doux);
	}
	.icon.added {
		color: var(--emeraude-texte);
	}
	.icon.modified {
		color: var(--ambre-texte);
	}
	.icon.deleted {
		color: var(--rouge-texte);
	}
	.path {
		font-size: var(--t-s);
		overflow-wrap: break-word;
		min-width: 0;
	}
	a.path {
		color: var(--texte);
		text-decoration: none;
	}
	a.path:hover {
		color: var(--emeraude-texte);
		text-decoration: underline;
	}
	s.path,
	a.path.deleted {
		color: var(--texte-doux);
	}
	a.path.deleted {
		text-decoration: line-through;
	}
	a.path[aria-current] {
		color: var(--emeraude-texte);
		font-weight: 600;
	}
	/* A narrow panel: the badge keeps its letter, the word stays for screen
	   readers and in the tooltip. */
	.changes {
		container-type: inline-size;
	}
	.short {
		display: none;
	}
	@container (max-width: 22rem) {
		li .badge .long {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip-path: inset(50%);
			white-space: nowrap;
		}
		li .badge .short {
			display: inline;
		}
	}
</style>
