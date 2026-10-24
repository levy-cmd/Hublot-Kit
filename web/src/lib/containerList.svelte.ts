// The container list, shared by the Containers page and the counter of the
// side bar: one GET /api/containers for both.
import { ApiError, listContainers, type Container } from './api.ts';

class ContainerList {
	items = $state<Container[] | null>(null);
	error = $state<ApiError | null>(null);
	loading = $state(false);
	#asked = 0;

	/** Loads the list again; a late answer of an older call is dropped. */
	async load(): Promise<void> {
		const n = ++this.#asked;
		this.loading = true;
		try {
			const list = await listContainers();
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

	/** Forgets everything (logout): another session starts empty. */
	clear(): void {
		this.#asked++;
		this.items = null;
		this.error = null;
		this.loading = false;
	}
}

export const containerList = new ContainerList();
