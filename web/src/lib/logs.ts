import type { LogLine } from './api.ts';

/** Lines kept by the view; older ones leave it (the download has them all). */
export const MAX_LINES = 5000;

/** A line of the view: key is local and never reused (keyed {#each}). */
export type ShownLine = LogLine & { key: number };

/** The data of a line event, or null if it is not one. */
export function parseLine(data: string): LogLine | null {
	let v: unknown;
	try {
		v = JSON.parse(data);
	} catch {
		return null;
	}
	if (typeof v !== 'object' || v === null) return null;
	const o = v as Record<string, unknown>;
	if ((o.stream !== 'stdout' && o.stream !== 'stderr') || typeof o.line !== 'string') return null;
	const out: LogLine = { stream: o.stream, line: o.line };
	if (typeof o.ts === 'string') out.ts = o.ts;
	if (o.truncated === true) out.truncated = true;
	return out;
}

/** The count of a "dropped" event (0 if unreadable). */
export function parseCount(data: string): number {
	try {
		const n = JSON.parse(data)?.count;
		return typeof n === 'number' && n > 0 ? n : 0;
	} catch {
		return 0;
	}
}

/** The error code of a "failure" event. */
export function parseFailure(data: string): string {
	try {
		const code = JSON.parse(data)?.error;
		return typeof code === 'string' ? code : 'unknown';
	} catch {
		return 'unknown';
	}
}

/** What the view does at an end of stream (Q29, Q37): never reconnect. */
export type StreamEnd = { status: 'ended' | 'error'; code?: string; login?: boolean };

/**
 * "end": the container's stream ended. "failure": an error of §6;
 * "unauthorized" (logout, expiry) sends to the login page. "evicted": a
 * newer stream took the slot.
 */
export function streamEnd(name: 'end' | 'failure' | 'evicted', data = ''): StreamEnd {
	if (name === 'end') return { status: 'ended' };
	if (name === 'evicted') return { status: 'error', code: 'evicted' };
	const code = parseFailure(data);
	return code === 'unauthorized' ? { status: 'error', code, login: true } : { status: 'error', code };
}

/** Appends a batch, keeps the newest max lines; removed counts the others. */
export function appendLines(lines: ShownLine[], batch: LogLine[], max = MAX_LINES): { lines: ShownLine[]; removed: number } {
	let key = lines.length ? lines[lines.length - 1].key + 1 : 0;
	const all = lines.concat(batch.map((l) => ({ ...l, key: key++ })));
	const removed = Math.max(0, all.length - max);
	return { lines: removed ? all.slice(removed) : all, removed };
}

export type LogFilter = { query: string; stdout: boolean; stderr: boolean };

/** The lines of the checked streams that contain the query (case ignored). */
export function filterLines(lines: ShownLine[], f: LogFilter): ShownLine[] {
	const q = f.query.trim().toLowerCase();
	return lines.filter((l) => (l.stream === 'stdout' ? f.stdout : f.stderr) && (q === '' || l.line.toLowerCase().includes(q)));
}

/** HH:MM:SS.mmm of an RFC 3339 timestamp, in timeZone (default: the browser's). */
export function formatTs(ts: string | undefined, timeZone?: string): string {
	if (!ts) return '';
	const d = new Date(ts);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleTimeString('en-GB', { hour12: false, timeZone, hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 });
}
