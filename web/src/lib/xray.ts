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
	if (!p.ip || p.ip === '0.0.0.0') return `${host} → ${inner}`;
	// An IPv6 address between brackets, as in a URL: "[::1]:8080".
	return `${p.ip.includes(':') ? `[${p.ip}]` : p.ip}:${host} → ${inner}`;
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
 * The values the user revealed, for one container only. They live in the
 * component's state: never in localStorage, the URL or the console.
 */
export type Reveals = { id: string; values: Record<string, string> };
export const noReveals: Reveals = { id: '', values: {} };

/** The values revealed for container id: none if they belong to another. */
export const revealedFor = (r: Reveals, id: string): Record<string, string> => (r.id === id ? r.values : {});

/**
 * Adds the answer to a reveal asked for container `asked` while `shown` is
 * displayed: dropped if the container changed meanwhile; a reveal on
 * another container starts from nothing.
 */
export function addReveal(r: Reveals, asked: string, shown: string, name: string, value: string): Reveals {
	if (asked !== shown) return r;
	return { id: shown, values: { ...revealedFor(r, shown), [name]: value } };
}

/** Hide: forgets one revealed value. */
export function removeReveal(r: Reveals, name: string): Reveals {
	const values = { ...r.values };
	delete values[name];
	return { id: r.id, values };
}

/**
 * Stopped as the agent sees it (fsview: Running && Pid != 0, else the archive
 * index). The JSON has no PID: a restarting container is Running with PID 0,
 * and its status is the only one that tells it.
 */
export const isStopped = (s: ContainerState): boolean => !s.running || s.status === 'restarting';
