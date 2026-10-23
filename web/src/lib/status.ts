// Docker's Status column ("Up 3 hours (healthy)", "Exited (137) 2 days
// ago"), parsed then translated (spec v0.2 §3, Uptime). The durations are
// those of HumanDuration (go-units), a finite vocabulary; any text it does
// not recognise is shown as is.
import { lang as currentLang, tl, type Lang } from './i18n/index.ts';

type Unit = 'second' | 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year';
export type Duration = { n: number; unit: Unit } | { approx: 'less' | 'aboutMinute' | 'aboutHour' };
type Health = 'healthy' | 'unhealthy' | 'starting';

export type Status =
	| { kind: 'up'; since: Duration; health?: Health; paused?: boolean }
	| { kind: 'exited' | 'restarting'; code: number; ago: Duration }
	| { kind: 'created' }
	| { kind: 'raw'; text: string };

const UNITS: readonly Unit[] = ['second', 'minute', 'hour', 'day', 'week', 'month', 'year'];

function parseDuration(s: string): Duration | null {
	if (s === 'Less than a second') return { approx: 'less' };
	if (s === 'About a minute') return { approx: 'aboutMinute' };
	if (s === 'About an hour') return { approx: 'aboutHour' };
	const m = /^(\d+) (second|minute|hour|day|week|month|year)s?$/.exec(s);
	if (!m || !UNITS.includes(m[2] as Unit)) return null;
	return { n: Number(m[1]), unit: m[2] as Unit };
}

const HEALTH: Record<string, Health> = { healthy: 'healthy', unhealthy: 'unhealthy', 'health: starting': 'starting' };

export function parseStatus(s: string): Status {
	const raw: Status = { kind: 'raw', text: s };
	if (s === 'Created') return { kind: 'created' };
	let m = /^Up (.+?)(?: \((healthy|unhealthy|health: starting|Paused)\))?$/.exec(s);
	if (m) {
		const since = parseDuration(m[1]);
		if (!since) return raw;
		if (m[2] === 'Paused') return { kind: 'up', since, paused: true };
		return m[2] ? { kind: 'up', since, health: HEALTH[m[2]] } : { kind: 'up', since };
	}
	m = /^(Exited|Restarting) \((\d+)\) (.+) ago$/.exec(s);
	if (m) {
		const ago = parseDuration(m[3]);
		if (!ago) return raw;
		return { kind: m[1] === 'Exited' ? 'exited' : 'restarting', code: Number(m[2]), ago };
	}
	return raw;
}

const fill = (text: string, vars: Record<string, string | number>) =>
	text.replace(/\{(\w+)\}/g, (all, k: string) => (k in vars ? String(vars[k]) : all));

function durationText(d: Duration, l: Lang): string {
	if ('approx' in d) return tl(l, `status.${d.approx}`);
	return fill(tl(l, `status.${d.unit}.${d.n === 1 ? 'one' : 'other'}`), { n: d.n });
}

export function statusText(st: Status, l: Lang = currentLang): string {
	switch (st.kind) {
		case 'raw':
			return st.text;
		case 'created':
			return tl(l, 'status.created');
		case 'up': {
			let out = fill(tl(l, 'status.up'), { d: durationText(st.since, l) });
			if (st.health) out += tl(l, `status.${st.health}`);
			if (st.paused) out += tl(l, 'status.paused');
			return out;
		}
		default: {
			const out = fill(tl(l, `status.${st.kind}`), { d: durationText(st.ago, l) });
			return st.code !== 0 ? out + fill(tl(l, 'status.code'), { n: st.code }) : out;
		}
	}
}

/** The exit code an exited or restarting status names; undefined otherwise. */
export function exitCodeOf(st: Status): number | undefined {
	return st.kind === 'exited' || st.kind === 'restarting' ? st.code : undefined;
}
