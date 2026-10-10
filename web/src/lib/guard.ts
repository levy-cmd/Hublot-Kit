/** Token for stale-response guards: only the last begun, non-cancelled request is current. */
export function latest() {
	let n = 0;
	return {
		begin: () => ++n,
		isCurrent: (token: number) => token === n,
		/** Navigation: every request in flight becomes stale. */
		cancel: () => void n++
	};
}

/** Mutex for a write: busy is set synchronously, so a double click starts one write. */
export function lock() {
	let busy = false;
	return {
		busy: () => busy,
		async run<T>(fn: () => Promise<T>): Promise<T | undefined> {
			if (busy) return undefined;
			busy = true;
			try {
				return await fn();
			} finally {
				busy = false;
			}
		}
	};
}
