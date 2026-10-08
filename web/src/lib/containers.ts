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
