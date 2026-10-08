---
branche: phase/socle
base: b2cb4b1
de: levy (avec Claude)
le: 2026-10-08
repris_par:
verif: PATH=/opt/homebrew/bin:$PATH make test && make build
---

## Objectif
Phase Socle de Hublot-Kit : agent Go + interface SvelteKit embarquée, auth, audit, F1 (liste), page /servers, CI. Plan : `docs/superpowers/plans/2026-10-07-socle.md`.

## Fait
- Les 15 tâches du plan, chacune relue par un sous-agent + Codex — `git log --oneline 671e91a..b2cb4b1`
- Revue finale (opus) faite ; vague de correction finale appliquée et poussée (d6d54e8 CSP, c573bae dossier de données, 58fb3fe CI smoke, b2cb4b1 mineurs) — vérifié : `make test`, `make build`, Chrome headless affiche /login (input#token, lang=fr)
- Branche poussée sur origin ; aucune PR ouverte (`gh` absent)

## Décisions
- Journal complet des décisions (« Ruling: ») et mineurs reportés : `.superpowers/sdd/2026-10-07-socle/progress.md` (local, ignoré par git) — à recopier dans le message final avant de supprimer ce dossier
- CSP : empreintes sha256 des scripts inline d'index.html calculées au démarrage (`internal/web`), `style-src 'unsafe-inline'` accepté — parce que SvelteKit démarre par un script inline (page blanche sinon)
- Dossier de données : créé en 0700 ; existant inscriptible par groupe/autres → refus de démarrer — parce que chmod forcé cassait /tmp en root
- SvelteKit 3 : config dans `web/vite.config.ts`, imports `#lib/….ts` (pas de `$lib`), Node ≥ 22.17 — imposé par l'outillage réel
- Actions GitHub en v7 (dernières majeures au 2026-10-08)

## Impasses
- `svelte.config.js` : refusé par SvelteKit 3 (config_file_unsupported)
- Règle « point dans le dernier segment → 404 » dans le handler statique : cassait /c/web.1 ; remplacée par une liste d'extensions connues + exemption de c/
- Journaliser chaque 429 : inondait le journal d'audit ; une ligne par IP et par minute

## Prochain geste
1. Re-revue ciblée de la vague finale : paquet `.superpowers/sdd/2026-10-07-socle/review-9be5099..b2cb4b1.diff`, constats et décisions dans `final-review-findings.md` du même dossier ; un relecteur (opus) + Codex. Si propre → ligne « FINAL fix wave reviewed » dans progress.md.
2. Skill `superpowers:finishing-a-development-branch` : la base est `main` ; la PR spike/semaine-1 → main doit passer d'abord. Proposer les 3 options, puis donner les « Rulings » de progress.md dans le message final, et seulement après supprimer `.superpowers/sdd/2026-10-07-socle/`.

## Pièges
- Node par défaut du shell = 20 : toujours `PATH=/opt/homebrew/bin:$PATH`
- La CI ne tourne que sur PR ou push sur main : le test de fumée du binaire n'a encore jamais tourné sur GitHub
- Codex fait souvent de fausses alertes ici : vérifier dans le code avant d'appliquer

## Pré-requis
skills : superpowers (subagent-driven-development, finishing-a-development-branch), codex | variables d'env : aucune | services : Docker Desktop (DOCKER_HOST=unix://$HOME/.docker/run/docker.sock pour les tests d'intégration), Go 1.27, Node 24, Chrome
