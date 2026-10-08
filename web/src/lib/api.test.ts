import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from './api';

afterEach(() => vi.unstubAllGlobals());

function stubFetch(impl: () => Promise<Response>) {
	vi.stubGlobal('fetch', vi.fn(impl));
}

describe('api', () => {
	it('returns the JSON body on success', async () => {
		stubFetch(async () => new Response(JSON.stringify({ version: 'dev', readOnly: false }), { status: 200 }));
		await expect(api('/meta')).resolves.toEqual({ version: 'dev', readOnly: false });
	});

	it('maps an API error body to ApiError', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'docker_unavailable', detail: 'dial' }), { status: 503 }));
		await expect(api('/containers')).rejects.toMatchObject({ status: 503, code: 'docker_unavailable', detail: 'dial' });
	});

	it('maps a network failure to code "network"', async () => {
		stubFetch(async () => {
			throw new TypeError('Failed to fetch');
		});
		const err = await api<never>('/meta').catch((e: ApiError) => e);
		expect(err).toBeInstanceOf(ApiError);
		expect(err.code).toBe('network');
	});

	it('returns undefined on 204', async () => {
		stubFetch(async () => new Response(null, { status: 204 }));
		await expect(api('/logout', { method: 'POST' })).resolves.toBeUndefined();
	});

	it('maps a non-JSON error to code "unknown"', async () => {
		stubFetch(async () => new Response('Bad Gateway', { status: 502 }));
		await expect(api('/meta')).rejects.toMatchObject({ status: 502, code: 'unknown' });
	});
});
