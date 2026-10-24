// The Secrets page (spec v0.2 §5.9): the inventory of what the radiography
// masks, for every container, by name only. The agent never sends a value
// here; one variable at a time is revealed by POST /env/reveal (journaled),
// and the value lives in the page's state only: never in a browser store,
// the URL or the console, forgotten on leaving the page and on pagehide.
import { api } from './api.ts';
import { NO_PROJECT } from './containers.ts';
import { t, tf } from './i18n/index.ts';

/** One container of GET /api/secrets: names, never values. */
export type SecretRow = {
	id: string;
	name: string;
	project: string;
	service: string;
	state: string;
	/** Names of the variables masked as secrets. */
	env: string[];
	/** Masked arguments of the entrypoint and command: position and name detected, never text. */
	args: SecretArg[];
	/** Keys of the masked labels (every label outside the whitelist, v0.2 §5.2). */
	labels: string[];
	/** The Inspect did not answer in 3 s (a container locked by an export). */
	unavailable: boolean;
};

/**
 * A masked argument (review of tranche 4): its place, and what the agent
 * detected (the flag before it, the secret name of NAME=value, the shell of
 * "sh -c"…). Its text never leaves the agent: a partial mask would leave a
 * rest of the value in it.
 */
export type SecretArg = { part: 'entrypoint' | 'cmd'; index: number; kind: string; name?: string };

export const getSecrets = (signal?: AbortSignal) => api<SecretRow[]>('/secrets', { signal });

/** Each name once, in order: Docker may keep two variables of the same name. */
export const uniqueNames = (names: string[]): string[] => [...new Set(names)];

const ARG_KINDS = ['flag', 'assignment', 'shell', 'url', 'authorization', 'other'];

/** "Command, argument 3: after --requirepass"; an unknown kind reads as "other". */
export function argText(a: SecretArg): string {
	const kind = ARG_KINDS.includes(a.kind) && (a.name || !['flag', 'assignment', 'shell'].includes(a.kind)) ? a.kind : 'other';
	return tf('secrets.arg', {
		part: t(`secrets.arg.part.${a.part === 'entrypoint' ? 'entrypoint' : 'cmd'}`),
		n: a.index + 1,
		what: tf(`secrets.arg.kind.${kind}`, { name: a.name ?? '' })
	});
}

/**
 * The secrets detected in a container: masked variables and arguments. A
 * masked label is not counted: every label outside the whitelist is masked,
 * secret or not.
 */
export const secretCount = (r: SecretRow): number => r.env.length + r.args.length;

/** A copy sorted by number of secrets, then masked labels, then name; unavailable last. */
export function sortRows(rows: SecretRow[]): SecretRow[] {
	return [...rows].sort(
		(a, b) =>
			Number(a.unavailable) - Number(b.unavailable) ||
			secretCount(b) - secretCount(a) ||
			b.labels.length - a.labels.length ||
			a.name.localeCompare(b.name)
	);
}

/** The Compose projects of the inventory, sorted. */
export const projectsOfRows = (rows: SecretRow[]): string[] =>
	[...new Set(rows.flatMap((r) => (r.project ? [r.project] : [])))].sort((a, b) => a.localeCompare(b));

/** The rows of one project ("": any, NO_PROJECT: none), as the containers list. */
export const filterRows = (rows: SecretRow[], project: string): SecretRow[] =>
	rows.filter((r) => project === '' || (project === NO_PROJECT ? !r.project : r.project === project));

/**
 * The values revealed on the page, by container ID then variable name. gen
 * counts the forgettings (pagehide, leaving the page): an answer asked
 * before one is dropped.
 */
export type InvReveals = { gen: number; values: Record<string, Record<string, string>> };
export const noInvReveals: InvReveals = { gen: 0, values: {} };

/** A pending reveal: the container and the generation, taken before the await. */
export type InvAsk = { id: string; gen: number };
export const askInv = (r: InvReveals, id: string): InvAsk => ({ id, gen: r.gen });

/** The revealed value of variable name in container id, or null. */
export function invRevealed(r: InvReveals, id: string, name: string): string | null {
	if (!Object.hasOwn(r.values, id)) return null;
	const c = r.values[id];
	return Object.hasOwn(c, name) ? c[name] : null;
}

/** Adds the answer of a reveal; dropped if the values were forgotten meanwhile. */
export function addInvReveal(r: InvReveals, asked: InvAsk, name: string, value: string): InvReveals {
	if (asked.gen !== r.gen) return r;
	const prev = Object.hasOwn(r.values, asked.id) ? r.values[asked.id] : {};
	return { gen: r.gen, values: { ...r.values, [asked.id]: { ...prev, [name]: value } } };
}

/** Hide: forgets one revealed value. */
export function removeInvReveal(r: InvReveals, id: string, name: string): InvReveals {
	if (!Object.hasOwn(r.values, id)) return r;
	const c = { ...r.values[id] };
	delete c[name];
	return { gen: r.gen, values: { ...r.values, [id]: c } };
}

/** A row folded: forgets the values revealed in that container. */
export function dropInvContainer(r: InvReveals, id: string): InvReveals {
	if (!Object.hasOwn(r.values, id)) return r;
	const values = { ...r.values };
	delete values[id];
	return { gen: r.gen, values };
}

/** pagehide, leaving the page, reload: forgets every value and every pending reveal. */
export const forgetInv = (r: InvReveals): InvReveals => ({ gen: r.gen + 1, values: {} });
