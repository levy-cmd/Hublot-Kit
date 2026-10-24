import { describe, expect, it } from 'vitest';
import type { Container } from './api.ts';
import {
	countGroups, filterContainers, formatPorts, NO_PROJECT, projectsOf, rowStep, stateLabel, stateTone
} from './containers.ts';
import { t } from './i18n/index.ts';

const c = (name: string, image: string, state: string): Container => ({
	id: name, name, image, state, status: '', created: '2026-10-08T09:00:00Z', ports: []
});

describe('formatPorts', () => {
	it('formats published and internal ports, sorted', () => {
		expect(
			formatPorts([
				{ private: 443, type: 'tcp' },
				{ private: 80, public: 8080, type: 'tcp', ip: '0.0.0.0' }
			])
		).toBe('8080→80/tcp, 443/tcp');
	});
	it('sorts published ports first, unpublished last', () => {
		expect(
			formatPorts([
				{ private: 22, type: 'tcp' },
				{ private: 443, public: 8443, type: 'tcp' },
				{ private: 80, public: 8080, type: 'tcp' }
			])
		).toBe('8080→80/tcp, 8443→443/tcp, 22/tcp');
	});
	it('orders tcp before udp on the same port', () => {
		expect(
			formatPorts([
				{ private: 53, public: 5353, type: 'udp' },
				{ private: 53, public: 5353, type: 'tcp' }
			])
		).toBe('5353→53/tcp, 5353→53/udp');
	});
	it('is empty without ports', () => {
		expect(formatPorts([])).toBe('');
	});
});

describe('filterContainers', () => {
	const list = [
		c('web', 'nginx:1.27', 'running'),
		c('db', 'postgres:17', 'exited'),
		c('cache', 'redis', 'paused'),
		c('job', 'alpine', 'created')
	];
	const names = (l: Container[]) => l.map((x) => x.name);
	it('shows every container without a state', () => {
		expect(filterContainers(list, '', '')).toHaveLength(4);
	});
	it('groups paused and restarting with running, the rest with stopped', () => {
		expect(names(filterContainers(list, '', 'running'))).toEqual(['web', 'cache']);
		expect(names(filterContainers(list, '', 'stopped'))).toEqual(['db', 'job']);
	});
	it('filters on an exact Docker state', () => {
		expect(names(filterContainers(list, '', 'paused'))).toEqual(['cache']);
		expect(names(filterContainers(list, '', 'exited'))).toEqual(['db']);
	});
	it('treats a whitespace-only query as empty', () => {
		expect(filterContainers(list, '   ', '')).toEqual(filterContainers(list, '', ''));
	});
	it('searches name and image, case-insensitive', () => {
		expect(names(filterContainers(list, 'NGINX', ''))).toEqual(['web']);
		expect(names(filterContainers(list, 'postgres', ''))).toEqual(['db']);
		expect(names(filterContainers(list, 'db', 'running'))).toEqual([]);
	});
	it('counts the tabs', () => {
		expect(countGroups(list)).toEqual({ all: 4, running: 2, stopped: 2 });
	});
});

// Spec v0.2 §5.1: the Project list offers the projects present, plus "No
// project"; the filter is done here, on what the agent sent.
describe('projects', () => {
	const p = (name: string, project?: string, state = 'running'): Container => ({ ...c(name, 'alpine', state), project });
	const list = [p('shop-web-1', 'shop'), p('shop-db-1', 'shop', 'exited'), p('blog-web-1', 'blog'), p('lone')];
	const names = (l: Container[]) => l.map((x) => x.name);
	it('lists the projects present, sorted, each once', () => {
		expect(projectsOf(list)).toEqual(['blog', 'shop']);
		expect(projectsOf([p('lone')])).toEqual([]);
	});
	it('filters on one project, or on no project', () => {
		expect(names(filterContainers(list, '', '', 'shop'))).toEqual(['shop-web-1', 'shop-db-1']);
		expect(names(filterContainers(list, '', '', NO_PROJECT))).toEqual(['lone']);
		expect(filterContainers(list, '', '', '')).toHaveLength(4);
		expect(names(filterContainers(list, '', 'stopped', 'shop'))).toEqual(['shop-db-1']);
		expect(filterContainers(list, '', '', 'gone')).toEqual([]);
	});
	it('marks "no project" with what no Compose project name can be', () => {
		// Compose names start with a lowercase letter or a digit.
		expect(NO_PROJECT).toMatch(/^[^a-z0-9]/);
	});
});

describe('stateLabel', () => {
	it('translates known states and keeps unknown ones', () => {
		expect(stateLabel('running')).toBe(t('state.running'));
		expect(stateLabel('exited')).toBe(t('state.exited'));
		expect(stateLabel('weird')).toBe('weird');
	});
});

// Spec v0.2 §4.1: emerald running, amber for alerts, soft text for a clean stop.
describe('stateTone', () => {
	it('maps a state and its exit code to a tone', () => {
		expect(stateTone('running')).toBe('ok');
		expect(stateTone('paused')).toBe('warn');
		expect(stateTone('restarting', 1)).toBe('warn');
		expect(stateTone('exited', 0)).toBe('idle');
		expect(stateTone('exited', 137)).toBe('warn');
		expect(stateTone('dead')).toBe('warn');
		expect(stateTone('created')).toBe('idle');
		expect(stateTone('weird')).toBe('idle');
	});
	it('never uses red for a state', () => {
		for (const s of ['running', 'paused', 'restarting', 'exited', 'dead', 'created', 'removing', '']) {
			for (const code of [undefined, 0, 1, 137]) expect(stateTone(s, code)).not.toBe('bad');
		}
	});
});

describe('rowStep', () => {
	it('moves with the arrows and jumps with Home and End', () => {
		expect(rowStep(0, 'ArrowDown', 3)).toBe(1);
		expect(rowStep(2, 'ArrowDown', 3)).toBe(2);
		expect(rowStep(1, 'ArrowUp', 3)).toBe(0);
		expect(rowStep(0, 'ArrowUp', 3)).toBe(0);
		expect(rowStep(1, 'Home', 3)).toBe(0);
		expect(rowStep(0, 'End', 3)).toBe(2);
	});
	it('ignores other keys and empty lists', () => {
		expect(rowStep(0, 'a', 3)).toBeNull();
		expect(rowStep(0, 'ArrowDown', 0)).toBeNull();
	});
});
