import { describe, expect, it } from 'vitest';
import { exitCodeOf, parseStatus, statusText } from './status.ts';

// Docker's Status column (spec v0.2 §3, Uptime): HumanDuration of go-units.
describe('status (fr)', () => {
	const fr = (s: string) => statusText(parseStatus(s), 'fr');

	it('translates a running container', () => {
		expect(fr('Up 3 hours (healthy)')).toBe('depuis 3 heures, sain');
		expect(fr('Up Less than a second')).toBe("depuis moins d'une seconde");
		expect(fr('Up About a minute')).toBe('depuis environ une minute');
		expect(fr('Up 1 second')).toBe('depuis 1 seconde');
		expect(fr('Up 2 minutes (health: starting)')).toBe('depuis 2 minutes, santé en cours de vérification');
		expect(fr('Up 5 days (unhealthy)')).toBe('depuis 5 jours, en mauvaise santé');
		expect(fr('Up 3 weeks (Paused)')).toBe('depuis 3 semaines, en pause');
		expect(fr('Up 4 months')).toBe('depuis 4 mois');
		expect(fr('Up 2 years')).toBe('depuis 2 ans');
	});

	it('translates a stopped container, with the exit code only when not zero', () => {
		expect(fr('Exited (137) 2 days ago')).toBe('arrêté il y a 2 jours, code 137');
		expect(fr('Exited (0) About an hour ago')).toBe('arrêté il y a environ une heure');
		expect(fr('Restarting (1) 5 seconds ago')).toBe('redémarre, arrêté il y a 5 secondes, code 1');
		expect(fr('Created')).toBe('créé');
	});

	it('keeps unknown text', () => {
		expect(fr('Removal In Progress')).toBe('Removal In Progress');
		expect(fr('Dead')).toBe('Dead');
		expect(fr('Up for a while')).toBe('Up for a while');
		expect(fr('Exited (x) 2 days ago')).toBe('Exited (x) 2 days ago');
		expect(fr('Up 3 fortnights')).toBe('Up 3 fortnights');
		expect(fr('')).toBe('');
	});
});

describe('status (en)', () => {
	const en = (s: string) => statusText(parseStatus(s), 'en');

	it('translates the same statuses', () => {
		expect(en('Up 3 hours (healthy)')).toBe('up 3 hours, healthy');
		expect(en('Up 1 second')).toBe('up 1 second');
		expect(en('Up Less than a second')).toBe('up less than a second');
		expect(en('Exited (137) 2 days ago')).toBe('exited 2 days ago, code 137');
		expect(en('Exited (0) About an hour ago')).toBe('exited about an hour ago');
		expect(en('Removal In Progress')).toBe('Removal In Progress');
	});
});

describe('exitCodeOf', () => {
	it('reads the code of an exited or restarting container', () => {
		expect(exitCodeOf(parseStatus('Exited (137) 2 days ago'))).toBe(137);
		expect(exitCodeOf(parseStatus('Exited (0) 2 days ago'))).toBe(0);
		expect(exitCodeOf(parseStatus('Restarting (1) 5 seconds ago'))).toBe(1);
		expect(exitCodeOf(parseStatus('Up 2 hours'))).toBeUndefined();
		expect(exitCodeOf(parseStatus('?'))).toBeUndefined();
	});
});
