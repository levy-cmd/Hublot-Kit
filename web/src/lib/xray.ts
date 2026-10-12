import type { ContainerState, EnvVar, Mount, Port, Radiography } from './api.ts';

/** An argument as a POSIX shell reads it: quoted when it has to be. */
function quote(arg: string): string {
	if (arg !== '' && !/[\s'"\\$`!*?[\]{}()<>|&;#~]/.test(arg)) return arg;
	return `'${arg.replaceAll("'", `'\\''`)}'`;
}

/** The entrypoint or command on one line (masked parts stay "••••••"). */
export const commandLine = (args: string[]): string => args.map(quote).join(' ');

/**
 * "127.0.0.1:8080 → 80/tcp"; "8080 → 80/tcp" on every address; with an IP
 * but no public port, a -p whose host port Docker picks at start (auto);
 * "9000/tcp" for a port only exposed (neither IP nor public).
 */
export function portLabel(p: Port, auto: string): string {
	const inner = `${p.private}/${p.type}`;
	if (!p.public && !p.ip) return inner;
	const host = p.public ? String(p.public) : auto;
	return `${p.ip && p.ip !== '0.0.0.0' ? `${p.ip}:` : ''}${host} → ${inner}`;
}

/** "source → destination": the volume's name rather than its data path. */
export const mountLabel = (m: Mount): string => `${m.type === 'volume' && m.name ? m.name : (m.source ?? '')} → ${m.destination}`;

/** "on-failure (5) · 3": policy, retries for on-failure, restarts so far. */
export function restartLabel(policy: Radiography['restartPolicy'], count: number): string {
	const name = policy.name || 'no';
	return `${name}${policy.maxRetry ? ` (${policy.maxRetry})` : ''} · ${count}`;
}

/** The 12 first hex digits of an image ID. */
export const shortId = (id: string): string => id.replace(/^sha256:/, '').slice(0, 12);

/** The value to show: the agent's, or the one revealed; null while masked. */
export function envValue(v: EnvVar, revealed: Record<string, string>): string | null {
	if (!v.masked) return v.value;
	return Object.hasOwn(revealed, v.name) ? revealed[v.name] : null;
}

/**
 * Stopped as the agent sees it (fsview: Running && Pid != 0, else the archive
 * index). The JSON has no PID: a restarting container is Running with PID 0,
 * and its status is the only one that tells it.
 */
export const isStopped = (s: ContainerState): boolean => !s.running || s.status === 'restarting';
