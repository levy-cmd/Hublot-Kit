<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { ApiError, listContainers, type Container } from '#lib/api.ts';
	import { filterContainers, formatPorts, rowStep, stateLabel, stateTone } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let all = $state<Container[]>([]);
	let query = $state('');
	let showAll = $state(false);
	let error = $state('');
	let loading = $state(false);
	let shown = $derived(filterContainers(all, query, showAll));
	let rows = $state<HTMLElement | null>(null);
	let search = $state<HTMLInputElement | null>(null);

	async function load() {
		loading = true;
		error = '';
		try {
			all = await listContainers();
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) return goto('/login');
			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		} finally {
			loading = false;
		}
	}

	onMount(load);

	// Arrow keys, Home and End move between the rows' links; Enter opens.
	function onRowsKey(event: KeyboardEvent) {
		if (!rows) return;
		const links = [...rows.querySelectorAll<HTMLAnchorElement>('a.nom')];
		const at = links.indexOf(document.activeElement as HTMLAnchorElement);
		const next = rowStep(at, event.key, links.length);
		if (next === null) return;
		event.preventDefault();
		links[next].focus();
	}

	// "/" jumps to the search field, as in most consoles.
	function onWindowKey(event: KeyboardEvent) {
		const target = event.target as HTMLElement | null;
		if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey) return;
		if (target?.closest('input, textarea, select, [contenteditable]')) return;
		event.preventDefault();
		search?.focus();
	}
</script>

<svelte:window onkeydown={onWindowKey} />

<main>
	<div class="entete">
		<h1>{t('containers.title')}</h1>
		<div class="outils">
			<input
				type="search"
				bind:this={search}
				aria-label={t('containers.search')}
				placeholder={t('containers.search')}
				bind:value={query}
			/>
			<label class="case"><input type="checkbox" bind:checked={showAll} /> {t('containers.showAll')}</label>
			<button type="button" onclick={load} disabled={loading}>{t('containers.refresh')}</button>
		</div>
	</div>

	{#if error}<p role="alert">{error}</p>{/if}

	{#if error}
		<!-- error shown above -->
	{:else if shown.length === 0 && loading}
		<p class="vide">{t('containers.loading')}</p>
	{:else if shown.length === 0}
		<p class="vide">{t('containers.empty')}</p>
	{:else}
		<div class="scroll">
			<table>
				<thead>
					<tr>
						<th scope="col">{t('containers.col.name')}</th>
						<th scope="col">{t('containers.col.image')}</th>
						<th scope="col">{t('containers.col.state')}</th>
						<th scope="col">{t('containers.col.status')}</th>
						<th scope="col">{t('containers.col.ports')}</th>
					</tr>
				</thead>
				<tbody bind:this={rows}>
					{#each shown as c (c.id)}
						<tr class={stateTone(c.state)}>
							<td class="c-nom"><a class="nom" href={`/c/${c.id}`} onkeydown={onRowsKey}>{c.name}</a></td>
							<td class="c-image mono">{c.image}</td>
							<td class="c-etat">{stateLabel(c.state)}</td>
							<td class="c-statut">{c.status}</td>
							<td class="c-ports mono">{formatPorts(c.ports)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</main>

<style>
	main {
		padding: var(--e5) var(--gouttiere) var(--e7);
	}
	.entete {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--e3) var(--e5);
		margin-bottom: var(--e5);
	}
	h1 {
		font-size: 2.25rem;
		font-stretch: 75%;
		font-weight: 800;
	}
	.outils {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--e3) var(--e4);
	}
	.outils input[type='search'] {
		width: 20rem;
		max-width: 100%;
	}
	.case {
		display: inline-flex;
		align-items: center;
		gap: var(--e2);
		font-size: var(--t-s);
		cursor: pointer;
	}
	.vide {
		color: var(--texte-doux);
	}

	.scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--t-s);
	}
	th {
		text-align: left;
		font-size: var(--t-xs);
		font-weight: 600;
		color: var(--texte-doux);
		padding: 0 var(--e3) var(--e2);
		border-bottom: 1px solid var(--encre);
		white-space: nowrap;
	}
	td {
		padding: 0 var(--e3);
		height: 2.375rem;
		border-bottom: 1px solid var(--trait-fin);
		white-space: nowrap;
	}
	.c-image,
	.c-ports {
		font-size: var(--t-xs);
	}
	.c-statut {
		color: var(--texte-doux);
	}

	/* l'état : texte et liseré à la couleur du quai */
	.ok {
		--ton: var(--vert);
		--ton-texte: var(--vert-texte);
	}
	.warn {
		--ton: var(--ambre);
		--ton-texte: var(--ambre-texte);
	}
	.bad {
		--ton: var(--rouille);
		--ton-texte: var(--rouille-texte);
	}
	.idle {
		--ton: var(--acier);
		--ton-texte: var(--acier-texte);
	}
	td.c-nom {
		border-left: 4px solid var(--ton);
		padding-left: var(--e3);
	}
	.c-etat {
		color: var(--ton-texte);
		font-weight: 600;
	}
	a.nom {
		font-weight: 600;
		text-decoration: none;
		white-space: normal;
		overflow-wrap: anywhere;
	}
	a.nom:hover {
		text-decoration: underline;
	}
	/* la ligne au focus clavier : bande jaune, marquage au sol */
	a.nom:focus-visible {
		outline: none;
		box-shadow: none;
	}
	tbody tr:hover {
		background: var(--survol);
	}
	tbody tr:has(a.nom:focus-visible) {
		background: var(--jaune);
		color: var(--sur-jaune);
	}
	tbody tr:has(a.nom:focus-visible) td {
		color: inherit;
		border-bottom-color: var(--sur-jaune);
	}
	tbody tr:has(a.nom:focus-visible) a.nom {
		text-decoration: underline;
		text-decoration-thickness: 2px;
	}

	@media (max-width: 40rem) {
		h1 {
			font-size: var(--t-xl);
		}
		.outils,
		.outils input[type='search'] {
			width: 100%;
		}
		thead {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0 0 0 0);
		}
		table,
		tbody {
			display: block;
		}
		tbody {
			border-top: 1px solid var(--encre);
		}
		tr {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			grid-template-areas:
				'nom etat'
				'image image'
				'statut statut'
				'ports ports';
			column-gap: var(--e3);
			padding: var(--e2) var(--e3) var(--e2) var(--e3);
			border-left: 4px solid var(--ton);
			border-bottom: 1px solid var(--trait-fin);
		}
		td {
			height: auto;
			padding: 0;
			border: 0;
			white-space: normal;
			overflow-wrap: anywhere;
		}
		td.c-nom {
			grid-area: nom;
			border: 0;
			padding: 0;
		}
		.c-etat {
			grid-area: etat;
		}
		.c-image {
			grid-area: image;
		}
		.c-statut {
			grid-area: statut;
		}
		.c-ports {
			grid-area: ports;
		}
		.c-ports:empty {
			display: none;
		}
	}
</style>
