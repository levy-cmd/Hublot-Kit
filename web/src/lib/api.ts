export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		public detail?: string
	) {
		super(code);
	}
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
	let res: Response;
	try {
		res = await fetch(`/api${path}`, {
			credentials: 'same-origin',
			...init,
			headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) }
		});
	} catch {
		throw new ApiError(0, 'network');
	}
	if (res.status === 204) return undefined as T;
	const body = await res.json().catch(() => null);
	if (!res.ok) throw new ApiError(res.status, body?.error ?? 'unknown', body?.detail);
	return body as T;
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
