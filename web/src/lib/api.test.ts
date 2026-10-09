import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile } from './api';

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

	it('sends no Content-Type on a GET and JSON on login', async () => {
		const fetchMock = vi.fn<typeof fetch>(async () => new Response('{"ok":true}', { status: 200 }));
		vi.stubGlobal('fetch', fetchMock);
		await api('/meta');
		expect(new Headers(fetchMock.mock.calls[0][1]?.headers).has('Content-Type')).toBe(false);
		await login('x');
		expect(new Headers(fetchMock.mock.calls[1][1]?.headers).get('Content-Type')).toBe('application/json');
	});
});

describe('file routes', () => {
	it('lists a directory with an encoded id and path', async () => {
		const fetchMock = vi.fn<typeof fetch>(
			async () => new Response(JSON.stringify({ path: '/a b', type: 'dir', entries: [], truncated: false }))
		);
		vi.stubGlobal('fetch', fetchMock);
		await expect(listDir('web', '/a b')).resolves.toMatchObject({ path: '/a b', type: 'dir' });
		expect(fetchMock.mock.calls[0][0]).toBe('/api/containers/web/fs?path=%2Fa%20b');
	});

	it('builds the download URL', () => {
		expect(downloadUrl('web', '/etc/my file')).toBe('/api/containers/web/fs/download?path=%2Fetc%2Fmy%20file');
	});

	it('previews text, images and binaries', async () => {
		stubFetch(async () => new Response('hello', { headers: { 'Content-Type': 'text/plain; charset=utf-8', ETag: '"abc"' } }));
		await expect(previewFile('web', '/f')).resolves.toEqual({ kind: 'text', text: 'hello', etag: '"abc"' });

		stubFetch(async () => new Response(new Uint8Array([137, 80, 78, 71]), { headers: { 'Content-Type': 'image/png', ETag: '"x"' } }));
		const img = await previewFile('web', '/i.png');
		expect(img.kind).toBe('image');
		if (img.kind === 'image') expect(img.blob.size).toBe(4);

		stubFetch(async () => new Response(null, { headers: { 'Content-Type': 'application/octet-stream' } }));
		await expect(previewFile('web', '/bin')).resolves.toEqual({ kind: 'binary' });
	});

	it('maps preview errors', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'too_large' }), { status: 413 }));
		await expect(previewFile('web', '/big')).rejects.toMatchObject({ status: 413, code: 'too_large' });
	});

	it('checks a download without starting it', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'link_in_path' }), { status: 422 }));
		await expect(checkDownload('/api/x?path=%2Fl')).rejects.toMatchObject({ code: 'link_in_path' });
		const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
		vi.stubGlobal('fetch', fetchMock);
		await expect(checkDownload('/api/x?path=%2Fetc')).resolves.toBeUndefined();
		expect(fetchMock.mock.calls[0][0]).toBe('/api/x?path=%2Fetc&check=1');
	});
});
