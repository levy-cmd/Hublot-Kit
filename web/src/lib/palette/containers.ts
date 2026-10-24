// Palette provider: the containers and their Compose projects, from the list
// the browser already has (spec v0.2 §5.8). Names only: never an
// environment value, a command or a label value.
import { containerList } from '../containerList.svelte.ts';
import { projectsOf } from '../containers.ts';
import { tf } from '../i18n/index.ts';
import type { PaletteItem } from '../palette.ts';

export default function containers(): PaletteItem[] {
	const list = containerList.items ?? [];
	const out: PaletteItem[] = list.map((c) => ({
		kind: 'container',
		label: c.name,
		detail: [c.image, c.project && c.service ? `${c.project}/${c.service}` : c.project].filter(Boolean).join(' · '),
		href: `/?c=${encodeURIComponent(c.id)}`,
		terms: [c.id.slice(0, 12), c.image, c.project ?? '', c.service ?? ''].filter(Boolean)
	}));
	for (const p of projectsOf(list)) {
		const n = list.filter((c) => c.project === p).length;
		out.push({ kind: 'project', label: p, detail: tf('palette.projectCount', { n }), href: `/?project=${encodeURIComponent(p)}` });
	}
	return out;
}
