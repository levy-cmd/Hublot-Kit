<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';
	import { withParams } from '#lib/url.ts';
	import ContainerDetail, { type DetailTab } from '#lib/components/ContainerDetail.svelte';

	// The full page of a container (spec v0.2 §3): kept for links and for the
	// phone, with the tabs of the right panel and the bottom panels.
	let id = $derived(page.params.id ?? '');
	const readOnly = getContext<() => boolean>('readOnly');
	const TABS: DetailTab[] = ['overview', 'logs', 'diff', 'files', 'secrets'];
	// v0.1 links: ?tab=changes is the diff, ?tab=xray the overview.
	const OLD: Record<string, DetailTab> = { changes: 'diff', xray: 'overview' };
	let tab = $derived.by<DetailTab>(() => {
		const q = page.url.searchParams.get('tab') ?? '';
		return TABS.find((x) => x === q) ?? OLD[q] ?? (page.url.searchParams.has('path') ? 'files' : 'overview');
	});
	const tabHref = (k: DetailTab) => `?${new URLSearchParams(k === 'overview' ? {} : { tab: k })}`;
	const fileHref = (path: string) => withParams(new URLSearchParams(), { tab: 'files', path });
</script>

<div class="page">
	<a class="back" href="/"><Icon name="arrowLeft" /> {t('container.back')}</a>
	<div class="panel card">
		{#key id}
			<ContainerDetail {id} mode="page" {tab} readOnly={readOnly()} {tabHref} {fileHref} />
		{/key}
	</div>
</div>

<style>
	.page {
		display: grid;
		gap: 0.75rem;
		min-width: 0;
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		justify-self: start;
		color: var(--texte-doux);
		text-decoration: none;
		font-weight: 550;
	}
	.back:hover {
		color: var(--texte);
	}
	.card {
		padding: 1.25rem;
	}
	@media (max-width: 48rem) {
		.card {
			padding: 1rem 0.75rem;
		}
	}
</style>
