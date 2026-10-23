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
- Q57 (reformulée après la revue, puis reposée) : choix (a). Prise et suppression d'une photo permises, y compris en `--read-only`, après une confirmation qui affiche leur coût. Plus aucune question ouverte.

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

## Dernière retouche (2026-10-23)
- Le relecteur conclut « plan prêt ». Codex garde un point, traité : les en-têtes PAX. `archive/tar` de Go 1.27 plafonne chaque en-tête spécial à `maxSpecialFileSize = 1 << 20` (`readSpecialFile`, `io.LimitReader`), ce qui borne la mémoire. Une chaîne d'en-têtes dans un seul `Next()` est arrêtée par le lecteur compteur placé sous `archive/tar` (contexte et plafonds testés à chaque `Read`). Tests ajoutés : `TestImageIndexHugePAXHeader` et `TestImageIndexPAXChain`.
- Minor du relecteur traités :
  - `secrets.History` remplace `secrets.Line` dans la spec §7, §9 et dans les Global Constraints du plan ;
  - la tranche 9 consomme `Store.Reader` ;
  - contrat 8/6 ci-dessus ;
  - `TestWalkNoAtime` vieillit l'atime avant la lecture ;
  - l'interface relance une extraction toutes les 2 s (tranche 6).

## Points à faire trancher ou relire
- Q59 corrige une hypothèse de la demande : sans contenu conservé, même « contre le conteneur actuel », il n'y a pas de diff ligne à ligne. L'exception est le cas où l'ancienne empreinte égale celle de la version de l'image.
- Q56 : la maquette dit « sécurisé par défaut », or `--read-only` vaut `false` par défaut. L'encart affichera l'état réel.
- Q55 : F8 à F10, promis pour la v0.2 par la spec v0.1, sont reportés si l'utilisateur confirme.
- Non vérifié :
  - le format de `docker save` sous le pilote overlay2 (CI, tranche 6) ;
  - le coût d'un snapshot d'un conteneur actif, et la taille réelle d'un manifeste (VM, tranche 8).

## Contrat de parallélisme
- Tranches 8 et 6 (si elles partent en parallèle) :
  - `internal/fsview/views.go` appartient à la tranche 6 ;
  - la tranche 8 le lit sans l'écrire : elle appelle `(*Views).Cached(info dockerx.ContainerInfo) (FS, bool)`, livrée par la tranche 2, pour la photo des métadonnées (et `CachedCount(info) (n int, at time.Time, ok bool)` pour le comptage) ;
  - contrat révisé à la revue de la tranche 2 (le plan donnait un ID seul, ce qui ne permet pas de vérifier la clé `ID + FinishedAt` sans un `Inspect` caché) : **l'appelant passe un `Inspect` qu'il vient de faire**, car `newTarFS` reprend `info.Mounts` et `info.Tmpfs`, et la clé `info.ID + info.FinishedAt` garantit que l'index décrit l'état présent ; `false` pour un conteneur actif, un index jamais servi, en cours ou en échec ;
  - si la tranche 6 change `Cached` (signature ou comportement, avec le cache partagé avec les images), elle l'écrit ici avant la fusion, et la tranche 8 se rebase ;
  - `internal/fsview/fsview.go` appartient à la tranche 8.
- **Contrat de la recherche (tranche 3, révisé à la revue)** : `(*Views).SearchCached(ctx context.Context, info dockerx.ContainerInfo, q string, limit int) (hits []SearchHit, truncated, indexed bool)` remplace `SearchCached(key string, …)` du plan.
  - Même clé que `Cached(info)` (ID complet et `FinishedAt`). L'appelant passe un `Inspect` qu'il vient de faire. Seul un index déjà servi est lu, sans construction ni visite.
  - `(*Views).Indexing(ref string) bool` : une construction en cours de `ref` (ID complet ou nom, jamais un préfixe), sans visite (contrairement à `Pending`).
  - La clé `"image:" + ID` du plan n'existe plus. La tranche 6 ajoute dans `search.go` `(*Views).SearchImage(ctx, imageID string, q string, limit int) (hits []SearchHit, truncated, indexed bool)` sur son cache d'images (index déjà servi seulement, jamais d'export), qui réutilise `(*index).search`, et la route `POST /api/images/{id}/fs/search` (corps `{"q"}`).
  - Route : `POST /api/containers/{id}/fs/search`, corps `{"q": "…"}`, sous le contrôle d'Origin ; la requête n'est jamais dans une URL.
- Autres paires : séquentielles par défaut. Avant tout parallèle (4 avec 3, 8 avec 5 et 6), écrire ici les fichiers possédés de chaque côté, en s'appuyant sur le découpage de l'i18n par module et sur `nav.ts` (plan, « Ordre, branches et parallélisme »).

## Tranches
(à remplir : commits, vérifications, revues, arbitrages)

## Suivi d'exécution (session de coordination)
- **Spec et plan v0.2** : c5ab9c0, puis fa75747 et 023747d. Validés par le relecteur et par Codex après deux correctifs : sécurité du parseur de couches, snapshots, coûts, historique `|N`, PAX, propriété de `server.go` et `fserr.go`.
  - Décisions de l'utilisateur : Q55 (F8 à F10 reportées), Q56 (l'état réel de la lecture seule est affiché, ce n'est pas le défaut), Q57 (snapshot permis en lecture seule après confirmation du coût), Q59 (sans contenu), Q60 (labels en liste blanche) ; les autres questions suivent la recommandation.
- **Tranche 1 (coquille Graphite & Émeraude)** : 9660cbd (tests rouges), 56e2af7, bfe5efe (tests rouges), 809f624. Validée par le relecteur et par Codex ; `make test`, `e2e` et `make build` verts.
  - Correctif de revue : sous 90 rem, un panneau du bas ne se monte qu'à sa première ouverture. Sinon, un panneau caché lançait un `docker export` d'un conteneur arrêté : 4 Gio, 5 min, conteneur verrouillé.
  - Aussi : contrôle i18n étendu ; `connected` égal à `Ping` ; liste avec Ports et noms prioritaires ; commande limitée à 3 lignes ; hauteur des panneaux.
  - Ruling: le point de Codex (sous 90 rem, un panneau déjà ouvert reste monté quand on le cache, et son flux de logs continue) passe en tête de la tranche 2. Le flux de logs doit se couper quand son panneau est caché, pour libérer une des 8 places.
  - Minor reporté : le test i18n ne vérifie que le préfixe des clés en gabarit (`container.tab.${k}`).
  - Reste pour la clôture : refaire les captures `docs/images/` (README encore en v0.1) et aligner `V=0.1.0` dans la doc d'installation.
- **Tranche 2 (projets Compose, labels, chiffres clés)** : 9e2b362 (flux de logs coupé quand son panneau est caché), 4500e64 (tests rouges), 364b960, ecb5695 ; correctif de revue : f0b78bc (tests rouges), puis le correctif.
  - Écarts au plan : contrat de `Cached` révisé (ci-dessus) ; dernière sortie lue dans `state.finishedAt` (déjà présent, omis pour la date zéro) ; fichiers touchés hors liste : `routes/+page.svelte`, `containers.ts`, `i18n/containers.*`, `panels.ts`, `nav.ts`, `Sidebar.svelte`, `e2e/parcours.e2e.ts`.
  - Coûts mesurés (Docker Desktop, 40 conteneurs) : liste 98 ms, sans `size` ; `figures` 14 à 96 ms au premier appel, 3 ms en cache ; fichiers tirés de l'index déjà servi seulement.
  - Revue : le relecteur valide ; Codex demande de regrouper les `Inspect` avec `size` simultanés d'un même conteneur (fait, regroupement maison, un appel détaché borné par le délai de 30 s ; `TestFiguresSizeCoalesced`).
  - **Décision de la session principale (resserre Q60)** : les labels de Compose qui donnent des chemins de l'hôte (`com.docker.compose.project.config_files`, `.working_dir`, `.environment_file`) sont masqués comme un label hors liste blanche. Test témoin `TestLabelsHostPathsMasked` (`/home/alice/…`).
  - Retouches : les labels en clair en puces, les labels masqués repliés sur une ligne « N autres labels masqués » qui ne montre que leurs clés ; `getFigures` annulé au changement de conteneur ; l'e2e vérifie la reprise du flux avec `after=`.
  - Barre latérale : « Diff avec l'image » surligné sur la page Conteneurs n'était pas un survol. La détection marquait comme page courante toute entrée dont les paramètres étaient dans l'adresse (`?panel=diff`). Corrigé : une entrée de panneau (Diff, Logs) n'est jamais la page courante, Conteneurs l'est (`currentNav`, test dans `nav.test.ts`).
- **Tranche 2 (projets, labels, chiffres clés)** : 9e2b362, 4500e64 (tests rouges), 364b960, ecb5695, f0b78bc (tests rouges), 2b26234. Validée par le relecteur et par Codex.
  - Mesures : liste en 98 ms pour 40 conteneurs, sans tailles ; `figures` en 14 à 96 ms, puis 3 ms en cache.
  - Correctif de revue :
    - contrat `Cached(info)` consigné ;
    - calculs `ContainerSize` regroupés par ID complet ;
    - `getFigures` annulé au changement de conteneur ;
    - l'e2e vérifie `after=` à la reprise des logs ;
    - labels de la liste blanche en puces, les autres dans une ligne repliée (clés seules) ;
    - page active de la barre latérale corrigée (défaut réel : `?panel=diff`).
  - Ruling (session principale, resserre Q60) : les labels Compose qui portent des chemins de l'hôte (`project.config_files`, `project.working_dir`, `project.environment_file`) ont leur valeur masquée.
  - À faire en tête de la tranche 3 : un test du regroupement sur DEUX conteneurs (une clé commune n'est détectée par aucun test).
  - Minor reporté : une requête qui rejoint un calcul en cours reçoit 503 au lieu de 504 quand il expire.
- **Tranche 3 (recherche globale Ctrl+K)** : 2b1b75a (test du regroupement sur deux conteneurs), a335abd (tests rouges), 982bc90 ; correctif de revue : voir ci-dessous.
  - Écarts au plan :
    - contrat `SearchCached(ctx, info, …)` (ci-dessus) ;
    - pas de résultats « images » avant la tranche 5 (pas de page, donc pas de lien) ;
    - « NGINX » donne aussi le dossier `/etc/nginx` (chemin complet, spec §5.8) ;
    - fichiers hors liste : `nav.ts` (`action`), `Sidebar.svelte`, `nav.test.ts`, `web/e2e/palette.e2e.ts`, `ContainerDetail.svelte` (chiffres demandés par l'ID complet).
  - Mesures : recherche servie par l'agent en 2,5 à 3,3 ms (index réel) ; parcours complet 3,1 ms pour 37 224 entrées, 245 ms pour 1 000 000 (M4).
  - Revue : le relecteur valide ; Codex demande un correctif ; première CI (PR brouillon #10) rouge sur `TestFiguresTimeout`.
  - Correctif de revue :
    - 504 pour toute requête qui attend un calcul de taille expiré, qu'elle l'ait lancé ou rejoint (Minor de la tranche 2, cause de la CI rouge) ; délai du calcul séparé (`figuresCallTimeout`) pour des tests déterministes, `-count=50` vert ;
    - recherche en `POST` avec la requête dans le corps (Codex) ;
    - l'interface n'envoie que l'ID complet aux routes de fichiers, de recherche et de chiffres (`fullId`, figures par l'ID de la radiographie ; e2e sur `/c/<ID court>`). Pas de correspondance par préfixe côté agent (v0.1.1) : la limite d'un ID court pendant un export est écrite dans la spec et les limites ;
    - `Indexing` au lieu de `Pending` dans la recherche (pas une visite) ;
    - sections de la palette en `role="group"` nommés.
- **Tranche 3 (recherche Ctrl+K)** : 2b1b75a, a335abd (tests rouges), 982bc90, 5e83086 (tests rouges), 823a4d7. Validée par le relecteur et par Codex. CI de la PR brouillon #10 verte (run 39990583708), overlay2 compris.
  - PR brouillon #10 (`phase/v0.2` → `main`) ouverte le 2026-10-23 par la session principale, UNIQUEMENT pour que la CI tourne. Ne pas la fusionner, ne pas la sortir du brouillon avant la clôture.
  - Correctif de revue :
    - un délai dépassé donne toujours 504 (cause de l'instabilité en CI overlay2) ;
    - recherche en POST, la requête dans le corps (jamais dans une URL ni dans les journaux d'un proxy) ;
    - l'interface n'envoie que l'ID complet (`fullId`) ;
    - `Indexing` ne compte pas comme une visite ;
    - sections de la palette en groupes nommés.
  - Limite documentée : un appel à l'API avec un ID court pendant un export attend le verrou de dockerd. Pas de correspondance par préfixe, par sécurité.
  - À faire en tête de la tranche 4 (Minor de Codex) : `fs_search.go:63` doit refuser toute donnée après le premier objet JSON (second `Decode` égal à `io.EOF`).
