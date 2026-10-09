---
branche: phase/explorateur-2
base: d1ee349951b64868f52a5c501ff0cc2eb765e32f
de: Livaille An-nour (avec Claude)
le: 2026-10-09
repris_par:
verif: export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock; make test && make build
---

## Objectif
Exécuter le plan 2/3 de la phase Explorateur : index d'en-têtes tar pour les conteneurs arrêtés, cache et `202 indexing`, F5 (`GET /changes`, onglet Changements), plus trois correctifs reportés du plan 1 (destination sous un lien par identifiant de montage, vivacité par `poll` sur le pidfd, `testing` hors du binaire). Plan : `docs/superpowers/plans/2026-10-09-explorateur-2-tarindex-diff.md` (13 tâches).

## Fait
- Plan 1/3 fusionné dans `main` et poussé ; CI verte (go, web, integration, binary).
- Plan 2/3 écrit puis relu en deux tours (sous-agent Opus/Sonnet + Codex) : section « Décisions de revue » R1–R24.
- Q10 à Q13 confirmées par l'utilisateur le 2026-10-09 (recommandations retenues).
- Le code Go du plan n'a PAS été compilé : la première tâche qui le touche le vérifie.

## Décisions
- Toutes dans le plan (« Décisions de revue », « Questions ouvertes »). Ne pas les rediscuter.
- Toujours en mode econo et tout en sous-agents (mémoire du projet) ; chaque tâche relue par un sous-agent et par Codex (CLAUDE.md).
- Codex fait de fausses alertes : vérifier chaque constat avant de l'appliquer.

## Impasses
- Ne jamais lancer un conteneur avec un bind de `/proc`, `/sys` ou `/` de l'hôte puis un tar de dossier : dockerd se bloque.
- L'extension Chrome peut être déconnectée : piloter Chrome sans tête par CDP depuis Node 24 (WebSocket intégré) a marché pour l'essai d'interface.

## Prochain geste
1. `/pickup`, puis `superpowers:subagent-driven-development` sur le plan, à partir de la tâche 1.
2. Ne pas pousser entre les tâches 2 et 3 (tests rouges).
3. Fin du plan : revue finale, PR ou fusion selon le choix de l'utilisateur, retirer cette passation.

## Pièges
- Node par défaut = 20 : toujours `export PATH=/opt/homebrew/bin:$PATH`.
- Disque du Mac ~92 % : surveiller `df -h /System/Volumes/Data` (VM Lima et images).
- La CI ne tourne que sur une PR ou un push sur `main`.

## Pré-requis
skills : econo, superpowers (subagent-driven-development, finishing-a-development-branch), codex | DOCKER_HOST=unix://$HOME/.docker/run/docker.sock | Docker Desktop, Go 1.27, Node 24, gh, VM Lima hk-ubuntu / hk-fedora
