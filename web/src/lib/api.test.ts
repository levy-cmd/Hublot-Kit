import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile, saveFile, uploadFile } from './api';

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
		await expect(previewFile('web', '/f')).resolves.toEqual({ kind: 'text', text: 'hello', etag: '"abc"', utf8: true });

		stubFetch(async () => new Response(new Uint8Array([0x61, 0xe9]), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } }));
		await expect(previewFile('web', '/latin1')).resolves.toMatchObject({ kind: 'text', utf8: false });

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

describe('202 indexing (spec §6)', () => {
	const indexing = () =>
		new Response(JSON.stringify({ error: 'indexing', detail: 'indexation en cours (42 %)', progress: 0.42 }), { status: 202 });
	it('listDir rejects with the progress', async () => {
		stubFetch(async () => indexing());
		await expect(listDir('db', '/')).rejects.toMatchObject({ status: 202, code: 'indexing', progress: 0.42 });
	});
	it('previewFile rejects instead of showing a binary file', async () => {
		stubFetch(async () => indexing());
		await expect(previewFile('db', '/etc/hostname')).rejects.toMatchObject({ code: 'indexing' });
	});
	it('checkDownload rejects: the browser never saves the JSON', async () => {
		stubFetch(async () => indexing());
		await expect(checkDownload(downloadUrl('db', '/etc'))).rejects.toMatchObject({ code: 'indexing' });
	});
});

describe('writes (F4)', () => {
	it('saveFile sends the text with If-Match and returns the new ETag', async () => {
		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ etag: '"new"' })));
		vi.stubGlobal('fetch', fetchMock);
		await expect(saveFile('web', '/etc/a b', 'x\r\n', '"old"')).resolves.toBe('"new"');
		const [url, init] = fetchMock.mock.calls[0];
		expect(url).toBe('/api/containers/web/fs/file?path=%2Fetc%2Fa%20b');
		expect(init?.method).toBe('PUT');
		expect(init?.body).toBe('x\r\n');
		const h = new Headers(init?.headers);
		expect(h.get('If-Match')).toBe('"old"');
		expect(h.get('Content-Type')).toBe('text/plain; charset=utf-8');
	});

	it('uploadFile posts multipart with If-None-Match: *, without a JSON Content-Type', async () => {
		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ path: '/etc/n', etag: '"e"' }), { status: 201 }));
		vi.stubGlobal('fetch', fetchMock);
		const file = new File(['abc'], 'é.txt');
		await expect(uploadFile('web', '/etc', file)).resolves.toEqual({ path: '/etc/n', etag: '"e"' });
		const [url, init] = fetchMock.mock.calls[0];
		expect(url).toBe('/api/containers/web/fs/upload?dir=%2Fetc');
		expect(init?.method).toBe('POST');
		expect(init?.body).toBeInstanceOf(FormData);
		expect((init?.body as FormData).get('file')).toBeInstanceOf(File);
		expect(((init?.body as FormData).get('file') as File).name).toBe('é.txt');
		const h = new Headers(init?.headers);
		expect(h.get('If-None-Match')).toBe('*');
		expect(h.has('Content-Type')).toBe(false); // the browser sets the multipart boundary
	});

	it('maps a refused write', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'conflict' }), { status: 409 }));
		await expect(saveFile('web', '/f', 'x', '"a"')).rejects.toMatchObject({ status: 409, code: 'conflict' });
	});
});
