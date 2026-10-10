# Explorateur 3/3 — Écriture (F4) — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (règle du dépôt, `CLAUDE.md`) :** chaque tâche est relue par un sous-agent indépendant **et** par Codex avant la suivante. Les tâches 2 et 3 écrivent dans un conteneur : relecture prioritaire sur l'évasion (spec §13) et sur l'ordre de §5.3.
>
> **Compilé et vérifié avant rédaction.** Leçon du plan 2 (code jamais compilé, 8 défauts trouvés au scan préalable) : tout le code de ce plan a été écrit puis exécuté sur une copie isolée du dépôt (`git worktree` sur `d6bb68e`, supprimée ensuite). Les blocs de code ci-dessous sont **extraits de cette copie**, pas recopiés.
> - Go : `go vet ./...` et `go vet -tags integration ./...`, en macOS et en `GOOS=linux` ; `gofmt -l` vide ; `go test -race ./cmd/... ./internal/...` vert sur macOS ; `scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...` vert (Docker Desktop 29.7.2, API 1.55, conteneur `golang` root), **aucun SKIP** ; `go list -deps ./cmd/hublot` ne contient pas `testing`.
> - Rouge avant le code : les tests de la tâche 2 échouent à la compilation (`undefined: Writer`) en macOS comme en `GOOS=linux` ; ceux de la tâche 4 compilent et échouent à l'exécution (routes absentes : `404`) ; ceux de la tâche 6, 4 tests sur 3 fichiers.
> - Mutations (une fois chacune, puis annulées) : `fsys.Stat` à la place de `fsys.StatNoLinks` dans `files.go` → `TestProcLinksRefused` (« ÉVASION : PutArchive appelé malgré un lien ») et `TestLinksRefused` échouent ; contexte de la requête passé tel quel à `PutArchive` → `TestPutSurvivesRequestEnd` échoue.
> - Mesuré sur Docker Desktop : le refus d'écriture sur un rootfs `--read-only` ou un volume `:ro` est un `InvalidArgument` (`container rootfs is marked read-only`, `mounted volume is marked read-only`) ; un dossier absent est un `NotFound` ; Docker **supprime puis recrée** le fichier (inode 570993 → 320851) ; sans `copyUIDGID`, le mode et l'uid/gid du tar sont gardés (`640 1000 1001`) ; l'écriture marche sur un conteneur en pause.
> - Interface : CodeMirror installé aux versions de la tâche 6 ; Vitest 54 tests verts ; `npm run check` 0 erreur ; `npm run lint` propre ; `vite build` : CodeMirror et ses langages ne sont atteints que par `import()` dynamique (le nœud de la page conteneur pèse 22 Ko). `scripts/e2e.sh` : le parcours unique passe, et le parcours jetable d'écriture de la tâche 7 (non commité) passe contre un vrai agent, sous la CSP.
> - **Revérifié après la revue 1** (même méthode : copie reconstruite depuis les blocs du plan `a43755e`, corrections appliquées, puis supprimée). `go vet` (avec et sans `-tags integration`) en macOS et `GOOS=linux`, `go test -race` macOS, `scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...` : verts, aucun SKIP. Mutations nouvelles : verrou retiré → `TestWritesAreSerialized` échoue ; `fstatfs` retiré de `procFS.StatNoLinks` → `TestProcPseudoFSRefused` échoue. Vitest 55 tests, `check`, `lint`, `build` verts ; parcours jetable rejoué, avec un fichier CRLF relu octet pour octet après édition (`un 1\r\ndeux\r\n`). Mesuré en plus (Docker Desktop) : écrire dans `/dev`, `/dev/shm` ou un `--tmpfs` d'un conteneur actif réussit sans que le conteneur voie le fichier ; `/run` (pas un tmpfs) est vu ; `/etc/hosts` d'un conteneur jamais démarré s'écrit (`HostsPath` vide), Docker le masquera au démarrage ; `mime.ParseMediaType` décode `filename*=UTF-8''..%2Fx` en `../x`, `filename="a\/b"` en `a/b`, et préfère `filename*` à `filename`.
> - **Revérifié après la revue 2** (même méthode, copie reconstruite depuis `3b20faf`). `go vet` macOS et Linux, avec et sans `integration` ; `go test -race` macOS ; `scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...` : verts, aucun SKIP. Mutations nouvelles, chacune attrapée : contrôle `ctx.Err()` de `put` retiré → `TestEndedRequestWritesNothing` (3 essais sur 3) ; attente du verrou sourde à `ctx` → `TestLockWaitAbandoned` ; `unlock` différé après `Open` → `TestLockReleasedOnPanic` ; caractères de format acceptés → `TestUploadInvalidName` ; contrôle de point de montage retiré → `TestIntegrationWriteRunning` (`/sbin/docker-init` arrive jusqu'à `If-Match`). Mesuré en plus : `PUT /archive` vers un fichier existant (`/etc/motd`) → `InvalidArgument` « extraction point is not a directory » ; `--init` monte `/sbin/docker-init` sans le montrer dans `Inspect.Mounts`. Interface inchangée par ce tour.
> - **Non vérifié** : les VM `hk-ubuntu` et `hk-fedora` (tâche 8).

**Goal:** L'utilisateur modifie un fichier texte d'un conteneur, actif ou arrêté, dans un éditeur CodeMirror, ou y envoie un fichier. Chaque écriture passe par une confirmation qui nomme le conteneur et le chemin, est journalisée avant et après, et n'écrit jamais à travers un lien.

**Architecture:** Un nouveau paquet `internal/files`, seul point d'écriture (§4, §7), applique l'ordre de §5.3 dans une fonction par écriture : lecture seule, contrôle du chemin par la vue `fsview` (`StatNoLinks` : `openat2 RESOLVE_NO_SYMLINKS` et `fstatfs` pour un conteneur actif, l'index pour un conteneur arrêté), `If-Match`, audit (intention), `dockerx.PutArchive` sur le parent du chemin vérifié, `Views.Forget`, audit (résultat). `api` gagne `PUT /fs/file` et `POST /fs/upload`, qui lisent le corps puis appellent `files` sous la garde de §13. L'interface gagne un éditeur CodeMirror 6 chargé à la demande, une boîte de confirmation et le bouton d'envoi, absents en lecture seule.

**Tech Stack:** Go 1.27 (`archive/tar`, `mime/multipart`, `net/http`), `github.com/moby/moby/client` v0.6.1 (`CopyToContainer`), `github.com/cyphar/filepath-securejoin` v0.7.0 (inchangé), `golang.org/x/sys` v0.48.0 ; SvelteKit 3 / Svelte 5, Vitest, CodeMirror 6 (`codemirror` 6.0.2, `@codemirror/state` 6.7.6, `@codemirror/view` 6.43.14, `@codemirror/language` 6.13.1, `@codemirror/lang-json` 6.0.2, `@codemirror/lang-yaml` 6.1.3, `@codemirror/lang-javascript` 6.2.5, `@codemirror/lang-python` 6.2.1, `@codemirror/legacy-modes` 6.5.5).

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§3, §4, §5.2 étape 1, §5.3, §5.4, §6, §7, §9, §10, §13). Périmètre : `docs/superpowers/plans/2026-10-08-explorateur-1-procfs.md`, « Découpage de la phase Explorateur », bloc « Plan 3 ». Plan précédent (format, décisions en vigueur, Q1 à Q13) : `docs/superpowers/plans/2026-10-09-explorateur-2-tarindex-diff.md`.

## Décisions de revue

Revue du plan, premier tour (relecteur Opus et Codex, « Prêt avec réserves »). Une ligne par correction, avec la tâche qui l'applique.

- **R1 (Opus I1) :** l'éditeur rendait `doc.toString()`, qui joint toujours par LF : un fichier CRLF était enregistré en LF (mesuré, `@codemirror/state` 6.7.6). `editor.ts` gagne `docText(state)` = `state.sliceDoc()`, qui garde le séparateur de l'état ; test Vitest d'aller-retour (CRLF, LF, CR isolé) ; le parcours jetable édite un fichier CRLF. → Tâches 6, 7.
- **R2 (Opus I4) :** les écritures ne se sérialisaient pas : deux onglets avec le même `If-Match`, ou deux envois du même nom, écrivaient tous deux. `Writer` prend un verrou par conteneur de l'étape 2 à l'étape 6, abandonné avec la requête. Test `TestWritesAreSerialized`, écrit d'abord. → Tâches 2, 3 ; Q23.
- **R3 (Codex) :** `Upload` vérifiait `If-None-Match` avant le nom et le chemin. Ordre de §5.3 : étape 2 (nom, chemin, parent), puis étape 3 (précondition, existence). Tests `TestUploadChecksPathBeforePrecondition` et, par la route, `../x` sans `If-None-Match` → `invalid_name`. → Tâches 2, 3, 4.
- **R4 (Opus Q19, Codex) :** noms refusés en plus : non UTF-8, caractères de contrôle (C0, DEL, C1), `\`. La valeur validée est celle que `mime.ParseMediaType` décode (`filename*` RFC 2231, paires entre guillemets) : c'est elle qui part dans le tar. Test `TestUploadNameIsTheDecodedValue`. → Tâches 2, 3, 4 ; Q19.
- **R5 (Opus Minor 1) :** `InvalidArgument` → `ErrRefused` se limite à `PutArchive` (`wrapPut`) ; `wrap`, donc les routes de lecture, ne change pas. → Tâche 1 ; Q15.
- **R6 (Opus Minor 2, Codex) :** une requête déjà terminée (la garde a répondu 504) ne commence pas d'écriture (`TestEndedRequestWritesNothing`) ; après un 504, l'interface dit que l'écriture a peut-être eu lieu ; la troncature au-delà d'une minute est une limite de §13. → Tâches 3, 6, 8 ; Q17.
- **R7 (Opus I2) :** Docker écrit dans sa propre vue des montages. Refus avant l'intention, par l'`Inspect` : `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`, la `Destination` d'un montage, tout chemin sous un tmpfs (`Mounts` de type `tmpfs` et `HostConfig.Tmpfs`, nouveau champ `ContainerInfo.Tmpfs`), sous `/dev`, `/proc` ou `/sys` → `forbidden_path` qui nomme la cause. Tests d'abord (`TestRefusesWhereDockerDoesNotWrite`), intégration `--tmpfs` et `/etc/hosts`. → Tâches 1, 2, 3, 5 ; Q21.
- **R8 (Opus I3, Codex Critical 1 et 2) :** la règle `fsview.HostBind` (§5.2 étape 7 et ses extensions) s'applique à toute écriture, conteneur actif compris. Test `TestHostBindRefusesEveryWrite`, écrit d'abord. → Tâches 2, 3 ; Q22.
- **R9 :** avec R7, `TestProcPseudoFSRefused` serait passé par la règle de préfixe, plus par `fstatfs`. Il sert désormais le `/proc` de l'hôte comme racine (`/`, `/version`) : seul `fstatfs` refuse ; mutation vérifiée. → Tâche 2.
- **R10 (Minor, premier tour) :** volume `:ro` testé (tâche 1) ; `EDIT_MAX` refuse avant la confirmation (tâche 7) ; `/api/meta` n'est plus demandé deux fois, la page lit `readOnly` dans un contexte du layout (tâche 7) ; contrôle `MARKER` décoratif retiré (tâche 2) ; userns-remap, extraction chrootée de dockerd et vue des montages écrits dans §5.3 et §13 (tâche 8) ; la taille et la lecture seule passent avant l'étape 2, noté dans « Décisions » ; « Structure des fichiers » complétée.

Second tour (relecteur Opus et Codex, « Prêt avec réserves ») :

- **R11 (Codex) :** `wrapPut` ne classe en `ErrRefused` (`read_only`) que les deux refus de lecture seule que Docker donne (« marked read-only », mesuré) ; tout autre `InvalidArgument` de `PutArchive` devient `ErrPutInvalid` → `conflict` (409) : le chemin a changé après les contrôles (mesuré : « extraction point is not a directory »). Un test par cas, unitaire et sur Docker. → Tâches 1, 4 ; Q15.
- **R12 (Codex) :** le nom est validé avant les règles de l'`Inspect` : un nom invalide vers un dossier interdit donne `invalid_name`. Tests `TestUploadNameBeforeInspectRules` et, par la route, `a/b` vers `/proc`. → Tâches 2, 4.
- **R13 (Codex, Opus) :** tests du verrou : requête abandonnée pendant l'attente (rien d'écrit, verrou libre ensuite), panique après la prise (verrou rendu). `TestEndedRequestWritesNothing` annule le contexte une fois le verrou pris, dans `Open` : il vise désormais le seul contrôle de `put`. Mutations ajoutées. → Tâches 2, 3.
- **R14 (Codex) :** `writable` lit l'`Inspect` hors du verrou. Pas de second `Inspect` sous verrou : les montages d'un conteneur sont fixés à sa création (`docker update` ne les change pas) et un ID n'est jamais réutilisé ; le verrou sérialise nos écritures, pas la configuration de Docker, qu'un second `Inspect` ne figerait pas davantage. → Décisions.
- **R15 (Opus Minor 2, Q21) :** une édition sur un conteneur actif refuse une cible qui est un point de montage : `STATX_MNT_ID` de la cible et de son parent, nouvelle interface facultative `fsview.MountPointChecker`, implémentée par `procfs`. Ferme par défaut les montages absents d'`Inspect` (configs et secrets Swarm, `--init`, CDI). Tests d'abord : `TestIsMountPoint` (Linux, sur `/proc`, sans privilège), `TestEditRefusesMountPoint` (portable), `--init` en intégration. → Tâches 2, 3, 5 ; Q21.
- **R16 (Opus I1) :** Q22 n'est **pas tranchée** : c'est à l'utilisateur. La prose qui en disait trop est corrigée : `fsview.HostBind` ne refuse que les binds de `/`, de `/proc` et `/sys` (et dessous) et de `/dev` ; `-v /etc:/hostetc` reste inscriptible. Q22 est réécrite avec deux options ; le plan suit (a) en attendant, avec un test qui montre `/etc` accepté. → Tâches 2, 8 ; Q22.
- **R17 (Opus Minor) :** texte périmé de `TestProcPseudoFSRefused` ; ordre réel `PutArchive` → `Forget` → audit (résultat) dans le texte ; verrou décrit « pris avant d'ouvrir la vue » ; cas « fichier monté » en intégration (bind `:ro` de `/etc/os-release`) ; lignes §13 : FUSE muet qui garde le verrou (et `504` trompeur), montages absents d'`Inspect` ; caractères de format (bidi, largeur nulle) et séparateurs de ligne et de paragraphe refusés dans un nom. → Tâches 2, 3, 5, 8.

## Ce que les plans précédents ont préparé

- `fsview.FS.StatNoLinks` (plan 1, Q2) : chemin nettoyé, aucun lien sur toute la chaîne, dernier composant compris (`ErrLinkInPath`), `fstatfs` du descripteur (`ErrForbiddenPath`). `procfs` l'implémente par `openat2` `RESOLVE_IN_ROOT | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS` en `O_PATH` ; `tarFS`, par l'index. Un lien pendant est déjà refusé (`TestStatNoLinks`, `/dangling`). `files` n'ouvre donc rien lui-même.
- `fsview.Views.Forget(id)` (plan 2) : « plan Explorateur 3 calls it after a write on a stopped container ».
- L'`ETag` de l'aperçu (plan 1) : SHA-256 du contenu, entre guillemets, « the If-Match of a later edit ». La tâche 4 le fait calculer par `files.ETag`, la même fonction que celle qui compare.
- `fsview.MemFS` refuse les liens dans `StatNoLinks` comme `procfs` : les tests portables de `files` s'en servent.
- La garde (`guarded`, 4 opérations, 10 s) et le contrôle d'`Origin` sur toute méthode autre que GET/HEAD sont en place : les routes d'écriture en héritent.

## Décisions de ce plan (la spec ne tranche pas)

- **`files.Writer`.** `Writer{ReadOnly, Docker, Audit, Open, Forget}`, méthodes `Allowed()`, `Edit(ctx, c, p, content, ifMatch) (etag, error)`, `Upload(ctx, c, dir, name, content, ifNoneMatch) (path, error)`, où `c` est l'`Inspect` que la route a déjà fait (`dockerx.ContainerInfo`). `Open` vaut `Deps.OpenFS` (donc `Views.For`) et `Forget` vaut `Views.Forget` : `files` ne connaît ni `procfs` ni `tarindex`. `Docker` est un `dockerx.Client` complet : `HostBind` lit les options des volumes (R8). L'interface étroite `Auditor` laisse les tests ordonner l'audit et l'écriture. → Tâches 2, 3.
- **Ordre exact.** Lecture seule ; plafond de taille (un refus sans contact avec le conteneur, avant §5.3) ; étape 2 : nom (envoi), `writable` sur l'`Inspect` (R7, R8), verrou du conteneur (R2), vue, `StatNoLinks` du parent ou de la cible, type, point de montage (édition, R15) ; étape 3 : `If-Match` (relecture) ou `If-None-Match` puis existence ; étapes 4 à 6 dans `put` : requête encore vivante, intention, `PutArchive`, `Forget`, résultat. → Tâche 3.
- **Lecture seule à un seul endroit (§7).** `files` refuse en premier (`ErrReadOnly`), avant même d'ouvrir la vue. La route appelle `files.Allowed()` avant de lire le corps, pour ne pas lire 32 Mio pour rien ; c'est toujours `files` qui décide. → Tâches 3, 4.
- **Chemin d'écriture.** Édition : `fsview.CleanPath(p)`. Envoi : `fsview.CleanPath(parent + "/" + nom)`, avec un nom déjà validé ; jamais `path.Join` ni `filepath.Join`. `PutArchive` reçoit `path.Dir` de ce chemin, et rien d'autre. → Tâche 3.
- **Existence avant un envoi.** `StatNoLinks(chemin)` : absent (`fs.ErrNotExist`) → on continue ; lien, même pendant → `link_in_path` (`openat2` refuse un lien en dernier composant sans le suivre : vérifié sous Linux) ; toute autre entrée → `conflict`. C'est la sémantique `lstat` de §5.3 étape 3, sans `fstatat` à écrire dans `fsview`. → Tâches 2, 3.
- **Tar d'une entrée.** Nom de base, `Mode` par `fsview.UnixPerm` (setuid, setgid et sticky gardés), `Uid`, `Gid`, `ModTime` = maintenant. Nouveau fichier : `0644`, uid/gid du dossier parent. Pas de `copyUIDGID` : Docker donnerait le fichier à l'utilisateur configuré du conteneur. `noOverwriteDirNonDir` reste (défaut du client) : Docker ne remplace jamais un dossier par un fichier. → Tâches 1, 3.
- **Audit.** `file.write` ou `file.upload`, `container` = ID complet, `path` = chemin d'écriture, `result` = `intent` puis `ok` ou `error` (avec `error`). Une intention qui ne s'écrit pas arrête tout (`internal`, rien n'est écrit). Un résultat qui ne s'écrit pas part dans les logs (`slog.Error`) : l'écriture a eu lieu, la masquer pousserait à la refaire. Les refus avant l'étape 4 ne sont pas journalisés (rien n'a été tenté). → Tâche 3.
- **Verrou.** Un canal de capacité 1 par ID de conteneur, créé à la première écriture, jamais retiré (un par conteneur écrit, quelques octets). Pris avant d'ouvrir la vue, rendu par `defer` après l'étape 6, panique comprise ; l'attente s'arrête avec la requête (`ctx`). `writable` lit l'`Inspect` avant le verrou : les montages sont fixés à la création du conteneur (R14). → Tâche 3 ; Q23.
- **Corps lu avant la garde.** Un client lent ne tient pas un créneau de la garde (§13) pendant qu'il envoie. L'écriture elle-même prend un créneau : `StatNoLinks` peut buter sur un FUSE. → Tâche 4.
- **Réponses.** `PUT /fs/file` → `200 {"etag":"\"…\""}` et en-tête `ETag` (l'interface continue d'éditer sans relire). `POST /fs/upload` → `201 {"path":"/…","etag":"\"…\""}`. → Tâche 4.
- **Multipart.** Une seule partie utile, `file` ; les autres sont ignorées ; aucune partie `file` → nom vide → `invalid_name`. Le nom est le `filename` **brut** (Q19). → Tâche 4.
- **Statuts.** `read_only` 403, `conflict` 409, `invalid_name` 400, `too_large` 413 (corps au-delà de `http.MaxBytesReader` compris). → Tâche 4.
- **Lien physique d'un conteneur arrêté.** `StatNoLinks` le montre comme un fichier ; l'éditer le remplace par un fichier ordinaire (Docker supprime puis recrée, comme sur un conteneur actif). Pas de refus : c'est le comportement de `docker cp`.
- **Interface.**
  - `readOnly` vient du `/api/meta` du layout, par un contexte (`setContext('readOnly', …)`), et vaut `true` tant qu'il n'a pas répondu : les boutons d'écriture n'apparaissent jamais par erreur (§9).
  - Un texte de plus de 2 Mio (`EDIT_MAX`, en octets UTF-8) est refusé avant la confirmation. Après un `504`, le message dit que l'écriture a peut-être eu lieu (R6).
  - L'éditeur garde le séparateur de lignes du fichier (`EditorState.lineSeparator`, relu par `sliceDoc`, R1) ; un fichier non UTF-8 ou à fins de ligne mélangées n'est pas modifiable (Q16).
  - CodeMirror lui-même est chargé à l'ouverture de l'éditeur, pas seulement les langages : le téléphone (§9, consultation) ne le télécharge jamais.
  - Boîte de confirmation : `<dialog>` natif, le bouton Annuler a le focus ; elle rappelle le nom du conteneur, ses 12 premiers caractères d'ID, le chemin, la taille (envoi) et, pour un conteneur arrêté, que l'index sera reconstruit.
  - Après un `conflict`, l'éditeur reste ouvert : l'utilisateur copie ses modifications avant de recharger.
  - Après un envoi, l'explorateur recharge le dossier et sélectionne le nouveau fichier.
  - Le parcours Playwright unique (§10) ne change pas ; la tâche 7 vérifie l'écriture par un parcours jetable, non commité.
- **Dépendances interface.** Les neuf paquets CodeMirror vont dans `dependencies` (ils partent dans le bundle), versions de la tâche 6. `@codemirror/state`, `view` et `language` sont listés parce que le code les importe directement.

## Questions ouvertes

Le plan suit la recommandation de chaque question ; une réponse différente change la tâche indiquée.

- **Q14 — Plafond de taille d'une édition et d'un envoi.** La spec n'en fixe pas.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* édition **2 Mio**, envoi **32 Mio**, tenus en mémoire. Raisons : l'éditeur ne reçoit que ce que l'aperçu a servi, et l'aperçu s'arrête à 2 Mio (§6) ; au-delà, l'`ETag` n'existe pas. 32 Mio couvrent un fichier de configuration, un petit binaire ou un certificat, le cas « je corrige mon conteneur ». Le contenu est lu en entier avant `PutArchive` (l'en-tête tar exige la taille), puis le tar en double la place : 64 Mio au pire, un instant, pour un seul utilisateur. Plus gros : `docker cp`, que le message d'erreur nomme. Le hachage du fichier actuel est lu au plus jusqu'à 2 Mio + 1 octet (`too_large` au-delà). → Tâches 3, 4, 6.
- **Q15 — Refus de Docker (rootfs `--read-only`, volume `:ro`).** Docker répond `InvalidArgument` (mesuré sur les deux) ; `wrap` le classait en `docker_unavailable` (503), ce qui accuse le démon à tort.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* pour `PutArchive` seulement (`wrapPut`, R5), un `InvalidArgument` dont le message dit « marked read-only » → `dockerx.ErrRefused` → `read_only` (403), avec le message de Docker en détail ; tout autre `InvalidArgument` → `dockerx.ErrPutInvalid` → `conflict` (409), car le chemin a changé après nos contrôles (R11). Le classement par message plutôt que par un second `Inspect` (`ReadonlyRootfs`, `RW` du montage) : les deux messages sont ceux du code de Docker depuis des années, et un `Inspect` ne dirait pas quel montage la cible traverse. Les routes de lecture gardent `docker_unavailable`. L'interface dit « Écriture refusée : la cible est en lecture seule (container rootfs is marked read-only) ». Aucun code nouveau dans §6. → Tâches 1, 4, 6.
- **Q16 — Fichier non UTF-8, BOM, fins de ligne CRLF.** `res.text()` remplace les octets invalides par U+FFFD et retire le BOM ; CodeMirror réécrit toutes les fins de ligne en `\n` par défaut. Enregistrer changerait donc des octets que l'utilisateur n'a pas touchés.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* décodage strict (`TextDecoder` `fatal`, `ignoreBOM`) ; non UTF-8 → aperçu seul, avec un message ; CRLF ou LF uniformes → éditables, avec le même séparateur, relu par `state.sliceDoc()` (jamais `doc.toString()`, qui rend du LF : R1) ; mélange → aperçu seul. Pas de choix d'encodage en v0.1. → Tâches 6, 7.
- **Q17 — Écriture coupée.** Docker supprime le fichier puis le recrée (mesuré : l'inode change). Si la requête HTTP vers Docker s'arrête au milieu (onglet fermé, délai de 10 s de la garde), le fichier peut disparaître ou rester tronqué.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* une requête déjà terminée ne commence rien (`ctx.Err()` avant l'intention, R6). Une fois l'intention journalisée, `PutArchive` reçoit un contexte détaché de la requête (`context.WithoutCancel`), borné à 1 min (`PutTimeout`). Le tar est construit en mémoire avant l'appel. La garde peut répondre `504` pendant que l'écriture continue : l'interface dit alors « l'écriture a peut-être eu lieu, rechargez avant de réessayer ». Restent des limites de §13 : l'écriture n'est pas atomique (pas d'API Docker pour écrire à côté puis renommer), et une écriture qui dépasse la minute est coupée, fichier perdu ou tronqué. → Tâches 3, 6, 8.
- **Q18 — `If-Match` absent ou mal formé, `If-None-Match` absent.** La spec dit « obligatoire » sans code ; `428 Precondition Required` n'est pas un code de §6.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* `conflict` (409) dans les trois cas. `If-Match` doit être exactement l'`ETag` de l'aperçu (`"<64 hex>"`) : ni `*`, ni `W/…`, ni hex sans guillemets. → Tâches 2, 3.
- **Q19 — Nom d'un envoi.** `multipart.Part.FileName()` ne garde que le nom de base : `a/b` deviendrait `b` au lieu d'être refusé (§10 exige `invalid_name`).
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* lire le `filename` de `Content-Disposition` tel que `mime.ParseMediaType` le décode (`filename*` RFC 2231 prioritaire, paires entre guillemets), et valider **cette** valeur, celle qui part dans le tar (R4). Refuser aussi l'octet nul, plus de 255 octets (`NAME_MAX`), un nom non UTF-8 (l'interface ne pourrait plus l'ouvrir, §13), les caractères de contrôle C0, DEL et C1 (un saut de ligne fausserait la boîte de confirmation) et `\`. Limite connue : un navigateur encode `"`, CR et LF dans un nom de fichier (`%22`…), le fichier arrive sous ce nom encodé. → Tâches 2, 3, 4.
- **Q20 — Index retiré même après un échec.** §5.3 étape 6 dit « en cas de succès ».
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* `Forget` après **tout** `PutArchive`. Un échec peut avoir supprimé le fichier (Q17) ; un index qui le montrerait encore mentirait. Le coût est une réindexation. → Tâches 2, 3, 8.
- **Q21 — Où Docker écrit vraiment.** `PUT /archive` extrait dans la vue de dockerd : le rootfs plus les montages configurés, pas l'espace de noms de montage du conteneur (mesuré par Opus et ici). Un fichier monté seul (`-v ./conf.yml:/app/conf.yml`) ou `/etc/hosts` → HTTP 500 « device or resource busy », traduit en `503 docker_unavailable` trompeur. Un `--tmpfs`, `/dev`, `/dev/shm` → 200, mais le conteneur ne voit jamais le fichier : faux succès, audit `ok`.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* refuser avant l'intention, à partir de l'`Inspect` (`files.writable`) : chemin égal à `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf` (toujours : Docker les monte à chaque démarrage, même s'ils sont encore absents d'un conteneur jamais démarré) ; égal à la `Destination` d'un montage (« modifiez sa source ») ; sous un tmpfs (`Mounts` de type `tmpfs`, `HostConfig.Tmpfs` → `ContainerInfo.Tmpfs`) ; sous `/dev`, `/proc` ou `/sys`, que le runtime monte au démarrage (couvre aussi le conteneur arrêté, Opus Minor 4). Code `forbidden_path` (403) : le chemin est interdit en écriture, et le détail nomme la cause. Un dossier monté par bind reste inscriptible : Docker y écrit, le conteneur le voit.
  - *En plus, fermé par défaut (R15) :* sur un conteneur actif, une édition dont la cible est un point de montage (son `STATX_MNT_ID` diffère de celui de son parent, par `openat2` sans lien) est refusée. Cela couvre ce qu'`Inspect` ne montre pas : configs et secrets Swarm, `/sbin/docker-init` (`--init`, mesuré), fichiers CDI ou `--gpus`, et une `Destination` sous un lien du conteneur. Coût : une paire d'`openat2` + `statx` par édition ; noyau 5.8 pour `STATX_MNT_ID` (sinon l'édition est refusée, comme le tar de §5.4 b). Hors de portée : un envoi (la cible n'existe pas encore ; un envoi dans un tmpfs absent d'`Inspect` reste un faux succès) et un conteneur arrêté (pas de process, donc pas de table des montages). Limites écrites en §13. → Tâches 1, 2, 3, 5, 8.
- **Q22 — Écrire à travers un bind de l'hôte (à trancher par l'utilisateur).** Un conteneur arrêté dont un montage expose l'hôte est refusé en entier (§5.2 étape 7). `fsview.HostBind` ne vise que les binds dont la source est `/`, `/proc` ou `/sys` (et dessous), ou `/dev` exactement, plus les volumes `local` en `o=bind` vers ces sources, les volumes `type=` pseudo ou FUSE, les pilotes tiers et les options illisibles. Un bind d'un autre dossier de l'hôte (`-v /etc:/hostetc`, `-v /root:/r`, `-v /var/lib/docker:/d`) n'est **pas** refusé. Deux risques : l'utilisateur de l'interface écrit dans l'hôte par ce bind (il est déjà root de fait, §7) ; et, par la course que §5.3 assume, un process non root du conteneur remplace un dossier qu'il possède par un lien vers `/hostetc/cron.d`, et dockerd, qui extrait en root dans sa vue chrootée, y dépose le fichier choisi par l'utilisateur (« confused deputy » : le contenu n'est pas celui de l'attaquant, la destination si).
  - **(a) Portée actuelle.** `HostBind` tel quel sur toute écriture (actif comme arrêté). Un bind d'un dossier de l'hôte hors `/`, `/proc`, `/sys`, `/dev` reste inscriptible, et la course peut y mener Docker : limite écrite en §13 et dans §7. Test : `/etc` → `/hostetc` accepté (fige la portée). Coût en faux refus : nul de plus.
  - **(b) Règle étendue, propre à l'écriture.** En plus de `HostBind`, refuser pour tout le conteneur un bind (ou un volume `local` en `o=bind`) dont la source est l'une de ces racines ou en dessous : `/etc`, `/root`, `/home`, `/boot`, `/usr`, `/bin`, `/sbin`, `/lib`, `/lib32`, `/lib64`, `/opt`, `/var/lib/docker`, `/var/lib/containerd`, `/var/spool/cron`, `/run`, `/var/run`. Ce que ça change pour la course : plus aucun de ces dossiers n'est dans la vue de dockerd, la redirection ne peut atteindre que le rootfs du conteneur ou des binds de données (`/srv`, `/data`, `/mnt`…). Coût en faux refus, élevé : `-v /etc/localtime:/etc/localtime:ro` et `/etc/timezone` (très courants), `/var/run/docker.sock` (Portainer, Traefik, Watchtower), tout projet de développement monté depuis `/home/<user>/…` (Compose de dev), `/run/…` pour les sockets. Une variante ne viserait que les binds de **dossiers** (un bind de fichier ne peut pas servir de dossier parent à la course), mais `Inspect` ne dit pas si la source est un fichier : il faudrait lire la vue `procfs` du conteneur actif, et rien pour un conteneur arrêté.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* option (a) retenue, pour la v0.1. L'utilisateur est root de fait (§7) ; la course exige un process malveillant dans le conteneur, un timing serré et une écriture lancée par l'utilisateur au même moment, et dockerd reste chrooté dans sa vue ; (b) rendrait l'écriture impossible sur une grande partie des conteneurs réels (fuseau horaire, socket Docker, code monté depuis `/home`). Si l'utilisateur retient (b), la tâche 2 gagne un test (`/etc` refusé, `/srv/conf` accepté) et la tâche 3 une fonction `writeSensitive(src)` appelée dans `writable` sur les binds et les volumes `local` en `o=bind`. → Tâches 2, 3, 8.
- **Q23 — Portée du verrou.** Par conteneur, par chemin, ou global ?
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-10) :* par conteneur. Les écritures sont rares (un utilisateur, un clic confirmé chacune) : la contention est nulle. Limite : une écriture bloquée sur un FUSE muet (`StatNoLinks`, relecture) garde le verrou jusqu'au redémarrage de l'agent, et chaque écriture suivante sur ce conteneur reçoit `504` après 10 s, avec un message « l'écriture a peut-être eu lieu » trompeur (rien n'a commencé) : ligne §13. Un verrou par chemin demanderait de raisonner sur un envoi dans un dossier pendant l'édition d'un fichier voisin, et sur deux chemins qui désignent le même inode (lien physique). Un verrou global bloquerait sans raison deux conteneurs, dont l'un peut attendre une minute (`PutTimeout`). L'attente s'arrête avec la requête : au pire, la garde répond `504` (Q17). → Tâche 3.

## Global Constraints

- **Module et dépendances Go** : module `github.com/levy-cmd/hublot-kit` ; dépendances autorisées (§3) : SDK Docker `github.com/moby/moby/client`, `github.com/cyphar/filepath-securejoin` **v0.7.0** (`pathrs-lite`), `golang.org/x/sys` **v0.48.0**. Aucune autre. Interface : CodeMirror 6 seulement (§3 : « langages chargés à la demande : JSON, YAML, JavaScript, Python, shell, texte brut »), pas de bibliothèque de composants.
- **Accès aux fichiers d'un conteneur (CLAUDE.md, §5).** `files` n'ouvre aucun chemin : tous ses contrôles passent par `fsview.FS` (`StatNoLinks`, `Open`), toutes ses écritures par `dockerx.PutArchive`. **Jamais** `filepath.Join`, `path.Join` ni `os.Open` sur un chemin du conteneur. `PutArchive` reçoit exactement le parent du chemin vérifié (§5.3 étape 5).
- **Ordre de §5.3, dans une seule fonction** : lecture seule → contrôle du chemin (nom, `Inspect`, verrou, vue, point de montage) → `If-Match` / `If-None-Match` → audit (intention) → `PutArchive` → `Forget` → audit (résultat) → verrou rendu.
- **Tests d'évasion avant le code qu'ils protègent** (CLAUDE.md), voir « Ordre ». Chaque refus est prouvé par l'absence de tout appel à `PutArchive` et de toute ligne d'audit, **même quand une erreur est renvoyée**. Un piège a son contrôle (le lien mène bien hors de la racine). Un test ne se saute que sur une condition exacte, nommée dans le message.
- **Code Linux** testé sous Linux (`scripts/test-linux.sh`, VM) : `files_linux_test.go` (`//go:build linux`) ; `go test ./cmd/... ./internal/...` reste vert sur macOS. **Jamais** de test qui monte le `/proc`, le `/sys` ou le `/` de l'hôte dans un conteneur puis demande un tar ; `TestProcPseudoFSRefused` sert le `/proc` de l'hôte de test comme racine par `NewProcFS`, sans Docker ni tar.
- **Erreurs API** : `{"error": "<code>", "detail": "<texte>"}`, codes de §6 uniquement ; `202` porte en plus `progress`.
- **Interface** : `{@html}` interdit ; contenu du conteneur toujours échappé par Svelte (noms, détails d'erreur) ; bouton « Recharger » sur les états d'erreur existants ; garde contre les réponses périmées ; grilles en `minmax(0,1fr)` ; textes dans `fr.ts` et `en.ts` (mêmes clés) ; imports `#lib/….ts` dans les `.svelte`.
- **Node ≥ 22.17** : sur le Mac, `export PATH=/opt/homebrew/bin:$PATH` avant `npm`/`npx`. **Docker Desktop** : `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock`.
- **Commits** en français, terminés par une ligne vide puis `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Sorties filtrées** : chaque commande de vérification se termine par `| tail -n 20` (ou `-run` ciblé).

## Review Focus

1. **Le fichier change dans le conteneur entre l'ouverture et l'enregistrement** (un process le réécrit, ou un autre onglet l'a enregistré, même au même instant) : `conflict`, rien n'est écrit, et l'éditeur reste ouvert pour que l'utilisateur copie ses modifications. → Tâche 2 `TestEditIfMatch`, `TestWritesAreSerialized` ; tâche 5 `TestIntegrationWriteStopped` (ancien `If-Match` → 409) ; tâche 7 (éditeur gardé, parcours jetable).
2. **La requête s'arrête pendant l'écriture** (onglet fermé, délai de 10 s de la garde, réseau coupé) : l'écriture commencée va au bout et son résultat est journalisé, sans fichier tronqué par une coupure de notre côté. → Tâche 2 `TestPutSurvivesRequestEnd`.
3. **Fichier non UTF-8, avec BOM, en CRLF ou à fins de ligne mélangées** : jamais réécrit en silence. → Tâche 6 `decodeUtf8`, `lineSeparatorOf`, `docText`, `previews text` (`utf8: false`) ; tâche 7 (bouton absent, message, aller-retour CRLF du parcours jetable).
4. **Nom d'envoi inhabituel** (`a/b` brut ou encodé RFC 2231, `..`, vide, octet nul, saut de ligne, `\`, non UTF-8, 256 octets, nom d'un lien pendant, nom existant, deux envois simultanés) : refus, rien n'est écrit, rien n'est remplacé. → Tâche 2 `TestUploadInvalidName`, `TestUploadNeverReplaces`, `TestWritesAreSerialized`, `TestProcLinksRefused` ; tâche 4 `TestWriteRoutesErrors`, `TestUploadNameIsTheDecodedValue`.
5. **Cible que Docker n'écrit pas comme le conteneur la voit** (`--read-only`, volume `:ro`, fichier monté, `/etc/hosts`, tmpfs, `/dev`, `/sbin/docker-init` d'un `--init`) : un refus clair qui nomme la cause, jamais `docker_unavailable` ni un faux succès. → Tâche 1 `TestPutArchiveRealDocker` ; tâche 2 `TestRefusesWhereDockerDoesNotWrite`, `TestEditRefusesMountPoint`, `TestIsMountPoint` ; tâche 4 `TestWriteDockerRefused` ; tâche 5 `TestIntegrationWriteStopped`, `TestIntegrationWriteRunning`.

## Ordre : chaque test d'évasion précède le code qu'il protège

| Protection (spec) | Tests écrits | Code protégé |
|---|---|---|
| Aucun lien dans la chaîne, final ou parent, lien pendant compris ; rien hors de la racine (§5.3 ét. 2) | Tâche 2 (`TestProcLinksRefused`, `TestLinksRefused`) | Tâche 3 |
| Non régulier refusé sans bloquer (FIFO), pseudo-système refusé (`fstatfs`) (§5.3 ét. 2) | Tâche 2 (`TestProcNotRegular`, `TestProcPseudoFSRefused`) | Tâche 3 |
| Lecture seule d'abord, `If-Match`, `If-None-Match`, noms d'envoi (§5.3 ét. 1 à 3) | Tâche 2 | Tâche 3 |
| Ordre audit → écriture → audit, chemin exact envoyé à `PutArchive`, index retiré (§5.3 ét. 4 à 6) | Tâche 2 (`TestEditOrderAndTar`, `TestProcWriteSendsExactPath`, `TestStoppedContainerIndexDropped`) | Tâche 3 |
| Écriture refusée pour un conteneur dont un montage tombe sous la règle de §5.2 ét. 7 (bind de `/`, `/proc`, `/sys`, `/dev`…), actif compris ; bind de `/etc` accepté (Q22 a) | Tâche 2 (`TestHostBindRefusesEveryWrite`) | Tâche 3 |
| Pas d'écriture là où Docker n'écrit pas comme le conteneur voit (Q21), point de montage hors `Inspect` compris | Tâche 2 (`TestRefusesWhereDockerDoesNotWrite`, `TestEditRefusesMountPoint`, `TestIsMountPoint`) | Tâche 3 |
| Écritures d'un conteneur sérialisées, verrou rendu en cas d'abandon ou de panique (Q23) ; nom avant l'`Inspect`, chemin avant la précondition | Tâche 2 (`TestWritesAreSerialized`, `TestLockWaitAbandoned`, `TestLockReleasedOnPanic`, `TestUploadNameBeforeInspectRules`, `TestUploadChecksPathBeforePrecondition`) | Tâche 3 |
| Câblage par les routes : session, `Origin`, codes, nom décodé validé | Tâche 4, étape 1 | Tâche 4, étape 3 |

**Ne pas pousser la branche entre les tâches 2 et 3** : le paquet `files` n'y compile pas.

## Où tester

| Repère | Commande (racine du dépôt) |
|---|---|
| **macOS** | `go test ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; interface : `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 \| tail -n 15 && npm run check 2>&1 \| tail -n 5 && npm run lint 2>&1 \| tail -n 5` |
| **Docker Desktop (API Docker, conteneur arrêté)** | `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ ./internal/api/ -run 'PutArchive\|IntegrationWriteStopped' 2>&1 \| tail -n 20` |
| **Linux root** | `scripts/test-linux.sh -race ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; avec Docker : ajouter `-tags integration` |
| **VM Ubuntu / Fedora** | `limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 \| tail -n 30` (idem `hk-fedora`) |
| **Parcours Playwright** | `export PATH=/opt/homebrew/bin:$PATH && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock scripts/e2e.sh 2>&1 \| tail -n 20` |

## Structure des fichiers

```
internal/dockerx/dockerx.go                  + PutArchive (interface et moteur), ErrRefused, ErrPutInvalid, wrapPut, ContainerInfo.Tmpfs
internal/fsview/fsview.go                    + MountPointChecker (interface facultative)
internal/fsview/mountpoint_linux.go (+ test) procFS.IsMountPoint (STATX_MNT_ID de la cible et du parent)
internal/dockerx/fake.go                     + PutErr, Put, PutCalls()
internal/files/files.go                      Writer : Allowed, Edit, Upload, ETag ; ordre de §5.3, writable, verrou
internal/files/files_test.go                 tests portables (MemFS, faux Docker, vraie Views)
internal/files/files_linux_test.go           tests d'évasion sur un vrai procfs (racine temporaire)
internal/api/fs_write.go                     PUT /fs/file, POST /fs/upload, lecture du multipart
internal/api/server.go                       + server.files, routes
internal/api/fserr.go                        + read_only, conflict (ErrPutInvalid compris), invalid_name, MaxBytesError
internal/api/fs.go                           fsRequest(w, r, param)
internal/api/fs_download.go                  appel de fsRequest(w, r, "path")
internal/api/fs_preview.go                   ETag par files.ETag
internal/api/fs_write_test.go                routes, faux Docker
internal/api/integration_write_test.go       conteneur arrêté, vrai Docker (portable)
internal/api/integration_write_linux_test.go conteneur actif, vrai Docker (Linux root)
web/package.json, web/package-lock.json      CodeMirror 6
web/src/lib/write.ts (+ test)                plafonds, décodage strict, fins de ligne, langage, messages
web/src/lib/editor.ts (+ test)               chargement des langages à la demande, docText
web/src/lib/api.ts (+ test)                  saveFile, uploadFile, Preview.utf8, Content-Type
web/src/lib/i18n/fr.ts, en.ts                clés write.* et confirm.*
web/src/lib/components/FileEditor.svelte     CodeMirror chargé à l'ouverture
web/src/lib/components/ConfirmDialog.svelte  confirmation : conteneur, chemin
web/src/lib/components/FilePreview.svelte    bouton Modifier, enregistrement
web/src/lib/components/FileBrowser.svelte    bouton d'envoi
web/src/routes/+layout.svelte                contexte readOnly (un seul /api/meta)
web/src/routes/c/[id]/+page.svelte           readOnly lu dans le contexte
```

---

### Task 1: `dockerx.PutArchive`, refus de Docker classés, tmpfs de l'`Inspect`

**Où tester :** macOS (unitaires), Docker Desktop (intégration, portable).

**Files:**
- Modify: `internal/dockerx/dockerx.go`, `internal/dockerx/fake.go`
- Test: `internal/dockerx/dockerx_test.go`, `internal/dockerx/integration_test.go`

**Interfaces:**
- Consumes: `wrap`, `client.CopyToContainer` (`client.CopyToContainerOptions{DestinationPath, Content}`, SDK v0.6.1).
- Produces: `Client.PutArchive(ctx context.Context, id, dir string, content io.Reader) error` ; `dockerx.ErrRefused` (lecture seule) et `dockerx.ErrPutInvalid` (tout autre refus d'extraction), produits par `PutArchive` seulement, via `wrapPut` ; `ContainerInfo.Tmpfs []string` (destinations de `--tmpfs`, triées) ; `Fake.PutErr error`, `type Put struct{ ID, Dir string; Tar []byte }`, `(*Fake).PutCalls() []Put` (copie sous verrou ; un appel en échec est enregistré aussi).

> **À comprendre :** `PUT /containers/{id}/archive` (`docker cp` vers le conteneur) extrait un tar dans un dossier. Il marche sur un conteneur actif, arrêté ou en pause. Docker refuse lui-même d'écrire sur un rootfs `--read-only` ou un volume `:ro`, avec une erreur `InvalidArgument` dont le message dit « marked read-only » (mesuré) : `wrapPut` la classe à part (`ErrRefused`), sinon l'API répondrait « Docker indisponible ». Un autre `InvalidArgument`, comme « extraction point is not a directory » quand le dossier vérifié est devenu un fichier, devient `ErrPutInvalid` (`conflict`). Seul `PutArchive` utilise `wrapPut` : sur une route de lecture, un `InvalidArgument` reste `docker_unavailable` (Q15). `copyUIDGID` n'est pas demandé : il donnerait le fichier à l'utilisateur du conteneur au lieu de garder l'uid/gid du tar. `ContainerInfo.Tmpfs` lit `HostConfig.Tmpfs` : un `--tmpfs` n'apparaît pas dans `Mounts`, et Docker n'y écrit pas (Q21).

- [ ] **Step 1: Tests unitaires qui échouent** — ajouter à `internal/dockerx/dockerx_test.go` (et `"strings"` aux imports) :

```diff
--- a/internal/dockerx/dockerx_test.go
+++ b/internal/dockerx/dockerx_test.go
@@ -8,6 +8,7 @@ import (
 	"net/netip"
 	"reflect"
 	"slices"
+	"strings"
 	"testing"
 	"time"
 
@@ -181,3 +182,56 @@ func TestFakeSizesDiffsCalls(t *testing.T) {
 		t.Fatalf("appels : %v", c)
 	}
 }
+
+// Q15: read_only only for the two read-only refusals Docker gives (measured
+// messages); any other InvalidArgument of PutArchive is ErrPutInvalid
+// (conflict: the path changed under us). On the read routes an
+// InvalidArgument stays docker_unavailable.
+func TestWrapPut(t *testing.T) {
+	invalid := func(msg string) error {
+		return fmt.Errorf("Error response from daemon: %s: %w", msg, cerrdefs.ErrInvalidArgument)
+	}
+	for _, msg := range []string{"container rootfs is marked read-only", "mounted volume is marked read-only"} {
+		if err := wrapPut(invalid(msg)); !errors.Is(err, ErrRefused) || errors.Is(err, ErrPutInvalid) || errors.Is(err, ErrUnavailable) {
+			t.Errorf("%s : %v, attendu ErrRefused", msg, err)
+		}
+	}
+	if err := wrapPut(invalid("extraction point is not a directory")); !errors.Is(err, ErrPutInvalid) || errors.Is(err, ErrRefused) {
+		t.Errorf("point d'extraction : %v, attendu ErrPutInvalid", err)
+	}
+	if err := wrapPut(fmt.Errorf("put: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) {
+		t.Errorf("introuvable : %v", err)
+	}
+	if err := wrap(invalid("container rootfs is marked read-only")); errors.Is(err, ErrRefused) || !errors.Is(err, ErrUnavailable) {
+		t.Errorf("hors PutArchive, InvalidArgument reste ErrUnavailable : %v", err)
+	}
+}
+
+// Q21: the --tmpfs destinations (HostConfig.Tmpfs), sorted.
+func TestFromInspectTmpfs(t *testing.T) {
+	got := fromInspect(container.InspectResponse{ID: "x", HostConfig: &container.HostConfig{Tmpfs: map[string]string{"/tmp": "", "/run": "size=1m"}}})
+	if !reflect.DeepEqual(got.Tmpfs, []string{"/run", "/tmp"}) {
+		t.Fatalf("Tmpfs : %v", got.Tmpfs)
+	}
+	if got := fromInspect(container.InspectResponse{ID: "x"}); got.Tmpfs != nil {
+		t.Fatalf("sans HostConfig : %v", got.Tmpfs)
+	}
+}
+
+func TestFakePutArchive(t *testing.T) {
+	f := &Fake{}
+	if err := f.PutArchive(context.Background(), "abc", "/etc", strings.NewReader("TAR")); err != nil {
+		t.Fatal(err)
+	}
+	puts := f.PutCalls()
+	if len(puts) != 1 || puts[0].ID != "abc" || puts[0].Dir != "/etc" || string(puts[0].Tar) != "TAR" {
+		t.Fatalf("appels : %+v", puts)
+	}
+	f.PutErr = ErrRefused
+	if err := f.PutArchive(context.Background(), "abc", "/etc", strings.NewReader("")); !errors.Is(err, ErrRefused) {
+		t.Fatalf("PutErr : %v", err)
+	}
+	if len(f.PutCalls()) != 2 {
+		t.Fatal("un appel en échec est enregistré aussi")
+	}
+}
```

Run: `go vet ./internal/dockerx/ 2>&1 | tail -n 3` — Expected: échec de compilation (`undefined: wrapPut`, puis `ErrRefused`, `ErrPutInvalid`…).

- [ ] **Step 2: Implémenter** — dans `internal/dockerx/dockerx.go` et `internal/dockerx/fake.go` :

```diff
--- a/internal/dockerx/dockerx.go
+++ b/internal/dockerx/dockerx.go
@@ -42,6 +42,9 @@ type ContainerInfo struct {
 	// cache key (spec §5.2 step 1). "0001-01-01T00:00:00Z" if never started.
 	FinishedAt string
 	Mounts     []Mount
+	// Tmpfs: destinations of --tmpfs (HostConfig.Tmpfs), sorted. Absent from
+	// Mounts; Docker never writes into them (plan Explorateur 3, Q21).
+	Tmpfs []string
 }
 
 // Change is one entry of GET /containers/{id}/changes (F5).
@@ -60,9 +63,16 @@ type Mount struct {
 }
 
 // ErrNotFound: no such container (or no such path, for an archive).
+// ErrRefused: Docker refused a write because its target is read-only
+// (measured on Docker 29.7: "container rootfs is marked read-only",
+// "mounted volume is marked read-only").
+// ErrPutInvalid: any other InvalidArgument of PutArchive, e.g. "extraction
+// point is not a directory": the path changed after the agent's checks.
 // ErrUnavailable: any other failure of the daemon or of the connection to it.
 var (
 	ErrNotFound    = errors.New("introuvable dans Docker")
+	ErrRefused     = errors.New("refusé par Docker")
+	ErrPutInvalid  = errors.New("extraction refusée par Docker")
 	ErrUnavailable = errors.New("Docker indisponible")
 )
 
@@ -83,6 +93,12 @@ type Client interface {
 	SizeRootFs(ctx context.Context, id string) (int64, error)
 	// Changes is GET /containers/{id}/changes (F5): never nil.
 	Changes(ctx context.Context, id string) ([]Change, error)
+	// PutArchive extracts the tar read from content into directory dir of the
+	// container (PUT /containers/{id}/archive). dir is sent as is: package
+	// files gives the parent of the path it checked (spec §5.3 step 5).
+	// Docker never replaces a directory by a file (noOverwriteDirNonDir) and
+	// keeps the UID/GID of the tar headers (no copyUIDGID).
+	PutArchive(ctx context.Context, id, dir string, content io.Reader) error
 }
 
 type engine struct{ c *client.Client }
@@ -181,6 +197,12 @@ func fromInspect(c container.InspectResponse) ContainerInfo {
 			Type: string(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: m.Driver,
 		})
 	}
+	if c.HostConfig != nil {
+		for d := range c.HostConfig.Tmpfs {
+			info.Tmpfs = append(info.Tmpfs, d)
+		}
+		sort.Strings(info.Tmpfs)
+	}
 	return info
 }
 
@@ -238,6 +260,28 @@ func fromChanges(cs []container.FilesystemChange) ([]Change, error) {
 	return out, nil
 }
 
+func (e *engine) PutArchive(ctx context.Context, id, dir string, content io.Reader) error {
+	_, err := e.c.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: dir, Content: content})
+	if err != nil {
+		return wrapPut(err)
+	}
+	return nil
+}
+
+// wrapPut: wrap, plus Docker's own refusals of a write, InvalidArgument:
+// the target is read-only (both measured messages say "marked read-only") →
+// ErrRefused; anything else → ErrPutInvalid. Only for PutArchive: on a read
+// route an InvalidArgument stays docker_unavailable (plan Explorateur 3, Q15).
+func wrapPut(err error) error {
+	if cerrdefs.IsInvalidArgument(err) {
+		if strings.Contains(err.Error(), "marked read-only") {
+			return fmt.Errorf("%w : %v", ErrRefused, err)
+		}
+		return fmt.Errorf("%w : %v", ErrPutInvalid, err)
+	}
+	return wrap(err)
+}
+
 // wrap classifies a daemon error for the API (not_found / docker_unavailable).
 func wrap(err error) error {
 	if cerrdefs.IsNotFound(err) {
```

```diff
--- a/internal/dockerx/fake.go
+++ b/internal/dockerx/fake.go
@@ -14,7 +14,7 @@ import (
 // are safe: an index is built in a goroutine, so GetArchive may run while the
 // test reads Calls().
 type Fake struct {
-	mu         sync.Mutex // guards ArchiveCalls
+	mu         sync.Mutex // guards ArchiveCalls and the PutArchive calls
 	Containers []Container
 	// Infos answers Inspect, by full ID or by name (tests add both keys).
 	Infos map[string]ContainerInfo
@@ -30,7 +30,17 @@ type Fake struct {
 	ArchiveCloses atomic.Int32
 	// Volumes answers VolumeOptions, by volume name.
 	Volumes map[string]map[string]string
-	Err     error
+	// PutErr answers PutArchive (nil: success); every call is recorded,
+	// failed ones included (read them with PutCalls).
+	PutErr error
+	puts   []Put
+	Err    error
+}
+
+// Put is one PutArchive call: the tar is read whole.
+type Put struct {
+	ID, Dir string
+	Tar     []byte
 }
 
 func (f *Fake) ListContainers(context.Context) ([]Container, error) {
@@ -83,6 +93,27 @@ func (f *Fake) VolumeOptions(_ context.Context, name string) (map[string]string,
 	return nil, fmt.Errorf("%w : volume %s", ErrNotFound, name)
 }
 
+func (f *Fake) PutArchive(_ context.Context, id, dir string, content io.Reader) error {
+	b, err := io.ReadAll(content)
+	if err != nil {
+		return err
+	}
+	f.mu.Lock()
+	f.puts = append(f.puts, Put{ID: id, Dir: dir, Tar: b})
+	f.mu.Unlock()
+	if f.Err != nil {
+		return f.Err
+	}
+	return f.PutErr
+}
+
+// PutCalls returns a copy of the PutArchive calls, read under the lock.
+func (f *Fake) PutCalls() []Put {
+	f.mu.Lock()
+	defer f.mu.Unlock()
+	return slices.Clone(f.puts)
+}
+
 // Calls returns a copy of ArchiveCalls, read under the lock.
 func (f *Fake) Calls() []string {
 	f.mu.Lock()
```

Run: `go test ./internal/dockerx/ 2>&1 | tail -n 3 && go vet ./... 2>&1 | tail -n 3` — Expected: `ok`, aucun autre implémenteur de `Client` à compléter (les doubles de test embarquent `*Fake`).

- [ ] **Step 3: Test sur un vrai Docker** — ajouter à `internal/dockerx/integration_test.go` (et `"bytes"`, `"io"`, `"slices"`, `"strings"` aux imports) :

```diff
--- a/internal/dockerx/integration_test.go
+++ b/internal/dockerx/integration_test.go
@@ -4,9 +4,13 @@ package dockerx
 
 import (
 	"archive/tar"
+	"bytes"
 	"context"
 	"errors"
+	"io"
 	"os/exec"
+	"slices"
+	"strings"
 	"testing"
 	"time"
 )
@@ -136,3 +140,95 @@ func TestMeasureSizeRootFs(t *testing.T) {
 	}
 	t.Logf("MESURE : SizeRootFs = %d octets", size) // 0: the progress stays at 0 % (not an error)
 }
+
+// oneFileTar is the tar package files sends: one regular entry, base name.
+func oneFileTar(t *testing.T, name, body string, mode int64, uid, gid int) *bytes.Buffer {
+	t.Helper()
+	var b bytes.Buffer
+	tw := tar.NewWriter(&b)
+	if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Uid: uid, Gid: gid,
+		Size: int64(len(body)), ModTime: time.Now()}); err != nil {
+		t.Fatal(err)
+	}
+	io.WriteString(tw, body)
+	if err := tw.Close(); err != nil {
+		t.Fatal(err)
+	}
+	return &b
+}
+
+// PutArchive on a running and on a stopped container: content, mode and
+// owner come from the tar header; the existing file is replaced by a new
+// inode (Docker removes, then creates); Docker's refusals are classified.
+func TestPutArchiveRealDocker(t *testing.T) {
+	run, stopped, ro, rov := "hublot-it-put", "hublot-it-put-stopped", "hublot-it-put-ro", "hublot-it-put-rov"
+	exec.Command("docker", "rm", "-f", run, stopped, ro, rov).Run()
+	for _, args := range [][]string{
+		{"run", "-d", "--name", run, "--tmpfs", "/scratch", "alpine:3", "sleep", "300"},
+		{"create", "--name", stopped, "alpine:3", "true"},
+		{"run", "-d", "--name", ro, "--read-only", "alpine:3", "sleep", "300"},
+		{"run", "-d", "--name", rov, "-v", "hublot-it-put-rov:/data:ro", "alpine:3", "sleep", "300"},
+	} {
+		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
+			t.Fatalf("docker %v : %v %s", args, err, out)
+		}
+	}
+	t.Cleanup(func() {
+		exec.Command("docker", "rm", "-f", run, stopped, ro, rov).Run()
+		exec.Command("docker", "volume", "rm", "hublot-it-put-rov").Run()
+	})
+	c, err := New()
+	if err != nil {
+		t.Fatal(err)
+	}
+	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
+	defer cancel()
+
+	inode := func() string {
+		out, _ := exec.Command("docker", "exec", run, "stat", "-c", "%i", "/etc/motd").Output()
+		return strings.TrimSpace(string(out))
+	}
+	before := inode()
+	if err := c.PutArchive(ctx, run, "/etc", oneFileTar(t, "motd", "bonjour\n", 0o640, 1000, 1001)); err != nil {
+		t.Fatalf("conteneur actif : %v", err)
+	}
+	out, err := exec.Command("docker", "exec", run, "stat", "-c", "%a %u %g %s", "/etc/motd").Output()
+	if err != nil || strings.TrimSpace(string(out)) != "640 1000 1001 8" {
+		t.Fatalf("mode et propriétaire : %q %v", out, err)
+	}
+	t.Logf("MESURE inode de /etc/motd : %s avant, %s après", before, inode())
+
+	if err := c.PutArchive(ctx, stopped, "/etc", oneFileTar(t, "hublot.txt", "arrêté\n", 0o644, 0, 0)); err != nil {
+		t.Fatalf("conteneur arrêté : %v", err)
+	}
+	rc, err := c.GetArchive(ctx, stopped, "/etc/hublot.txt")
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer rc.Close()
+	tr := tar.NewReader(rc)
+	if _, err := tr.Next(); err != nil {
+		t.Fatal(err)
+	}
+	if b, _ := io.ReadAll(tr); string(b) != "arrêté\n" {
+		t.Fatalf("relu : %q", b)
+	}
+
+	if err := c.PutArchive(ctx, ro, "/etc", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrRefused) {
+		t.Fatalf("rootfs en lecture seule : %v, attendu ErrRefused", err)
+	}
+	if err := c.PutArchive(ctx, rov, "/data", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrRefused) {
+		t.Fatalf("volume :ro : %v, attendu ErrRefused", err)
+	}
+	// Q21: the --tmpfs destinations come from Inspect (they are not in Mounts).
+	if info, err := c.Inspect(ctx, run); err != nil || !slices.Equal(info.Tmpfs, []string{"/scratch"}) {
+		t.Fatalf("Tmpfs : %v %v", info.Tmpfs, err)
+	}
+	// A file where a directory was checked (the path changed under us).
+	if err := c.PutArchive(ctx, run, "/etc/motd", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrPutInvalid) {
+		t.Fatalf("point d'extraction qui n'est pas un dossier : %v, attendu ErrPutInvalid", err)
+	}
+	if err := c.PutArchive(ctx, run, "/absent", oneFileTar(t, "x", "x", 0o644, 0, 0)); !errors.Is(err, ErrNotFound) {
+		t.Fatalf("dossier absent : %v, attendu ErrNotFound", err)
+	}
+}
```

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ -run PutArchive -v 2>&1 | grep -E '^(---|ok|FAIL)|MESURE' | tail -n 5`
Expected: `--- PASS: TestPutArchiveRealDocker` (rootfs `--read-only` et volume `:ro` → `ErrRefused`, extraction sur le fichier `/etc/motd` → `ErrPutInvalid`, `Tmpfs` = `[/scratch]`) ; la ligne `MESURE` montre deux inodes différents (Docker supprime puis recrée le fichier : base de Q17).

- [ ] **Step 4: Commit**

```bash
git add internal/dockerx
git commit -F - <<'EOF'
dockerx: PutArchive, refus de Docker classés à part (ErrRefused), tmpfs de l'Inspect

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 2: Tests du paquet `files` (rouges)

**Où tester :** macOS (compilation) et Linux (`GOOS=linux go vet`) ; ils passeront à la tâche 3, sous macOS et sous `scripts/test-linux.sh`.

**Files:**
- Create: `internal/files/files_test.go`, `internal/files/files_linux_test.go`, `internal/fsview/mountpoint_linux_test.go`

**Interfaces:**
- Consumes: `fsview.MemFS` (`NewMemFS`, `AddDir`, `AddFile`, `AddLink`, `Entries`, `Err`), `fsview.NewProcFS`, `fsview.NewViews`, `fsview.Options{}`, `dockerx.Fake` (`PutErr`, `PutCalls`, `Calls`), `dockerx.ContainerInfo` (`Mounts`, `Tmpfs`), `audit.Event`.
- Produces (attendu par les tests, écrit à la tâche 3) : `files.Writer{ReadOnly bool; Docker dockerx.Client; Audit Auditor; Open func(context.Context, string) (fsview.FS, error); Forget func(string)}`, `(*Writer).Allowed() error`, `(*Writer).Edit(ctx context.Context, c dockerx.ContainerInfo, p string, content []byte, ifMatch string) (string, error)`, `(*Writer).Upload(ctx context.Context, c dockerx.ContainerInfo, dir, name string, content []byte, ifNoneMatch string) (string, error)`, `files.ETag([]byte) string`, `MaxEditBytes`, `MaxUploadBytes`, `ErrReadOnly`, `ErrConflict`, `ErrInvalidName` ; dans `fsview` : `type MountPointChecker interface{ IsMountPoint(ctx context.Context, path string) (bool, error) }`, implémentée par `procFS`.

> **À comprendre :** deux familles de tests. Les tests portables (`files_test.go`) jouent le conteneur avec `MemFS`, qui refuse les liens comme `procfs`, et un faux Docker qui enregistre les tars reçus. Un `journal` unique note, dans l'ordre, les lignes d'audit et les appels à `PutArchive` : l'ordre de §5.3 se lit d'un coup d'œil (`audit:intent`, `put:/etc`, `audit:ok`). `TestStoppedContainerIndexDropped` passe par une vraie `Views` : pendant l'indexation, l'écriture reçoit `indexing` ; après, l'index est retiré et l'accès suivant relance `GetArchive("/")`.
>
> Les refus tirés de l'`Inspect` (Q21, Q22) se jouent aussi sur `MemFS` : ils doivent tomber **avant** d'ouvrir la vue (`r.opens == 0`). Le fichier de test est `/etc/app.conf`, puisque `/etc/hosts` est désormais refusé. `TestWritesAreSerialized` bloque le premier `PutArchive` (`gatedPutter`), lance la seconde écriture, et vérifie qu'elle n'atteint pas `PutArchive` pendant 200 ms ; puis la première applique son écriture à la vue, et la seconde doit finir en `conflict`. `TestLockWaitAbandoned` annule une requête qui attend le verrou : elle rend `context.Canceled` sans rien écrire, et une écriture suivante passe. `TestLockReleasedOnPanic` fait paniquer `Open` une fois le verrou pris. `TestEndedRequestWritesNothing` annule le contexte **dans** `Open`, donc verrou pris : seul le contrôle de `put` peut l'arrêter (`MemFS` ignore le contexte). `TestEditRefusesMountPoint` enveloppe `MemFS` dans une vue qui déclare `/etc/app.conf` point de montage, avec son contrôle (la même vue sans point de montage écrit).
>
> Les tests Linux (`files_linux_test.go`) servent une racine temporaire par un vrai `procfs` : `openat2 RESOLVE_NO_SYMLINKS` et `fstatfs` décident, pas une imitation. `/evil` mène à un dossier de l'hôte : le contrôle vérifie que le piège marche ; l'assertion vérifie que `PutArchive` n'est **jamais** appelé, même quand une erreur est renvoyée. Un appel au faux Docker n'écrit rien, mais il prouve que le vrai aurait écrit. `TestProcPseudoFSRefused` sert le `/proc` de l'hôte comme racine : `/` et `/version` ne commencent pas par `/proc`, la règle de préfixe de Q21 les laisse passer, et seul `fstatfs` les refuse (R9). `TestIsMountPoint` (dans `fsview`) prend `/proc`, point de montage sur tout Linux, hôte ou conteneur, et `/etc/passwd`, qui n'en est pas un : aucun privilège, aucun montage à créer.

- [ ] **Step 1: Tests portables** — créer `internal/files/files_test.go` :

```go
package files

// Tests of the only writer (spec §5.3, §10), written before the code. An
// in-memory view (fsview.MemFS) plays the container; a fake Docker records
// what PutArchive receives; a recorder keeps the order audit → write → audit.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

const cid = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// ctr is the Inspect of the test container: running, no mount.
var ctr = dockerx.ContainerInfo{ID: cid, Name: "web", Running: true, Pid: 42}

// journal records audit events and PutArchive calls in one ordered list.
type journal struct {
	mu       sync.Mutex
	steps    []string
	events   []audit.Event
	auditErr error             // returned by Write when set
	onEvent  func(audit.Event) // called by Write, outside the lock
}

func (j *journal) Write(e audit.Event) error {
	if j.onEvent != nil {
		j.onEvent(e)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.auditErr != nil {
		return j.auditErr
	}
	j.steps = append(j.steps, "audit:"+e.Result)
	j.events = append(j.events, e)
	return nil
}

func (j *journal) record(s string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.steps = append(j.steps, s)
}

func (j *journal) Steps() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.steps)
}

// putter wraps the fake Docker: it records the call in the journal and the
// state of the context it received.
type putter struct {
	*dockerx.Fake
	j      *journal
	ctxErr error
}

func (p *putter) PutArchive(ctx context.Context, id, dir string, r io.Reader) error {
	p.j.record("put:" + dir)
	p.ctxErr = ctx.Err()
	return p.Fake.PutArchive(ctx, id, dir, r)
}

type rig struct {
	w      *Writer
	mem    *fsview.MemFS
	fake   *dockerx.Fake
	put    *putter
	j      *journal
	opens  int
	forgot []string
}

func newRig(t *testing.T, readOnly bool) *rig {
	t.Helper()
	r := &rig{mem: fsview.NewMemFS(), fake: &dockerx.Fake{}, j: &journal{}}
	r.put = &putter{Fake: r.fake, j: r.j}
	r.mem.AddDir("/etc").AddFile("/etc/app.conf", []byte("127.0.0.1 localhost\n"))
	r.mem.Entries["/etc"] = fsview.Entry{Name: "etc", Type: fsview.TypeDir, Mode: 0o755, UID: 7, GID: 8}
	r.mem.Entries["/etc/app.conf"] = fsview.Entry{Name: "app.conf", Type: fsview.TypeFile, Mode: 0o640 | fs.ModeSetgid, UID: 101, GID: 102, Size: 20}
	r.w = &Writer{
		ReadOnly: readOnly,
		Docker:   r.put,
		Audit:    r.j,
		Open: func(_ context.Context, id string) (fsview.FS, error) {
			r.opens++
			if id != cid {
				t.Errorf("vue ouverte sur %q, attendu l'ID complet", id)
			}
			return r.mem, nil
		},
		Forget: func(id string) { r.forgot = append(r.forgot, id) },
	}
	return r
}

var confTag = ETag([]byte("127.0.0.1 localhost\n"))

// nothingWritten: no audit line, no PutArchive.
func (r *rig) nothingWritten(t *testing.T) {
	t.Helper()
	if s := r.j.Steps(); len(s) != 0 {
		t.Fatalf("rien ne doit être écrit ni journalisé : %v", s)
	}
	if n := len(r.fake.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
}

// entryOf reads the single entry of a tar sent to PutArchive, to io.EOF.
func entryOf(t *testing.T, b []byte) (*tar.Header, string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(b))
	h, err := tr.Next()
	if err != nil {
		t.Fatalf("tar : %v", err)
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("une seule entrée attendue : %v", err)
	}
	return h, string(body)
}

func TestETag(t *testing.T) {
	// SHA-256 of "abc", quoted: the ETag header of the preview (spec §6).
	if got := ETag([]byte("abc")); got != `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"` {
		t.Fatalf("ETag : %s", got)
	}
}

func TestReadOnlyRefusesFirst(t *testing.T) {
	r := newRig(t, true)
	if err := r.w.Allowed(); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Allowed : %v", err)
	}
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Edit : %v", err)
	}
	// Read-only beats every other refusal, an invalid name included.
	if _, err := r.w.Upload(ctx, ctr, "/etc", "a/b", []byte("x"), "*"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Upload : %v", err)
	}
	if r.opens != 0 {
		t.Fatal("en lecture seule, la vue du conteneur ne s'ouvre même pas")
	}
	r.nothingWritten(t)
}

var ctx = context.Background()

func TestEditIfMatch(t *testing.T) {
	for _, tag := range []string{"", "*", `W/` + confTag, strings.Trim(confTag, `"`), ETag([]byte("autre"))} {
		r := newRig(t, false)
		if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), tag); !errors.Is(err, ErrConflict) {
			t.Errorf("If-Match %q : %v, attendu ErrConflict", tag, err)
		}
		r.nothingWritten(t)
	}
}

func TestEditOrderAndTar(t *testing.T) {
	r := newRig(t, false)
	got, err := r.w.Edit(ctx, ctr, "/etc/../etc//app.conf", []byte("10.0.0.1 db\n"), confTag)
	if err != nil {
		t.Fatal(err)
	}
	if got != ETag([]byte("10.0.0.1 db\n")) {
		t.Errorf("nouvel ETag : %s", got)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "audit:ok"}) {
		t.Fatalf("ordre : %v, attendu audit → écriture → audit", s)
	}
	for _, e := range r.j.events {
		if e.Action != "file.write" || e.Container != cid || e.Path != "/etc/app.conf" {
			t.Errorf("événement : %+v", e)
		}
	}
	puts := r.fake.PutCalls()
	if len(puts) != 1 || puts[0].ID != cid || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
	h, body := entryOf(t, puts[0].Tar)
	if h.Name != "app.conf" || h.Typeflag != tar.TypeReg || body != "10.0.0.1 db\n" || h.Size != int64(len(body)) {
		t.Errorf("entrée : %+v %q", h, body)
	}
	if h.Mode != 0o2640 || h.Uid != 101 || h.Gid != 102 {
		t.Errorf("mode %o uid %d gid %d : ceux du fichier existant attendus (2640, 101, 102)", h.Mode, h.Uid, h.Gid)
	}
	if !slices.Equal(r.forgot, []string{cid}) {
		t.Errorf("Forget : %v", r.forgot)
	}
}

func TestUploadOrderAndTar(t *testing.T) {
	r := newRig(t, false)
	got, err := r.w.Upload(ctx, ctr, "/etc/", "new.conf", []byte("port=80\n"), "*")
	if err != nil || got != "/etc/new.conf" {
		t.Fatalf("Upload : %q %v", got, err)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "audit:ok"}) {
		t.Fatalf("ordre : %v", s)
	}
	if e := r.j.events[0]; e.Action != "file.upload" || e.Path != "/etc/new.conf" {
		t.Errorf("événement : %+v", e)
	}
	h, body := entryOf(t, r.fake.PutCalls()[0].Tar)
	if h.Name != "new.conf" || body != "port=80\n" || h.Mode != 0o644 || h.Uid != 7 || h.Gid != 8 {
		t.Errorf("nouveau fichier : 0644 et uid/gid du dossier parent attendus : %+v", h)
	}
	// At the root, the parent sent is "/" and the path written "/x".
	r = newRig(t, false)
	if got, err := r.w.Upload(ctx, ctr, "/", "x", nil, "*"); err != nil || got != "/x" || r.fake.PutCalls()[0].Dir != "/" {
		t.Fatalf("racine : %q %v", got, err)
	}
}

func TestUploadInvalidName(t *testing.T) {
	for _, n := range []string{"", ".", "..", "a/b", "/x", "x/", "a\x00b", strings.Repeat("n", 256),
		`a\b`, "a\nb", "a\x7fb", "a\u0085b", "\xffx", // backslash, C0, DEL, C1, not UTF-8
		"txt.\u202eexe", "a\u2028b", "a\u2029b", "a\u200bb"} { // bidi override, line and paragraph separators, zero width
		r := newRig(t, false)
		if _, err := r.w.Upload(ctx, ctr, "/etc", n, []byte("x"), "*"); !errors.Is(err, ErrInvalidName) {
			t.Errorf("nom %q : %v, attendu ErrInvalidName", n, err)
		}
		if r.opens != 0 {
			t.Errorf("nom %q : refusé avant d'ouvrir la vue", n)
		}
		r.nothingWritten(t)
	}
	for _, n := range []string{strings.Repeat("n", 255), "été 2026.txt", "-x"} {
		r := newRig(t, false)
		if _, err := r.w.Upload(ctx, ctr, "/etc", n, nil, "*"); err != nil {
			t.Errorf("nom %q : %v", n, err)
		}
	}
}

func TestUploadNeverReplaces(t *testing.T) {
	r := newRig(t, false)
	if _, err := r.w.Upload(ctx, ctr, "/etc", "app.conf", []byte("x"), "*"); !errors.Is(err, ErrConflict) {
		t.Fatalf("nom existant : %v, attendu ErrConflict", err)
	}
	r.nothingWritten(t)
	r = newRig(t, false)
	r.mem.AddDir("/etc/conf.d")
	if _, err := r.w.Upload(ctx, ctr, "/etc", "conf.d", []byte("x"), "*"); !errors.Is(err, ErrConflict) {
		t.Fatalf("dossier existant : %v, attendu ErrConflict", err)
	}
	r = newRig(t, false)
	if _, err := r.w.Upload(ctx, ctr, "/etc", "new", []byte("x"), ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("sans If-None-Match: * : %v, attendu ErrConflict", err)
	}
	r.nothingWritten(t)
}

func TestLinksRefused(t *testing.T) {
	setup := func(t *testing.T) *rig {
		r := newRig(t, false)
		r.mem.AddLink("/etc/alias", "app.conf").AddLink("/conf", "etc").AddLink("/etc/dangling", "/nope")
		return r
	}
	cases := []struct {
		name string
		run  func(r *rig) error
	}{
		{"lien final", func(r *rig) error { _, err := r.w.Edit(ctx, ctr, "/etc/alias", []byte("x"), confTag); return err }},
		{"lien parent", func(r *rig) error { _, err := r.w.Edit(ctx, ctr, "/conf/app.conf", []byte("x"), confTag); return err }},
		{"dossier d'envoi lié", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/conf", "new", []byte("x"), "*"); return err }},
		{"envoi sur un lien pendant", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/etc", "dangling", []byte("x"), "*"); return err }},
		{"envoi sur un lien", func(r *rig) error { _, err := r.w.Upload(ctx, ctr, "/etc", "alias", []byte("x"), "*"); return err }},
	}
	for _, c := range cases {
		r := setup(t)
		if err := c.run(r); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("%s : %v, attendu ErrLinkInPath", c.name, err)
		}
		r.nothingWritten(t)
	}
}

func TestEditNotRegular(t *testing.T) {
	r := newRig(t, false)
	r.mem.Entries["/srv"] = fsview.Entry{Name: "srv", Type: fsview.TypeDir}
	r.mem.Entries["/srv/app.sock"] = fsview.Entry{Name: "app.sock", Type: fsview.TypeOther}
	for _, p := range []string{"/srv/app.sock", "/etc", "/"} {
		if _, err := r.w.Edit(ctx, ctr, p, []byte("x"), confTag); !errors.Is(err, fsview.ErrNotRegular) {
			t.Errorf("%s : %v, attendu ErrNotRegular", p, err)
		}
	}
	r.nothingWritten(t)
	if _, err := r.w.Upload(ctx, ctr, "/etc/app.conf", "x", []byte("x"), "*"); !errors.Is(err, fsview.ErrNotDir) {
		t.Errorf("envoi dans un fichier : %v, attendu ErrNotDir", err)
	}
	if _, err := r.w.Edit(ctx, ctr, "/etc/absent", []byte("x"), confTag); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent : %v", err)
	}
	r.nothingWritten(t)
}

func TestSizeCaps(t *testing.T) {
	r := newRig(t, false)
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", make([]byte, MaxEditBytes+1), confTag); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("édition : %v", err)
	}
	if _, err := r.w.Upload(ctx, ctr, "/etc", "big", make([]byte, MaxUploadBytes+1), "*"); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("envoi : %v", err)
	}
	// The current file is read with a cap too: its hash never needs more.
	r.mem.AddFile("/etc/huge", make([]byte, MaxEditBytes+1))
	if _, err := r.w.Edit(ctx, ctr, "/etc/huge", []byte("x"), ETag(make([]byte, MaxEditBytes+1))); !errors.Is(err, fsview.ErrTooLarge) {
		t.Errorf("fichier actuel trop gros : %v", err)
	}
	r.nothingWritten(t)
}

func TestViewErrorsPassThrough(t *testing.T) {
	for _, want := range []error{fsview.ErrForbiddenPath, &fsview.IndexingError{Progress: 0.4}, dockerx.ErrUnavailable} {
		r := newRig(t, false)
		r.w.Open = func(context.Context, string) (fsview.FS, error) { return nil, want }
		if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, want) {
			t.Errorf("Edit : %v, attendu %v", err, want)
		}
		if _, err := r.w.Upload(ctx, ctr, "/etc", "n", []byte("x"), "*"); !errors.Is(err, want) {
			t.Errorf("Upload : %v, attendu %v", err, want)
		}
		r.nothingWritten(t)
	}
	r := newRig(t, false)
	r.mem.Err = fsview.ErrForbiddenPath // e.g. the parent directory is on a pseudo filesystem
	if _, err := r.w.Upload(ctx, ctr, "/etc", "n", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("dossier interdit : %v", err)
	}
	r.nothingWritten(t)
}

func TestAuditIntentFailureWritesNothing(t *testing.T) {
	r := newRig(t, false)
	r.j.auditErr = errors.New("disque plein")
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); err == nil {
		t.Fatal("sans ligne d'intention, pas d'écriture")
	}
	if n := len(r.fake.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
}

func TestPutFailureIsAudited(t *testing.T) {
	r := newRig(t, false)
	r.fake.PutErr = dockerx.ErrRefused
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, dockerx.ErrRefused) {
		t.Fatalf("Edit : %v", err)
	}
	if s := r.j.Steps(); !slices.Equal(s, []string{"audit:intent", "put:/etc", "audit:error"}) {
		t.Fatalf("ordre : %v", s)
	}
	if e := r.j.events[1]; !strings.Contains(e.Error, "refusé") {
		t.Errorf("résultat : %+v", e)
	}
	if !slices.Equal(r.forgot, []string{cid}) {
		t.Errorf("un échec de PutArchive retire aussi l'index : %v", r.forgot)
	}
}

// Once the intent is in the journal, the write goes through even if the
// request ends: Docker removes the file before creating it again, a cut
// write would lose it.
func TestPutSurvivesRequestEnd(t *testing.T) {
	r := newRig(t, false)
	c, cancel := context.WithCancel(ctx)
	r.j.onEvent = func(e audit.Event) {
		if e.Result == "intent" {
			cancel()
		}
	}
	if _, err := r.w.Edit(c, ctr, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatal(err)
	}
	if r.put.ctxErr != nil {
		t.Fatalf("PutArchive a reçu un contexte annulé : %v", r.put.ctxErr)
	}
}

// Stopped container through the real Views (spec §5.2 step 1): indexing
// while the index is built, then a write drops it, and the next access
// builds it again.
func TestStoppedContainerIndexDropped(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range []*tar.Header{
		{Name: "/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "/etc/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "/etc/app.conf", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4},
	} {
		tw.WriteHeader(h)
		if h.Size > 0 {
			io.WriteString(tw, "old\n")
		}
	}
	tw.Close()
	var file bytes.Buffer
	fw := tar.NewWriter(&file)
	fw.WriteHeader(&tar.Header{Name: "app.conf", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4})
	io.WriteString(fw, "old\n")
	fw.Close()
	info := dockerx.ContainerInfo{ID: cid, Name: "db", FinishedAt: "2026-10-09T10:00:00Z"}
	fake := &dockerx.Fake{
		Infos:    map[string]dockerx.ContainerInfo{cid: info},
		Archives: map[string][]byte{cid + ":/": buf.Bytes(), cid + ":/etc/app.conf": file.Bytes()},
	}
	views := fsview.NewViews(fake, fsview.Options{})
	w := &Writer{Docker: fake, Audit: &journal{}, Open: views.For, Forget: views.Forget}
	if _, err := w.Edit(ctx, info, "/etc/app.conf", []byte("new\n"), ETag([]byte("old\n"))); !errors.Is(err, fsview.ErrIndexing) {
		t.Fatalf("pendant l'indexation : %v, attendu ErrIndexing", err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		_, err := w.Edit(ctx, info, "/etc/app.conf", []byte("new\n"), ETag([]byte("old\n")))
		if err == nil {
			break
		}
		if !errors.Is(err, fsview.ErrIndexing) || time.Now().After(deadline) {
			t.Fatal(err)
		}
	}
	builds := func() int {
		n := 0
		for _, c := range fake.Calls() {
			if c == cid+":/" {
				n++
			}
		}
		return n
	}
	if n := builds(); n != 1 {
		t.Fatalf("une construction attendue, %d", n)
	}
	if _, err := views.For(ctx, cid); !errors.Is(err, fsview.ErrIndexing) {
		t.Fatalf("après l'écriture, l'index doit être retiré : %v", err)
	}
	// The new build runs in a goroutine: wait for its GetArchive of "/".
	for deadline := time.Now().Add(5 * time.Second); builds() != 2; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("l'accès suivant reconstruit l'index : %d construction(s)", builds())
		}
	}
}

// Spec §5.3: the path and the name are checked (step 2) before the
// precondition (step 3). Without If-None-Match, a bad name is still
// invalid_name and a link is still link_in_path, not conflict.
func TestUploadChecksPathBeforePrecondition(t *testing.T) {
	r := newRig(t, false)
	r.mem.AddLink("/conf", "etc")
	if _, err := r.w.Upload(ctx, ctr, "/etc", "../x", []byte("x"), ""); !errors.Is(err, ErrInvalidName) {
		t.Errorf("../x sans If-None-Match : %v, attendu ErrInvalidName", err)
	}
	if _, err := r.w.Upload(ctx, ctr, "/conf", "x", []byte("x"), ""); !errors.Is(err, fsview.ErrLinkInPath) {
		t.Errorf("dossier lié sans If-None-Match : %v, attendu ErrLinkInPath", err)
	}
	r.nothingWritten(t)
}

// Q22: a container whose mounts fall under the host-bind rule (spec §5.2
// step 7: a bind of /, /proc, /sys, /dev…) gets no
// write at all, running or not: no view opened, nothing written.
func TestHostBindRefusesEveryWrite(t *testing.T) {
	r := newRig(t, false)
	c := ctr
	c.Mounts = []dockerx.Mount{{Type: "bind", Source: "/", Destination: "/host"}}
	if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("édition : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := r.w.Upload(ctx, c, "/etc", "n", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("envoi : %v, attendu ErrForbiddenPath", err)
	}
	if r.opens != 0 {
		t.Error("refusé sur Inspect, avant d'ouvrir la vue")
	}
	r.nothingWritten(t)
	// Control, and the scope of HostBind (Q22, option a, open): without the
	// bind of /, or with a bind of the host's /etc, the write goes through.
	for _, m := range [][]dockerx.Mount{nil, {{Type: "bind", Source: "/etc", Destination: "/hostetc"}}} {
		r := newRig(t, false)
		c := ctr
		c.Mounts = m
		if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); err != nil {
			t.Errorf("montages %v : %v", m, err)
		}
	}
}

// Q21: Docker writes in its own view of the mounts, not in the container's.
// A file mounted on its own and the files Docker manages answer "device or
// resource busy"; a tmpfs, /dev, /proc or /sys take the write and the
// container never sees it (measured). Refused before the intent.
func TestRefusesWhereDockerDoesNotWrite(t *testing.T) {
	c := ctr
	c.Mounts = []dockerx.Mount{
		{Type: "bind", Source: "/srv/app.conf", Destination: "/etc/app.conf"},
		{Type: "tmpfs", Destination: "/run/t"},
		{Type: "bind", Source: "/srv/conf", Destination: "/etc/conf.d"},
	}
	c.Tmpfs = []string{"/scratch"}
	cases := []struct {
		name string
		run  func(r *rig) error
	}{
		{"fichier monté", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); return err }},
		{"/etc/hosts", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/hosts", []byte("x"), confTag); return err }},
		{"/etc/resolv.conf", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/etc", "resolv.conf", []byte("x"), "*"); return err }},
		{"/etc/hostname", func(r *rig) error { _, err := r.w.Edit(ctx, c, "/etc/hostname", []byte("x"), confTag); return err }},
		{"--mount type=tmpfs", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/run/t", "n", []byte("x"), "*"); return err }},
		{"--tmpfs", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/scratch/sub", "n", []byte("x"), "*"); return err }},
		{"/dev", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/dev/shm", "n", []byte("x"), "*"); return err }},
		{"/proc", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/proc", "n", []byte("x"), "*"); return err }},
		{"/sys", func(r *rig) error { _, err := r.w.Upload(ctx, c, "/sys/x", "n", []byte("x"), "*"); return err }},
	}
	for _, k := range cases {
		r := newRig(t, false)
		if err := k.run(r); !errors.Is(err, fsview.ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", k.name, err)
		}
		if r.opens != 0 {
			t.Errorf("%s : refusé avant d'ouvrir la vue", k.name)
		}
		r.nothingWritten(t)
	}
	// Control: a bind of a directory is in Docker's view, the write lands
	// where the container sees it.
	r := newRig(t, false)
	r.mem.AddDir("/etc/conf.d")
	if _, err := r.w.Upload(ctx, c, "/etc/conf.d", "n", []byte("x"), "*"); err != nil {
		t.Errorf("dossier monté : %v", err)
	}
}

// A request that ends once the lock is held (the guard answered 504 while
// the checks ran) never starts a write: only the check of put stops it,
// MemFS ignores the context.
func TestEndedRequestWritesNothing(t *testing.T) {
	r := newRig(t, false)
	c, cancel := context.WithCancel(ctx)
	r.w.Open = func(context.Context, string) (fsview.FS, error) {
		cancel() // the lock is held: the request ends now
		return r.mem, nil
	}
	if _, err := r.w.Edit(c, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, context.Canceled) {
		t.Fatalf("requête terminée : %v", err)
	}
	r.nothingWritten(t)
}

// A request abandoned while it waits for the lock writes nothing and does
// not keep the lock.
func TestLockWaitAbandoned(t *testing.T) {
	r := newRig(t, false)
	g := &gatedPutter{Fake: r.fake, entered: make(chan struct{}, 2), gate: make(chan struct{}), apply: func(string, []byte) {}}
	r.w.Docker = g
	first := make(chan error, 1)
	go func() {
		_, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("v2\n"), confTag)
		first <- err
	}()
	<-g.entered // the first write holds the lock, inside PutArchive
	c, cancel := context.WithCancel(ctx)
	waiting := make(chan error, 1)
	go func() {
		_, err := r.w.Upload(c, ctr, "/etc", "n", []byte("x"), "*")
		waiting <- err
	}()
	cancel() // the lock is taken: whenever the upload reaches it, only ctx.Done is ready
	select {
	case err := <-waiting:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("attente abandonnée : %v, attendu context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("l'attente du verrou ne s'arrête pas avec la requête")
	}
	close(g.gate)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if n := len(r.fake.PutCalls()); n != 1 {
		t.Fatalf("%d appels à PutArchive, attendu 1 (l'envoi abandonné n'écrit rien)", n)
	}
	r.w.Docker = r.put
	if _, err := r.w.Upload(ctx, ctr, "/etc", "n2", []byte("x"), "*"); err != nil {
		t.Fatalf("le verrou est resté pris : %v", err)
	}
}

// A panic once the lock is held (the guard recovers it) releases the lock.
func TestLockReleasedOnPanic(t *testing.T) {
	r := newRig(t, false)
	open := r.w.Open
	r.w.Open = func(context.Context, string) (fsview.FS, error) { panic("vue cassée") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("la panique devait remonter")
			}
		}()
		r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag)
	}()
	r.w.Open = open
	done := make(chan error, 1)
	go func() {
		_, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verrou gardé après une panique")
	}
}

// The name is checked before the rules of the Inspect: a bad name toward a
// forbidden place is invalid_name.
func TestUploadNameBeforeInspectRules(t *testing.T) {
	r := newRig(t, false)
	c := ctr
	c.Mounts = []dockerx.Mount{{Type: "bind", Source: "/", Destination: "/host"}}
	for _, dir := range []string{"/proc", "/host"} {
		if _, err := r.w.Upload(ctx, c, dir, "../x", []byte("x"), ""); !errors.Is(err, ErrInvalidName) {
			t.Errorf("../x dans %s : %v, attendu ErrInvalidName", dir, err)
		}
	}
	r.nothingWritten(t)
}

// gatedPutter blocks every PutArchive on gate, then applies the write to the
// in-memory view, as Docker would.
type gatedPutter struct {
	*dockerx.Fake
	entered chan struct{}
	gate    chan struct{}
	apply   func(dir string, tarball []byte)
}

func (g *gatedPutter) PutArchive(ctx context.Context, id, dir string, r io.Reader) error {
	b, _ := io.ReadAll(r)
	g.entered <- struct{}{}
	<-g.gate
	g.apply(dir, b)
	return g.Fake.PutArchive(ctx, id, dir, bytes.NewReader(b))
}

// Two writes on one container never interleave (Q23): two tabs with the same
// If-Match, or two uploads of one name, give one write and one conflict.
func TestWritesAreSerialized(t *testing.T) {
	for _, k := range []struct {
		name  string
		write func(w *Writer) error
	}{
		{"deux éditions, même If-Match", func(w *Writer) error {
			_, err := w.Edit(ctx, ctr, "/etc/app.conf", []byte("v2\n"), confTag)
			return err
		}},
		{"deux envois, même nom", func(w *Writer) error {
			_, err := w.Upload(ctx, ctr, "/etc", "new.conf", []byte("v2\n"), "*")
			return err
		}},
	} {
		r := newRig(t, false)
		g := &gatedPutter{Fake: r.fake, entered: make(chan struct{}, 2), gate: make(chan struct{}),
			apply: func(dir string, tarball []byte) {
				h, body := entryOf(t, tarball)
				r.mem.AddFile(dir+"/"+h.Name, []byte(body))
			}}
		r.w.Docker = g
		errs := make(chan error, 2)
		go func() { errs <- k.write(r.w) }()
		<-g.entered // the first write is inside PutArchive
		go func() { errs <- k.write(r.w) }()
		select {
		case <-g.entered:
			t.Fatalf("%s : la seconde écriture a passé ses contrôles pendant la première", k.name)
		case <-time.After(200 * time.Millisecond):
		}
		close(g.gate)
		e1, e2 := <-errs, <-errs
		if (e1 == nil) == (e2 == nil) || !(errors.Is(e1, ErrConflict) || errors.Is(e2, ErrConflict)) {
			t.Errorf("%s : %v, %v ; attendu une écriture et un conflit", k.name, e1, e2)
		}
		if n := len(r.fake.PutCalls()); n != 1 {
			t.Errorf("%s : %d appels à PutArchive, attendu 1", k.name, n)
		}
	}
}

// mountView is MemFS plus the mount points of a running container
// (fsview.MountPointChecker, implemented by procfs).
type mountView struct {
	*fsview.MemFS
	points map[string]bool
}

func (m mountView) IsMountPoint(_ context.Context, p string) (bool, error) { return m.points[p], nil }

// Q21: a mount point Inspect does not show (a Swarm config, docker-init, a
// CDI file) is refused for an edit, through the view, before the intent.
func TestEditRefusesMountPoint(t *testing.T) {
	r := newRig(t, false)
	view := mountView{MemFS: r.mem, points: map[string]bool{"/etc/app.conf": true}}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Fatalf("point de montage : %v, attendu ErrForbiddenPath", err)
	}
	r.nothingWritten(t)
	view.points = map[string]bool{}
	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
	if _, err := r.w.Edit(ctx, ctr, "/etc/app.conf", []byte("x"), confTag); err != nil {
		t.Fatalf("contrôle, fichier ordinaire : %v", err)
	}
}
```

- [ ] **Step 2: Tests d'évasion Linux** — créer `internal/files/files_linux_test.go` :

```go
//go:build linux

package files

// Evasion tests of the writer on a real procfs view (spec §5.3 step 2, §10):
// a temporary directory plays the container root, the checks are openat2
// RESOLVE_NO_SYMLINKS and fstatfs. Building the traps uses host paths (the
// fixture); the code under test only sees the view. The fake Docker writes
// nothing: a refusal is proven by the absence of any PutArchive call.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// procRig serves dir as the container root through fsview.NewProcFS.
func procRig(t *testing.T, dir string) (*Writer, *dockerx.Fake) {
	t.Helper()
	fake := &dockerx.Fake{}
	w := &Writer{Docker: fake, Audit: &journal{}, Open: func(context.Context, string) (fsview.FS, error) {
		fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, err
		}
		return fsview.NewProcFS(os.NewFile(uintptr(fd), "test-root")), nil
	}}
	return w, fake
}

// trapRoot: /d/f.txt ("x"), /d/fifo, links /lf → d/f.txt, /ld → d,
// /d/dangling → /nope, /evil → a host directory outside the root.
func trapRoot(t *testing.T) (root, hostDir string) {
	t.Helper()
	hostDir = t.TempDir()
	must(t, os.WriteFile(filepath.Join(hostDir, "MARKER"), []byte("HOST-SECRET"), 0o644))
	root = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(root, "d", "f.txt"), []byte("x"), 0o640))
	must(t, unix.Mkfifo(filepath.Join(root, "d", "fifo"), 0o644))
	must(t, os.Symlink("d/f.txt", filepath.Join(root, "lf")))
	must(t, os.Symlink("d", filepath.Join(root, "ld")))
	must(t, os.Symlink("/nope", filepath.Join(root, "d", "dangling")))
	must(t, os.Symlink(hostDir, filepath.Join(root, "evil")))
	return root, hostDir
}

var xTag = ETag([]byte("x"))

// putDirs: the directories PutArchive received, without the tar bytes.
func putDirs(puts []dockerx.Put) []string {
	var dirs []string
	for _, p := range puts {
		dirs = append(dirs, p.Dir)
	}
	return dirs
}

func TestProcLinksRefused(t *testing.T) {
	root, _ := trapRoot(t)
	w, fake := procRig(t, root)
	// Control: a naive write path through /evil reaches the host.
	if _, err := os.Stat(filepath.Join(root, "evil", "MARKER")); err != nil {
		t.Fatalf("contrôle : le lien /evil devait mener à l'hôte : %v", err)
	}
	edits := []string{"/lf", "/ld/f.txt", "/evil/MARKER", "/d/dangling"}
	for _, p := range edits {
		if _, err := w.Edit(ctx, ctr, p, []byte("y"), xTag); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("édition %s : %v, attendu ErrLinkInPath", p, err)
		}
	}
	uploads := [][2]string{{"/ld", "new"}, {"/evil", "new"}, {"/d", "dangling"}, {"/", "lf"}, {"/", "ld"}}
	for _, u := range uploads {
		if _, err := w.Upload(ctx, ctr, u[0], u[1], []byte("y"), "*"); !errors.Is(err, fsview.ErrLinkInPath) {
			t.Errorf("envoi %s/%s : %v, attendu ErrLinkInPath", u[0], u[1], err)
		}
	}
	// The fake Docker writes nothing: the proof is that it was never asked.
	if puts := fake.PutCalls(); len(puts) != 0 {
		t.Fatalf("ÉVASION : PutArchive appelé malgré un lien, dossiers %v", putDirs(puts))
	}
}

func TestProcNotRegular(t *testing.T) {
	root, _ := trapRoot(t)
	w, fake := procRig(t, root)
	done := make(chan error, 1)
	go func() {
		_, err := w.Edit(ctx, ctr, "/d/fifo", []byte("y"), xTag)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, fsview.ErrNotRegular) {
			t.Errorf("FIFO : %v, attendu ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la FIFO a bloqué l'édition")
	}
	if _, err := w.Edit(ctx, ctr, "/d", []byte("y"), xTag); !errors.Is(err, fsview.ErrNotRegular) {
		t.Errorf("dossier : %v", err)
	}
	if len(fake.PutCalls()) != 0 {
		t.Fatal("PutArchive appelé")
	}
}

// The parent of an upload, or the target of an edit, on a pseudo filesystem
// → forbidden_path (spec §5.3 step 2, fstatfs). The host's /proc is the
// root: "/" and "/version" do not start with /proc, so the prefix rule of
// Q21 lets them through and only fstatfs on the descriptor refuses them.
func TestProcPseudoFSRefused(t *testing.T) {
	w, fake := procRig(t, "/proc")
	if _, err := w.Upload(ctx, ctr, "/", "x", []byte("y"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("envoi dans un dossier procfs : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := w.Edit(ctx, ctr, "/version", []byte("y"), xTag); !errors.Is(err, fsview.ErrForbiddenPath) {
		t.Errorf("édition d'un fichier procfs : %v, attendu ErrForbiddenPath", err)
	}
	if len(fake.PutCalls()) != 0 {
		t.Fatal("PutArchive appelé")
	}
}

func TestProcWriteSendsExactPath(t *testing.T) {
	root, _ := trapRoot(t)
	uid, gid := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		uid, gid = 1234, 5678
		must(t, os.Chown(filepath.Join(root, "d", "f.txt"), uid, gid))
		must(t, os.Chown(filepath.Join(root, "d"), uid+1, gid+1))
	}
	w, fake := procRig(t, root)
	if _, err := w.Edit(ctx, ctr, "/d/./f.txt", []byte("y"), xTag); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Upload(ctx, ctr, "/d", "new.txt", []byte("z"), "*"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Upload(ctx, ctr, "/d", "f.txt", []byte("z"), "*"); !errors.Is(err, ErrConflict) {
		t.Errorf("fichier existant : %v, attendu ErrConflict", err)
	}
	puts := fake.PutCalls()
	if len(puts) != 2 || puts[0].Dir != "/d" || puts[1].Dir != "/d" {
		t.Fatalf("PutArchive doit recevoir le dossier parent nettoyé /d : %v", putDirs(puts))
	}
	h, body := entryOf(t, puts[0].Tar)
	if h.Name != "f.txt" || body != "y" || h.Mode != 0o640 || h.Uid != uid || h.Gid != gid {
		t.Errorf("édition : %+v %q", h, body)
	}
	h, _ = entryOf(t, puts[1].Tar)
	wantUID, wantGID := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		wantUID, wantGID = uid+1, gid+1
	}
	if h.Name != "new.txt" || h.Mode != 0o644 || h.Uid != wantUID || h.Gid != wantGID {
		t.Errorf("envoi : %+v", h)
	}
}
```

- [ ] **Step 3: Test du point de montage dans `fsview`** — créer `internal/fsview/mountpoint_linux_test.go` :

```go
//go:build linux

package fsview

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// IsMountPoint (plan Explorateur 3, Q21): the STATX_MNT_ID of a path and of
// its parent differ. /proc is a mount point on any Linux host or container;
// /etc/passwd is not. No root needed.
func TestIsMountPoint(t *testing.T) {
	host, ok := openTestRoot(t, "/").(MountPointChecker)
	if !ok {
		t.Fatal("procfs doit implémenter MountPointChecker")
	}
	for p, want := range map[string]bool{"/proc": true, "/": true, "/etc/passwd": false, "/etc": false} {
		if got, err := host.IsMountPoint(ctx, p); err != nil || got != want {
			t.Errorf("%s : %v %v, attendu %v", p, got, err, want)
		}
	}
	// A link anywhere is refused, as by StatNoLinks: never followed.
	dir := t.TempDir()
	must(t, os.Symlink("/proc", filepath.Join(dir, "p")))
	if _, err := host.IsMountPoint(ctx, dir+"/p"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("lien : %v, attendu ErrLinkInPath", err)
	}
}
```

- [ ] **Step 4: Vérifier qu'ils échouent**

Run: `go vet ./internal/files/ 2>&1 | tail -n 3 ; GOOS=linux go vet ./internal/files/ 2>&1 | tail -n 3 ; GOOS=linux go vet ./internal/fsview/ 2>&1 | tail -n 3`
Expected : `undefined: Writer` (les deux premiers) ; `undefined: MountPointChecker` (le troisième).

- [ ] **Step 5: Commit des tests rouges** (ne pas pousser avant la tâche 3)

```bash
git add internal/files internal/fsview/mountpoint_linux_test.go
git commit -F - <<'EOF'
files: tests d'évasion et d'ordre de l'écriture, écrits avant le code (rouges)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 3: Paquet `files` : l'ordre de §5.3 dans une fonction par écriture

**Où tester :** macOS, puis Linux root (`scripts/test-linux.sh`).

**Files:**
- Create: `internal/files/files.go`, `internal/fsview/mountpoint_linux.go`
- Modify: `internal/fsview/fsview.go`

**Interfaces:**
- Consumes: `fsview.FS` (`StatNoLinks`, `Open`, `Close`), `fsview.HostBind(ctx, dockerx.Client, []dockerx.Mount) error`, `fsview.CleanPath`, `fsview.UnixPerm`, `fsview.ErrNotRegular`, `fsview.ErrNotDir`, `fsview.ErrTooLarge`, `fsview.ErrForbiddenPath`, `audit.Event`, `dockerx.ContainerInfo`, `Auditor` (satisfait par `*audit.Log`).
- Produces: voir tâche 2, plus `PutTimeout = time.Minute`, `type Auditor interface{ Write(audit.Event) error }`, `fsview.MountPointChecker` et `(*procFS).IsMountPoint`.

> **À comprendre :** `Edit` et `Upload` suivent la spec ligne à ligne, les numéros d'étape sont en commentaire. Rien n'ouvre de chemin : la vue fait tous les contrôles (`StatNoLinks` refuse un lien n'importe où, dernier composant compris, et `procfs` y fait `fstatfs`), Docker fait l'écriture. Le chemin d'écriture est une **chaîne** : celle qui a été contrôlée est celle dont `path.Dir` part vers Docker. `writable` refuse, sur l'`Inspect` seul, ce que Docker n'écrirait pas comme le conteneur le voit (Q21) et tout conteneur dont un montage tombe sous la règle de §5.2 étape 7, bind de `/`, `/proc`, `/sys` ou `/dev` (Q22, portée (a)). `lock` prend le verrou du conteneur avant d'ouvrir la vue, `defer` le rend après l'étape 6, panique comprise (Q23). Une édition refuse une cible qui est un point de montage, si la vue sait le dire (`MountPointChecker`, `procfs` seulement : R15). `put` regroupe les étapes 4 à 6 : rien si la requête est déjà finie, intention, `PutArchive` avec un contexte détaché de la requête (Q17), `Forget` (Q20), résultat.

- [ ] **Step 1: Implémenter** — dans `fsview`, l'interface facultative et sa version `procfs` :

```diff
--- a/internal/fsview/fsview.go
+++ b/internal/fsview/fsview.go
@@ -42,6 +42,14 @@ type OpenChecker interface {
 	CheckOpen(ctx context.Context, path string) error
 }
 
+// MountPointChecker is implemented by a view that can tell whether a path
+// is a mount point of the running container (procfs: STATX_MNT_ID of the
+// path and of its parent, no link followed). Package files refuses to edit
+// one: Docker writes in its own view of the mounts (plan Explorateur 3, Q21).
+type MountPointChecker interface {
+	IsMountPoint(ctx context.Context, path string) (bool, error)
+}
+
 // FS is a read view of a container's tree. Paths are container paths;
 // List, Stat and Open follow symbolic links, never out of the container root.
 type FS interface {
```

```go
//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"golang.org/x/sys/unix"
)

// IsMountPoint: the mount ID of CleanPath(name) differs from its parent's.
// Both are opened like StatNoLinks (openat2, no link anywhere). Without
// STATX_MNT_ID (kernel before 5.8) the answer is unknown: an error, so the
// edit is refused (fail closed).
func (p *procFS) IsMountPoint(_ context.Context, name string) (bool, error) {
	clean := CleanPath(name)
	if clean == "/" {
		return true, nil
	}
	id, err := p.mountIDNoLinks(clean)
	if err != nil {
		return false, err
	}
	parent, err := p.mountIDNoLinks(path.Dir(clean))
	if err != nil {
		return false, err
	}
	return id != parent, nil
}

func (p *procFS) mountIDNoLinks(clean string) (uint64, error) {
	fd, err := unix.Openat2(int(p.root.Fd()), clean, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ELOOP) {
		return 0, fmt.Errorf("%w : %s", ErrLinkInPath, clean)
	}
	if err != nil {
		return 0, &fs.PathError{Op: "openat2", Path: clean, Err: err}
	}
	defer unix.Close(fd)
	var st unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &st); err != nil {
		return 0, &fs.PathError{Op: "statx", Path: clean, Err: err}
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return 0, errors.New("statx sans STATX_MNT_ID (noyau antérieur à 5.8) : point de montage indécidable")
	}
	return st.Mnt_id, nil
}
```

Puis créer `internal/files/files.go` :

```go
// Package files is the agent's only writer (spec §4, §5.3, §7): read-only
// mode, link checks, If-Match, audit, then PutArchive, in one function per
// write. It never opens a container path itself: every check goes through
// the fsview view, every write through Docker.
package files

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// Size caps (plan Explorateur 3, Q14). An edit never exceeds the preview
// (2 MiB, spec §6): the editor only gets what the preview served.
const (
	MaxEditBytes   = 2 << 20
	MaxUploadBytes = 32 << 20
)

// PutTimeout bounds a PutArchive once its intent is in the audit log. The
// write no longer depends on the request: Docker removes the existing file
// before creating it again (measured), a cut write would lose it.
const PutTimeout = time.Minute

var (
	ErrReadOnly    = errors.New("agent en lecture seule")
	ErrConflict    = errors.New("le fichier a changé ou existe déjà")
	ErrInvalidName = errors.New("nom de fichier invalide")
)

// Auditor is the part of *audit.Log the writer uses.
type Auditor interface {
	Write(audit.Event) error
}

type Writer struct {
	ReadOnly bool
	// Docker: PutArchive, and VolumeOptions for the host-bind rule (Q22).
	Docker dockerx.Client
	Audit  Auditor
	// Open opens the view of a container by full ID: fsview.Views.For.
	Open func(ctx context.Context, id string) (fsview.FS, error)
	// Forget drops the index of a stopped container: fsview.Views.Forget.
	// Called after every PutArchive, failed ones included (Docker may have
	// removed the file already). nil: nothing to drop.
	Forget func(id string)

	mu    sync.Mutex
	locks map[string]chan struct{} // one per container: steps 2 to 6 never interleave (Q23)
}

// ETag is the quoted SHA-256 of content: the ETag of the preview (spec §6)
// and the If-Match of an edit.
func ETag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// Allowed is step 1 alone (spec §5.3): the API calls it before reading a
// request body, then Edit and Upload check it again.
func (w *Writer) Allowed() error {
	if w.ReadOnly {
		return ErrReadOnly
	}
	return nil
}

// Edit replaces the regular file p of container c (its Inspect) by content,
// if the file's current ETag is ifMatch. It returns the new ETag.
func (w *Writer) Edit(ctx context.Context, c dockerx.ContainerInfo, p string, content []byte, ifMatch string) (string, error) {
	// 1. Read-only mode.
	if err := w.Allowed(); err != nil {
		return "", err
	}
	if len(content) > MaxEditBytes {
		return "", fmt.Errorf("%w : édition limitée à %d octets", fsview.ErrTooLarge, MaxEditBytes)
	}
	// 2. The write path is the cleaned request path, nothing else; a place
	// Docker really writes to (Inspect); no link anywhere on it; fstatfs
	// (procfs) or the host-bind rule (tarindex) inside the view; a regular file.
	target := fsview.CleanPath(p)
	if err := w.writable(ctx, c, target); err != nil {
		return "", err
	}
	unlock, err := w.lock(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer unlock()
	fsys, err := w.Open(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer fsys.Close()
	e, err := fsys.StatNoLinks(ctx, target)
	if err != nil {
		return "", err
	}
	if e.Type != fsview.TypeFile {
		return "", fmt.Errorf("%w : %s", fsview.ErrNotRegular, target)
	}
	// A mount point of the running container that Inspect may not show (a
	// file bind, a Swarm config, docker-init, a CDI file): Docker cannot
	// replace it, or writes in its own view (Q21). Fail closed.
	if mp, ok := fsys.(fsview.MountPointChecker); ok {
		point, err := mp.IsMountPoint(ctx, target)
		if err != nil {
			return "", err
		}
		if point {
			return "", fmt.Errorf("%w : %s est un point de montage du conteneur : modifiez sa source", fsview.ErrForbiddenPath, target)
		}
	}
	// 3. Read the file again, compare its SHA-256 to If-Match.
	current, err := readCapped(ctx, fsys, target)
	if err != nil {
		return "", err
	}
	if ifMatch == "" || ETag(current) != ifMatch {
		return "", fmt.Errorf("%w : %s (If-Match)", ErrConflict, target)
	}
	tarball, err := oneFile(path.Base(target), content, e.Mode, e.UID, e.GID)
	if err != nil {
		return "", err
	}
	// 4 to 6.
	if err := w.put(ctx, "file.write", c.ID, target, tarball); err != nil {
		return "", err
	}
	return ETag(content), nil
}

// Upload creates the file name in directory dir of container c (its
// Inspect); it never replaces anything (If-None-Match: *). It returns the
// path written.
func (w *Writer) Upload(ctx context.Context, c dockerx.ContainerInfo, dir, name string, content []byte, ifNoneMatch string) (string, error) {
	// 1. Read-only mode.
	if err := w.Allowed(); err != nil {
		return "", err
	}
	if len(content) > MaxUploadBytes {
		return "", fmt.Errorf("%w : envoi limité à %d octets", fsview.ErrTooLarge, MaxUploadBytes)
	}
	// 2. A single component; a place Docker really writes to (Inspect); then
	// the parent, without any link, not on a pseudo filesystem (inside
	// StatNoLinks), a directory.
	if !validName(name) {
		return "", fmt.Errorf("%w : %q", ErrInvalidName, name)
	}
	parent := fsview.CleanPath(dir)
	target := fsview.CleanPath(parent + "/" + name)
	if err := w.writable(ctx, c, target); err != nil {
		return "", err
	}
	unlock, err := w.lock(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer unlock()
	fsys, err := w.Open(ctx, c.ID)
	if err != nil {
		return "", err
	}
	defer fsys.Close()
	d, err := fsys.StatNoLinks(ctx, parent)
	if err != nil {
		return "", err
	}
	if d.Type != fsview.TypeDir {
		return "", fmt.Errorf("%w : %s", fsview.ErrNotDir, parent)
	}
	// 3. If-None-Match: *, then the last component, without following a
	// link: absent → go on; a link, even dangling → link_in_path (from
	// StatNoLinks); anything else → conflict.
	if ifNoneMatch != "*" {
		return "", fmt.Errorf("%w : un envoi exige If-None-Match: *", ErrConflict)
	}
	switch _, err := fsys.StatNoLinks(ctx, target); {
	case err == nil:
		return "", fmt.Errorf("%w : %s existe", ErrConflict, target)
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	tarball, err := oneFile(name, content, 0o644, d.UID, d.GID)
	if err != nil {
		return "", err
	}
	// 4 to 6.
	if err := w.put(ctx, "file.upload", c.ID, target, tarball); err != nil {
		return "", err
	}
	return target, nil
}

// put is steps 4 to 6: intent, PutArchive on the parent of the checked path
// (exactly that string), result. A failed intent writes nothing, and so
// does a request that has already ended (the guard answered 504).
func (w *Writer) put(ctx context.Context, action, id, target string, tarball []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("requête terminée avant l'écriture, rien n'est écrit : %w", err)
	}
	ev := audit.Event{Action: action, Container: id, Path: target, Result: "intent"}
	if err := w.Audit.Write(ev); err != nil {
		return fmt.Errorf("journal d'audit, rien n'est écrit : %w", err)
	}
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), PutTimeout)
	defer cancel()
	err := w.Docker.PutArchive(pctx, id, path.Dir(target), bytes.NewReader(tarball))
	if w.Forget != nil {
		w.Forget(id)
	}
	ev.Result = "ok"
	if err != nil {
		ev.Result, ev.Error = "error", err.Error()
	}
	if aerr := w.Audit.Write(ev); aerr != nil {
		// The write is done (or failed) already: report it as it is.
		slog.Error("audit write", "err", aerr, "action", action, "path", target)
	}
	return err
}

// readCapped reads the current file through the view (regular files only),
// at most MaxEditBytes: its hash never needs more.
func readCapped(ctx context.Context, fsys fsview.FS, p string) ([]byte, error) {
	rc, err := fsys.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, MaxEditBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxEditBytes {
		return nil, fmt.Errorf("%w : %s dépasse %d octets", fsview.ErrTooLarge, p, MaxEditBytes)
	}
	return b, nil
}

// lock takes the lock of container id, or gives up with ctx.
func (w *Writer) lock(ctx context.Context, id string) (func(), error) {
	w.mu.Lock()
	if w.locks == nil {
		w.locks = map[string]chan struct{}{}
	}
	l, ok := w.locks[id]
	if !ok {
		l = make(chan struct{}, 1)
		w.locks[id] = l
	}
	w.mu.Unlock()
	select {
	case l <- struct{}{}:
		return func() { <-l }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// writable refuses, from the Inspect alone and before any intent, a write
// Docker would not make where the container sees it (Q21), and any write to
// a container whose mounts fall under the host-bind rule of spec §5.2 step 7
// (a bind of /, /proc, /sys, /dev…; Q22: a bind of /etc stays writable). Docker
// extracts in its own view: the root filesystem plus the configured mounts.
func (w *Writer) writable(ctx context.Context, c dockerx.ContainerInfo, target string) error {
	if err := fsview.HostBind(ctx, w.Docker, c.Mounts); err != nil {
		return fmt.Errorf("écriture refusée pour tout le conteneur : %w", err)
	}
	for _, d := range []string{"/proc", "/sys", "/dev"} {
		if under(target, d) {
			return fmt.Errorf("%w : %s : %s est monté au démarrage du conteneur, Docker n'y écrit pas", fsview.ErrForbiddenPath, target, d)
		}
	}
	switch target {
	case "/etc/hosts", "/etc/hostname", "/etc/resolv.conf":
		return fmt.Errorf("%w : %s est géré et monté par Docker", fsview.ErrForbiddenPath, target)
	}
	for _, m := range c.Mounts {
		d := fsview.CleanPath(m.Destination)
		if d == target {
			return fmt.Errorf("%w : %s est monté (%s %s) : modifiez sa source", fsview.ErrForbiddenPath, target, m.Type, m.Source+m.Name)
		}
		if m.Type == "tmpfs" && under(target, d) {
			return fmt.Errorf("%w : %s est sous le tmpfs %s, Docker n'y écrit pas", fsview.ErrForbiddenPath, target, d)
		}
	}
	for _, d := range c.Tmpfs {
		if under(target, fsview.CleanPath(d)) {
			return fmt.Errorf("%w : %s est sous le tmpfs %s, Docker n'y écrit pas", fsview.ErrForbiddenPath, target, d)
		}
	}
	return nil
}

// under: p is dir or below it (both clean, absolute).
func under(p, dir string) bool {
	return p == dir || dir == "/" || strings.HasPrefix(p, dir+"/")
}

// validName: a single path component Linux accepts (NAME_MAX 255 bytes),
// valid UTF-8 (the UI could not open it otherwise, spec §13), without
// control characters (C0, DEL, C1), format characters (bidi overrides, zero
// width) or line and paragraph separators: a misleading confirmation; nor
// backslash.
func validName(n string) bool {
	if n == "" || n == "." || n == ".." || len(n) > 255 || !utf8.ValidString(n) || strings.ContainsAny(n, `/\`) {
		return false
	}
	for _, r := range n {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return false
		}
	}
	return true
}

// oneFile is the tar of a single regular file: base name, mode, owner.
func oneFile(name string, content []byte, mode fs.FileMode, uid, gid int) ([]byte, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	h := &tar.Header{Typeflag: tar.TypeReg, Name: name, Size: int64(len(content)),
		Mode: int64(fsview.UnixPerm(mode)), Uid: uid, Gid: gid, ModTime: time.Now()}
	if err := tw.WriteHeader(h); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
```

- [ ] **Step 2: Vérifier sur macOS**

Run: `go test -race ./internal/files/ 2>&1 | tail -n 5 && go vet ./... && GOOS=linux go vet ./... && gofmt -l internal`
Expected: `ok`, aucune sortie de `vet` ni de `gofmt`.

- [ ] **Step 3: Vérifier sous Linux root**

Run: `scripts/test-linux.sh -race ./internal/files/ -v 2>&1 | grep -E '^(---|ok|FAIL)' | tail -n 25`
Expected: tous `PASS` (28 tests) ; `scripts/test-linux.sh ./internal/fsview/ -run IsMountPoint 2>&1 | tail -n 2` → `ok`. Passent notamment `TestProcLinksRefused`, `TestProcNotRegular` (la FIFO ne bloque pas : `StatNoLinks` ne l'ouvre jamais en lecture), `TestProcPseudoFSRefused`, `TestProcWriteSendsExactPath`, `TestWritesAreSerialized`, `TestRefusesWhereDockerDoesNotWrite`, `TestHostBindRefusesEveryWrite` ; aucun SKIP.

- [ ] **Step 4: Contrôles de mutation** (une fois chacun, puis annuler avec `git checkout internal/files/files.go`)
  - Remplacer `fsys.StatNoLinks(` par `fsys.Stat(` (une vue qui suit les liens) : `scripts/test-linux.sh ./internal/files/ -run 'Links|Proc' 2>&1 | grep -E '^(---|FAIL)' | tail -n 8` doit montrer `FAIL: TestProcLinksRefused` (« ÉVASION : PutArchive appelé malgré un lien ») et `FAIL: TestLinksRefused`.
  - Remplacer `context.WithoutCancel(ctx)` par `ctx` : `go test ./internal/files/ -run Survives 2>&1 | tail -n 3` doit échouer.
  - Faire rendre à `lock` un `func() {}` sans rien attendre : `go test ./internal/files/ -run Serialized 2>&1 | tail -n 3` doit échouer (« la seconde écriture a passé ses contrôles pendant la première »).
  - Retirer le contrôle `ctx.Err()` du début de `put` : `go test -count=3 ./internal/files/ -run Ended 2>&1 | tail -n 3` doit échouer à chaque fois.
  - Retirer le cas `case <-ctx.Done():` de `lock` : `-run LockWait` doit échouer (après 5 s).
  - Déplacer `defer unlock()` après l'ouverture de la vue : `-run Panic` doit échouer.
  - Retirer `unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)` de `validName` : `-run InvalidName` doit échouer.
  - Dans `internal/fsview/procfs_linux.go`, neutraliser `checkNotPseudo` de `StatNoLinks` : `scripts/test-linux.sh ./internal/files/ -run Pseudo 2>&1 | tail -n 3` doit échouer ; puis `git checkout internal/fsview`.

- [ ] **Step 5: Commit**

```bash
git add internal/files internal/fsview
git commit -F - <<'EOF'
files: seul point d'écriture, lecture seule, chemin sans lien, montages, verrou, If-Match, audit, PutArchive ; fsview: IsMountPoint

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 4: Routes `PUT /fs/file` et `POST /fs/upload`

**Où tester :** macOS (`httptest`).

**Files:**
- Create: `internal/api/fs_write.go`, `internal/api/fs_write_test.go`
- Modify: `internal/api/server.go`, `internal/api/fserr.go`, `internal/api/fs.go`, `internal/api/fs_preview.go`, `internal/api/fs_download.go`

**Interfaces:**
- Consumes: `files.Writer` et ses erreurs (tâche 3 ; `Edit` et `Upload` reçoivent l'`info` de `fsRequest`), `files.ETag`, `files.MaxEditBytes`, `files.MaxUploadBytes`, `guarded`, `writeFSError`, `fsRequest`, `Deps.OpenFS`, `Deps.Views.Forget`, `dockerx.ErrRefused` (→ `read_only`), `dockerx.ErrPutInvalid` (→ `conflict`).
- Produces: `server.files *files.Writer` ; `fsRequest(w, r, param string)` (`"path"` ou `"dir"`) ; `PUT /api/containers/{id}/fs/file?path=` → `200 {"etag"}` + en-tête `ETag` ; `POST /api/containers/{id}/fs/upload?dir=` → `201 {"path","etag"}` ; helpers de test `(*fixture).send(method, path string, body []byte, cookie *http.Cookie, headers map[string]string)`, `(*fixture).upload(t, c, ctr, dir, filename string, content []byte, inm string)`, `multipartBody(t, field, filename string, content []byte) ([]byte, string)`, `multipartRaw(t, disposition string, content []byte) ([]byte, string)`, `tarEntry(t, b []byte) (string, string)`.

> **À comprendre :** la route ne décide de rien : elle lit le corps (borné), puis appelle `files` sous la garde de §13. Elle demande d'abord `files.Allowed()` pour répondre `read_only` sans lire le corps ni interroger Docker. Le nom d'un envoi est le `filename` de la partie `file` tel que `mime.ParseMediaType` le décode, et c'est cette valeur que `files` valide puis écrit (Q19) : `Part.FileName()` aurait changé `a/b` en `b`. Les tests envoient des en-têtes bruts (`multipartRaw`) : `filename*=UTF-8''..%2Fx` se décode en `../x` et doit être refusé ; `filename*=UTF-8''%C3%A9t%C3%A9.txt` doit arriver dans le tar sous `été.txt`.

- [ ] **Step 1: Tests qui échouent** — créer `internal/api/fs_write_test.go` :

```go
package api

// PUT /fs/file and POST /fs/upload (spec §6, F4) through the real wiring of
// New: package files on an in-memory view, a fake Docker.

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/files"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// send is f.do with a raw body and extra headers.
func (f *fixture) send(method, path string, body []byte, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.com"+path, bytes.NewReader(body))
	r.RemoteAddr = "192.0.2.1:40000"
	r.Header.Set("Origin", "http://example.com")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, r)
	return rec
}

// multipartBody builds the body of an upload whose part field carries the
// filename given, quoted by %q (no base name).
func multipartBody(t *testing.T, field, filename string, content []byte) ([]byte, string) {
	t.Helper()
	return multipartRaw(t, fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename), content)
}

// multipartRaw builds the body of an upload with a raw Content-Disposition.
func multipartRaw(t *testing.T, disposition string, content []byte) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", disposition)
	h.Set("Content-Type", "application/octet-stream")
	p, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	p.Write(content)
	mw.Close()
	return b.Bytes(), mw.FormDataContentType()
}

func (f *fixture) upload(t *testing.T, c *http.Cookie, ctr, dir, filename string, content []byte, inm string) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, "file", filename, content)
	h := map[string]string{"Content-Type": ct}
	if inm != "" {
		h["If-None-Match"] = inm
	}
	return f.send("POST", "/api/containers/"+ctr+"/fs/upload?dir="+dir, body, c, h)
}

func writeFixture(t *testing.T, readOnly bool) (*fixture, *http.Cookie) {
	t.Helper()
	f := newFixture(t, readOnly)
	f.fs.AddDir("/etc").AddFile("/etc/app.conf", []byte("port=80\n")).AddLink("/etc/alias", "app.conf")
	return f, f.session(t)
}

func TestEditRoute(t *testing.T) {
	f, c := writeFixture(t, false)
	// The If-Match is the ETag of the preview, as is.
	tag := f.do("GET", "/api/containers/web/fs/file?path=/etc/app.conf", "", c).Header().Get("ETag")
	rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("10.0.0.1\n"), c, map[string]string{"If-Match": tag})
	var body struct{ ETag string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || body.ETag != files.ETag([]byte("10.0.0.1\n")) || rec.Header().Get("ETag") != body.ETag {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	puts := f.docker.PutCalls()
	if len(puts) != 1 || puts[0].ID != webID || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
	log, _ := os.ReadFile(f.auditP)
	if n := strings.Count(string(log), `"action":"file.write"`); n != 2 {
		t.Fatalf("journal : %d ligne(s) file.write, attendu 2 (intention, résultat)\n%s", n, log)
	}
}

func TestWriteRoutesErrors(t *testing.T) {
	f, c := writeFixture(t, false)
	tag := files.ETag([]byte("port=80\n"))
	put := func(p string, body []byte, ifMatch string) *httptest.ResponseRecorder {
		return f.send("PUT", "/api/containers/web/fs/file?path="+p, body, c, map[string]string{"If-Match": ifMatch})
	}
	cases := []struct {
		name   string
		rec    *httptest.ResponseRecorder
		status int
		code   string
	}{
		{"If-Match faux", put("/etc/app.conf", []byte("x"), files.ETag([]byte("autre"))), 409, "conflict"},
		{"If-Match absent", put("/etc/app.conf", []byte("x"), ""), 409, "conflict"},
		{"lien final", put("/etc/alias", []byte("x"), tag), 422, "link_in_path"},
		{"dossier", put("/etc", []byte("x"), tag), 422, "not_regular"},
		{"absent", put("/etc/nope", []byte("x"), tag), 404, "not_found"},
		{"trop gros", put("/etc/app.conf", make([]byte, files.MaxEditBytes+1), tag), 413, "too_large"},
		{"nom a/b brut", f.upload(t, c, "web", "/etc", "a/b", []byte("x"), "*"), 400, "invalid_name"},
		{"nom ..", f.upload(t, c, "web", "/etc", "..", []byte("x"), "*"), 400, "invalid_name"},
		{"nom vide", f.upload(t, c, "web", "/etc", "", []byte("x"), "*"), 400, "invalid_name"},
		{"sans If-None-Match", f.upload(t, c, "web", "/etc", "n", []byte("x"), ""), 409, "conflict"},
		{"nom existant", f.upload(t, c, "web", "/etc", "app.conf", []byte("x"), "*"), 409, "conflict"},
		{"nom ../x sans If-None-Match", f.upload(t, c, "web", "/etc", "../x", []byte("x"), ""), 400, "invalid_name"},
		{"nom avec \\", f.upload(t, c, "web", "/etc", `a\b`, []byte("x"), "*"), 400, "invalid_name"},
		{"/etc/hosts (Q21)", f.upload(t, c, "web", "/etc", "hosts", []byte("x"), "*"), 403, "forbidden_path"},
		{"nom a/b vers /proc : le nom d'abord", f.upload(t, c, "web", "/proc", "a/b", []byte("x"), "*"), 400, "invalid_name"},
		{"envoi sur un lien", f.upload(t, c, "web", "/etc", "alias", []byte("x"), "*"), 422, "link_in_path"},
		{"envoi trop gros", f.upload(t, c, "web", "/etc", "big", make([]byte, files.MaxUploadBytes+1), "*"), 413, "too_large"},
	}
	for _, k := range cases {
		if k.rec.Code != k.status || errCode(t, k.rec) != k.code {
			t.Errorf("%s : %d %s, attendu %d %s", k.name, k.rec.Code, k.rec.Body, k.status, k.code)
		}
	}
	// A part other than "file" only: no file, an empty name.
	body, ct := multipartBody(t, "autre", "x", []byte("x"))
	if rec := f.send("POST", "/api/containers/web/fs/upload?dir=/etc", body, c, map[string]string{"Content-Type": ct, "If-None-Match": "*"}); rec.Code != 400 || errCode(t, rec) != "invalid_name" {
		t.Errorf("sans champ file : %d %s", rec.Code, rec.Body)
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois malgré les refus", n)
	}
}

func TestUploadRoute(t *testing.T) {
	f, c := writeFixture(t, false)
	rec := f.upload(t, c, "web", "/etc", "new.conf", []byte("port=80\n"), "*")
	var body struct{ Path, ETag string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusCreated || body.Path != "/etc/new.conf" || body.ETag != files.ETag([]byte("port=80\n")) {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if puts := f.docker.PutCalls(); len(puts) != 1 || puts[0].Dir != "/etc" {
		t.Fatalf("PutArchive : %d appel(s)", len(puts))
	}
}

func TestWriteRoutesReadOnly(t *testing.T) {
	f, c := writeFixture(t, true)
	// Refused first: before the container is even looked up.
	for _, rec := range []*httptest.ResponseRecorder{
		f.send("PUT", "/api/containers/absent/fs/file?path=/etc/app.conf", []byte("x"), c, map[string]string{"If-Match": "x"}),
		f.upload(t, c, "web", "/etc", "n", []byte("x"), "*"),
	} {
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" {
			t.Errorf("lecture seule : %d %s", rec.Code, rec.Body)
		}
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatal("PutArchive appelé en lecture seule")
	}
}

func TestWriteRoutesNeedSessionAndOrigin(t *testing.T) {
	f, c := writeFixture(t, false)
	if rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("x"), nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("sans session : %d", rec.Code)
	}
	if rec := f.send("PUT", "/api/containers/web/fs/file?path=/etc/app.conf", []byte("x"), c, map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_origin" {
		t.Errorf("Origin étrangère : %d %s", rec.Code, rec.Body)
	}
}

// Docker's own refusals: read-only rootfs or :ro volume → read_only, with
// Docker's message as detail; any other invalid extraction (the path
// changed after the checks) → conflict.
func TestWriteDockerRefused(t *testing.T) {
	f, c := writeFixture(t, false)
	f.docker.PutErr = fmt.Errorf("%w : container rootfs is marked read-only", dockerx.ErrRefused)
	rec := f.upload(t, c, "web", "/etc", "n", []byte("x"), "*")
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" || !strings.Contains(rec.Body.String(), "rootfs") {
		t.Fatalf("refus de Docker : %d %s", rec.Code, rec.Body)
	}
	f.docker.PutErr = fmt.Errorf("%w : extraction point is not a directory", dockerx.ErrPutInvalid)
	rec = f.upload(t, c, "web", "/etc", "n2", []byte("x"), "*")
	if rec.Code != http.StatusConflict || errCode(t, rec) != "conflict" {
		t.Fatalf("extraction refusée : %d %s", rec.Code, rec.Body)
	}
}

// Stopped container: the write waits for the index like every file route.
func TestWriteStoppedIndexing(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	f, c := stoppedFixture(t, g, fake, fsview.Options{})
	rec := f.send("PUT", "/api/containers/db/fs/file?path=/etc/motd", []byte("x"), c, map[string]string{"If-Match": files.ETag([]byte("db\n"))})
	if rec.Code != http.StatusAccepted || errCode(t, rec) != "indexing" {
		t.Fatalf("pendant l'indexation : %d %s", rec.Code, rec.Body)
	}
	close(g.open)
}

// The filename is decoded by mime.ParseMediaType (RFC 2231 filename*,
// quoted pairs) before it is validated: the value checked is the value
// written in the tar, never another one (Q19).
func TestUploadNameIsTheDecodedValue(t *testing.T) {
	f, c := writeFixture(t, false)
	send := func(disposition string) *httptest.ResponseRecorder {
		body, ct := multipartRaw(t, disposition, []byte("x"))
		return f.send("POST", "/api/containers/web/fs/upload?dir=/etc", body, c, map[string]string{"Content-Type": ct, "If-None-Match": "*"})
	}
	for _, d := range []string{
		`form-data; name="file"; filename*=UTF-8''..%2Fx`, // decodes to ../x
		`form-data; name="file"; filename="a\/b"`,         // quoted pair: a/b
		`form-data; name="file"; filename*=UTF-8''a%0Ab`,  // a, LF, b
	} {
		if rec := send(d); rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_name" {
			t.Errorf("%s : %d %s", d, rec.Code, rec.Body)
		}
	}
	if n := len(f.docker.PutCalls()); n != 0 {
		t.Fatalf("PutArchive appelé %d fois", n)
	}
	rec := send(`form-data; name="file"; filename*=UTF-8''%C3%A9t%C3%A9.txt`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"/etc/été.txt"`) {
		t.Fatalf("nom encodé : %d %s", rec.Code, rec.Body)
	}
	if h, _ := tarEntry(t, f.docker.PutCalls()[0].Tar); h != "été.txt" {
		t.Fatalf("nom écrit dans le tar : %q", h)
	}
}

// tarEntry returns the name and body of the single entry of a tar.
func tarEntry(t *testing.T, b []byte) (string, string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(b))
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(tr)
	return h.Name, string(body)
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/api/ -run 'Edit|Write|Upload' 2>&1 | tail -n 8`
Expected: `FAIL` ; les routes n'existent pas encore (`404 {"error":"not_found"}`).

- [ ] **Step 3: Implémenter** — créer `internal/api/fs_write.go` :

```go
package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/levy-cmd/hublot-kit/internal/files"
)

// uploadOverhead: room for the multipart boundaries and headers around the
// file of an upload.
const uploadOverhead = 1 << 20

// fsEdit is PUT /fs/file (F4, spec §5.3): the raw body replaces the file if
// If-Match is its current ETag. The body is read before taking a guard slot:
// a slow client must not hold one.
func (s *server) fsEdit(w http.ResponseWriter, r *http.Request) {
	if err := s.files.Allowed(); err != nil {
		writeFSError(w, err)
		return
	}
	info, p, ok := s.fsRequest(w, r, "path")
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, files.MaxEditBytes+1)) // files reports too_large
	if err != nil {
		writeFSError(w, err)
		return
	}
	ctx := r.Context()
	tag, release, err := guarded(s.guard, ctx, info.ID, func() (string, error) {
		return s.files.Edit(ctx, info, p, body, r.Header.Get("If-Match"))
	}, nil)
	release()
	if err != nil {
		writeFSError(w, err)
		return
	}
	w.Header().Set("ETag", tag)
	writeJSON(w, http.StatusOK, map[string]string{"etag": tag})
}

// fsUpload is POST /fs/upload?dir= (F4): one multipart part "file", whose
// filename is the name created in dir. It never replaces anything.
func (s *server) fsUpload(w http.ResponseWriter, r *http.Request) {
	if err := s.files.Allowed(); err != nil {
		writeFSError(w, err)
		return
	}
	info, dir, ok := s.fsRequest(w, r, "dir")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, files.MaxUploadBytes+uploadOverhead)
	name, content, err := readUpload(r)
	if err != nil {
		writeFSError(w, err)
		return
	}
	ctx := r.Context()
	written, release, err := guarded(s.guard, ctx, info.ID, func() (string, error) {
		return s.files.Upload(ctx, info, dir, name, content, r.Header.Get("If-None-Match"))
	}, nil)
	release()
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": written, "etag": files.ETag(content)})
}

// readUpload returns the filename and the content of the part "file". The
// filename is the value mime.ParseMediaType decodes (RFC 2231 filename*,
// quoted pairs): package files validates that very value, the one written
// in the tar. Not Part.FileName(): it keeps only the base name, and "a/b"
// would become "b" instead of being refused (spec §5.3: invalid_name). No
// part "file": an empty name, refused the same way. The content is read up
// to one byte past the cap: package files reports too_large.
func readUpload(r *http.Request) (string, []byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return "", nil, fmt.Errorf("%w : corps multipart attendu (%v)", files.ErrInvalidName, err)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return "", nil, fmt.Errorf("%w : aucun champ file", files.ErrInvalidName)
		}
		if err != nil {
			return "", nil, err
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			return "", nil, fmt.Errorf("%w : %v", files.ErrInvalidName, err)
		}
		b, err := io.ReadAll(io.LimitReader(part, files.MaxUploadBytes+1))
		if err != nil {
			return "", nil, err
		}
		return params["filename"], b, nil
	}
}

// isTooLarge: the body went past http.MaxBytesReader.
func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
```

Puis câbler `files` et les routes (`server.go`), classer les erreurs (`fserr.go`), passer le nom du paramètre à `fsRequest` (`fs.go`, et les deux appels existants) et calculer l'`ETag` de l'aperçu par `files.ETag` :

```diff
--- a/internal/api/server.go
+++ b/internal/api/server.go
@@ -14,6 +14,7 @@ import (
 	"github.com/levy-cmd/hublot-kit/internal/audit"
 	"github.com/levy-cmd/hublot-kit/internal/auth"
 	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+	"github.com/levy-cmd/hublot-kit/internal/files"
 	"github.com/levy-cmd/hublot-kit/internal/fsview"
 )
 
@@ -74,6 +75,7 @@ type server struct {
 	d        Deps
 	throttle *throttleLog
 	guard    *fsGuard
+	files    *files.Writer // the only writer (spec §7: read-only applied there)
 }
 
 // Spec §13 (FUSE): at most 4 file operations at once per container, each one
@@ -94,6 +96,8 @@ func New(d Deps) http.Handler {
 		d:        d,
 		throttle: &throttleLog{last: map[string]time.Time{}, now: logClock},
 		guard:    newFSGuard(fsMaxPerContainer, fsTimeout),
+		files: &files.Writer{ReadOnly: d.ReadOnly, Docker: d.Docker, Audit: d.Audit,
+			Open: d.OpenFS, Forget: d.Views.Forget},
 	}
 	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }
 
@@ -105,6 +109,8 @@ func New(d Deps) http.Handler {
 	mux.Handle("GET /api/containers/{id}/fs", session(s.fsList))
 	mux.Handle("GET /api/containers/{id}/fs/file", session(s.fsFile))
 	mux.Handle("GET /api/containers/{id}/fs/download", session(s.fsDownload))
+	mux.Handle("PUT /api/containers/{id}/fs/file", session(s.fsEdit))
+	mux.Handle("POST /api/containers/{id}/fs/upload", session(s.fsUpload))
 	mux.Handle("GET /api/containers/{id}/changes", session(s.changes))
 	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
 		writeError(w, http.StatusNotFound, "not_found", "")
```

```diff
--- a/internal/api/fserr.go
+++ b/internal/api/fserr.go
@@ -8,6 +8,7 @@ import (
 	"syscall"
 
 	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+	"github.com/levy-cmd/hublot-kit/internal/files"
 	"github.com/levy-cmd/hublot-kit/internal/fsview"
 )
 
@@ -43,6 +44,14 @@ func fsErrorStatus(err error) (int, string) {
 		return http.StatusGatewayTimeout, "internal"
 	case errors.Is(err, dockerx.ErrNotFound):
 		return http.StatusNotFound, "not_found"
+	case errors.Is(err, files.ErrReadOnly), errors.Is(err, dockerx.ErrRefused): // Q15: Docker's read-only rootfs or volume
+		return http.StatusForbidden, "read_only"
+	case errors.Is(err, files.ErrConflict), errors.Is(err, dockerx.ErrPutInvalid): // the path changed after the checks
+		return http.StatusConflict, "conflict"
+	case errors.Is(err, files.ErrInvalidName):
+		return http.StatusBadRequest, "invalid_name"
+	case isTooLarge(err):
+		return http.StatusRequestEntityTooLarge, "too_large"
 	case errors.Is(err, dockerx.ErrUnavailable):
 		return http.StatusServiceUnavailable, "docker_unavailable"
 	case errors.Is(err, fsview.ErrContainerChanged):
```

```diff
--- a/internal/api/fs.go
+++ b/internal/api/fs.go
@@ -17,18 +17,19 @@ import (
 // API URLs: it must never carry "/" nor start with ".".
 var containerRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
 
-// fsRequest reads {id} and ?path=, then resolves the container by Inspect
+// fsRequest reads {id} and the query parameter param (path, or dir for an
+// upload), then resolves the container by Inspect
 // (bounded by fsTimeout: a stuck daemon must not hang the request). Every
 // later step uses info.ID, the full ID: the guard counts per container, and a
 // container recreated under the same name cannot step in mid-request. On any
 // failure it answers itself.
-func (s *server) fsRequest(w http.ResponseWriter, r *http.Request) (info dockerx.ContainerInfo, p string, ok bool) {
+func (s *server) fsRequest(w http.ResponseWriter, r *http.Request, param string) (info dockerx.ContainerInfo, p string, ok bool) {
 	id := r.PathValue("id")
 	if !containerRef.MatchString(id) {
 		writeError(w, http.StatusNotFound, "not_found", "identifiant de conteneur invalide")
 		return info, "", false
 	}
-	raw := r.URL.Query().Get("path")
+	raw := r.URL.Query().Get(param)
 	if strings.ContainsRune(raw, 0) {
 		writeError(w, http.StatusNotFound, "not_found", "chemin invalide")
 		return info, "", false
@@ -71,7 +72,7 @@ type listingJSON struct {
 }
 
 func (s *server) fsList(w http.ResponseWriter, r *http.Request) {
-	info, p, ok := s.fsRequest(w, r)
+	info, p, ok := s.fsRequest(w, r, "path")
 	if !ok {
 		return
 	}
```

```diff
--- a/internal/api/fs_preview.go
+++ b/internal/api/fs_preview.go
@@ -2,12 +2,11 @@ package api
 
 import (
 	"bytes"
-	"crypto/sha256"
-	"encoding/hex"
 	"fmt"
 	"io"
 	"net/http"
 
+	"github.com/levy-cmd/hublot-kit/internal/files"
 	"github.com/levy-cmd/hublot-kit/internal/fsview"
 )
 
@@ -18,9 +17,9 @@ const binaryType = "application/octet-stream"
 
 // fsFile serves the preview of a regular file (spec §6, §7): text/plain or
 // one of four raster image types, never an active type. The ETag is the
-// SHA-256 of the content: the If-Match of a later edit (plan Explorateur 3).
+// SHA-256 of the content (files.ETag): the If-Match of an edit.
 func (s *server) fsFile(w http.ResponseWriter, r *http.Request) {
-	info, p, ok := s.fsRequest(w, r)
+	info, p, ok := s.fsRequest(w, r, "path")
 	if !ok {
 		return
 	}
@@ -50,9 +49,8 @@ func (s *server) fsFile(w http.ResponseWriter, r *http.Request) {
 		writeFSError(w, err)
 		return
 	}
-	sum := sha256.Sum256(b)
 	ct := previewType(b)
-	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
+	w.Header().Set("ETag", files.ETag(b)) // the If-Match of an edit
 	w.Header().Set("Content-Type", ct)
 	if ct == binaryType {
 		return // no text preview for a binary file: the UI offers the download
```

```diff
--- a/internal/api/fs_download.go
+++ b/internal/api/fs_download.go
@@ -14,7 +14,7 @@ import (
 // GetArchive) and answers 204: the UI asks first, then lets the browser
 // download.
 func (s *server) fsDownload(w http.ResponseWriter, r *http.Request) {
-	info, p, ok := s.fsRequest(w, r)
+	info, p, ok := s.fsRequest(w, r, "path")
 	if !ok {
 		return
 	}
```

- [ ] **Step 4: Vérifier**

Run: `go test -race ./internal/api/ 2>&1 | tail -n 3 && go vet ./... && GOOS=linux go vet ./... && gofmt -l internal`
Expected: `ok` ; les tests d'aperçu existants passent toujours (l'`ETag` garde son format).

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -F - <<'EOF'
api: PUT /fs/file et POST /fs/upload (F4), codes read_only, conflict, invalid_name

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 5: Écriture de bout en bout sur un vrai Docker

**Où tester :** Docker Desktop (conteneur arrêté, portable) ; Linux root (conteneur actif).

**Files:**
- Create: `internal/api/integration_write_test.go` (`//go:build integration`), `internal/api/integration_write_linux_test.go` (`//go:build integration && linux`)

**Interfaces:**
- Consumes: `(*fixture).init`, `session`, `send`, `upload` (tâche 4), `errCode`, `dockerCLI` (`integration_test.go`, même build tag Linux), `dockerx.New`.
- Produces: `whileIndexing(t, do func() *httptest.ResponseRecorder) *httptest.ResponseRecorder`.

> **À comprendre :** ces tests prouvent ce que le faux ne peut pas : que l'index est bien reconstruit après une écriture (l'aperçu relit `v2`), que Docker garde le mode et le propriétaire du tar (`640 1000 1001`), qu'un lien pendant reste intact après un envoi refusé, que `/etc/hosts`, un `--tmpfs` et un fichier monté (bind `:ro` de `/etc/os-release`, présent sur tout hôte de démon ; en lecture seule, une régression ne pourrait rien écrire dans l'hôte) sont refusés avant Docker, que `/sbin/docker-init` d'un `--init`, absent d'`Inspect.Mounts`, est refusé par le point de montage (Q21, R15), et que le refus d'un rootfs `--read-only` arrive en `read_only` avec le message de Docker. Une écriture sur un conteneur arrêté retire l'index : la requête suivante peut recevoir `202`, d'où `whileIndexing`.

- [ ] **Step 1: Conteneur arrêté** — créer `internal/api/integration_write_test.go` :

```go
//go:build integration

package api

// Writes end to end on a stopped container (spec §5.3, §10): real Docker,
// real Views. Portable: Docker Desktop answers from macOS.

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// whileIndexing repeats a request while it answers 202 (60 s at most).
func whileIndexing(t *testing.T, do func() *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	rec := do()
	for deadline := time.Now().Add(60 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		rec = do()
	}
	if rec.Code == http.StatusAccepted {
		t.Fatalf("indexation non terminée après 60 s : %s", rec.Body)
	}
	return rec
}

func TestIntegrationWriteStopped(t *testing.T) {
	name, ro := "hublot-it-write-stopped", "hublot-it-write-ro"
	exec.Command("docker", "rm", "-f", name, ro).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name, ro).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "sh", "-c", "echo v1 > /etc/app.conf").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	if out, err := exec.Command("docker", "create", "--name", ro, "--read-only", "alpine:3").CombinedOutput(); err != nil {
		t.Fatalf("docker create : %v %s", err, out)
	}
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // real Views, DefaultOptions
	c := f.session(t)
	file := "/api/containers/" + name + "/fs/file?path="
	preview := func(p string) func() *httptest.ResponseRecorder {
		return func() *httptest.ResponseRecorder { return f.do("GET", file+p, "", c) }
	}
	rec := whileIndexing(t, preview("/etc/app.conf"))
	if rec.Code != http.StatusOK || rec.Body.String() != "v1\n" {
		t.Fatalf("aperçu : %d %q", rec.Code, rec.Body)
	}
	tag := rec.Header().Get("ETag")
	if rec := f.send("PUT", file+"/etc/app.conf", []byte("v2\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusOK {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	// The index was dropped: the next access builds it again and sees v2.
	if rec := whileIndexing(t, preview("/etc/app.conf")); rec.Code != http.StatusOK || rec.Body.String() != "v2\n" {
		t.Fatalf("relu après l'édition : %d %q", rec.Code, rec.Body)
	}
	if rec := f.send("PUT", file+"/etc/app.conf", []byte("v3\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusConflict {
		t.Fatalf("ancien If-Match : %d %s", rec.Code, rec.Body)
	}
	rec = whileIndexing(t, func() *httptest.ResponseRecorder { return f.upload(t, c, name, "/etc", "new.conf", []byte("n\n"), "*") })
	if rec.Code != http.StatusCreated {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if rec := whileIndexing(t, preview("/etc/new.conf")); rec.Code != http.StatusOK || rec.Body.String() != "n\n" {
		t.Fatalf("fichier envoyé : %d %q", rec.Code, rec.Body)
	}
	// --read-only: Docker refuses the write itself → read_only, its message as detail.
	rec = whileIndexing(t, func() *httptest.ResponseRecorder { return f.upload(t, c, ro, "/etc", "x", []byte("x"), "*") })
	if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" {
		t.Fatalf("rootfs en lecture seule : %d %s", rec.Code, rec.Body)
	}
	t.Logf("MESURE refus de Docker : %s", rec.Body)
}
```

- [ ] **Step 2: Conteneur actif** — créer `internal/api/integration_write_linux_test.go` :

```go
//go:build integration && linux

package api

// Writes end to end on a running container (spec §5.3, §10): real Docker,
// real procfs (openat2, fstatfs). Needs root: scripts/test-linux.sh or the
// CI privileged job.

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestIntegrationWriteRunning(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
	name := "hublot-it-write-run"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	// /etc/os-release exists on every daemon host (Docker Desktop, Ubuntu,
	// Fedora); :ro, so a regression could never write into the host.
	dockerCLI(t, "run", "-d", "--name", name, "--tmpfs", "/scratch", "--init",
		"-v", "/etc/os-release:/app/conf.yml:ro", "alpine:3", "sleep", "300")
	dockerCLI(t, "exec", name, "sh", "-c", "echo v1 > /etc/app.conf && chown 1000:1001 /etc/app.conf && chmod 640 /etc/app.conf && ln -s /etc /linkdir && ln -s /nope /tmp/dangling")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil)
	c := f.session(t)
	base := "/api/containers/" + name
	rec := f.do("GET", base+"/fs/file?path=/etc/app.conf", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("aperçu : %d %s", rec.Code, rec.Body)
	}
	tag := rec.Header().Get("ETag")
	if rec := f.send("PUT", base+"/fs/file?path=/etc/app.conf", []byte("v2\n"), c, map[string]string{"If-Match": tag}); rec.Code != http.StatusOK {
		t.Fatalf("édition : %d %s", rec.Code, rec.Body)
	}
	if got := dockerCLI(t, "exec", name, "sh", "-c", "cat /etc/app.conf; stat -c '%a %u %g' /etc/app.conf"); got != "v2\n640 1000 1001" {
		t.Fatalf("contenu, mode et propriétaire gardés : %q", got)
	}
	for _, k := range []struct {
		label  string
		rec    func() (int, string)
		status int
		code   string
	}{
		{"lien parent", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/linkdir/app.conf", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 422, "link_in_path"},
		{"envoi sur un lien pendant", func() (int, string) {
			r := f.upload(t, c, name, "/tmp", "dangling", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 422, "link_in_path"},
		{"envoi dans /proc", func() (int, string) {
			r := f.upload(t, c, name, "/proc", "x", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		// Q21: Docker would answer "device or resource busy" (500), or take
		// the write where the container never sees it.
		{"édition de /etc/hosts", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/etc/hosts", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		{"envoi dans un --tmpfs", func() (int, string) {
			r := f.upload(t, c, name, "/scratch", "x", []byte("x"), "*")
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		{"fichier monté (Destination d'Inspect)", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/app/conf.yml", []byte("x"), c, map[string]string{"If-Match": tag})
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
		// --init mounts /sbin/docker-init without showing it in Inspect.Mounts:
		// only the STATX_MNT_ID check sees it (Q21).
		{"montage absent d'Inspect (--init)", func() (int, string) {
			r := f.send("PUT", base+"/fs/file?path=/sbin/docker-init", []byte("x"), c, map[string]string{"If-Match": tag})
			if !strings.Contains(r.Body.String(), "point de montage") {
				t.Errorf("docker-init : refus attendu par le point de montage : %s", r.Body)
			}
			return r.Code, errCode(t, r)
		}, 403, "forbidden_path"},
	} {
		if status, code := k.rec(); status != k.status || code != k.code {
			t.Errorf("%s : %d %s, attendu %d %s", k.label, status, code, k.status, k.code)
		}
	}
	if rec := f.upload(t, c, name, "/tmp", "new.txt", []byte("n\n"), "*"); rec.Code != http.StatusCreated {
		t.Fatalf("envoi : %d %s", rec.Code, rec.Body)
	}
	if got := dockerCLI(t, "exec", name, "sh", "-c", "cat /tmp/new.txt; stat -c '%a %u %g' /tmp/new.txt; readlink /tmp/dangling"); got != "n\n644 0 0\n/nope" {
		t.Fatalf("fichier envoyé, lien pendant intact : %q", got)
	}
}
```

- [ ] **Step 3: Vérifier**

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/api/ -run IntegrationWriteStopped -v 2>&1 | grep -E '^(---|ok|FAIL)|MESURE' | tail -n 5`
Expected: `PASS`, et la ligne `MESURE refus de Docker : {"error":"read_only","detail":"refusé par Docker : … container rootfs is marked read-only"}`.

Run: `scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 | tail -n 12`
Expected: `ok` pour chaque paquet ; `TestIntegrationWriteRunning` passe (vérifier avec `-run IntegrationWrite -v` qu'il n'est pas sauté). Contrôle une fois : sans le contrôle de point de montage de `files.Edit`, le cas `docker-init` échoue (la requête arrive jusqu'à `If-Match`).

- [ ] **Step 4: Commit**

```bash
git add internal/api
git commit -F - <<'EOF'
api: écriture de bout en bout sur un vrai Docker, conteneur actif et arrêté

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 6: Interface : API d'écriture, décodage strict, langages à la demande

**Où tester :** macOS (Vitest).

**Files:**
- Modify: `web/package.json`, `web/package-lock.json` (CodeMirror), `web/src/lib/api.ts`, `web/src/lib/api.test.ts`, `web/src/lib/i18n/fr.ts`, `web/src/lib/i18n/en.ts`
- Create: `web/src/lib/write.ts`, `web/src/lib/write.test.ts`, `web/src/lib/editor.ts`, `web/src/lib/editor.test.ts`

**Interfaces:**
- Consumes: `api()`, `fsPath`, `errorMessage`, `t`.
- Produces: `saveFile(id: string, path: string, text: string, etag: string): Promise<string>` ; `uploadFile(id: string, dir: string, file: File): Promise<{ path: string; etag: string }>` ; `Preview` texte : `{ kind: 'text'; text; etag; utf8: boolean }` ; `write.ts` : `EDIT_MAX`, `UPLOAD_MAX`, `decodeUtf8(bytes: Uint8Array): string | null`, `lineSeparatorOf(text: string): '\n' | '\r\n' | null`, `type Language`, `languageOf(path: string): Language`, `writeErrorMessage(kind: 'edit' | 'upload', code: string, detail?: string, status?: number): string` (`status` 504 → « l'écriture a peut-être eu lieu ») ; `editor.ts` : `loadLanguage(l: Language): Promise<Extension>`, `docText(state: EditorState): string` ; clés i18n `write.*`, `confirm.*`.

> **À comprendre :** `api()` posait `Content-Type: application/json` dès qu'un corps existait ; un `FormData` doit laisser le navigateur poser le `boundary` du multipart. La condition devient « corps texte ». L'aperçu lit désormais les octets et les décode strictement (Q16) : `utf8: false` garde l'affichage avec U+FFFD, mais l'interface n'offrira pas l'édition. Chaque langage est un `import()` séparé : Vite en fait un morceau chargé quand un fichier de ce type s'ouvre dans l'éditeur. `docText` relit le texte par `state.sliceDoc()` : `state.doc.toString()` joint toujours par LF, et un fichier CRLF serait enregistré en LF (R1 ; le test le montre).

- [ ] **Step 1: Dépendances**

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npm install codemirror@6.0.2 @codemirror/state@6.7.6 @codemirror/view@6.43.14 @codemirror/language@6.13.1 @codemirror/lang-json@6.0.2 @codemirror/lang-yaml@6.1.3 @codemirror/lang-javascript@6.2.5 @codemirror/lang-python@6.2.1 @codemirror/legacy-modes@6.5.5 2>&1 | tail -n 3`
Expected: `package.json` gagne un bloc `dependencies` :

```diff
--- a/web/package.json
+++ b/web/package.json
@@ -37,5 +37,16 @@
 	"imports": {
 		"#lib": "./src/lib/index.js",
 		"#lib/*": "./src/lib/*"
+	},
+	"dependencies": {
+		"@codemirror/lang-javascript": "^6.2.5",
+		"@codemirror/lang-json": "^6.0.2",
+		"@codemirror/lang-python": "^6.2.1",
+		"@codemirror/lang-yaml": "^6.1.3",
+		"@codemirror/language": "^6.13.1",
+		"@codemirror/legacy-modes": "^6.5.5",
+		"@codemirror/state": "^6.7.6",
+		"@codemirror/view": "^6.43.14",
+		"codemirror": "^6.0.2"
 	}
 }
```

- [ ] **Step 2: Tests qui échouent** — créer `web/src/lib/write.test.ts` et `web/src/lib/editor.test.ts`, compléter `web/src/lib/api.test.ts` :

```ts
import { describe, expect, it } from 'vitest';
import { t } from './i18n/index.ts';
import { decodeUtf8, languageOf, lineSeparatorOf, writeErrorMessage } from './write.ts';

describe('decodeUtf8', () => {
	it('keeps valid UTF-8, BOM included', () => {
		expect(decodeUtf8(new TextEncoder().encode('été'))).toBe('été');
		expect(decodeUtf8(new Uint8Array([0xef, 0xbb, 0xbf, 0x61]))).toBe('﻿a');
	});
	it('refuses invalid bytes: saving would replace them by U+FFFD', () => {
		expect(decodeUtf8(new Uint8Array([0x61, 0xe9, 0x62]))).toBeNull(); // Latin-1 "aéb"
	});
});

describe('lineSeparatorOf', () => {
	it('finds LF, CRLF, or refuses a mix', () => {
		expect(lineSeparatorOf('a\nb\n')).toBe('\n');
		expect(lineSeparatorOf('sans fin de ligne')).toBe('\n');
		expect(lineSeparatorOf('a\r\nb\r\n')).toBe('\r\n');
		expect(lineSeparatorOf('a\r\nb\n')).toBeNull();
	});
});

describe('languageOf', () => {
	it('maps the languages of spec §3, anything else is plain text', () => {
		expect(languageOf('/app/package.json')).toBe('json');
		expect(languageOf('/etc/compose.YML')).toBe('yaml');
		expect(languageOf('/srv/app.yaml')).toBe('yaml');
		expect(languageOf('/app/index.mjs')).toBe('javascript');
		expect(languageOf('/app/main.py')).toBe('python');
		expect(languageOf('/entrypoint.sh')).toBe('shell');
		expect(languageOf('/root/.bashrc')).toBe('shell');
		expect(languageOf('/etc/nginx/nginx.conf')).toBe('text');
		expect(languageOf('/etc/hosts')).toBe('text');
		expect(languageOf('/json')).toBe('text');
	});
});

describe('writeErrorMessage', () => {
	it('says what to do for each refusal', () => {
		expect(writeErrorMessage('edit', 'conflict')).toBe(t('write.conflictEdit'));
		expect(writeErrorMessage('upload', 'conflict')).toBe(t('write.conflictUpload'));
		expect(writeErrorMessage('upload', 'read_only', 'container rootfs is marked read-only')).toBe(
			`${t('write.readOnly')} (container rootfs is marked read-only)`
		);
		expect(writeErrorMessage('edit', 'link_in_path')).toBe(t('write.linkInPath'));
		expect(writeErrorMessage('edit', 'too_large')).toBe(t('write.editTooLarge'));
		expect(writeErrorMessage('upload', 'too_large')).toBe(t('write.uploadTooLarge'));
		expect(writeErrorMessage('upload', 'invalid_name')).toBe(t('error.invalid_name'));
		expect(writeErrorMessage('upload', 'forbidden_path', 'tmpfs /scratch')).toBe(`${t('write.forbidden')} (tmpfs /scratch)`);
		// 504: the guard gave up, the write may still happen (Q17).
		expect(writeErrorMessage('edit', 'internal', 'délai dépassé', 504)).toBe(t('write.maybeWritten'));
		expect(writeErrorMessage('edit', 'internal', undefined, 500)).toBe(t('error.internal'));
	});
});
```

```ts
import { EditorState } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { docText, loadLanguage } from './editor.ts';

describe('loadLanguage', () => {
	it('loads every language on demand', async () => {
		for (const l of ['json', 'yaml', 'javascript', 'python', 'shell', 'text'] as const) {
			await expect(loadLanguage(l)).resolves.toBeDefined();
		}
	});
});

describe('docText', () => {
	it('gives back the text with the file own line separator, byte for byte', () => {
		const cases: [string, string][] = [
			['a\r\nb\r\n', '\r\n'],
			['a\nb\n', '\n'],
			['a\rb\n', '\n'] // a lone CR stays a character
		];
		for (const [doc, sep] of cases) {
			const state = EditorState.create({ doc, extensions: [EditorState.lineSeparator.of(sep)] });
			expect(docText(state)).toBe(doc);
		}
		// The trap: Text.toString() always joins with LF.
		const crlf = EditorState.create({ doc: 'a\r\nb', extensions: [EditorState.lineSeparator.of('\r\n')] });
		expect(crlf.doc.toString()).toBe('a\nb');
	});
});
```

```diff
--- a/web/src/lib/api.test.ts
+++ b/web/src/lib/api.test.ts
@@ -1,5 +1,5 @@
 import { afterEach, describe, expect, it, vi } from 'vitest';
-import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile } from './api';
+import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile, saveFile, uploadFile } from './api';
 
 afterEach(() => vi.unstubAllGlobals());
 
@@ -63,7 +63,10 @@ describe('file routes', () => {
 
 	it('previews text, images and binaries', async () => {
 		stubFetch(async () => new Response('hello', { headers: { 'Content-Type': 'text/plain; charset=utf-8', ETag: '"abc"' } }));
-		await expect(previewFile('web', '/f')).resolves.toEqual({ kind: 'text', text: 'hello', etag: '"abc"' });
+		await expect(previewFile('web', '/f')).resolves.toEqual({ kind: 'text', text: 'hello', etag: '"abc"', utf8: true });
+
+		stubFetch(async () => new Response(new Uint8Array([0x61, 0xe9]), { headers: { 'Content-Type': 'text/plain; charset=utf-8' } }));
+		await expect(previewFile('web', '/latin1')).resolves.toMatchObject({ kind: 'text', utf8: false });
 
 		stubFetch(async () => new Response(new Uint8Array([137, 80, 78, 71]), { headers: { 'Content-Type': 'image/png', ETag: '"x"' } }));
 		const img = await previewFile('web', '/i.png');
@@ -105,3 +108,38 @@ describe('202 indexing (spec §6)', () => {
 		await expect(checkDownload(downloadUrl('db', '/etc'))).rejects.toMatchObject({ code: 'indexing' });
 	});
 });
+
+describe('writes (F4)', () => {
+	it('saveFile sends the text with If-Match and returns the new ETag', async () => {
+		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ etag: '"new"' })));
+		vi.stubGlobal('fetch', fetchMock);
+		await expect(saveFile('web', '/etc/a b', 'x\r\n', '"old"')).resolves.toBe('"new"');
+		const [url, init] = fetchMock.mock.calls[0];
+		expect(url).toBe('/api/containers/web/fs/file?path=%2Fetc%2Fa%20b');
+		expect(init?.method).toBe('PUT');
+		expect(init?.body).toBe('x\r\n');
+		const h = new Headers(init?.headers);
+		expect(h.get('If-Match')).toBe('"old"');
+		expect(h.get('Content-Type')).toBe('text/plain; charset=utf-8');
+	});
+
+	it('uploadFile posts multipart with If-None-Match: *, without a JSON Content-Type', async () => {
+		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ path: '/etc/n', etag: '"e"' }), { status: 201 }));
+		vi.stubGlobal('fetch', fetchMock);
+		const file = new File(['abc'], 'n');
+		await expect(uploadFile('web', '/etc', file)).resolves.toEqual({ path: '/etc/n', etag: '"e"' });
+		const [url, init] = fetchMock.mock.calls[0];
+		expect(url).toBe('/api/containers/web/fs/upload?dir=%2Fetc');
+		expect(init?.method).toBe('POST');
+		expect(init?.body).toBeInstanceOf(FormData);
+		expect((init?.body as FormData).get('file')).toBeInstanceOf(File);
+		const h = new Headers(init?.headers);
+		expect(h.get('If-None-Match')).toBe('*');
+		expect(h.has('Content-Type')).toBe(false); // the browser sets the multipart boundary
+	});
+
+	it('maps a refused write', async () => {
+		stubFetch(async () => new Response(JSON.stringify({ error: 'conflict' }), { status: 409 }));
+		await expect(saveFile('web', '/f', 'x', '"a"')).rejects.toMatchObject({ status: 409, code: 'conflict' });
+	});
+});
```

Run: `npx vitest run 2>&1 | tail -n 5` — Expected: `FAIL` ; `write.ts` et `editor.ts` n'existent pas, `saveFile` et `uploadFile` ne sont pas exportés.

- [ ] **Step 3: Implémenter** — créer `web/src/lib/write.ts` et `web/src/lib/editor.ts`, modifier `api.ts`, ajouter les textes (mêmes clés en français et en anglais) :

```ts
import { errorMessage, t } from './i18n/index.ts';

/** Caps of the agent (package files, Q14): an edit never exceeds the preview. */
export const EDIT_MAX = 2 * 1024 * 1024;
export const UPLOAD_MAX = 32 * 1024 * 1024;

/**
 * Strict UTF-8, BOM kept. null when the bytes are not valid UTF-8: the
 * editor would save U+FFFD in place of each invalid byte.
 */
export function decodeUtf8(bytes: Uint8Array): string | null {
	try {
		return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
	} catch {
		return null;
	}
}

/** The line separator of a text; null when CRLF and lone LF are mixed (saving would rewrite them all). */
export function lineSeparatorOf(text: string): '\n' | '\r\n' | null {
	const crlf = text.split('\r\n').length - 1;
	const lf = text.split('\n').length - 1;
	if (crlf === 0) return '\n';
	return crlf === lf ? '\r\n' : null;
}

/** Languages of spec §3; each one is a chunk loaded on demand (editor.ts). */
export type Language = 'json' | 'yaml' | 'javascript' | 'python' | 'shell' | 'text';

const BY_EXT: Record<string, Language> = {
	json: 'json', yml: 'yaml', yaml: 'yaml', js: 'javascript', mjs: 'javascript', cjs: 'javascript',
	py: 'python', sh: 'shell', bash: 'shell', zsh: 'shell'
};
const SHELL_NAMES = ['.bashrc', '.bash_profile', '.profile', '.zshrc'];

export function languageOf(path: string): Language {
	const name = (path.split('/').pop() ?? '').toLowerCase();
	if (SHELL_NAMES.includes(name)) return 'shell';
	const dot = name.lastIndexOf('.');
	return (dot > 0 && BY_EXT[name.slice(dot + 1)]) || 'text';
}

/**
 * Message for a refused edit or upload. A read_only from Docker and a
 * forbidden_path name their cause in the detail (shown as text). A 504 means
 * the guard gave up while the agent may still write (Q17).
 */
export function writeErrorMessage(kind: 'edit' | 'upload', code: string, detail?: string, status?: number): string {
	if (status === 504) return t('write.maybeWritten');
	switch (code) {
		case 'conflict':
			return t(kind === 'edit' ? 'write.conflictEdit' : 'write.conflictUpload');
		case 'read_only':
			return detail ? `${t('write.readOnly')} (${detail})` : t('write.readOnly');
		case 'link_in_path':
			return t('write.linkInPath');
		case 'too_large':
			return t(kind === 'edit' ? 'write.editTooLarge' : 'write.uploadTooLarge');
		case 'forbidden_path':
			return detail ? `${t('write.forbidden')} (${detail})` : t('write.forbidden');
		default:
			return errorMessage(code);
	}
}
```

```ts
import type { EditorState, Extension } from '@codemirror/state';
import type { Language } from './write.ts';

// Each language is its own chunk, loaded when a file of that kind is edited
// (spec §3: languages on demand).
const loaders: Record<Language, () => Promise<Extension>> = {
	json: async () => (await import('@codemirror/lang-json')).json(),
	yaml: async () => (await import('@codemirror/lang-yaml')).yaml(),
	javascript: async () => (await import('@codemirror/lang-javascript')).javascript(),
	python: async () => (await import('@codemirror/lang-python')).python(),
	shell: async () => {
		const [{ StreamLanguage }, { shell }] = await Promise.all([
			import('@codemirror/language'),
			import('@codemirror/legacy-modes/mode/shell')
		]);
		return StreamLanguage.define(shell);
	},
	text: async () => []
};

export const loadLanguage = (l: Language): Promise<Extension> => loaders[l]();

/**
 * The edited text with the state's line separator (EditorState.lineSeparator).
 * Not state.doc.toString(): it always joins lines with LF, and a CRLF file
 * would be saved in LF (Q16).
 */
export const docText = (state: EditorState): string => state.sliceDoc();
```

```diff
--- a/web/src/lib/api.ts
+++ b/web/src/lib/api.ts
@@ -1,3 +1,5 @@
+import { decodeUtf8 } from './write.ts';
+
 export class ApiError extends Error {
 	constructor(
 		public status: number,
@@ -21,7 +23,8 @@ export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
 		res = await fetch(`/api${path}`, {
 			credentials: 'same-origin',
 			...init,
-			headers: { ...(init.body !== undefined ? { 'Content-Type': 'application/json' } : {}), ...(init.headers ?? {}) }
+			// JSON for a string body; a FormData gets its multipart boundary from the browser.
+			headers: { ...(typeof init.body === 'string' ? { 'Content-Type': 'application/json' } : {}), ...(init.headers ?? {}) }
 		});
 	} catch {
 		throw new ApiError(0, 'network');
@@ -65,7 +68,7 @@ export type Entry = {
 // type is "dir" for a directory; for another path, entries is empty.
 export type Listing = { path: string; type: EntryType; entries: Entry[]; truncated: boolean };
 export type Preview =
-	| { kind: 'text'; text: string; etag: string }
+	| { kind: 'text'; text: string; etag: string; utf8: boolean } // utf8 false: shown with U+FFFD, not editable
 	| { kind: 'image'; blob: Blob; etag: string }
 	| { kind: 'binary' };
 
@@ -92,7 +95,11 @@ export async function previewFile(id: string, path: string): Promise<Preview> {
 	const type = res.headers.get('Content-Type') ?? '';
 	const etag = res.headers.get('ETag') ?? '';
 	if (/^image\/(png|jpeg|gif|webp)$/.test(type)) return { kind: 'image', blob: await res.blob(), etag };
-	if (type.startsWith('text/plain')) return { kind: 'text', text: await res.text(), etag };
+	if (type.startsWith('text/plain')) {
+		const bytes = new Uint8Array(await res.arrayBuffer());
+		const strict = decodeUtf8(bytes);
+		return { kind: 'text', text: strict ?? new TextDecoder().decode(bytes), etag, utf8: strict !== null };
+	}
 	return { kind: 'binary' };
 }
 
@@ -105,3 +112,24 @@ export async function checkDownload(url: string): Promise<void> {
 
 export type Change = { path: string; kind: 'added' | 'modified' | 'deleted' };
 export const listChanges = (id: string) => api<Change[]>(`/containers/${encodeURIComponent(id)}/changes`);
+
+// F4 (spec §5.3). The If-Match is the ETag of the preview, as received.
+export async function saveFile(id: string, path: string, text: string, etag: string): Promise<string> {
+	const res = await api<{ etag: string }>(fsPath(id, '/file', path), {
+		method: 'PUT',
+		body: text,
+		headers: { 'If-Match': etag, 'Content-Type': 'text/plain; charset=utf-8' }
+	});
+	return res.etag;
+}
+
+// One file per request; the agent refuses an existing name (If-None-Match: *).
+export function uploadFile(id: string, dir: string, file: File): Promise<{ path: string; etag: string }> {
+	const body = new FormData();
+	body.append('file', file, file.name);
+	return api(`/containers/${encodeURIComponent(id)}/fs/upload?dir=${encodeURIComponent(dir)}`, {
+		method: 'POST',
+		body,
+		headers: { 'If-None-Match': '*' }
+	});
+}
```

```diff
--- a/web/src/lib/i18n/fr.ts
+++ b/web/src/lib/i18n/fr.ts
@@ -68,6 +68,37 @@ export const fr = {
 	'files.unit.mib': 'Mio',
 	'files.unit.gib': 'Gio',
 	'files.unit.tib': 'Tio',
+	'write.edit': 'Modifier',
+	'write.save': 'Enregistrer…',
+	'write.cancel': 'Annuler',
+	'write.saving': 'Enregistrement…',
+	'write.saved': 'Enregistré.',
+	'write.editorLoading': "Chargement de l'éditeur…",
+	'write.editorFailed': "L'éditeur n'a pas pu se charger. Rechargez la page.",
+	'write.notEditable':
+		"Modification impossible ici : le fichier n'est pas en UTF-8, ou mélange les fins de ligne CRLF et LF. Téléchargez-le.",
+	'write.upload': 'Envoyer un fichier ici',
+	'write.uploading': 'Envoi…',
+	'write.conflictEdit':
+		'Le fichier a changé depuis son ouverture. Copiez vos modifications, puis rechargez-le : elles seraient perdues.',
+	'write.conflictUpload': "Un fichier, un dossier ou un lien porte déjà ce nom ici. Renommez le fichier avant de l'envoyer.",
+	'write.readOnly': 'Écriture refusée : la cible est en lecture seule',
+	'write.linkInPath':
+		"Ce chemin passe par un lien symbolique : l'agent n'écrit que sur un chemin sans lien. Ouvrez la cible du lien.",
+	'write.editTooLarge': "Plus de 2 Mio : l'éditeur s'arrête là.",
+	'write.uploadTooLarge': 'Plus de 32 Mio : utilisez docker cp pour ce fichier.',
+	'write.forbidden':
+		"Écriture refusée à cet endroit : Docker n'y écrit pas (fichier ou tmpfs monté, /etc/hosts, /dev, /proc, /sys), ou un montage du conteneur expose l'hôte",
+	'write.maybeWritten':
+		"L'agent a dépassé son délai, mais l'écriture a peut-être eu lieu. Rechargez le fichier avant de réessayer.",
+	'confirm.edit': 'Enregistrer ce fichier dans le conteneur ?',
+	'confirm.upload': 'Envoyer ce fichier dans le conteneur ?',
+	'confirm.container': 'Conteneur',
+	'confirm.path': 'Chemin',
+	'confirm.size': 'Taille',
+	'confirm.stoppedNote': 'Conteneur arrêté : son index sera reconstruit au prochain accès.',
+	'confirm.ok': 'Confirmer',
+	'confirm.cancel': 'Annuler',
 	'error.unauthorized': 'Jeton invalide ou session expirée.',
 	'error.invalid_name': 'Nom de fichier invalide.',
 	'error.forbidden_origin':
```

```diff
--- a/web/src/lib/i18n/en.ts
+++ b/web/src/lib/i18n/en.ts
@@ -69,6 +69,34 @@ export const en: Record<Key, string> = {
 	'files.unit.mib': 'MiB',
 	'files.unit.gib': 'GiB',
 	'files.unit.tib': 'TiB',
+	'write.edit': 'Edit',
+	'write.save': 'Save…',
+	'write.cancel': 'Cancel',
+	'write.saving': 'Saving…',
+	'write.saved': 'Saved.',
+	'write.editorLoading': 'Loading the editor…',
+	'write.editorFailed': 'The editor could not load. Reload the page.',
+	'write.notEditable':
+		'Cannot edit here: the file is not UTF-8, or it mixes CRLF and LF line endings. Download it.',
+	'write.upload': 'Upload a file here',
+	'write.uploading': 'Uploading…',
+	'write.conflictEdit': 'The file changed since you opened it. Copy your changes, then reload it: they would be lost.',
+	'write.conflictUpload': 'A file, folder or link already has this name here. Rename the file before uploading it.',
+	'write.readOnly': 'Write refused: the target is read-only',
+	'write.linkInPath': 'This path goes through a symbolic link: the agent only writes on a path without links. Open the link target.',
+	'write.editTooLarge': 'Over 2 MiB: the editor stops there.',
+	'write.uploadTooLarge': 'Over 32 MiB: use docker cp for this file.',
+	'write.forbidden':
+		'Write refused here: Docker does not write there (mounted file or tmpfs, /etc/hosts, /dev, /proc, /sys), or a mount of the container exposes the host',
+	'write.maybeWritten': 'The agent timed out, but the write may have happened. Reload the file before trying again.',
+	'confirm.edit': 'Save this file in the container?',
+	'confirm.upload': 'Upload this file to the container?',
+	'confirm.container': 'Container',
+	'confirm.path': 'Path',
+	'confirm.size': 'Size',
+	'confirm.stoppedNote': 'Stopped container: its index will be rebuilt on next access.',
+	'confirm.ok': 'Confirm',
+	'confirm.cancel': 'Cancel',
 	'error.unauthorized': 'Invalid token or expired session.',
 	'error.invalid_name': 'Invalid file name.',
 	'error.forbidden_origin':
```

- [ ] **Step 4: Vérifier**

Run: `npx vitest run 2>&1 | tail -n 4 && npm run check 2>&1 | tail -n 3 && npm run lint 2>&1 | tail -n 3`
Expected: `55 passed` ; `0 ERRORS 0 WARNINGS` ; ESLint sans sortie.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/src/lib
git commit -F - <<'EOF'
web: saveFile et uploadFile, décodage UTF-8 strict, langages CodeMirror à la demande

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 7: Interface : éditeur, boîte de confirmation, envoi, boutons cachés en lecture seule

**Où tester :** macOS (`svelte-check`, ESLint, build), puis `scripts/e2e.sh` avec un parcours jetable.

**Files:**
- Create: `web/src/lib/components/FileEditor.svelte`, `web/src/lib/components/ConfirmDialog.svelte`
- Modify: `web/src/lib/components/FilePreview.svelte`, `web/src/lib/components/FileBrowser.svelte`, `web/src/routes/+layout.svelte`, `web/src/routes/c/[id]/+page.svelte`

**Interfaces:**
- Consumes: tâche 6 (`saveFile`, `uploadFile`, `Preview.utf8`, `write.ts`, `loadLanguage`, `docText`), le `meta` du layout, `joinPath`, `formatSize`.
- Produces: `<FileEditor {path} {text} />` avec la méthode `content(): string | null` (par `bind:this`) ; `<ConfirmDialog title container path details? onconfirm oncancel />` ; `FilePreview` gagne les props `name: string`, `readOnly?: boolean` (défaut `true`) ; `FileBrowser` gagne `name: string`, `readOnly?: boolean` (défaut `true`) ; contexte `'readOnly'` : `() => boolean`, posé par le layout.

> **À comprendre :** le layout demande déjà `/api/meta` ; il expose `readOnly` par un contexte, la page le lit et le passe à l'explorateur, qui le passe à l'aperçu. Tant que la réponse n'est pas là, ou si elle échoue, `readOnly` vaut `true` : aucun bouton d'écriture (§9). Un texte de plus de `EDIT_MAX` octets est refusé avant la confirmation. L'éditeur importe CodeMirror quand il s'ouvre, avec le séparateur de lignes du fichier ; la boîte de confirmation est un `<dialog>` natif monté seulement pendant la confirmation (Échap vaut Annuler). Un enregistrement ou un envoi dont la réponse arrive après un changement de fichier n'écrase rien à l'écran (`path !== wanted`, `dir === u.dir`).

- [ ] **Step 1: Composants nouveaux** — créer `web/src/lib/components/FileEditor.svelte` et `web/src/lib/components/ConfirmDialog.svelte` :

```svelte
<script lang="ts">
	import type { EditorView } from '@codemirror/view';
	import { docText, loadLanguage } from '#lib/editor.ts';
	import { t } from '#lib/i18n/index.ts';
	import { languageOf, lineSeparatorOf } from '#lib/write.ts';

	// CodeMirror and the language are loaded when the editor opens: none of
	// it is in the initial bundle (spec §3).
	let { path, text }: { path: string; text: string } = $props();

	let host = $state<HTMLDivElement>();
	let view: EditorView | undefined;
	let ready = $state(false);
	let failed = $state(false);

	$effect(() => {
		const target = host;
		const doc = text;
		const lang = languageOf(path);
		if (!target) return;
		let current = true;
		ready = false;
		failed = false;
		Promise.all([import('codemirror'), import('@codemirror/state'), loadLanguage(lang)])
			.then(([{ EditorView, basicSetup }, { EditorState }, language]) => {
				if (!current) return;
				// The file's own line separator: saving never rewrites line endings.
				const sep = lineSeparatorOf(doc) ?? '\n';
				view = new EditorView({
					doc,
					parent: target,
					extensions: [basicSetup, language, EditorState.lineSeparator.of(sep)]
				});
				ready = true;
			})
			.catch(() => {
				if (current) failed = true;
			});
		return () => {
			current = false;
			view?.destroy();
			view = undefined;
		};
	});

	/** The edited text, with the file's line separator; null while the editor loads. */
	export function content(): string | null {
		return view ? docText(view.state) : null;
	}
</script>

{#if failed}
	<p role="alert">{t('write.editorFailed')}</p>
{:else if !ready}
	<p>{t('write.editorLoading')}</p>
{/if}
<div class="editor" bind:this={host}></div>

<style>
	.editor { border: 1px solid #ddd; border-radius: 0.25rem; max-height: 70vh; overflow: auto; }
	.editor :global(.cm-editor) { min-height: 12rem; }
	[role='alert'] { color: #b00020; }
</style>
```

```svelte
<script lang="ts">
	import { t } from '#lib/i18n/index.ts';

	// Every write goes through this dialog (spec §9): it names the container
	// and the path. Shown while mounted; Cancel has the focus first.
	let {
		title,
		container,
		path,
		details = [],
		onconfirm,
		oncancel
	}: {
		title: string;
		container: string;
		path: string;
		details?: string[];
		onconfirm: () => void;
		oncancel: () => void;
	} = $props();

	let dialog = $state<HTMLDialogElement>();

	$effect(() => {
		const d = dialog;
		d?.showModal();
		return () => d?.close();
	});
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="confirm-title"
	oncancel={(e) => {
		e.preventDefault(); // Escape: same as Cancel, the parent unmounts the dialog
		oncancel();
	}}
>
	<h2 id="confirm-title">{title}</h2>
	<dl>
		<dt>{t('confirm.container')}</dt>
		<dd>{container}</dd>
		<dt>{t('confirm.path')}</dt>
		<dd><code>{path}</code></dd>
	</dl>
	{#each details as d, i (i)}<p>{d}</p>{/each}
	<div class="actions">
		<button type="button" onclick={oncancel}>{t('confirm.cancel')}</button>
		<button type="button" class="primary" onclick={onconfirm}>{t('confirm.ok')}</button>
	</div>
</dialog>

<style>
	dialog { max-width: min(36rem, calc(100vw - 2rem)); }
	h2 { font-size: 1.1rem; margin-top: 0; }
	dl { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 0.25rem 0.75rem; }
	dd { margin: 0; overflow-wrap: anywhere; }
	.actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
	.primary { font-weight: 600; }
</style>
```

- [ ] **Step 2: Aperçu, explorateur, layout, page**

```diff
--- a/web/src/lib/components/FilePreview.svelte
+++ b/web/src/lib/components/FilePreview.svelte
@@ -1,10 +1,20 @@
 <script lang="ts">
 	import { goto } from '$app/navigation';
-	import { ApiError, checkDownload, downloadUrl, previewFile, type Entry, type Preview } from '#lib/api.ts';
+	import { ApiError, checkDownload, downloadUrl, previewFile, saveFile, type Entry, type Preview } from '#lib/api.ts';
 	import { baseName, downloadErrorMessage, formatMode, formatSize, fsErrorMessage, indexingPercent } from '#lib/files.ts';
 	import { t } from '#lib/i18n/index.ts';
+	import { EDIT_MAX, lineSeparatorOf, writeErrorMessage } from '#lib/write.ts';
+	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
+	import FileEditor from '#lib/components/FileEditor.svelte';
 
-	let { id, path, entry, stopped = false }: { id: string; path: string; entry?: Entry; stopped?: boolean } = $props();
+	let {
+		id,
+		name,
+		path,
+		entry,
+		stopped = false,
+		readOnly = true
+	}: { id: string; name: string; path: string; entry?: Entry; stopped?: boolean; readOnly?: boolean } = $props();
 
 	let preview = $state<Preview | null>(null);
 	let imageUrl = $state('');
@@ -13,6 +23,14 @@
 	let loading = $state(false);
 	let downloadError = $state('');
 	let reloads = $state(0); // bumped by the Reload button: same path, new request
+	let editing = $state(false);
+	let editor = $state<FileEditor>();
+	let pending = $state<string | null>(null); // text waiting for the confirmation
+	let saving = $state(false);
+	let writeError = $state('');
+	let saved = $state(false);
+	// Editable: valid UTF-8 and one kind of line ending (saving would change the bytes otherwise).
+	let editable = $derived(preview?.kind === 'text' && preview.utf8 && lineSeparatorOf(preview.text) !== null);
 
 	$effect(() => {
 		const wanted = path;
@@ -24,6 +42,10 @@
 		imageUrl = '';
 		error = '';
 		downloadError = '';
+		editing = false;
+		pending = null;
+		writeError = '';
+		saved = false;
 		loading = true;
 		previewFile(id, wanted)
 			.then((p) => {
@@ -60,6 +82,41 @@
 		};
 	});
 
+	function askSave() {
+		const text = editor?.content() ?? null; // null while the editor loads: nothing to confirm
+		writeError = '';
+		if (text !== null && new TextEncoder().encode(text).length > EDIT_MAX) {
+			writeError = t('write.editTooLarge'); // the agent would refuse it: no confirmation
+			return;
+		}
+		pending = text;
+	}
+
+	async function save() {
+		const text = pending;
+		const wanted = path;
+		pending = null;
+		if (text === null || preview?.kind !== 'text') return;
+		saving = true;
+		try {
+			const etag = await saveFile(id, wanted, text, preview.etag);
+			if (path !== wanted) return;
+			preview = { ...preview, text, etag };
+			editing = false;
+			saved = true;
+		} catch (e) {
+			if (path !== wanted) return;
+			if (e instanceof ApiError && e.status === 401) {
+				goto('/login').catch(() => {});
+				return;
+			}
+			writeError =
+				e instanceof ApiError ? writeErrorMessage('edit', e.code, e.detail, e.status) : writeErrorMessage('edit', 'unknown');
+		} finally {
+			saving = false;
+		}
+	}
+
 	async function download() {
 		downloadError = '';
 		const url = downloadUrl(id, path);
@@ -82,7 +139,17 @@
 		</p>
 	{/if}
 	<button type="button" onclick={download}>{t('files.download')}</button>
+	{#if !readOnly && editable && !editing}
+		<button
+			type="button"
+			onclick={() => {
+				editing = true;
+				saved = false;
+			}}>{t('write.edit')}</button
+		>
+	{/if}
 	{#if downloadError}<p role="alert">{downloadError}</p>{/if}
+	{#if saved}<p role="status">{t('write.saved')}</p>{/if}
 </header>
 
 {#if indexing !== null}
@@ -94,14 +161,40 @@
 {:else if error}
 	<p role="alert">{error}</p>
 	<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
+{:else if preview?.kind === 'text' && editing}
+	<FileEditor bind:this={editor} {path} text={preview.text} />
+	<div class="actions">
+		<button type="button" onclick={askSave} disabled={saving}>{saving ? t('write.saving') : t('write.save')}</button>
+		<button
+			type="button"
+			disabled={saving}
+			onclick={() => {
+				editing = false;
+				writeError = '';
+			}}>{t('write.cancel')}</button
+		>
+	</div>
+	{#if writeError}<p role="alert">{writeError}</p>{/if}
 {:else if preview?.kind === 'text'}
 	<pre>{preview.text}</pre>
+	{#if !readOnly && !editable}<p class="note">{t('write.notEditable')}</p>{/if}
 {:else if preview?.kind === 'image' && imageUrl}
 	<img src={imageUrl} alt={baseName(path)} />
 {:else if preview?.kind === 'binary'}
 	<p>{t('files.binary')}</p>
 {/if}
 
+{#if pending !== null}
+	<ConfirmDialog
+		title={t('confirm.edit')}
+		container={`${name} (${id.slice(0, 12)})`}
+		{path}
+		details={stopped ? [t('confirm.stoppedNote')] : []}
+		onconfirm={save}
+		oncancel={() => (pending = null)}
+	/>
+{/if}
+
 <style>
 	header { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: baseline; }
 	h2 { font-size: 1.1rem; margin: 0; word-break: break-all; }
@@ -109,5 +202,6 @@
 	pre { overflow: auto; max-height: 70vh; padding: 0.75rem; background: #f6f6f6; border-radius: 0.25rem; white-space: pre; }
 	img { max-width: 100%; height: auto; }
 	.note { font-style: italic; }
+	.actions { display: flex; gap: 0.5rem; margin-top: 0.5rem; }
 	[role='alert'] { color: #b00020; }
 </style>
```

```diff
--- a/web/src/lib/components/FileBrowser.svelte
+++ b/web/src/lib/components/FileBrowser.svelte
@@ -9,7 +9,8 @@
 		listDir,
 		type Change,
 		type Entry,
-		type Listing
+		type Listing,
+		uploadFile
 	} from '#lib/api.ts';
 	import { changeMarks, deletedIn, kindLabel, kindLetter, truncateNames } from '#lib/changes.ts';
 	import {
@@ -25,9 +26,16 @@
 		sortEntries
 	} from '#lib/files.ts';
 	import { errorMessage, t } from '#lib/i18n/index.ts';
+	import { UPLOAD_MAX, writeErrorMessage } from '#lib/write.ts';
+	import ConfirmDialog from '#lib/components/ConfirmDialog.svelte';
 	import FilePreview from '#lib/components/FilePreview.svelte';
 
-	let { id, stopped = false }: { id: string; stopped?: boolean } = $props();
+	let {
+		id,
+		name,
+		stopped = false,
+		readOnly = true
+	}: { id: string; name: string; stopped?: boolean; readOnly?: boolean } = $props();
 
 	// The URL carries the state (?path=dir&file=selected): back button and links work.
 	let dir = $derived(page.url.searchParams.get('path') || '/');
@@ -43,6 +51,10 @@
 	let marks = $derived(changeMarks(changes));
 	let deleted = $derived(deletedIn(changes, dir));
 	let selected = $derived(file ? entries.find((e) => joinPath(dir, e.name) === file) : undefined);
+	let picker = $state<HTMLInputElement>();
+	let toUpload = $state<{ file: File; dir: string } | null>(null); // waiting for the confirmation
+	let uploading = $state(false);
+	let uploadError = $state('');
 
 	const href = (path: string, selectedFile?: string) =>
 		'?' + new URLSearchParams(selectedFile ? { path, file: selectedFile } : { path });
@@ -114,6 +126,41 @@
 		else go(dir, p);
 	}
 
+	function picked(e: Event & { currentTarget: HTMLInputElement }) {
+		const f = e.currentTarget.files?.[0];
+		e.currentTarget.value = ''; // the same file can be picked again
+		uploadError = '';
+		if (!f) return;
+		if (f.size > UPLOAD_MAX) {
+			uploadError = t('write.uploadTooLarge');
+			return;
+		}
+		toUpload = { file: f, dir };
+	}
+
+	async function upload() {
+		const u = toUpload;
+		toUpload = null;
+		if (!u) return;
+		uploading = true;
+		try {
+			const res = await uploadFile(id, u.dir, u.file);
+			reloads++;
+			if (dir === u.dir) go(u.dir, res.path);
+		} catch (e) {
+			if (e instanceof ApiError && e.status === 401) {
+				goto('/login').catch(() => {});
+				return;
+			}
+			uploadError =
+				e instanceof ApiError
+					? writeErrorMessage('upload', e.code, e.detail, e.status)
+					: writeErrorMessage('upload', 'unknown');
+		} finally {
+			uploading = false;
+		}
+	}
+
 	async function downloadDir() {
 		downloadError = '';
 		const url = downloadUrl(id, dir);
@@ -137,8 +184,15 @@
 		</nav>
 		<div class="tools">
 			<button type="button" onclick={downloadDir}>{t('files.downloadDir')}</button>
+			{#if !readOnly}
+				<input type="file" bind:this={picker} onchange={picked} hidden />
+				<button type="button" disabled={uploading} onclick={() => picker?.click()}>
+					{uploading ? t('write.uploading') : t('write.upload')}
+				</button>
+			{/if}
 		</div>
 		{#if downloadError}<p role="alert">{downloadError}</p>{/if}
+		{#if uploadError}<p role="alert">{uploadError}</p>{/if}
 
 		{#if indexing !== null}
 			<p role="status">{t('files.indexing')} : {indexing} %</p>
@@ -198,13 +252,27 @@
 
 	<section class="preview">
 		{#if file}
-			<FilePreview {id} path={file} entry={selected} {stopped} />
+			<FilePreview {id} {name} path={file} entry={selected} {stopped} {readOnly} />
 		{:else}
 			<p>{t('files.selectFile')}</p>
 		{/if}
 	</section>
 </div>
 
+{#if toUpload}
+	<ConfirmDialog
+		title={t('confirm.upload')}
+		container={`${name} (${id.slice(0, 12)})`}
+		path={joinPath(toUpload.dir, toUpload.file.name)}
+		details={[
+			`${t('confirm.size')} : ${formatSize(toUpload.file.size)}`,
+			...(stopped ? [t('confirm.stoppedNote')] : [])
+		]}
+		onconfirm={upload}
+		oncancel={() => (toUpload = null)}
+	/>
+{/if}
+
 <style>
 	.browser { display: grid; gap: 1rem; grid-template-columns: minmax(0, 1fr); }
 	@media (min-width: 60rem) {
```

```diff
--- a/web/src/routes/+layout.svelte
+++ b/web/src/routes/+layout.svelte
@@ -1,5 +1,6 @@
 <script lang="ts">
 	import favicon from '#lib/assets/favicon.svg';
+	import { setContext } from 'svelte';
 	import { goto } from '$app/navigation';
 	import { page } from '$app/state';
 	import { ApiError, getMeta, logout, type Meta } from '#lib/api.ts';
@@ -7,6 +8,9 @@
 
 	let { children } = $props();
 	let meta = $state<Meta | null>(null);
+	// Pages read it instead of asking /api/meta again; true until the answer
+	// arrives: write buttons never show by mistake (spec §9).
+	setContext('readOnly', () => meta?.readOnly ?? true);
 
 	// Re-checks the session on every navigation; an expired session sends back to /login.
 	$effect(() => {
```

```diff
--- a/web/src/routes/c/[id]/+page.svelte
+++ b/web/src/routes/c/[id]/+page.svelte
@@ -1,6 +1,7 @@
 <script lang="ts">
 	import { goto } from '$app/navigation';
 	import { page } from '$app/state';
+	import { getContext } from 'svelte';
 	import { ApiError, listContainers, type Container } from '#lib/api.ts';
 	import { stateLabel } from '#lib/containers.ts';
 	import { errorMessage, t } from '#lib/i18n/index.ts';
@@ -12,6 +13,8 @@
 	let error = $state('');
 	let reloads = $state(0); // bumped by the Reload button
 	let tab = $derived(page.url.searchParams.get('tab') === 'changes' ? 'changes' : 'files');
+	// From the layout's /api/meta: true until it answers (spec §9).
+	const readOnly = getContext<() => boolean>('readOnly');
 
 	// The list gives the name, image and state; GET /api/containers/{id}
 	// arrives with the X-ray tab (phase Observation).
@@ -59,7 +62,12 @@
 		{#if tab === 'changes'}
 			<ChangesList id={container.id} />
 		{:else}
-			<FileBrowser id={container.id} stopped={container.state !== 'running' && container.state !== 'paused'} />
+			<FileBrowser
+				id={container.id}
+				name={container.name}
+				readOnly={readOnly()}
+				stopped={container.state !== 'running' && container.state !== 'paused'}
+			/>
 		{/if}
 	{/if}
 </main>
```

- [ ] **Step 3: Vérifier le typage, le style et le découpage**

Run: `npm run check 2>&1 | tail -n 3 && npm run lint 2>&1 | tail -n 3 && npx vitest run 2>&1 | tail -n 3 && npm run build 2>&1 | tail -n 2 && grep -l 'cm-content' build/_app/immutable/nodes/*.js build/_app/immutable/entry/*.js ; grep -l 'cm-content' build/_app/immutable/chunks/*.js | wc -l`
Expected: `0 ERRORS 0 WARNINGS` ; ESLint muet ; `55 passed` ; build réussi ; le premier `grep` ne trouve rien (CodeMirror n'est ni dans l'entrée ni dans un nœud de page) ; le second compte `1` morceau.

- [ ] **Step 4: Parcours jetable d'écriture** — créer `web/e2e/zz-ecriture.e2e.ts` (il ne sera **pas** commité : §10 garde un seul parcours) :

```ts
import { expect, test } from '@playwright/test';

// Throwaway check (not committed): edit and upload on the stopped container
// hublot-e2e, through CodeMirror and the confirmation dialog, under the CSP.
test('écriture : édition et envoi', async ({ page }) => {
	const errors: string[] = [];
	page.on('console', (m) => {
		if (m.type() === 'error') errors.push(m.text());
	});
	await page.goto('/login');
	await page.locator('#token').fill(process.env.HUBLOT_TOKEN!);
	await page.locator('button[type=submit]').click();
	await page.goto('/c/hublot-e2e?path=%2Froot&file=%2Froot%2Fnote.txt');
	await expect(page.locator('.preview pre')).toContainText('bonjour hublot');
	await page.getByRole('button', { name: 'Modifier', exact: true }).click();
	const editor = page.locator('.cm-content');
	await expect(editor).toBeVisible();
	await editor.click();
	await page.keyboard.press('ControlOrMeta+End');
	await page.keyboard.type(' édité');
	await page.getByRole('button', { name: 'Enregistrer…' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toContainText('hublot-e2e');
	await expect(dialog).toContainText('/root/note.txt');
	await expect(dialog).toContainText('index sera reconstruit');
	await expect(dialog.getByRole('button', { name: 'Annuler' })).toBeFocused();
	await dialog.getByRole('button', { name: 'Confirmer' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Enregistré.' })).toBeVisible();
	await expect(page.locator('.preview pre')).toContainText('bonjour hublot édité');
	// Reload: the index is rebuilt, the agent serves the new content.
	await page.reload();
	await expect(page.locator('.preview pre')).toContainText('bonjour hublot édité');
	// Upload in /root.
	await page.locator('input[type=file]').setInputFiles({ name: 'envoi.txt', mimeType: 'text/plain', buffer: Buffer.from('salut\n') });
	await expect(page.getByRole('dialog')).toContainText('/root/envoi.txt');
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(page).toHaveURL(/file=%2Froot%2Fenvoi\.txt/);
	await expect(page.locator('.preview pre')).toContainText('salut');
	// Same name again: refused, nothing replaced.
	await page.locator('input[type=file]').setInputFiles({ name: 'envoi.txt', mimeType: 'text/plain', buffer: Buffer.from('autre\n') });
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(page.getByRole('alert').filter({ hasText: 'porte déjà ce nom' })).toBeVisible();
	// A CRLF file keeps its CRLF line endings after an edit (Q16), byte for byte.
	await page.locator('input[type=file]').setInputFiles({ name: 'crlf.txt', mimeType: 'text/plain', buffer: Buffer.from('un\r\ndeux\r\n') });
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(page.locator('.preview pre')).toContainText('deux');
	await page.getByRole('button', { name: 'Modifier', exact: true }).click();
	await page.locator('.cm-content').click();
	await page.keyboard.press('ControlOrMeta+Home');
	await page.keyboard.press('End');
	await page.keyboard.type(' 1');
	await page.getByRole('button', { name: 'Enregistrer…' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Confirmer' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'Enregistré.' })).toBeVisible();
	const raw = await page.evaluate(async () => {
		for (;;) {
			const r = await fetch('/api/containers/hublot-e2e/fs/file?path=%2Froot%2Fcrlf.txt');
			if (r.status !== 202) return r.text();
			await new Promise((ok) => setTimeout(ok, 200));
		}
	});
	expect(raw).toBe('un 1\r\ndeux\r\n');
	expect(errors.filter((e) => /Content Security Policy|Refused/.test(e))).toEqual([]);
});
```

Run: `cd .. && export PATH=/opt/homebrew/bin:$PATH && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock scripts/e2e.sh 2>&1 | tail -n 6 ; rm web/e2e/zz-ecriture.e2e.ts`
Expected: `2 passed` (le parcours unique et le jetable). Le jetable prouve, contre un vrai agent et sous la CSP : `readOnly` arrive par le contexte (le bouton « Modifier » existe), CodeMirror se charge, la boîte nomme le conteneur et le chemin, Annuler a le focus, l'écriture est relue après réindexation, l'envoi sélectionne le nouveau fichier, un nom existant est refusé, et un fichier CRLF modifié revient octet pour octet (`un 1\r\ndeux\r\n`).

- [ ] **Step 5: Essai manuel en lecture seule** — lancer l'agent de dev avec `--read-only` (`scripts/dev-agent.sh --read-only`), ouvrir un fichier texte : ni « Modifier », ni « Envoyer un fichier ici » ; le badge « Lecture seule » est dans la barre. À 390 px de large, la boîte de confirmation tient dans l'écran.

- [ ] **Step 6: Commit**

```bash
git status --short web/e2e   # rien : le parcours jetable est supprimé
git add web/src
git commit -F - <<'EOF'
web: édition CodeMirror, envoi, confirmation qui nomme conteneur et chemin, boutons cachés en lecture seule

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 8: Spec, vérification sur les VM et clôture

**Où tester :** `hk-ubuntu` (AppArmor), `hk-fedora` (SELinux), Docker Desktop.

**Files:**
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§5.2 étape 1, §5.3, §6, §7, §13), `docs/superpowers/plans/2026-10-09-explorateur-3-ecriture.md` (cases cochées, constats)

- [ ] **Step 1: Reporter les décisions dans la spec** — remplacer chaque passage par le suivant (chacun est unique dans la spec). Les textes ci-dessous suivent Q22 (a). Si l'utilisateur retient (b), les items 2, 8, 10 et 15 nomment en plus la liste de (b) et ses faux refus.

1. §5.2 étape 1 — remplacer :

   ```text
   1. Clé de cache : `containerID + State.FinishedAt`. Un redémarrage puis un arrêt changent la clé. Une écriture réussie sur le conteneur arrêté (§5.3) supprime son index du cache ; il est reconstruit au prochain accès.
   ```

   par :

   ```text
   1. Clé de cache : `containerID + State.FinishedAt`. Un redémarrage puis un arrêt changent la clé. Toute écriture sur le conteneur arrêté (§5.3), réussie ou non, supprime son index du cache ; il est reconstruit au prochain accès.
   ```

2. §5.3 étape 2, nom d'un envoi (Q19) — remplacer :

   ```text
   Le nom du fichier envoyé doit être un composant unique : ni `/`, ni `.`, ni `..`, ni vide, sinon `400 invalid_name`.
   ```

   par :

   ```text
   Le nom du fichier envoyé doit être un composant unique : ni `/`, ni `.`, ni `..`, ni vide, ni `\`, ni caractère de contrôle (octet nul compris), de format (bidi, largeur nulle) ou séparateur de ligne ou de paragraphe, ni plus de 255 octets, et en UTF-8 valide, sinon `400 invalid_name`. C'est le `filename` de la partie `file` du multipart tel que le décode `mime.ParseMediaType` (`filename*` prioritaire), jamais réduit à son nom de base : la valeur validée est celle qui part dans le tar.

      Avant d'ouvrir la vue, à partir de l'`Inspect` (refus `forbidden_path`, détail qui nomme la cause) : tout le conteneur si un de ses montages tombe sous la règle de §5.2 étape 7 et ses extensions (bind de `/`, `/proc`, `/sys` ou dessous, `/dev` ; conteneur actif compris) ; un bind d'un autre dossier de l'hôte reste inscriptible ; tout chemin où Docker n'écrit pas comme le conteneur le voit, car Docker extrait dans sa propre vue (rootfs et montages configurés), pas dans l'espace de noms de montage du conteneur : `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`, la `Destination` d'un montage, un chemin sous un tmpfs (`Mounts`, `HostConfig.Tmpfs`), sous `/dev`, `/proc` ou `/sys`. Pour une édition sur un conteneur actif, une cible qui est un point de montage (son `STATX_MNT_ID` diffère de celui de son parent) est refusée de même : elle couvre les montages qu'`Inspect` ne montre pas (configs Swarm, `--init`, CDI). Le verrou du conteneur est pris avant d'ouvrir la vue et rendu après l'étape 6 : deux écritures sur un même conteneur ne s'entrelacent jamais.
   ```

3. §5.3 étape 3, `If-Match` (Q14, Q18) — remplacer :

   ```text
   3. Relire le fichier par `fsview`, calculer son SHA-256, comparer à `If-Match` → `conflict` si différent.
   ```

   par :

   ```text
   3. Relire le fichier par `fsview` (au plus 2 Mio, sinon `too_large`), calculer son SHA-256, comparer à `If-Match`, qui doit être l'`ETag` de l'aperçu (`"<sha256>"`) → `conflict` si différent ou absent.
   ```

4. §5.3 étape 3, `If-None-Match` (Q18) — remplacer :

   ```text
   Pour un nouveau fichier (envoi), `If-None-Match: *` : le test
   ```

   par :

   ```text
   Pour un nouveau fichier (envoi), `If-None-Match: *` est exigé (sinon `conflict`) : le test
   ```

5. §5.3 étape 5 (Q17) — remplacer :

   ```text
   puis `PutArchive` sur le dossier parent **du chemin d'écriture validé à l'étape 2** : c'est exactement cette chaîne qui part vers Docker, jamais un chemin résolu ni recalculé.
   ```

   par :

   ```text
   puis `PutArchive` sur le dossier parent **du chemin d'écriture validé à l'étape 2** : c'est exactement cette chaîne qui part vers Docker, jamais un chemin résolu ni recalculé. Sans `copyUIDGID` (qui donnerait le fichier à l'utilisateur du conteneur), avec `noOverwriteDirNonDir`. Une fois l'intention journalisée, `PutArchive` ne dépend plus de la requête (contexte détaché, délai propre d'une minute) : Docker supprime le fichier existant avant de le recréer (mesuré : l'inode change), une écriture coupée le perdrait.
   ```

6. §5.3 étape 6 (Q20) — remplacer :

   ```text
   6. `audit` : écrire le résultat. Conteneur arrêté : en cas de succès, son index est retiré du cache (§5.2).
   ```

   par :

   ```text
   6. `audit` : écrire le résultat (`ok` ou `error`). Après tout `PutArchive`, réussi ou non, l'index du conteneur est retiré du cache (§5.2) : un échec peut avoir déjà supprimé le fichier.
   ```

7. §5.3, limite assumée et plafonds (Q14, Q17) — remplacer :

   ```text
   Notre contrôle garantit la cohérence (on écrit là où on a vérifié), pas l'atomicité. Pour un seul utilisateur, c'est acceptable.
   ```

   par :

   ```text
   Notre contrôle garantit la cohérence (on écrit là où on a vérifié), pas l'atomicité. Pour un seul utilisateur, c'est acceptable. L'écriture n'est pas atomique non plus : Docker remplace le fichier par un nouvel inode, un lien physique vers l'ancien est rompu, et un process qui le garde ouvert continue de lire l'ancien contenu. Une requête déjà terminée (la garde a répondu `504`) ne commence aucune écriture ; une écriture commencée continue après le `504`, et l'interface le dit.

   Plafonds : édition 2 Mio (l'aperçu s'arrête là, et avec lui l'`ETag`), envoi 32 Mio ; au-delà, `too_large`.
   ```

8. §5.3, filet de sécurité (Opus Minor 11) — remplacer :

   ```text
   Docker résout alors le chemin lui-même, dans la racine du conteneur : c'est le filet de sécurité.
   ```

   par :

   ```text
   Docker résout alors le chemin lui-même, dans la racine du conteneur : c'est le filet de sécurité. Il repose sur l'extraction chrootée de dockerd dans la racine du conteneur (correctif de CVE-2018-15664, présent dans toute API ≥ 1.44, §8). Un lien posé pendant la course peut mener Docker vers un autre montage du conteneur : aucun bind de `/`, `/proc`, `/sys` ni `/dev` n'y est accepté (étape 2), mais un bind d'un autre dossier de l'hôte reste atteignable (§13).
   ```

9. §6, définition de `forbidden_path` (Q21, Q22) — remplacer :

   ```text
   ou tar d'un dossier refusé par le contrôle des montages, §5.4)
   ```

   par :

   ```text
   ou tar d'un dossier refusé par le contrôle des montages, §5.4, ou écriture là où Docker n'écrit pas comme le conteneur le voit, ou dans un conteneur dont un montage expose l'hôte, §5.3 étape 2)
   ```

10. §7, binds de l'hôte (Q22) — remplacer :

   ```text
   La lecture et le listing ne sont pas refusés : l'utilisateur de l'interface est de fait root sur l'hôte (début de §7), et les pseudo-systèmes et les périphériques restent refusés.
   ```

   par :

   ```text
   La lecture et le listing ne sont pas refusés : l'utilisateur de l'interface est de fait root sur l'hôte (début de §7), et les pseudo-systèmes et les périphériques restent refusés. L'écriture, elle, est refusée pour tout le conteneur quand un montage tombe sous la règle de §5.2 étape 7 (bind de `/`, `/proc`, `/sys`, `/dev`…), comme l'explorateur d'un conteneur arrêté (§5.3 étape 2). Un bind d'un autre dossier de l'hôte (`/etc`, `/root`…) reste inscriptible, comme il est lisible : l'utilisateur est root de fait, et la course de §5.3 peut y mener Docker (§13).
   ```

11. §6, statuts (Q15, Q18) — remplacer :

   ```text
   Statuts HTTP : `not_found` 404 (`ELOOP` compris : boucle de liens, lien magique refusé), `forbidden_path` 403,
   ```

   par :

   ```text
   Statuts HTTP : `not_found` 404 (`ELOOP` compris : boucle de liens, lien magique refusé), `read_only` 403 (agent `--read-only`, ou refus de Docker : rootfs `--read-only`, volume `:ro`, avec son message en détail), `conflict` 409 (précondition, ou extraction refusée par Docker parce que le chemin a changé après les contrôles), `forbidden_path` 403,
   ```

12. §6, route `PUT /fs/file` — remplacer :

   ```text
   | PUT | `/api/containers/{id}/fs/file?path=` | F4 édition, `If-Match` obligatoire ; `link_in_path` (lien dans la chaîne, dernier composant compris), `not_regular`, `forbidden_path`, `indexing` |
   ```

   par :

   ```text
   | PUT | `/api/containers/{id}/fs/file?path=` | F4 édition, corps brut (au plus 2 Mio), `If-Match` obligatoire (l'`ETag` de l'aperçu, sinon `conflict`) → `200 {"etag"}` et en-tête `ETag` ; `link_in_path` (lien dans la chaîne, dernier composant compris), `not_regular`, `forbidden_path`, `read_only`, `too_large`, `indexing` |
   ```

13. §6, route `POST /fs/upload` — remplacer :

   ```text
   | POST | `/api/containers/{id}/fs/upload?dir=` | F4 envoi, multipart, `If-None-Match: *` ; `link_in_path` (lien dans la chaîne, ou nom existant qui est un lien, même pendant), `conflict`, `forbidden_path` (dossier dans un pseudo-système), `invalid_name`, `indexing` |
   ```

   par :

   ```text
   | POST | `/api/containers/{id}/fs/upload?dir=` | F4 envoi, multipart (partie `file`, au plus 32 Mio), `If-None-Match: *` obligatoire → `201 {"path", "etag"}` ; `link_in_path` (lien dans la chaîne, ou nom existant qui est un lien, même pendant), `conflict`, `forbidden_path` (dossier dans un pseudo-système), `invalid_name`, `read_only`, `too_large`, `indexing` |
   ```

14. §7, journal d'audit — remplacer :

   ```text
   Écrit avant et après chaque écriture.
   ```

   par :

   ```text
   Écrit avant (`result` : `intent`) et après (`ok` ou `error`) chaque écriture ; si l'intention ne s'écrit pas, rien n'est écrit dans le conteneur.
   ```

15. §13, nouveau risque (ligne ajoutée après celle des noms non UTF-8) — remplacer :

   ```text
   | Noms de fichiers non UTF-8 : JSON les transporte avec U+FFFD, le fichier ne s'ouvre plus depuis l'interface (`not_found`) | limite v0.1 ; le listing ne casse pas |
   ```

   par :

   ```text
   | Noms de fichiers non UTF-8 : JSON les transporte avec U+FFFD, le fichier ne s'ouvre plus depuis l'interface (`not_found`) | limite v0.1 ; le listing ne casse pas |
   | Écriture non atomique : Docker supprime le fichier puis le recrée ; une écriture coupée le perdrait | `PutArchive` détaché de la requête une fois l'intention journalisée ; journal avant et après. Au-delà du délai d'une minute, l'écriture est coupée : fichier perdu ou tronqué, limite assumée pour la v0.1 (§5.3) |
   | La garde répond `504` pendant qu'une écriture continue | l'interface dit que l'écriture a peut-être eu lieu et demande de recharger avant de réessayer ; une requête déjà terminée ne commence rien (§5.3) |
   | Montages qu'`Inspect` ne montre pas (configs et secrets Swarm, `--init`, CDI ou `--gpus`), ou dont la `Destination` passe par un lien du conteneur : Docker répond `device or resource busy` (500 → `docker_unavailable`) ou écrit où le conteneur ne voit rien | édition sur un conteneur actif : refusée par le point de montage (`STATX_MNT_ID`, §5.3 étape 2) ; envoi (la cible n'existe pas encore) et conteneur arrêté : limite assumée pour la v0.1 |
   | Course de §5.3 : un lien posé entre les contrôles et `PutArchive` mène Docker vers un autre montage du conteneur, où root écrit avec le propriétaire du tar ; un bind d'un dossier de l'hôte autre que `/`, `/proc`, `/sys`, `/dev` (ex. `-v /etc:/hostetc`) est atteignable | binds de `/`, `/proc`, `/sys`, `/dev` refusés pour toute écriture (§5.3 étape 2) ; un montage `:ro` reste refusé par Docker ; le contenu écrit est celui de l'utilisateur ; limite assumée pour la v0.1 (Q22 du plan Explorateur 3) |
   | Écriture bloquée sur un FUSE muet (contrôles ou relecture) : elle garde le verrou du conteneur jusqu'au redémarrage de l'agent ; chaque écriture suivante reçoit `504` après 10 s, et l'interface dit à tort « l'écriture a peut-être eu lieu » | limite assumée pour la v0.1 ; les lectures ne prennent pas ce verrou |
   | userns-remap (hors v0.1, §2) : `procfs` donne les uid/gid de l'hôte, le tar attend ceux du conteneur | non pris en charge en v0.1 |
   ```

- [ ] **Step 2: Suite complète sur les deux VM**

Run (pour chaque VM) : `limactl start hk-ubuntu >/dev/null && limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 | grep -E '^(ok|FAIL|---)|MESURE' | tail -n 30`, puis idem `hk-fedora`.
Expected: `ok` pour chaque paquet ; `TestIntegrationWriteRunning`, `TestProcLinksRefused` et `TestProcPseudoFSRefused` passent, sans SKIP (vérifier par `-run 'IntegrationWrite|Proc' -v` si besoin) ; les lignes `MESURE` (inode, refus de Docker) sont relevées. Sur `hk-fedora`, aucun AVC (`sudo ausearch -m avc -ts recent`, vide).

- [ ] **Step 3: Revue de branche** — demander la revue complète de la branche (sous-agent indépendant + Codex), corriger, puis cocher les cases de ce plan et noter les constats sous « Constats de la tâche 8 » : résultat des deux VM, lignes `MESURE`, AVC éventuels, verdict de la revue.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers
git commit -F - <<'EOF'
docs: Explorateur 3/3 vérifié sur hk-ubuntu et hk-fedora, spec de l'écriture complétée

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

#### Constats de la rédaction

- Copie isolée (`git worktree`, `d6bb68e`) : tout le code Go des tâches 1 à 5 compile et passe, en macOS et sous Linux root (`scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...`, Docker Desktop 29.7.2), sans SKIP. Les VM n'ont pas été lancées.
- Docker Desktop : `PUT /archive` sur un rootfs `--read-only` → `InvalidArgument` « container rootfs is marked read-only » ; sur un volume `:ro` → `InvalidArgument` « mounted volume is marked read-only » ; dans un dossier absent → `NotFound` ; sur un chemin qui est un fichier → `InvalidArgument` « extraction point is not a directory » (inatteignable après les contrôles de `files`, sauf course).
- Docker remplace le fichier par un nouvel inode (570993 → 320851) : un lien physique est rompu, un process qui garde le fichier ouvert continue de lire l'ancien contenu (Q17).
- `docker cp` vers un conteneur en pause réussit ; l'interface traite déjà un conteneur en pause comme actif (`procfs`).
- `openat2 RESOLVE_NO_SYMLINKS` refuse un lien pendant en dernier composant (`ELOOP` → `link_in_path`) : `TestProcLinksRefused`, envoi sur `/d/dangling`.
- Revue 1 : sur un conteneur actif, `docker cp` vers `/dev`, `/dev/shm` ou un `--tmpfs` réussit et le conteneur ne voit pas le fichier ; vers `/run` (pas un tmpfs), il le voit. Un conteneur jamais démarré a `HostsPath` vide et accepte `/etc/hosts`, que Docker masquera au démarrage. `HostConfig.Tmpfs` porte les `--tmpfs`, absents de `Mounts`.
- Revue 2 : `PUT /archive` vers un fichier existant → `InvalidArgument` « extraction point is not a directory » (désormais `ErrPutInvalid`, `conflict`). `--init` monte `/sbin/docker-init` (601 296 octets sur Docker Desktop) sans entrée dans `Inspect.Mounts`. `/etc/os-release` existe sur l'hôte du démon de Docker Desktop.
- Build de l'interface : CodeMirror et ses langages vivent dans des morceaux atteints par `import()` seulement ; le nœud de la page conteneur pèse 22 Ko, le plus gros morceau CodeMirror 263 Ko (non compressé).

#### Constats de la tâche 8

À remplir à l'étape 3 de la tâche 8 : résultat des deux VM, lignes `MESURE`, AVC éventuels, verdict de la revue de branche.
