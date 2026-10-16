---
branche: phase/lancement
base: 559ab12
de: Livaille An-nour (avec Claude)
le: 2026-10-16
repris_par:
verif: export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock; make test && make build && bash scripts/check-docs.sh
---

## Objectif
Publier la v0.1 de Hublot-Kit : plan `docs/superpowers/plans/2026-10-13-lancement.md`, tâche 10, étapes 5 à 11.

## Fait
- Tâches 1 à 9, et étapes 1 à 4 de la tâche 10, faites et validées (relecteur + Codex) — journal complet dans `.claude/handoff/phase-lancement.registre.md`.
- `main` = b8e11da (PR #6 fusionnée). Tag `v0.1.0` sur b8e11da ; run 38910819137 vert, terminé par `RELEASE OK : v0.1.0`.
- Image `ghcr.io/levy-cmd/hublot-kit` : étiquettes `0.1.0` et `0.1`, plus les `sha256-…` de signature, sans `latest` ; vérifiée sur hk-ubuntu et hk-fedora.
- **Tout est encore privé** : release `v0.1.0` en brouillon, paquet GHCR privé, dépôt privé. L'utilisateur a répondu « pas encore » au passage en public.

## Décisions
- La publication se lance seulement par `workflow_dispatch`, derrière `guard`. Les jobs à droits revérifient le propriétaire et le tag, parce que « Re-run failed jobs » ne repasse pas par `guard`.
- `GORELEASER_CURRENT_TAG` est passé à GoReleaser, sinon un tag `v0.1.0` posé sur le commit de la rc construisait `0.1.0-rc.1`.
- Visibilité : on a nettoyé le léger. Le lien claude.ai est retiré de la spec et 18 branches de travail sont supprimées. Restent visibles l'historique, `.claude/` et la méthode.
- Le brouillon de la rc sera supprimé à l'étape 9 (déjà accepté).
- Reprise des logs : « ni doublon, ni perte silencieuse ». Une ligne dont le début est perdu est marquée tronquée.

## Impasses
- Mutations qui affaiblissent la sécurité dans le dépôt (retirer `--cgroupns=host`, identité en `refs/heads/`) : refusées par le classifieur. Les jouer sur une copie, ou par un faux journal.
- Environnement GitHub à relecteur obligatoire : impossible sur un dépôt privé en offre Free. D'où le `guard`.
- Épingler par empreinte les actions de GitHub (`checkout`, `setup-*`) : écarté par Q40. Codex le redemande à chaque revue ; c'est déjà arbitré.

## Prochain geste
Plan, tâche 10, étape 5 : demander l'accord explicite de l'utilisateur pour rendre le dépôt public et activer le signalement privé des vulnérabilités. Ensuite :
- étape 6 : publier la release (ARRÊT) ;
- étape 7 : l'utilisateur rend le paquet GHCR public dans l'interface, faute d'API ;
- étape 8 : vérification anonyme ;
- étape 9 : supprimer le brouillon de la rc ;
- étapes 10 et 11 : PR de constats sur `main`, relue.

## Pièges
- Toujours vérifier la branche du dépôt principal après un agent qui crée des branches : on l'a retrouvé une fois sur `ci/release-dispatch`.
- La CI d'une PR construit le commit de fusion, pas la tête de branche. La version d'un snapshot porte donc cette empreinte.
- `ubuntu-latest` passe à Ubuntu 26 le 2026-10-19. Si un nouveau build est nécessaire, relancer d'abord la CI.
- Codex ne lance pas les tests (bac à sable sans répertoire temporaire) : exécuter soi-même ce qu'il n'a fait que lire.
- Node par défaut en version 20, d'où le `PATH` Homebrew. Disque du Mac à environ 91 % : pas d'image lourde.

## Pré-requis
skills : econo | outils : gh (avec `read:packages`), Docker Desktop, Go 1.27, Node 24, VM Lima hk-ubuntu / hk-fedora (cosign v3.1.3 dans hk-ubuntu) | agents : `.claude/agents/implementeur.md`, `relecteur.md`, `codex:codex-rescue` (lecture seule).
