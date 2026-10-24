import { describe, expect, it } from 'vitest';
import type { LogLine } from './api';
import { appendLines, enqueue, filterLines, formatTs, levelOf, MAX_LINES, parseCount, parseFailure, parseLine, streamEnd } from './logs';

const line = (text: string, stream: 'stdout' | 'stderr' = 'stdout'): LogLine => ({ ts: '2026-10-10T07:14:22.769046552Z', stream, line: text });

describe('logs (F6)', () => {
	it('parses a line event, refuses anything else', () => {
		expect(parseLine('{"ts":"2026-10-10T07:14:22Z","stream":"stderr","line":"x","truncated":true}')).toEqual({
			ts: '2026-10-10T07:14:22Z', stream: 'stderr', line: 'x', truncated: true
		});
		expect(parseLine('{"stream":"stdout","line":""}')).toEqual({ stream: 'stdout', line: '' });
		for (const bad of ['', 'nope', '{}', '{"stream":"stdin","line":"x"}', '{"stream":"stdout","line":3}', 'null']) {
			expect(parseLine(bad)).toBeNull();
		}
	});

	it('parses the dropped and failure events', () => {
		expect(parseCount('{"count":12}')).toBe(12);
		expect(parseCount('{"count":"x"}')).toBe(0);
		expect(parseCount('{"count":-3}')).toBe(0);
		expect(parseCount('garbage')).toBe(0);
		expect(parseFailure('{"error":"logs_unavailable","detail":"none"}')).toBe('logs_unavailable');
		expect(parseFailure('garbage')).toBe('unknown');
	});

	it('keeps at most MAX_LINES, the newest, and counts the others', () => {
		const first = appendLines([], [line('a'), line('b')], 3);
		expect(first.lines.map((l) => l.line)).toEqual(['a', 'b']);
		expect(first.lines.map((l) => l.key)).toEqual([0, 1]);
		const next = appendLines(first.lines, [line('c'), line('d')], 3);
		expect(next.lines.map((l) => l.line)).toEqual(['b', 'c', 'd']);
		expect(next.removed).toBe(1);
		expect(next.lines.map((l) => l.key)).toEqual([1, 2, 3]);
		// After a trim, the next key follows the last one, not the length.
		const third = appendLines(next.lines, [line('e')], 3);
		expect(third.lines.map((l) => l.key)).toEqual([2, 3, 4]); // keys never reused
		expect(MAX_LINES).toBe(5000);
	});

	// A hidden tab suspends requestAnimationFrame: the queue must stay bounded.
	it('bounds the queue of pending lines, drops the oldest and counts them', () => {
		const q: LogLine[] = [];
		let removed = 0;
		for (let i = 0; i < 10; i++) removed += enqueue(q, line(String(i)), 3);
		expect(q.length).toBeLessThanOrEqual(6);
		expect(q.length + removed).toBe(10);
		expect(q[q.length - 1].line).toBe('9'); // the newest stay
		expect(q.map((l) => Number(l.line))).toEqual([...q.keys()].map((k) => 10 - q.length + k));
	});

	it('filters by stream and by a case-insensitive text', () => {
		const ls = appendLines([], [line('GET /health 200'), line('Error: boom', 'stderr'), line('get /api 500')]).lines;
		expect(filterLines(ls, { query: '', stdout: true, stderr: true })).toHaveLength(3);
		expect(filterLines(ls, { query: 'get', stdout: true, stderr: true }).map((l) => l.line)).toEqual(['GET /health 200', 'get /api 500']);
		expect(filterLines(ls, { query: '', stdout: false, stderr: true }).map((l) => l.line)).toEqual(['Error: boom']);
		expect(filterLines(ls, { query: '  boom ', stdout: true, stderr: false })).toEqual([]);
	});

	it('formats a timestamp to the millisecond', () => {
		expect(formatTs('2026-10-10T07:14:22.769046552Z', 'UTC')).toBe('07:14:22.769');
		expect(formatTs(undefined, 'UTC')).toBe('');
		expect(formatTs('garbage', 'UTC')).toBe('');
	});

	// Q37: how each end of a stream is handled; none reconnects.
	it('decides what an end of stream does', () => {
		expect(streamEnd('end')).toEqual({ status: 'ended' });
		expect(streamEnd('failure', '{"error":"unauthorized","detail":"session ended"}')).toEqual({ status: 'error', code: 'unauthorized', login: true });
		expect(streamEnd('failure', '{"error":"logs_unavailable"}')).toEqual({ status: 'error', code: 'logs_unavailable' });
		expect(streamEnd('failure', 'garbage')).toEqual({ status: 'error', code: 'unknown' });
		expect(streamEnd('evicted', '{"error":"internal"}')).toEqual({ status: 'error', code: 'evicted' });
	});
});

// Spec v0.2 §3: the level is guessed from the text (heuristic, said in the tooltip).
describe('levelOf', () => {
	it('reads an upper-case level word', () => {
		expect(levelOf('2026-10-23 ERROR boom')).toBe('error');
		expect(levelOf('[WARN] disk almost full')).toBe('warn');
		expect(levelOf('2025-10-18 08:57:12,345 INFO Processed task')).toBe('info');
		expect(levelOf('DEBUG cache hit')).toBe('debug');
		expect(levelOf('FATAL: cannot bind')).toBe('error');
		expect(levelOf('WARNING: deprecated')).toBe('warn');
	});

	it('takes the first level word of the line', () => {
		expect(levelOf('INFO retry after ERROR')).toBe('info');
	});

	it('reads a JSON or logfmt level field', () => {
		expect(levelOf('{"level":"warn","msg":"x"}')).toBe('warn');
		expect(levelOf('{"severity":"ERROR","msg":"x"}')).toBe('error');
		expect(levelOf('ts=1 level=debug msg=x')).toBe('debug');
		expect(levelOf('time="x" level=info msg="started"')).toBe('info');
	});

	it('guesses nothing from plain text', () => {
		expect(levelOf('plain line')).toBeNull();
		expect(levelOf('no error here')).toBeNull(); // lower case: prose, not a level
		expect(levelOf('INFORMATION')).toBeNull();
		expect(levelOf('{"level":3}')).toBeNull();
		expect(levelOf('')).toBeNull();
	});
});
