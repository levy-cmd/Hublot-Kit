import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import type { Radiography } from './api';
import {
	addReveal, askFor, commandLine, envValue, forgetReveals, isStopped, lastExit, mountLabel, noReveals, portLabel,
	removeReveal, restartLabel, revealedFor, shortId, splitLabels
} from './xray';

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
		// An IPv6 address between brackets, as in a URL.
		expect(portLabel({ private: 80, public: 8080, type: 'tcp', ip: '::1' }, '?')).toBe('[::1]:8080 → 80/tcp');
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

	it('keeps a revealed value for its container only, and Hide forgets it', () => {
		const masked = (name: string) => ({ name, value: null, masked: true });
		const a = addReveal(noReveals, askFor(noReveals, 'a'), 'a', 'DB_PASSWORD', 's3cr3t');
		expect(envValue(masked('DB_PASSWORD'), revealedFor(a, 'a'))).toBe('s3cr3t');
		// Same variable name, another container: nothing shown.
		expect(envValue(masked('DB_PASSWORD'), revealedFor(a, 'b'))).toBeNull();
		// An answer for a container no longer shown is dropped.
		const late = addReveal(noReveals, askFor(noReveals, 'a'), 'b', 'DB_PASSWORD', 's3cr3t');
		expect(envValue(masked('DB_PASSWORD'), revealedFor(late, 'b'))).toBeNull();
		expect(envValue(masked('DB_PASSWORD'), revealedFor(late, 'a'))).toBeNull();
		// A reveal on another container starts from nothing.
		const b = addReveal(a, askFor(a, 'b'), 'b', 'TOKEN', 't0k');
		expect(revealedFor(b, 'b')).toEqual({ TOKEN: 't0k' });
		expect(revealedFor(b, 'a')).toEqual({});
		// Hide forgets one value, keeps the others.
		const two = addReveal(a, askFor(a, 'a'), 'a', 'TOKEN', 't0k');
		const hidden = removeReveal(two, 'DB_PASSWORD');
		expect(envValue(masked('DB_PASSWORD'), revealedFor(hidden, 'a'))).toBeNull();
		expect(envValue(masked('TOKEN'), revealedFor(hidden, 'a'))).toBe('t0k');
		// Nothing above changed the earlier states.
		expect(revealedFor(a, 'a')).toEqual({ DB_PASSWORD: 's3cr3t' });
		expect(revealedFor(noReveals, '')).toEqual({});
	});

	it('drops an answer that comes back after pagehide or unmount (generation)', () => {
		const masked = { name: 'DB_PASSWORD', value: null, masked: true };
		const a = addReveal(noReveals, askFor(noReveals, 'a'), 'a', 'TOKEN', 't0k');
		const asked = askFor(a, 'a'); // before the await
		const forgotten = forgetReveals(a); // pagehide while the reveal is pending
		expect(revealedFor(forgotten, 'a')).toEqual({});
		const late = addReveal(forgotten, asked, 'a', 'DB_PASSWORD', 's3cr3t');
		expect(envValue(masked, revealedFor(late, 'a'))).toBeNull();
		// A reveal asked after the forgetting is kept.
		const again = addReveal(forgotten, askFor(forgotten, 'a'), 'a', 'DB_PASSWORD', 's3cr3t');
		expect(envValue(masked, revealedFor(again, 'a'))).toBe('s3cr3t');
		// Hide keeps the generation: a pending reveal of another variable still lands.
		const hid = removeReveal(a, 'TOKEN');
		expect(revealedFor(addReveal(hid, askFor(a, 'a'), 'a', 'DB_PASSWORD', 's3cr3t'), 'a')).toEqual({ DB_PASSWORD: 's3cr3t' });
	});

	it('keeps an empty value in clear as a chip, not as masked (spec v0.2 §5.2)', () => {
		expect(splitLabels([{ key: 'com.docker.compose.depends_on', value: '', masked: false }])).toEqual({
			shown: [{ key: 'com.docker.compose.depends_on', value: '' }],
			masked: []
		});
		expect(splitLabels([])).toEqual({ shown: [], masked: [] });
	});

	it('shows the labels in clear as chips, the masked ones as keys only (review of tranche 2)', () => {
		const split = splitLabels([
			{ key: 'architecture', value: null, masked: true },
			{ key: 'com.docker.compose.project', value: 'shop', masked: false },
			{ key: 'com.docker.compose.project.working_dir', value: null, masked: true },
			{ key: 'org.opencontainers.image.source', value: 'https://github.com/o/r', masked: false },
			{ key: 'backup.dsn', value: 's3cr3t', masked: true } // masked wins over a value
		]);
		expect(split.shown).toEqual([
			{ key: 'com.docker.compose.project', value: 'shop' },
			{ key: 'org.opencontainers.image.source', value: 'https://github.com/o/r' }
		]);
		expect(split.masked).toEqual(['architecture', 'com.docker.compose.project.working_dir', 'backup.dsn']);
		expect(JSON.stringify(split)).not.toContain('s3cr3t');
	});

	it('reads the last exit, "never" for Docker\'s zero date (spec v0.2 §5.3)', () => {
		const s = (finishedAt: string | undefined, running: boolean, exitCode: number) => ({
			status: running ? 'running' : 'exited', running, exitCode, oomKilled: false, finishedAt
		});
		expect(lastExit(s(undefined, true, 0))).toBeNull();
		expect(lastExit(s('0001-01-01T00:00:00Z', false, 0))).toBeNull();
		expect(lastExit(s('2026-10-20T08:00:00Z', false, 137))).toEqual({ at: '2026-10-20T08:00:00Z', code: 137 });
		expect(lastExit(s('2026-10-20T08:00:00Z', false, 0))).toEqual({ at: '2026-10-20T08:00:00Z', code: 0 });
		// Restarted since: the exit is known, its code is not (reset at start).
		expect(lastExit(s('2026-10-20T08:00:00Z', true, 0))).toEqual({ at: '2026-10-20T08:00:00Z', code: null });
	});

	it('treats a restarting container as stopped, like the agent (Running && Pid != 0)', () => {
		const s = (status: string, running: boolean) => ({ status, running, exitCode: 0, oomKilled: false });
		expect(isStopped(s('running', true))).toBe(false);
		expect(isStopped(s('paused', true))).toBe(false);
		expect(isStopped(s('restarting', true))).toBe(true);
		expect(isStopped(s('exited', false))).toBe(true);
		expect(isStopped(s('created', false))).toBe(true);
		expect(isStopped(s('dead', false))).toBe(true);
	});

	it('reads the JSON the agent writes (golden file of internal/api/radio_golden_test.go)', () => {
		const raw = readFileSync(new URL('../../../internal/api/testdata/radio.golden.json', import.meta.url), 'utf8');
		expect(raw).not.toContain('s3cr3t');
		const r = JSON.parse(raw) as Radiography;
		expect(r.state.finishedAt).toBe('2026-10-20T08:00:00.123456789Z');
		expect(r.state.exitCode).toBe(137);
		expect(lastExit(r.state)).toEqual({ at: '2026-10-20T08:00:00.123456789Z', code: 137 });
		expect(isStopped(r.state)).toBe(true);
		// Spec v0.2 §5.2: the label rule applied by the agent.
		expect(r.labels).toEqual([
			{ key: 'com.docker.compose.project', value: 'shop', masked: false },
			{ key: 'com.docker.compose.x', value: null, masked: true },
			{ key: 'traefik.http.middlewares.auth.basicauth.users', value: null, masked: true }
		]);
		expect(restartLabel(r.restartPolicy, r.restartCount)).toBe('on-failure (5) · 0');
		expect(r.ports.map((p) => portLabel(p, '(auto)'))).toEqual(['[::1]:8080 → 80/tcp', '(auto) → 90/tcp', '9000/tcp']);
		expect(r.env.map((v) => [v.name, envValue(v, {})])).toEqual([['DB_PASSWORD', null], ['PLAIN', 'visible']]);
		expect(r.commandMasked).toBe(true);
		expect(commandLine(r.cmd)).toBe('redis-server --requirepass ••••••');
		expect(commandLine(r.entrypoint)).toBe('');
		expect(shortId(r.imageId)).toBe('0123456789ab');
		expect(r.mounts.map((m) => [mountLabel(m), m.rw])).toEqual([['data → /data', false], ['/srv/conf → /etc/app', true]]);
		expect(r.tmpfs).toEqual(['/run']);
		expect(r.networks.map((n) => n.name)).toEqual(['bridge']);
		expect(r.workingDir).toBe('/data');
		expect(r.user).toBeUndefined();
	});
});
