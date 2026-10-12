import { decodeUtf8 } from './write.ts';

export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		public detail?: string,
		public progress?: number
	) {
		super(code);
	}
}

async function errorOf(res: Response): Promise<ApiError> {
	const body = await res.json().catch(() => null);
	const progress = typeof body?.progress === 'number' ? body.progress : undefined;
	return new ApiError(res.status, body?.error ?? 'unknown', body?.detail, progress);
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
	let res: Response;
	try {
		res = await fetch(`/api${path}`, {
			credentials: 'same-origin',
			...init,
			// JSON for a string body; a FormData gets its multipart boundary from the browser.
			headers: { ...(typeof init.body === 'string' ? { 'Content-Type': 'application/json' } : {}), ...(init.headers ?? {}) }
		});
	} catch {
		throw new ApiError(0, 'network');
	}
	// 202 is res.ok but never a result: the index is being built (spec §6).
	if (res.status === 202) throw await errorOf(res);
	if (res.status === 204) return undefined as T;
	if (!res.ok) throw await errorOf(res);
	return (await res.json().catch(() => null)) as T;
}

export type Port = { private: number; public?: number; type: string; ip?: string };
export type Container = {
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	created: string;
	ports: Port[];
};
export type Meta = { version: string; readOnly: boolean };

export const login = (token: string) =>
	api<{ ok: boolean }>('/login', { method: 'POST', body: JSON.stringify({ token }) });
export const logout = () => api<void>('/logout', { method: 'POST' });
export const getMeta = () => api<Meta>('/meta');
export const listContainers = () => api<Container[]>('/containers');

export type EntryType = 'file' | 'dir' | 'symlink' | 'other';
export type Entry = {
	name: string;
	type: EntryType;
	size: number;
	mode: number; // Unix bits 0o7777
	uid: number;
	gid: number;
	modTime: string;
	linkTarget?: string;
};
// type is "dir" for a directory; for another path, entries is empty.
export type Listing = { path: string; type: EntryType; entries: Entry[]; truncated: boolean };
export type Preview =
	| { kind: 'text'; text: string; etag: string; utf8: boolean } // utf8 false: shown with U+FFFD, not editable
	| { kind: 'image'; blob: Blob; etag: string }
	| { kind: 'binary' };

const fsPath = (id: string, route: string, path: string) =>
	`/containers/${encodeURIComponent(id)}/fs${route}?path=${encodeURIComponent(path)}`;

export const listDir = (id: string, path: string) => api<Listing>(fsPath(id, '', path));
export const downloadUrl = (id: string, path: string) => `/api${fsPath(id, '/download', path)}`;

// GET of a non-JSON route: errors still arrive as JSON.
async function get(url: string, signal?: AbortSignal): Promise<Response> {
	let res: Response;
	try {
		res = await fetch(url, { credentials: 'same-origin', signal });
	} catch {
		throw new ApiError(0, 'network');
	}
	if (res.status === 202 || !res.ok) throw await errorOf(res);
	return res;
}

export async function previewFile(id: string, path: string): Promise<Preview> {
	const res = await get(`/api${fsPath(id, '/file', path)}`);
	const type = res.headers.get('Content-Type') ?? '';
	const etag = res.headers.get('ETag') ?? '';
	if (/^image\/(png|jpeg|gif|webp)$/.test(type)) return { kind: 'image', blob: await res.blob(), etag };
	if (type.startsWith('text/plain')) {
		const bytes = new Uint8Array(await res.arrayBuffer());
		const strict = decodeUtf8(bytes);
		return { kind: 'text', text: strict ?? new TextDecoder().decode(bytes), etag, utf8: strict !== null };
	}
	return { kind: 'binary' };
}

// With &check=1 the agent runs every check of a download, then answers 204
// without reading anything (no tar started at Docker). The UI checks first,
// shows the error if any, otherwise lets the browser download by navigation.
export async function checkDownload(url: string): Promise<void> {
	await get(`${url}&check=1`);
}

export type Change = { path: string; kind: 'added' | 'modified' | 'deleted' };
export const listChanges = (id: string) => api<Change[]>(`/containers/${encodeURIComponent(id)}/changes`);

// F4 (spec §5.3). The If-Match is the ETag of the preview, as received.
export async function saveFile(id: string, path: string, text: string, etag: string): Promise<string> {
	// No ETag, no condition: an empty If-Match would be an unconditional-looking write.
	if (!etag) throw new ApiError(0, 'unknown');
	const res = await api<{ etag: string }>(fsPath(id, '/file', path), {
		method: 'PUT',
		body: text,
		headers: { 'If-Match': etag, 'Content-Type': 'text/plain; charset=utf-8' }
	});
	return res.etag;
}

// One file per request; the agent refuses an existing name (If-None-Match: *).
export function uploadFile(id: string, dir: string, file: File): Promise<{ path: string; etag: string }> {
	const body = new FormData();
	body.append('file', file, file.name);
	return api(`/containers/${encodeURIComponent(id)}/fs/upload?dir=${encodeURIComponent(dir)}`, {
		method: 'POST',
		body,
		headers: { 'If-None-Match': '*' }
	});
}

// F6 (spec §6): an SSE stream, one event per line; "end", "dropped" and
// "failure" events. after resumes after a line ID (pause, "follow again").
export type LogLine = { ts?: string; stream: 'stdout' | 'stderr'; line: string; truncated?: boolean };

export function logsUrl(id: string, opts: { tail?: number; after?: string } = {}): string {
	const q = new URLSearchParams();
	if (opts.tail !== undefined) q.set('tail', String(opts.tail));
	if (opts.after) q.set('after', opts.after);
	const query = q.toString();
	return `/api/containers/${encodeURIComponent(id)}/logs${query ? '?' + query : ''}`;
}

export const logsDownloadUrl = (id: string) => `/api/containers/${encodeURIComponent(id)}/logs/download`;
