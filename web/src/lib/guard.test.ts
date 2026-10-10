import { describe, expect, it } from 'vitest';
import { latest, lock } from './guard.ts';

describe('latest', () => {
	it('only the last started request is current', () => {
		const g = latest();
		const a = g.begin();
		expect(g.isCurrent(a)).toBe(true);
		const b = g.begin();
		expect(g.isCurrent(a)).toBe(false);
		expect(g.isCurrent(b)).toBe(true);
	});
	it('cancel makes the pending request stale, even if the same file is shown again (A, B, A)', () => {
		const g = latest();
		const a = g.begin();
		g.cancel(); // navigation to B
		g.cancel(); // and back to A: a new token still differs
		expect(g.isCurrent(a)).toBe(false);
	});
});

describe('lock', () => {
	it('ignores a second call made before the first one settles (double click)', async () => {
		const l = lock();
		let calls = 0;
		let release!: () => void;
		const first = l.run(() => {
			calls++;
			return new Promise<void>((ok) => (release = ok));
		});
		expect(l.busy()).toBe(true); // set synchronously, before any await
		await l.run(async () => {
			calls++;
		});
		release();
		await first;
		expect(calls).toBe(1);
		expect(l.busy()).toBe(false);
	});
	it('releases the lock when the function throws', async () => {
		const l = lock();
		await expect(l.run(async () => Promise.reject(new Error('x')))).rejects.toThrow('x');
		expect(l.busy()).toBe(false);
	});
});
