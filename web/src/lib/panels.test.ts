import { describe, expect, it } from 'vitest';
import { logsLive, mountedPanels, PANELS } from './panels.ts';

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

	it('keeps a panel once opened (its index is not rebuilt)', () => {
		expect([...mountedPanels(new Set(['logs']), 'diff', false)].sort()).toEqual(['diff', 'logs']);
	});
});

// Review of tranche 1 (Codex), ruled for tranche 2: a logs panel opened,
// then hidden, stays mounted, but must not keep its stream open.
describe('logsLive', () => {
	it('under 90 rem, a hidden logs panel opens no stream', () => {
		expect(logsLive('diff', false)).toBe(false);
		expect(logsLive('files', false)).toBe(false);
		expect(logsLive('logs', false)).toBe(true);
	});

	it('from 90 rem, the logs panel is always in sight: its stream stays open', () => {
		for (const p of PANELS) expect(logsLive(p, true)).toBe(true);
	});
});
