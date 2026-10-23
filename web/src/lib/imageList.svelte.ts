// The image list, shared by the Images page, the counter of the side bar and
// the palette: one GET /api/images for all. The agent answers simultaneous
// lists with one walk of Docker; the page loads it again on each visit.
import { ApiError } from './api.ts';
import { getImages, type ImageRow } from './images.ts';

class ImageList {
	items = $state<ImageRow[] | null>(null);
	error = $state<ApiError | null>(null);
	loading = $state(false);
	#asked = 0;
	#tried = false;

	/** Loads the list again; a late answer of an older call is dropped. */
	async load(): Promise<void> {
		this.#tried = true;
		const n = ++this.#asked;
		this.loading = true;
		try {
			const list = await getImages();
			if (n !== this.#asked) return;
			this.items = list ?? [];
			this.error = null;
		} catch (e) {
			if (n !== this.#asked) return;
			this.error = e instanceof ApiError ? e : new ApiError(0, 'unknown');
		} finally {
			if (n === this.#asked) this.loading = false;
		}
	}

	/**
	 * Loads the list once, for a reader that only needs it present (the
	 * counter, the palette). Called while rendering: the load starts in a
	 * microtask, never inside the render.
	 */
	ensure(): void {
		if (this.#tried) return;
		this.#tried = true;
		queueMicrotask(() => void this.load());
	}
}

export const imageList = new ImageList();
