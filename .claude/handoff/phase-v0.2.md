---
branche: phase/v0.2
base: 371ccdb
de: Livaille An-nour (session Claude principale)
le: 2026-10-24
repris_par:
verif: make test
---

## Objectif
Publier la v0.2 (interface Graphite & Émeraude, Secrets, Images, diff, Snapshots) : plan `docs/superpowers/plans/2026-10-23-v0.2.md`, suivi `.claude/handoff/phase-v0.2.registre.md`.

## Fait
- Tranches 1 à 9 validées (relecteur + Codex), consignées au registre.
- Tranche 10, étapes 1 à 3 : test de lecture seule côté Docker, specs et doc à jour, gitleaks à 0, revue de toute la branche « prête pour la rc ». CI PR #10 verte sur 371ccdb.
- v0.1.2 publiée (correctifs d'audit), fusionnée dans `main` puis dans `phase/v0.2`.

## Décisions
- Pas de v0.1.3 : les trous restants de la v0.1.2 exigent une session et sont corrigés dans la v0.2.0.
- Chaque étape extérieure (fusion dans `main`, tags, releases) attend l'accord de l'utilisateur.

## Impasses
- Les revues Codex sont en lecture seule : trois de leurs constats se sont révélés faux à la vérification (registre, tranches 7, 9 et report v0.1.2). Toujours vérifier dans le code avant d'agir.
- Contrôler des numéros calculés ne prouve pas leur rendu : la gouttière du diff n'a été prise qu'en Chromium.

## Prochain geste
1. Étape 4 : PR #10 hors brouillon, fusion dans `main` (accord de l'utilisateur).
2. Étape 5 : `v0.2.0-rc.1` (procédure de la tâche 9 du plan Lancement), vérification anonyme.
3. PR de bascule de la doc d'installation sur 0.2 (registre, tranche 10), puis étape 6 : `v0.2.0`.

## Pièges
- Le dépôt principal reste sur `phase/interface` ; registre et handoff s'écrivent par un worktree détaché, `set -e`, HEAD vérifié.
- Une démo locale tourne (`hublot-v02-demo`, port 7878, worktree `scratchpad/demo-v02`) : à arrêter avec l'accord de l'utilisateur.
- cosign n'est installé ni sur le Mac ni dans hk-ubuntu : la vérification anonyme télécharge la v3.1.3 officielle dans un dossier temporaire.

## Pré-requis
skills : econo, codex | variables d'env : PATH (Homebrew), DOCKER_HOST | services : Docker Desktop, Lima, `gh` authentifié
