# Registre de la phase v0.2

- Spec (complément de la v0.1) : `docs/superpowers/specs/2026-10-23-hublot-kit-v0.2-design.md`.
- Plan : `docs/superpowers/plans/2026-10-23-v0.2.md`.
- Branche de phase : `phase/v0.2`, depuis `origin/main` (6b30eab, v0.1.1 comprise). Tranches sur `phase/v0.2-tN`.

## Décisions de l'utilisateur (2026-10-23)
- Disposition de sa maquette ; identité « Graphite & Émeraude », sombre par défaut, clair assorti, bascule mémorisée.
- Fonctions :
  - recherche globale (Ctrl+K) ;
  - filtre par projet Compose ;
  - labels dans le détail ;
  - chiffres clés ;
  - page Images, fichiers compris, en lecture seule ;
  - page Snapshots (manifeste, sans contenu) ;
  - page Secrets (noms seulement, révélation existante).
- La tranche 1 « Quai » (c963718, `origin/phase/interface`) est abandonnée. Ce qui en est repris est listé en spec v0.2 §4.4. La branche reste en place.

## Rédaction (2026-10-23)
- Mesures faites avant rédaction, sans rien écrire sur le disque (scripts hors du dépôt) :
  - format de `GET /images/{id}/get` : Docker 29.7.2 (Docker Desktop) et 29.8.2 (hk-ubuntu), magasin containerd dans les deux cas. Blobs avant `manifest.json`, couches en gzip ;
  - export, décompression et lecture des en-têtes : de 0,1 s (`alpine:3.20`, 517 entrées) à 6,8 s (`keycloak:26.3`, 11 162 entrées) ; `golang:1.27` en 5,2 s, 37 224 entrées, 900 Mo décompressés ;
  - HMAC-SHA256 de 900 Mo : 0,1 s de plus que sans HMAC ;
  - `Inspect` avec `size=true` : de 11 à 16 ms, contre 2 à 9 ms sans ; liste avec `size=1` : 565 ms contre 69 ms (39 conteneurs) ;
  - `GET /images/json` : 172 ms.
- hk-ubuntu démarrée pour une mesure (export de `hublot-test` en 2,4 s), puis arrêtée. Aucune VM n'a été recréée.
- Contrastes AA calculés pour tous les jetons (spec v0.2 §4.2). Deux jetons corrigés en cours de calcul :
  - `--trait-fort` en sombre (`#5B6773` à 2,99:1 → `#6B7784`) ;
  - `--emeraude-texte` en clair (`#047857` à 4,50:1 sur la sélection → `#036B4D`).
  
  Exception documentée : le remplissage émeraude n'est jamais seul sur une surface claire (2,40:1).
- Questions ouvertes : Q55 à Q68 (spec v0.2 §0). Le plan applique les recommandations en attendant les réponses.

## Points à faire trancher ou relire
- Q59 corrige une hypothèse de la demande : sans contenu conservé, même « contre le conteneur actuel », il n'y a pas de diff ligne à ligne. L'exception est le cas où l'ancienne empreinte égale celle de la version de l'image.
- Q56 : la maquette dit « sécurisé par défaut », or `--read-only` vaut `false` par défaut. L'encart affichera l'état réel.
- Q55 : F8 à F10, promis pour la v0.2 par la spec v0.1, sont reportés si l'utilisateur confirme.
- Non vérifié :
  - le format de `docker save` sous le pilote overlay2 (CI, tranche 6) ;
  - le coût d'un snapshot d'un conteneur actif, et la taille réelle d'un manifeste (VM, tranche 8).

## Contrat de parallélisme
Néant pour l'instant : les tranches sont séquentielles. Avant tout parallèle (4 avec 3, 8 avec 5 et 6), écrire ici les fichiers possédés de chaque côté, en s'appuyant sur le découpage de l'i18n par module et sur `nav.ts` (plan, « Ordre, branches et parallélisme »).

## Tranches
(à remplir : commits, vérifications, revues, arbitrages)
