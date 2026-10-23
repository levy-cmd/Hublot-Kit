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

## Décisions de l'utilisateur sur les questions (2026-10-23)
- Q55 : F8 à F10 reportées après la v0.2.
- Q56 : état réel affiché ; la lecture seule n'est pas le défaut.
- Q59 : snapshots sans contenu ; pas de diff ligne à ligne entre deux photos.
- Q60 : liste blanche des labels ; les autres affichent leur nom avec une valeur masquée, sans révélation.
- Q58, Q61 à Q68 : recommandation adoptée.
- Q57 reste ouverte, reformulée (spec v0.2 §0.2) : le coordinateur la repose à l'utilisateur.

## Revue du plan (2026-10-23)
- Codex : « plan prêt ». Le relecteur demande un correctif avant les tranches 6 et 8. Il est intégré dans la spec et dans le plan, tests compris.
- Constats Important corrigés :
  1. Entrée creuse : index par `Next()` seul, contexte testé à chaque tour, `TestImageIndexSparseBomb`.
  2. Mémoire : noms et cibles de 4 Kio au plus, 256 Mio de noms au total, `PAXRecords`, `Xattrs`, `Uname` et `Gname` jetés.
  3. Extraction et `image-match` :
     - plafonds (4 Gio, 16 Gio décompressés, 2 min), deux extractions au plus pour l'agent, annulation après 15 s sans client ;
     - le rang de l'entrée remplace son nom ;
     - l'empreinte est vérifiée en lisant jusqu'à la fin du blob utile, et pas au-delà.
  4. Fusion des couches : un dossier remplacé perd ses descendants ; `.wh.`, `.wh..` et `.wh...` sont ignorés. Tests ajoutés.
  5. Parcours d'un conteneur actif : `openNoLinks` (RESOLVE_NO_SYMLINKS) et `O_NOATIME` ; `TestWalkLinkRace`.
  6. Coûts écrits :
     - un export lit aussi les volumes et verrouille le conteneur arrêté ;
     - une photo d'un conteneur actif n'est pas atomique ;
     - les octets lus sont plafonnés et le pic mémoire est chiffré.

     Plafonds ramenés à 500 000 entrées et 1 Gio haché ; export d'un conteneur arrêté plafonné à 4 Gio et 5 min. Ajout d'une photo des métadonnées tirée de l'index déjà construit. Q57 reformulée.
  7. Historique d'image : `secrets.History` masque toutes les valeurs après `|N` (`GH_PAT`, `NPM_AUTH`).
  8. Conflits de fichiers :
     - la tranche 1 crée des points d'extension (`moduleRoutes` et `routes_<module>.go`, codes d'erreur réservés, `nav/` et `i18n/` par module) : `server.go` et `fserr.go` n'ont plus qu'un écrivain ;
     - la tranche 9 dépend aussi de la 7.
- Minor traités :
  - clé HMAC : `linkat`, exactement 32 octets, rotation `hublot rotate-snapshot-key`, `snapshots/` ouvert sans suivre de lien et testé ;
  - réponse 202 de la prise à la forme de `indexing` ;
  - mesure du HMAC refaite : 0,31 s pour 900 Mio seul ; la mesure combinée est trop bruitée (3,9 à 5,4 s) pour isoler l'écart, et l'ancienne phrase « +0,1 s » est retirée ;
  - ustar à l'octet 257, avec test de couche non compressée ;
  - tests rouges des tranches 3 et 9 commités seuls.

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
