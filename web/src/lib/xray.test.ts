import { describe, expect, it } from 'vitest';
import type { Radiography } from './api';
import { commandLine, envValue, isStopped, mountLabel, portLabel, restartLabel, shortId } from './xray';

describe('xray (F7)', () => {
	it('shows a command as a shell would read it, masked parts kept', () => {
		expect(commandLine(['nginx', '-g', 'daemon off;'])).toBe("nginx -g 'daemon off;'");
		expect(commandLine(['redis-server', '--requirepass', '••••••'])).toBe('redis-server --requirepass ••••••');
		expect(commandLine(['sh', '-c', "echo 'a'"])).toBe(`sh -c 'echo '\\''a'\\'''`);
		expect(commandLine(['a', ''])).toBe("a ''");
		expect(commandLine([])).toBe('');
	});

	it('labels ports: published, published on a port chosen at start, exposed only', () => {
		expect(portLabel({ private: 80, public: 8080, type: 'tcp', ip: '127.0.0.1' }, '?')).toBe('127.0.0.1:8080 → 80/tcp');
		expect(portLabel({ private: 80, public: 8080, type: 'tcp', ip: '0.0.0.0' }, '?')).toBe('8080 → 80/tcp');
		// -p 90 on a stopped container: an IP, no public port yet.
		expect(portLabel({ private: 90, type: 'tcp', ip: '0.0.0.0' }, '(auto)')).toBe('(auto) → 90/tcp');
		expect(portLabel({ private: 90, type: 'udp', ip: '127.0.0.1' }, '(auto)')).toBe('127.0.0.1:(auto) → 90/udp');
		// EXPOSE without -p: neither IP nor public.
		expect(portLabel({ private: 9000, type: 'tcp' }, '?')).toBe('9000/tcp');
	});

	it('labels mounts and the restart policy', () => {
		const m = { type: 'bind', source: '/srv/conf', destination: '/etc/app', mode: 'ro', rw: false };
		expect(mountLabel(m)).toBe('/srv/conf → /etc/app');
		expect(mountLabel({ type: 'volume', name: 'data', source: '/var/lib/docker/volumes/data/_data', destination: '/data', rw: true })).toBe('data → /data');
		expect(restartLabel({ name: 'on-failure', maxRetry: 5 }, 3)).toBe('on-failure (5) · 3');
		expect(restartLabel({ name: 'no' }, 0)).toBe('no · 0');
		expect(restartLabel({ name: '' }, 1)).toBe('no · 1');
	});

	it('shortens an image ID', () => {
		expect(shortId('sha256:0123456789abcdef0123')).toBe('0123456789ab');
		expect(shortId('')).toBe('');
	});

	it('shows a revealed value, never a masked one', () => {
		const masked = { name: 'DB_PASSWORD', value: null, masked: true };
		expect(envValue(masked, {})).toBeNull();
		expect(envValue(masked, { DB_PASSWORD: 's3cr3t' })).toBe('s3cr3t');
		// A name inherited from Object.prototype is not a revealed value.
		expect(envValue({ name: 'constructor', value: null, masked: true }, {})).toBeNull();
		expect(envValue({ name: 'PATH', value: '/bin', masked: false }, {})).toBe('/bin');
	});

	it('treats a restarting container as stopped, like the agent (Running && Pid != 0)', () => {
		const s = (status: string, running: boolean) => ({ status, running, exitCode: 0, oomKilled: false });
		expect(isStopped(s('running', true))).toBe(false);
		expect(isStopped(s('paused', true))).toBe(false);
		expect(isStopped(s('restarting', true))).toBe(true);
		expect(isStopped(s('exited', false))).toBe(true);
		expect(isStopped(s('created', false))).toBe(true);
	});

	it('reads the JSON of GET /api/containers/{id} as the agent writes it (omitempty fields absent)', () => {
		// Shape of radioJSON (internal/api/container.go): a created container,
		// zero dates emptied by the agent, a masked variable as null.
		const r = JSON.parse(`{"id":"abc","name":"web","image":"nginx:1.27","imageId":"sha256:0123456789abcdef","created":"2026-10-10T07:14:22Z",
			"state":{"status":"created","running":false,"exitCode":0,"oomKilled":false},
			"restartCount":0,"restartPolicy":{"name":"no"},"entrypoint":[],"cmd":["redis-server","--requirepass","••••••"],"commandMasked":true,
			"tty":false,"logDriver":"json-file",
			"env":[{"name":"DB_PASSWORD","value":null,"masked":true},{"name":"PLAIN","value":"visible","masked":false}],
			"ports":[{"private":90,"type":"tcp","ip":"0.0.0.0"},{"private":9000,"type":"tcp"}],
			"networks":[],"mounts":[{"type":"volume","name":"data","source":"/var/lib/docker/volumes/data/_data","destination":"/data","rw":true}],
			"tmpfs":["/run"]}`) as Radiography;
		expect(r.state.startedAt).toBeUndefined();
		expect(r.state.finishedAt).toBeUndefined();
		expect(isStopped(r.state)).toBe(true);
		expect(restartLabel(r.restartPolicy, r.restartCount)).toBe('no · 0');
		expect(r.ports.map((p) => portLabel(p, '(auto)'))).toEqual(['(auto) → 90/tcp', '9000/tcp']);
		expect(r.env.map((v) => envValue(v, {}))).toEqual([null, 'visible']);
		expect(commandLine(r.cmd)).toBe('redis-server --requirepass ••••••');
		expect(shortId(r.imageId)).toBe('0123456789ab');
		expect(r.mounts.map(mountLabel)).toEqual(['data → /data']);
	});
});
