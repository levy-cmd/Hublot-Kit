// Palette provider: the images of the list the browser already has (spec
// v0.2 §5.8), by tag and short ID. Names only: never a configuration value.
import { imageList } from '../imageList.svelte.ts';
import { formatSize } from '../files.ts';
import { imageHref, imageName, imageShortId } from '../images.ts';
import type { PaletteItem } from '../palette.ts';

export default function images(): PaletteItem[] {
	imageList.ensure();
	return (imageList.items ?? []).map((img) => ({
		kind: 'image',
		label: imageName(img),
		detail: [img.tags.length > 1 ? img.tags.slice(1).join(', ') : '', imageShortId(img.id), formatSize(img.size)]
			.filter(Boolean)
			.join(' · '),
		href: imageHref(img.id),
		terms: [...img.tags, imageShortId(img.id)]
	}));
}
