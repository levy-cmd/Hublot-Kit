<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { stateTone } from '#lib/containers.ts';
	import { formatSize } from '#lib/files.ts';
	import { errorMessage, lang, t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { imageList } from '#lib/imageList.svelte.ts';
	import {
		containersText, filterImages, IMAGE_SORTS, imageHref, imageName, imageShortId, sortImages, type ImageSort
	} from '#lib/images.ts';
	import { withParams } from '#lib/url.ts';

	// Spec v0.2 §5.4: the images of the daemon, untagged ones included (by
	// their short ID), each opening by its full ID. The sort lives in the URL;
	// the filter does not (a name can be sensitive, as on the Containers page).
	let query = $state('');
	let params = $derived(page.url.searchParams);
	let sort = $derived.by((): ImageSort => {
		const s = params.get('sort') as ImageSort;
		return IMAGE_SORTS.includes(s) ? s : 'created';
	});
	let all = $derived(imageList.items ?? []);
	let shown = $derived(sortImages(filterImages(all, query), sort));
	let error = $derived(imageList.error ? errorMessage(imageList.error.code) : '');

	onMount(() => {
		void imageList.load();
	});
	$effect(() => {
		if (imageList.error?.status === 401) goto('/login').catch(() => {});
	});

	function pickSort(e: Event & { currentTarget: HTMLSelectElement }) {
		const v = e.currentTarget.value;
		goto(withParams(params, { sort: v === 'created' ? null : v }), { reset: false }).catch(() => {});
	}

	function when(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(lang, { dateStyle: 'medium' });
	}
</script>

<div class="page">
	<section class="panel images" aria-labelledby="images-title">
		<header class="head">
			<div>
				<h1 id="images-title">{t('images.title')}</h1>
				<p class="sub">{t('images.subtitle')}</p>
			</div>
			<div class="head-right">
				{#if imageList.items}
					<span class="total">{tf(all.length === 1 ? 'images.total.one' : 'images.total.other', { n: all.length })}</span>
				{/if}
				<button
					type="button"
					class="icon-btn"
					onclick={() => imageList.load()}
					disabled={imageList.loading}
					aria-label={t('images.refresh')}
					title={t('images.refresh')}
				>
					<Icon name="refresh" size={20} />
				</button>
			</div>
		</header>

		<div class="filters">
			<label class="search">
				<span class="sr-only">{t('images.search')}</span>
				<Icon name="search" />
				<input type="search" bind:value={query} placeholder={t('images.search')} />
			</label>
			<label class="sort">
				<span>{t('images.sort')}</span>
				<select value={sort} onchange={pickSort}>
					{#each IMAGE_SORTS as s (s)}
						<option value={s}>{t(`images.sort.${s}`)}</option>
					{/each}
				</select>
			</label>
		</div>

		{#if error}
			<p role="alert">{error}</p>
			<button type="button" class="btn small" onclick={() => imageList.load()}>{t('files.reload')}</button>
		{:else if !imageList.items}
			<p class="muted" aria-live="polite">{t('images.loading')}</p>
		{:else if all.length === 0}
			<p class="muted">{t('images.empty')}</p>
		{:else if shown.length === 0}
			<p class="muted">{t('images.noMatch')}</p>
		{:else}
			<div class="scroll">
				<table>
					<thead>
						<tr>
							<th scope="col" class="c-name">{t('images.col.name')}</th>
							<th scope="col" class="c-id">{t('images.col.id')}</th>
							<th scope="col" class="c-size">{t('images.col.size')}</th>
							<th scope="col" class="c-created">{t('images.col.created')}</th>
							<th scope="col" class="c-ctrs">{t('images.col.containers')}</th>
						</tr>
					</thead>
					<tbody>
						{#each shown as img (img.id)}
							<tr>
								<td class="c-name">
									<span class="cell">
										<span class="tile" aria-hidden="true"><Icon name="image" /></span>
										<span class="names">
											<a class="name mono" href={imageHref(img.id)} title={img.tags.join('\n') || img.id}>{imageName(img)}</a>
											{#if img.tags.length === 0}
												<span class="pill">{t('images.untagged')}</span>
											{:else if img.tags.length > 1}
												<span class="more mono">{img.tags.slice(1).join(', ')}</span>
											{/if}
										</span>
									</span>
								</td>
								<td class="c-id"><span class="mono">{imageShortId(img.id)}</span></td>
								<td class="c-size">{formatSize(img.size)}</td>
								<td class="c-created">{when(img.created)}</td>
								<td class="c-ctrs">
									{#if img.containers.length === 0}
										<span class="muted">{containersText(0)}</span>
									{:else}
										<span class="ctrs">
											{#each img.containers.slice(0, 3) as c (c.id)}
												<a class="ctr" href="/?c={c.id}"><span class="dot {stateTone(c.state)}"></span>{c.name}</a>
											{/each}
											{#if img.containers.length > 3}
												<span class="muted">+{img.containers.length - 3}</span>
											{/if}
										</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>
</div>

<style>
	.page {
		display: grid;
		gap: 1rem;
		min-width: 0;
	}
	.images {
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
	.head-right {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	.total {
		color: var(--texte-doux);
		font-size: var(--t-s);
		white-space: nowrap;
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
	.search {
		flex: 1 1 18rem;
		position: relative;
		display: flex;
		align-items: center;
		color: var(--texte-doux);
	}
	.search :global(.icon) {
		position: absolute;
		left: 0.65rem;
		pointer-events: none;
	}
	.search input {
		width: 100%;
		padding-left: 2.1rem;
	}
	.sort {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-size: var(--t-s);
		color: var(--texte-doux);
	}
	.scroll {
		overflow-x: auto;
	}
	table {
		width: 100%;
		border-collapse: separate;
		border-spacing: 0;
		table-layout: fixed;
	}
	th {
		text-align: left;
		font-size: var(--t-s);
		font-weight: 600;
		color: var(--texte-doux);
		padding: 0.5rem 0.6rem;
		background: var(--releve);
		border-bottom: 1px solid var(--trait);
		white-space: nowrap;
	}
	th:first-child {
		border-top-left-radius: var(--rayon);
	}
	th:last-child {
		border-top-right-radius: var(--rayon);
	}
	th.c-name {
		width: 38%;
	}
	th.c-id {
		width: 9rem;
	}
	th.c-size {
		width: 7rem;
	}
	th.c-created {
		width: 9rem;
	}
	td {
		padding: 0.5rem 0.6rem;
		border-bottom: 1px solid var(--trait);
		vertical-align: middle;
		overflow: hidden;
	}
	tbody tr {
		position: relative;
		transition: background-color var(--vite);
	}
	tbody tr:hover {
		background: var(--releve);
	}
	.cell {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-width: 0;
	}
	.tile {
		display: inline-grid;
		place-items: center;
		flex: none;
		width: 1.75rem;
		height: 1.75rem;
		border-radius: var(--rayon);
		background: var(--releve);
		border: 1px solid var(--trait);
	}
	.names {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.15rem 0.5rem;
		min-width: 0;
	}
	.name {
		color: var(--texte);
		font-weight: 600;
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
	}
	.name:hover {
		text-decoration: underline;
	}
	/* The whole row opens the image: the name link covers it. */
	.name::after {
		content: '';
		position: absolute;
		inset: 0;
	}
	.more {
		color: var(--texte-doux);
		font-size: var(--t-xs);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
	}
	.c-id,
	.c-size,
	.c-created {
		color: var(--texte-doux);
		font-size: var(--t-s);
		white-space: nowrap;
	}
	.ctrs {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.2rem 0.6rem;
		font-size: var(--t-s);
	}
	.ctr {
		position: relative; /* above the row link */
		z-index: 1;
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		color: var(--texte);
	}
	@media (max-width: 48rem) {
		.images {
			padding: 1rem 0.75rem;
		}
		h1 {
			font-size: var(--t-xl);
		}
		thead {
			display: none;
		}
		table,
		tbody {
			display: block;
		}
		tbody tr {
			display: grid;
			grid-template-columns: minmax(0, 1fr) auto;
			gap: 0.15rem 0.75rem;
			padding: 0.6rem 0.5rem;
			border-bottom: 1px solid var(--trait);
		}
		td {
			display: block;
			padding: 0;
			border: 0;
			min-width: 0;
		}
		.c-name {
			grid-column: 1 / -1;
		}
		.c-id {
			grid-column: 1;
			padding-left: 2.35rem;
		}
		.c-size {
			grid-column: 2;
			grid-row: 2;
			text-align: right;
		}
		.c-created {
			grid-column: 1;
			padding-left: 2.35rem;
		}
		.c-ctrs {
			grid-column: 1 / -1;
			padding-left: 2.35rem;
		}
	}
</style>
