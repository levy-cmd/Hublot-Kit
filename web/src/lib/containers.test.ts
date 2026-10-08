import { describe, expect, it } from 'vitest';
import type { Container } from './api.ts';
import { filterContainers, formatPorts, stateLabel } from './containers.ts';
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
	const list = [c('web', 'nginx:1.27', 'running'), c('db', 'postgres:17', 'exited'), c('cache', 'redis', 'running')];
	it('hides stopped containers unless showAll', () => {
		expect(filterContainers(list, '', false).map((x) => x.name)).toEqual(['web', 'cache']);
		expect(filterContainers(list, '', true)).toHaveLength(3);
	});
	it('treats a whitespace-only query as empty', () => {
		expect(filterContainers(list, '   ', true)).toEqual(filterContainers(list, '', true));
	});
	it('matches on image only', () => {
		expect(filterContainers(list, 'postgres', true).map((x) => x.name)).toEqual(['db']);
	});
	it('searches name and image, case-insensitive', () => {
		expect(filterContainers(list, 'NGINX', true).map((x) => x.name)).toEqual(['web']);
		expect(filterContainers(list, 'db', true).map((x) => x.name)).toEqual(['db']);
	});
});

describe('stateLabel', () => {
	it('translates known states and keeps unknown ones', () => {
		expect(stateLabel('running')).toBe(t('state.running'));
		expect(stateLabel('exited')).toBe(t('state.exited'));
		expect(stateLabel('weird')).toBe('weird');
	});
});
