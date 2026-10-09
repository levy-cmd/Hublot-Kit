# Explorateur 1/3 — Conteneur actif (F2, F3) — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (règle du dépôt, `CLAUDE.md`) :** chaque tâche est relue par un sous-agent indépendant **et** par Codex avant la suivante. Toute tâche qui touche `internal/fsview` est relue en priorité sur l'évasion (spec §13, « revue de toute PR qui touche `fsview` »).
>
> **Apprendre Go :** les tâches marquées 🎓 (1 et 2) n'utilisent que la bibliothèque standard, sans Linux ni Docker. Elles se codent bien à la main en suivant le plan.
>
> **Vérifié avant rédaction (après deux tours de revue) :** le code Go de ce plan a été compilé et testé sur une copie du dépôt, sur macOS et sous Linux dans un conteneur `golang`, en root comme en non-root, avec `-race` et avec `-tags integration` sur Docker Desktop. Les états intermédiaires ont été rejoués : après la tâche 4, 20 tests rouges sur 21 ; les tâches 5 et 6 compilent et passent. Les nouveaux tests ont été éprouvés par mutation (premier tour) : un `Reopen` placé avant le contrôle du type, l'absence de `pidfd_send_signal` dans `openRoot` ou un créneau rendu avant la copie les font échouer. Au second tour, d'autres mutations sont détectées : la perte de `mounts: info.Mounts` dans `openProcFS`, la perte de la règle « tout le conteneur », une erreur de `VolumeOptions` renvoyée telle quelle, un pilote de volume tiers accepté, l'option `type=` ignorée, un échappement de `mountinfo` invalide accepté. La mutation « `pidfd_send_signal` retiré de `CheckArchive` » n'a pas pu être rejouée : Docker Desktop a cessé de répondre (disque de l'hôte plein) ; à rejouer avant l'exécution de la tâche 7. Le troisième tour (petites retouches) n'a été vérifié que de façon portable, Docker ne répondant toujours pas : tests Go sur macOS avec `-race`, `GOOS=linux go vet` (avec et sans `-tags integration`), compilation des binaires de test Linux (`go test -c`, arm64 et amd64), construction du binaire Linux, et pour l'interface `check`, `eslint`, Vitest (33 tests) et `build`. Deux mutations portables le confirment : un pilote tiers accepté, et un `device` relatif accepté, font échouer `TestHostBind`. Le code de l'interface a passé `npm run check`, `eslint`, Vitest et `npm run build`. Il a aussi été parcouru dans Chrome sans tête, contre l'agent lancé par `scripts/dev-agent.sh` : liste, lien vers un fichier, `/proc`, téléchargement de `/` refusé avec son message, `/etc` accepté, `link_in_path` sur un dossier atteint par un lien, aucune erreur console. Le scénario C1 (bind du `/proc` de l'hôte) n'a **pas** été rejoué contre un vrai dockerd : il ne se teste qu'avec le faux client. Les VM Lima n'ont pas été lancées : la tâche 16 le fait.

**Goal:** Sur un conteneur qui tourne, l'utilisateur parcourt toute l'arborescence. Il voit la taille, les droits et le propriétaire de chaque entrée, prévisualise un fichier texte ou une image, et télécharge un fichier ou un dossier (tar). Aucune lecture, ni de l'agent ni de Docker, ne peut sortir de la racine du conteneur vers l'hôte.

**Architecture:** Nouveau paquet `internal/fsview`. Il porte l'interface `FS` (spec §4) et sa première implémentation `procfs` (spec §5.1) :
- pidfd + ancrage cgroup + `/proc/<pid>/root` en `O_PATH` ;
- `pathrs-lite` pour toute résolution ;
- `fstatfs` contre les pseudo-systèmes ;
- fichiers réguliers seulement ;
- `openat2 RESOLVE_NO_SYMLINKS` pour les chemins sans lien ;
- contrôle des montages avant tout tar de dossier (binds de l'hôte, `mountinfo`).

`dockerx` gagne `Inspect` (avec les montages), `GetArchive` et `VolumeOptions`. `api` expose trois routes GET (§6) derrière une garde par conteneur (délai et plafond, §13). L'interface SvelteKit gagne la page `/c/[id]`, onglet Fichiers.

**Tech Stack:** Go 1.27 (stdlib `net/http`), `github.com/moby/moby/client` v0.6.1, `github.com/cyphar/filepath-securejoin` v0.7.0 (`pathrs-lite`), `golang.org/x/sys` v0.48.0 ; SvelteKit 3 / Svelte 5, Vitest. `golang.org/x/sys` passe de v0.33.0 (indirecte, tirée par le SDK Docker) à v0.48.0 (directe) à la tâche 4, étape 2 (`go get golang.org/x/sys@v0.48.0`) ; la tâche 3 ne fait passer en directe que `github.com/containerd/errdefs`.

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§4, §5.1, §5.2 étape 7, §5.4, §6, §7, §8, §9, §10, §13). Résultats du spike : `docs/superpowers/spikes/2026-10-07-resultats-spike.md`. Le code `procroot` du spike (parent du commit `72e1b50`) sert de base à `procfs`.

## Décisions de revue

Revue du plan par un relecteur Opus et par Codex, constats vérifiés. Une ligne par décision, avec la tâche qui l'applique.

- **C1 (critique) :** télécharger `/host` d'un conteneur lancé avec `-v /proc:/host/proc` archive le `/proc` de l'hôte (`/proc/kcore`, 254 Tio, dockerd bloqué). Avant tout `GetArchive` de dossier, `CheckArchive` applique :
  - (a) la règle des binds de §5.2 étape 7 sur `Inspect.Mounts`, volumes `local` en `o=bind|rbind` + `device=` compris ;
  - (b) `/proc/<pid>/mountinfo`, lu pour le process épinglé par le pidfd : aucun pseudo-système ni FUSE sur, sous ou au-dessus du dossier.

  Échec → `forbidden_path` qui nomme le bind ou le montage. Les tests passent par le faux client et vérifient qu'aucun `GetArchive` n'est appelé. → Tâches 3 (`Mounts`, `VolumeOptions`), 7, 12 ; spec §5.4 et §13 (tâche 12).
- **Bind d'un dossier ordinaire de l'hôte (conteneur actif) :** pas de refus en lecture ni au listing. L'utilisateur est root de fait, et les pseudo-systèmes et périphériques restent refusés. Une phrase le dit dans la spec §7. → Tâche 12.
- **I1 :** test `inotify` `IN_OPEN` sur la FIFO et le nœud de périphérique ; zéro événement exigé (le refus précède `Reopen`). → Tâche 4.
- **I2 :** cible `--pid=host` : `Open`, `List` et `StatNoLinks` sur `/proc/1/root/…` et par un lien `/m -> /proc/1/root` → erreur, jamais de contenu. Écrit d'abord en test unitaire avec `/` comme racine (tâche 4), confirmé sur Docker (tâche 8).
- **I3 (= Codex n°5) :** `Inspect` d'abord, borné à 10 s ; la garde FUSE compte par ID complet. → Tâche 10.
- **I4 :** points d'injection `readCgroup` et `afterRootOpen` ; un test tue le process entre l'ouverture de sa racine et `pidfd_send_signal`, et attend `ErrContainerChanged`. → Tâche 6.
- **Codex n°4 :** le créneau de la garde reste pris pendant toute la copie (fichier et dossier), jusqu'au vrai retour de la lecture. Un test lance plusieurs téléchargements sur un lecteur bloqué. → Tâches 9, 12.
- **M1 :** « mauvaise erreur » est signalée à part d'« ÉVASION ». → Tâche 4.
- **M2 :** mode `&check=1`, qui fait tous les contrôles et répond `204` sans `GetArchive` ; l'interface l'utilise avant de télécharger. → Tâches 12, 13.
- **M3 :** la vue est fermée dès l'ouverture d'un fichier. → Tâche 12.
- **M4 :** `ELOOP` (boucle, lien magique) → `not_found`. → Tâche 9.
- **M5 :** la spec §8 cite les scripts au lieu de `docker compose`. → Tâche 15.
- **M6 :** la limite Q6 entre dans la spec §13. → Tâche 12.
- **M7 :** l'ancienne tâche 13 est scindée en tâche 14 (page) et tâche 15 (agent de dev, essai manuel, README).
- **M8 :** ne pas pousser entre les tâches 4 et 5. → Tâche 4.
- **Q1 à Q7 :** recommandations conservées (le relecteur est d'accord), confirmées par l'utilisateur le 2026-10-09.

Second tour (relecteur Opus et Codex sur la version `af9a207`, constats vérifiés) :

- **R1 (câblage de C1) :** trois tests le couvrent.
  - (i) `TestForWiresMountsIntoCheckArchive` : `For` avec un faux Docker, dont le « conteneur » est un `sleep` du test, plus l'injection `readCgroup`. `CheckArchive("/etc")` doit refuser en nommant le bind. → Tâche 7.
  - (ii) `TestCheckArchiveDeadProcess` passe par l'injection `readMountinfo` : il lit le vrai contenu, tue et récolte l'enfant, rend le contenu, puis exige `ErrContainerChanged` et que le pid lu soit `p.pid`. → Tâche 7.
  - (iii) `TestDownloadMountCheckThroughTheRealView` passe par la route, à travers `fsRequest` et l'`OpenFS` réel, grâce à `fsview.SetCgroupReaderForTests`. Il vérifie le statut, le code et zéro `GetArchive`. → Tâche 12.
- **R2 :** quatre volets.
  - (1) Règle « tout le conteneur » (`PseudoMountInMounts`) : un pseudo-système ou un FUSE monté sur ou sous la destination d'un montage d'`Inspect.Mounts` refuse tout tar. La règle par dossier reste.
  - (2) `HostBind` refuse aussi un volume `local` dont l'option `type` est un pseudo-système ou FUSE, et tout volume d'un autre pilote (inconnu : refus).
  - (3) Spec §5.2 étape 7 mise à jour. → Tâche 7. *Revu au troisième tour (T1) : la phrase va au §5.4 seulement, voir Q9.*
  - (4) La note sur les sous-montages d'un `rbind` est nuancée (propagation `rprivate`), ainsi que la limite §13. → Tâches 7, 12.
- **R3 (course entre `CheckArchive` et `GetArchive`) :** limite assumée, sans mécanisme en plus, écrite dans la spec §5.4 et §13. Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1 (formulation du troisième tour). dockerd archive à partir des montages configurés (mesuré) ; et R2 (1) couvre tout le conteneur. À remesurer sur `hk-ubuntu`. → Tâches 12, 16.
- **R4 :** tout échec de `VolumeOptions` donne `ErrForbiddenPath`, avec le nom du volume et sa destination dans le détail. Le test qui acceptait `ErrNotFound` est corrigé, et un test de route vérifie statut, code et zéro `GetArchive`. → Tâches 7, 12.
- **R5 :** cas `mountinfo` explicites : deux montages empilés au même point, champs de propagation (`shared:`, `master:`, `propagate_from:`), échappements `\011`, `\012`, `\134`. Un échappement invalide fait refuser (échec fermé). → Tâche 7.
- **R6 :** six points.
  - Tâche 8 : attendre que `List("/")` montre `m` au lieu d'un `sleep` ; la mesure de Q8 ne conclut « ne voit pas » que sur `dockerx.ErrNotFound`, sinon elle dit « non concluante ».
  - Tâche 4 : un lien vers `/proc/self/root`, efficace sans root (CI).
  - Tâche 16 : `/` et `/sys` aussi par `&check=1`.
  - Tâche 10 : la borne de 10 s d'`Inspect` est notée « non testée ».
  - Tâche 12 : la puce §7 devient « Binds de l'hôte (conteneur actif) » et rappelle que le tar y est refusé.
- **Q8 :** le refus strict reste en v0.1 (recommandation retenue, à confirmer). **Portée de (a) :** tout dossier du conteneur (retenue).

Troisième tour (relecture finale « Prêt avec réserves » et deux retouches Codex) :

- **T1 :** les extensions de `HostBind` (pilote tiers, options illisibles, `type=`) ne touchent pas encore la spec §5.2 : elles changeraient la portée pour les conteneurs arrêtés, dont tout l'explorateur deviendrait indisponible. La phrase de spec est cantonnée au tar d'un conteneur actif (§5.4) ; la question devient **Q9**. → Tâches 7, 12.
- **T2 :** `TestHostBind` donne à `ceph` et `anon` des options lisibles : seule la règle du pilote peut les refuser. → Tâche 7.
- **T3 :** `hostSensitive` refuse toute source (ou `device`) non absolue (échec fermé), avec test. → Tâche 7.
- **T4 :** le message de l'interface dit « téléchargez un fichier, ou un sous-dossier si le refus ne porte que sur ce dossier », et affiche le détail qui nomme le montage (`downloadErrorMessage`, testé). → Tâches 13, 14.
- **T5 :** spec §5.4 puce (a) et §7 : « qui nomme le montage ; volumes `type=` pseudo/FUSE, pilotes tiers, options illisibles ». → Tâche 12.
- **T6 :** formulation de R3 (§5.4, §13, note de la tâche 7) : « Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1 ». Tâche 16 : « confirmer R3 » veut dire relever la ligne `MESURE`. → Tâches 7, 12, 16.
- **T7 :** tâche 12 : « Où tester » cite le test Linux ; le paragraphe « Toutes les erreurs… `&check=1` » est de nouveau séparé. → Tâche 12.
- **T8 :** tâche 4 : `/etc/passwd` au lieu de `/etc/hostname` partout dans `procfs_test.go` (présent sur toute machine de test). → Tâche 4.
- **T9 :** `fsview.SetCgroupReaderForTests` garde sa forme, avec `if !testing.Testing() { panic(…) }` ; les tests qui l'utilisent n'appellent pas `t.Parallel` (noté dans le code et à la tâche 12). → Tâches 6, 12.
- **T10 :** l'en-tête précise que `golang.org/x/sys` passe de v0.33.0 à v0.48.0, et à quelle tâche : la tâche 4 (étape 2), pas la tâche 3, qui ne fait passer en directe que `containerd/errdefs`.

## Découpage de la phase Explorateur (semaines 5 à 9)

La phase couvre quatre fonctions indépendantes à tester : lecture d'un conteneur actif, lecture d'un conteneur arrêté, diff, écriture. Elle est découpée en trois plans ; chacun livre un logiciel qui marche et se teste seul.

| Plan | Fichier | Durée visée |
|---|---|---|
| **1 (ce plan)** | `2026-10-08-explorateur-1-procfs.md` | 2 semaines |
| 2 | `2026-10-08-explorateur-2-tarindex-diff.md` (à écrire) | 1,5 à 2 semaines |
| 3 | `2026-10-08-explorateur-3-ecriture.md` (à écrire) | 1,5 semaine |

**Plan 1 (ce plan) :**
- `fsview` (interface, `procfs`, contrôle des montages) ;
- `dockerx.Inspect` (avec `Mounts`), `GetArchive`, `VolumeOptions` ;
- routes `GET /fs`, `/fs/file`, `/fs/download` sur conteneur actif ;
- garde FUSE ;
- onglet Fichiers en lecture.

**Plan 2 (tarindex + F5) :**
- `tarindex` (§5.2), tests d'évasion et de résolution écrits d'abord, sur des tar en mémoire : liens relatifs et absolus, `..` bloqué, `..` après un dossier lié, chaîne, boucle à 40 sauts, liens physiques ;
- binds de l'hôte (§5.2 étape 7) : réutiliser `fsview.HostBind` de ce plan, tests d'abord ; la portée de ses extensions pour un conteneur arrêté est à trancher (Q9) ;
- cache LRU 3 avec la clé `ID + FinishedAt`, construction unique, progression plafonnée à 99 %, plafonds 4 Gio / 5 min → `too_large` ;
- `202 indexing` et sa barre de progression. Attention : `api()` côté interface traite tout `res.ok` comme un succès, un `202` doit y être reconnu ;
- `StatNoLinks` et `CheckArchive` par l'index ; `fsview.For` bascule sur `tarindex`, ce qui retire `ErrStopped` ;
- F5 : `dockerx.Changes`, `GET /changes`, onglet Changements, marques du diff dans l'explorateur ;
- le parcours Playwright unique (§10) et la matrice CI overlay2 / containerd.

**Plan 3 (écriture F4, §5.3) :**
- paquet `files`, tests écrits d'abord : lecture seule, `If-Match`, ordre audit → écriture → audit, `link_in_path` final et parent, `not_regular`, `forbidden_path`, `invalid_name`, lien pendant, chemin exact envoyé à `PutArchive`, index retiré du cache ;
- `dockerx.PutArchive` ; `PUT /fs/file`, `POST /fs/upload` ;
- CodeMirror 6 (langages chargés à la demande), boîte de confirmation qui rappelle conteneur et chemin, boutons cachés en lecture seule ;
- à trancher : plafond de taille d'un envoi et d'une édition (la spec n'en fixe pas).

L'ordre 1 → 2 → 3 diffère de l'ordre « procfs, écriture, tarindex » envisagé. L'écriture sur un conteneur arrêté (§5.3 étape 2) passe par l'index : l'écrire avant `tarindex` obligerait à livrer F4 à moitié puis à y revenir (voir Q3).

## Questions ouvertes

Le plan suit la recommandation de chaque question. Pour Q1 à Q7, la recommandation est retenue et confirmée par l'utilisateur (2026-10-09) ; une réponse différente change la tâche indiquée.

- **Q1 — « Arborescence complète » (F2, §9).** Arbre dépliable, ou un dossier à la fois avec fil d'Ariane ?
  - *Recommandation (retenue, à confirmer) :* un dossier à la fois, avec fil d'Ariane, lien « dossier parent » et l'état dans l'URL (`?path=…&file=…`). Ça marche au téléphone (§9), avec le retour arrière du navigateur et des liens partageables. Un arbre dépliable reste possible plus tard sans changer l'API. → Tâche 14.
- **Q2 — Interface `fsview.FS` (§4).** La spec liste `List`, `Stat`, `Open`. Il faut aussi :
  - (a) libérer les descripteurs de la racine et du pidfd ;
  - (b) le contrôle « aucun lien sur toute la chaîne » de §5.3 étape 2 et §5.4 ;
  - (c) le contrôle des montages avant un tar (C1).

  *Recommandation (retenue, à confirmer) :* ajouter `StatNoLinks(ctx, path) (Entry, error)`, `CheckArchive(ctx, dir) error` et `Close() error` à l'interface, et les reporter dans la spec §4. → Tâche 1, étape 5.
- **Q3 — Ordre des plans 2 et 3.** Voir ci-dessus.
  - *Recommandation (retenue, à confirmer) :* `tarindex` + F5 avant l'écriture.
- **Q4 — Conteneur arrêté pendant le plan 1.** `tarindex` n'arrive qu'au plan 2.
  - *Recommandation (retenue, à confirmer) :* `fsview.ErrStopped` temporaire → `501` code `internal`, car aucun code de §6 ne dit « pas encore ».
  - L'interface n'affiche pas l'explorateur d'un conteneur arrêté et annonce qu'il arrive à l'étape suivante.
  - Le plan 2 retire `ErrStopped`, la clé i18n `container.stoppedSoon` et le cas `501`. → Tâches 1, 9, 14.
- **Q5 — Aperçu d'un fichier binaire (§6, « pas d'aperçu texte »).** La spec ne dit pas ce que répond la route.
  - *Recommandation (retenue, à confirmer) :* `200`, `Content-Type: application/octet-stream`, corps vide, avec l'`ETag`. L'interface affiche « fichier binaire » et propose le téléchargement. → Tâche 11.
- **Q6 — Noms de fichiers non UTF-8.** JSON ne transporte que de l'UTF-8 : Go remplace les octets invalides par U+FFFD. Le nom affiché ne permet donc plus d'ouvrir le fichier (`not_found`).
  - *Recommandation (retenue, à confirmer) :* limite assumée en v0.1, reportée dans la spec §13 à la tâche 12 (M6). Le listing ne casse pas (tâche 5), et les lignes du tableau sont indexées par position (tâche 14).
- **Q7 — cgroup v1 (plusieurs lignes dans `/proc/<pid>/cgroup`).** §5.1 étape 3 parle « du chemin ».
  - *Recommandation (retenue, à confirmer, codée à la tâche 2) :* chaque ligne est ancrée séparément.
  - Une ligne sans composant de forme ID Docker (contrôleur v1 resté à `/`) ne prouve rien et est ignorée.
  - Un `..`, une ligne mal formée ou un ancrage sur un autre ID refusent tout. Au moins une ligne doit correspondre.
  - En cgroup v2 (une seule ligne `0::`), c'est exactement la règle de la spec.
- **Q8 (nouvelle, issue de C1) — Le `/` d'un conteneur actif.** La règle (b) de C1 refuse le tar de `/`, `/proc` et `/sys` d'un conteneur actif, parce que son propre `/proc` est monté dessous. Or une mesure sûre (tâche 8, `TestIntegrationMeasureOwnProcInArchive`, Docker Desktop, Docker 29) montre que le tar d'un conteneur actif **ne voit pas** son propre `/proc` : seuls les binds et volumes y sont montés. Le refus de `/` est donc plus strict que nécessaire.
  - On ne peut pas l'assouplir simplement : dans `mountinfo`, le `/proc` du conteneur et un bind du `/proc` de l'hôte ont le même type.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-09) :* garder la règle stricte en v0.1. L'interface explique de télécharger un sous-dossier. Mesurer de nouveau sur `hk-ubuntu` (tâche 16) avant tout assouplissement, par exemple ne compter que les montages situés sous une destination d'`Inspect.Mounts` (règle R2 (1), déjà en place).
  - Portée de la règle (a) (binds) : tout dossier du conteneur, comme pour l'explorateur d'un conteneur arrêté (retenue).

- **Q9 (troisième tour) — Extensions de `HostBind` pour un conteneur arrêté.** Pour le tar d'un dossier d'un conteneur actif (§5.4), `HostBind` refuse aussi :
  - les volumes `local` dont `type=` est un pseudo-système ou FUSE ;
  - les volumes d'un pilote tiers ;
  - les volumes dont les options ne se lisent pas.

  Pour un conteneur **arrêté**, la règle de §5.2 étape 7 rend l'explorateur **entier** indisponible (`List`, `Stat`, `Open`, téléchargement). Y appliquer ces extensions couperait l'exploration de tout conteneur arrêté qui a un volume de pilote tiers (plugin NFS, rexray, s3fs…), même inoffensif. La spec §5.2 n'est donc pas modifiée par ce plan.
  - *Recommandation (confirmée par l'utilisateur le 2026-10-09, à appliquer au plan 2) :* les appliquer aussi aux conteneurs arrêtés. L'index se construit par `GET /archive` de `/`, qui traverse tous les volumes. Un volume FUSE ou de pilote tiers qui ne répond pas y bloquerait dockerd exactement comme le tar d'un dossier, et nos plafonds (4 Gio, 5 min) n'arrêtent pas un dockerd bloqué.
  - Le message nomme le volume. Pour explorer ce conteneur, il reste le conteneur actif : listing et lecture par `procfs`, sans tar.

## Décisions de ce plan (la spec ne tranche pas)

- **Statuts HTTP.** La spec fixe `202 indexing`, `400 invalid_name` et `429`. Le plan ajoute :

  | Code | Statut |
  |---|---|
  | `not_found` | 404 |
  | `forbidden_path` | 403 |
  | `container_changed` | 409 |
  | `not_regular`, `link_in_path` | 422 |
  | `too_large` | 413 |
  | `docker_unavailable` | 503 |
  | `internal` | 500 ; garde FUSE : 503 si le plafond est atteint, 504 si le délai est dépassé ; `ErrStopped` : 501 (temporaire, Q4) |

  Reporté dans la spec §6 à la tâche 9.
- **Forme JSON de `GET /fs`.**
  - Réponse : `{path, type, entries, truncated}`.
  - `type` vaut `"dir"` pour un dossier. Pour un autre chemin (lien vers un fichier, lien profond), `type` donne la nature du chemin résolu et `entries` est vide : l'interface en affiche l'aperçu.
  - Une entrée : `{name, type, size, mode, uid, gid, modTime, linkTarget?}`. `mode` porte les bits Unix `0o7777` (permissions + setuid/setgid/sticky) ; l'interface les met en forme comme `ls`.
- **Chemins et identifiants.**
  - Toute route nettoie `?path=` par `path.Clean("/" + p)` (`fsview.CleanPath`) avant usage ; un octet nul → `404 not_found`.
  - `{id}` doit correspondre à `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`, car il part dans les URL de l'API Docker.
  - `Inspect` résout ensuite l'identifiant, borné à 10 s. Seul l'ID complet sert après : garde, `OpenFS`, `GetArchive`.
- **Navigation sur un lien (§5.1 étape 8).**
  - L'interface calcule la cible comme texte : absolue telle quelle, relative depuis le dossier du lien, `..` résolu lexicalement. L'agent la résout ensuite par `pathrs`.
  - Comme on navigue toujours vers les cibles, le dossier courant ne contient en pratique pas de lien, et le résultat lexical coïncide avec celui du noyau.
  - Sinon, on tombe sur un autre dossier **du conteneur**, jamais hors de la racine.
- **Aperçu.** « 2 Mo » = 2 Mio (2 097 152 octets) ; au-delà, `413 too_large`. L'`ETag` vaut `"<sha256 hex>"` (forme HTTP, entre guillemets).
- **Garde FUSE (§13).**
  - Au plus 4 opérations de fichiers simultanées par conteneur (ID complet), abandon après 10 s.
  - Un créneau n'est libéré qu'au vrai retour du travail : une goroutine abandonnée le garde jusqu'à son retour, un téléchargement jusqu'à la fin de sa copie (Codex n°4).
  - Un fichier FUSE bloqué en pleine copie bloque donc cette requête et un créneau, jamais plus de 4 par conteneur.
- **Téléchargement.**
  - `Content-Disposition` est construit par `mime.FormatMediaType` (RFC 2231 pour les noms non ASCII ou à caractères de contrôle). Dossier → `<nom>.tar`.
  - Toutes les erreurs arrivent en JSON avant le premier octet. `&check=1` fait tous les contrôles et répond `204` sans rien lire.
  - Dossier d'un conteneur actif : `StatNoLinks`, puis `CheckArchive` (C1), puis `GetArchive` de l'ID complet et du chemin nettoyé.
- **Développement sur macOS.** Deux scripts remplacent le `docker compose` du §8 (spec mise à jour à la tâche 15) : un seul fichier chacun, pas de service à maintenir.
  - `scripts/test-linux.sh` : tests Go en root dans un conteneur `golang`, avec les droits de l'agent recommandé ;
  - `scripts/dev-agent.sh` : l'agent dans un conteneur.

## Global Constraints

- **Module et dépendances Go.**
  - Module : `github.com/levy-cmd/hublot-kit`.
  - Dépendances autorisées (§3) : `github.com/moby/moby/client` et ses dépendances (dont `github.com/containerd/errdefs`, qui passe en directe) ; `github.com/cyphar/filepath-securejoin` **v0.7.0** (sous-paquet `pathrs-lite`, paquet `pathrs`) ; `golang.org/x/sys` **v0.48.0**. Aucune autre.
- **Accès aux fichiers d'un conteneur (CLAUDE.md, §5).**
  - Permis : `pathrs.OpenatInRoot` / `pathrs.Reopen` sur le descripteur de la racine ; `openat2` avec `RESOLVE_IN_ROOT` ; `fstatat`/`readlinkat` relatifs à un descripteur de dossier, avec un seul composant.
  - **Jamais** `filepath.Join`, `os.Open`, `os.Stat`, `os.ReadFile` sur un chemin du conteneur. Les fichiers `/proc/<pid>/cgroup` et `/proc/<pid>/mountinfo` sont des fichiers de l'hôte, lus pour le process épinglé par le pidfd, qui doit être vivant après la lecture.
  - Le paquet `path` ne sert qu'à manipuler du texte (nettoyer, nom de base). Les tests construisent leurs pièges avec des chemins de l'hôte : c'est permis, le code testé ne les voit jamais.
- **Tests d'évasion avant le code qu'ils protègent (CLAUDE.md)** : voir le tableau « Ordre » ci-dessous.
- **Code Linux.** Il vit dans des fichiers `_linux.go` (`//go:build linux`), avec un repli `_other.go` (`//go:build !linux`) : `go test ./cmd/... ./internal/...` compile et passe sur macOS.
- **Erreurs API :** `{"error": "<code>", "detail": "<texte>"}`, codes de §6 uniquement.
- **Limites des routes de fichiers.**
  - Listing plafonné à 10 000 entrées (`truncated`).
  - Aperçu : 2 Mio. Binaire = octet nul dans les 8 192 premiers octets. Images : PNG, JPEG, GIF, WebP seulement ; SVG, HTML et XML toujours en `text/plain; charset=utf-8`.
  - Téléchargement : `Content-Disposition: attachment` + `application/octet-stream`.
- **Interface.**
  - `{@html}` interdit.
  - Textes dans `web/src/lib/i18n/fr.ts` et `en.ts`, avec les mêmes clés.
  - Imports `#lib/….ts` dans les `.svelte` (SvelteKit 3, pas de `$lib`).
  - `goto(url, { replace, reset })` : SvelteKit 3 n'a plus `keepFocus`/`noScroll`.
- **Node ≥ 22.17.** Sur le Mac, préfixer les commandes `npm`/`npx` par `export PATH=/opt/homebrew/bin:$PATH` (le Node par défaut du shell est trop ancien).
- **Docker Desktop depuis le Mac :** `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock`.
- **Commits** en français, terminés par une ligne vide puis `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Noms de fichiers inhabituels** (espace, retour à la ligne, emoji, `"`, `;`, octet non UTF-8) : listés sans erreur, téléchargés sous leur vrai nom. → Tâche 5 `TestListOddNames` ; tâche 12 `TestDownloadOddFileName`.
2. **`?path=` maladroit ou hostile** (vide, relatif, `..`, `//`, `/` final, `%2F`, octet nul) : nettoyé, jamais hors racine ; octet nul → `404`. → Tâche 4 `TestEvasionDotDotPath` ; tâche 10 `TestFSListCleansPath`, `TestFSNULInPath`.
3. **Identifiant de conteneur hostile dans l'URL** (`.hidden`, `a%2Fb`, `%2E%2E`) : `404`, aucune vue ouverte, rien n'atteint l'API Docker. → Tâche 10 `TestFSBadContainerRef`.
4. **Conteneur redémarré, arrêté ou supprimé entre deux clics.**
   - Attendu : `container_changed` / `not_found` / message « arrêté », avec un bouton « Recharger », jamais une page blanche.
   - Après un redémarrage, l'exploration repart avec le nouveau PID.
   - → Tâche 6 `TestOpenRootDetectsDeadProcess` ; tâche 8 `TestIntegrationProcFSAfterRestart`, `TestIntegrationForStoppedContainer` ; tâche 10 `TestFSListErrors` ; tâche 14 (bouton Recharger).
5. **Très gros dossier et fichiers aux limites.**
   - Plus de 10 000 entrées → listing tronqué, avec un bandeau.
   - Fichier vide et fichier d'exactement 2 Mio → aperçu ; 2 Mio + 1 → `too_large`.
   - → Tâche 5 `TestListTruncatesAtMaxEntries` ; tâche 11 `TestPreviewSizeLimit` ; tâche 14 (bandeau `files.truncated`).

## Ordre : chaque test d'évasion précède le code qu'il protège

| Protection (spec) | Tests écrits | Code qu'ils protègent |
|---|---|---|
| Ancrage cgroup (§5.1 ét. 3, §10) | Tâche 2, étape 1 | Tâche 2, étape 3 |
| Liens piégés, `..`, boucle, lien vers lien, dossier lié (§5.1 ét. 6–8, §10) | Tâche 4 (commit des tests rouges) | Tâche 5 |
| Fichiers non réguliers : FIFO sans blocage, socket, périphérique (§5.1 ét. 6) | Tâche 4 | Tâche 5 |
| Refus **avant** `Reopen` : zéro `IN_OPEN` sur la FIFO et le périphérique (revue I1) | Tâche 4 (`TestRefusalHappensBeforeReopen`) | Tâche 5 |
| Liens magiques `/proc/<pid>/root`, directs ou par un lien (revue I2) | Tâche 4 (`TestMagicLinksNeverFollowed`, racine `/`) | Tâche 5 ; confirmé sur une cible `--pid=host` à la tâche 8 |
| Pseudo-systèmes par `fstatfs`, lien vers `/proc` (§5.1 ét. 9) | Tâche 4 | Tâche 5 |
| Chemin sans lien pour le téléchargement d'un dossier (§5.4) | Tâche 4 (`TestStatNoLinks`) | Tâche 5 (`StatNoLinks`) ; tâche 12 (route) |
| PID réattribué, mauvais cgroup, process mort avant `pidfd_send_signal` (§5.1 ét. 2–5, revue I4) | Tâche 6, étape 1 | Tâche 6, étape 3 |
| Tar d'un dossier qui traverserait l'hôte : binds, volumes `type=`/pilotes tiers, options illisibles, pseudo-systèmes ou FUSE sous le dossier ou sous un montage Docker, `mountinfo` piégé (§5.2 ét. 7, §5.4, revue C1, R2, R4, R5) | Tâche 7, étape 1 (faux client, process de test) | Tâche 7, étape 3 |
| Câblage de ce contrôle : `Inspect.Mounts` → `For` → `CheckArchive`, pidfd vivant après `mountinfo`, route sans `GetArchive` (revue R1) | Tâche 7, étape 1 (i, ii) ; tâche 12, étape 1 (iii) | Tâche 7, étape 3 (déjà câblé par la tâche 6 : le test le prouve) ; tâche 12, étape 2 |
| Contenu actif servi en texte (§7) | Tâche 11, étape 1 | Tâche 11, étape 2 |

Entre la tâche 4 et la tâche 5, `go test ./internal/fsview/` échoue **sous Linux** : c'est voulu, les tests sont relus seuls avant l'implémentation. Sur macOS, ces fichiers ne sont pas compilés. **Ne pas pousser la branche entre les tâches 4 et 5** (M8).

## Où tester

| Repère | Commande (depuis la racine du dépôt) |
|---|---|
| **macOS** | `go test ./cmd/... ./internal/...` ; interface : `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run && npm run check && npm run lint` |
| **Docker Desktop (API Docker seule)** | `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ -v` |
| **Linux root** (conteneur `golang`, droits de l'agent recommandé, §8) | `scripts/test-linux.sh <arguments de go test>` (créé tâche 4) ; tout : `make test-linux` |
| **VM Ubuntu (AppArmor)** | `limactl start hk-ubuntu && limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...` |
| **VM Fedora (SELinux)** | idem avec `hk-fedora` |
| **CI** | job `go` (Linux non root) : tests d'évasion, les tests de périphérique se sautent ; job `integration` (root sur l'hôte) : tout. Rien à changer dans `ci.yml`. |

## Structure des fichiers

```
internal/fsview/fsview.go                 interface FS, Entry, erreurs, ModeFromUnix/UnixPerm, CleanPath
internal/fsview/cgroup.go                 CgroupHasID, NoteCgroupNamespace, changed()
internal/fsview/pseudofs_linux.go         les 15 pseudo-systèmes refusés (fstatfs)
internal/fsview/procfs_linux.go           procFS : List, Stat, Open, StatNoLinks, Close
internal/fsview/procfs_archive_linux.go   procFS.CheckArchive (binds + mountinfo)
internal/fsview/mounts.go                 HostBind, PseudoMountUnder, PseudoMountInMounts (portables)
internal/fsview/root_linux.go             openRoot/OpenRoot (pidfd + cgroup + /proc/<pid>/root), openProcFS, SetCgroupReaderForTests
internal/fsview/procfs_other.go           repli hors Linux
internal/fsview/for.go                    For(ctx, docker, id)
internal/fsview/memfs.go                  MemFS : faux FS pour les tests d'autres paquets
internal/fsview/*_test.go                 dont procfs_test.go (évasion), integration_test.go (tag integration)
internal/dockerx/dockerx.go               + Inspect (Mounts), GetArchive, VolumeOptions, ErrNotFound, ErrUnavailable
internal/dockerx/fake.go                  + Infos, Archives, ArchiveCalls, Volumes
internal/api/fsguard.go                   garde FUSE (délai, plafond par ID complet, créneau rendu au vrai retour)
internal/api/fserr.go                     erreurs → codes §6
internal/api/fs.go                        fsRequest (Inspect d'abord), GET /fs
internal/api/fs_preview.go                GET /fs/file
internal/api/fs_download.go               GET /fs/download (&check=1)
internal/api/fs_download_linux_test.go    contrôle des montages par la route, vue procfs réelle (sans Docker)
internal/api/integration_test.go          bout en bout (tag integration)
cmd/hublot/main.go                        + contrôle cgroupns au démarrage
scripts/test-linux.sh, scripts/test.Dockerfile, scripts/dev-agent.sh
web/src/lib/api.ts                        + listDir, previewFile, downloadUrl, checkDownload
web/src/lib/files.ts                      chemins, formatMode, formatSize, sortEntries
web/src/lib/components/FileBrowser.svelte, FilePreview.svelte
web/src/routes/c/[id]/+page.svelte        page du conteneur, onglet Fichiers
```

---

### Task 1: 🎓 Types de `fsview`, conversion des droits, chemins

**Où tester :** macOS.

**Files:**
- Create: `internal/fsview/fsview.go`, `internal/fsview/fsview_test.go`
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§4, selon Q2)

**Interfaces:**
- Consumes: rien
- Produces: `type EntryType string` (`TypeFile`, `TypeDir`, `TypeSymlink`, `TypeOther`) ; `type Entry struct { Name string; Type EntryType; Size int64; Mode fs.FileMode; UID, GID int; ModTime time.Time; LinkTarget string }` ; `const MaxEntries = 10_000` ; `type FS interface { List(ctx, dir string) ([]Entry, bool, error); Stat(ctx, path string) (Entry, error); Open(ctx, path string) (io.ReadCloser, error); StatNoLinks(ctx, path string) (Entry, error); CheckArchive(ctx, dir string) error; Close() error }` ; erreurs `ErrNotDir`, `ErrNotRegular`, `ErrForbiddenPath`, `ErrLinkInPath`, `ErrContainerChanged`, `ErrTooLarge`, `ErrStopped` ; `func CleanPath(p string) string` ; `func ModeFromUnix(m uint32) fs.FileMode` ; `func UnixPerm(m fs.FileMode) uint32`

> **À comprendre :** `st_mode` (Linux) et le mode d'un en-tête tar rangent setuid, setgid et sticky dans les bits `0o4000`, `0o2000`, `0o1000`. `fs.FileMode` de Go les range ailleurs (`fs.ModeSetuid`…, des bits hauts). Un simple `fs.FileMode(st.Mode & 0o7777)` perdrait donc ces trois drapeaux sans erreur : d'où une conversion explicite, testée bit par bit (spec §4). Un chemin « absent » se signale par une erreur qui vérifie `errors.Is(err, fs.ErrNotExist)` : pas besoin d'erreur maison.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/fsview/fsview_test.go` :

```go
package fsview

import (
	"io/fs"
	"testing"
)

func TestModeFromUnixSpecialBits(t *testing.T) {
	cases := []struct {
		name string
		in   uint32
		want fs.FileMode
	}{
		{"permissions only, type bits dropped", 0o100644, 0o644},
		{"setuid", 0o104755, fs.ModeSetuid | 0o755},
		{"setgid", 0o102755, fs.ModeSetgid | 0o755},
		{"sticky", 0o41777, fs.ModeSticky | 0o777},
		{"all three", 0o7777, fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o777},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModeFromUnix(c.in); got != c.want {
				t.Fatalf("ModeFromUnix(%#o) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestUnixPermRoundTrip(t *testing.T) {
	for _, m := range []uint32{0, 0o644, 0o755, 0o4755, 0o2755, 0o1777, 0o7777} {
		if got := UnixPerm(ModeFromUnix(m)); got != m {
			t.Errorf("UnixPerm(ModeFromUnix(%#o)) = %#o", m, got)
		}
	}
}

func TestCleanPath(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		"etc":           "/etc",
		"/etc/":         "/etc",
		"//etc//passwd": "/etc/passwd",
		"/a/./b":        "/a/b",
		"../../etc":     "/etc",
		"/a/../../b":    "/b",
	}
	for in, want := range cases {
		if got := CleanPath(in); got != want {
			t.Errorf("CleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/fsview/`
Expected: FAIL à la compilation (`undefined: ModeFromUnix`).

- [ ] **Step 3: Écrire `fsview.go`**

```go
// Package fsview reads the files of a container (spec §4, §5): one interface,
// procfs for a running container, tarindex for a stopped one (plan
// Explorateur 2). It never writes: package files is the only writer.
package fsview

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"time"
)

type EntryType string

const (
	TypeFile    EntryType = "file"
	TypeDir     EntryType = "dir"
	TypeSymlink EntryType = "symlink"
	TypeOther   EntryType = "other" // device, FIFO, socket
)

type Entry struct {
	Name       string
	Type       EntryType
	Size       int64
	Mode       fs.FileMode // permission bits + fs.ModeSetuid/Setgid/Sticky, never type bits
	UID, GID   int
	ModTime    time.Time
	LinkTarget string // raw, never resolved while listing
}

// MaxEntries caps a listing (spec §5.1 step 7).
const MaxEntries = 10_000

// FS is a read view of a container's tree. Paths are container paths;
// List, Stat and Open follow symbolic links, never out of the container root.
type FS interface {
	List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
	Stat(ctx context.Context, path string) (Entry, error)
	// Open returns a regular file only (ErrNotRegular otherwise).
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	// StatNoLinks stats CleanPath(path) and refuses a symbolic link anywhere
	// in it, last component included (ErrLinkInPath): spec §5.3 step 2, §5.4.
	StatNoLinks(ctx context.Context, path string) (Entry, error)
	// CheckArchive refuses (ErrForbiddenPath) the tar of directory dir when
	// Docker would read the host through it: a bind of the host's /, /proc,
	// /sys or /dev (spec §5.2 step 7), or a pseudo filesystem or FUSE mount on,
	// under or above dir (spec §5.4). Called before every GetArchive of a directory.
	CheckArchive(ctx context.Context, dir string) error
	// Close releases the view (procfs: the root and pidfd descriptors).
	Close() error
}

// Errors of the views; package api maps each one to a code of spec §6.
// A missing path is reported with an error that matches fs.ErrNotExist.
var (
	ErrNotDir           = errors.New("pas un dossier")
	ErrNotRegular       = errors.New("pas un fichier régulier")
	ErrForbiddenPath    = errors.New("chemin interdit")
	ErrLinkInPath       = errors.New("lien symbolique dans le chemin")
	ErrContainerChanged = errors.New("le conteneur a changé")
	ErrTooLarge         = errors.New("trop volumineux")
	// ErrStopped is temporary: the explorer of stopped containers (tarindex)
	// arrives with plan Explorateur 2, which removes this error.
	ErrStopped = errors.New("conteneur arrêté : explorateur pas encore disponible")
)

// CleanPath returns the absolute, lexically cleaned form of a container path.
// It only manipulates the string: it never touches a file.
func CleanPath(p string) string { return path.Clean("/" + p) }

// Unix special bits, identical in st_mode and in tar headers.
const (
	unixSetuid = 0o4000
	unixSetgid = 0o2000
	unixSticky = 0o1000
)

// ModeFromUnix converts st_mode (or a tar header mode) to Entry.Mode (spec §4):
// permission bits pass as is, setuid, setgid and sticky are mapped explicitly
// (their values differ in fs.FileMode), file type bits are dropped.
func ModeFromUnix(m uint32) fs.FileMode {
	mode := fs.FileMode(m & 0o777)
	if m&unixSetuid != 0 {
		mode |= fs.ModeSetuid
	}
	if m&unixSetgid != 0 {
		mode |= fs.ModeSetgid
	}
	if m&unixSticky != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}

// UnixPerm is the reverse of ModeFromUnix: the 0o7777 bits shown by ls and stat.
func UnixPerm(m fs.FileMode) uint32 {
	p := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		p |= unixSetuid
	}
	if m&fs.ModeSetgid != 0 {
		p |= unixSetgid
	}
	if m&fs.ModeSticky != 0 {
		p |= unixSticky
	}
	return p
}
```

- [ ] **Step 4: Vérifier**

Run: `go vet ./internal/fsview/ && go test ./internal/fsview/ -v`
Expected: PASS (3 tests, dont 5 sous-tests de bits).

- [ ] **Step 5: Reporter l'interface dans la spec (Q2)**

Dans `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` :
- §4, arborescence : remplacer `fsview.go          type FS interface { List, Stat, Open } + types Entry` par `fsview.go          type FS interface { List, Stat, Open, StatNoLinks, CheckArchive, Close } + types Entry`.
- §4, bloc de l'interface : remplacer le bloc `type FS interface { … }` par :

```go
  type FS interface {
      List(ctx context.Context, dir string) (entries []Entry, truncated bool, err error)
      Stat(ctx context.Context, path string) (Entry, error)
      Open(ctx context.Context, path string) (io.ReadCloser, error)
      // chemin nettoyé, aucun lien sur toute la chaîne (§5.3 étape 2, §5.4)
      StatNoLinks(ctx context.Context, path string) (Entry, error)
      // avant tout GetArchive d'un dossier : ni bind de l'hôte (§5.2 étape 7),
      // ni pseudo-système ou FUSE sur, sous ou au-dessus du dossier (§5.4)
      CheckArchive(ctx context.Context, dir string) error
      Close() error // procfs : ferme les descripteurs de la racine et du pidfd
  }
```

- [ ] **Step 6: Commit**

```bash
git add internal/fsview docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: types de fsview, conversion des droits Unix, nettoyage des chemins

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 2: 🎓 Ancrage cgroup et contrôle de l'espace de noms

**Où tester :** macOS (logique pure sur du texte ; la lecture de `/proc` arrive tâche 6).

**Files:**
- Create: `internal/fsview/cgroup.go`, `internal/fsview/cgroup_test.go`

**Interfaces:**
- Consumes: `ErrContainerChanged` (tâche 1)
- Produces: `func CgroupHasID(cgroup, id string) bool` ; `func NoteCgroupNamespace(selfCgroup string) bool` ; `func changed(format string, a ...any) error` (interne : `ErrContainerChanged` + détail, + « lancez l'agent avec --cgroupns=host » si l'espace de noms est privé)

> **À comprendre :** `/proc/<pid>/cgroup` contient des lignes `hiérarchie:contrôleurs:chemin`, par ex. `0::/system.slice/docker-<id>.scope` (pilote systemd) ou `0::/docker/<id>` (pilote cgroupfs). Le spike comparait par sous-chaîne : un conteneur nommant son cgroup `not<id>evil.scope` passait. La règle (§5.1 étape 3) : on découpe le chemin en composants, on prend le **plus externe** qui a la forme d'un ID Docker, et il doit être l'ID complet. Tout composant `..` est refusé (agent sans `--cgroupns=host`). Q7 précise le cas cgroup v1. Le test est une table : une ligne par cas de §10, plus les cas cgroup v1.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/fsview/cgroup_test.go` :

```go
package fsview

import (
	"errors"
	"strings"
	"testing"
)

func TestCgroupHasID(t *testing.T) {
	id := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)
	cases := []struct {
		name, cgroup string
		want         bool
	}{
		{"systemd", "0::/system.slice/docker-" + id + ".scope\n", true},
		{"cgroupfs", "0::/docker/" + id + "\n", true},
		{"docker:dind child", "0::/system.slice/docker-" + id + ".scope/init\n", true},
		{"cgroupfs child", "0::/docker/" + id + "/child\n", true},
		{"systemd with --cgroup-parent", "0::/custom.slice/docker-" + id + ".scope\n", true},
		{"namespace root", "0::/\n", false},
		{"empty", "", false},
		{"short id", "0::/docker/" + id[:12] + "\n", false},
		{"substring in a scope", "0::/system.slice/not" + id + "evil.scope\n", false},
		{"id followed by junk", "0::/docker/" + id + "x\n", false},
		{"scope followed by junk", "0::/system.slice/docker-" + id + ".scopex\n", false},
		{"uppercase id", "0::/docker/" + strings.ToUpper(id) + "\n", false},
		{"dotdot cgroupfs", "0::/../" + id + "\n", false},
		{"dotdot systemd", "0::/../docker-" + id + ".scope\n", false},
		{"dotdot after the anchor", "0::/docker/" + id + "/..\n", false},
		{"nested scope, other outer", "0::/system.slice/docker-" + other + ".scope/docker-" + id + ".scope\n", false},
		{"nested cgroupfs, other outer", "0::/docker/" + other + "/docker/" + id + "\n", false},
		{"cgroupfs with --cgroup-parent (v0.1 limit)", "0::/myparent/" + id + "\n", false},
		{"cgroup v1, every controller", "12:pids:/docker/" + id + "\n11:memory:/docker/" + id + "\n0::/docker/" + id + "\n", true},
		{"cgroup v1, a controller at /", "12:rdma:/\n11:memory:/docker/" + id + "\n", true},
		{"cgroup v1, conflicting lines", "12:pids:/docker/" + other + "\n11:memory:/docker/" + id + "\n", false},
		{"malformed line", "garbage\n0::/docker/" + id + "\n", false},
	}
	for _, c := range cases {
		if got := CgroupHasID(c.cgroup, id); got != c.want {
			t.Errorf("%s: CgroupHasID(%q) = %v, want %v", c.name, c.cgroup, got, c.want)
		}
	}
	if !CgroupHasID("0::/system.slice/docker-"+other+".scope/docker-"+id+".scope\n", other) {
		t.Error("the outer container must still match its own nested cgroups")
	}
	if CgroupHasID("0::/docker/abc\n", "abc") {
		t.Error("a short id must never match")
	}
}

func TestChangedHintInPrivateNamespace(t *testing.T) {
	t.Cleanup(func() { NoteCgroupNamespace("") })
	if !NoteCgroupNamespace("0::/\n") {
		t.Fatal("0::/ is the root of a private cgroup namespace")
	}
	err := changed("pid %d", 7)
	if !errors.Is(err, ErrContainerChanged) || !strings.Contains(err.Error(), "--cgroupns=host") {
		t.Fatalf("got %v", err)
	}
	if NoteCgroupNamespace("0::/system.slice/docker-" + strings.Repeat("ab", 32) + ".scope\n") {
		t.Fatal("a host cgroup path is not a private namespace")
	}
	if err := changed("pid %d", 7); strings.Contains(err.Error(), "--cgroupns=host") {
		t.Fatalf("hint only in a private namespace: %v", err)
	}
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/fsview/`
Expected: FAIL à la compilation (`undefined: CgroupHasID`).

- [ ] **Step 3: Écrire `cgroup.go`**

```go
package fsview

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
)

// CgroupHasID reports whether the content of /proc/<pid>/cgroup places the
// process inside the container with this full ID (spec §5.1 step 3).
//
// Each line's path is anchored on its outermost Docker-shaped component:
// "docker-<64 hex>.scope" (systemd driver) or "<64 hex>" right under a
// "docker" component (cgroupfs driver). Child cgroups of the anchor are
// accepted (docker:dind moves its PID 1 to ".../init"). A ".." component
// anywhere, an anchor for another ID or a malformed line refuses the whole
// content. A line without any Docker-shaped component (a cgroup v1
// controller left at "/") proves nothing and is skipped; at least one line
// must match.
func CgroupHasID(cgroup, id string) bool {
	if !isDockerID(id) {
		return false
	}
	matched := false
	for _, line := range strings.Split(cgroup, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 3) // hierarchy:controllers:path
		if len(fields) != 3 {
			return false
		}
		comps := strings.Split(fields[2], "/")
		if slices.Contains(comps, "..") {
			return false
		}
		switch anchor := outermostID(comps); anchor {
		case "":
			continue
		case id:
			matched = true
		default:
			return false
		}
	}
	return matched
}

// outermostID returns the ID of the first Docker-shaped component from the
// root, or "" if there is none.
func outermostID(comps []string) string {
	for i, c := range comps {
		if s, ok := strings.CutPrefix(c, "docker-"); ok {
			if s, ok := strings.CutSuffix(s, ".scope"); ok && isDockerID(s) {
				return s
			}
		}
		if i > 0 && comps[i-1] == "docker" && isDockerID(c) {
			return c
		}
	}
	return ""
}

// isDockerID: exactly 64 lowercase hexadecimal characters.
func isDockerID(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}

var privateCgroupNS atomic.Bool

// NoteCgroupNamespace records, from the content of /proc/self/cgroup, whether
// the agent runs in a private cgroup namespace ("0::/", spec §8), and reports
// it. Every later ErrContainerChanged then carries the --cgroupns=host hint.
func NoteCgroupNamespace(selfCgroup string) bool {
	private := slices.Contains(strings.Split(selfCgroup, "\n"), "0::/")
	privateCgroupNS.Store(private)
	return private
}

// changed builds an ErrContainerChanged with its detail.
func changed(format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if privateCgroupNS.Load() {
		msg += " ; lancez l'agent avec --cgroupns=host"
	}
	return fmt.Errorf("%w : %s", ErrContainerChanged, msg)
}
```

- [ ] **Step 4: Vérifier**

Run: `go vet ./internal/fsview/ && go test ./internal/fsview/ -run 'Cgroup|Changed' -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/fsview
git commit -F - <<'EOF'
explorateur: ancrage cgroup sur le composant le plus externe, indice --cgroupns=host

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 3: `dockerx` : `Inspect` (avec les montages), `GetArchive`, `VolumeOptions`, erreurs classées

**Où tester :** macOS (unitaires) ; Docker Desktop (intégration).

**Files:**
- Modify: `internal/dockerx/dockerx.go`, `internal/dockerx/fake.go`, `internal/dockerx/dockerx_test.go`, `internal/dockerx/integration_test.go`, `go.mod`, `go.sum`

**Interfaces:**
- Consumes: rien de nouveau
- Produces: `type ContainerInfo struct { ID, Name string; Running bool; Pid int; Mounts []Mount }` ; `type Mount struct { Type, Name, Source, Destination, Driver string }` ; `var ErrNotFound, ErrUnavailable error` ; dans `Client` : `Inspect(ctx, id string) (ContainerInfo, error)` (nom, ID court ou complet), `GetArchive(ctx, id, path string) (io.ReadCloser, error)`, `VolumeOptions(ctx, name string) (map[string]string, error)` ; `Fake` gagne `Infos map[string]ContainerInfo` (clé : ID complet ou nom), `Archives map[string][]byte` (clé `id + ":" + path`), `ArchiveCalls []string`, `Volumes map[string]map[string]string`

> **À comprendre :** le SDK renvoie des erreurs typées par `github.com/containerd/errdefs` (`cerrdefs.IsNotFound`). `dockerx` les classe en deux sentinelles : `ErrNotFound` (conteneur ou chemin absent → `404 not_found`) et `ErrUnavailable` (tout le reste → `503 docker_unavailable`). On enveloppe avec `%v` et non `%w` pour l'erreur d'origine : un socket absent est un `ENOENT`, qui ne doit jamais passer pour un fichier absent du conteneur. `Mounts` et `VolumeOptions` servent au contrôle des binds de l'hôte (tâche 7, revue C1) : un volume `local` créé avec `o=bind` et `device=/proc` est un bind déguisé, et seules ses options le disent.

- [ ] **Step 1: Écrire les tests unitaires qui échouent**

Dans `internal/dockerx/dockerx_test.go`, remplacer le bloc d'imports par :

```go
import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"reflect"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
)
```

et ajouter à la fin :

```go
func TestFromInspect(t *testing.T) {
	got := fromInspect(container.InspectResponse{
		ID: "abc", Name: "/web", State: &container.State{Running: true, Pid: 42},
		Mounts: []container.MountPoint{
			{Type: "bind", Source: "/proc", Destination: "/host/proc"},
			{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local"},
		},
	})
	want := ContainerInfo{ID: "abc", Name: "web", Running: true, Pid: 42, Mounts: []Mount{
		{Type: "bind", Source: "/proc", Destination: "/host/proc"},
		{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if got := fromInspect(container.InspectResponse{ID: "x"}); got.Running || got.Pid != 0 || got.Mounts != nil {
		t.Fatalf("a nil State must read as stopped: %+v", got)
	}
}

func TestWrapClassifiesErrors(t *testing.T) {
	if err := wrap(fmt.Errorf("inspect: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("not found: %v", err)
	}
	if err := wrap(errors.New("dial unix /var/run/docker.sock: connect: no such file or directory")); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		t.Fatalf("unavailable: %v", err)
	}
}

func TestFakeInspectAndArchive(t *testing.T) {
	f := &Fake{
		Infos:    map[string]ContainerInfo{"web": {ID: "full", Name: "web", Running: true, Pid: 7}},
		Archives: map[string][]byte{"full:/etc": []byte("TAR")},
	}
	if info, err := f.Inspect(context.Background(), "web"); err != nil || info.ID != "full" {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	if _, err := f.Inspect(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown container: %v", err)
	}
	rc, err := f.GetArchive(context.Background(), "full", "/etc")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	if string(b) != "TAR" || len(f.ArchiveCalls) != 1 || f.ArchiveCalls[0] != "full:/etc" {
		t.Fatalf("archive %q, calls %v", b, f.ArchiveCalls)
	}
	if _, err := f.GetArchive(context.Background(), "full", "/absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent path: %v", err)
	}
	f.Volumes = map[string]map[string]string{"v": {"o": "bind", "device": "/srv"}}
	if opts, err := f.VolumeOptions(context.Background(), "v"); err != nil || opts["device"] != "/srv" {
		t.Fatalf("volume: %v %v", opts, err)
	}
	if _, err := f.VolumeOptions(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent volume: %v", err)
	}
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/dockerx/`
Expected: FAIL à la compilation (`undefined: fromInspect`).

- [ ] **Step 3: Écrire le code**

`internal/dockerx/dockerx.go` (contenu complet ; `fromSummary` et `ListContainers` sont inchangés) :

```go
// Package dockerx is the agent's only gateway to the Docker Engine API.
package dockerx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type Port struct {
	Private uint16 `json:"private"`
	Public  uint16 `json:"public,omitempty"`
	Type    string `json:"type"`
	IP      string `json:"ip,omitempty"`
}

type Container struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	Ports   []Port    `json:"ports"`
}

// ContainerInfo is the part of `docker inspect` the agent uses.
type ContainerInfo struct {
	ID      string // full 64-character ID
	Name    string // without the leading "/"
	Running bool
	Pid     int // 0 when the container does not run
	Mounts  []Mount
}

// Mount is one entry of Inspect.Mounts (binds and volumes).
type Mount struct {
	Type        string // "bind", "volume", "tmpfs"…
	Name        string // volume name
	Source      string // host path of a bind
	Destination string // path in the container
	Driver      string // volume driver ("local"…)
}

// ErrNotFound: no such container (or no such path, for an archive).
// ErrUnavailable: any other failure of the daemon or of the connection to it.
var (
	ErrNotFound    = errors.New("introuvable dans Docker")
	ErrUnavailable = errors.New("Docker indisponible")
)

type Client interface {
	ListContainers(ctx context.Context) ([]Container, error)
	Ping(ctx context.Context) error
	// Inspect accepts a full ID, a short ID or a name.
	Inspect(ctx context.Context, id string) (ContainerInfo, error)
	// GetArchive streams the tar of path (GET /containers/{id}/archive).
	// The caller closes it.
	GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error)
	// VolumeOptions returns the driver options of a named volume
	// (GET /volumes/{name}, field Options), e.g. o=bind, device=/proc.
	VolumeOptions(ctx context.Context, name string) (map[string]string, error)
}

type engine struct{ c *client.Client }

// New connects lazily: no request is sent until the first call.
// The API version is negotiated on the first request.
func New() (Client, error) {
	c, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &engine{c: c}, nil
}

func (e *engine) Ping(ctx context.Context) error {
	_, err := e.c.Ping(ctx, client.PingOptions{})
	return err
}

func (e *engine) ListContainers(ctx context.Context) ([]Container, error) {
	res, err := e.c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, fromSummary(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func fromSummary(s container.Summary) Container {
	name := ""
	for _, n := range s.Names {
		if t := strings.TrimPrefix(n, "/"); t != "" && !strings.Contains(t, "/") {
			name = t // link aliases look like "/other/alias"
			break
		}
	}
	if name == "" && len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	}
	if name == "" {
		name = s.ID[:min(12, len(s.ID))]
	}
	type portKey struct {
		private, public uint16
		typ, ip         string
	}
	ports := make([]Port, 0, len(s.Ports))
	seen := map[portKey]bool{}
	for _, p := range s.Ports {
		ip := ""
		if p.IP.IsValid() {
			ip = p.IP.String()
			if p.IP.IsUnspecified() {
				ip = "0.0.0.0" // 0.0.0.0 and :: are the same mapping
			}
		}
		key := portKey{p.PrivatePort, p.PublicPort, p.Type, ip}
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, Port{Private: p.PrivatePort, Public: p.PublicPort, Type: p.Type, IP: ip})
	}
	return Container{
		ID:      s.ID,
		Name:    name,
		Image:   s.Image,
		State:   string(s.State),
		Status:  s.Status,
		Created: time.Unix(s.Created, 0).UTC(),
		Ports:   ports,
	}
}

func (e *engine) Inspect(ctx context.Context, id string) (ContainerInfo, error) {
	res, err := e.c.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return ContainerInfo{}, wrap(err)
	}
	return fromInspect(res.Container), nil
}

func fromInspect(c container.InspectResponse) ContainerInfo {
	info := ContainerInfo{ID: c.ID, Name: strings.TrimPrefix(c.Name, "/")}
	if c.State != nil {
		info.Running = c.State.Running
		info.Pid = c.State.Pid
	}
	for _, m := range c.Mounts {
		info.Mounts = append(info.Mounts, Mount{
			Type: string(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: m.Driver,
		})
	}
	return info
}

func (e *engine) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
	res, err := e.c.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return nil, wrap(err)
	}
	return res.Content, nil
}

func (e *engine) VolumeOptions(ctx context.Context, name string) (map[string]string, error) {
	res, err := e.c.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return nil, wrap(err)
	}
	return res.Volume.Options, nil
}

// wrap classifies a daemon error for the API (not_found / docker_unavailable).
func wrap(err error) error {
	if cerrdefs.IsNotFound(err) {
		return fmt.Errorf("%w : %v", ErrNotFound, err)
	}
	return fmt.Errorf("%w : %v", ErrUnavailable, err)
}
```

`internal/dockerx/fake.go` (contenu complet) :

```go
package dockerx

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// Fake is an in-memory Client for tests of other packages. It is not safe for
// concurrent use: tests send one request at a time.
type Fake struct {
	Containers []Container
	// Infos answers Inspect, by full ID or by name (tests add both keys).
	Infos map[string]ContainerInfo
	// Archives answers GetArchive, keyed by id + ":" + path.
	Archives map[string][]byte
	// ArchiveCalls records every GetArchive call as id + ":" + path.
	ArchiveCalls []string
	// Volumes answers VolumeOptions, by volume name.
	Volumes map[string]map[string]string
	Err     error
}

func (f *Fake) ListContainers(context.Context) ([]Container, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Containers, nil
}

func (f *Fake) Ping(context.Context) error { return f.Err }

func (f *Fake) Inspect(_ context.Context, id string) (ContainerInfo, error) {
	if f.Err != nil {
		return ContainerInfo{}, f.Err
	}
	if info, ok := f.Infos[id]; ok {
		return info, nil
	}
	return ContainerInfo{}, fmt.Errorf("%w : %s", ErrNotFound, id)
}

func (f *Fake) GetArchive(_ context.Context, id, path string) (io.ReadCloser, error) {
	f.ArchiveCalls = append(f.ArchiveCalls, id+":"+path)
	if f.Err != nil {
		return nil, f.Err
	}
	b, ok := f.Archives[id+":"+path]
	if !ok {
		return nil, fmt.Errorf("%w : %s:%s", ErrNotFound, id, path)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *Fake) VolumeOptions(_ context.Context, name string) (map[string]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if opts, ok := f.Volumes[name]; ok {
		return opts, nil
	}
	return nil, fmt.Errorf("%w : volume %s", ErrNotFound, name)
}
```

Run: `go mod tidy && go vet ./... && go test ./internal/dockerx/ -v`
Expected: PASS ; `go.mod` montre `github.com/containerd/errdefs v1.0.0` en dépendance directe.

- [ ] **Step 4: Test d'intégration**

Dans `internal/dockerx/integration_test.go`, ajouter `"archive/tar"` et `"errors"` aux imports, puis à la fin :

```go
func TestInspectAndArchiveRealDocker(t *testing.T) {
	name := "hublot-it-inspect"
	exec.Command("docker", "rm", "-f", name).Run()
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "alpine:3", "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := c.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.ID) != 64 || info.Name != name || !info.Running || info.Pid == 0 {
		t.Fatalf("inspect: %+v", info)
	}
	rc, err := c.GetArchive(ctx, info.ID, "/etc/hostname")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	h, err := tar.NewReader(rc).Next()
	if err != nil || h.Name != "hostname" {
		t.Fatalf("archive: %+v %v", h, err)
	}
	if _, err := c.Inspect(ctx, "hublot-it-absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent container: %v", err)
	}
	if _, err := c.GetArchive(ctx, info.ID, "/absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent path: %v", err)
	}
}

func TestMountsAndVolumeOptionsRealDocker(t *testing.T) {
	vol, name := "hublot-it-vol", "hublot-it-mounts"
	exec.Command("docker", "rm", "-f", name).Run()
	exec.Command("docker", "volume", "rm", "-f", vol).Run()
	// A local volume that is a bind in disguise (spec §5.2 step 7). Creating it
	// mounts nothing; the container below mounts it on /data.
	if out, err := exec.Command("docker", "volume", "create", "--opt", "type=none", "--opt", "o=bind",
		"--opt", "device=/tmp", vol).CombinedOutput(); err != nil {
		t.Fatalf("docker volume create: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "volume", "rm", "-f", vol).Run() })
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "-v", vol+":/data", "alpine:3", "sleep", "300").CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := c.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Mounts) != 1 || info.Mounts[0].Type != "volume" || info.Mounts[0].Name != vol ||
		info.Mounts[0].Driver != "local" || info.Mounts[0].Destination != "/data" {
		t.Fatalf("mounts: %+v", info.Mounts)
	}
	opts, err := c.VolumeOptions(ctx, vol)
	if err != nil || opts["o"] != "bind" || opts["device"] != "/tmp" {
		t.Fatalf("options: %v %v", opts, err)
	}
}
```

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ -run RealDocker -v`
Expected: PASS (3 tests). Un `GET /archive` sur un chemin absent répond 404 : c'est bien `ErrNotFound`. Le volume du troisième test est un bind de `/tmp` (sans danger) : il ne fait que lire ses options.

- [ ] **Step 5: Commit**

```bash
git add internal/dockerx go.mod go.sum
git commit -F - <<'EOF'
explorateur: dockerx.Inspect avec les montages, GetArchive, VolumeOptions, erreurs classées

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 4: Tests d'évasion de `procfs` (rouges) et banc de test Linux

**Où tester :** Linux root (`scripts/test-linux.sh`, créé ici) ; macOS pour vérifier que rien ne casse hors Linux.

**Files:**
- Create: `scripts/test.Dockerfile`, `scripts/test-linux.sh`, `internal/fsview/procfs_linux.go` (squelette), `internal/fsview/procfs_test.go`
- Modify: `Makefile`, `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `FS`, `Entry`, `MaxEntries`, erreurs (tâche 1)
- Produces: `func NewProcFS(root *os.File) FS` (Linux ; prend possession de `root`). C'est un squelette ici, implémenté à la tâche 5. Aussi : `scripts/test-linux.sh [args de go test]` et la cible `make test-linux`.

> **À comprendre :** ces tests sont la protection principale de l'agent (spec §10, §13).
>
> - Ils sont écrits et relus **seuls**, avant l'implémentation, pour qu'on ne les taille pas sur le code.
> - Chaque piège a un *contrôle* : la lecture naïve par le chemin de l'hôte, elle, doit sortir de la racine ; sinon le piège ne prouve rien.
> - `assertNoEscape` distingue deux échecs (revue M1) : « ÉVASION » si le secret de l'hôte a été lu, « mauvaise erreur » si l'appel a échoué autrement qu'attendu.
> - Une racine de conteneur est simulée par un dossier temporaire ouvert en `O_PATH|O_DIRECTORY`, comme `/proc/<pid>/root`.
> - Les tests des pseudo-systèmes et des liens magiques prennent la vraie racine `/` de la machine de test, dont `/proc` et `/sys` sont réels. Suivre `/proc/1/root` ou `/proc/self/root` y ramènerait sur un fichier lisible : le test exige une erreur (revue I2). Le lien vers `/proc/self/root` garde le test utile sans root (job CI `go`), où `/proc/1/root` est de toute façon refusé (revue R6).
> - `TestRefusalHappensBeforeReopen` (revue I1) surveille la FIFO et le nœud de périphérique par `inotify`. Un refus correct ne les ouvre **jamais** en lecture : zéro `IN_OPEN`. Une ouverture `O_PATH` n'en produit pas ; si le noyau en produisait, le test se sauterait avec un message.
> - Le squelette renvoie une erreur partout : 20 tests sur 21 échouent. Seul `TestSymlinkLoopFails` passe, car il n'exige qu'une erreur.
>
> `scripts/test-linux.sh` lance `go test` dans un conteneur `golang` avec les droits de l'agent recommandé (§8) : root, `CAP_MKNOD` (défaut de Docker), `--pid=host --cgroupns=host --cap-add SYS_PTRACE`, AppArmor et SELinux levés, socket Docker monté. Le client `docker` y est installé : les tests d'intégration créent leurs cibles avec. Le script marche tel quel avec Docker Desktop et dans les VM Lima.

- [ ] **Step 1: Le banc de test Linux**

`scripts/test.Dockerfile` :

```dockerfile
# Image des tests Linux (scripts/test-linux.sh) : Go + client Docker.
FROM golang:1.27
COPY --from=docker:cli /usr/local/bin/docker /usr/local/bin/docker
```

`scripts/test-linux.sh` :

```bash
#!/usr/bin/env bash
# Lance les tests Go sous Linux, en root, dans un conteneur golang (CLAUDE.md,
# spec §10). Mêmes droits que l'agent recommandé (spec §8) : le conteneur voit
# /proc/<pid>/root des autres conteneurs et parle au démon par son socket.
# Marche avec Docker Desktop (macOS) et dans les VM Lima hk-ubuntu / hk-fedora.
#
# Usage : scripts/test-linux.sh [arguments de go test]
#   scripts/test-linux.sh ./internal/fsview/ -run Evasion -v
#   scripts/test-linux.sh -tags integration ./internal/...
set -euo pipefail
cd "$(dirname "$0")/.."
docker build -q -t hublot-test -f scripts/test.Dockerfile scripts >/dev/null
if [ "$#" -eq 0 ]; then set -- ./cmd/... ./internal/...; fi
exec docker run --rm \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD":/src -w /src \
  -v hublot-gomod:/go/pkg/mod -v hublot-gocache:/root/.cache/go-build \
  hublot-test go test "$@"
```

Run: `chmod +x scripts/test-linux.sh`

`Makefile` : ajouter `test-linux` à la ligne `.PHONY`, puis la cible :

```make
test-linux:
	scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/...
```

- [ ] **Step 2: Le squelette**

`go get golang.org/x/sys@v0.48.0` (version de la spec §5.1 étape 9 ; les tests utilisent `unix`).

`internal/fsview/procfs_linux.go` :

```go
//go:build linux

package fsview

import (
	"context"
	"errors"
	"io"
	"os"
)

// Skeleton: the evasion tests of procfs_test.go are written and reviewed
// before the code they protect (CLAUDE.md). Task 5 replaces this file.
var errNotImplemented = errors.New("procfs : pas encore implémenté (tâche 5)")

type procFS struct{ root *os.File }

// NewProcFS serves the tree under root, a descriptor on the container's root
// directory (O_PATH is enough). It takes ownership of root: Close closes it.
func NewProcFS(root *os.File) FS { return &procFS{root: root} }

func (p *procFS) Close() error { return p.root.Close() }

func (p *procFS) List(context.Context, string) ([]Entry, bool, error) {
	return nil, false, errNotImplemented
}

func (p *procFS) Stat(context.Context, string) (Entry, error) { return Entry{}, errNotImplemented }

func (p *procFS) Open(context.Context, string) (io.ReadCloser, error) { return nil, errNotImplemented }

func (p *procFS) StatNoLinks(context.Context, string) (Entry, error) {
	return Entry{}, errNotImplemented
}

func (p *procFS) CheckArchive(context.Context, string) error { return errNotImplemented }
```

- [ ] **Step 3: Écrire les tests d'évasion**

`internal/fsview/procfs_test.go` :

```go
//go:build linux

package fsview

// Evasion tests of procfs (spec §5.1, §10), written before the code. A
// temporary directory plays the container root; a host file outside it holds
// HOST-SECRET. Building the traps uses host paths (filepath, os.Symlink): that
// is the test fixture, never the code under test, which only sees the FS.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var ctx = context.Background()

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// openTestRoot opens dir the way OpenRoot opens /proc/<pid>/root.
func openTestRoot(t *testing.T, dir string) FS {
	t.Helper()
	fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	must(t, err)
	fsys := NewProcFS(os.NewFile(uintptr(fd), "test-root"))
	t.Cleanup(func() { fsys.Close() })
	return fsys
}

// newRoot returns a fake container root and a host file outside it.
func newRoot(t *testing.T) (fsys FS, rootPath, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "MARKER")
	must(t, os.WriteFile(marker, []byte("HOST-SECRET"), 0o644))
	rootPath = t.TempDir()
	return openTestRoot(t, rootPath), rootPath, marker
}

func readAll(fsys FS, p string) ([]byte, error) {
	rc, err := fsys.Open(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// control proves the trap works: a naive read through the host path escapes.
func control(t *testing.T, hostPath string) {
	t.Helper()
	if b, err := os.ReadFile(hostPath); err != nil || string(b) != "HOST-SECRET" {
		t.Fatalf("contrôle : la lecture naïve devait sortir de la racine, lu %q err=%v", b, err)
	}
}

// assertNoEscape: the read never saw the secret (ÉVASION otherwise), and
// failed with "not found" (a wrong error is reported apart: it is a bug, not
// an escape).
func assertNoEscape(t *testing.T, b []byte, err error) {
	t.Helper()
	if bytes.Contains(b, []byte("HOST-SECRET")) {
		t.Fatalf("ÉVASION : le secret de l'hôte a été lu (%q)", b)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur : %v (attendu fs.ErrNotExist ; pas d'évasion)", err)
	}
}

func TestEvasionAbsoluteSymlink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(marker, filepath.Join(rp, "evil")))
	control(t, filepath.Join(rp, "evil"))
	b, err := readAll(fsys, "/evil")
	assertNoEscape(t, b, err)
}

func TestEvasionRelativeSymlink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(strings.Repeat("../", 20)+strings.TrimPrefix(marker, "/"), filepath.Join(rp, "evil-rel")))
	control(t, filepath.Join(rp, "evil-rel"))
	b, err := readAll(fsys, "/evil-rel")
	assertNoEscape(t, b, err)
}

func TestEvasionDotDotPath(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	dotdot := strings.Repeat("../", 20) + strings.TrimPrefix(marker, "/")
	control(t, rp+"/"+dotdot)
	b, err := readAll(fsys, "/"+dotdot)
	assertNoEscape(t, b, err)
}

func TestEvasionLinkToLink(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink("b", filepath.Join(rp, "a")))
	must(t, os.Symlink(marker, filepath.Join(rp, "b")))
	control(t, filepath.Join(rp, "a"))
	b, err := readAll(fsys, "/a")
	assertNoEscape(t, b, err)

	// A chain that stays inside the root still works.
	must(t, os.WriteFile(filepath.Join(rp, "in.txt"), []byte("inside"), 0o644))
	must(t, os.Symlink("d", filepath.Join(rp, "c")))
	must(t, os.Symlink("/in.txt", filepath.Join(rp, "d")))
	if b, err := readAll(fsys, "/c"); err != nil || string(b) != "inside" {
		t.Fatalf("chaîne interne : %q %v", b, err)
	}
}

func TestEvasionIntermediateSymlinkDir(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.Symlink(filepath.Dir(marker), filepath.Join(rp, "evil-dir")))
	control(t, filepath.Join(rp, "evil-dir", "MARKER"))
	b, err := readAll(fsys, "/evil-dir/MARKER")
	assertNoEscape(t, b, err)
	if e, err := fsys.Stat(ctx, "/evil-dir/MARKER"); err == nil {
		t.Fatalf("ÉVASION : Stat a vu le fichier de l'hôte (%+v)", e)
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur pour Stat : %v", err)
	}
	entries, _, err := fsys.List(ctx, "/evil-dir")
	if len(entries) != 0 {
		t.Fatalf("ÉVASION : List a rendu le dossier de l'hôte %v", entries)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("mauvaise erreur pour List : %v", err)
	}
}

func TestSymlinkLoopFails(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.Symlink("loop2", filepath.Join(rp, "loop1")))
	must(t, os.Symlink("loop1", filepath.Join(rp, "loop2")))
	if _, err := readAll(fsys, "/loop1"); err == nil {
		t.Fatal("une boucle de liens doit échouer")
	}
	if _, _, err := fsys.List(ctx, "/loop1"); err == nil {
		t.Fatal("une boucle de liens doit échouer au listing")
	}
}

func TestListShowsLinksWithoutFollowing(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "a.txt"), []byte("hello"), 0o640))
	must(t, os.Chmod(filepath.Join(rp, "a.txt"), 0o640)) // the umask may have changed it
	must(t, os.Mkdir(filepath.Join(rp, "dir"), 0o755))
	must(t, os.Symlink("/etc/passwd", filepath.Join(rp, "link")))
	entries, truncated, err := fsys.List(ctx, "/")
	must(t, err)
	if truncated {
		t.Fatal("troncature inattendue")
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Name] = e
	}
	if e := got["a.txt"]; e.Type != TypeFile || e.Size != 5 || e.Mode != 0o640 || e.UID != os.Getuid() || e.ModTime.IsZero() {
		t.Errorf("a.txt : %+v", e)
	}
	if e := got["dir"]; e.Type != TypeDir {
		t.Errorf("dir : %+v", e)
	}
	if e := got["link"]; e.Type != TypeSymlink || e.LinkTarget != "/etc/passwd" {
		t.Errorf("link : %+v (le lien doit être montré, jamais suivi)", e)
	}
}

func TestListOddNames(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	names := []string{"with space", "émoji-🎉", "new\nline", "latin1-\xe9", "-dash"}
	for _, n := range names {
		must(t, os.WriteFile(filepath.Join(rp, n), nil, 0o644))
	}
	entries, _, err := fsys.List(ctx, "/")
	must(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Errorf("absent : %q", n)
		}
	}
}

func TestListTruncatesAtMaxEntries(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	big := filepath.Join(rp, "big")
	must(t, os.Mkdir(big, 0o755))
	for i := 0; i <= MaxEntries; i++ {
		f, err := os.Create(filepath.Join(big, fmt.Sprintf("f%05d", i)))
		must(t, err)
		f.Close()
	}
	entries, truncated, err := fsys.List(ctx, "/big")
	must(t, err)
	if len(entries) != MaxEntries || !truncated {
		t.Fatalf("%d entrées, truncated=%v ; attendu %d, true", len(entries), truncated, MaxEntries)
	}
	must(t, os.Remove(filepath.Join(big, "f00000")))
	entries, truncated, err = fsys.List(ctx, "/big")
	must(t, err)
	if len(entries) != MaxEntries || truncated {
		t.Fatalf("%d entrées, truncated=%v ; attendu %d, false", len(entries), truncated, MaxEntries)
	}
}

func TestOpenAndStatInRoot(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.MkdirAll(filepath.Join(rp, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "d", "f.txt"), []byte("in-root"), 0o644))
	must(t, os.Symlink("d/f.txt", filepath.Join(rp, "l")))
	if b, err := readAll(fsys, "/d/f.txt"); err != nil || string(b) != "in-root" {
		t.Fatalf("lecture : %q %v", b, err)
	}
	if b, err := readAll(fsys, "/l"); err != nil || string(b) != "in-root" {
		t.Fatalf("lecture par un lien interne : %q %v", b, err)
	}
	e, err := fsys.Stat(ctx, "/l")
	if err != nil || e.Type != TypeFile || e.Size != 7 || e.Name != "l" {
		t.Fatalf("Stat suit les liens : %+v %v", e, err)
	}
	if _, _, err := fsys.List(ctx, "/d/f.txt"); !errors.Is(err, ErrNotDir) {
		t.Fatalf("lister un fichier : %v, attendu ErrNotDir", err)
	}
	if _, err := fsys.Stat(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent : %v", err)
	}
}

func TestModeSpecialBitsFromDisk(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.WriteFile(filepath.Join(rp, "suid"), nil, 0o755))
	must(t, os.Chmod(filepath.Join(rp, "suid"), 0o755|os.ModeSetuid))
	must(t, os.Mkdir(filepath.Join(rp, "tmp"), 0o777))
	must(t, os.Chmod(filepath.Join(rp, "tmp"), 0o777|os.ModeSticky))
	if e, err := fsys.Stat(ctx, "/suid"); err != nil || e.Mode != fs.ModeSetuid|0o755 {
		t.Fatalf("setuid : %v %v", e.Mode, err)
	}
	if e, err := fsys.Stat(ctx, "/tmp"); err != nil || e.Mode != fs.ModeSticky|0o777 {
		t.Fatalf("sticky : %v %v", e.Mode, err)
	}
}

// openWithTimeout fails the test if Open blocks (a FIFO opened for reading
// without O_NONBLOCK waits for a writer forever).
func openWithTimeout(t *testing.T, fsys FS, p string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		rc, err := fsys.Open(ctx, p)
		if err == nil {
			rc.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Open a bloqué sur un fichier non régulier")
		return nil
	}
}

func TestOpenRefusesFIFO(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, unix.Mkfifo(filepath.Join(rp, "fifo"), 0o644))
	if err := openWithTimeout(t, fsys, "/fifo"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesSocket(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	l, err := net.Listen("unix", filepath.Join(rp, "sock"))
	must(t, err)
	defer l.Close()
	if err := openWithTimeout(t, fsys, "/sock"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesDirectory(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	must(t, os.Mkdir(filepath.Join(rp, "d"), 0o755))
	if err := openWithTimeout(t, fsys, "/d"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

func TestOpenRefusesDevice(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	// /dev/null's numbers: reading it would read the host device.
	if err := unix.Mknod(filepath.Join(rp, "dev"), unix.S_IFCHR|0o666, int(unix.Mkdev(1, 3))); err != nil {
		t.Skipf("mknod refusé (%v) : ce test demande CAP_MKNOD (scripts/test-linux.sh, job CI privilégié)", err)
	}
	if err := openWithTimeout(t, fsys, "/dev"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("attendu ErrNotRegular, reçu %v", err)
	}
}

// The pseudo-filesystem tests use the test machine's own "/" as the root: its
// /proc and /sys are real procfs and sysfs.
func TestPseudoFSRefused(t *testing.T) {
	host := openTestRoot(t, "/")
	if _, err := readAll(host, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc/kallsyms : %v, attendu ErrForbiddenPath", err)
	}
	if _, err := host.Stat(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Stat /proc : %v", err)
	}
	if _, _, err := host.List(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List /proc : %v", err)
	}
	if _, _, err := host.List(ctx, "/sys"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List /sys : %v", err)
	}
	// A link that leads there is refused too: the check is on the descriptor.
	dir := t.TempDir()
	must(t, os.Symlink("/proc", filepath.Join(dir, "p")))
	if _, err := readAll(host, dir+"/p/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("lien vers /proc : %v", err)
	}
	if _, _, err := host.List(ctx, dir+"/p"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("List du lien vers /proc : %v", err)
	}
}

func TestRootListingShowsPseudoFSAsDirs(t *testing.T) {
	host := openTestRoot(t, "/")
	entries, _, err := host.List(ctx, "/")
	must(t, err)
	for _, e := range entries {
		if e.Name == "proc" {
			if e.Type != TypeDir {
				t.Fatalf("proc : %+v", e)
			}
			return
		}
	}
	t.Fatal("proc absent du listing de /")
}

func TestDevIsListableButNotReadable(t *testing.T) {
	host := openTestRoot(t, "/")
	if _, _, err := host.List(ctx, "/dev"); err != nil {
		t.Fatalf("List /dev : %v", err)
	}
	if err := openWithTimeout(t, host, "/dev/null"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("/dev/null : %v, attendu ErrNotRegular", err)
	}
}

func TestStatNoLinks(t *testing.T) {
	fsys, rp, marker := newRoot(t)
	must(t, os.MkdirAll(filepath.Join(rp, "d"), 0o755))
	must(t, os.WriteFile(filepath.Join(rp, "d", "f.txt"), []byte("x"), 0o644))
	must(t, os.Symlink("d", filepath.Join(rp, "ld")))
	must(t, os.Symlink("d/f.txt", filepath.Join(rp, "lf")))
	must(t, os.Symlink("/nope", filepath.Join(rp, "dangling")))
	must(t, os.Symlink(filepath.Dir(marker), filepath.Join(rp, "evil")))

	for _, p := range []string{"/d/f.txt", "/d", "/", "/../../d/f.txt", "d//f.txt"} {
		if _, err := fsys.StatNoLinks(ctx, p); err != nil {
			t.Errorf("%q : %v", p, err)
		}
	}
	if e, err := fsys.StatNoLinks(ctx, "/d/f.txt"); err != nil || e.Type != TypeFile || e.Name != "f.txt" {
		t.Errorf("entrée : %+v %v", e, err)
	}
	for _, p := range []string{"/lf", "/ld/f.txt", "/ld", "/dangling", "/evil/MARKER"} {
		if _, err := fsys.StatNoLinks(ctx, p); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("%q : %v, attendu ErrLinkInPath", p, err)
		}
	}
	if _, err := fsys.StatNoLinks(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent : %v", err)
	}
	host := openTestRoot(t, "/")
	if _, err := host.StatNoLinks(ctx, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Errorf("/proc/kallsyms : %v, attendu ErrForbiddenPath", err)
	}
}

// countOpens returns the number of IN_OPEN events on hostPath while fn runs.
// An O_PATH open (pathrs.OpenatInRoot) generates none; a Reopen for reading does.
func countOpens(t *testing.T, hostPath string, fn func()) int {
	t.Helper()
	in, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	must(t, err)
	defer unix.Close(in)
	_, err = unix.InotifyAddWatch(in, hostPath, unix.IN_OPEN)
	must(t, err)
	fn()
	buf := make([]byte, 64*unix.SizeofInotifyEvent)
	n, err := unix.Read(in, buf)
	if err != nil || n <= 0 {
		return 0 // EAGAIN: no event
	}
	count := 0
	for off := 0; off+unix.SizeofInotifyEvent <= n; count++ {
		nameLen := binary.NativeEndian.Uint32(buf[off+12:]) // struct inotify_event.len
		off += unix.SizeofInotifyEvent + int(nameLen)
	}
	return count
}

// The refusal of a non-regular file must happen on the O_PATH descriptor,
// before any Reopen: the FIFO and the device node are never opened for
// reading (zero IN_OPEN), not just closed right after.
func TestRefusalHappensBeforeReopen(t *testing.T) {
	fsys, rp, _ := newRoot(t)
	reg := filepath.Join(rp, "reg")
	must(t, os.WriteFile(reg, []byte("x"), 0o644))
	if n := countOpens(t, reg, func() { fsys.Stat(ctx, "/reg") }); n != 0 {
		t.Skipf("ce noyau signale IN_OPEN pour un O_PATH (%d événements) : test non concluant", n)
	}
	if n := countOpens(t, reg, func() { readAll(fsys, "/reg") }); n == 0 {
		t.Fatal("contrôle : la lecture d'un fichier régulier doit produire un IN_OPEN")
	}
	t.Run("fifo", func(t *testing.T) {
		fifo := filepath.Join(rp, "fifo")
		must(t, unix.Mkfifo(fifo, 0o644))
		if n := countOpens(t, fifo, func() { openWithTimeout(t, fsys, "/fifo") }); n != 0 {
			t.Fatalf("FIFO ouverte %d fois : le refus doit précéder Reopen", n)
		}
	})
	t.Run("device", func(t *testing.T) {
		dev := filepath.Join(rp, "dev")
		if err := unix.Mknod(dev, unix.S_IFCHR|0o666, int(unix.Mkdev(1, 3))); err != nil {
			t.Skipf("mknod refusé (%v) : ce test demande CAP_MKNOD (scripts/test-linux.sh, job CI privilégié)", err)
		}
		if n := countOpens(t, dev, func() { openWithTimeout(t, fsys, "/dev") }); n != 0 {
			t.Fatalf("périphérique ouvert %d fois : le refus doit précéder Reopen", n)
		}
	})
}

// A magic link (/proc/<pid>/root) leads out of any root: never followed,
// directly or through an ordinary link. The test machine's "/" is the root:
// following it would land on a readable file. /proc/self/root works without
// root (CI job "go"); /proc/1/root needs it.
func TestMagicLinksNeverFollowed(t *testing.T) {
	host := openTestRoot(t, "/")
	dir := t.TempDir()
	must(t, os.Symlink("/proc/1/root", filepath.Join(dir, "m")))
	must(t, os.Symlink("/proc/self/root", filepath.Join(dir, "s")))
	if _, err := os.ReadFile(dir + "/s/etc/passwd"); err != nil {
		t.Fatalf("contrôle : la lecture naïve par /proc/self/root doit réussir : %v", err)
	}
	for _, p := range []string{"/proc/1/root/etc/passwd", "/proc/self/root/etc/passwd",
		dir + "/m/etc/passwd", dir + "/s/etc/passwd"} {
		if b, err := readAll(host, p); err == nil {
			t.Errorf("%s lu (%d octets) : un lien magique ne doit jamais être suivi", p, len(b))
		}
	}
	for _, l := range []string{"/m", "/s"} {
		if entries, _, err := host.List(ctx, dir+l); err == nil {
			t.Errorf("List par le lien magique %s : %d entrées", l, len(entries))
		}
		if _, err := host.StatNoLinks(ctx, dir+l+"/etc"); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("StatNoLinks par le lien magique %s : %v", l, err)
		}
	}
}
```

- [ ] **Step 4: Vérifier qu'ils échouent sous Linux, et que macOS ne casse pas**

Run: `go mod tidy && go vet ./... && go test ./internal/fsview/`
Expected (macOS) : PASS (les fichiers `_linux` ne sont pas compilés).

Run: `GOOS=linux go vet ./internal/fsview/`
Expected : aucune sortie.

Run: `scripts/test-linux.sh ./internal/fsview/ -v 2>&1 | grep -cE '^--- FAIL'`
Expected : `20`. Seul `TestSymlinkLoopFails` passe ; les messages disent `procfs : pas encore implémenté`. Aucun échec ne doit porter `ÉVASION` : ce serait un piège mal construit.

- [ ] **Step 5: Commit (tests rouges, voulu ; ne pas pousser avant la tâche 5)**

```bash
git add scripts Makefile internal/fsview go.mod go.sum
git commit -F - <<'EOF'
explorateur: tests d'évasion de procfs (rouges) et banc de test Linux

Les tests précèdent le code qu'ils protègent (CLAUDE.md) : procfs est un
squelette qui échoue partout ; la tâche suivante l'implémente. Ne pas
pousser la branche avant la tâche 5.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 5: `procfs` : lecture sûre d'un conteneur actif

**Où tester :** Linux root (`scripts/test-linux.sh`) ; Linux non root (comme le job CI `go`).

**Files:**
- Create: `internal/fsview/pseudofs_linux.go`, `internal/fsview/procfs_archive_linux.go` (garde fermée, remplacée tâche 7)
- Modify: `internal/fsview/procfs_linux.go` (remplace le squelette), `go.mod`, `go.sum`

**Interfaces:**
- Consumes: tests de la tâche 4 ; `ModeFromUnix`, `CleanPath`, erreurs (tâche 1) ; `dockerx.Client`, `dockerx.Mount` (tâche 3)
- Produces:
  - `NewProcFS(root *os.File) FS` complet ;
  - `type procFS struct { root *os.File; pid, pidfd int; docker dockerx.Client; mounts []dockerx.Mount }` (interne, `pidfd = -1` sans process) ;
  - `checkNotPseudo(fd int, path string) error` (interne) ;
  - `CheckArchive` qui refuse tout jusqu'à la tâche 7.

> **À comprendre :** toute résolution de chemin passe par `pathrs.OpenatInRoot(root, chemin)`. La bibliothèque suit les liens *dans* la racine : un lien absolu repart de la racine du conteneur, `..` s'arrête à elle, un lien magique (`/proc/<pid>/root`) est refusé (`ELOOP`).
>
> - On obtient un descripteur `O_PATH`, qui ne permet pas de lire. On fait dessus `fstatfs` (pseudo-système → `forbidden_path`) et `fstat` (pas un fichier régulier → `not_regular`), **avant** `pathrs.Reopen`, la seule façon d'obtenir un descripteur lisible.
> - Le listing ne suit rien : `fstatat(dirfd, nom, AT_SYMLINK_NOFOLLOW)` et `readlinkat(dirfd, nom)`, un seul composant à la fois.
> - `StatNoLinks` passe par `openat2` avec `RESOLVE_IN_ROOT|RESOLVE_NO_SYMLINKS|RESOLVE_NO_MAGICLINKS` : le noyau répond `ELOOP` au premier lien rencontré, dernier composant compris.
> - `Open` ne renvoie jamais un `*os.File` nul enveloppé dans l'interface (piège Go : une interface qui contient un pointeur nul n'est pas `nil`).
> - Les champs `pid`, `pidfd`, `docker` et `mounts` servent au contrôle des montages (tâches 6 et 7). D'ici là, `CheckArchive` **refuse tout** (fermé par défaut) : aucun téléchargement de dossier ne peut passer avant que le contrôle existe.

- [ ] **Step 1: Les pseudo-systèmes**

`internal/fsview/pseudofs_linux.go` :

```go
//go:build linux

package fsview

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Local constants, absent from x/sys (spec §5.1 step 9).
const (
	configfsMagic     = 0x62656570 // include/uapi/linux/magic.h
	fuseCtlSuperMagic = 0x65735543 // fs/fuse/control.c
)

// pseudoFS lists the 15 filesystems whose files are read with the agent's host
// privileges (e.g. /proc/kallsyms gives the real kernel addresses).
var pseudoFS = map[uint32]string{
	unix.PROC_SUPER_MAGIC:    "proc",
	unix.SYSFS_MAGIC:         "sysfs",
	unix.CGROUP_SUPER_MAGIC:  "cgroup",
	unix.CGROUP2_SUPER_MAGIC: "cgroup2",
	unix.DEBUGFS_MAGIC:       "debugfs",
	unix.TRACEFS_MAGIC:       "tracefs",
	unix.SECURITYFS_MAGIC:    "securityfs",
	unix.BPF_FS_MAGIC:        "bpf",
	unix.PSTOREFS_MAGIC:      "pstore",
	unix.EFIVARFS_MAGIC:      "efivarfs",
	unix.BINFMTFS_MAGIC:      "binfmt_misc",
	unix.SELINUX_MAGIC:       "selinuxfs",
	unix.SMACK_MAGIC:         "smackfs",
	configfsMagic:            "configfs",
	fuseCtlSuperMagic:        "fusectl",
}

// checkNotPseudo refuses a descriptor (O_PATH is enough) that lives on a
// pseudo filesystem. It checks the resolved descriptor, never the path text,
// so a link that leads there is refused too.
func checkNotPseudo(fd int, path string) error {
	var st unix.Statfs_t
	if err := unix.Fstatfs(fd, &st); err != nil {
		return fmt.Errorf("fstatfs %s : %w", path, err)
	}
	if name, bad := pseudoFS[uint32(st.Type)]; bad {
		return fmt.Errorf("%w : %s est sur %s", ErrForbiddenPath, path, name)
	}
	return nil
}
```

- [ ] **Step 2: L'implémentation**

`go get github.com/cyphar/filepath-securejoin@v0.7.0`

`internal/fsview/procfs_linux.go` (remplace le squelette) :

```go
//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// procFS reads a running container through a descriptor on its root
// (spec §5.1). Every path goes through pathrs.OpenatInRoot or openat2
// RESOLVE_IN_ROOT: never filepath.Join, never os.Open on a container path.
type procFS struct {
	root *os.File
	// Set by openProcFS for a real container (CheckArchive needs them);
	// pidfd is -1 for a view built by NewProcFS.
	pid    int
	pidfd  int
	docker dockerx.Client
	mounts []dockerx.Mount
}

// NewProcFS serves the tree under root, a descriptor on a root directory
// (O_PATH is enough), without any process behind it: tests use it with a
// temporary directory. It takes ownership of root: Close closes it.
func NewProcFS(root *os.File) FS { return &procFS{root: root, pidfd: -1} }

func (p *procFS) Close() error {
	if p.pidfd >= 0 {
		unix.Close(p.pidfd)
		p.pidfd = -1
	}
	return p.root.Close()
}

// resolve opens name inside the root, links followed without ever leaving it,
// as an O_PATH handle, then refuses pseudo filesystems before any Reopen
// (spec §5.1 steps 6 and 9). It also returns the handle's fstat.
func (p *procFS) resolve(name string) (*os.File, *unix.Stat_t, error) {
	h, err := pathrs.OpenatInRoot(p.root, name)
	if err != nil {
		return nil, nil, err
	}
	if err := checkNotPseudo(int(h.Fd()), name); err != nil {
		h.Close()
		return nil, nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(h.Fd()), &st); err != nil {
		h.Close()
		return nil, nil, err
	}
	return h, &st, nil
}

func (p *procFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	h, st, err := p.resolve(dir)
	if err != nil {
		return nil, false, err
	}
	defer h.Close()
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, false, fmt.Errorf("%w : %s", ErrNotDir, dir)
	}
	d, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC)
	if err != nil {
		return nil, false, err
	}
	defer d.Close()
	names, err := d.Readdirnames(MaxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(names) > MaxEntries
	if truncated {
		names = names[:MaxEntries]
	}
	// One path component at a time, relative to the directory descriptor,
	// without following anything (spec §5.1 step 7): nothing can leave the root.
	dfd := int(d.Fd())
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dfd, n, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue // removed while listing
		}
		e := entryFromStat(n, &st)
		if e.Type == TypeSymlink {
			buf := make([]byte, 4096)
			if k, err := unix.Readlinkat(dfd, n, buf); err == nil {
				e.LinkTarget = string(buf[:k])
			}
		}
		out = append(out, e)
	}
	return out, truncated, nil
}

func (p *procFS) Stat(_ context.Context, name string) (Entry, error) {
	h, st, err := p.resolve(name)
	if err != nil {
		return Entry{}, err
	}
	h.Close()
	return entryFromStat(path.Base(CleanPath(name)), st), nil
}

func (p *procFS) Open(_ context.Context, name string) (io.ReadCloser, error) {
	h, st, err := p.resolve(name)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	// Regular files only, checked before Reopen: a device node created by
	// mknod in the container would read the host device (spec §5.1 step 6).
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, fmt.Errorf("%w : %s", ErrNotRegular, name)
	}
	// O_NONBLOCK: never block if the file was swapped for a FIFO meanwhile.
	f, err := pathrs.Reopen(h, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC)
	if err != nil {
		return nil, err // never a typed nil *os.File inside the interface
	}
	return f, nil
}

func (p *procFS) StatNoLinks(_ context.Context, name string) (Entry, error) {
	clean := CleanPath(name)
	fd, err := unix.Openat2(int(p.root.Fd()), clean, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ELOOP) {
		return Entry{}, fmt.Errorf("%w : %s", ErrLinkInPath, clean)
	}
	if err != nil {
		return Entry{}, &fs.PathError{Op: "openat2", Path: clean, Err: err}
	}
	defer unix.Close(fd)
	if err := checkNotPseudo(fd, clean); err != nil {
		return Entry{}, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return Entry{}, err
	}
	return entryFromStat(path.Base(clean), &st), nil
}

func entryFromStat(name string, st *unix.Stat_t) Entry {
	e := Entry{
		Name:    name,
		Size:    st.Size,
		Mode:    ModeFromUnix(st.Mode),
		UID:     int(st.Uid),
		GID:     int(st.Gid),
		ModTime: time.Unix(int64(st.Mtim.Sec), int64(st.Mtim.Nsec)).UTC(),
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		e.Type = TypeFile
	case unix.S_IFDIR:
		e.Type = TypeDir
	case unix.S_IFLNK:
		e.Type = TypeSymlink
	default:
		e.Type = TypeOther
	}
	return e
}
```

`internal/fsview/procfs_archive_linux.go` :

```go
//go:build linux

package fsview

import (
	"context"
	"errors"
)

// Fails closed until task 7 (mount checks before a tar) replaces this file:
// no directory download can pass meanwhile.
func (p *procFS) CheckArchive(context.Context, string) error {
	return errors.New("contrôle des montages : pas encore implémenté (tâche 7)")
}
```

- [ ] **Step 3: Vérifier en root**

Run: `go mod tidy && GOOS=linux go vet ./internal/fsview/ && scripts/test-linux.sh ./internal/fsview/ -v 2>&1 | grep -E '^(--- |    --- |ok|FAIL)'`
Expected : `--- PASS` partout, y compris `TestOpenRefusesDevice` et `TestRefusalHappensBeforeReopen/device` (root a `CAP_MKNOD`) ; `ok … internal/fsview`. `go.mod` montre `github.com/cyphar/filepath-securejoin v0.7.0` et `cyphar.com/go-pathrs v0.2.5 // indirect`.

- [ ] **Step 4: Vérifier en non-root avec le détecteur de courses (comme le job CI `go`)**

Run: `docker run --rm --user 1000:1000 -e GOCACHE=/tmp/gocache -e HOME=/tmp -v "$PWD":/src -w /src -v hublot-gomod:/go/pkg/mod:ro hublot-test go test -race ./internal/fsview/ -v 2>&1 | grep -E '^(--- (FAIL|SKIP)|    --- (FAIL|SKIP)|ok)|mknod'`
Expected : `--- SKIP: TestOpenRefusesDevice` et `--- SKIP: TestRefusalHappensBeforeReopen/device`, chacun avec le message `mknod refusé (operation not permitted) : ce test demande CAP_MKNOD…`, puis `ok`.

Run (macOS) : `go test ./internal/...`
Expected : PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fsview go.mod go.sum
git commit -F - <<'EOF'
explorateur: procfs, lecture par pathrs-lite, fichiers réguliers, refus des pseudo-systèmes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 6: `OpenRoot` (pidfd + cgroup), `fsview.For`, contrôle au démarrage

**Où tester :** Linux root (`OpenRoot`) ; macOS (`For`, `main`).

**Files:**
- Create: `internal/fsview/root_linux.go`, `internal/fsview/root_linux_test.go`, `internal/fsview/procfs_other.go`, `internal/fsview/for.go`, `internal/fsview/for_test.go`
- Modify: `cmd/hublot/main.go`, `cmd/hublot/main_test.go`

**Interfaces:**
- Consumes: `CgroupHasID`, `NoteCgroupNamespace`, `changed` (tâche 2) ; `procFS` (tâche 5) ; `dockerx.Client.Inspect`, `dockerx.ContainerInfo` (avec `Mounts`), `dockerx.ErrNotFound`, `dockerx.Fake.Infos` (tâche 3)
- Produces:
  - `func OpenRoot(pid int, containerID string) (*os.File, error)` (Linux) et `openRoot(pid, id) (*os.File, pidfd int, error)` (interne) ;
  - points d'injection de test `var readCgroup func(pid int) ([]byte, error)` et `var afterRootOpen func()`, plus `func SetCgroupReaderForTests(f func(pid int) ([]byte, error)) (restore func())` pour les tests d'autres paquets (tâche 12, revue R1) ;
  - `func For(ctx context.Context, docker dockerx.Client, id string) (FS, error)` et `openProcFS(docker dockerx.Client, info dockerx.ContainerInfo) (FS, error)` (interne, Linux et repli) ;
  - dans `main` : `var selfCgroup func() ([]byte, error)`.

> **À comprendre :** comparer deux `Inspect` ne prouve rien : un PID peut être réattribué entre-temps. Le **pidfd** désigne un process précis : s'il meurt, `pidfd_send_signal(pidfd, 0)` échoue, même si un autre process reprend son numéro.
>
> La séquence suit §5.1, étapes 2 à 5 :
> 1. `pidfd_open` ;
> 2. lire `/proc/<pid>/cgroup` (fichier de l'hôte, pas du conteneur) et appeler `CgroupHasID` ;
> 3. ouvrir `/proc/<pid>/root` en `O_PATH` ;
> 4. `pidfd_send_signal(0)` doit réussir.
>
> - La vue garde le pidfd ouvert : la tâche 7 s'en sert pour lire `mountinfo` du même process.
> - Deux points d'injection rendent l'étape 4 testable sans Docker (revue I4). `readCgroup` donne au propre enfant du test (`sleep`) le cgroup d'un conteneur. `afterRootOpen` tue cet enfant, puis le récolte, juste après l'ouverture de sa racine : son PID est alors libre, et seul le pidfd peut le détecter.
> - Un `EACCES` à l'ouverture a deux causes mesurées par le spike (§8) : le message les nomme.
> - Au démarrage, `0::/` dans `/proc/self/cgroup` signale un espace de noms privé. L'agent affiche un avertissement, et toute erreur `container_changed` porte ensuite l'indice `--cgroupns=host`.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/fsview/root_linux_test.go` :

```go
//go:build linux

package fsview

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOpenRootRejectsForeignCgroup(t *testing.T) {
	_, err := OpenRoot(os.Getpid(), strings.Repeat("a", 64))
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

func TestOpenRootRejectsMissingPid(t *testing.T) {
	_, err := OpenRoot(0x7ffffffe, strings.Repeat("a", 64))
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

func TestOpenRootRejectsShortID(t *testing.T) {
	_, err := OpenRoot(os.Getpid(), "abc")
	if !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu ErrContainerChanged, reçu %v", err)
	}
}

// Spec §5.1 step 5: a process that dies between the open of its root and the
// liveness check must be detected (its PID may already belong to another
// process). The test pins one of its own children, gives it the container's
// cgroup through the readCgroup seam, and kills it at the afterRootOpen seam.
func TestOpenRootDetectsDeadProcess(t *testing.T) {
	id := strings.Repeat("ab", 32)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	oldCgroup, oldHook := readCgroup, afterRootOpen
	t.Cleanup(func() { readCgroup, afterRootOpen = oldCgroup, oldHook })
	readCgroup = func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil }

	root, err := OpenRoot(cmd.Process.Pid, id) // control: alive, the sequence succeeds
	if err != nil {
		t.Fatalf("contrôle : %v", err)
	}
	root.Close()

	afterRootOpen = func() { cmd.Process.Kill(); cmd.Wait() } // dead and reaped: its PID is free
	if _, err := OpenRoot(cmd.Process.Pid, id); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("process mort après l'ouverture de sa racine : %v, attendu ErrContainerChanged", err)
	}
}
```

`internal/fsview/for_test.go` :

```go
package fsview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestForStoppedContainer(t *testing.T) {
	id := strings.Repeat("ab", 32)
	f := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{
		"db":     {ID: id, Name: "db"},
		"zombie": {ID: id, Name: "zombie", Running: true}, // running without a pid
	}}
	for _, name := range []string{"db", "zombie"} {
		if _, err := For(context.Background(), f, name); !errors.Is(err, ErrStopped) {
			t.Errorf("%s : %v, attendu ErrStopped", name, err)
		}
	}
}

func TestForPassesDockerErrors(t *testing.T) {
	if _, err := For(context.Background(), &dockerx.Fake{}, "nope"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("inconnu : %v", err)
	}
	down := &dockerx.Fake{Err: fmt.Errorf("%w : dial", dockerx.ErrUnavailable)}
	if _, err := For(context.Background(), down, "web"); !errors.Is(err, dockerx.ErrUnavailable) {
		t.Fatalf("Docker arrêté : %v", err)
	}
}
```

Dans `cmd/hublot/main_test.go`, ajouter l'import `"github.com/levy-cmd/hublot-kit/internal/fsview"` (bloc séparé après `"testing"`), puis à la fin :

```go
func TestRunWarnsPrivateCgroupNamespace(t *testing.T) {
	old := selfCgroup
	selfCgroup = func() ([]byte, error) { return []byte("0::/\n"), nil }
	t.Cleanup(func() { selfCgroup = old; fsview.NoteCgroupNamespace("") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", t.TempDir(), "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(errb.String(), "--cgroupns=host") {
		t.Fatalf("avertissement absent : %q", errb.String())
	}
}

func TestRunNoWarningOnHostCgroup(t *testing.T) {
	old := selfCgroup
	selfCgroup = func() ([]byte, error) { return []byte("0::/system.slice/hublot.service\n"), nil }
	t.Cleanup(func() { selfCgroup = old; fsview.NoteCgroupNamespace("") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, []string{"--data-dir", t.TempDir(), "--listen", "127.0.0.1:0"}, noenv, &out, &errb); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb.String())
	}
	if strings.Contains(errb.String(), "cgroupns") {
		t.Fatalf("avertissement inattendu : %q", errb.String())
	}
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/fsview/ ./cmd/hublot/ ; GOOS=linux go vet ./internal/fsview/`
Expected : FAIL à la compilation (`undefined: For`, `undefined: selfCgroup`, puis sous Linux `undefined: OpenRoot`, `undefined: readCgroup`).

- [ ] **Step 3: Écrire le code**

`internal/fsview/root_linux.go` :

```go
//go:build linux

package fsview

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Test seams of spec §5.1 steps 3 and 5: tests give a fake cgroup content to
// a process of their own, and kill it between the open of its root and the
// liveness check.
var (
	readCgroup = func(pid int) ([]byte, error) {
		return os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid)) // a host file, not a container path
	}
	afterRootOpen = func() {}
)

// SetCgroupReaderForTests replaces the reading of /proc/<pid>/cgroup and
// returns the function that restores it. Tests of other packages (api) use it
// to drive a real procfs view of one of their own processes through the real
// wiring; production code never calls it.
// It panics outside a test binary. Tests that use it must not call t.Parallel.
func SetCgroupReaderForTests(f func(pid int) ([]byte, error)) (restore func()) {
	if !testing.Testing() {
		panic("fsview.SetCgroupReaderForTests : réservé aux tests")
	}
	old := readCgroup
	readCgroup = f
	return func() { readCgroup = old }
}

// openRoot runs spec §5.1 steps 2 to 5 and returns an O_PATH descriptor on the
// root directory of the container whose main process is pid, and the pidfd
// that pins this process. The caller closes both.
func openRoot(pid int, containerID string) (*os.File, int, error) {
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return nil, -1, changed("pidfd_open(%d) : %v", pid, err)
	}
	fail := func(err error) (*os.File, int, error) {
		unix.Close(pidfd)
		return nil, -1, err
	}
	cg, err := readCgroup(pid)
	if err != nil {
		return fail(changed("lecture du cgroup du pid %d : %v", pid, err))
	}
	if !CgroupHasID(string(cg), containerID) {
		return fail(changed("le pid %d n'est pas dans le cgroup du conteneur %.12s", pid, containerID))
	}
	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	switch {
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ESRCH):
		return fail(changed("ouverture de /proc/%d/root : %v", pid, err))
	case errors.Is(err, unix.EACCES):
		return fail(fmt.Errorf("ouverture de /proc/%d/root : %w ; causes mesurées (spec §8) : "+
			"--cap-add SYS_PTRACE absent, ou cible non confinée (--privileged, apparmor=unconfined) "+
			"sans --security-opt apparmor=unconfined sur l'agent", pid, err))
	case err != nil:
		return fail(fmt.Errorf("ouverture de /proc/%d/root : %w", pid, err))
	}
	afterRootOpen()
	// Still alive: the PID was not reused between pidfd_open and the open above,
	// so fd is the root of the process checked at step 3.
	if err := unix.PidfdSendSignal(pidfd, 0, nil, 0); err != nil {
		unix.Close(fd)
		return fail(changed("le process %d a disparu : %v", pid, err))
	}
	return os.NewFile(uintptr(fd), "container-root"), pidfd, nil
}

// OpenRoot is openRoot without the pidfd.
func OpenRoot(pid int, containerID string) (*os.File, error) {
	root, pidfd, err := openRoot(pid, containerID)
	if err != nil {
		return nil, err
	}
	unix.Close(pidfd)
	return root, nil
}

func openProcFS(docker dockerx.Client, info dockerx.ContainerInfo) (FS, error) {
	root, pidfd, err := openRoot(info.Pid, info.ID)
	if err != nil {
		return nil, err
	}
	return &procFS{root: root, pid: info.Pid, pidfd: pidfd, docker: docker, mounts: info.Mounts}, nil
}
```

`internal/fsview/procfs_other.go` :

```go
//go:build !linux

package fsview

import (
	"errors"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// The agent reads /proc/<pid>/root: Linux only. On macOS, run it in a
// container (scripts/dev-agent.sh); the tests that need Linux run through
// scripts/test-linux.sh.
func openProcFS(dockerx.Client, dockerx.ContainerInfo) (FS, error) {
	return nil, errors.New("explorateur : Linux uniquement (sur macOS, lancez l'agent dans un conteneur)")
}
```

`internal/fsview/for.go` :

```go
package fsview

import (
	"context"
	"fmt"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// For returns the view of a container's files (spec §4): procfs while it runs.
// id may be a name or a short ID. Errors of docker.Inspect pass through
// (dockerx.ErrNotFound, dockerx.ErrUnavailable).
func For(ctx context.Context, docker dockerx.Client, id string) (FS, error) {
	info, err := docker.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if !info.Running || info.Pid == 0 {
		return nil, fmt.Errorf("%w : %s", ErrStopped, info.Name)
	}
	return openProcFS(docker, info)
}
```

`cmd/hublot/main.go` : ajouter l'import `"github.com/levy-cmd/hublot-kit/internal/fsview"`, puis sous `var version = …` :

```go
// selfCgroup reads the agent's own cgroup for the startup check of spec §8;
// tests replace it. Outside Linux the read fails and the check is skipped.
var selfCgroup = func() ([]byte, error) { return os.ReadFile("/proc/self/cgroup") }
```

et juste après le bloc `docker, err := dockerx.New() … return fail("client Docker", err) }` :

```go
	if b, err := selfCgroup(); err == nil && fsview.NoteCgroupNamespace(string(b)) {
		fmt.Fprintln(stderr, "ATTENTION : espace de noms cgroup privé (/proc/self/cgroup = 0::/) : l'explorateur répondra container_changed. Lancez l'agent avec --cgroupns=host.")
	}
```

- [ ] **Step 4: Vérifier**

Run (macOS) : `go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/...`
Expected : PASS.

Run : `GOOS=linux go vet ./cmd/... ./internal/... && scripts/test-linux.sh ./cmd/... ./internal/... && scripts/test-linux.sh ./internal/fsview/ -run OpenRoot -v 2>&1 | grep -E '^--- '`
Expected : `ok` pour chaque paquet, et 4 tests `TestOpenRoot…` en `PASS`, dont `TestOpenRootDetectsDeadProcess`.

- [ ] **Step 5: Commit**

```bash
git add internal/fsview cmd/hublot
git commit -F - <<'EOF'
explorateur: ouverture de la racine par pidfd et cgroup, fsview.For, alerte cgroupns au démarrage

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 7: Contrôle des montages avant le tar d'un dossier (revue C1)

**Où tester :** macOS (règles pures, faux client) ; Linux root et non root (un process de test joue le conteneur). **Jamais** contre un vrai dockerd avec un bind de `/proc` : le scénario a verrouillé dockerd en revue.

**Files:**
- Create: `internal/fsview/mounts.go`, `internal/fsview/mounts_test.go`, `internal/fsview/procfs_archive_linux_test.go`
- Modify: `internal/fsview/procfs_archive_linux.go` (remplace la garde fermée)

**Interfaces:**
- Consumes: `procFS` et ses champs (tâche 5) ; `openRoot`, `openProcFS`, `For`, `SetCgroupReaderForTests` (tâche 6) ; `dockerx.Mount`, `dockerx.Client.VolumeOptions`, `dockerx.Fake.Volumes` (tâche 3) ; `ErrForbiddenPath`, `changed`, `CleanPath`
- Produces:
  - `func HostBind(ctx context.Context, docker dockerx.Client, mounts []dockerx.Mount) error` ;
  - `func PseudoMountUnder(mountinfo, dir string) (mountPoint, fsType string, found bool)` ;
  - `func PseudoMountInMounts(mountinfo string, mounts []dockerx.Mount) (mountPoint, fsType, destination string, found bool)` ;
  - `(*procFS) CheckArchive(ctx, dir) error` ;
  - `var readMountinfo func(pid int) ([]byte, error)` (interne, point d'injection de test).

> **À comprendre :** Docker construit `GET /archive` d'un dossier avec les binds et les volumes du conteneur montés dedans. En revue, le tar de `/host`, sur un conteneur lancé avec `-v /proc:/host/proc`, a lu `/proc/kcore` de l'hôte (254 Tio) : dockerd est resté bloqué et le conteneur verrouillé. Nos contrôles de §5.1 ne voient rien de cela : c'est **Docker** qui lit, pas nous. D'où deux contrôles avant tout `GetArchive` de dossier.
>
> - **(a) `HostBind` : la règle de §5.2 étape 7.** Elle porte sur `Inspect.Mounts` : un bind dont la source est `/`, `/proc`, `/sys` ou en dessous, ou exactement `/dev`, est refusé.
>   - Un volume `local` dont les options `o` contiennent `bind` ou `rbind`, avec un `device=…`, est un bind déguisé : sa source est le `device`.
>   - Un volume `local` dont l'option `type` est un pseudo-système ou FUSE est refusé (revue R2).
>   - Tout volume d'un autre pilote est refusé : on ne sait pas ce qu'il monte, donc fermé par défaut (revue R2).
>   - Des options illisibles font refuser par `ErrForbiddenPath`, avec le nom du volume et sa destination (revue R4).
>   - Une source de bind (ou un `device`) qui n'est pas un chemin absolu ne peut pas être jugée : elle est refusée (échec fermé, troisième tour T3).
>   - Le refus vaut pour tout dossier du conteneur, comme pour l'explorateur d'un conteneur arrêté.
>   - Ces extensions (pilote tiers, options illisibles, `type=`) ne s'appliquent pour l'instant qu'au tar d'un conteneur actif ; leur portée pour un conteneur arrêté est la question Q9 (plan 2).
> - **(b) `/proc/<pid>/mountinfo`.** Ce fichier de l'hôte est lu pour le process épinglé par le pidfd, qui doit être vivant *après* la lecture. Ses points de montage sont relatifs à la racine du conteneur. On y cherche les montages de l'un des 15 pseudo-systèmes ou FUSE (`fuse`, `fuseblk`, `fuse.*`) : un serveur FUSE muet bloquerait dockerd comme `/proc` (§13). Deux règles s'appliquent :
>   - **pour tout le conteneur** (`PseudoMountInMounts`, revue R2) : un tel montage sur ou sous la destination d'un montage d'`Inspect.Mounts` refuse tout tar, car dockerd remonte ces montages pour tout tar du conteneur ;
>   - **par dossier** (`PseudoMountUnder`) : un tel montage sur, sous ou au-dessus du dossier refuse ce tar.
>
>   Ce contrôle voit les sous-montages d'un `rbind` présents dans le conteneur (`-v /:/host` montre `/host/proc`), que `Inspect.Mounts` ne montre pas. Il a une limite : avec la propagation `rprivate` par défaut, un montage fait **plus tard** sur l'hôte sous la source d'un bind n'apparaît pas dans le conteneur, alors que dockerd pourrait le voir en remontant le bind. Cette limite est écrite au §13 (tâche 12). Une ligne illisible, ou un échappement invalide, fait refuser (revue R5 ; les montages empilés au même point comptent tous).
> - **Course entre le contrôle et `GetArchive`** (revue R3) : c'est une limite assumée, écrite au §5.4 et au §13 à la tâche 12. Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1.
>
> Conséquence : le `/` d'un conteneur actif est toujours refusé, car son propre `/proc` est dessous (voir Q8). Les tests n'utilisent que le faux client et un process `sleep` du test, jamais un vrai bind de `/proc`. Ils prouvent aussi le câblage (revue R1) : `TestForWiresMountsIntoCheckArchive` fait passer les montages d'`Inspect` par `For` jusqu'à `CheckArchive`, et `TestCheckArchiveDeadProcess` tue l'enfant juste après la lecture de son `mountinfo`.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/fsview/mounts_test.go` :

```go
package fsview

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestHostBind(t *testing.T) {
	ctx := context.Background()
	docker := &dockerx.Fake{Volumes: map[string]map[string]string{
		"proc-vol":  {"type": "none", "o": "bind", "device": "/proc"},
		"sys-vol":   {"o": "rbind,ro", "device": "/sys/kernel"},
		"proc-type": {"type": "proc", "device": "proc"},
		"sshfs":     {"type": "fuse.sshfs", "device": "host:/"},
		"plain-vol": {},
		"srv-vol":   {"type": "none", "o": "bind", "device": "/srv/data"},
		"nfs-vol":   {"type": "nfs", "o": "addr=10.0.0.1", "device": ":/export"},
		"rel-vol":   {"type": "none", "o": "bind", "device": "proc"},
		// Readable options: only the driver rule may refuse these two.
		"ceph": {},
		"anon": {},
	}}
	bind := func(src, dst string) dockerx.Mount { return dockerx.Mount{Type: "bind", Source: src, Destination: dst} }
	vol := func(name, dst, driver string) dockerx.Mount {
		return dockerx.Mount{Type: "volume", Name: name, Destination: dst, Driver: driver}
	}
	refused := []struct {
		m    dockerx.Mount
		want []string // the error names the mount
	}{
		{bind("/", "/host"), []string{"bind / -> /host"}},
		{bind("/proc", "/host/proc"), []string{"bind /proc -> /host/proc"}},
		{bind("/proc/1/ns", "/ns"), []string{"bind /proc/1/ns -> /ns"}},
		{bind("/sys", "/host/sys"), []string{"bind /sys -> /host/sys"}},
		{bind("/sys/fs/cgroup", "/cg"), []string{"bind /sys/fs/cgroup -> /cg"}},
		{bind("/dev", "/host/dev"), []string{"bind /dev -> /host/dev"}},
		{bind("/proc/", "/p"), []string{"bind /proc/ -> /p"}},
		{vol("proc-vol", "/data", "local"), []string{"bind /proc -> /data", "proc-vol"}},
		{vol("sys-vol", "/data", "local"), []string{"bind /sys/kernel -> /data", "sys-vol"}},
		{vol("proc-type", "/p", "local"), []string{"proc-type", "type=proc", "/p"}},
		{vol("sshfs", "/remote", "local"), []string{"sshfs", "type=fuse.sshfs", "/remote"}},
		{vol("ceph", "/c", "rexray/rbd"), []string{"ceph", "/c", "rexray/rbd"}},
		{vol("anon", "/a", ""), []string{"anon", "/a"}},
		{vol("gone", "/g", "local"), []string{"gone", "/g", "options illisibles"}},
		{bind("proc", "/rel"), []string{"bind proc -> /rel"}},                   // not absolute: fail closed
		{vol("rel-vol", "/r", "local"), []string{"bind proc -> /r", "rel-vol"}}, // idem for a device
	}
	for _, c := range refused {
		err := HostBind(ctx, docker, []dockerx.Mount{bind("/srv", "/srv"), c.m})
		if !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%+v : %v, attendu ErrForbiddenPath", c.m, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%+v : %q ne nomme pas %q", c.m, err, w)
			}
		}
	}
	allowed := []dockerx.Mount{
		bind("/srv/data", "/data"), bind("/dev/log", "/dev/log"), bind("/dev/null", "/x"),
		bind("/procfs-backup", "/b"), bind("/system", "/s"),
		vol("plain-vol", "/v", "local"), vol("srv-vol", "/w", "local"), vol("nfs-vol", "/n", "local"),
		{Type: "tmpfs", Destination: "/tmp"},
	}
	if err := HostBind(ctx, docker, allowed); err != nil {
		t.Fatalf("montages ordinaires refusés : %v", err)
	}
	if err := HostBind(ctx, nil, []dockerx.Mount{vol("plain-vol", "/v", "local")}); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("sans client Docker, un volume doit être refusé : %v", err)
	}
}

const sampleMountinfo = `600 500 0:50 / / rw,relatime master:1 - overlay overlay rw,lowerdir=/l
601 600 0:52 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
602 600 0:53 / /dev rw,nosuid - tmpfs tmpfs rw,size=65536k,mode=755
603 602 0:54 / /dev/pts rw,nosuid,noexec,relatime - devpts devpts rw,gid=5,mode=620
604 600 0:55 / /sys ro,nosuid,nodev,noexec,relatime - sysfs sysfs ro
605 604 0:26 / /sys/fs/cgroup ro,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw
606 600 254:1 /docker/volumes/x/_data /data rw,relatime - ext4 /dev/vda1 rw
607 606 0:60 / /data/remote rw,nosuid,nodev,relatime - fuse.sshfs host:/ rw,user_id=0
608 600 0:61 / /host/proc rw,relatime - proc proc rw
609 600 254:1 /srv /my\040dir rw,relatime - ext4 /dev/vda1 rw
610 609 0:62 / /my\040dir/sys rw,relatime - sysfs sysfs rw
`

func TestPseudoMountUnder(t *testing.T) {
	cases := []struct {
		dir, mountPoint, fsType string
		found                   bool
	}{
		{"/", "/proc", "proc", true},
		{"/etc", "", "", false},
		{"/dev", "", "", false}, // tmpfs, devpts: harmless
		{"/sys", "/sys", "sysfs", true},
		{"/sys/fs", "/sys", "sysfs", true}, // below a pseudo mount
		{"/data", "/data/remote", "fuse.sshfs", true},
		{"/data/other", "", "", false},
		{"/host", "/host/proc", "proc", true},
		{"/host/proc/1", "/host/proc", "proc", true},
		{"/hostfoo", "", "", false},
		{"/my dir", "/my dir/sys", "sysfs", true},
		{"/proc/../etc", "", "", false}, // cleaned: /etc
	}
	for _, c := range cases {
		mp, typ, found := PseudoMountUnder(sampleMountinfo, c.dir)
		if found != c.found || mp != c.mountPoint || typ != c.fsType {
			t.Errorf("%q : (%q, %q, %v), attendu (%q, %q, %v)", c.dir, mp, typ, found, c.mountPoint, c.fsType, c.found)
		}
	}
	if _, _, found := PseudoMountUnder("garbage line\n", "/etc"); !found {
		t.Fatal("une ligne illisible doit refuser (échec fermé)")
	}
	if _, _, found := PseudoMountUnder("700 600 0:70 / /mnt rw - fuseblk /dev/sdb1 rw\n", "/"); !found {
		t.Fatal("fuseblk sous / doit refuser")
	}
}

// Review R5: stacked mounts, propagation fields, escapes.
func TestPseudoMountUnderEdgeCases(t *testing.T) {
	const mi = `611 600 254:1 / /stack rw,relatime - ext4 /dev/vda1 rw
612 611 0:63 / /stack rw,relatime - proc proc rw
613 612 0:64 / /stack rw,relatime - tmpfs tmpfs rw
614 600 0:65 / /prop rw,relatime shared:5 master:1 propagate_from:2 - sysfs sysfs rw
615 600 0:66 / /t\011ab rw - proc proc rw
616 600 0:67 / /n\012l rw - proc proc rw
617 600 0:68 / /b\134s rw - proc proc rw
`
	cases := []struct{ dir, mountPoint, fsType string }{
		{"/stack", "/stack", "proc"}, // hidden under a tmpfs: still counted
		{"/prop", "/prop", "sysfs"},  // optional fields before "-"
		{"/t\tab", "/t\tab", "proc"},
		{"/n\nl", "/n\nl", "proc"},
		{"/b\\s", "/b\\s", "proc"},
	}
	for _, c := range cases {
		mp, typ, found := PseudoMountUnder(mi, c.dir)
		if !found || mp != c.mountPoint || typ != c.fsType {
			t.Errorf("%q : (%q, %q, %v), attendu (%q, %q, true)", c.dir, mp, typ, found, c.mountPoint, c.fsType)
		}
	}
	for _, bad := range []string{`/bad\08x`, `/bad\9`, `/end\`, `/short\04`} {
		line := "700 600 254:1 / " + bad + " rw - ext4 /dev/vda1 rw\n"
		if _, typ, found := PseudoMountUnder(line, "/etc"); !found || !strings.Contains(typ, "échappement") {
			t.Errorf("%q : échappement invalide accepté (%q, %v)", bad, typ, found)
		}
	}
}

// Review R2: a dangerous mount under a Docker mount refuses the whole container.
func TestPseudoMountInMounts(t *testing.T) {
	m := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "bind", Source: "/srv", Destination: dst} }
	cases := []struct {
		mounts                []dockerx.Mount
		mountPoint, fsType, d string
		found                 bool
	}{
		{[]dockerx.Mount{m("/etc/app")}, "", "", "", false},
		{[]dockerx.Mount{m("/etc/app"), m("/host")}, "/host/proc", "proc", "/host", true},
		{[]dockerx.Mount{m("/data/")}, "/data/remote", "fuse.sshfs", "/data", true},
		{[]dockerx.Mount{m("/host/proc")}, "/host/proc", "proc", "/host/proc", true},
		{[]dockerx.Mount{m("/hostfoo")}, "", "", "", false},
		{nil, "", "", "", false}, // the container's own /proc is not under any Docker mount
	}
	for _, c := range cases {
		mp, typ, d, found := PseudoMountInMounts(sampleMountinfo, c.mounts)
		if found != c.found || mp != c.mountPoint || typ != c.fsType || d != c.d {
			t.Errorf("%+v : (%q, %q, %q, %v)", c.mounts, mp, typ, d, found)
		}
	}
}
```

`internal/fsview/procfs_archive_linux_test.go` :

```go
//go:build linux

package fsview

// CheckArchive on a real process of the test machine (no Docker): one of the
// test's own children plays the container. Its mount table is the test
// machine's, where /proc and /sys are pseudo filesystems.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func startChild(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	must(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd
}

func childView(t *testing.T) (*procFS, *exec.Cmd) {
	t.Helper()
	cmd := startChild(t)
	pid := cmd.Process.Pid
	pidfd, err := unix.PidfdOpen(pid, 0)
	must(t, err)
	fd, err := unix.Open(fmt.Sprintf("/proc/%d/root", pid), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	must(t, err)
	p := &procFS{root: os.NewFile(uintptr(fd), "root"), pid: pid, pidfd: pidfd, docker: &dockerx.Fake{}}
	t.Cleanup(func() { p.Close() })
	return p, cmd
}

func TestCheckArchiveMountTable(t *testing.T) {
	p, _ := childView(t)
	for _, dir := range []string{"/", "/proc", "/proc/1", "/sys", "/sys/fs"} {
		if err := p.CheckArchive(ctx, dir); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", dir, err)
		}
	}
	if err := p.CheckArchive(ctx, "/etc"); err != nil {
		t.Errorf("/etc : %v", err)
	}
}

func TestCheckArchiveHostBind(t *testing.T) {
	p, _ := childView(t)
	p.mounts = []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}}
	err := p.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Fatalf("bind de /proc : %v", err)
	}
}

// Review R2: a pseudo filesystem under a Docker mount refuses every directory,
// not only those that overlap it. Here the test machine's /sys plays a mount
// found under the destination of a harmless Docker mount.
func TestCheckArchiveContainerWideRule(t *testing.T) {
	p, _ := childView(t)
	p.mounts = []dockerx.Mount{{Type: "tmpfs", Destination: "/sys"}}
	err := p.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "sous le montage /sys") {
		t.Fatalf("montage dangereux sous un montage Docker : %v", err)
	}
}

// Review R1 (ii): the mount table read must belong to the pinned process. The
// child dies right after its table is read: CheckArchive must notice.
func TestCheckArchiveDeadProcess(t *testing.T) {
	p, cmd := childView(t)
	old := readMountinfo
	t.Cleanup(func() { readMountinfo = old })
	readPid := 0
	readMountinfo = func(pid int) ([]byte, error) {
		readPid = pid
		b, err := old(pid)
		cmd.Process.Kill()
		cmd.Wait() // reaped: its PID is free
		return b, err
	}
	if err := p.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("process mort après la lecture de mountinfo : %v, attendu ErrContainerChanged", err)
	}
	if readPid != p.pid {
		t.Fatalf("mountinfo lu pour le pid %d, pas pour le process épinglé %d", readPid, p.pid)
	}
}

func TestCheckArchiveWithoutProcessFailsClosed(t *testing.T) {
	fsys, _, _ := newRoot(t)
	if err := fsys.CheckArchive(ctx, "/"); err == nil {
		t.Fatal("une vue sans process ne peut pas autoriser un tar")
	}
}

// Review R1 (i): the mounts of Inspect reach the view built by For.
func TestForWiresMountsIntoCheckArchive(t *testing.T) {
	cmd := startChild(t)
	id := strings.Repeat("ab", 32)
	restore := SetCgroupReaderForTests(func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil })
	t.Cleanup(restore)
	docker := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{"c": {
		ID: id, Name: "c", Running: true, Pid: cmd.Process.Pid,
		Mounts: []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}},
	}}}
	fsys, err := For(context.Background(), docker, "c")
	must(t, err)
	defer fsys.Close()
	err = fsys.CheckArchive(ctx, "/etc")
	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Fatalf("les montages d'Inspect doivent atteindre la vue : %v", err)
	}
}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go test ./internal/fsview/`
Expected : FAIL à la compilation (`undefined: HostBind`, `undefined: PseudoMountUnder`).

- [ ] **Step 3: Écrire le code**

`internal/fsview/mounts.go` :

```go
package fsview

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Docker builds GET /archive of a directory with the container's binds and
// volumes mounted in it: a bind of the host's /proc puts /proc/kcore (hundreds
// of TiB) in the tar and locks dockerd (measured in review). Checks run before
// every GetArchive of a directory (spec §5.2 step 7, §5.4).

// HostBind refuses a container whose mounts may expose the host to the tar
// (spec §5.2 step 7), with an ErrForbiddenPath that names the mount:
//   - a bind of the host's /, /proc, /sys or a path below them, or of /dev
//     exactly;
//   - a "local" volume with o=bind/rbind and device=… (a bind in disguise,
//     judged by its device), or whose type=… is a pseudo filesystem or FUSE;
//   - a volume of any other driver (unknown: refused, fail closed);
//   - a volume whose options cannot be read (fail closed).
func HostBind(ctx context.Context, docker dockerx.Client, mounts []dockerx.Mount) error {
	for _, m := range mounts {
		switch m.Type {
		case "bind":
			if hostSensitive(m.Source) {
				return fmt.Errorf("%w : bind %s -> %s", ErrForbiddenPath, m.Source, m.Destination)
			}
		case "volume":
			if m.Driver != "local" {
				return fmt.Errorf("%w : volume %s -> %s : pilote %q inconnu, refusé", ErrForbiddenPath, m.Name, m.Destination, m.Driver)
			}
			if docker == nil {
				return fmt.Errorf("%w : volume %s -> %s : options illisibles (pas de client Docker)", ErrForbiddenPath, m.Name, m.Destination)
			}
			opts, err := docker.VolumeOptions(ctx, m.Name)
			if err != nil {
				return fmt.Errorf("%w : volume %s -> %s : options illisibles (%v)", ErrForbiddenPath, m.Name, m.Destination, err)
			}
			if t := opts["type"]; dangerousFSType(t) {
				return fmt.Errorf("%w : volume %s (type=%s) -> %s", ErrForbiddenPath, m.Name, t, m.Destination)
			}
			if hasBindOption(opts["o"]) && opts["device"] != "" && hostSensitive(opts["device"]) {
				return fmt.Errorf("%w : bind %s -> %s (volume %s)", ErrForbiddenPath, opts["device"], m.Destination, m.Name)
			}
		}
	}
	return nil
}

func hasBindOption(o string) bool {
	for _, opt := range strings.Split(o, ",") {
		if opt = strings.TrimSpace(opt); opt == "bind" || opt == "rbind" {
			return true
		}
	}
	return false
}

// hostSensitive works on a host path string (never opened). A path that is
// not absolute cannot be judged: refused (fail closed).
func hostSensitive(src string) bool {
	if !path.IsAbs(src) {
		return true
	}
	s := path.Clean(src)
	return s == "/" || s == "/dev" ||
		s == "/proc" || strings.HasPrefix(s, "/proc/") ||
		s == "/sys" || strings.HasPrefix(s, "/sys/")
}

// pseudoFSNames are the mountinfo names of the 15 pseudo filesystems of
// spec §5.1 step 9 (pseudofs_linux.go lists their magic numbers).
var pseudoFSNames = []string{
	"proc", "sysfs", "cgroup", "cgroup2", "debugfs", "tracefs", "securityfs", "bpf",
	"pstore", "efivarfs", "binfmt_misc", "selinuxfs", "smackfs", "configfs", "fusectl",
}

// dangerousFSType: a pseudo filesystem, or FUSE (a server that never answers
// would block dockerd while it builds the tar, spec §13).
func dangerousFSType(t string) bool {
	return slices.Contains(pseudoFSNames, t) || t == "fuse" || t == "fuseblk" || strings.HasPrefix(t, "fuse.")
}

// dangerousMount returns the first mount of /proc/<pid>/mountinfo (mount
// points relative to the process's root) of a dangerous type whose mount point
// matches. Every line is read, stacked mounts included. A line that cannot be
// parsed, or with an invalid escape, counts as dangerous: fail closed.
func dangerousMount(mountinfo string, match func(mountPoint string) bool) (mountPoint, fsType string, found bool) {
	for _, line := range strings.Split(mountinfo, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// id parent major:minor root mount-point options [optional fields…] - type source super-options
		f := strings.Fields(line)
		sep := slices.Index(f, "-")
		if len(f) < 5 || sep < 5 || sep+1 >= len(f) {
			return line, "ligne de mountinfo illisible", true
		}
		mp, ok := unescapeMountinfo(f[4])
		if !ok {
			return f[4], "échappement de mountinfo invalide", true
		}
		if typ := f[sep+1]; dangerousFSType(typ) && match(mp) {
			return mp, typ, true
		}
	}
	return "", "", false
}

// PseudoMountUnder: a dangerous mount on, under or above dir (spec §5.4).
func PseudoMountUnder(mountinfo, dir string) (mountPoint, fsType string, found bool) {
	dir = CleanPath(dir)
	return dangerousMount(mountinfo, func(mp string) bool {
		return mp == dir || isUnder(mp, dir) || isUnder(dir, mp)
	})
}

// PseudoMountInMounts: a dangerous mount on or under the destination of one of
// the container's Docker mounts. dockerd mounts those again to build any tar of
// the container, so such a mount refuses every directory (review R2).
func PseudoMountInMounts(mountinfo string, mounts []dockerx.Mount) (mountPoint, fsType, destination string, found bool) {
	for _, m := range mounts {
		dst := CleanPath(m.Destination)
		if mp, typ, ok := dangerousMount(mountinfo, func(mp string) bool { return mp == dst || isUnder(mp, dst) }); ok {
			return mp, typ, dst, true
		}
	}
	return "", "", "", false
}

func isUnder(p, parent string) bool {
	return parent == "/" || strings.HasPrefix(p, parent+"/")
}

// unescapeMountinfo decodes the octal escapes of mountinfo (\040 space,
// \011 tab, \012 newline, \134 backslash). A backslash not followed by three
// octal digits is invalid: ok is false.
func unescapeMountinfo(s string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+4 > len(s) {
			return "", false
		}
		n, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if err != nil {
			return "", false
		}
		b.WriteByte(byte(n))
		i += 3
	}
	return b.String(), true
}
```

`internal/fsview/procfs_archive_linux.go` (remplace la garde fermée de la tâche 5) :

```go
//go:build linux

package fsview

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// readMountinfo reads a host file: /proc/<pid>/mountinfo of the process pinned
// by the view's pidfd (its mount points are relative to the container root).
// Tests replace it.
var readMountinfo = func(pid int) ([]byte, error) {
	return os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid))
}

// CheckArchive (spec §5.2 step 7, §5.4), in this order: the container's Docker
// mounts (HostBind); its mount table, read for the very process the pidfd pins
// and still alive afterwards; no dangerous mount under any Docker mount (the
// whole container); none on, under or above dir.
func (p *procFS) CheckArchive(ctx context.Context, dir string) error {
	if p.pidfd < 0 {
		return errors.New("contrôle des montages impossible : vue sans process")
	}
	if err := HostBind(ctx, p.docker, p.mounts); err != nil {
		return err
	}
	mi, err := readMountinfo(p.pid)
	if err != nil {
		return changed("lecture de /proc/%d/mountinfo : %v", p.pid, err)
	}
	if err := unix.PidfdSendSignal(p.pidfd, 0, nil, 0); err != nil {
		return changed("le process %d a disparu : %v", p.pid, err)
	}
	if mp, typ, dst, bad := PseudoMountInMounts(string(mi), p.mounts); bad {
		return fmt.Errorf("%w : %s monté sur %s, sous le montage %s du conteneur : aucun tar", ErrForbiddenPath, typ, mp, dst)
	}
	if mp, typ, bad := PseudoMountUnder(string(mi), dir); bad {
		return fmt.Errorf("%w : %s monté sur %s, le tar de %s l'inclurait", ErrForbiddenPath, typ, mp, CleanPath(dir))
	}
	return nil
}
```

- [ ] **Step 4: Vérifier**

Run (macOS) : `go vet ./internal/... && go test ./internal/fsview/ -run 'HostBind|PseudoMount' -v`
Expected : PASS (4 tests : `TestHostBind`, `TestPseudoMountUnder`, `TestPseudoMountUnderEdgeCases`, `TestPseudoMountInMounts`).

Run : `GOOS=linux go vet ./internal/fsview/ && scripts/test-linux.sh ./internal/fsview/ -run 'CheckArchive|ForWires' -v 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected : 6 tests en `PASS` (5 `TestCheckArchive…` et `TestForWiresMountsIntoCheckArchive`). Dans le conteneur de test, `/`, `/proc`, `/proc/1`, `/sys` et `/sys/fs` sont refusés, `/etc` est accepté.

Mutations à rejouer (chacune doit faire échouer au moins un test, puis on restaure) : retirer `mounts: info.Mounts` dans `openProcFS` ; mettre `&& false` derrière le `pidfd_send_signal` de `CheckArchive` ; passer `nil` à `PseudoMountInMounts` ; renvoyer l'erreur de `VolumeOptions` telle quelle ; accepter un pilote autre que `local` ; ignorer `type=` ; accepter un échappement invalide ; accepter une source non absolue (retirer le test `!path.IsAbs`).

Run (non root, comme la CI) : `docker run --rm --user 1000:1000 -e GOCACHE=/tmp/gocache -e HOME=/tmp -v "$PWD":/src -w /src -v hublot-gomod:/go/pkg/mod:ro hublot-test go test -race ./internal/fsview/`
Expected : `ok`.

La spec n'est pas modifiée ici : les extensions de `HostBind` sont écrites au §5.4 (tâche 12), pas au §5.2 étape 7, dont la portée pour les conteneurs arrêtés reste à trancher (Q9).

- [ ] **Step 5: Commit**

```bash
git add internal/fsview
git commit -F - <<'EOF'
explorateur: contrôle des montages avant le tar d'un dossier (binds de l'hôte, mountinfo)

Docker monte les binds du conteneur pour construire GET /archive : un bind
du /proc de l'hôte y mettait /proc/kcore et bloquait dockerd (revue C1).

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 8: `procfs` sur de vrais conteneurs

**Où tester :** Linux root avec Docker (`scripts/test-linux.sh -tags integration`, Docker Desktop) ; les VM et la CI à la tâche 16.

**Files:**
- Create: `internal/fsview/integration_test.go`

**Interfaces:**
- Consumes: `For`, `FS`, `CheckArchive`, erreurs (tâches 1 à 7) ; `dockerx.New`, `GetArchive`
- Produces: rien (tests)

> **À comprendre :** ces tests valident la séquence complète de §5.1 sur Docker.
>
> - **alpine :** lien `/etc/os-release` → `../usr/lib/os-release`, `/dev/null`, `/proc`, `/sys`.
> - **distroless non root en `--read-only` :** variante `:debug`, la seule à contenir un `sleep` (`/busybox/sleep`).
> - **cible `--privileged` :** non confinée, elle exige `apparmor=unconfined` sur un hôte AppArmor (§8).
> - **redémarrage :** nouveau PID ; **conteneur arrêté :** `ErrStopped` ; **conteneur absent :** `ErrNotFound`.
> - **cible `--pid=host` (revue I2) :** `/proc/1/root` y est la racine de l'hôte. Ni directement, ni par le lien `/m`, rien ne doit être lu.
> - **`CheckArchive`** sur un conteneur sans bind.
> - **une mesure (revue C1, Q8) :** le tar d'un conteneur actif voit-il son propre `/proc` ?
>
> La mesure est sûre : un seul petit pseudo-fichier (`/proc/sys/kernel/hostname`, taille 0 dans l'en-tête tar), un délai de 10 s, jamais un dossier de `/proc`. Le test **consigne** le résultat (`MESURE : …`) sans échouer. Il ne conclut « ne voit pas » que sur `dockerx.ErrNotFound` ; toute autre erreur (délai, démon indisponible) donne « non concluante » (revue R6). Sur Docker Desktop, Docker 29, la réponse est « ne voit pas ».
>
> La cible `--pid=host` crée `/m` au démarrage : le test attend que `List("/")` le montre, au lieu d'un `sleep` (revue R6).
>
> L'aide `docker` lit la sortie standard seule : un `docker run` qui télécharge l'image écrit sa progression sur la sortie d'erreur, qui ne doit pas se mêler à l'ID renvoyé.

- [ ] **Step 1: Écrire les tests**

`internal/fsview/integration_test.go` :

```go
//go:build integration && linux

package fsview

// procfs on real containers (spec §10, privileged job). Needs root, the host
// PID namespace and the Docker socket: scripts/test-linux.sh or the CI
// integration job.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
}

// docker runs the CLI and returns its stdout (a pull writes its progress on
// stderr, which must not end up in the returned ID).
func docker(t *testing.T, args ...string) string {
	t.Helper()
	var stderr bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker %s : %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return strings.TrimSpace(string(out))
}

// runTarget starts a container and returns its full ID; removed at the end.
func runTarget(t *testing.T, name string, args ...string) string {
	t.Helper()
	exec.Command("docker", "rm", "-f", name).Run()
	id := docker(t, append([]string{"run", "-d", "--name", name}, args...)...)
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	return id
}

func openFor(t *testing.T, id string) FS {
	t.Helper()
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fsys, err := For(ctx, dc, id)
	if err != nil {
		t.Fatalf("For(%s) : %v", id, err)
	}
	t.Cleanup(func() { fsys.Close() })
	return fsys
}

func hasEntry(entries []Entry, name string, typ EntryType) bool {
	for _, e := range entries {
		if e.Name == name && e.Type == typ {
			return true
		}
	}
	return false
}

func TestIntegrationProcFSAlpine(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-alpine", "alpine:3", "sleep", "300")
	fsys := openFor(t, id)
	ctx := context.Background()

	entries, _, err := fsys.List(ctx, "/")
	if err != nil || !hasEntry(entries, "etc", TypeDir) || !hasEntry(entries, "proc", TypeDir) {
		t.Fatalf("List / : %v %v", entries, err)
	}
	rc, err := fsys.Open(ctx, "/etc/os-release") // a link to ../usr/lib/os-release, followed in the root
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !strings.Contains(string(b), "Alpine") {
		t.Fatalf("os-release : %q", b)
	}
	if _, err := fsys.StatNoLinks(ctx, "/etc/os-release"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("StatNoLinks d'un lien : %v", err)
	}
	if _, err := fsys.Open(ctx, "/dev/null"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("/dev/null : %v", err)
	}
	if _, _, err := fsys.List(ctx, "/proc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc : %v", err)
	}
	if _, err := fsys.Open(ctx, "/proc/kallsyms"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/proc/kallsyms : %v", err)
	}
	if _, err := fsys.Stat(ctx, "/sys"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("/sys : %v", err)
	}
}

func TestIntegrationProcFSDistrolessNonRoot(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-distroless", "--user", "65532", "--read-only",
		"--entrypoint", "/busybox/sleep", "gcr.io/distroless/static-debian12:debug", "300")
	fsys := openFor(t, id)
	rc, err := fsys.Open(context.Background(), "/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if !strings.Contains(string(b), "nonroot") {
		t.Fatalf("/etc/passwd : %q", b)
	}
}

func TestIntegrationProcFSPrivilegedTarget(t *testing.T) {
	needRoot(t)
	// Unconfined target: needs an agent with apparmor=unconfined on an AppArmor
	// host (spec §8, spike 3b). scripts/test-linux.sh gives it.
	id := runTarget(t, "hublot-it-fs-priv", "--privileged", "alpine:3", "sleep", "300")
	if _, _, err := openFor(t, id).List(context.Background(), "/etc"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationProcFSAfterRestart(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-restart", "alpine:3", "sleep", "300")
	before := docker(t, "inspect", "-f", "{{.State.Pid}}", id)
	docker(t, "restart", "-t", "0", id)
	if after := docker(t, "inspect", "-f", "{{.State.Pid}}", id); after == before {
		t.Fatalf("le pid n'a pas changé (%s)", after)
	}
	if _, _, err := openFor(t, id).List(context.Background(), "/etc"); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationForStoppedContainer(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-stopped", "alpine:3", "true")
	docker(t, "wait", id)
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := For(context.Background(), dc, id); !errors.Is(err, ErrStopped) {
		t.Fatalf("conteneur arrêté : %v", err)
	}
	if _, err := For(context.Background(), dc, "hublot-it-absent"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("conteneur absent : %v", err)
	}
}

// A target sharing the host's PID namespace sees the host's processes:
// /proc/1/root is the host's root. Never followed, directly or by a link
// (spec §5.1 steps 6 and 9; pathrs refuses magic links).
func TestIntegrationPidHostTarget(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-pidhost", "--pid=host", "alpine:3", "sh", "-c", "ln -s /proc/1/root /m && sleep 300")
	fsys := openFor(t, id)
	ctx := context.Background()
	// Wait for the shell to create /m (seen in the listing of /, never followed).
	for deadline := time.Now().Add(10 * time.Second); ; {
		entries, _, err := fsys.List(ctx, "/")
		if err == nil && hasEntry(entries, "m", TypeSymlink) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/m n'apparaît pas : %v %v", entries, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, p := range []string{"/proc/1/root/etc/hostname", "/m/etc/hostname", "/m/etc/shadow"} {
		if rc, err := fsys.Open(ctx, p); err == nil {
			b, _ := io.ReadAll(rc)
			rc.Close()
			t.Errorf("ÉVASION : %s lu (%q)", p, b)
		}
	}
	for _, p := range []string{"/proc/1/root", "/m", "/m/etc"} {
		if entries, _, err := fsys.List(ctx, p); err == nil {
			t.Errorf("ÉVASION : List %s a rendu %d entrées", p, len(entries))
		}
	}
	if _, err := fsys.StatNoLinks(ctx, "/m/etc"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("StatNoLinks /m/etc : %v", err)
	}
}

// The mount check on a real container without binds: its own /proc and /sys
// are under "/", /etc has none.
func TestIntegrationCheckArchive(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-archive", "alpine:3", "sleep", "300")
	fsys := openFor(t, id)
	ctx := context.Background()
	for _, dir := range []string{"/", "/proc", "/sys"} {
		if err := fsys.CheckArchive(ctx, dir); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("%s : %v, attendu ErrForbiddenPath", dir, err)
		}
	}
	if err := fsys.CheckArchive(ctx, "/etc"); err != nil {
		t.Errorf("/etc : %v", err)
	}
}

// Measurement, not an assertion (review ruling C1): does GET /archive of a
// running container see the container's own procfs? One small pseudo-file,
// a short delay: never a directory of /proc (a tar of /proc can lock dockerd).
func TestIntegrationMeasureOwnProcInArchive(t *testing.T) {
	needRoot(t)
	id := runTarget(t, "hublot-it-fs-ownproc", "alpine:3", "sleep", "300")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rc, err := dc.GetArchive(ctx, id, "/proc/sys/kernel/hostname")
	switch {
	case err == nil:
		rc.Close()
		t.Logf("MESURE : GET /archive VOIT le /proc propre du conteneur")
	case errors.Is(err, dockerx.ErrNotFound):
		t.Logf("MESURE : GET /archive ne voit pas le /proc propre du conteneur (%v)", err)
	default:
		t.Logf("MESURE : non concluante (%v)", err)
	}
}
```

- [ ] **Step 2: Lancer**

Run: `scripts/test-linux.sh -tags integration ./internal/fsview/ ./internal/dockerx/ -run 'Integration|RealDocker' -v 2>&1 | grep -E '^(--- |ok|FAIL)|MESURE'`
Expected : `--- PASS` pour les 8 tests `TestIntegration…` et les 3 tests `…RealDocker`, plus une ligne `MESURE :`, à recopier dans le rapport de tâche.

Run (macOS, contrôle) : `go test -tags integration ./internal/fsview/`
Expected : PASS sans rien lancer (`integration && linux`).

- [ ] **Step 3: Commit**

```bash
git add internal/fsview
git commit -F - <<'EOF'
explorateur: tests d'intégration de procfs (distroless, privilégié, --pid=host, montages, mesure du /proc propre)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 9: API : garde FUSE, codes d'erreur, faux `FS`

**Où tester :** macOS.

**Files:**
- Create: `internal/api/fsguard.go`, `internal/api/fsguard_test.go`, `internal/api/fserr.go`, `internal/api/fserr_test.go`, `internal/fsview/memfs.go`, `internal/fsview/memfs_test.go`
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§6, statuts)

**Interfaces:**
- Consumes: erreurs de `fsview` (tâche 1) ; `dockerx.ErrNotFound`, `dockerx.ErrUnavailable` (tâche 3) ; `writeError` (Socle)
- Produces:
  - `newFSGuard(max int, timeout time.Duration) *fsGuard` ;
  - `guarded[T any](g *fsGuard, ctx context.Context, id string, fn func() (T, error), cleanup func(T)) (T, release func(), error)` : `release` n'est jamais nil ;
  - `errBusy`, `errTimeout` ;
  - `writeFSError(w http.ResponseWriter, err error)` et `fsErrorStatus(err error) (int, string)` ;
  - `fsview.NewMemFS() *MemFS`, avec `AddDir(p)`, `AddFile(p, content)`, `AddLink(p, target)` (chaînables) et les champs `Entries`, `Content`, `Err`, `ArchiveErr`, `OpenFile func(p string) (io.ReadCloser, error)`, `Closed atomic.Int32`.

> **À comprendre :** un montage FUSE du conteneur qui ne répond plus bloque *indéfiniment* la goroutine qui le lit (§13) ; on ne peut pas l'interrompre.
>
> - `guarded` prend un créneau du conteneur, lance l'opération dans sa propre goroutine et rend la main après le délai (ou quand le client part).
> - Un créneau n'est rendu qu'au **vrai** retour du travail (revue Codex n°4) :
>   - opération abandonnée : sa goroutine garde le créneau jusqu'à son retour, puis `cleanup` ferme ce qu'elle a ouvert ;
>   - opération réussie : l'appelant reçoit `release` et l'appelle quand il a fini du résultat ; pour un téléchargement, c'est après la copie.
> - Le nombre de goroutines bloquées reste donc borné à `max` par conteneur.
> - `guarded` est une fonction générique (`[T any]`) : Go n'accepte pas de paramètre de type sur une méthode.
> - `ELOOP` (boucle de liens, lien magique refusé) devient `not_found` (revue M4).
> - `MemFS` est le faux de `fsview.FS` pour les tests de `api`, comme `dockerx.Fake` pour Docker. `ArchiveErr` simule un refus de `CheckArchive` ; `OpenFile` simule un fichier FUSE qui bloque.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/api/fsguard_test.go` :

```go
package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func busy(g *fsGuard) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.busy)
}

func TestGuardKeepsTheSlotUntilRelease(t *testing.T) {
	g := newFSGuard(1, time.Second)
	v, release, err := guarded(g, context.Background(), "c", func() (int, error) { return 7, nil }, nil)
	if v != 7 || err != nil {
		t.Fatalf("%d %v", v, err)
	}
	// The caller still uses the result (a download streams it): slot taken.
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { return 1, nil }, nil); !errors.Is(err, errBusy) {
		t.Fatalf("attendu errBusy tant que release n'est pas appelé, reçu %v", err)
	}
	release()
	release() // idempotent
	if busy(g) != 0 {
		t.Fatal("créneau non rendu")
	}
}

func TestGuardFreesTheSlotOnError(t *testing.T) {
	g := newFSGuard(1, time.Second)
	_, release, err := guarded(g, context.Background(), "c", func() (int, error) { return 0, errors.New("x") }, nil)
	release() // a no-op, but always callable
	if err == nil || busy(g) != 0 {
		t.Fatalf("err %v, créneaux %d", err, busy(g))
	}
}

func TestGuardTimesOutAndCleansUpLateResult(t *testing.T) {
	g := newFSGuard(2, 20*time.Millisecond)
	unblock := make(chan struct{})
	cleaned := make(chan int, 1)
	_, release, err := guarded(g, context.Background(), "c", func() (int, error) { <-unblock; return 9, nil },
		func(v int) { cleaned <- v })
	release()
	if !errors.Is(err, errTimeout) {
		t.Fatalf("attendu errTimeout, reçu %v", err)
	}
	if busy(g) != 1 {
		t.Fatal("la goroutine bloquée doit garder son créneau")
	}
	close(unblock)
	select {
	case v := <-cleaned:
		if v != 9 {
			t.Fatalf("nettoyage de %d", v)
		}
	case <-time.After(time.Second):
		t.Fatal("le résultat tardif n'a pas été nettoyé")
	}
	deadline := time.Now().Add(time.Second)
	for busy(g) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if busy(g) != 0 {
		t.Fatal("créneau non rendu après le vrai retour")
	}
}

func TestGuardBoundsEachContainer(t *testing.T) {
	g := newFSGuard(1, 20*time.Millisecond)
	block := make(chan struct{})
	defer close(block)
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { <-block; return 0, nil }, nil); !errors.Is(err, errTimeout) {
		t.Fatalf("attendu errTimeout, reçu %v", err)
	}
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { return 1, nil }, nil); !errors.Is(err, errBusy) {
		t.Fatalf("attendu errBusy, reçu %v", err)
	}
	v, release, err := guarded(g, context.Background(), "other", func() (int, error) { return 1, nil }, nil)
	release()
	if err != nil || v != 1 {
		t.Fatalf("un autre conteneur a son propre budget : %d %v", v, err)
	}
}

func TestGuardStopsWithTheRequest(t *testing.T) {
	g := newFSGuard(1, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	block := make(chan struct{})
	defer close(block)
	if _, _, err := guarded(g, ctx, "c", func() (int, error) { <-block; return 0, nil }, nil); !errors.Is(err, errTimeout) {
		t.Fatalf("requête annulée : %v", err)
	}
}
```

`internal/api/fserr_test.go` :

```go
package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"syscall"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func TestFSErrorStatus(t *testing.T) {
	wrapped := func(e error) error { return fmt.Errorf("%w : détail", e) }
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{wrapped(errBusy), http.StatusServiceUnavailable, "internal"},
		{wrapped(errTimeout), http.StatusGatewayTimeout, "internal"},
		{wrapped(dockerx.ErrNotFound), http.StatusNotFound, "not_found"},
		{wrapped(dockerx.ErrUnavailable), http.StatusServiceUnavailable, "docker_unavailable"},
		// A missing Docker socket is ENOENT: it must not read as a missing file.
		{fmt.Errorf("%w : %w", dockerx.ErrUnavailable, syscall.ENOENT), http.StatusServiceUnavailable, "docker_unavailable"},
		{wrapped(fsview.ErrContainerChanged), http.StatusConflict, "container_changed"},
		{wrapped(fsview.ErrForbiddenPath), http.StatusForbidden, "forbidden_path"},
		{wrapped(fsview.ErrLinkInPath), http.StatusUnprocessableEntity, "link_in_path"},
		{wrapped(fsview.ErrNotRegular), http.StatusUnprocessableEntity, "not_regular"},
		{wrapped(fsview.ErrTooLarge), http.StatusRequestEntityTooLarge, "too_large"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}, http.StatusNotFound, "not_found"},
		{&fs.PathError{Op: "open", Path: "/etc/hosts/x", Err: syscall.ENOTDIR}, http.StatusNotFound, "not_found"},
		{wrapped(fsview.ErrNotDir), http.StatusNotFound, "not_found"},
		{&fs.PathError{Op: "openat2", Path: "/m/etc", Err: syscall.ELOOP}, http.StatusNotFound, "not_found"},
		{fmt.Errorf("%w : /lien", fsview.ErrLinkInPath), http.StatusUnprocessableEntity, "link_in_path"},
		{wrapped(fsview.ErrStopped), http.StatusNotImplemented, "internal"},
		{errors.New("autre"), http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		if status, code := fsErrorStatus(c.err); status != c.status || code != c.code {
			t.Errorf("%v : %d %s, attendu %d %s", c.err, status, code, c.status, c.code)
		}
	}
}
```

`internal/fsview/memfs_test.go` :

```go
package fsview

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
)

func TestMemFS(t *testing.T) {
	ctx := context.Background()
	m := NewMemFS().AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1")).AddLink("/etc/l", "hosts").
		AddDir("/ld").AddLink("/link", "/etc")
	entries, _, err := m.List(ctx, "/etc")
	if err != nil || len(entries) != 2 || entries[0].Name != "hosts" || entries[1].LinkTarget != "hosts" {
		t.Fatalf("List : %+v %v", entries, err)
	}
	if _, _, err := m.List(ctx, "/etc/hosts"); !errors.Is(err, ErrNotDir) {
		t.Fatalf("List d'un fichier : %v", err)
	}
	rc, err := m.Open(ctx, "/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "127.0.0.1" {
		t.Fatalf("Open : %q", b)
	}
	if _, err := m.Open(ctx, "/etc"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Open d'un dossier : %v", err)
	}
	if _, err := m.Stat(ctx, "/absent"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("absent : %v", err)
	}
	m.AddDir("/link/sub")
	if _, err := m.StatNoLinks(ctx, "/link/sub"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("lien parent : %v", err)
	}
	if _, err := m.StatNoLinks(ctx, "/etc/l"); !errors.Is(err, ErrLinkInPath) {
		t.Fatalf("lien final : %v", err)
	}
	if _, err := m.StatNoLinks(ctx, "/etc/hosts"); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckArchive(ctx, "/etc"); err != nil {
		t.Fatalf("CheckArchive : %v", err)
	}
	m.ArchiveErr = ErrForbiddenPath
	if err := m.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("ArchiveErr : %v", err)
	}
	m.OpenFile = func(p string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("hook:" + p)), nil }
	rc, _ = m.Open(ctx, "/etc/hosts")
	if b, _ := io.ReadAll(rc); string(b) != "hook:/etc/hosts" {
		t.Fatalf("OpenFile : %q", b)
	}
	m.Err = ErrForbiddenPath
	if _, err := m.Stat(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) {
		t.Fatalf("Err : %v", err)
	}
	m.Close()
	if m.Closed.Load() != 1 {
		t.Fatal("Close non compté")
	}
}
```

Run: `go test ./internal/api/ ./internal/fsview/`
Expected: FAIL à la compilation (`undefined: newFSGuard`, `undefined: NewMemFS`).

- [ ] **Step 2: Écrire le code**

`internal/api/fsguard.go` :

```go
package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Spec §13: a FUSE mount of the container that never answers blocks the
// goroutine that reads it. The request must fail anyway, and such goroutines
// must not pile up: at most max operations per container (by full ID), each
// one given up after timeout. A slot is freed only when the work really ends:
// the goroutine of a given-up operation keeps it until it returns, and a
// download keeps it until its copy returns.
var (
	errBusy    = errors.New("trop d'opérations de fichiers en cours sur ce conteneur (montage FUSE bloqué ?)")
	errTimeout = errors.New("délai dépassé (montage FUSE bloqué ?)")
)

type fsGuard struct {
	mu      sync.Mutex
	busy    map[string]int
	max     int
	timeout time.Duration
}

func newFSGuard(max int, timeout time.Duration) *fsGuard {
	return &fsGuard{busy: map[string]int{}, max: max, timeout: timeout}
}

// take reserves a slot of container id; the returned func frees it (once).
func (g *fsGuard) take(id string) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[id] >= g.max {
		return nil, false
	}
	g.busy[id]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.busy[id]--; g.busy[id] <= 0 {
				delete(g.busy, id)
			}
		})
	}, true
}

func noop() {}

// guarded takes a slot of container id and runs fn in its own goroutine.
// Success: it returns fn's result and release, which the caller calls once it
// is done with the result (after streaming a download). Error: the slot is
// already free. Timeout (or end of ctx): errTimeout; the slot stays taken
// until fn returns, then fn's late result goes to cleanup (which closes what
// fn opened). release is never nil: defer it right away.
func guarded[T any](g *fsGuard, ctx context.Context, id string, fn func() (T, error), cleanup func(T)) (T, func(), error) {
	var zero T
	release, ok := g.take(id)
	if !ok {
		return zero, noop, errBusy
	}
	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	timer := time.NewTimer(g.timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil {
			release()
			return zero, noop, r.err
		}
		return r.v, release, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	go func() {
		if r := <-done; r.err == nil && cleanup != nil {
			cleanup(r.v)
		}
		release()
	}()
	return zero, noop, fmt.Errorf("%w (%s)", errTimeout, g.timeout)
}
```

`internal/api/fserr.go` :

```go
package api

import (
	"errors"
	"io/fs"
	"net/http"
	"syscall"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func writeFSError(w http.ResponseWriter, err error) {
	status, code := fsErrorStatus(err)
	writeError(w, status, code, err.Error())
}

// fsErrorStatus maps the errors of file routes to the codes of spec §6. The
// spec fixes 202 indexing, 400 invalid_name and 429; the other statuses are
// the choice of plan Explorateur 1 ("Décisions"). Order matters: Docker
// errors first (a missing socket is ENOENT too, it is not a missing file).
func fsErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, errBusy):
		return http.StatusServiceUnavailable, "internal"
	case errors.Is(err, errTimeout):
		return http.StatusGatewayTimeout, "internal"
	case errors.Is(err, dockerx.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, dockerx.ErrUnavailable):
		return http.StatusServiceUnavailable, "docker_unavailable"
	case errors.Is(err, fsview.ErrContainerChanged):
		return http.StatusConflict, "container_changed"
	case errors.Is(err, fsview.ErrForbiddenPath):
		return http.StatusForbidden, "forbidden_path"
	case errors.Is(err, fsview.ErrLinkInPath):
		return http.StatusUnprocessableEntity, "link_in_path"
	case errors.Is(err, fsview.ErrNotRegular):
		return http.StatusUnprocessableEntity, "not_regular"
	case errors.Is(err, fsview.ErrTooLarge):
		return http.StatusRequestEntityTooLarge, "too_large"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fsview.ErrNotDir), errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.ELOOP): // a link loop, or a magic link refused by pathrs
		return http.StatusNotFound, "not_found"
	case errors.Is(err, fsview.ErrStopped):
		return http.StatusNotImplemented, "internal"
	}
	return http.StatusInternalServerError, "internal"
}
```

`internal/fsview/memfs.go` :

```go
package fsview

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"sync/atomic"
)

// MemFS is an in-memory FS for the tests of other packages (api). Keys are
// CleanPath forms; "/" is always a directory. Stat and Open do not follow
// links (tests that need real resolution use procfs); StatNoLinks refuses
// them like procfs. Build it with NewMemFS and the Add* methods.
type MemFS struct {
	Entries map[string]Entry
	Content map[string][]byte
	Err     error // when set, every call except Close returns it
	// ArchiveErr is CheckArchive's answer (nil: the tar is allowed).
	ArchiveErr error
	// OpenFile, when set, replaces the reader of every regular file (tests of
	// a reader that blocks, like a FUSE file that never answers).
	OpenFile func(p string) (io.ReadCloser, error)
	Closed   atomic.Int32 // number of Close calls
}

func NewMemFS() *MemFS {
	return &MemFS{Entries: map[string]Entry{}, Content: map[string][]byte{}}
}

func (m *MemFS) AddDir(p string) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeDir, Mode: 0o755}
	return m
}

func (m *MemFS) AddFile(p string, content []byte) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeFile, Mode: 0o644, Size: int64(len(content))}
	m.Content[p] = content
	return m
}

func (m *MemFS) AddLink(p, target string) *MemFS {
	p = CleanPath(p)
	m.Entries[p] = Entry{Name: path.Base(p), Type: TypeSymlink, Mode: 0o777, LinkTarget: target}
	return m
}

func (m *MemFS) get(p string) (Entry, error) {
	if m.Err != nil {
		return Entry{}, m.Err
	}
	p = CleanPath(p)
	if p == "/" {
		return Entry{Name: "/", Type: TypeDir, Mode: 0o755}, nil
	}
	if e, ok := m.Entries[p]; ok {
		return e, nil
	}
	return Entry{}, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
}

func (m *MemFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	d, err := m.get(dir)
	if err != nil {
		return nil, false, err
	}
	if d.Type != TypeDir {
		return nil, false, fmt.Errorf("%w : %s", ErrNotDir, dir)
	}
	dir = CleanPath(dir)
	out := []Entry{}
	for p, e := range m.Entries {
		if path.Dir(p) == dir {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, false, nil
}

func (m *MemFS) Stat(_ context.Context, p string) (Entry, error) { return m.get(p) }

func (m *MemFS) Open(_ context.Context, p string) (io.ReadCloser, error) {
	e, err := m.get(p)
	if err != nil {
		return nil, err
	}
	if e.Type != TypeFile {
		return nil, fmt.Errorf("%w : %s", ErrNotRegular, p)
	}
	if m.OpenFile != nil {
		return m.OpenFile(CleanPath(p))
	}
	return io.NopCloser(bytes.NewReader(m.Content[CleanPath(p)])), nil
}

func (m *MemFS) StatNoLinks(_ context.Context, p string) (Entry, error) {
	p = CleanPath(p)
	for cur := p; cur != "/"; cur = path.Dir(cur) {
		if e, err := m.get(cur); err == nil && e.Type == TypeSymlink {
			return Entry{}, fmt.Errorf("%w : %s", ErrLinkInPath, p)
		}
	}
	return m.get(p)
}

func (m *MemFS) CheckArchive(_ context.Context, dir string) error {
	if _, err := m.get(dir); err != nil {
		return err
	}
	return m.ArchiveErr
}

func (m *MemFS) Close() error {
	m.Closed.Add(1)
	return nil
}
```

- [ ] **Step 3: Vérifier**

Run: `go vet ./internal/... && go test -race ./internal/api/ ./internal/fsview/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected : PASS : 5 tests `TestGuard…`, `TestFSErrorStatus`, `TestMemFS`, et les tests existants.

- [ ] **Step 4: Reporter les statuts dans la spec**

Dans la spec §6, après la phrase « Conteneur arrêté : toute route de fichiers peut aussi renvoyer `too_large` (construction de l'index plafonnée, §5.2 étape 3). », ajouter :

> Statuts HTTP : `not_found` 404 (`ELOOP` compris : boucle de liens, lien magique refusé), `forbidden_path` 403, `container_changed` 409, `not_regular` et `link_in_path` 422, `too_large` 413, `docker_unavailable` 503, `internal` 500. Garde des routes de fichiers (§13) : 503 si 4 opérations sont déjà en cours sur le conteneur, 504 après 10 s ; un créneau n'est rendu qu'au vrai retour de l'opération, copie d'un téléchargement comprise.

- [ ] **Step 5: Commit**

```bash
git add internal/api internal/fsview docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: garde des routes de fichiers (délai, plafond, créneau rendu au vrai retour), codes d'erreur, faux FS

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 10: API : `GET /api/containers/{id}/fs` (liste)

**Où tester :** macOS.

**Files:**
- Create: `internal/api/fs.go`, `internal/api/fs_test.go`
- Modify: `internal/api/server.go`, `internal/api/server_test.go`

**Interfaces:**
- Consumes: `guarded`, `writeFSError` (tâche 9) ; `fsview.For`, `FS`, `CleanPath`, `UnixPerm`, `MemFS` (tâches 1, 6, 9) ; `dockerx.Client.Inspect`, `Fake.Infos` (tâche 3)
- Produces:
  - `Deps.OpenFS func(ctx context.Context, id string) (fsview.FS, error)` (nil → `fsview.For` sur `Deps.Docker`) ;
  - constantes `fsMaxPerContainer = 4`, `fsTimeout = 10 * time.Second` ;
  - `(s *server) fsRequest(w, r) (info dockerx.ContainerInfo, p string, ok bool)` ;
  - `entryJSON`, `listingJSON` (champs JSON `path`, `type`, `entries`, `truncated` ; entrée `name`, `type`, `size`, `mode`, `uid`, `gid`, `modTime`, `linkTarget`) ;
  - côté tests : `fixture.fs`, `fixture.openErr`, `fixture.opened()`, `fixture.init(t, readOnly, docker, openFS)`, `const webID`, `fixture.session(t)`, `decodeListing(t, rec)`.

> **À comprendre :** chaque requête commence par `Inspect`, borné à `fsTimeout` : un démon bloqué ne doit pas suspendre la requête (revue I3).
>
> - Le nom ou l'ID court devient l'**ID complet**, et seul l'ID complet sert ensuite. La garde compte donc par conteneur, quel que soit le nom employé.
> - Un conteneur supprimé puis recréé sous le même nom ne peut pas se substituer à l'autre au milieu d'une requête.
> - La vue est fermée à la fin de chaque opération : le descripteur de la racine ne vit pas au-delà.
> - La fixture des tests passe d'une construction en bloc à `newFixture` + `init`. `init` sert aussi au test de bout en bout (tâche 16), avec un vrai Docker.
> - `OpenFS` tourne dans les goroutines de la garde : l'ID reçu est protégé par un mutex.
> - La borne de 10 s d'`Inspect` n'est pas testée (revue R6) : il faudrait un faux Docker qui bloque, pour un gain faible. Elle se vérifie à la relecture de `fsRequest`.

- [ ] **Step 1: Adapter la fixture**

Dans `internal/api/server_test.go`, ajouter `"context"`, `"sync"` et `"github.com/levy-cmd/hublot-kit/internal/fsview"` aux imports. Remplacer ensuite tout ce qui va de `type fixture struct {` jusqu'à la ligne qui précède `func (f *fixture) do(` par :

```go
type fixture struct {
	h      http.Handler
	token  string
	auditP string
	docker *dockerx.Fake // nil when the fixture talks to a real Docker
	// File routes: OpenFS returns fs (or openErr) and records the ID it got.
	fs       *fsview.MemFS
	openErr  error
	mu       sync.Mutex // OpenFS runs in the guard's goroutines
	openedID string
}

// opened returns the ID the last OpenFS call received ("" if none).
func (f *fixture) opened() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.openedID
}

// webID is the full ID of the fake "web" container of newFixture.
const webID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newFixture(t *testing.T, readOnly bool) *fixture {
	t.Helper()
	web := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: 42}
	fake := &dockerx.Fake{
		Containers: []dockerx.Container{{ID: "abc", Name: "web", Image: "nginx", State: "running", Ports: []dockerx.Port{}}},
		Infos:      map[string]dockerx.ContainerInfo{"web": web, webID: web},
		Archives:   map[string][]byte{},
	}
	f := &fixture{docker: fake, fs: fsview.NewMemFS()}
	f.init(t, readOnly, fake, func(_ context.Context, id string) (fsview.FS, error) {
		f.mu.Lock()
		f.openedID = id
		f.mu.Unlock()
		if f.openErr != nil {
			return nil, f.openErr
		}
		return f.fs, nil
	})
	return f
}

// init builds the handler; openFS nil means the real fsview.For.
func (f *fixture) init(t *testing.T, readOnly bool, docker dockerx.Client, openFS func(context.Context, string) (fsview.FS, error)) {
	t.Helper()
	dir := t.TempDir()
	tok, _, err := auth.EnsureToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.LoadVerifier(dir)
	if err != nil {
		t.Fatal(err)
	}
	ap := filepath.Join(dir, "audit.log")
	al, err := audit.Open(ap)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { al.Close() })
	f.h = New(Deps{
		Docker: docker, OpenFS: openFS, Verifier: v, Audit: al, Version: "test", ReadOnly: readOnly,
		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
	})
	f.token, f.auditP = tok, ap
}
```

Run: `go vet ./internal/api/`
Expected : FAIL (`unknown field OpenFS in struct literal`) : la fixture attend la nouvelle `Deps`.

- [ ] **Step 2: Écrire les tests qui échouent**

`internal/api/fs_test.go` :

```go
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// session logs in and returns the cookie for the file routes.
func (f *fixture) session(t *testing.T) *http.Cookie {
	t.Helper()
	_, c := f.login(t, f.token)
	if c == nil {
		t.Fatal("connexion impossible")
	}
	return c
}

func decodeListing(t *testing.T, rec *httptest.ResponseRecorder) listingJSON {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
	}
	var l listingJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFSListNeedsSession(t *testing.T) {
	f := newFixture(t, false)
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("statut %d", rec.Code)
	}
}

func TestFSList(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1")).AddLink("/etc/os-release", "../usr/lib/os-release")
	f.fs.Entries["/etc/hosts"] = fsview.Entry{Name: "hosts", Type: fsview.TypeFile, Size: 9, Mode: fs.ModeSetuid | 0o644, UID: 0, GID: 101}
	l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path=/etc", "", f.session(t)))
	if l.Path != "/etc" || l.Type != "dir" || l.Truncated || len(l.Entries) != 2 {
		t.Fatalf("listing : %+v", l)
	}
	if e := l.Entries[0]; e.Name != "hosts" || e.Type != "file" || e.Size != 9 || e.Mode != 0o4644 || e.GID != 101 {
		t.Fatalf("hosts : %+v", e)
	}
	if e := l.Entries[1]; e.Name != "os-release" || e.Type != "symlink" || e.LinkTarget != "../usr/lib/os-release" {
		t.Fatalf("lien : %+v", e)
	}
	if f.opened() != webID {
		t.Fatalf("la vue doit s'ouvrir sur l'ID complet, pas sur le nom : %q", f.opened())
	}
	if f.fs.Closed.Load() != 1 {
		t.Fatalf("la vue doit être fermée une fois, fermée %d fois", f.fs.Closed.Load())
	}
}

func TestFSListCleansPath(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc")
	c := f.session(t)
	for _, raw := range []string{"etc", "/etc/", "//etc", "/etc/../etc", "../../etc", "%2Fetc"} {
		if l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path="+raw, "", c)); l.Path != "/etc" {
			t.Errorf("%q → %q", raw, l.Path)
		}
	}
	if l := decodeListing(t, f.do("GET", "/api/containers/web/fs", "", c)); l.Path != "/" || l.Entries == nil {
		t.Errorf("sans path → %+v", l)
	}
}

func TestFSListOfAFile(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("x"))
	l := decodeListing(t, f.do("GET", "/api/containers/web/fs?path=/etc/hosts", "", f.session(t)))
	if l.Type != "file" || len(l.Entries) != 0 || l.Path != "/etc/hosts" {
		t.Fatalf("listing d'un fichier : %+v", l)
	}
}

func TestFSListErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fsview.ErrForbiddenPath, http.StatusForbidden, "forbidden_path"},
		{fmt.Errorf("%w : pid 7", fsview.ErrContainerChanged), http.StatusConflict, "container_changed"},
		{fs.ErrNotExist, http.StatusNotFound, "not_found"},
	}
	for _, c := range cases {
		f := newFixture(t, false)
		f.fs.Err = c.err
		rec := f.do("GET", "/api/containers/web/fs?path=/x", "", f.session(t))
		if rec.Code != c.status || errCode(t, rec) != c.code {
			t.Errorf("%v : %d %s", c.err, rec.Code, rec.Body)
		}
	}
	f := newFixture(t, false)
	f.openErr = fmt.Errorf("%w : web", fsview.ErrStopped)
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", f.session(t)); rec.Code != http.StatusNotImplemented || errCode(t, rec) != "internal" {
		t.Errorf("conteneur arrêté : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/nope/fs?path=/", "", f.session(t)); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("conteneur inconnu : %d %s", rec.Code, rec.Body)
	}
	f = newFixture(t, false)
	c := f.session(t)
	f.docker.Err = fmt.Errorf("%w : dial", dockerx.ErrUnavailable)
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker arrêté : %d %s", rec.Code, rec.Body)
	}
}

func TestFSBadContainerRef(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	for _, id := range []string{".hidden", "a%2Fb", "%2E%2E", "-x", "a%20b"} {
		rec := f.do("GET", "/api/containers/"+id+"/fs?path=/", "", c)
		if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
			t.Errorf("%q : %d %s", id, rec.Code, rec.Body)
		}
	}
	// A literal ".." segment is cleaned by ServeMux itself (redirect).
	if rec := f.do("GET", "/api/containers/../fs?path=/", "", c); rec.Code == http.StatusOK {
		t.Errorf(".. : %d", rec.Code)
	}
	if f.opened() != "" {
		t.Fatalf("aucune vue ne doit s'ouvrir, ouverte pour %q", f.opened())
	}
}

func TestFSNULInPath(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do("GET", "/api/containers/web/fs?path=/etc%00/passwd", "", f.session(t))
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" || f.opened() != "" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
```

Run: `go test ./internal/api/`
Expected: FAIL à la compilation (`undefined: listingJSON`, `unknown field OpenFS`).

- [ ] **Step 3: Écrire le code**

`internal/api/server.go` (contenu complet). Nouveautés : `Deps.OpenFS`, `server.guard`, les constantes de la garde, `OpenFS` par défaut et la route `GET /api/containers/{id}/fs`.

```go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/audit"
	"github.com/levy-cmd/hublot-kit/internal/auth"
	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

type Deps struct {
	Docker dockerx.Client
	// OpenFS opens the file view of a container by full ID; nil means
	// fsview.For on Docker. API tests give an in-memory view.
	OpenFS       func(ctx context.Context, id string) (fsview.FS, error)
	Verifier     *auth.Verifier
	Sessions     *auth.Sessions
	Limiter      *auth.Limiter
	Audit        *audit.Log
	Version      string
	ReadOnly     bool
	TLS          bool
	PublicOrigin string       // normalized, or ""
	Static       http.Handler // embedded UI; nil in API tests
	ScriptHashes []string     // CSP sources of the UI's inline scripts (web.ScriptHashes)
}

// logClock is the clock used by throttleLog; tests may replace it before New.
var logClock = time.Now

// throttleSweepThreshold is the map size above which throttleLog drops stale IPs.
var throttleSweepThreshold = 1024

// throttleLog limits a repeated audit line to once per IP per minute.
type throttleLog struct {
	mu        sync.Mutex
	last      map[string]time.Time
	now       func() time.Time
	lastSweep time.Time
}

func (t *throttleLog) shouldLog(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if at, ok := t.last[ip]; ok && now.Sub(at) < time.Minute {
		return false
	}
	if len(t.last) > throttleSweepThreshold && now.Sub(t.lastSweep) >= time.Minute {
		t.lastSweep = now
		for k, at := range t.last {
			if now.Sub(at) >= time.Minute {
				delete(t.last, k)
			}
		}
	}
	t.last[ip] = now
	return true
}

type server struct {
	d        Deps
	throttle *throttleLog
	guard    *fsGuard
}

// Spec §13 (FUSE): at most 4 file operations at once per container, each one
// given up after 10 s.
const (
	fsMaxPerContainer = 4
	fsTimeout         = 10 * time.Second
)

func New(d Deps) http.Handler {
	if d.OpenFS == nil {
		docker := d.Docker
		d.OpenFS = func(ctx context.Context, id string) (fsview.FS, error) { return fsview.For(ctx, docker, id) }
	}
	s := &server{
		d:        d,
		throttle: &throttleLog{last: map[string]time.Time{}, now: logClock},
		guard:    newFSGuard(fsMaxPerContainer, fsTimeout),
	}
	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.Handle("GET /api/meta", session(s.meta))
	mux.Handle("GET /api/containers", session(s.containers))
	mux.Handle("GET /api/containers/{id}/fs", session(s.fsList))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "")
	})
	if d.Static != nil {
		mux.Handle("/", d.Static)
	}
	return securityHeaders(originChecker{public: d.PublicOrigin, tls: d.TLS}.wrap(mux), d.ScriptHashes)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.d.Limiter.Allowed(ip) {
		if s.throttle.shouldLog(ip) {
			if err := s.d.Audit.Write(audit.Event{Action: "login.fail", Result: "rate_limited", Remote: ip}); err != nil {
				slog.Error("audit write", "err", err)
			}
		}
		writeError(w, http.StatusTooManyRequests, "rate_limited", "")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	err := dec.Decode(&body)
	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
		err = errors.New("trailing data")
	}
	if err != nil || !s.d.Verifier.Check(strings.TrimSpace(body.Token)) {
		s.d.Limiter.Fail(ip)
		if err := s.d.Audit.Write(audit.Event{Action: "login.fail", Result: "refused", Remote: ip}); err != nil {
			slog.Error("audit write", "err", err)
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}
	id, err := s.d.Sessions.Create()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(),
		MaxAge: int(auth.SessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) secureCookies() bool {
	return s.d.TLS || strings.HasPrefix(s.d.PublicOrigin, "https://")
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.d.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookies(), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) meta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.d.Version, "readOnly": s.d.ReadOnly})
}

func (s *server) containers(w http.ResponseWriter, r *http.Request) {
	list, err := s.d.Docker.ListContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", err.Error())
		return
	}
	if list == nil {
		list = []dockerx.Container{}
	}
	writeJSON(w, http.StatusOK, list)
}
```

`internal/api/fs.go` :

```go
package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// containerRef accepts Docker names and IDs only. The value goes into Docker
// API URLs: it must never carry "/" nor start with ".".
var containerRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// fsRequest reads {id} and ?path=, then resolves the container by Inspect
// (bounded by fsTimeout: a stuck daemon must not hang the request). Every
// later step uses info.ID, the full ID: the guard counts per container, and a
// container recreated under the same name cannot step in mid-request. On any
// failure it answers itself.
func (s *server) fsRequest(w http.ResponseWriter, r *http.Request) (info dockerx.ContainerInfo, p string, ok bool) {
	id := r.PathValue("id")
	if !containerRef.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "identifiant de conteneur invalide")
		return info, "", false
	}
	raw := r.URL.Query().Get("path")
	if strings.ContainsRune(raw, 0) {
		writeError(w, http.StatusNotFound, "not_found", "chemin invalide")
		return info, "", false
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
	defer cancel()
	info, err := s.d.Docker.Inspect(ctx, id)
	if err != nil {
		writeFSError(w, err)
		return info, "", false
	}
	return info, fsview.CleanPath(raw), true
}

type entryJSON struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"` // Unix bits 0o7777: permissions + setuid/setgid/sticky
	UID        int       `json:"uid"`
	GID        int       `json:"gid"`
	ModTime    time.Time `json:"modTime"`
	LinkTarget string    `json:"linkTarget,omitempty"`
}

func toEntryJSON(e fsview.Entry) entryJSON {
	return entryJSON{
		Name: e.Name, Type: string(e.Type), Size: e.Size, Mode: fsview.UnixPerm(e.Mode),
		UID: e.UID, GID: e.GID, ModTime: e.ModTime, LinkTarget: e.LinkTarget,
	}
}

// listingJSON: Type is "dir" for a directory. For any other path, Type gives
// the kind of the resolved path and Entries is empty: the UI then previews it.
type listingJSON struct {
	Path      string      `json:"path"`
	Type      string      `json:"type"`
	Entries   []entryJSON `json:"entries"`
	Truncated bool        `json:"truncated"`
}

func (s *server) fsList(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	res, release, err := guarded(s.guard, ctx, info.ID, func() (listingJSON, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return listingJSON{}, err
		}
		defer fsys.Close()
		entries, truncated, err := fsys.List(ctx, p)
		if errors.Is(err, fsview.ErrNotDir) {
			e, err := fsys.Stat(ctx, p)
			if err != nil {
				return listingJSON{}, err
			}
			return listingJSON{Path: p, Type: string(e.Type), Entries: []entryJSON{}}, nil
		}
		if err != nil {
			return listingJSON{}, err
		}
		out := listingJSON{Path: p, Type: string(fsview.TypeDir), Truncated: truncated, Entries: make([]entryJSON, 0, len(entries))}
		for _, e := range entries {
			out.Entries = append(out.Entries, toEntryJSON(e))
		}
		sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
		return out, nil
	}, nil)
	release()
	if err != nil {
		writeFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
```

- [ ] **Step 4: Vérifier**

Run: `go vet ./... && go test -race ./internal/api/ -v 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected : PASS. Les nouveaux tests : `TestFSListNeedsSession`, `TestFSList`, `TestFSListCleansPath`, `TestFSListOfAFile`, `TestFSListErrors`, `TestFSBadContainerRef`, `TestFSNULInPath`. Tous les tests du Socle passent aussi.

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -F - <<'EOF'
explorateur: route de listing des fichiers (GET /fs), Inspect d'abord, garde par ID complet

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 11: API : `GET /api/containers/{id}/fs/file` (aperçu)

**Où tester :** macOS.

**Files:**
- Create: `internal/api/fs_preview.go`, `internal/api/fs_preview_test.go`
- Modify: `internal/api/server.go` (une route)

**Interfaces:**
- Consumes: `fsRequest` (tâche 10) ; `guarded`, `writeFSError` (tâche 9) ; `fsview.ErrTooLarge`
- Produces:
  - `const previewMax = 2 << 20` ;
  - `previewType(b []byte) string` ;
  - réponse : le corps, `Content-Type` (`text/plain; charset=utf-8`, `image/png|jpeg|gif|webp`, ou `application/octet-stream` sans corps) et `ETag: "<sha256 hex>"`.

> **À comprendre :** l'aperçu ne doit jamais servir un type que le navigateur exécute (§7) : un SVG peut porter du JavaScript, un HTML aussi.
>
> - `http.DetectContentType` sert seulement à reconnaître quatre images matricielles.
> - Tout le reste est du texte brut, ou « binaire » s'il y a un octet nul dans les 8 premiers Kio.
> - `X-Content-Type-Options: nosniff`, posé par le Socle sur toute réponse, empêche le navigateur de deviner autre chose.
> - On lit au plus 2 Mio + 1 octet : un octet de trop suffit à savoir que le fichier dépasse. Le contenu est en mémoire quand le créneau est rendu.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/api/fs_preview_test.go` :

```go
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func preview(t *testing.T, content []byte) (*fixture, *http.Response, []byte) {
	t.Helper()
	f := newFixture(t, false)
	f.fs.AddDir("/d").AddFile("/d/f", content)
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d/f", "", f.session(t))
	return f, rec.Result(), rec.Body.Bytes()
}

func TestPreviewText(t *testing.T) {
	_, res, body := preview(t, []byte("hello\n"))
	sum := sha256.Sum256([]byte("hello\n"))
	if res.StatusCode != http.StatusOK || string(body) != "hello\n" ||
		res.Header.Get("Content-Type") != "text/plain; charset=utf-8" ||
		res.Header.Get("ETag") != `"`+hex.EncodeToString(sum[:])+`"` ||
		res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("%d %v %q", res.StatusCode, res.Header, body)
	}
}

func TestPreviewActiveContentIsText(t *testing.T) {
	for _, doc := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		`<!DOCTYPE html><html><script>alert(1)</script></html>`,
		`<?xml version="1.0"?><x/>`,
	} {
		if _, res, _ := preview(t, []byte(doc)); res.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("%.20q servi en %q", doc, res.Header.Get("Content-Type"))
		}
	}
}

func TestPreviewImage(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), 0, 0, 0, 13)
	_, res, body := preview(t, png)
	if res.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, png) {
		t.Fatalf("%q %q", res.Header.Get("Content-Type"), body)
	}
}

func TestPreviewBinaryHasNoBody(t *testing.T) {
	_, res, body := preview(t, []byte("ELF\x00\x01\x02"))
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/octet-stream" || len(body) != 0 {
		t.Fatalf("%d %q %q", res.StatusCode, res.Header.Get("Content-Type"), body)
	}
	// A NUL byte after the first 8 KiB does not make a binary.
	_, res, _ = preview(t, append([]byte(strings.Repeat("a", 8<<10)), 0))
	if res.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("NUL après 8 Kio : %q", res.Header.Get("Content-Type"))
	}
}

func TestPreviewSizeLimit(t *testing.T) {
	if _, res, body := preview(t, nil); res.StatusCode != http.StatusOK || len(body) != 0 || res.Header.Get("ETag") == "" {
		t.Fatalf("fichier vide : %d %v", res.StatusCode, res.Header)
	}
	if _, res, body := preview(t, bytes.Repeat([]byte("a"), previewMax)); res.StatusCode != http.StatusOK || len(body) != previewMax {
		t.Fatalf("exactement 2 Mio : %d (%d octets)", res.StatusCode, len(body))
	}
	f, _, _ := preview(t, nil)
	f.fs.AddFile("/d/big", bytes.Repeat([]byte("a"), previewMax+1))
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d/big", "", f.session(t))
	if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Fatalf("2 Mio + 1 : %d %s", rec.Code, rec.Body)
	}
}

func TestPreviewNotRegular(t *testing.T) {
	f, _, _ := preview(t, nil)
	rec := f.do("GET", "/api/containers/web/fs/file?path=/d", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "not_regular" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
```

Run: `go test ./internal/api/`
Expected: FAIL (`undefined: previewMax`).

- [ ] **Step 2: Écrire le code**

`internal/api/fs_preview.go` :

```go
package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// previewMax caps the preview (spec §6 "2 Mo", taken as 2 MiB).
const previewMax = 2 << 20

const binaryType = "application/octet-stream"

// fsFile serves the preview of a regular file (spec §6, §7): text/plain or
// one of four raster image types, never an active type. The ETag is the
// SHA-256 of the content: the If-Match of a later edit (plan Explorateur 3).
func (s *server) fsFile(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	b, release, err := guarded(s.guard, ctx, info.ID, func() ([]byte, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		defer fsys.Close()
		rc, err := fsys.Open(ctx, p)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, previewMax+1))
		if err != nil {
			return nil, err
		}
		if len(b) > previewMax {
			return nil, fmt.Errorf("%w : aperçu limité à %d octets", fsview.ErrTooLarge, previewMax)
		}
		return b, nil
	}, nil)
	release() // the content is in memory: nothing left to read
	if err != nil {
		writeFSError(w, err)
		return
	}
	sum := sha256.Sum256(b)
	ct := previewType(b)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:])+`"`)
	w.Header().Set("Content-Type", ct)
	if ct == binaryType {
		return // no text preview for a binary file: the UI offers the download
	}
	w.Write(b)
}

// previewType: PNG, JPEG, GIF or WebP as detected by http.DetectContentType;
// otherwise binary when a NUL byte appears in the first 8 KiB; otherwise
// text/plain, SVG and HTML included (spec §6, §7).
func previewType(b []byte) string {
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	}
	if bytes.IndexByte(b[:min(len(b), 8<<10)], 0) >= 0 {
		return binaryType
	}
	return "text/plain; charset=utf-8"
}
```

Dans `internal/api/server.go`, sous la route `GET /api/containers/{id}/fs` :

```go
	mux.Handle("GET /api/containers/{id}/fs/file", session(s.fsFile))
```

- [ ] **Step 3: Vérifier**

Run: `go vet ./... && go test -race ./internal/api/ -run Preview -v 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected : PASS (6 tests `TestPreview…`).

- [ ] **Step 4: Commit**

```bash
git add internal/api
git commit -F - <<'EOF'
explorateur: aperçu des fichiers (texte ou image, 2 Mio, ETag), SVG et HTML servis en texte

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 12: API : `GET /api/containers/{id}/fs/download` (fichier ou dossier)

**Où tester :** macOS (faux client : aucun vrai `GetArchive` dans les cas refusés) ; Linux root et non root, sans Docker, pour `fs_download_linux_test.go` (vue `procfs` réelle d'un process du test, `scripts/test-linux.sh` ou job CI `go`).

**Files:**
- Create: `internal/api/fs_download.go`, `internal/api/fs_download_test.go`, `internal/api/fs_download_linux_test.go`
- Modify: `internal/api/server.go` (une route), `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§5.4, §6, §7, §13)

**Interfaces:**
- Consumes: `fsRequest` (tâche 10) ; `guarded`, `writeFSError` (tâche 9) ; `FS.Stat`, `FS.Open`, `FS.StatNoLinks`, `FS.CheckArchive` ; `MemFS.ArchiveErr`, `MemFS.OpenFile` ; `fsview.SetCgroupReaderForTests` (tâche 6) ; `dockerx.Client.GetArchive`, `Fake.Archives`, `Fake.ArchiveCalls`, `Fake.Volumes` (tâche 3)
- Produces:
  - `setAttachment(w, name string)` ;
  - le paramètre `&check=1` (tous les contrôles, puis `204` sans rien lire) ;
  - côté tests : `filename(t, rec) string`, `escape(s string) string`, `stuckReader`.

> **À comprendre :** les deux cas suivent §5.4.
>
> - **Fichier.**
>   - Il passe par `fsview.Open` : contrôles `not_regular` et `forbidden_path`, liens suivis dans la racine.
>   - La vue est fermée dès l'ouverture (revue M3) : le fichier ouvert reste lisible sans elle.
> - **Dossier.**
>   - On prend le chemin **nettoyé**, sans aucun lien sur toute la chaîne (`StatNoLinks`, sinon `link_in_path`).
>   - Puis `CheckArchive` (tâche 7, revue C1) : ni bind de l'hôte, ni pseudo-système ou FUSE dans le tar.
>   - Enfin `GetArchive` de **cette** chaîne avec l'ID complet, appelé sous la garde, car un démon bloqué y attendrait.
>
> Les deux copies sont relayées en flux, et le créneau de la garde reste pris jusqu'à la fin de la copie (revue Codex n°4).
>
> Toutes les erreurs sont connues avant le premier octet : elles partent en JSON avec leur statut. `&check=1` s'arrête juste avant de lire (revue M2) : l'interface demande d'abord, puis laisse le navigateur télécharger, sans qu'un tar ait été lancé pour rien chez Docker.
>
> `fs_download_linux_test.go` (revue R1 iii, R4) fait passer le contrôle des montages par la route réelle. Il utilise `fsRequest`, l'`OpenFS` par défaut (`fsview.For`), `procfs` et `CheckArchive`.
> - Le « conteneur » est un `sleep` du test, rattaché au cgroup voulu par `SetCgroupReaderForTests` (qui panique hors d'un binaire de test ; ce test n'appelle pas `t.Parallel`, car l'injection est globale).
> - Un faux Docker le décrit, avec trois montages : un bind de `/proc`, un volume aux options illisibles, un pilote tiers.
> - Le test vérifie 403, `forbidden_path`, le nom du montage dans le détail et zéro `GetArchive`.
> - Il tourne sous Linux sans Docker : en root, en non-root et dans le job CI `go`.

- [ ] **Step 1: Écrire les tests qui échouent**

`internal/api/fs_download_test.go` :

```go
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// escape encodes a query value (spaces, newlines, '/' and all).
func escape(s string) string { return url.QueryEscape(s) }

func filename(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	disp, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
	if err != nil || disp != "attachment" {
		t.Fatalf("Content-Disposition %q : %v", rec.Header().Get("Content-Disposition"), err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	return params["filename"]
}

func TestDownloadFile(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("127.0.0.1"))
	rec := f.do("GET", "/api/containers/web/fs/download?path=/etc/hosts", "", f.session(t))
	if rec.Code != http.StatusOK || rec.Body.String() != "127.0.0.1" || filename(t, rec) != "hosts" {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	if f.fs.Closed.Load() != 1 || len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("fermetures %d, appels archive %v", f.fs.Closed.Load(), f.docker.ArchiveCalls)
	}
}

func TestDownloadOddFileName(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	for _, name := range []string{"rapport final.txt", "été-🎉.log", "a\"b;c.txt", "new\nline"} {
		f.fs.AddFile("/"+name, []byte("x"))
		rec := f.do("GET", "/api/containers/web/fs/download?path="+escape(name), "", c)
		if rec.Code != http.StatusOK || filename(t, rec) != name {
			t.Errorf("%q : %d %q", name, rec.Code, rec.Header().Get("Content-Disposition"))
		}
	}
}

func TestDownloadDirectoryAsTar(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc")
	f.docker.Archives[webID+":/etc"] = []byte("TAR")
	rec := f.do("GET", "/api/containers/web/fs/download?path=/etc/", "", f.session(t))
	if rec.Code != http.StatusOK || rec.Body.String() != "TAR" || filename(t, rec) != "etc.tar" {
		t.Fatalf("%d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	// Exactly the cleaned path, with the full ID (never the name "web").
	if len(f.docker.ArchiveCalls) != 1 || f.docker.ArchiveCalls[0] != webID+":/etc" {
		t.Fatalf("appels archive : %v", f.docker.ArchiveCalls)
	}
	f.docker.Archives[webID+":/"] = []byte("ROOT")
	if rec := f.do("GET", "/api/containers/web/fs/download?path=/", "", f.session(t)); filename(t, rec) != "rootfs.tar" {
		t.Fatalf("racine : %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestDownloadDirectoryThroughLink(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddLink("/link", "/etc").AddDir("/link/sub")
	rec := f.do("GET", "/api/containers/web/fs/download?path=/link/sub", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "link_in_path" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit pas être appelé : %v", f.docker.ArchiveCalls)
	}
}

func TestDownloadNotRegular(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddLink("/l", "/etc/hosts") // MemFS.Stat does not follow: a "symlink" entry is not regular
	rec := f.do("GET", "/api/containers/web/fs/download?path=/l", "", f.session(t))
	if rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "not_regular" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

// Review ruling C1: a tar that would cross a host bind, a pseudo filesystem
// or a FUSE mount is refused before any GetArchive (the real check, through
// the real view: fs_download_linux_test.go).
func TestDownloadDirectoryRefusedByMountCheck(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/host")
	f.fs.ArchiveErr = fmt.Errorf("%w : bind /proc -> /host/proc", fsview.ErrForbiddenPath)
	c := f.session(t)
	for _, q := range []string{"", "&check=1"} {
		rec := f.do("GET", "/api/containers/web/fs/download?path=/host"+q, "", c)
		var body apiError
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusForbidden || body.Error != "forbidden_path" ||
			!strings.Contains(body.Detail, "bind /proc -> /host/proc") {
			t.Errorf("%q : %d %s", q, rec.Code, rec.Body)
		}
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit jamais être appelé : %v", f.docker.ArchiveCalls)
	}
}

// Review ruling M2: check=1 runs every check and stops before GetArchive.
func TestDownloadCheckMode(t *testing.T) {
	f := newFixture(t, false)
	f.fs.AddDir("/etc").AddFile("/etc/hosts", []byte("x")).AddLink("/l", "/etc").AddDir("/l/sub")
	c := f.session(t)
	for _, p := range []string{"/etc", "/etc/hosts"} {
		if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path="+p, "", c); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Errorf("%s : %d %q", p, rec.Code, rec.Body)
		}
	}
	if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path=/l/sub", "", c); errCode(t, rec) != "link_in_path" {
		t.Errorf("lien : %d %s", rec.Code, rec.Body)
	}
	if len(f.docker.ArchiveCalls) != 0 {
		t.Fatalf("le contrôle ne doit pas lancer GetArchive : %v", f.docker.ArchiveCalls)
	}
}

// stuckReader is a file of a FUSE mount that never answers: its first Read
// says so on started, then waits for unblock.
type stuckReader struct {
	once             sync.Once
	started, unblock chan struct{}
}

func (r *stuckReader) Read([]byte) (int, error) {
	r.once.Do(func() { r.started <- struct{}{} })
	<-r.unblock
	return 0, io.EOF
}

func (r *stuckReader) Close() error { return nil }

// Review rulings (Codex 4, I3, M3): a download keeps its guard slot during the
// whole copy; the budget is per full ID (name and ID share it); the view is
// closed as soon as the file is open.
func TestDownloadKeepsItsSlotDuringTheCopy(t *testing.T) {
	f := newFixture(t, false)
	started, unblock := make(chan struct{}, fsMaxPerContainer), make(chan struct{})
	f.fs.AddFile("/stuck", nil)
	f.fs.OpenFile = func(string) (io.ReadCloser, error) {
		return &stuckReader{started: started, unblock: unblock}, nil
	}
	c := f.session(t)
	var wg sync.WaitGroup
	for range fsMaxPerContainer {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.do("GET", "/api/containers/web/fs/download?path=/stuck", "", c)
		}()
	}
	for range fsMaxPerContainer {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("les téléchargements n'ont pas commencé")
		}
	}
	if n := f.fs.Closed.Load(); n != fsMaxPerContainer {
		t.Errorf("vues fermées pendant la copie : %d, attendu %d", n, fsMaxPerContainer)
	}
	rec := f.do("GET", "/api/containers/"+webID+"/fs?path=/", "", c)
	if rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "internal" {
		t.Errorf("budget plein (par l'ID complet) : %d %s", rec.Code, rec.Body)
	}
	close(unblock)
	wg.Wait()
	if rec := f.do("GET", "/api/containers/web/fs?path=/", "", c); rec.Code != http.StatusOK {
		t.Fatalf("créneaux non rendus après la copie : %d %s", rec.Code, rec.Body)
	}
}
```

`internal/api/fs_download_linux_test.go` :

```go
//go:build linux

package api

// The mount check through the real wiring (review R1 iii, R4): fsRequest,
// the default OpenFS (fsview.For), procfs and CheckArchive, with a fake Docker
// whose "container" is one of the test's own processes. No real container,
// no real bind: GetArchive must never be called.

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

func TestDownloadMountCheckThroughTheRealView(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	t.Cleanup(fsview.SetCgroupReaderForTests(func(int) ([]byte, error) {
		return []byte("0::/docker/" + webID + "\n"), nil
	}))
	fake := &dockerx.Fake{Infos: map[string]dockerx.ContainerInfo{}, Archives: map[string][]byte{},
		Volumes: map[string]map[string]string{}}
	setMounts := func(mounts ...dockerx.Mount) {
		info := dockerx.ContainerInfo{ID: webID, Name: "web", Running: true, Pid: cmd.Process.Pid, Mounts: mounts}
		fake.Infos["web"], fake.Infos[webID] = info, info
	}
	f := &fixture{docker: fake}
	f.init(t, false, fake, nil) // nil: the real fsview.For
	c := f.session(t)

	refused := []struct {
		mount dockerx.Mount
		names []string
	}{
		{dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"}, []string{"bind /proc -> /host/proc"}},
		{dockerx.Mount{Type: "volume", Name: "gone", Destination: "/data", Driver: "local"}, []string{"gone", "/data"}},
		{dockerx.Mount{Type: "volume", Name: "ceph", Destination: "/c", Driver: "rexray/rbd"}, []string{"ceph", "/c"}},
	}
	for _, r := range refused {
		setMounts(r.mount)
		for _, q := range []string{"", "&check=1"} {
			rec := f.do("GET", "/api/containers/web/fs/download?path=/etc"+q, "", c)
			var body apiError
			json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusForbidden || body.Error != "forbidden_path" {
				t.Errorf("%+v %q : %d %s", r.mount, q, rec.Code, rec.Body)
				continue
			}
			for _, n := range r.names {
				if !strings.Contains(body.Detail, n) {
					t.Errorf("%+v : le détail %q ne nomme pas %q", r.mount, body.Detail, n)
				}
			}
		}
	}
	if len(fake.ArchiveCalls) != 0 {
		t.Fatalf("GetArchive ne doit jamais être appelé : %v", fake.ArchiveCalls)
	}
	// Control: the same process without dangerous mounts passes the check.
	setMounts(dockerx.Mount{Type: "bind", Source: "/srv/data", Destination: "/data"})
	if rec := f.do("GET", "/api/containers/web/fs/download?check=1&path=/etc", "", c); rec.Code != http.StatusNoContent {
		t.Fatalf("contrôle : %d %s", rec.Code, rec.Body)
	}
}
```

Run: `go test ./internal/api/ -run Download ; scripts/test-linux.sh ./internal/api/ -run ThroughTheRealView`
Expected: FAIL. Les tests compilent, mais la route n'existe pas encore (`404 not_found`) : `--- FAIL: TestDownloadFile`, `TestDownloadDirectoryRefusedByMountCheck`, `TestDownloadMountCheckThroughTheRealView`, etc.

- [ ] **Step 2: Écrire le code**

`internal/api/fs_download.go` :

```go
package api

import (
	"io"
	"mime"
	"net/http"
	"path"

	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

// fsDownload streams a regular file, or a directory as a tar (spec §5.4).
// With &check=1 it runs every check, stops before reading anything (no
// GetArchive) and answers 204: the UI asks first, then lets the browser
// download.
func (s *server) fsDownload(w http.ResponseWriter, r *http.Request) {
	info, p, ok := s.fsRequest(w, r)
	if !ok {
		return
	}
	check := r.URL.Query().Get("check") == "1"
	ctx := r.Context()
	type opened struct {
		rc  io.ReadCloser // nil in check mode
		dir bool
	}
	o, release, err := guarded(s.guard, ctx, info.ID, func() (opened, error) {
		fsys, err := s.d.OpenFS(ctx, info.ID)
		if err != nil {
			return opened{}, err
		}
		defer fsys.Close() // an opened file stays readable without the view
		e, err := fsys.Stat(ctx, p)
		if err != nil {
			return opened{}, err
		}
		if e.Type != fsview.TypeDir {
			rc, err := fsys.Open(ctx, p)
			if err != nil {
				return opened{}, err
			}
			if check {
				rc.Close()
				return opened{}, nil
			}
			return opened{rc: rc}, nil
		}
		// The cleaned request path, without any link in it, is exactly what
		// goes to GetArchive; and the tar must not cross a host bind, a pseudo
		// filesystem or a FUSE mount (CheckArchive).
		if _, err := fsys.StatNoLinks(ctx, p); err != nil {
			return opened{}, err
		}
		if err := fsys.CheckArchive(ctx, p); err != nil {
			return opened{}, err
		}
		if check {
			return opened{dir: true}, nil
		}
		rc, err := s.d.Docker.GetArchive(ctx, info.ID, p)
		if err != nil {
			return opened{}, err
		}
		return opened{rc: rc, dir: true}, nil
	}, func(o opened) {
		if o.rc != nil {
			o.rc.Close()
		}
	})
	defer release() // after the copy: a reader stuck on FUSE keeps its slot
	if err != nil {
		writeFSError(w, err)
		return
	}
	if check {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	defer o.rc.Close()
	name := path.Base(p)
	if o.dir {
		if p == "/" {
			name = "rootfs"
		}
		name += ".tar"
	}
	setAttachment(w, name)
	io.Copy(w, o.rc) // streamed, never held in memory; the client may leave
}

// setAttachment: always saved, never rendered (spec §5.4, §7).
func setAttachment(w http.ResponseWriter, name string) {
	cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if cd == "" {
		cd = "attachment"
	}
	w.Header().Set("Content-Disposition", cd)
	w.Header().Set("Content-Type", "application/octet-stream")
}
```

Dans `internal/api/server.go`, sous la route `…/fs/file` :

```go
	mux.Handle("GET /api/containers/{id}/fs/download", session(s.fsDownload))
```

- [ ] **Step 3: Vérifier**

Run: `go vet ./... && go test -race -count=3 ./internal/api/ && go test -race ./cmd/... ./internal/... 2>&1 | tail -9`
Expected : `ok` partout. `-count=3` repasse `TestDownloadKeepsItsSlotDuringTheCopy`, qui fait tourner des requêtes concurrentes.

Run (Linux, sur le vrai `procfs`) : `scripts/test-linux.sh ./internal/... && scripts/test-linux.sh ./internal/api/ -run ThroughTheRealView -v`
Expected : `ok` partout, et `--- PASS: TestDownloadMountCheckThroughTheRealView`.

- [ ] **Step 4: Mettre la spec à jour (§5.4, §6, §7, §13)**

§5.4, remplacer la puce « Conteneur actif : ouverture par `openat2` … relayé en `.tar` sans mise en mémoire. » par :

> - Conteneur actif : ouverture par `openat2` `RESOLVE_IN_ROOT | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS` sur `rootfd`, puis `fstatfs` du descripteur (`forbidden_path`, §5.1 étape 9). Vient ensuite le **contrôle des montages** (`FS.CheckArchive`) : Docker construit le tar avec les binds et volumes du conteneur montés dedans, et un bind du `/proc` de l'hôte y met `/proc/kcore` (254 Tio, dockerd bloqué : mesuré en revue). Deux contrôles :
>   - (a) la règle des binds de §5.2 étape 7 sur `Inspect.Mounts`, volumes `local` en `o=bind`/`rbind` compris, source non absolue refusée → `forbidden_path` qui nomme le montage. Pour ce tar, sont refusés aussi les volumes `local` dont `type=` est un pseudo-système ou FUSE, les volumes d'un pilote tiers et ceux dont les options ne se lisent pas (ce qu'ils montent est inconnu). Ces extensions ne valent pas, pour l'instant, pour l'explorateur d'un conteneur arrêté (§5.2) ;
>   - (b) `/proc/<pid>/mountinfo`, lu pour le process épinglé par le pidfd, qui doit être vivant après la lecture. On y cherche les montages de l'un des 15 types de §5.1 étape 9 ou FUSE (`fuse`, `fuseblk`, `fuse.*`). Un tel montage sur ou sous la destination d'un montage d'`Inspect.Mounts` refuse **tout** tar du conteneur, car dockerd remonte ces montages pour chaque tar. Un tel montage sur, sous ou au-dessus du dossier refuse ce tar. Dans les deux cas → `forbidden_path`. Une ligne illisible fait refuser.
>
>   Le `/` d'un conteneur actif est donc refusé, car son propre `/proc` est dessous. Puis `GetArchive` sur exactement le chemin nettoyé, relayé en `.tar` sans mise en mémoire ; le créneau de la garde (§13) reste pris jusqu'à la fin de la copie.
>
>   Limite assumée : entre le contrôle et `GetArchive`, un montage peut apparaître dans le conteneur. Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1. dockerd construit le tar à partir des montages configurés : mesuré sur Docker Desktop, il ne voit pas le `/proc` propre du conteneur. Et la règle « tout le conteneur » couvre ce qui est monté sous un montage Docker au moment du contrôle. À remesurer sur une VM Ubuntu.
> - `&check=1` : tous les contrôles, puis `204` sans rien lire (aucun `GetArchive`). L'interface vérifie ainsi avant de laisser le navigateur télécharger.

§6, ligne `GET /api/containers/{id}/fs/download?path=` : à la fin de la colonne « Rôle », ajouter « ; `&check=1` : contrôles seuls, `204` ; dossier d'un conteneur actif : `forbidden_path` si le tar traverserait un bind de l'hôte, un pseudo-système ou un FUSE (§5.4) ».

§7, après la puce « **Lecture seule** … », ajouter :

> - **Binds de l'hôte (conteneur actif)** : un conteneur lancé avec `-v /:/host` montre l'hôte dans l'explorateur ; par exemple, `/host/etc/shadow` est lisible. La lecture et le listing ne sont pas refusés : l'utilisateur de l'interface est de fait root sur l'hôte (début de §7), et les pseudo-systèmes et les périphériques restent refusés. Le tar d'un dossier, lui, est refusé pour tout le conteneur dès qu'un montage le rend dangereux, et le refus nomme le montage : bind sensible (`/`, `/proc`, `/sys` ou dessous, `/dev` : §5.2 étape 7), volume `type=` pseudo/FUSE, volume d'un pilote tiers, volume aux options illisibles (§5.4).

§13, remplacer la ligne « Bind d'un dossier hôte contenant un sous-montage `/proc` ou `/sys` (rbind) | … » par :

> | Bind d'un dossier hôte contenant un sous-montage `/proc` ou `/sys` (rbind) | conteneur actif : les sous-montages présents dans le conteneur sont vus par `mountinfo` et refusent tout tar (§5.4). Avec la propagation `rprivate` par défaut, un montage ajouté **plus tard** sur l'hôte sous la source n'apparaît pas dans le conteneur et n'est pas détecté, alors que dockerd pourrait le voir en remontant le bind. Conteneur arrêté : non détecté en v0.1 (`Inspect.Mounts` ne montre que la `Source`) |

puis ajouter trois lignes :

> | Montage FUSE ou pseudo-système sous un dossier téléchargé : dockerd le lit pour construire le tar et peut rester bloqué, conteneur verrouillé | refus avant `GetArchive` par `Inspect.Mounts` et `mountinfo` (§5.4) ; nos propres lectures restent sous la garde (4 par conteneur, 10 s) |
> | Course entre le contrôle des montages et `GetArchive` (conteneur actif) | Monter quoi que ce soit dans un conteneur actif exige `CAP_SYS_ADMIN` dans le conteneur (conteneur privilégié, ou `--cap-add SYS_ADMIN` : par exemple rclone ou s3fs avec `/dev/fuse`), ou root sur l'hôte (propagation `rshared`) ; limite assumée pour la v0.1 ; dockerd archive à partir des montages configurés ; règle « tout le conteneur » (§5.4) ; à remesurer sur une VM Ubuntu |
> | Noms de fichiers non UTF-8 : JSON les transporte avec U+FFFD, le fichier ne s'ouvre plus depuis l'interface (`not_found`) | limite v0.1 ; le listing ne casse pas |

- [ ] **Step 5: Commit**

```bash
git add internal/api docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: téléchargement d'un fichier ou d'un dossier, contrôle des montages, mode check, créneau tenu pendant la copie

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 13: Interface : client des routes de fichiers et fonctions de chemins

**Où tester :** macOS (Node via `export PATH=/opt/homebrew/bin:$PATH`).

**Files:**
- Create: `web/src/lib/files.ts`, `web/src/lib/files.test.ts`
- Modify: `web/src/lib/api.ts`, `web/src/lib/api.test.ts`, `web/src/lib/i18n/fr.ts`, `web/src/lib/i18n/en.ts`

**Interfaces:**
- Consumes: routes des tâches 10 à 12 (dont `&check=1`) ; `ApiError`, `api` (Socle) ; `t` (Socle)
- Produces (`api.ts`) : `type EntryType = 'file' | 'dir' | 'symlink' | 'other'` ; `type Entry = { name; type; size; mode; uid; gid; modTime; linkTarget? }` ; `type Listing = { path; type; entries; truncated }` ; `type Preview = { kind: 'text'; text; etag } | { kind: 'image'; blob; etag } | { kind: 'binary' }` ; `listDir(id, path): Promise<Listing>` ; `previewFile(id, path): Promise<Preview>` ; `downloadUrl(id, path): string` ; `checkDownload(url): Promise<void>`. (`files.ts`) : `normalize`, `joinPath`, `parentPath`, `baseName`, `resolveLinkTarget(dir, target)`, `breadcrumbs(p)`, `sortEntries(entries)`, `formatMode(type, mode)`, `formatSize(n)`, `downloadErrorMessage(code, detail?)` (message d'un téléchargement de dossier refusé, avec le détail qui nomme le montage). Clés i18n `container.*` et `files.*`.

> **À comprendre :** les chemins du conteneur ne sont ici que du texte, pour afficher et naviguer ; c'est l'agent qui les résout dans la racine. `checkDownload` existe parce qu'un lien `<a download>` vers une réponse d'erreur donne au mieux « échec du téléchargement » sans explication. On demande d'abord `&check=1` : l'agent fait tous ses contrôles et répond `204` ou l'erreur en JSON, sans rien lire ni lancer de tar chez Docker (revue M2). Ensuite seulement, le navigateur télécharge par navigation (`Content-Disposition: attachment` : la page reste affichée).

- [ ] **Step 1: Écrire les tests qui échouent**

`web/src/lib/files.test.ts` :

```ts
import { describe, expect, it } from 'vitest';
import type { Entry } from './api.ts';
import {
	baseName,
	breadcrumbs,
	downloadErrorMessage,
	formatMode,
	formatSize,
	joinPath,
	normalize,
	parentPath,
	resolveLinkTarget,
	sortEntries
} from './files.ts';
import { errorMessage, t } from './i18n/index.ts';

const e = (name: string, type: Entry['type']): Entry => ({
	name, type, size: 0, mode: 0o644, uid: 0, gid: 0, modTime: '2026-10-08T09:00:00Z'
});

describe('paths', () => {
	it('normalizes', () => {
		expect(normalize('')).toBe('/');
		expect(normalize('etc//x/./y/')).toBe('/etc/x/y');
		expect(normalize('/../../etc')).toBe('/etc');
	});
	it('joins, finds the parent and the base name', () => {
		expect(joinPath('/', 'etc')).toBe('/etc');
		expect(joinPath('/etc', 'hosts')).toBe('/etc/hosts');
		expect(parentPath('/usr/lib')).toBe('/usr');
		expect(parentPath('/')).toBe('/');
		expect(baseName('/etc/hosts')).toBe('hosts');
		expect(baseName('/')).toBe('/');
	});
	it('resolves link targets as text', () => {
		expect(resolveLinkTarget('/etc', '../usr/lib/os-release')).toBe('/usr/lib/os-release');
		expect(resolveLinkTarget('/usr/bin', '/bin/busybox')).toBe('/bin/busybox');
		expect(resolveLinkTarget('/', '../../x')).toBe('/x');
		expect(resolveLinkTarget('/a', 'b/./c/')).toBe('/a/b/c');
	});
	it('builds breadcrumbs', () => {
		expect(breadcrumbs('/')).toEqual([{ name: '/', path: '/' }]);
		expect(breadcrumbs('/usr/lib')).toEqual([
			{ name: '/', path: '/' },
			{ name: 'usr', path: '/usr' },
			{ name: 'lib', path: '/usr/lib' }
		]);
	});
});

describe('sortEntries', () => {
	it('puts directories first, then sorts by name', () => {
		const sorted = sortEntries([e('b.txt', 'file'), e('z', 'dir'), e('a', 'symlink'), e('c', 'dir')]);
		expect(sorted.map((x) => x.name)).toEqual(['c', 'z', 'a', 'b.txt']);
	});
});

describe('formatMode', () => {
	it('formats like ls', () => {
		expect(formatMode('dir', 0o755)).toBe('drwxr-xr-x');
		expect(formatMode('file', 0o644)).toBe('-rw-r--r--');
		expect(formatMode('symlink', 0o777)).toBe('lrwxrwxrwx');
		expect(formatMode('other', 0o600)).toBe('?rw-------');
	});
	it('shows setuid, setgid and sticky', () => {
		expect(formatMode('file', 0o4755)).toBe('-rwsr-xr-x');
		expect(formatMode('file', 0o4644)).toBe('-rwSr--r--');
		expect(formatMode('file', 0o2755)).toBe('-rwxr-sr-x');
		expect(formatMode('dir', 0o1777)).toBe('drwxrwxrwt');
		expect(formatMode('dir', 0o1776)).toBe('drwxrwxrwT');
	});
});

describe('formatSize', () => {
	it('uses binary units', () => {
		expect(formatSize(0)).toBe(`0 ${t('files.unit.b')}`);
		expect(formatSize(1023)).toBe(`1023 ${t('files.unit.b')}`);
		expect(formatSize(1536)).toBe(`1.5 ${t('files.unit.kib')}`);
		expect(formatSize(2 * 1024 * 1024)).toBe(`2.0 ${t('files.unit.mib')}`);
	});
});

describe('downloadErrorMessage', () => {
	it('explains a refused directory download and names the mount', () => {
		expect(downloadErrorMessage('forbidden_path', 'chemin interdit : bind /proc -> /host/proc')).toBe(
			`${t('files.archiveForbidden')} (chemin interdit : bind /proc -> /host/proc)`
		);
		expect(downloadErrorMessage('forbidden_path')).toBe(t('files.archiveForbidden'));
		expect(downloadErrorMessage('link_in_path', 'x')).toBe(t('files.linkDirHint'));
		expect(downloadErrorMessage('docker_unavailable', 'dial')).toBe(errorMessage('docker_unavailable'));
	});
});
```

Dans `web/src/lib/api.test.ts`, remplacer la ligne d'import de `./api` par :

```ts
import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile } from './api';
```

et ajouter à la fin :

```ts
describe('file routes', () => {
	it('lists a directory with an encoded id and path', async () => {
		const fetchMock = vi.fn<typeof fetch>(
			async () => new Response(JSON.stringify({ path: '/a b', type: 'dir', entries: [], truncated: false }))
		);
		vi.stubGlobal('fetch', fetchMock);
		await expect(listDir('web', '/a b')).resolves.toMatchObject({ path: '/a b', type: 'dir' });
		expect(fetchMock.mock.calls[0][0]).toBe('/api/containers/web/fs?path=%2Fa%20b');
	});

	it('builds the download URL', () => {
		expect(downloadUrl('web', '/etc/my file')).toBe('/api/containers/web/fs/download?path=%2Fetc%2Fmy%20file');
	});

	it('previews text, images and binaries', async () => {
		stubFetch(async () => new Response('hello', { headers: { 'Content-Type': 'text/plain; charset=utf-8', ETag: '"abc"' } }));
		await expect(previewFile('web', '/f')).resolves.toEqual({ kind: 'text', text: 'hello', etag: '"abc"' });

		stubFetch(async () => new Response(new Uint8Array([137, 80, 78, 71]), { headers: { 'Content-Type': 'image/png', ETag: '"x"' } }));
		const img = await previewFile('web', '/i.png');
		expect(img.kind).toBe('image');
		if (img.kind === 'image') expect(img.blob.size).toBe(4);

		stubFetch(async () => new Response(null, { headers: { 'Content-Type': 'application/octet-stream' } }));
		await expect(previewFile('web', '/bin')).resolves.toEqual({ kind: 'binary' });
	});

	it('maps preview errors', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'too_large' }), { status: 413 }));
		await expect(previewFile('web', '/big')).rejects.toMatchObject({ status: 413, code: 'too_large' });
	});

	it('checks a download without starting it', async () => {
		stubFetch(async () => new Response(JSON.stringify({ error: 'link_in_path' }), { status: 422 }));
		await expect(checkDownload('/api/x?path=%2Fl')).rejects.toMatchObject({ code: 'link_in_path' });
		const fetchMock = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
		vi.stubGlobal('fetch', fetchMock);
		await expect(checkDownload('/api/x?path=%2Fetc')).resolves.toBeUndefined();
		expect(fetchMock.mock.calls[0][0]).toBe('/api/x?path=%2Fetc&check=1');
	});
});
```

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run`
Expected : FAIL (`files.ts` introuvable, `listDir` non exporté).

- [ ] **Step 2: Les textes**

Dans `web/src/lib/i18n/fr.ts`, juste avant `'error.unauthorized'`, insérer :

```ts
	'container.tab.files': 'Fichiers',
	'container.tab.changes': 'Changements',
	'container.tab.logs': 'Logs',
	'container.tab.xray': 'Radiographie',
	'container.soon': 'bientôt',
	'container.stoppedSoon':
		"Conteneur arrêté : l'explorateur des conteneurs arrêtés arrive à l'étape suivante. Démarrez-le pour explorer ses fichiers.",
	'files.path': 'Chemin',
	'files.parent': 'Dossier parent',
	'files.loading': 'Chargement…',
	'files.empty': 'Dossier vide',
	'files.truncated': 'Seules les 10 000 premières entrées sont affichées.',
	'files.inaccessible': 'Dossier inaccessible : pseudo-système de fichiers (/proc, /sys…).',
	'files.col.name': 'Nom',
	'files.col.size': 'Taille',
	'files.col.mode': 'Droits',
	'files.col.owner': 'Propriétaire',
	'files.col.modified': 'Modifié',
	'files.selectFile': 'Choisissez un fichier pour voir son aperçu.',
	'files.download': 'Télécharger',
	'files.downloadDir': 'Télécharger ce dossier (.tar)',
	'files.binary': "Fichier binaire : pas d'aperçu.",
	'files.tooLarge': "Plus de 2 Mio : pas d'aperçu. Téléchargez-le.",
	'files.linkDirHint': "Ce chemin passe par un lien symbolique : ouvrez d'abord sa cible, puis téléchargez.",
	'files.archiveForbidden':
		"Téléchargement refusé : ce dossier, ou le conteneur, expose un pseudo-système de fichiers (/proc, /sys…), un montage FUSE ou un dossier sensible de l'hôte. Téléchargez un fichier, ou un sous-dossier si le refus ne porte que sur ce dossier.",
	'files.reload': 'Recharger',
	'files.unit.b': 'o',
	'files.unit.kib': 'Kio',
	'files.unit.mib': 'Mio',
	'files.unit.gib': 'Gio',
	'files.unit.tib': 'Tio',
```

Dans `web/src/lib/i18n/en.ts`, au même endroit :

```ts
	'container.tab.files': 'Files',
	'container.tab.changes': 'Changes',
	'container.tab.logs': 'Logs',
	'container.tab.xray': 'X-ray',
	'container.soon': 'soon',
	'container.stoppedSoon':
		'Stopped container: the explorer for stopped containers comes in the next step. Start it to explore its files.',
	'files.path': 'Path',
	'files.parent': 'Parent folder',
	'files.loading': 'Loading…',
	'files.empty': 'Empty folder',
	'files.truncated': 'Only the first 10,000 entries are shown.',
	'files.inaccessible': 'Inaccessible folder: pseudo filesystem (/proc, /sys…).',
	'files.col.name': 'Name',
	'files.col.size': 'Size',
	'files.col.mode': 'Permissions',
	'files.col.owner': 'Owner',
	'files.col.modified': 'Modified',
	'files.selectFile': 'Pick a file to preview it.',
	'files.download': 'Download',
	'files.downloadDir': 'Download this folder (.tar)',
	'files.binary': 'Binary file: no preview.',
	'files.tooLarge': 'Over 2 MiB: no preview. Download it.',
	'files.linkDirHint': 'This path goes through a symbolic link: open its target first, then download.',
	'files.archiveForbidden':
		'Download refused: this folder, or the container, exposes a pseudo filesystem (/proc, /sys…), a FUSE mount or a sensitive host folder. Download a file, or a subfolder if the refusal only concerns this folder.',
	'files.reload': 'Reload',
	'files.unit.b': 'B',
	'files.unit.kib': 'KiB',
	'files.unit.mib': 'MiB',
	'files.unit.gib': 'GiB',
	'files.unit.tib': 'TiB',
```

- [ ] **Step 3: Écrire le code**

`web/src/lib/api.ts` (contenu complet ; `api()` lit désormais l'erreur par `errorOf`, comportement inchangé) :

```ts
export class ApiError extends Error {
	constructor(
		public status: number,
		public code: string,
		public detail?: string
	) {
		super(code);
	}
}

async function errorOf(res: Response): Promise<ApiError> {
	const body = await res.json().catch(() => null);
	return new ApiError(res.status, body?.error ?? 'unknown', body?.detail);
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
	let res: Response;
	try {
		res = await fetch(`/api${path}`, {
			credentials: 'same-origin',
			...init,
			headers: { ...(init.body !== undefined ? { 'Content-Type': 'application/json' } : {}), ...(init.headers ?? {}) }
		});
	} catch {
		throw new ApiError(0, 'network');
	}
	if (res.status === 204) return undefined as T;
	if (!res.ok) throw await errorOf(res);
	return (await res.json().catch(() => null)) as T;
}

export type Port = { private: number; public?: number; type: string; ip?: string };
export type Container = {
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	created: string;
	ports: Port[];
};
export type Meta = { version: string; readOnly: boolean };

export const login = (token: string) =>
	api<{ ok: boolean }>('/login', { method: 'POST', body: JSON.stringify({ token }) });
export const logout = () => api<void>('/logout', { method: 'POST' });
export const getMeta = () => api<Meta>('/meta');
export const listContainers = () => api<Container[]>('/containers');

export type EntryType = 'file' | 'dir' | 'symlink' | 'other';
export type Entry = {
	name: string;
	type: EntryType;
	size: number;
	mode: number; // Unix bits 0o7777
	uid: number;
	gid: number;
	modTime: string;
	linkTarget?: string;
};
// type is "dir" for a directory; for another path, entries is empty.
export type Listing = { path: string; type: EntryType; entries: Entry[]; truncated: boolean };
export type Preview =
	| { kind: 'text'; text: string; etag: string }
	| { kind: 'image'; blob: Blob; etag: string }
	| { kind: 'binary' };

const fsPath = (id: string, route: string, path: string) =>
	`/containers/${encodeURIComponent(id)}/fs${route}?path=${encodeURIComponent(path)}`;

export const listDir = (id: string, path: string) => api<Listing>(fsPath(id, '', path));
export const downloadUrl = (id: string, path: string) => `/api${fsPath(id, '/download', path)}`;

// GET of a non-JSON route: errors still arrive as JSON.
async function get(url: string, signal?: AbortSignal): Promise<Response> {
	let res: Response;
	try {
		res = await fetch(url, { credentials: 'same-origin', signal });
	} catch {
		throw new ApiError(0, 'network');
	}
	if (!res.ok) throw await errorOf(res);
	return res;
}

export async function previewFile(id: string, path: string): Promise<Preview> {
	const res = await get(`/api${fsPath(id, '/file', path)}`);
	const type = res.headers.get('Content-Type') ?? '';
	const etag = res.headers.get('ETag') ?? '';
	if (/^image\/(png|jpeg|gif|webp)$/.test(type)) return { kind: 'image', blob: await res.blob(), etag };
	if (type.startsWith('text/plain')) return { kind: 'text', text: await res.text(), etag };
	return { kind: 'binary' };
}

// With &check=1 the agent runs every check of a download, then answers 204
// without reading anything (no tar started at Docker). The UI checks first,
// shows the error if any, otherwise lets the browser download by navigation.
export async function checkDownload(url: string): Promise<void> {
	await get(`${url}&check=1`);
}
```

`web/src/lib/files.ts` :

```ts
import type { Entry, EntryType } from './api.ts';
import { errorMessage, t } from './i18n/index.ts';

// Container paths are handled as text here, for display and navigation only:
// the agent resolves them inside the container root.

/** Absolute, without ".", "..", empty or trailing components ("/" stays "/"). */
export function normalize(p: string): string {
	const out: string[] = [];
	for (const c of p.split('/')) {
		if (c === '' || c === '.') continue;
		if (c === '..') out.pop();
		else out.push(c);
	}
	return '/' + out.join('/');
}

export const joinPath = (dir: string, name: string) => normalize(`${dir}/${name}`);
export const parentPath = (p: string) => normalize(`${p}/..`);
export const baseName = (p: string) => normalize(p).split('/').pop() || '/';

/** A link's target as a path to open: absolute as is, relative from the link's directory. */
export function resolveLinkTarget(dir: string, target: string): string {
	return target.startsWith('/') ? normalize(target) : joinPath(dir, target);
}

export function breadcrumbs(p: string): { name: string; path: string }[] {
	const crumbs = [{ name: '/', path: '/' }];
	let acc = '';
	for (const c of normalize(p).split('/').filter(Boolean)) {
		acc += '/' + c;
		crumbs.push({ name: c, path: acc });
	}
	return crumbs;
}

/** Directories first, then by name (code-point order, like the agent). */
export function sortEntries(entries: Entry[]): Entry[] {
	return [...entries].sort(
		(a, b) => Number(b.type === 'dir') - Number(a.type === 'dir') || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)
	);
}

const TYPE_CHAR: Record<EntryType, string> = { dir: 'd', symlink: 'l', file: '-', other: '?' };

/** ls-style permissions, e.g. "drwxr-sr-x"; s/S and t/T for setuid, setgid, sticky. */
export function formatMode(type: EntryType, mode: number): string {
	const bit = (m: number, c: string) => (mode & m ? c : '-');
	const exec = (m: number, special: number, c: string) =>
		mode & special ? (mode & m ? c : c.toUpperCase()) : bit(m, 'x');
	return (
		TYPE_CHAR[type] +
		bit(0o400, 'r') + bit(0o200, 'w') + exec(0o100, 0o4000, 's') +
		bit(0o040, 'r') + bit(0o020, 'w') + exec(0o010, 0o2000, 's') +
		bit(0o004, 'r') + bit(0o002, 'w') + exec(0o001, 0o1000, 't')
	);
}

/** Size in binary units: "0 o", "1.5 Kio" (fr) / "1.5 KiB" (en). */
export function formatSize(n: number): string {
	const units = [t('files.unit.b'), t('files.unit.kib'), t('files.unit.mib'), t('files.unit.gib'), t('files.unit.tib')];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return i === 0 ? `${v} ${units[0]}` : `${v.toFixed(1)} ${units[i]}`;
}

/**
 * Message for a refused directory download. forbidden_path there comes from the
 * mount check (spec §5.4): its detail names the mount, shown as is (escaped by
 * Svelte like any container content).
 */
export function downloadErrorMessage(code: string, detail?: string): string {
	if (code === 'link_in_path') return t('files.linkDirHint');
	if (code === 'forbidden_path') return detail ? `${t('files.archiveForbidden')} (${detail})` : t('files.archiveForbidden');
	return errorMessage(code);
}
```

- [ ] **Step 4: Vérifier**

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run && npm run check && npm run lint`
Expected : PASS (32 tests Vitest, dont le test i18n « mêmes clés »), 0 erreur `svelte-check`, `eslint` muet.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib
git commit -F - <<'EOF'
explorateur: client des routes de fichiers, chemins, droits et tailles mis en forme

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 14: Interface : page `/c/[id]`, explorateur et aperçu

**Où tester :** macOS (`svelte-check`, lint, Vitest, build). L'essai dans le navigateur suit à la tâche 15.

**Files:**
- Create: `web/src/lib/components/FileBrowser.svelte`, `web/src/lib/components/FilePreview.svelte`, `web/src/routes/c/[id]/+page.svelte`
- Modify: `web/src/routes/+page.svelte` (lien vers `/c/{id}`)

**Interfaces:**
- Consumes: tout `api.ts` et `files.ts` (tâche 13) ; `listContainers`, `stateLabel` (Socle)
- Produces: route `/c/[id]?path=<dossier>&file=<fichier>` ; composants `<FileBrowser id>` et `<FilePreview id path entry?>`

> **À comprendre :** l'état de l'explorateur vit dans l'URL (`?path=` le dossier, `&file=` le fichier ouvert) : le bouton retour et le rechargement marchent. Chaque `$effect` qui lance une requête pose un drapeau `current` et le baisse au nettoyage, pour qu'une réponse lente d'une navigation précédente n'écrase pas l'état (même patron que le `+layout.svelte` du Socle). Un clic sur un lien navigue vers sa cible (§5.1 étape 8) : si la cible n'est pas un dossier, la liste répond `type: "file"` et la page affiche son dossier parent avec l'aperçu. Une image est montrée par une URL `blob:` (la CSP autorise `img-src blob:`), révoquée au changement de fichier ; le texte passe par l'échappement de Svelte dans un `<pre>` (`{@html}` interdit). `forbidden_path` s'affiche comme « dossier inaccessible » (§5.1 étape 9) au listing. Au téléchargement d'un dossier, `downloadErrorMessage` donne le message `files.archiveForbidden` (« téléchargez un fichier, ou un sous-dossier si le refus ne porte que sur ce dossier ») suivi du détail de l'agent, qui nomme le montage. Le détail passe par l'échappement de Svelte comme tout contenu de conteneur. Les lignes du tableau sont indexées par position et non par nom : deux noms non UTF-8 peuvent devenir identiques une fois passés en JSON (Q6). Le bouton « Recharger » incrémente un compteur lu par l'effet : naviguer vers la même URL ne relancerait pas la requête. Au téléphone, les deux colonnes s'empilent.

- [ ] **Step 1: Les composants**

`web/src/lib/components/FilePreview.svelte` :

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { ApiError, checkDownload, downloadUrl, previewFile, type Entry, type Preview } from '#lib/api.ts';
	import { baseName, formatMode, formatSize } from '#lib/files.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';

	let { id, path, entry }: { id: string; path: string; entry?: Entry } = $props();

	let preview = $state<Preview | null>(null);
	let imageUrl = $state('');
	let error = $state('');
	let loading = $state(false);
	let downloadError = $state('');

	$effect(() => {
		const wanted = path;
		let current = true;
		let url = '';
		preview = null;
		imageUrl = '';
		error = '';
		downloadError = '';
		loading = true;
		previewFile(id, wanted)
			.then((p) => {
				if (!current) return;
				preview = p;
				if (p.kind === 'image') imageUrl = url = URL.createObjectURL(p.blob);
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				const code = e instanceof ApiError ? e.code : 'unknown';
				error = code === 'too_large' ? t('files.tooLarge') : errorMessage(code);
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
			if (url) URL.revokeObjectURL(url);
		};
	});

	async function download() {
		downloadError = '';
		const url = downloadUrl(id, path);
		try {
			await checkDownload(url);
			location.assign(url); // Content-Disposition: attachment, the page stays
		} catch (e) {
			downloadError = errorMessage(e instanceof ApiError ? e.code : 'unknown');
		}
	}
</script>

<header>
	<h2>{baseName(path)}</h2>
	{#if entry}
		<p class="meta">
			{formatSize(entry.size)} · <code>{formatMode(entry.type, entry.mode)}</code> · {entry.uid}:{entry.gid} ·
			{new Date(entry.modTime).toLocaleString()}
		</p>
	{/if}
	<button type="button" onclick={download}>{t('files.download')}</button>
	{#if downloadError}<p role="alert">{downloadError}</p>{/if}
</header>

{#if loading}
	<p>{t('files.loading')}</p>
{:else if error}
	<p role="alert">{error}</p>
{:else if preview?.kind === 'text'}
	<pre>{preview.text}</pre>
{:else if preview?.kind === 'image' && imageUrl}
	<img src={imageUrl} alt={baseName(path)} />
{:else if preview?.kind === 'binary'}
	<p>{t('files.binary')}</p>
{/if}

<style>
	header { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: baseline; }
	h2 { font-size: 1.1rem; margin: 0; word-break: break-all; }
	.meta { margin: 0; opacity: 0.8; font-size: 0.9rem; }
	pre { overflow: auto; max-height: 70vh; padding: 0.75rem; background: #f6f6f6; border-radius: 0.25rem; white-space: pre; }
	img { max-width: 100%; height: auto; }
	[role='alert'] { color: #b00020; }
</style>
```

`web/src/lib/components/FileBrowser.svelte` :

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, checkDownload, downloadUrl, listDir, type Entry, type Listing } from '#lib/api.ts';
	import {
		breadcrumbs,
		downloadErrorMessage,
		formatMode,
		formatSize,
		joinPath,
		parentPath,
		resolveLinkTarget,
		sortEntries
	} from '#lib/files.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import FilePreview from '#lib/components/FilePreview.svelte';

	let { id }: { id: string } = $props();

	// The URL carries the state (?path=dir&file=selected): back button and links work.
	let dir = $derived(page.url.searchParams.get('path') || '/');
	let file = $derived(page.url.searchParams.get('file'));
	let listing = $state<Listing | null>(null);
	let error = $state('');
	let loading = $state(false);
	let downloadError = $state('');
	let reloads = $state(0); // bumped by the Reload button: same URL, new request
	let entries = $derived(listing ? sortEntries(listing.entries) : []);
	let selected = $derived(file ? entries.find((e) => joinPath(dir, e.name) === file) : undefined);

	const href = (path: string, selectedFile?: string) =>
		'?' + new URLSearchParams(selectedFile ? { path, file: selectedFile } : { path });

	function go(path: string, selectedFile?: string, replace = false) {
		goto(href(path, selectedFile), { replace, reset: false }).catch(() => {});
	}

	$effect(() => {
		const wanted = dir;
		void reloads; // dependency: the Reload button re-runs this effect
		let current = true;
		loading = true;
		error = '';
		downloadError = '';
		listDir(id, wanted)
			.then((l) => {
				if (!current) return;
				// A link to a file, or a deep link: show its folder and preview it.
				if (l.type !== 'dir') go(parentPath(l.path), l.path, true);
				else listing = l;
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				listing = null;
				const code = e instanceof ApiError ? e.code : 'unknown';
				error = code === 'forbidden_path' ? t('files.inaccessible') : errorMessage(code);
			})
			.finally(() => {
				if (current) loading = false;
			});
		return () => {
			current = false;
		};
	});

	function open(e: Entry) {
		const p = joinPath(dir, e.name);
		if (e.type === 'dir') go(p);
		else if (e.type === 'symlink' && e.linkTarget !== undefined) go(resolveLinkTarget(dir, e.linkTarget));
		else go(dir, p);
	}

	async function downloadDir() {
		downloadError = '';
		const url = downloadUrl(id, dir);
		try {
			await checkDownload(url);
			location.assign(url);
		} catch (e) {
			downloadError =
				e instanceof ApiError ? downloadErrorMessage(e.code, e.detail) : downloadErrorMessage('unknown');
		}
	}
</script>

<div class="browser">
	<section class="listing">
		<nav class="crumbs" aria-label={t('files.path')}>
			{#each breadcrumbs(dir) as c, i (c.path)}
				{#if i > 1}<span aria-hidden="true">/</span>{/if}
				<a href={href(c.path)}>{c.name}</a>
			{/each}
		</nav>
		<div class="tools">
			<button type="button" onclick={downloadDir}>{t('files.downloadDir')}</button>
		</div>
		{#if downloadError}<p role="alert">{downloadError}</p>{/if}

		{#if error}
			<p role="alert">{error}</p>
			<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
		{:else if loading && !listing}
			<p>{t('files.loading')}</p>
		{:else if listing}
			{#if listing.truncated}<p class="note">{t('files.truncated')}</p>{/if}
			<div class="scroll">
				<table>
					<thead>
						<tr>
							<th scope="col">{t('files.col.name')}</th>
							<th scope="col">{t('files.col.size')}</th>
							<th scope="col">{t('files.col.mode')}</th>
							<th scope="col">{t('files.col.owner')}</th>
							<th scope="col">{t('files.col.modified')}</th>
						</tr>
					</thead>
					<tbody>
						{#if dir !== '/'}
							<tr>
								<td colspan="5"><a href={href(parentPath(dir))}>.. ({t('files.parent')})</a></td>
							</tr>
						{/if}
						{#each entries as e, i (i)}
							<tr class:selected={selected === e}>
								<td>
									<button type="button" class="name" onclick={() => open(e)}>
										{e.name}{e.type === 'dir' ? '/' : ''}
									</button>
									{#if e.type === 'symlink'}<span class="target">→ {e.linkTarget}</span>{/if}
								</td>
								<td>{e.type === 'dir' ? '' : formatSize(e.size)}</td>
								<td><code>{formatMode(e.type, e.mode)}</code></td>
								<td>{e.uid}:{e.gid}</td>
								<td>{new Date(e.modTime).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			{#if entries.length === 0}<p>{t('files.empty')}</p>{/if}
		{/if}
	</section>

	<section class="preview">
		{#if file}
			<FilePreview {id} path={file} entry={selected} />
		{:else}
			<p>{t('files.selectFile')}</p>
		{/if}
	</section>
</div>

<style>
	.browser { display: grid; gap: 1rem; }
	@media (min-width: 60rem) {
		.browser { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); }
	}
	.crumbs { display: flex; flex-wrap: wrap; gap: 0.25rem; word-break: break-all; }
	.tools { margin: 0.5rem 0; }
	.scroll { overflow-x: auto; }
	table { border-collapse: collapse; width: 100%; }
	th, td { text-align: left; padding: 0.3rem 0.5rem; border-bottom: 1px solid #eee; white-space: nowrap; }
	button.name { background: none; border: none; padding: 0; color: inherit; text-decoration: underline; cursor: pointer; font: inherit; }
	.target { opacity: 0.7; margin-left: 0.4rem; }
	tr.selected { background: #eef4ff; }
	.note { font-style: italic; }
	[role='alert'] { color: #b00020; }
</style>
```

- [ ] **Step 2: La page du conteneur et le lien depuis la liste**

`web/src/routes/c/[id]/+page.svelte` :

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ApiError, listContainers, type Container } from '#lib/api.ts';
	import { stateLabel } from '#lib/containers.ts';
	import { errorMessage, t } from '#lib/i18n/index.ts';
	import FileBrowser from '#lib/components/FileBrowser.svelte';

	let id = $derived(page.params.id ?? '');
	let container = $state<Container | null>(null);
	let error = $state('');

	// The list gives the name, image and state; GET /api/containers/{id}
	// arrives with the X-ray tab (phase Observation).
	$effect(() => {
		const wanted = id;
		let current = true;
		error = '';
		listContainers()
			.then((list) => {
				if (!current) return;
				container = list.find((c) => c.id === wanted || c.name === wanted) ?? null;
				if (!container) error = errorMessage('not_found');
			})
			.catch((e) => {
				if (!current) return;
				if (e instanceof ApiError && e.status === 401) {
					goto('/login').catch(() => {});
					return;
				}
				error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
			});
		return () => {
			current = false;
		};
	});
</script>

<main>
	<p><a href="/">← {t('nav.containers')}</a></p>
	{#if error}<p role="alert">{error}</p>{/if}
	{#if container}
		<h1>{container.name} <small class="state {container.state}">{stateLabel(container.state)}</small></h1>
		<p class="image">{container.image}</p>
		<nav class="tabs">
			<span class="tab active" aria-current="page">{t('container.tab.files')}</span>
			<span class="tab soon">{t('container.tab.changes')} ({t('container.soon')})</span>
			<span class="tab soon">{t('container.tab.logs')} ({t('container.soon')})</span>
			<span class="tab soon">{t('container.tab.xray')} ({t('container.soon')})</span>
		</nav>
		{#if container.state === 'running' || container.state === 'paused'}
			<FileBrowser id={container.id} />
		{:else}
			<p>{t('container.stoppedSoon')}</p>
		{/if}
	{/if}
</main>

<style>
	main { padding: 1rem; }
	h1 { margin-bottom: 0.25rem; word-break: break-all; }
	h1 small { font-size: 0.9rem; font-weight: normal; }
	.image { margin-top: 0; opacity: 0.8; }
	.tabs { display: flex; flex-wrap: wrap; gap: 0.5rem; margin: 1rem 0; border-bottom: 1px solid #ddd; }
	.tab { padding: 0.4rem 0.8rem; }
	.tab.active { border-bottom: 2px solid currentColor; font-weight: 600; }
	.tab.soon { opacity: 0.5; }
	.state.running { color: #1b7f3b; }
	.state.exited, .state.dead { color: #b00020; }
	[role='alert'] { color: #b00020; }
</style>
```

Dans `web/src/routes/+page.svelte`, remplacer `<td>{c.name}</td>` par :

```svelte
<td><a href={`/c/${c.id}`}>{c.name}</a></td>
```

- [ ] **Step 3: Vérifier**

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npm run check && npm run lint && npx vitest run && npm run build`
Expected : 0 erreur, 0 avertissement ; `Wrote site to "build"`.

- [ ] **Step 4: Commit**

```bash
git add web/src
git commit -F - <<'EOF'
explorateur: page du conteneur, onglet Fichiers (liste, aperçu, téléchargements)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 15: Agent de développement, essai manuel, README

**Où tester :** dans le navigateur sur le Mac, contre l'agent lancé en conteneur sur Docker Desktop.

**Files:**
- Create: `scripts/dev-agent.sh`
- Modify: `README.md` (section Développement), `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§8)

**Interfaces:**
- Consumes: le binaire (tâches 1 à 12) et l'interface (tâches 13 et 14)
- Produces: `scripts/dev-agent.sh [options de hublot]`

> **À comprendre :** avec Docker Desktop, `/proc/<pid>/root` n'existe que dans la VM Linux. L'agent tourne donc dans un conteneur alpine, avec le binaire Linux compilé sur le Mac et monté en lecture seule. Il reçoit les droits de la commande recommandée (§8). Ses données vivent dans le volume `hublot-dev-data`.

- [ ] **Step 1: Lancer l'agent en conteneur**

`scripts/dev-agent.sh` :

```bash
#!/usr/bin/env bash
# Lance l'agent dans un conteneur Linux pour le développement sur macOS
# (spec §8 : avec Docker Desktop, /proc/<pid>/root n'existe que dans la VM).
# Mêmes droits que la commande recommandée. L'interface embarquée est celle du
# dernier `make web` ; pour travailler l'interface, lancez plutôt `npm run dev`
# dans web/ (proxy /api vers 127.0.0.1:7878).
# Premier lancement : le jeton s'affiche. Ensuite :
#   docker run --rm -v hublot-dev-data:/data -v "$PWD/bin/hublot-linux:/hublot:ro" alpine:3 /hublot reset-token --data-dir /data
set -euo pipefail
cd "$(dirname "$0")/.."
arch=$(docker version -f '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o bin/hublot-linux ./cmd/hublot
docker rm -f hublot-dev >/dev/null 2>&1 || true
exec docker run --rm --name hublot-dev \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD/bin/hublot-linux:/hublot:ro" -v hublot-dev-data:/data \
  -p 127.0.0.1:7878:7878 \
  alpine:3 /hublot --listen 0.0.0.0:7878 --data-dir /data "$@"
```

Run: `chmod +x scripts/dev-agent.sh && make web && scripts/dev-agent.sh`
Expected : `Jeton d'accès (affiché une seule fois) : …` au premier lancement, puis `Hublot-Kit dev écoute sur [::]:7878`.

Dans un autre terminal : `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock docker run -d --name essai alpine:3 sleep 3600`.

- [ ] **Step 2: L'essai manuel**

Sur `http://127.0.0.1:7878` : se connecter, cliquer `essai`, puis vérifier, en notant le résultat dans le rapport de tâche :
1. `/` listé, `proc/` présent ; cliquer `proc/` → « Dossier inaccessible : pseudo-système… ».
2. `etc/` → `os-release → ../usr/lib/os-release` ; cliquer dessus → le dossier `/usr/lib` s'affiche avec l'aperçu d'`os-release` (texte « Alpine Linux »).
3. Droits affichés comme `ls` (`drwxr-xr-x`, `-rw-r--r--`), propriétaire `0:0`, tailles en o/Kio.
4. « Télécharger » sur `os-release` → fichier `os-release` enregistré. « Télécharger ce dossier (.tar) » sur `/etc` → `etc.tar` ; sur `/usr/lib`, atteint par le lien → marche (le chemin affiché est `/usr/lib`, sans lien). Sur `/` → message « Téléchargement refusé : … Téléchargez un fichier, ou un sous-dossier si le refus ne porte que sur ce dossier. (chemin interdit : proc monté sur /proc, …) » (Q8), aucun fichier.
5. `docker exec essai ln -s /etc /lien`, puis `/lien` dans la barre d'adresse (`?path=/lien`) → la liste s'affiche (lien suivi) ; « Télécharger ce dossier » → message « Ce chemin passe par un lien symbolique : ouvrez d'abord sa cible… ».
6. `/bin/busybox` → « Fichier binaire : pas d'aperçu. »
7. Bouton retour du navigateur → dossier précédent. Fenêtre étroite (≈ 375 px) → colonnes empilées, tableau défilant.
8. `docker restart essai` puis clic sur un dossier → la liste se recharge (nouveau PID) ; `docker stop essai` puis rechargement → message « Conteneur arrêté : … étape suivante ».

Arrêt : Ctrl-C, puis `docker rm -f essai`.

- [ ] **Step 3: README et spec §8 (revue M5)**

Dans `README.md`, section « Développement », remplacer le paragraphe « Lancer l'agent sur macOS (Docker Desktop) : » et sa commande par :

```markdown
Lancer l'agent sur macOS (Docker Desktop) : l'explorateur lit `/proc/<pid>/root`, qui n'existe que dans la VM de Docker Desktop ; l'agent tourne donc dans un conteneur, avec les droits recommandés :

    make web && scripts/dev-agent.sh

Tests Go sous Linux, en root (évasion, `procfs`, intégration avec Docker) :

    make test-linux
```

et la phrase sur le jeton par : « Le jeton d'accès s'affiche une seule fois au premier lancement ; la commande `reset-token` est rappelée en tête de `scripts/dev-agent.sh`. »

Dans la spec §8, paragraphe « **Développement sur macOS** », remplacer la phrase « L'agent se lance donc dans un conteneur pendant le développement (`docker compose` avec … `cgroup: host` est indispensable sur Docker Desktop, pilote cgroupfs), l'interface en `vite dev` sur le Mac avec un proxy vers l'agent. » par :

> L'agent se lance donc dans un conteneur pendant le développement. `scripts/dev-agent.sh` compile le binaire Linux sur le Mac et le lance avec les droits de la commande recommandée ; `--cgroupns=host` est indispensable sur Docker Desktop (pilote cgroupfs). Les tests Linux passent par `scripts/test-linux.sh` : conteneur `golang` en root, mêmes droits, socket Docker monté. L'interface tourne en `vite dev` sur le Mac, avec un proxy vers l'agent.

- [ ] **Step 4: Commit**

```bash
git add scripts/dev-agent.sh README.md docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: agent de développement en conteneur, essai manuel de l'onglet Fichiers, README et spec §8

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 16: De bout en bout, et la matrice Linux

**Où tester :** Docker Desktop (conteneur `golang`), VM `hk-ubuntu` (AppArmor), VM `hk-fedora` (SELinux), CI.

**Files:**
- Create: `internal/api/integration_test.go`

**Interfaces:**
- Consumes: `fixture.init`, `fixture.session`, `decodeListing`, `filename`, `errCode` (tâches 10 à 12, Socle) ; `dockerx.New`
- Produces: rien (tests)

> **À comprendre :** ce test fait passer de vraies requêtes HTTP (par `httptest`) à travers l'API complète, `fsview.For` réel, sur un vrai alpine : liste, aperçu d'un lien, contrôle `&check=1` puis tar de `/etc`, `forbidden_path` sur le tar de `/` et de `/sys`, directement et par `&check=1` (contrôle des montages), `link_in_path` sur un dossier atteint par un lien (créé par `docker exec`), `not_regular`, `forbidden_path`, `not_found`. Aucun bind de l'hôte n'est utilisé : le scénario C1 ne se teste qu'avec le faux client (tâches 7 et 12). Puis toute la suite tourne sur les trois Linux du projet : Docker Desktop (pilote cgroupfs, `/docker/<id>`), Ubuntu (systemd, `docker-<id>.scope`, AppArmor : la cible `--privileged` exige `apparmor=unconfined`), Fedora (SELinux `enforcing` : `label=disable`).

- [ ] **Step 1: Écrire le test**

`internal/api/integration_test.go` :

```go
//go:build integration && linux

package api

// File routes end to end (spec §10): real Docker, real procfs, real HTTP
// handler. Needs root and the host PID namespace: scripts/test-linux.sh or the
// CI integration job.

import (
	"archive/tar"
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func dockerCLI(t *testing.T, args ...string) string {
	t.Helper()
	var stderr bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker %s : %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return strings.TrimSpace(string(out))
}

func TestIntegrationFilesEndToEnd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
	}
	name := "hublot-it-api-fs"
	exec.Command("docker", "rm", "-f", name).Run()
	dockerCLI(t, "run", "-d", "--name", name, "alpine:3", "sleep", "300")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	dockerCLI(t, "exec", name, "ln", "-s", "/etc", "/linkdir")

	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // nil: the real fsview.For
	c := f.session(t)
	base := "/api/containers/" + name + "/fs"

	l := decodeListing(t, f.do("GET", base+"?path=/etc", "", c))
	found := false
	for _, e := range l.Entries {
		if e.Name == "os-release" && e.Type == "symlink" && e.LinkTarget == "../usr/lib/os-release" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/etc/os-release absent ou mal décrit : %+v", l.Entries)
	}

	rec := f.do("GET", base+"/file?path=/etc/os-release", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Alpine") || rec.Header().Get("ETag") == "" {
		t.Fatalf("aperçu : %d %s", rec.Code, rec.Body)
	}

	rec = f.do("GET", base+"/download?path=/etc", "", c)
	if rec.Code != http.StatusOK || filename(t, rec) != "etc.tar" {
		t.Fatalf("téléchargement de /etc : %d %s", rec.Code, rec.Body)
	}
	tr := tar.NewReader(rec.Body)
	hasHostname := false
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		hasHostname = hasHostname || h.Name == "etc/hostname"
	}
	if !hasHostname {
		t.Fatal("etc/hostname absent du tar")
	}

	if rec := f.do("GET", base+"/download?check=1&path=/etc", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("contrôle de /etc : %d %s", rec.Code, rec.Body)
	}
	for path, want := range map[string]string{
		"/download?path=/":            "forbidden_path", // its own /proc and /sys are under /
		"/download?path=/sys":         "forbidden_path",
		"/download?check=1&path=/":    "forbidden_path",
		"/download?check=1&path=/sys": "forbidden_path",
		"/download?path=/linkdir":     "link_in_path",
		"/download?path=/linkdir/apk": "link_in_path",
		"/file?path=/dev/null":        "not_regular",
		"?path=/proc":                 "forbidden_path",
		"/file?path=/proc/kallsyms":   "forbidden_path",
		"?path=/absent":               "not_found",
	} {
		if rec := f.do("GET", base+path, "", c); errCode(t, rec) != want {
			t.Errorf("%s : %d %s, attendu %s", path, rec.Code, rec.Body, want)
		}
	}
}
```

- [ ] **Step 2: Docker Desktop**

Run: `make test-linux 2>&1 | tail -9`
Expected : `ok` pour les 8 paquets (`cmd/hublot`, `api`, `audit`, `auth`, `config`, `dockerx`, `fsview`, `web`).

- [ ] **Step 3: VM Ubuntu (AppArmor)**

```bash
limactl start hk-ubuntu
limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 | tail -9
limactl shell hk-ubuntu -- sudo dmesg | grep -i 'apparmor="DENIED"' | tail -5
limactl stop hk-ubuntu
```

Expected : `ok` pour les 8 paquets ; aucune ligne `DENIED` datée de l'essai. Rejouer aussi, dans la VM, la mutation « `&& false` derrière le `pidfd_send_signal` de `CheckArchive` » de la tâche 7 (elle doit faire échouer `TestCheckArchiveDeadProcess`), et « confirmer R3 » : relever la ligne `MESURE :` ci-dessous ; R3 reste une limite assumée pour la v0.1. La ligne `MESURE :` (`limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -tags integration ./internal/fsview/ -run MeasureOwnProc -v`) : elle tranche Q8 sur Ubuntu. Si le dépôt n'est pas visible dans la VM (`limactl shell hk-ubuntu -- ls "$PWD"`), vérifier le montage `~` de `~/.lima/hk-ubuntu/lima.yaml`.

- [ ] **Step 4: VM Fedora (SELinux)**

```bash
limactl start hk-fedora
limactl shell --workdir "$PWD" hk-fedora -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 | tail -9
limactl shell hk-fedora -- sudo grep -c AVC /var/log/audit/audit.log
limactl stop hk-fedora
```

Expected : `ok` pour les 8 paquets ; le compte d'`AVC` n'augmente pas pendant l'essai (le noter avant et après ; `ausearch -m avc` ne trouve rien sur cette VM, voir les résultats du spike).

- [ ] **Step 5: Consigner et commiter**

Noter dans le rapport de tâche les trois sorties (8 × `ok`), le noyau de chaque VM (`uname -r`), le pilote cgroup (`docker info -f '{{.CgroupDriver}}'`) et les lignes `MESURE :`.

```bash
git add internal/api/integration_test.go
git commit -F - <<'EOF'
explorateur: test de bout en bout des routes de fichiers, passé sur Docker Desktop, Ubuntu et Fedora

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [ ] **Step 6: CI**

Rien à changer dans `.github/workflows/ci.yml` : le job `go` (non root) passe les tests d'évasion (le test du périphérique se saute avec son message), le job `integration` (`sudo go test -tags integration ./...`, root sur l'hôte) passe `procfs` et le bout en bout. Quand l'utilisateur pousse la branche et ouvre la PR, vérifier que les quatre jobs (`go`, `web`, `integration`, `binary`) sont verts.
