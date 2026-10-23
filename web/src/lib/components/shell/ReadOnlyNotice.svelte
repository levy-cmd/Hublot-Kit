<script lang="ts">
	import { t } from '#lib/i18n/index.ts';
	import Icon from '#lib/icons/Icon.svelte';

	// The real state of the agent (spec v0.2, Q56): read-only is not the
	// default, and the notice never speaks of the host.
	let { readOnly }: { readOnly: boolean } = $props();
</script>

<aside class="notice" class:on={readOnly}>
	<Icon name="shield" size={20} filled={readOnly} />
	<div>
		<p class="title">{readOnly ? t('shell.notice.readOnly') : t('shell.notice.writable')}</p>
		<p class="hint">{readOnly ? t('shell.notice.readOnlyHint') : t('shell.notice.writableHint')}</p>
	</div>
</aside>

<style>
	.notice {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
		padding: 0.75rem;
		border: 1px solid var(--trait);
		border-radius: var(--rayon);
		background: var(--releve);
		color: var(--texte-doux);
	}
	.notice.on {
		background: var(--selection);
		border-color: var(--selection);
		color: var(--emeraude-texte);
	}
	.title {
		color: var(--texte);
		font-weight: 600;
		font-size: var(--t-s);
		line-height: 1.35;
	}
	.hint {
		font-size: var(--t-xs);
		color: var(--texte-doux);
		margin-top: 0.25rem;
	}
</style>
