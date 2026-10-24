// The Images pages (spec v0.2 §5.4): the list of the daemon's images and
// the detail of one, by its full ID only. The agent masks the configuration
// and the history (secrets.Env, secrets.Args, secrets.Labels,
// secrets.History); there is no reveal here: the reveal of a variable stays
// that of a container.
import { api, ApiError, type EnvVar, type Label } from './api.ts';
import { t, tf } from './i18n/index.ts';

export type ImageContainer = { id: string; name: string; state: string };

/** One image of GET /api/images. */
export type ImageRow = {
	id: string; // "sha256:<64 hex>"
	tags: string[]; // [] for an untagged image
	size: number;
	created: string;
	containers: ImageContainer[];
};

export type ImageLayer = { createdBy: string; created: string; size: number; empty: boolean };

/** GET /api/images/{id}. */
export type ImageDetail = {
	id: string;
	tags: string[];
	size: number;
	created: string;
	os: string;
	architecture: string;
	variant: string;
	config: {
		env: EnvVar[];
		cmd: string[];
		entrypoint: string[];
		commandMasked: boolean;
		workingDir: string;
		user: string;
		exposedPorts: string[];
		labels: Label[];
	};
	layers: ImageLayer[];
	containers: ImageContainer[];
};

const FULL_ID = /^(?:sha256:)?([0-9a-f]{64})$/;

/**
 * The 64 hex characters of a full image ID ("sha256:" or not), else null:
 * the agent answers only to a full ID (never a name, which may change
 * target, nor a short ID), and so does the page.
 */
export function imageHex(id: string): string | null {
	return FULL_ID.exec(id)?.[1] ?? null;
}

export const getImages = (signal?: AbortSignal) => api<ImageRow[]>('/images', { signal });

/** The detail of a full ID; a name or a short ID is never sent. */
export function getImage(id: string, signal?: AbortSignal): Promise<ImageDetail> {
	const hex = imageHex(id);
	if (!hex) return Promise.reject(new ApiError(404, 'not_found'));
	return api<ImageDetail>(`/images/${hex}`, { signal });
}

/** The link of an image's page, by its full ID. */
export const imageHref = (id: string): string => `/images/${imageHex(id) ?? ''}`;

/** The 12 first hex characters, as Docker shows an ID. */
export const imageShortId = (id: string): string => id.replace(/^sha256:/, '').slice(0, 12);

/** The name of an image: its first tag, or its short ID when untagged. */
export const imageName = (img: { id: string; tags: string[] }): string => img.tags[0] ?? imageShortId(img.id);

export type ImageSort = 'created' | 'size' | 'name';
export const IMAGE_SORTS: ImageSort[] = ['created', 'size', 'name'];

/** A copy sorted by date or size (largest, newest first) or by name; then by ID. */
export function sortImages(rows: ImageRow[], by: ImageSort): ImageRow[] {
	const time = (r: ImageRow) => Date.parse(r.created) || 0;
	return [...rows].sort((a, b) => {
		const d =
			by === 'size' ? b.size - a.size : by === 'name' ? imageName(a).localeCompare(imageName(b)) : time(b) - time(a);
		return d || a.id.localeCompare(b.id);
	});
}

/** The rows whose tags, short ID or containers' names contain q (case ignored). */
export function filterImages(rows: ImageRow[], q: string): ImageRow[] {
	const s = q.trim().toLowerCase();
	if (!s) return rows;
	return rows.filter(
		(r) =>
			r.tags.some((tag) => tag.toLowerCase().includes(s)) ||
			imageShortId(r.id).startsWith(s) ||
			r.containers.some((c) => c.name.toLowerCase().includes(s))
	);
}

/**
 * "Layers" are the entries of the history (spec v0.2 §5.4): the real ones
 * are those that are not empty; the empty ones (ENV, CMD, LABEL…) are
 * folded by the page.
 */
export function layerCounts(layers: ImageLayer[]): { real: number; empty: number } {
	const real = layers.filter((l) => !l.empty).length;
	return { real, empty: layers.length - real };
}

/** "linux/arm64/v8", what Docker calls the platform. */
export const platform = (d: Pick<ImageDetail, 'os' | 'architecture' | 'variant'>): string =>
	[d.os, d.architecture, d.variant].filter(Boolean).join('/');

/** "2 containers", "no container". */
export function containersText(n: number): string {
	if (n === 0) return t('images.containers.none');
	return n === 1 ? t('images.containers.one') : tf('images.containers.other', { n });
}
