# Hublot-Kit

Explorer l'intérieur des conteneurs Docker d'un serveur depuis un navigateur, sans ligne de commande.

> **Sécurité : installer Hublot-Kit revient à lui donner les droits root sur l'hôte.**
> Il parle au socket Docker et lit les fichiers des conteneurs. Gardez-le sur `127.0.0.1`
> et accédez-y par un tunnel SSH ou un reverse proxy HTTPS.

État : en développement (v0.1). Spec : `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md`.

## Développement

Prérequis : Go 1.27+, Node 24 (≥ 22.17), Docker.

    make test        # tests Go + interface
    make build       # interface compilée, puis bin/hublot

Lancer l'agent sur macOS (Docker Desktop) :

    DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go run ./cmd/hublot --data-dir ./.data

Le jeton d'accès s'affiche une seule fois au premier lancement ; `go run ./cmd/hublot reset-token --data-dir ./.data` en crée un nouveau.

Interface en mode développement (proxy vers l'agent sur 127.0.0.1:7878) :

    cd web && npm run dev

## Notes d'exploitation

- Le dossier `--data-dir` est forcé en 0700 au démarrage : utilisez un dossier dédié.
- Après `hublot reset-token`, relancez l'agent : l'ancien jeton reste accepté par un agent déjà lancé.
- Derrière un reverse proxy HTTPS, renseignez `--public-origin`.

## Licence

MIT
