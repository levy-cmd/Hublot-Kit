---
branche: phase/explorateur
base: 58990fc
de: Livaille An-nour (avec Claude)
le: 2026-10-09
repris_par:
verif: export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock; make test && make build
---

## Objectif
Exécuter le plan 1/3 de la phase Explorateur : explorer et télécharger les fichiers d'un conteneur **actif** (F2, F3) via `/proc/<pid>/root`. Plan : `docs/superpowers/plans/2026-10-08-explorateur-1-procfs.md` (16 tâches, Task 1 à Task 16).

## Fait
- Spike (PR #1) et Socle (PR #2) fusionnés dans `main` ; CI GitHub verte (go, web, integration, binary).
- Plan 1 écrit puis relu trois tours (sous-agent opus + Codex) : section « Décisions de revue » en tête du plan.
- Q1 à Q9 confirmées par l'utilisateur le 2026-10-09 (toutes les recommandations retenues).
- Aucune ligne de code de l'Explorateur n'est encore écrite.

## Décisions
- Toutes dans le plan : « Décisions de revue » (C1, I1–I4, R1–R6, T1–T10) et « Questions ouvertes » (Q1–Q9). Les relire avant chaque tâche concernée, ne pas les rediscuter.
- Exécution : skill `superpowers:subagent-driven-development`, une tâche à la fois ; chaque tâche relue par un sous-agent indépendant **et** Codex (`codex:codex-rescue`) avant la suivante (CLAUDE.md).
- Codex fait de fausses alertes ici : vérifier chaque constat dans le code avant de l'appliquer.

## Impasses
- Lancer un conteneur avec un bind de `/proc`, `/sys` ou `/` de l'hôte, puis un `GetArchive` de dossier : bloque dockerd et verrouille le conteneur (vécu pendant la revue). Ne jamais le refaire contre le vrai Docker ; tester C1 avec le faux client.
- `timeout` n'existe pas sur macOS : pour tester si Docker répond, lancer en arrière-plan et tuer après 10 s.

## Prochain geste
1. `/pickup`, puis invoquer `superpowers:subagent-driven-development` sur le plan, en commençant à Task 1.
2. Ne pas pousser entre Task 4 (tests d'évasion rouges) et Task 5 : la CI `go` serait rouge.
3. Task 16 : passage sur les VM Lima `hk-ubuntu` et `hk-fedora` (`limactl start …`), rejouer la mutation `pidfd_send_signal` de `CheckArchive` et relever les lignes `MESURE` (Q8, R3).
4. Fin du plan : PR `phase/explorateur` → `main` avec `gh` (installé et connecté), retirer cette passation.

## Pièges
- Node par défaut du shell = 20 : toujours `export PATH=/opt/homebrew/bin:$PATH`.
- Le disque du Mac a saturé pendant la revue (Docker a planté) : surveiller `df -h /System/Volumes/Data`, faire nettoyer le scratchpad par les sous-agents.
- Docker garde l'image `hublot-test` et les volumes `hublot-gocache` / `hublot-gomod` : réutilisés par `scripts/test-linux.sh` (créé en Task 4).
- Le code Linux (`/proc`, `openat2`, pidfd) ne se teste pas sous macOS : conteneur golang en root via `scripts/test-linux.sh`, ou VM Lima.

## Pré-requis
skills : superpowers (subagent-driven-development, finishing-a-development-branch), codex | variables d'env : DOCKER_HOST=unix://$HOME/.docker/run/docker.sock | services : Docker Desktop, Go 1.27, Node 24, Chrome, gh, VM Lima hk-ubuntu / hk-fedora (Task 16)
