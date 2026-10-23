import type { Container, Port } from './api.ts';
import { t } from './i18n/index.ts';

export function formatPorts(ports: Port[]): string {
	return [...ports]
		.sort((a, b) => Number(!a.public) - Number(!b.public) || (a.public ?? a.private) - (b.public ?? b.private) ||
				a.type.localeCompare(b.type))
		.map((p) => (p.public ? `${p.public}→${p.private}/${p.type}` : `${p.private}/${p.type}`))
		.join(', ');
}

/** The tabs of the list: paused and restarting count as running (they hold a process). */
export type Group = 'running' | 'stopped';
const RUNNING = ['running', 'paused', 'restarting'];
export const groupOf = (state: string): Group => (RUNNING.includes(state) ? 'running' : 'stopped');

/**
 * The containers of a tab ("running", "stopped") or of one Docker state
 * ("paused"…), "" for all, whose name or image contains the query.
 */
export function filterContainers(list: Container[], query: string, state: string): Container[] {
	const q = query.trim().toLowerCase();
	return list.filter(
		(c) =>
			(state === '' || (state === 'running' || state === 'stopped' ? groupOf(c.state) === state : c.state === state)) &&
			(q === '' || c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q))
	);
}

export function countGroups(list: Container[]): { all: number; running: number; stopped: number } {
	const running = list.filter((c) => groupOf(c.state) === 'running').length;
	return { all: list.length, running, stopped: list.length - running };
}

export const STATES = ['running', 'paused', 'restarting', 'exited', 'created', 'removing', 'dead'];

export function stateLabel(state: string): string {
	return STATES.includes(state) ? t(`state.${state}`) : state;
}

// The colour that doubles a state's text (spec v0.2 §4.1): emerald when it
// runs, amber for an alert (pause, restart, abnormal exit, dead), soft text
// for a clean stop. Red is kept for UI errors and deletions in a diff.
export type Tone = 'ok' | 'warn' | 'idle';

export function stateTone(state: string, exitCode?: number): Tone {
	if (state === 'running') return 'ok';
	if (state === 'paused' || state === 'restarting' || state === 'dead') return 'warn';
	if (state === 'exited') return (exitCode ?? 0) === 0 ? 'idle' : 'warn';
	return 'idle';
}

// Keyboard moves in the container list: the index to focus, or null when
// the key is not a move.
export function rowStep(current: number, key: string, length: number): number | null {
	if (length === 0) return null;
	switch (key) {
		case 'ArrowDown':
			return Math.min(current + 1, length - 1);
		case 'ArrowUp':
			return Math.max(current - 1, 0);
		case 'Home':
			return 0;
		case 'End':
			return length - 1;
		default:
			return null;
	}
}
