import type { Container, Port } from './api.ts';
import { t, type Key } from './i18n/index.ts';

export function formatPorts(ports: Port[]): string {
	return [...ports]
		.sort((a, b) => Number(!a.public) - Number(!b.public) || (a.public ?? a.private) - (b.public ?? b.private) ||
				a.type.localeCompare(b.type))
		.map((p) => (p.public ? `${p.public}→${p.private}/${p.type}` : `${p.private}/${p.type}`))
		.join(', ');
}

export function filterContainers(list: Container[], query: string, showAll: boolean): Container[] {
	const q = query.trim().toLowerCase();
	return list.filter(
		(c) =>
			(showAll || c.state === 'running') &&
			(q === '' || c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q))
	);
}

const STATES = ['running', 'exited', 'created', 'paused', 'restarting', 'removing', 'dead'];

export function stateLabel(state: string): string {
	return STATES.includes(state) ? t(`state.${state}` as Key) : state;
}

// The colour of a state on the quay: signal green, amber, rust, or steel.
// Always shown next to the state's text, never alone.
export type Tone = 'ok' | 'warn' | 'bad' | 'idle';

export function stateTone(state: string): Tone {
	if (state === 'running') return 'ok';
	if (state === 'paused' || state === 'restarting') return 'warn';
	if (state === 'exited' || state === 'dead') return 'bad';
	return 'idle';
}

// The short id (12 characters, as `docker ps`) painted on the plate: the
// last character is boxed, as the check digit of an ISO 6346 marking.
export function plaqueId(id: string): { serial: string; check: string } {
	const short = id.slice(0, 12).toUpperCase();
	const body = short.slice(0, -1);
	return {
		serial: body.length > 6 ? `${body.slice(0, 6)} ${body.slice(6)}` : body,
		check: short.slice(-1)
	};
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
