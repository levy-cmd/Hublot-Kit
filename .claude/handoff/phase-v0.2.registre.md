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
- **Tranche 4 (page Secrets)** : 166be92 (test rouge), 0864588 (`decodeOne`, Minor de la tranche 3), 9ab9885 (tests rouges), a15e357, 1f42e4f ; correctif de revue : e747222 (tests rouges), 14945db.
  - Écarts au plan :
    - bornes ajoutées : 15 s et 1 000 conteneurs par parcours, sémaphore de 4 Inspect pour tout l'agent ;
    - tri : env + arguments comptent comme secrets, les labels masqués viennent ensuite (la liste blanche masque tous les labels) ; indisponibles en dernier ; valeur révélée oubliée aussi au repli de la ligne et à l'actualisation ;
    - fichiers hors liste : `nav.test.ts` (une ligne), `scripts/e2e.sh` (variables du conteneur `hublot-e2e`), `web/e2e/secrets.e2e.ts`.
  - **Contrat révisé à la revue** (Codex, critique) : `args` vaut `[{part, index, kind, name?}]`, jamais le texte masqué. Un masque partiel de la spec v0.1 §13 (`Password=••••••(üü)`, `--token` suivi de JSON) laisserait sortir un reste de la valeur. `name` n'est tiré que d'un drapeau ou d'une clé faits de `[A-Za-z0-9_.-]`, ou du shell et de son drapeau. Spec v0.2 §5.9 et plan à jour.
  - Correctif de revue :
    - noms de variables dédoublonnés dans l'agent, clé par index dans la page (relecteur : `each_key_duplicate` avec `API_TOKEN` en double ; e2e avec la variable en double) ;
    - inventaires simultanés regroupés : un seul parcours détaché, chaque appelant avec son délai (Codex) ;
    - erreur de `ListContainers` : `503 docker_unavailable`, détail fixe, journal par `secrets.Line` (Codex) ;
    - `revealEnv` passe par `decodeOne`.
  - Mesures (Docker Desktop, 42 conteneurs) : 66 à 182 ms ; un conteneur verrouillé par un `GET /archive?path=/` lu lentement est `unavailable` en 3,1 s, les autres complets.
  - Mutations : toutes tuées, sauf une, **acceptée** : le second `ctx.Err()` après l'acquisition d'une place (`secrets_inventory.go`). Il ne ferme qu'une course étroite (une place libérée à l'instant du délai, que `select` choisit au hasard) : le relecteur la juge quasi équivalente. Le contrôle reste ; il a corrigé un vrai défaut vu une fois sous `-race` (8 Inspect après le délai).
  - Second correctif (relecteur, critique) : `name` reprenait l'argument précédent sans vérifier que le masquage l'avait laissé intact (`["app","--password=","--Tok3n_SECRET_xyz","x"]` donnait `name:"--Tok3n_SECRET_xyz"`). Désormais un nom ne vient que d'un texte resté en clair : drapeau précédent intact, shell et son drapeau intacts, clé `NOM=` présente dans le texte masqué. 3355bd2 (test rouge), 1262582, 9ebf7a6 (témoin `clear(0)` : `["API_TOKEN=hunter/sh","-c",…]`).
  - Mutations : retrait de chacune des quatre gardes (drapeau précédent, shell, `clear(0)`, préfixe de clé), toutes tuées.
- **Tranche 4 (page Secrets)** : validée par le relecteur et par Codex. CI de la PR brouillon #10 verte sur 9ebf7a6 (run 40001838524).
- **Tranche 5 (Images, liste et détail)** : 132dd14, f74eeb4, af588b5, ae04691, b2db5ee, cb079dc ; correctifs de revue : f146848 (tests rouges), e047718, c850b07 (tests rouges), 8939d9c. Validée par le relecteur et par Codex (risque résiduel accepté, ci-dessous). CI de la PR brouillon #10 verte sur 8939d9c (run 40011643314).
  - Mesures (Docker Desktop, 61 images, 76 étiquettes) : `GET /api/images` en 165 à 199 ms, 16 Kio ; six listes simultanées en 246 ms sur une seule lecture de Docker ; détail en 58 à 68 ms.
  - **Révision du plan** (session principale) : l'onglet « Images » de la page Conteneurs, avec son compteur (spec v0.2 §3), n'était attribué à aucune tranche. Il est livré dans la tranche 5.
  - Écarts au plan :
    - fichiers hors liste : `imageList.svelte.ts`, `nav.test.ts`, `scripts/e2e.sh`, `web/e2e/images.e2e.ts`, `container.go` (`maskCommand`), `routes/+page.svelte` (onglet), `secrets_inventory.go` (frontière) ;
    - `dockerx.Container.ImageID` ajouté avec `json:"-"` (aucune route existante ne l'expose) ;
    - e2e indépendant du pilote de stockage : sous overlay2, le RUN témoin pèse 0 octet et sa couche était repliée.
  - Ruling (session principale, resserre la spec v0.2 §5.4) : `secrets.History` va au-delà des deux règles de la spec. Codex et le relecteur jugent ce surcroît juste.
    - BuildKit garde `ARG NOM=<valeur>` et `LABEL k=v` en clair : les valeurs d'`ARG` sont masquées, une ligne `LABEL` l'est en entier (y compris `ONBUILD`, tabulation, casse).
    - `ARG` n'accepte qu'une affectation, en dernier mot ; `|N` exige exactement N mots `NOM=`. Sinon, tout est masqué (relecteur : une valeur à blanc faisait passer `dXNl…==` pour un nom).
    - Après `|N`, deux marqueurs de shell ou plus masquent toute la ligne (Codex : une valeur contenant ` /bin/sh -c ` trompait le compte).
  - Correctif de revue :
    - `entrypoint` + `cmd` masqués bout à bout par `secrets.Args`, puis redécoupés (Codex : `--requirepass` en fin d'entrypoint laissait sortir le premier élément de cmd). Même défaut fermé dans la radiographie d'un conteneur (`maskCommand`) et dans l'inventaire des secrets (`part` et `index` restent ceux du champ d'origine) ;
    - page de détail : nom du conteneur, lien par l'ID complet.
  - Arbitrage (relecteur : validé ; Codex : correctif requis) : la fuite des deux marqueurs a été corrigée (Codex). **Risque résiduel accepté** : une valeur d'`ARG` qui contient ` /bin/sh -c `, suivie d'une commande en forme exec ou d'un `SHELL` autre que sh ou bash, ne laisse qu'un marqueur, et la suite passe par `Line` seul. L'historique ne permet pas de distinguer la valeur de la commande. Les deux revues jugent le cas très peu probable. À écrire dans les limites de la spec à la clôture (tranche 10).
  - Mutations : 12 + 10 + 6, toutes tuées (relecteur : 3 + 2, Codex : lecture seule).
- **Tranche 6 (Images, fichiers en lecture seule)** : aa119e7 (tests rouges), 2ac568f, e25b707, 8076073, 853b601 ; suivi de revue : 25004a8 (tests rouges), 5b3c947, 41b3880. Validée par le relecteur et par Codex. CI de la PR brouillon #10 verte sur 41b3880 (run 40028154018), overlay2 et containerd compris.
  - Vérifiée sur Docker Desktop, hk-ubuntu (aucun refus AppArmor) et hk-fedora (aucun AVC).
  - Mesures (index / aperçu) : alpine:3 0,18 / 0,11 s ; postgres:16-alpine 1,8 / 1,1 s ; golang:1.27 4,1 / 0,56 s (Docker Desktop) ; hublot-test 5,7 / 3,8 s (hk-ubuntu), 8,6 / 5,8 s (hk-fedora).
  - Format de `docker save` vu en CI : overlay2 donne des couches tar non compressées, containerd des couches gzip ; disposition OCI dans les deux cas.
  - Écarts au plan : fichiers hors liste `api.ts`, `CommandPalette.svelte`, `images-fs.e2e.ts`, `internal/secrets/secrets_test.go` (TestCost), `.github/workflows/ci.yml` (étape `-v`) ; `Idle` testé en temps réel court, sans horloge injectée. Contrat `Cached`/`SearchCached`/`Indexing` inchangé.
  - Revue : une couche faite de zéros est acceptée comme couche vide, sans contournement (empreinte vérifiée, aucune origine ne peut y pointer).
  - Correctif de revue : une erreur passagère d'extraction est oubliée après `Idle`, les erreurs définitives restent ; tests de `TypeXGlobalHeader`, `keepExtract(TooLarge)` et `unsupported_image`.
  - Arbitrage TestCost (Codex : affaibli à 5 s et ratio 20 ; relecteur : deux mutations passaient, 18,9 et 22,4). Seuils retenus : ratio ≤ 8 sous le plafond, ≤ 1 au-delà, 1 s par appel ; mesuré sous `-race` et charge, 1,8 au plus. Restent invisibles au ratio, comme avant : un ralentissement uniforme de `maskText`, et un coût quadratique sur une entrée ≤ 8 Kio.
  - Mutations : 42 (implémenteur, 39 tuées, 3 équivalentes) + 30 (relecteur, 24 tuées, 4 équivalentes, N1 et N10 tuées ensuite).
- **Hors tranche, audit de sécurité indépendant de 7640785** (relecteur en attaquant, Codex en lecture) : 5 constats prouvés (2 Important, 3 Minor). Décision de l'utilisateur (2026-10-23) : correctif v0.1.2 sur `phase/v0.1.2` (PR brouillon #11), puis report dans `phase/v0.2`. Détail au moment de la fusion.
- **v0.1.2 publiée** (2026-10-23) : PR #11 fusionnée dans main (0c78a96), tag annoté v0.1.2, release signée vérifiée par le workflow (`RELEASE OK`) puis en anonyme depuis hk-ubuntu (curl, cosign, jeton ghcr anonyme) ; `0.1` = `0.1.2`, pas de `latest`. Note « Security release » en tête. Correctifs : 350222b, 9c20ec0, a608f24, daee26f, fec145c, 9a8eadb, 5c5d76e, 893db2b (doc d'installation sur 0.1.2). Trois tours : audit (5 constats prouvés), contre-audit (téléchargement bloquant, plafond global, `--init`, budget de journal), dernier tour (/64 IPv6, délai d'écriture obligatoire, synthèse par minuteur).
  - Limites acceptées et documentées : course de lien avant le montage par dockerd ; lien que seul dockerd voit ; plafond global atteignable depuis plusieurs adresses locales ; une page rebindée peut prendre les 64 connexions locales ; synthèse `login.fail` perdue si l'agent s'arrête dans la minute ; limite de tentatives par adresse IPv6 entière (jeton de 256 bits).
- **Report v0.1.2 dans la v0.2** : fusion c3faab9 (sans conflit, `make test` vert) ; c7adfc5 (tests rouges), 38c9b39, 9235a3d (tests rouges), e2267be. Erreurs Docker à détail fixe sur les routes v0.2, délai d'écriture sur les deux aperçus, spec v0.1 §7 alignée sur le budget `login.fail`. Validé par l'auditeur (sonde `DOCKER_HOST` à mot de passe rejouée sur 17 routes). CI verte (run 40045769796).
  - Arbitrage : Codex signalait une fuite dans le SSE des logs et un aperçu de conteneur sans délai. Vérifié dans e2267be : `docker_unavailable` passe par `dockerDetail` (texte fixe) et `fs_preview.go` écrit par `writeTimed`. Les deux constats sont faux pour la v0.2 ; le second est vrai pour le tag v0.1.2.
  - **Pas de v0.1.3** (recommandation de l'auditeur et de l'implémenteur) : l'aperçu sans délai d'écriture de la v0.1.2 exige une session, ne tient ni créneau ni verrou, et reste borné à 2 Mio par connexion. Corrigé dans la v0.2.0.
- **Tranche 7 (diff de contenu avec l'image)** : 17f1ef7 (tests rouges et dépendance), 606edd4 ; correctifs de revue : bd7eabc (tests rouges), 124e67e, 9a5b672 (tests rouges), 445e67e. Validée par le relecteur et par Codex (arbitrage ci-dessous). CI de la PR #10 verte sur 445e67e (run 40054560404).
  - Dépendance `@codemirror/merge` 6.12.2 (MIT, publiée le 2026-06-09) : +45 Ko (16 Ko gzip) au total, chargée au premier diff ; +13 Ko (4 Ko gzip) au chargement initial.
  - Écart : `routes/+page.svelte` efface `diff` et `change` au changement de conteneur (sinon le diff d'un conteneur suivait sur le suivant).
  - Correctifs de revue :
    - le diff de la vue est calculé par un `lineDiff` maison passé par `diffConfig.override` (ni `timeout` ni `scanLimit` ne bornaient `findMatch` : une ligne de 300 Ko déplacée gelait l'onglet 12 à 20 s). Borne : lignes × modifications ≤ 10⁷, au plus 1 000, sinon un seul bloc ; affinage par caractère ≤ 4 Kio par bloc et 64 Kio en tout. Mesuré : 1 à 2 ms pour la ligne de 300 Ko, 103 ms pour 29 Mo ;
    - numéros de ligne tirés des blocs affichés ; relance du 202 coupée quand le panneau Diff est caché ; `highlightSpecialChars` (U+202E).
  - Arbitrage (Codex : « a » et « a\n » vus identiques ; relecteur : 20 000 paires aléatoires justes). Vérifié dans 445e67e : « a » donne `["a"]`, « a\n » donne `["a",""]`, la différence est vue. Constat écarté.
  - Minor reporté à la tranche 10 : aucun message ne signale le repli en un seul bloc.
  - Le `.env` s'affiche en clair dans le diff comme dans l'aperçu (spec v0.1 §7).
- **Tranche 8 (Snapshots : prise, liste, suppression, comptage)** : e4c51ac (tests rouges), 942b130, 19eeba3, 1d7680f, b93a175, c2238a0, 8b2c4b0 ; correctif de revue : 5976acd (tests rouges), b20fbff. Validée par l'auditeur (preuves) et par Codex. CI de la PR #10 verte sur b20fbff (run 40066004938).
  - Vérifiée sur hk-ubuntu et hk-fedora, sur les cinq cibles, actives et arrêtées (prise et comptage), sans refus AppArmor ni AVC.
  - Mesures (golang:1.27, 36 784 entrées, 941 Mo hachés, manifeste 1,5 Mo) : actif 7,3 s / 18,8 Mo (ubuntu), 4,9 s / 21,6 Mo (fedora) ; arrêté 13,4 s / 37 Mo, `docker inspect` bloqué 10 à 12,5 s pendant l'export.
  - Écarts au plan : `server.go`, `icons.ts`, `nav.test.ts`, `ContainerDetail.svelte` ; ajouts `Header.Limits`, `MaskMounts`, `Collector` ; `prepareDataDir` rend le descripteur du dossier de données. Correctif e2e : `GET /api/snapshots` perdait une prise finie.
  - Correctif de revue (auditeur, prouvé ; Codex) : `CaptureArchive` gardait ses entrées sans plafond de noms (≈ 5,4 Gio au plafond de 4 Gio, par un simple `GET /count`). Plafond de 256 Mio de noms et de cibles, comme l'index tar, dans l'export (prise et comptage) et sur les listings du parcours ; `too_large`, rien d'écrit. Prouvé : 80 000 liens à chemins de 4 Kio refusés, agent debout ; 65 000 juste sous le plafond, RSS 518 Mo. Détail fixe pour les 500 de la clé et du magasin. `TestWalkExistingMountNotTraversed` sur `/dev`, sans privilège.
  - Spec v0.2 corrigée (§5.7 « Mémoire », Q57, §9) : 250 Mo mesurés, 500 Mo au pire, au lieu de 150 Mo.
  - Réserves acceptées (Codex) : `tar.Reader.Next()` lit un en-tête avant le contrôle (borné par `archive/tar`) ; la rétention de 1 Gio peut être dépassée de la taille d'un snapshot.
  - Minor reportés : `count.go`, après un comptage arrêté au plafond, le premier relevé attend le verrou de dockerd et répond 503 au lieu de 413 (tranche 10) ; `parcours.e2e.ts:144-147`, attentes fixes (500 ms, 4,5 s) fragiles sous charge, deux échecs à load 9 (en tête de la tranche 9).
  - Mutations : 60 + 6, toutes tuées.
- **Tranche 9 (Snapshots : comparaison)** : d1b565f (parcours e2e sans attentes fixes, Minor de la tranche 8), 7d01619 (tests rouges), 51a4473, e97ab23, 7983be6 ; correctif de revue : 4d4863e (tests rouges), 41bf9d1. Validée par l'auditeur (preuves) et par Codex (arbitrage ci-dessous). CI de la PR #10 verte sur 41bf9d1 (run 40081633739).
  - Mesure : la fusion en flux tient au plus 3,2 Mio de tas pour 2 × 500 000 entrées.
  - Écarts au plan : `Compare` prend des flux (`iter.Seq2`) et renvoie une erreur ; ajouts `Result.Type`, `Merger`, `ExistingKey` (lit la clé sans la créer) ; `snapshots.e2e.ts` reçoit un second test.
  - Correctif de revue (auditeur, prouvé) : sans empreintes, les fichiers inchangés « inconnu » remplissaient la limite de 10 000 et évinçaient les vrais changements (20 000 inchangés + un ajout : l'ajout disparaissait). Les inconnus sont comptés à part (`unknownCount`, 1 000 listés, repliés dans la page) ; `truncated` ne porte que sur les vrais changements. Spec §5.7 et §6, guide EN/FR.
  - `image-match` : la spec est alignée sur le code (2 Mio, `match:false` si la taille diffère, rien de gardé) ; monter à 256 Mio demanderait de toucher fsview. Test de la course de `claim` ajouté (la mutation faisait paniquer l'agent).
  - Arbitrage : Codex jugeait la lecture d'`image-match` non bornée (`io.Copy`). Vérifié dans 41bf9d1 : la taille est refusée au-delà de `MaxExtract` avant la lecture (snapshots_compare.go:443), et l'extraction lit par `io.LimitReader(tr, MaxExtract+1)` (imageindex.go:746-750). Constat écarté.
  - Minor reporté à la tranche 10 : quand `truncated` est vrai, `unknownCount` n'est qu'un minimum, mais l'interface l'affiche comme un nombre exact (« au moins » à écrire).
  - Mutations : 41 + 5, toutes tuées.
