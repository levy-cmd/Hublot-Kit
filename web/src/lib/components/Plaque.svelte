<script lang="ts">
	// The plate: the container's marking, painted with a stencil on its door
	// (ISO 6346 style). The only place where capitals are allowed.
	import { plaqueId, stateLabel, stateTone } from '#lib/containers.ts';
	import { t } from '#lib/i18n/index.ts';

	let { id, name, image, state }: { id: string; name: string; image: string; state: string } = $props();
	let mark = $derived(plaqueId(id));
</script>

<section class="plaque {stateTone(state)}" aria-labelledby="plaque-nom">
	<p class="ident" title={id}>
		<span class="visuellement-cache">{t('container.id')} </span>{mark.serial}<span class="cle">{mark.check}</span>
	</p>
	<h1 id="plaque-nom">{name}</h1>
	<dl>
		<div class="image">
			<dt>{t('containers.col.image')}</dt>
			<dd>{image}</dd>
		</div>
		<div class="etat">
			<dt>{t('containers.col.state')}</dt>
			<dd><span class="pastille">{stateLabel(state)}</span></dd>
		</div>
	</dl>
</section>

<style>
	.plaque {
		background: var(--porte);
		color: var(--porte-texte);
		border-radius: var(--rayon);
		padding: var(--e5) var(--e6) 0;
		font-stretch: 62%;
		font-weight: 800;
		text-transform: uppercase;
	}
	.ok {
		--ton: var(--vert);
		--sur-ton: var(--pochoir-ok);
	}
	.warn {
		--ton: var(--ambre);
		--sur-ton: var(--pochoir-warn);
	}
	.bad {
		--ton: var(--rouille);
		--sur-ton: var(--pochoir-bad);
	}
	.idle {
		--ton: var(--acier);
		--sur-ton: var(--pochoir-idle);
	}
	.ident {
		margin: 0 0 var(--e2);
		font-size: 1.5rem;
		font-stretch: 75%;
		font-weight: 700;
		letter-spacing: 0.08em;
		line-height: 1;
		color: var(--porte-texte);
	}
	/* le dernier caractère encadré, comme le chiffre de contrôle ISO */
	.cle {
		display: inline-block;
		margin-left: 0.3em;
		padding: 0.1em 0.18em 0.06em 0.26em;
		border: 2px solid currentColor;
		border-radius: var(--rayon);
	}
	h1 {
		margin: 0;
		font-size: clamp(2.25rem, 4vw + 1rem, 4.25rem);
		font-stretch: 62%;
		font-weight: 800;
		line-height: 0.95;
		letter-spacing: 0.015em;
		overflow-wrap: anywhere;
	}
	dl {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		margin: var(--e5) calc(-1 * var(--e6)) 0;
		border-top: 1px solid var(--porte-trait);
	}
	dl > div {
		padding: var(--e3) var(--e6) var(--e4);
		min-width: 0;
	}
	dl > div + div {
		border-left: 1px solid var(--porte-trait);
	}
	dt {
		font-size: var(--t-xs);
		font-weight: 600;
		letter-spacing: 0.02em;
		color: var(--porte-doux);
		margin-bottom: var(--e1);
	}
	dd {
		margin: 0;
		font-size: var(--t-l);
		line-height: 1.2;
		overflow-wrap: anywhere;
	}
	.pastille {
		display: inline-block;
		padding: 0.05em 0.4em 0;
		background: var(--ton);
		color: var(--sur-ton);
		border-radius: var(--rayon);
		white-space: nowrap;
	}

	@media (max-width: 40rem) {
		.plaque {
			padding: var(--e4) var(--e4) 0;
		}
		.ident {
			font-size: 1.125rem;
		}
		dl {
			grid-template-columns: minmax(0, 1fr);
			margin: var(--e4) calc(-1 * var(--e4)) 0;
		}
		dl > div {
			padding: var(--e2) var(--e4) var(--e3);
		}
		dl > div + div {
			border-left: 0;
			border-top: 1px solid var(--porte-trait);
		}
	}
</style>
