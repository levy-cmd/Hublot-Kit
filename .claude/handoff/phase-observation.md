---
branche: phase/observation
base: bf6ab69
de: Livaille An-nour (avec Claude)
le: 2026-10-10
repris_par:
verif: export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock; make test && make build
---

## Objectif
Exécuter le plan Observation (F6 logs en direct SSE, F7 radiographie, correctif tmpfs hérité) : `docs/superpowers/plans/2026-10-10-observation-logs-radiographie.md` (9 tâches, Q24–Q38 confirmées).

## Fait
- Plan relu deux tours (Opus + Codex), R1–R17 ; questions confirmées par l'utilisateur (Q34 `logs_unavailable`, Q27 commande masquée sans révélation, Q28 révélation permise en lecture seule).
- Tâches 1 à 3 complètes et relues (tmpfs sous un lien, `dockerx.Logs`, routes SSE et téléchargement).
- Tâche 4 (`internal/secrets`) : code + correctif tour 1 commités (`a3f097d`, `bf6ab69`). **Re-revue du correctif PAS faite.**
- Tâche 7 (onglet Logs, web) : commencée en parallèle dans un worktree, interrompue par la limite de session. Travail non vérifié poussé sur `origin/phase/observation-t7-wip` (`c084431`), basé sur `a6ffdeb`.

## Décisions
- Registre complet (arbitrages `Ruling:`, Minor reportés, obligations pour la tâche 9) : `.claude/handoff/phase-observation.registre.md`. Le relire avant tout.
- Tests de sécurité commités rouges avant le code (P6) ; mutations annulées par réédition, jamais `git checkout` (P5).
- Revues Codex : agent `codex:codex-rescue` en lecture seule (sans `--write`, sinon refus du classifieur).
- Parallélisme demandé par l'utilisateur : seulement des tâches aux fichiers disjoints (web contre Go), en worktree.

## Impasses
- Ne jamais lancer un conteneur avec un bind de `/proc`, `/sys` ou `/` de l'hôte puis un tar : dockerd se bloque.
- Ne pas fusionner la branche wip T7 telle quelle : non testée, coupée en plein milieu des textes i18n.

## Prochain geste
1. Re-revue ciblée du correctif T4 : diff `854d549..bf6ab69`, constats I1–I3 + Unicode (constats : `.claude/handoff/phase-observation.t4-revue.md`). Noter : `env PWD=/srv` désormais masqué (faux positif), limites §13 à écrire en T9.
2. Tâche 5 (radiographie API), puis 6, 8, 9 en séquence ; T7 : repartir de `phase/observation-t7-wip` (rebaser sur `phase/observation`, finir, vérifier, relire) ou la refaire.
3. Fin : revue finale de branche (Opus + Codex), PR vers `main`.

## Pièges
- Node par défaut = 20 : `export PATH=/opt/homebrew/bin:$PATH`.
- Disque du Mac ~91 % (≈18 Go) : pas d'image lourde.
- La T9 a une liste d'obligations de spec (§5.3, §5.4 b, §13) dans le registre (« POUR LA T9 »).
- Le worktree local `.claude/worktrees/` n'est pas versionné ; la branche poussée fait foi.

## Pré-requis
skills : econo, superpowers (subagent-driven-development), codex | DOCKER_HOST | Docker Desktop, Go 1.27, Node 24, gh, VM Lima hk-ubuntu / hk-fedora
