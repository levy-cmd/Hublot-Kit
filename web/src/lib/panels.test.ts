import { describe, expect, it } from 'vitest';
import { mountedPanels, PANELS } from './panels.ts';

// Review of tranche 1: under 90 rem, a hidden panel must not be mounted. A
// file system panel mounted out of sight would start the export of a
// stopped container (up to 4 GiB, 5 min, the container locked by dockerd).
describe('mountedPanels', () => {
	it('mounts the four panels side by side from 90 rem (spec v0.2 §3)', () => {
		expect([...mountedPanels(new Set(), 'logs', true)].sort()).toEqual([...PANELS].sort());
	});

	it('under 90 rem, mounts only the open panel', () => {
		expect([...mountedPanels(new Set(), 'logs', false)]).toEqual(['logs']);
		expect(mountedPanels(new Set(), 'logs', false).has('files')).toBe(false);
	});

	it('keeps a panel once opened (its stream or its index is not restarted)', () => {
		expect([...mountedPanels(new Set(['logs']), 'diff', false)].sort()).toEqual(['diff', 'logs']);
	});
});
