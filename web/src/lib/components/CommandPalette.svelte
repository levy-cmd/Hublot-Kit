<script lang="ts">
	// The global search (spec v0.2 §5.8). Ctrl+K (Cmd+K on macOS), the field
	// of the top bar or the side bar entry open it; Escape closes it and the
	// focus goes back where it was. The query lives here only: never stored,
	// never in the page's URL; files are asked of the agent for the container
	// open only, in its index already built (Q61).
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError } from '#lib/api.ts';
	import { containerList } from '#lib/containerList.svelte.ts';
	import { errorMessage, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import type { IconName } from '#lib/icons/icons.ts';
	import {
		fileItems, isPaletteKey, MAX_QUERY, PALETTE_EVENT, rank, sections, searchFiles, validQuery,
		type FileSearch, type PaletteItem, type PaletteKind
	} from '#lib/palette.ts';
	import { paletteItems } from '#lib/palette/index.ts';

	const MAX_SHOWN = 50; // local results shown; the agent caps the files at 200
	const DEBOUNCE = 200; // ms before a file search

	let open = $state(false);
	let q = $state('');
	let active = $state(0);
	let dialog = $state<HTMLDialogElement>();
	let input = $state<HTMLInputElement>();
	let list = $state<HTMLUListElement>();

	// The container open: ?c= on the list, or the full page /c/<id>.
	let fullPage = $derived(page.url.pathname.startsWith('/c/'));
	let chosen = $derived(page.url.searchParams.get('c') ?? (fullPage ? (page.params.id ?? null) : null));
	let chosenName = $derived(
		containerList.items?.find((c) => c.id === chosen || c.name === chosen)?.name ?? chosen?.slice(0, 12) ?? ''
	);

	let files = $state<FileSearch | null>(null);
	let filesBusy = $state(false);
	let filesError = $state('');

	let local = $derived(open ? rank(paletteItems(), q).slice(0, MAX_SHOWN) : []);
	let fromFiles = $derived(files && chosen ? fileItems(files, chosen, fullPage) : []);
	let groups = $derived(sections([...local, ...fromFiles.filter((i) => i.kind !== 'file')]));
	let fileGroup = $derived(fromFiles.filter((i) => i.kind === 'file'));
	// One flat list for the keyboard, in the order shown.
	let flat = $derived([...groups.flatMap((g) => g.items), ...fileGroup]);

	// The option chosen by the keyboard, kept inside the list as it changes.
	let sel = $derived(flat.length ? Math.min(active, flat.length - 1) : -1);

	$effect(() => {
		window.addEventListener(PALETTE_EVENT, show);
		return () => window.removeEventListener(PALETTE_EVENT, show);
	});

	// Files: one request at a time, after a pause in typing; the previous
	// one is cancelled. Nothing is asked without a container open.
	$effect(() => {
		const query = q;
		const id = chosen;
		files = null;
		filesError = '';
		filesBusy = false;
		if (!open || !id || !validQuery(query)) return;
		const ctl = new AbortController();
		const timer = setTimeout(() => {
			filesBusy = true;
			searchFiles(id, query, ctl.signal)
				.then((res) => {
					if (!ctl.signal.aborted) files = res;
				})
				.catch((e) => {
					if (!ctl.signal.aborted) filesError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
				})
				.finally(() => {
					if (!ctl.signal.aborted) filesBusy = false;
				});
		}, DEBOUNCE);
		return () => {
			clearTimeout(timer);
			ctl.abort();
		};
	});

	// Shown while open: showModal makes the rest of the page inert; the
	// focus goes back to the element that had it once the dialog is gone.
	$effect(() => {
		if (!open) return;
		const d = dialog;
		const opener = document.activeElement;
		d?.showModal();
		input?.focus();
		return () => {
			if (d?.open) d.close();
			if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
		};
	});

	function show() {
		if (open) {
			input?.focus();
			return;
		}
		q = '';
		active = 0;
		open = true;
	}

	function hide() {
		open = false;
		q = '';
		files = null;
	}

	function choose(item: PaletteItem | undefined) {
		if (!item) return;
		hide();
		goto(item.href).catch(() => {});
	}

	function scrollActive() {
		queueMicrotask(() => list?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }));
	}

	// The focus stays in the palette: the field and the close button.
	function trapTab(e: KeyboardEvent) {
		if (e.key !== 'Tab') return;
		const stops = [...(dialog?.querySelectorAll<HTMLElement>('input, button') ?? [])];
		const i = stops.indexOf(document.activeElement as HTMLElement);
		e.preventDefault();
		stops[(i + (e.shiftKey ? -1 : 1) + stops.length) % stops.length]?.focus();
	}

	function onkeydown(e: KeyboardEvent) {
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				if (flat.length) active = (sel + 1) % flat.length;
				scrollActive();
				break;
			case 'ArrowUp':
				e.preventDefault();
				if (flat.length) active = (sel - 1 + flat.length) % flat.length;
				scrollActive();
				break;
			case 'Home':
			case 'End':
				if (!flat.length || !(e.ctrlKey || e.metaKey)) break;
				e.preventDefault();
				active = e.key === 'Home' ? 0 : flat.length - 1;
				scrollActive();
				break;
			case 'Enter':
				e.preventDefault();
				choose(flat[sel]);
				break;
			case 'Tab':
				trapTab(e);
				break;
		}
	}

	const ICON: Record<PaletteKind, IconName> = {
		container: 'container', image: 'image', project: 'folder', page: 'chevronRight', file: 'file'
	};
	// A file hit without file= is a directory.
	const iconOf = (it: PaletteItem): IconName => (it.kind === 'file' && !it.href.includes('file=') ? 'folder' : ICON[it.kind]);
	const optionId = (i: number) => `palette-opt-${i}`;
	const indexOf = (it: PaletteItem) => flat.indexOf(it);
</script>

<svelte:window
	onkeydown={(e) => {
		if (isPaletteKey(e)) {
			e.preventDefault(); // Firefox puts its own search there
			show();
		}
	}}
/>

{#if open}
	<dialog
		bind:this={dialog}
		class="palette"
		aria-labelledby="palette-title"
		oncancel={(e) => {
			e.preventDefault(); // Escape
			hide();
		}}
		onclick={(e) => {
			if (e.target === dialog) hide(); // a click on the backdrop
		}}
	>
		<h2 id="palette-title" class="sr-only">{t('palette.title')}</h2>
		<div class="field">
			<Icon name="search" size={20} />
			<input
				bind:this={input}
				bind:value={q}
				oninput={() => (active = 0)}
				{onkeydown}
				type="text"
				role="combobox"
				aria-label={t('palette.input')}
				aria-expanded={flat.length > 0}
				aria-controls="palette-list"
				aria-autocomplete="list"
				aria-activedescendant={sel >= 0 ? optionId(sel) : undefined}
				aria-describedby="palette-help"
				placeholder={t('palette.field')}
				maxlength={MAX_QUERY}
				autocomplete="off"
				autocapitalize="off"
				spellcheck="false"
			/>
			<button type="button" class="icon-btn" aria-label={t('palette.close')} title={t('palette.close')} onclick={hide} onkeydown={trapTab}>
				<Icon name="close" size={16} />
			</button>
		</div>

		<ul id="palette-list" role="listbox" aria-label={t('palette.title')} bind:this={list}>
			{#each groups as g (g.kind)}
				<li role="presentation" class="section">{t(`palette.section.${g.kind}`)}</li>
				{#each g.items as it (it.kind + it.href)}
					{@render option(it)}
				{/each}
			{/each}
			{#if fileGroup.length}
				<li role="presentation" class="section">{tf('palette.section.file', { name: chosenName })}</li>
				{#each fileGroup as it (it.href)}
					{@render option(it)}
				{/each}
			{/if}
		</ul>

		<div class="foot">
			<p class="status" role="status">
				{#if !validQuery(q)}
					{t('palette.type')}
				{:else if filesBusy}
					{t('palette.searching')}
				{:else if filesError}
					{filesError}
				{:else if flat.length === 0}
					{t('palette.none')}
				{:else if files?.truncated}
					{tf('palette.truncated', { n: files.results.length })}
				{:else}
					{tf('palette.count', { n: flat.length })}
				{/if}
				{#if validQuery(q) && !chosen}
					<span class="note">{t('palette.noContainer')}</span>
				{/if}
			</p>
			<p id="palette-help" class="help">{t('palette.help')}</p>
		</div>
	</dialog>
{/if}

{#snippet option(it: PaletteItem)}
	{@const i = indexOf(it)}
	<!-- The keyboard stays in the field (combobox, aria-activedescendant): the
	     options take the mouse only. -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<li
		id={optionId(i)}
		role="option"
		tabindex="-1"
		aria-selected={i === sel}
		class:active={i === sel}
		onclick={() => choose(it)}
		onmousemove={() => (active = i)}
	>
		<Icon name={iconOf(it)} size={16} />
		<span class="label" class:mono={it.kind === 'file'}>{it.label}</span>
		{#if it.detail}<span class="detail">{it.detail}</span>{/if}
	</li>
{/snippet}

<style>
	.palette {
		width: min(40rem, calc(100vw - 2rem));
		max-height: min(34rem, calc(100dvh - 6rem));
		margin: 4.5rem auto auto;
		padding: 0;
		display: grid;
		grid-template-rows: auto minmax(0, 1fr) auto;
		overflow: hidden;
	}
	.palette:not([open]) {
		display: none;
	}
	.field {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.6rem 0.6rem 0.6rem 1rem;
		border-bottom: 1px solid var(--trait);
		color: var(--texte-doux);
	}
	input {
		flex: 1;
		min-width: 0;
		border: 0;
		background: transparent;
		color: var(--texte);
		font: inherit;
		font-size: var(--t-l);
		padding: 0.4rem 0;
		outline: none;
	}
	.field:focus-within {
		box-shadow: inset 0 -2px 0 var(--focus);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0.25rem 0.5rem 0.5rem;
		overflow-y: auto;
	}
	.section {
		padding: 0.6rem 0.5rem 0.3rem;
		font-size: var(--t-xs);
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--texte-doux);
	}
	[role='option'] {
		display: flex;
		align-items: center;
		gap: 0.65rem;
		padding: 0.5rem 0.6rem;
		border-radius: var(--rayon);
		border-left: 3px solid transparent;
		cursor: pointer;
		color: var(--texte-doux);
		min-width: 0;
	}
	[role='option'].active {
		background: var(--selection);
		border-left-color: var(--emeraude);
		color: var(--texte);
	}
	.label {
		color: var(--texte);
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.label.mono {
		font-size: var(--t-s);
	}
	.detail {
		margin-left: auto;
		font-size: var(--t-s);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
		max-width: 50%;
	}
	.foot {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		gap: 0.25rem 1rem;
		padding: 0.55rem 1rem;
		border-top: 1px solid var(--trait);
		font-size: var(--t-xs);
		color: var(--texte-doux);
	}
	.foot p {
		margin: 0;
	}
	.note {
		margin-left: 0.5rem;
	}
	@media (max-width: 48rem) {
		.palette {
			margin-top: 1rem;
		}
		.help {
			display: none;
		}
	}
</style>
