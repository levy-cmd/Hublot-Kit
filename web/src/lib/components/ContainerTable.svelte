<script lang="ts">
	import type { Container } from '#lib/api.ts';
	import { formatPorts, rowStep, stateLabel, stateTone } from '#lib/containers.ts';
	import { t, tf } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { exitCodeOf, parseStatus, statusText } from '#lib/status.ts';

	// The list of spec v0.2 §3. A row is chosen by its name link (or a click
	// anywhere on it); the arrows, Home and End move between rows, Enter
	// follows the link. hrefFor gives the link of a row and of its actions.
	// capped: the list scrolls in its own box, so that the detail and the
	// bottom panels stay in sight (the mock-up's layout).
	let {
		list,
		chosen,
		capped = false,
		hrefFor
	}: {
		list: Container[];
		chosen: string | null;
		capped?: boolean;
		hrefFor: (c: Container, panel?: 'files' | 'logs' | 'diff') => string;
	} = $props();

	let box = $state<HTMLDivElement>();
	let body = $state<HTMLTableSectionElement>();

	function onkeydown(e: KeyboardEvent, i: number) {
		const next = rowStep(i, e.key, list.length);
		if (next === null) return;
		e.preventDefault();
		body?.querySelectorAll<HTMLAnchorElement>('a.name')[next]?.focus();
	}

	// The chosen row stays visible in a capped list, without scrolling the page.
	$effect(() => {
		void chosen;
		void list.length;
		if (!capped || !box) return;
		const row = box.querySelector<HTMLElement>('tr.chosen');
		if (!row) return;
		const r = row.getBoundingClientRect();
		const b = box.getBoundingClientRect();
		const head = box.querySelector('thead')?.getBoundingClientRect().height ?? 0;
		if (r.top < b.top + head || r.bottom > b.bottom) box.scrollTop += r.top - b.top - box.clientHeight / 3;
	});

	const ACTIONS = [
		{ panel: 'files', icon: 'folder', key: 'containers.action.files' },
		{ panel: 'logs', icon: 'logs', key: 'containers.action.logs' },
		{ panel: 'diff', icon: 'diff', key: 'containers.action.diff' }
	] as const;
</script>

<div class="scroll" class:capped bind:this={box}>
	<table aria-describedby="table-keys">
		<thead>
			<tr>
				<th scope="col" class="c-name">{t('containers.col.name')}</th>
				<th scope="col" class="c-image">{t('containers.col.image')}</th>
				<th scope="col" class="c-state">{t('containers.col.status')}</th>
				<th scope="col" class="c-ports">{t('containers.col.ports')}</th>
				<th scope="col" class="c-uptime">{t('containers.col.uptime')}</th>
				<th scope="col" class="c-actions"><span class="sr-only">{t('containers.col.actions')}</span></th>
			</tr>
		</thead>
		<tbody bind:this={body}>
			{#each list as c, i (c.id)}
				{@const st = parseStatus(c.status)}
				{@const tone = stateTone(c.state, exitCodeOf(st))}
				{@const ports = formatPorts(c.ports)}
				<tr class:chosen={c.id === chosen}>
					<td class="c-name">
						<div class="cell">
							<span class="tile tone-{tone}"><Icon name="container" /></span>
							<a
								class="name"
								href={hrefFor(c)}
								title={c.name}
								aria-current={c.id === chosen ? 'true' : undefined}
								onkeydown={(e) => onkeydown(e, i)}>{c.name}</a
							>
						</div>
					</td>
					<td class="c-image"><span class="cut mono" title={c.image}>{c.image}</span></td>
					<td class="c-state">
						<span class="state tone-{tone}"><span class="dot {tone}"></span>{stateLabel(c.state)}</span>
					</td>
					<td class="c-ports"><span class="cut mono" title={ports}>{ports || '—'}</span></td>
					<td class="c-uptime"><span class="cut" title={c.status}>{statusText(st)}</span></td>
					<td class="c-actions">
						{#each ACTIONS as a (a.panel)}
							<a
								class="icon-btn small"
								href={hrefFor(c, a.panel)}
								aria-label={tf('containers.action.label', { action: t(a.key), name: c.name })}
								title={t(a.key)}><Icon name={a.icon} /></a
							>
						{/each}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
	<p id="table-keys" class="sr-only">{t('containers.keys')}</p>
</div>

<style>
	.scroll {
		overflow-x: auto;
	}
	.scroll.capped {
		max-height: min(30rem, 55vh);
		overflow-y: auto;
		border-bottom: 1px solid var(--trait);
	}
	table {
		width: 100%;
		border-collapse: separate;
		border-spacing: 0;
		table-layout: fixed;
	}
	th {
		position: sticky;
		top: 0;
		z-index: 2;
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
		width: 32.5%;
	}
	th.c-image {
		width: 10.5%;
	}
	th.c-state {
		width: 6.5rem;
	}
	th.c-ports {
		width: 14%;
	}
	th.c-uptime {
		width: 15.5%;
	}
	th.c-actions {
		width: 6.25rem;
	}
	td {
		padding: 0.5rem 0.6rem;
		border-bottom: 1px solid var(--trait);
		white-space: nowrap;
		vertical-align: middle;
		overflow: hidden;
	}
	tr {
		position: relative;
		transition: background-color var(--vite);
	}
	tbody tr:hover {
		background: var(--releve);
	}
	tr.chosen,
	tbody tr.chosen:hover {
		background: var(--selection);
	}
	tr.chosen td:first-child {
		box-shadow: inset 3px 0 0 var(--focus);
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
	.name {
		color: var(--texte);
		font-weight: 600;
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		min-width: 0;
	}
	.name:hover {
		text-decoration: underline;
	}
	/* The whole row chooses the container: the name link covers it. */
	.name::after {
		content: '';
		position: absolute;
		inset: 0;
	}
	.cut {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.c-image,
	.c-ports,
	.c-uptime {
		color: var(--texte-doux);
	}
	.state {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		font-weight: 550;
	}
	.c-actions {
		text-align: right;
		padding-left: 0;
	}
	.c-actions a {
		position: relative; /* above the row link */
		z-index: 1;
		display: inline-grid;
	}
	tr.chosen .c-actions a:hover,
	tbody tr:hover .c-actions a:hover {
		background: var(--surface);
	}

	/* The name first: a narrower face keeps more of it, the image is the one
	   cut (whole in the title and for screen readers). */
	.name {
		font-stretch: 87.5%;
	}
	.c-ports,
	.c-uptime {
		font-size: var(--t-s);
	}
	.c-uptime .cut {
		white-space: normal;
	}
	@media (max-width: 48rem) {
		.c-ports {
			display: none;
		}
	}
	/* Phone: a dense list, name and state first (spec v0.2 §3). */
	@media (max-width: 48rem) {
		.scroll.capped {
			max-height: none;
		}
		thead {
			display: none;
		}
		table,
		tbody,
		tr {
			display: block;
		}
		tr {
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
		.c-image {
			grid-column: 1;
			grid-row: 2;
			padding-left: 2.35rem;
			font-size: var(--t-s);
		}
		.c-state {
			grid-column: 2;
			grid-row: 1;
			text-align: right;
		}
		.c-uptime {
			grid-column: 2;
			grid-row: 2;
			text-align: right;
			font-size: var(--t-s);
		}
		.c-actions {
			display: none;
		}
	}
</style>
