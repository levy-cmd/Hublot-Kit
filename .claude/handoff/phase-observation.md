---
branche: phase/observation
base: bf6ab69
de: Livaille An-nour (avec Claude)
le: 2026-10-10
repris_par: Claude (session du 2026-10-10, close le 2026-10-12)
verif: export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock; make test && make build
---

## Objectif
Exécuter le plan Observation (F6 logs en direct SSE, F7 radiographie, correctif tmpfs hérité) : `docs/superpowers/plans/2026-10-10-observation-logs-radiographie.md` (9 tâches, Q24–Q38 confirmées).

## Fait
- Phase terminée (2026-10-12) : tâches 1 à 9 faites, chacune relue par un sous-agent et par Codex, correctifs compris.
- T6 a révélé des doublons à la reprise des logs : l'ID devient `ts/flux/rang`, et le saut va jusqu'à la ligne de coupure (voir le registre).
- T7 et T8 ont été faites en worktree, puis fusionnées (c7258ff, f55d3ca).
- Vérification T9 : hk-ubuntu (AppArmor) et hk-fedora (SELinux Enforcing) donnent 387 PASS chacune. Docker Desktop donne 386 PASS et 1 SKIP prévu. Aucun AVC.
- Revue finale de branche (Opus + Codex) : « prête pour la PR ». Les Minor de documentation sont corrigés (7f08218).

## Décisions
- Registre complet (arbitrages `Ruling:`, Minor reportés) : `.claude/handoff/phase-observation.registre.md`.
- Tests de sécurité commités rouges avant le code (P6) ; mutations annulées par réédition, jamais `git checkout` (P5).
- Revues Codex : agent `codex:codex-rescue` en lecture seule (sans `--write`).

## Impasses
- Ne jamais lancer un conteneur avec un bind de `/proc`, `/sys` ou `/` de l'hôte suivi d'un tar : dockerd se bloque.

## Prochain geste
1. Relire et fusionner la PR `phase/observation` → `main` (humain).
2. Garder la branche après la fusion. Supprimer seulement les branches de travail `phase/observation-t7-wip`, `-t7` et `-t8`, si l'humain le veut.

## Reports (hors phase)
- Golden de la radiographie incomplet : `state.error`, `startedAt`, `health`, `entrypoint` non vide, et `ip`/`gateway`/`aliases` d'un réseau. Ces champs sont vérifiés seulement par le type TS.
- Pas de test de composant Svelte (pas de jsdom) : les branchements de `LogsView` et `XrayView` sont vérifiés par lecture.
- Préfixes de commits non uniformes (`docs:` et `docs :`) : cosmétique.

## Pièges
- Node par défaut = 20 : `export PATH=/opt/homebrew/bin:$PATH`.
- Disque du Mac à environ 91 % : pas d'image lourde.

## Pré-requis
skills : econo, superpowers (subagent-driven-development), codex | DOCKER_HOST | Docker Desktop, Go 1.27, Node 24, gh, VM Lima hk-ubuntu / hk-fedora
