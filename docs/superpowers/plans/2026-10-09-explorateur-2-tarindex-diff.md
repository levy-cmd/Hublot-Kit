# Explorateur 2/3 — Conteneur arrêté (`tarindex`) et diff (F5) — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (règle du dépôt, `CLAUDE.md`) :** chaque tâche est relue par un sous-agent indépendant **et** par Codex avant la suivante. Les tâches 2 à 5, 7 et 8 touchent `internal/fsview` : relecture prioritaire sur l'évasion (spec §13).
>
> **Non vérifié avant rédaction :** contrairement au plan 1, le code de ce plan n'a pas été compilé sur une copie du dépôt. Deux faits ont été mesurés pendant la rédaction : sur Docker Desktop, `GET /archive` de `/` nomme ses entrées `/`, `/bin/`, `/bin/arch` (chemins absolus, barre finale pour un dossier), et l'archive d'un fichier nomme son entrée par le nom de base (`hostname`). Les signatures du SDK (`ContainerInspectOptions.Size`, `ContainerDiff`, `FilesystemChange`, `State.FinishedAt` en `string`, `SizeRootFs *int64`) ont été lues dans `github.com/moby/moby/client@v0.6.1` et `api@v1.56.1`.

**Goal:** L'utilisateur explore un conteneur arrêté comme un conteneur actif (liste, aperçu, téléchargement), avec une barre de progression pendant la première indexation. Il voit ce que le conteneur a changé par rapport à son image (onglet Changements, marques dans l'explorateur). Aucune lecture ne passe par un lien non résolu, et aucun tar n'est demandé pour un conteneur dont les montages exposent l'hôte.

**Architecture:** `fsview` gagne `tarindex` (§5.2) : un index en mémoire des en-têtes de `GET /archive` de `/`, résolu composant par composant. Le constructeur `For` devient `Views.For` : un `Views` par agent garde le cache des index (LRU 3, clé `ID + FinishedAt`, une seule construction par clé). Une route encore en construction répond `202 indexing` avec la progression. `dockerx` gagne `FinishedAt`, `SizeRootFs` et `Changes` ; `api` gagne `GET /changes`. L'interface gagne la barre de progression, l'onglet Changements et les marques du diff. Le plan solde aussi les trois écarts laissés par le plan 1 : (a) destination Docker sous un lien, (b) vivacité du process (`EPERM` de `pidfd_send_signal`), (c) `testing` lié au binaire.

**Tech Stack:** Go 1.27 (`archive/tar`, `net/http`), `github.com/moby/moby/client` v0.6.1, `github.com/cyphar/filepath-securejoin` v0.7.0 (`pathrs-lite`), `golang.org/x/sys` v0.48.0 ; SvelteKit 3 / Svelte 5, Vitest, Playwright (`@playwright/test`, nouvelle dépendance de développement, exigée par §10).

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§4, §5.1 étape 5, §5.2, §5.4, §6, §9, §10, §13). Plan précédent (format, décisions en vigueur, Q1 à Q9) : `docs/superpowers/plans/2026-10-08-explorateur-1-procfs.md`.

## Décisions de revue

Revue du plan, premier tour (relecteur Opus et Codex, « Prêt avec réserves »). Une ligne par correction, avec la tâche qui l'applique.

- **R1 (I1) :** une construction en cours n'est jamais évincée, ni un index qu'aucune requête n'a encore vu. Si les 3 places sont prises ainsi, `202` (progression 0) sans rien lancer. Sinon quatre conteneurs interrogés ensemble s'évinceraient sans fin. Test `TestViewsFourPolledAtOnce`. → Tâche 4.
- **R2 (I2) :** `&check=1` et `HEAD` sur un fichier d'un conteneur arrêté ne demandent plus rien à Docker. Interface facultative `fsview.OpenChecker` (`CheckOpen`), implémentée par `tarFS` ; `procfs` garde `Open` + `Close`. → Tâches 2, 3, 5.
- **R3 (I3) :** la première occurrence d'un lien physique doit se résoudre en elle-même et être un fichier régulier, sinon `not_regular`. Test rouge `TestTarHardLinkThroughSymlink` (`/a/` devenu lien vers `/etc`). → Tâches 2, 3.
- **R4 (I4) :** le montage retrouvé par identifiant doit être le montage Docker. Sa racine (`mountinfo`, champ 4) termine la `Source` du bind, ou finit par `/<nom>/_data` pour un volume. Le sens retenu est « la racine termine la `Source` », et non l'inverse : la racine est un chemin relatif au système de fichiers source. P ≠ `/`, un même P pour une seule destination. Piège « chroot vers / » testé. → Tâche 8.
- **R5 (I5) :** un index inutilisé 10 min est retiré (`IdleTTL`) ; `MaxHeaders` reste à 1 000 000. `TestMeasureIndexMemory` relève les octets par entrée ; la tâche 13 baisse `MaxHeaders` si 1 000 000 d'entrées dépassent 300 Mo. → Tâches 3, 4, 13.
- **R6 (Codex) :** `tarFS.List` et `tarFS.Stat` sont donnés en code. → Tâche 3.
- **R7 :** la mutation qui ne compilait pas est remplacée (contrôle « non-dossier au milieu » supprimé, `maxHops = 41`, `resolve(first)` retiré). → Tâche 3.
- **R8 :** `io/fs` importé dans `mounts_test.go` ; aucun test n'appelle `scanMountinfo` directement, la phrase est corrigée. → Tâche 8.
- **R9 :** cas d'une cible de lien qui contient `..` (`/e` → `d/../share/x`, piège `/share/x`). → Tâche 2.
- **R10 :** `TestTarTrapsAreArmed` lit chaque piège jusqu'à `io.EOF`. → Tâche 2.
- **R11 (écart b) :** la vivacité passe par `poll` sans attente sur le pidfd, plus par `pidfd_send_signal`, qu'un profil seccomp peut faire répondre `EPERM`. Un zombie compte pour terminé ; le test ne dépend plus de root. → Tâche 7.
- **R12 :** une requête qui attendait une construction retirée entre-temps (redémarrage, `Forget`) reçoit `202`, jamais `503`. → Tâche 4.
- **R13 :** dans `CheckArchive`, les contrôles qui ne lisent rien dans le conteneur passent avant `LocateMounts` (`pathrs` + `statx`, qui peuvent buter sur un FUSE). → Tâche 8.
- **R14 :** le `too_large` du plafond de durée expire après 5 min (`SlowRetry`) ; ceux des octets et des entrées restent. Le message dit que les volumes comptent dans les 4 Gio. → Tâches 4, 10 ; Q10, Q11.
- **R15 :** `readlink` du test d'intégration passe par `exec`, sans le helper `docker()` qui ferait `Fatalf` : le saut redevient atteignable. → Tâche 8.
- **R16 :** `scripts/e2e.sh` échoue, avec les logs de l'agent, si l'agent ne répond jamais. → Tâche 12.
- **R17 :** « décommenter » remplacé par « ajout » partout (tâche 9). **R18 :** essai manuel avec `/tmp/hk-bind` au lieu de `/srv`, que Docker Desktop ne partage pas. → Tâche 10.
- **R19 (sobriété) :** retirés le troisième test 202 (`fs_test.go`), `TestViewsFirstWait`, la goroutine de `TestFakeSizesDiffsCalls` et Playwright dans la VM (la CI le couvre). Le test d'intégration de la tâche 1 ne garde que la ligne `MESURE`. → Tâches 1, 4, 5, 13.

Second tour (re-relecture « Prêt » avec cinq points) :

- **R20 :** un index fini qu'aucune requête n'a vu (sa requête est partie) devient évinçable après `Grace` (30 s) sans visite ; une construction en cours ne l'est jamais. Test `TestViewsUnseenIndexesExpire`. → Tâche 4.
- **R21 :** pas de minuterie d'inactivité pour un `too_large` : il ne contient pas d'index et reste collant jusqu'au changement de `FinishedAt` (sauf plafond de durée, R14). → Tâche 4.
- **R22 :** cas « chroot vers un autre montage » (P ≠ `/`, racine sans rapport) ajouté ; la phrase de mutation de `sameMount` est corrigée. → Tâche 8.
- **R23 :** `Forget`, `drop`, `touchLocked` (minuterie réarmée) et `removeLocked` donnés en code. → Tâche 4.
- **R24 :** `TestAliveReal` note que le pidfd d'un zombie est lisible avant la récolte (Linux ≥ 5.3, déjà exigé par §5.1). → Tâche 7.
- Pas de changement pour `HasSuffix(Source, root)` : `root` commence par `/`, la frontière de composant est respectée (fausse alerte vérifiée).

## Décisions de ce plan (la spec ne tranche pas)

- **`Views` remplace `For`.** `fsview.NewViews(docker, opt) *Views`, `(*Views).For(ctx, id) (FS, error)`, `(*Views).Forget(id)` (pour le plan 3, après une écriture, §5.2 étape 1). `api.Deps` gagne `Views *fsview.Views` (nil : `NewViews(Docker, DefaultOptions())`) ; `OpenFS` nil vaut `Views.For`. → Tâche 4.
- **Cache (§5.2 étapes 1, 2, 4).**
  - Clé `ID + "\x00" + FinishedAt`. Une nouvelle clé pour un ID efface les anciennes (redémarré puis arrêté).
  - Au plus 3 entrées, **constructions en cours comprises**. Une construction en cours n'est jamais évincée ; un index qu'aucune requête n'a encore vu ne l'est qu'après 30 s sans visite (`Grace`, R20). Si les 3 places sont prises ainsi, la requête reçoit `202` (progression 0) sans rien lancer (R1).
  - Un index inutilisé pendant **10 min** est retiré (R5) : l'objectif de 50 Mo « au repos » de §5.2 étape 4 vaut donc dès 10 min sans visite. `MaxHeaders` reste à 1 000 000 ; la tâche 3 mesure les octets par entrée.
  - `too_large` des plafonds d'octets et d'entrées reste en cache jusqu'au prochain arrêt du conteneur (ou au redémarrage de l'agent). Celui du plafond de durée expire après 5 min (Q11, R14). Toute autre erreur (Docker indisponible, `container_changed`) n'est pas gardée : le prochain accès relance.
  - La requête **qui lance** la construction attend au plus 1 s (`FirstWait`) : un petit conteneur (alpine : 250 ms mesurées au spike) répond sans 202. Les requêtes suivantes ne bloquent jamais : elles ne tiennent pas un créneau de la garde.
- **Plafond d'entrées.** En plus de 4 Gio et 5 min, la construction s'arrête au-delà de **1 000 000 d'entrées** (`too_large`). Raison : un tar de 4 Gio d'en-têtes vides compte 8 millions d'entrées, soit plus d'un gigaoctet d'index. Ajouté à la spec §5.2 étape 3. → Tâches 3, 4.
- **Démarré pendant la construction.** Après la fin du tar, un second `Inspect` doit montrer le même `FinishedAt` et un conteneur arrêté ; sinon l'index est jeté, `container_changed`. → Tâche 4.
- **Lecture d'un fichier (`Open`).** Seul le chemin entièrement résolu part vers `GetArchive` ; pour un lien physique, sa première occurrence, qui doit se résoudre en elle-même (R3). `&check=1` et `HEAD` jugent sur l'index seul, par `CheckOpen` (R2). L'unique entrée reçue doit être un fichier régulier du même nom de base. Sinon (le fichier a changé par `docker cp` depuis l'indexation) → `container_changed`, et l'index est retiré du cache. La lecture va jusqu'à `io.EOF` du tar : une seconde entrée ou une archive tronquée font échouer la lecture. → Tâches 2, 3.
- **Noms du tar.** Chaque nom passe par `CleanPath("/" + nom)` (`/bin/`, `./bin`, `bin` donnent `/bin`). Parents absents : créés comme dossiers. Entrée sous un non-dossier : ignorée (inatteignable). Doublon : le dernier gagne, listé une fois. Lien physique vers une cible inconnue ou non régulière : `TypeOther` (`Open` → `not_regular`). → Tâche 3.
- **Troncature.** `archive/tar` ne distingue pas une archive coupée net sur une frontière de bloc d'une archive finie. Ce cas n'est pas détecté ; une coupure du corps HTTP, elle, remonte une erreur du client Docker. Limite notée ici, pas dans la spec.
- **`/proc` et `/sys` d'un conteneur arrêté.** Montrés comme des dossiers vides (spec §5.2 étape 7 : leurs entrées sont ignorées). Pas de `forbidden_path`.
- **`CheckArchive` de `tarindex`** rejoue `HostBind` sur les montages du conteneur (défense en profondeur ; `For` l'a déjà fait).
- **Réponse 202.** `{"error":"indexing","detail":"…","progress":0.42}`, sur toutes les routes de fichiers, `&check=1` et `HEAD` compris, toujours avant le premier octet. → Tâche 5.
- **`GET /changes`.** Pas de garde FUSE (Docker calcule le diff de la couche du conteneur, sans traverser ses montages), délai 30 s, tri par chemin, tableau complet (`[]` si vide). Un type de changement inconnu → `docker_unavailable`. → Tâche 9.
- **Interface.**
  - Pendant l'indexation, nouvelle requête toutes les secondes ; la minuterie est annulée au démontage.
  - Onglets par `?tab=changes` ; les liens de l'onglet Changements mènent à `?path=<chemin>`, et l'explorateur sait déjà ouvrir un fichier comme ça.
  - Marques `A`/`M` dans l'explorateur ; les supprimés du dossier courant tiennent sur une ligne qui renvoie à l'onglet Changements (Q13).
  - Si le diff échoue dans l'explorateur, aucune marque, sans message : le diff est secondaire là, l'onglet Changements a son propre état d'erreur.
- **Écarts du plan 1.** Les trois sont traités ici :
  - (a) destination Docker sous un lien : comparaison par identifiant de montage (tâche 8). Raison : tout conteneur qui monte `/var/run/docker.sock` (Portainer, Traefik, Watchtower) voit aujourd'hui refusé le tar de chacun de ses dossiers, et le plan 3 (écriture) ne repasse pas par ce code.
  - (b) vivacité : `poll` sans attente sur le pidfd (lisible = terminé, zombie compris) remplace `pidfd_send_signal(pidfd, 0)`, qui confondait `EPERM` (droits, seccomp) avec une mort ; échec de `poll` → `internal` (tâche 7, R11).
  - (c) `testing` dans le binaire : `SetCgroupReaderForTests` disparaît au profit de `Options.ReadCgroup` (tâche 4). La vérification est `go list -deps`.
- **`dockerx.Fake`** gagne un verrou sur `ArchiveCalls` et `Calls()` : la construction d'un index appelle `GetArchive` depuis une goroutine.
- **Playwright.** Fichiers `web/e2e/*.e2e.ts`, pour que Vitest (qui prend `*.spec.ts` et `*.test.ts`) ne les ramasse pas. Langue `fr-FR`. Agent et conteneur cible lancés par `scripts/e2e.sh`, en local comme en CI.

## Questions ouvertes

Le plan suit la recommandation de chaque question ; une réponse différente change la tâche indiquée.

- **Q10 — Indexation automatique ou à la demande ?** Ouvrir l'onglet Fichiers d'un conteneur arrêté lance la lecture de tout son tar (jusqu'à 4 Gio, volumes compris : ils sont dans l'archive).
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-09) :* automatique, comme le dit §5.2 étape 2. Les plafonds bornent le coût, le cache évite de recommencer, et un bouton « Indexer » ajouterait un clic au cas d'usage principal (« pourquoi mon app plante »). → Tâches 4, 10.
- **Q11 — `too_large` définitif ?** Une fois un plafond atteint, faut-il refuser le conteneur jusqu'à son prochain arrêt ?
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-09) :* oui pour les plafonds d'octets et d'entrées : le résultat serait le même. Le plafond de durée, lui, peut tenir à un moment de lenteur de Docker : il expire après 5 min. Pas de bouton « Réessayer ». Le message dit que les volumes comptent dans les 4 Gio et propose de démarrer le conteneur pour l'explorer (procfs, sans tar). → Tâches 4, 10.
- **Q12 — Très gros diff.** Un `npm install` dans un conteneur peut produire 100 000 changements.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-09) :* l'API renvoie tout (forme de la spec). L'onglet affiche les 1 000 premiers, triés, avec le décompte par type et « N changements de plus ». Pas de filtre en v0.1. → Tâches 9, 11.
- **Q13 — Fichiers supprimés dans l'explorateur.** Ils n'existent plus, donc le listing ne les montre pas.
  - *Recommandation (retenue, confirmée par l'utilisateur le 2026-10-09) :* une ligne « N supprimé(s) dans ce dossier » avec un lien vers l'onglet Changements, plutôt que des lignes fantômes dans le tableau (les lignes fantômes n'ont ni taille, ni droits, ni aperçu). → Tâche 11.

## Global Constraints

- **Module et dépendances Go** : module `github.com/levy-cmd/hublot-kit` ; dépendances autorisées (§3) : SDK Docker `github.com/moby/moby/client`, `github.com/cyphar/filepath-securejoin` **v0.7.0** (`pathrs-lite`), `golang.org/x/sys` **v0.48.0**. Aucune autre. Interface : une seule dépendance ajoutée, `@playwright/test`, en développement.
- **Accès aux fichiers d'un conteneur (CLAUDE.md, §5).** Conteneur arrêté : jamais de `linkname` envoyé tel quel à Docker, seulement le chemin entièrement résolu dans l'index (§5.2 étape 5). Conteneur actif : rien ne change par rapport au plan 1 (`pathrs`, `openat2`, `fstatfs`, fichiers réguliers ; **jamais** `filepath.Join`/`os.Open` sur un chemin du conteneur).
- **Tests d'évasion avant le code qu'ils protègent** (CLAUDE.md), voir « Ordre ». Chaque piège a son contrôle, qui prouve qu'une implémentation naïve s'échapperait. Les assertions vérifient l'absence de données fuitées **même quand une erreur est renvoyée**. Un test ne se saute que sur une condition exacte, nommée dans le message (jamais « sur toute erreur »). Un lecteur de tar boucle jusqu'à `io.EOF` et échoue sur toute autre erreur.
- **Code Linux** dans `_linux.go`, repli `_other.go` : `go test ./cmd/... ./internal/...` reste vert sur macOS. **Jamais** de test qui monte le `/proc`, le `/sys` ou le `/` de l'hôte puis demande un tar : dockerd se bloque. Les règles de type C1 se testent avec le faux client.
- **Téléchargement** : une copie interrompue fait `panic(http.ErrAbortHandler)`, libération et fermeture en `defer` ; `HEAD` = `&check=1` ; toute erreur en JSON avant le premier octet (déjà en place, à ne pas casser).
- **Erreurs API** : `{"error": "<code>", "detail": "<texte>"}`, codes de §6 uniquement ; `202` porte en plus `progress`.
- **Interface** : `{@html}` interdit ; bouton « Recharger » sur chaque état d'erreur ; garde contre les réponses périmées (`current`) ; grilles en `minmax(0,1fr)` ; textes dans `fr.ts` et `en.ts` (mêmes clés) ; imports `#lib/….ts` dans les `.svelte`. Dans `api.ts`, `api()` traite tout `res.ok` comme un succès : **un 202 doit y être reconnu explicitement**.
- **Node ≥ 22.17** : sur le Mac, `export PATH=/opt/homebrew/bin:$PATH` avant `npm`/`npx`. **Docker Desktop** : `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock`.
- **Commits** en français, terminés par une ligne vide puis `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Sorties filtrées** : chaque commande de vérification se termine par `| tail -n 20` (ou `-run` ciblé) ; pas de sortie brute de `go test -v` sur tout un paquet.

## Review Focus

1. **Conteneur démarré, redémarré ou supprimé pendant l'indexation ou entre deux clics** : jamais un index qui mélange deux états ; `container_changed` ou une nouvelle indexation, puis bouton « Recharger ». → Tâche 4 `TestViewsStartedDuringBuild`, `TestViewsCacheKey` ; tâche 10 (Recharger).
2. **Conteneur arrêté énorme ou lent** (image de plusieurs Gio, volume lent, millions de petits fichiers) : progression jamais au-delà de 99 %, `too_large` au plafond, mémoire de l'agent bornée. → Tâche 2 `TestBuildIndexLimits` ; tâche 4 `TestViewsProgressCapped`, `TestViewsDurationCap`, `TestViewsTooLargeSticky`.
3. **Plusieurs onglets ouverts sur le même conteneur arrêté pendant l'indexation** (requêtes toutes les secondes) : une seule construction, aucun `503` de la garde. → Tâche 4 `TestViewsIndexingThenReady` ; tâche 5 `TestStoppedPollsDoNotWait`.
4. **Noms inhabituels dans le tar** (`./x`, `x`, `/x/`, doublons, entrée sous un fichier, lien physique vers rien) : l'index reste cohérent, rien n'est atteint par un chemin fantôme. → Tâche 2 `TestBuildIndexNames`.
5. **Conteneur jamais démarré (`docker create`)** : `FinishedAt` nul, l'explorateur marche quand même. → Tâche 6 `TestIntegrationTarIndexCreated`.

## Ordre : chaque test d'évasion précède le code qu'il protège

| Protection (spec) | Tests écrits | Code protégé |
|---|---|---|
| Résolution dans l'index : liens relatifs, absolus, `..` bloqué, chaîne, dossier lié, `..` après un dossier lié, boucle, 40 sauts sur un seul compteur (§5.2 ét. 5) | Tâche 2 (commit des tests rouges) | Tâche 3 |
| Seul le chemin résolu part vers `GetArchive` ; non réguliers refusés sans `GetArchive` ; entrée reçue vérifiée, lue jusqu'à `io.EOF` (§5.2 ét. 6) | Tâche 2 | Tâche 3 |
| Liens physiques (§5.2 ét. 6), `/proc` et `/sys` ignorés (§5.2 ét. 7), plafonds (§5.2 ét. 3) | Tâche 2 | Tâche 3 |
| Binds de l'hôte et extensions Q9 : aucun index, aucun tar (§5.2 ét. 7) | Tâche 4, étape 1 | Tâche 4, étape 3 |
| Câblage par les routes : `403` sans `GetArchive`, `202` avant tout octet | Tâche 5, étape 1 | Tâche 5, étape 3 |
| Lien physique dont la première occurrence passe par un lien (revue I3) ; `&check=1` sans `GetArchive` (revue I2) | Tâche 2 | Tâches 3, 5 |
| Vivacité par `poll` du pidfd : zombie = terminé, pas de faux « mort » sur `EPERM` (§5.1 ét. 5) | Tâche 7, étape 1 | Tâche 7, étape 2 |
| Destination sous un lien : identifiant de montage lié au montage Docker, échec fermé, piège du chroot (§5.4 b, revue I4) | Tâche 8, étape 1 | Tâche 8, étape 2 |

**Ne pas pousser la branche entre les tâches 2 et 3** : les tests de `tarindex` y sont rouges, sur macOS comme sous Linux.

## Où tester

| Repère | Commande (racine du dépôt) |
|---|---|
| **macOS** | `go test ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; interface : `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 \| tail -n 15 && npm run check 2>&1 \| tail -n 5 && npm run lint 2>&1 \| tail -n 5` |
| **Docker Desktop (API Docker, conteneur arrêté)** | `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ ./internal/fsview/ ./internal/api/ -run 'Stopped\|TarIndex\|Changes' 2>&1 \| tail -n 20` (sur macOS, seuls les fichiers `integration` sans `linux` compilent) |
| **Linux root** | `scripts/test-linux.sh -race ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; avec Docker : ajouter `-tags integration` |
| **VM Ubuntu / Fedora** | `limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 \| tail -n 30` (idem `hk-fedora`) |
| **Parcours Playwright** | `export PATH=/opt/homebrew/bin:$PATH && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock scripts/e2e.sh 2>&1 \| tail -n 20` (créé tâche 12) |

## Structure des fichiers

```
internal/dockerx/dockerx.go          + FinishedAt, SizeRootFs, Changes, Change
internal/dockerx/fake.go             + Sizes, Diffs, verrou, Calls()
internal/fsview/fsview.go            + ErrIndexing, IndexingError ; − ErrStopped
internal/fsview/tarindex.go          index (construction, résolution) et tarFS (vue d'un conteneur arrêté)
internal/fsview/tarindex_test.go     tests d'évasion et de résolution, tar en mémoire (portables)
internal/fsview/views.go             Views (ex-for.go) : For, cache, construction, Forget
internal/fsview/views_test.go        (ex-for_test.go) cache, binds, progression, plafonds
internal/fsview/mounts.go            + MountIdent, LocateMounts ; scanMountinfo lit l'ID
internal/fsview/root_linux.go        alive() (poll du pidfd), lecteur de cgroup passé en paramètre ; − SetCgroupReaderForTests
internal/fsview/procfs_archive_linux.go  + mountIdent (statx), LocateMounts, alive()
internal/fsview/integration_tarindex_test.go  conteneur arrêté réel (tag integration, portable)
internal/api/server.go               + Deps.Views, route /changes
internal/api/fserr.go                + 202 indexing (avec progress) ; − 501
internal/api/changes.go              GET /changes
internal/api/fs_stopped_test.go      routes sur un conteneur arrêté (faux Docker, vraie Views)
internal/api/integration_stopped_test.go  bout en bout, conteneur arrêté et diff (tag integration, portable)
web/src/lib/api.ts, files.ts, changes.ts (+ tests)
web/src/lib/components/FileBrowser.svelte, FilePreview.svelte, ChangesList.svelte
web/src/routes/c/[id]/+page.svelte   onglets Fichiers / Changements, plus d'écran « arrêté »
web/playwright.config.ts, web/e2e/parcours.e2e.ts, scripts/e2e.sh
.github/workflows/ci.yml             matrice overlay2 / containerd, job e2e
```

---

### Task 1: `dockerx` : `FinishedAt`, `SizeRootFs`, `Changes` ; faux client sûr en concurrence

**Où tester :** macOS (unitaires) ; Docker Desktop (intégration).

**Files:**
- Modify: `internal/dockerx/dockerx.go`, `internal/dockerx/fake.go`, `internal/dockerx/dockerx_test.go`, `internal/dockerx/integration_test.go`

**Interfaces:**
- Consumes: rien de nouveau.
- Produces: `ContainerInfo.FinishedAt string` (`State.FinishedAt` brut, `"0001-01-01T00:00:00Z"` si jamais lancé) ; `type Change struct { Path string \`json:"path"\`; Kind string \`json:"kind"\` }` (`"added"|"modified"|"deleted"`) ; dans `Client` : `SizeRootFs(ctx, id string) (int64, error)` (`Inspect` avec `Size: true` ; 0 si Docker ne la donne pas), `Changes(ctx, id string) ([]Change, error)` (jamais nil) ; `Fake` gagne `Sizes map[string]int64` (par ID), `Diffs map[string][]Change` (par id tel que donné), `Calls() []string` (copie de `ArchiveCalls` sous verrou).

- [x] **Step 1: Tests unitaires qui échouent** — ajouter à `internal/dockerx/dockerx_test.go` (importer `slices` si absent) :

```go
func TestFromInspectFinishedAt(t *testing.T) {
	got := fromInspect(container.InspectResponse{ID: "x", State: &container.State{FinishedAt: "2026-10-09T10:00:00Z"}})
	if got.FinishedAt != "2026-10-09T10:00:00Z" {
		t.Fatalf("FinishedAt : %q", got.FinishedAt)
	}
}

func TestFromChanges(t *testing.T) {
	got, err := fromChanges([]container.FilesystemChange{
		{Kind: container.ChangeAdd, Path: "/a"}, {Kind: container.ChangeModify, Path: "/b"}, {Kind: container.ChangeDelete, Path: "/c"},
	})
	want := []Change{{"/a", "added"}, {"/b", "modified"}, {"/c", "deleted"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := fromChanges([]container.FilesystemChange{{Kind: 9, Path: "/x"}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("type inconnu : %v", err)
	}
	if got, err := fromChanges(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("aucun changement : %#v %v (attendu un tableau vide, pas nil)", got, err)
	}
}

func TestFakeSizesDiffsCalls(t *testing.T) {
	f := &Fake{Sizes: map[string]int64{"full": 42}, Diffs: map[string][]Change{"web": {{"/a", "added"}}},
		Archives: map[string][]byte{"full:/": []byte("T")}}
	if n, err := f.SizeRootFs(context.Background(), "full"); err != nil || n != 42 {
		t.Fatalf("taille : %d %v", n, err)
	}
	if cs, err := f.Changes(context.Background(), "web"); err != nil || len(cs) != 1 {
		t.Fatalf("diff : %v %v", cs, err)
	}
	if _, err := f.Changes(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("diff inconnu : %v", err)
	}
	f.GetArchive(context.Background(), "full", "/")
	if c := f.Calls(); !slices.Equal(c, []string{"full:/"}) {
		t.Fatalf("appels : %v", c)
	}
}
```

Run: `go test ./internal/dockerx/ 2>&1 | tail -n 5` — Expected: FAIL à la compilation (`fromChanges`, `Change`, `FinishedAt`, `Sizes`… indéfinis).

- [x] **Step 2: Implémenter**

`dockerx.go` :
- `ContainerInfo` : ajouter `FinishedAt string // State.FinishedAt as Docker gives it: part of the index cache key (spec §5.2 step 1)` ; dans `fromInspect`, sous `if c.State != nil`, `info.FinishedAt = c.State.FinishedAt`.
- Le type `Change` ci-dessus, et dans `Client` :

```go
	// SizeRootFs is Inspect with size=true: the size of the container's
	// root filesystem, for the progress of an index (spec §5.2 step 3). Costly
	// for Docker: called once per index build. 0 when Docker gives none.
	SizeRootFs(ctx context.Context, id string) (int64, error)
	// Changes is GET /containers/{id}/changes (F5): never nil.
	Changes(ctx context.Context, id string) ([]Change, error)
```

Implémentations :
- `engine.SizeRootFs` : `e.c.ContainerInspect(ctx, id, client.ContainerInspectOptions{Size: true})` ; erreur → `wrap(err)` ; `res.Container.SizeRootFs` nil → 0.
- `engine.Changes` : `e.c.ContainerDiff(ctx, id, client.ContainerDiffOptions{})` ; erreur → `wrap(err)` ; puis `fromChanges(res.Changes)`.
- `fromChanges([]container.FilesystemChange) ([]Change, error)` : `make([]Change, 0, len(cs))` (jamais nil) ; `container.ChangeAdd` → `"added"`, `ChangeModify` → `"modified"`, `ChangeDelete` → `"deleted"`, tout autre → `fmt.Errorf("%w : type de changement inconnu %d pour %s", ErrUnavailable, c.Kind, c.Path)`.

`fake.go` : champ `mu sync.Mutex // guards ArchiveCalls: an index is built in a goroutine` en tête de `Fake`, champs `Sizes` et `Diffs`. Dans `GetArchive`, l'`append` passe sous `f.mu`. `Calls()` renvoie `slices.Clone(f.ArchiveCalls)` sous `f.mu`. `SizeRootFs` : `f.Err`, sinon `f.Sizes[id]` (0 si absent). `Changes` : `f.Err`, sinon `f.Diffs[id]` cloné, absent → `fmt.Errorf("%w : %s", ErrNotFound, id)`.

Run: `go test -race ./internal/dockerx/ 2>&1 | tail -n 5 && go vet ./... 2>&1 | tail -n 5` — Expected: `ok`, aucun avertissement.

- [x] **Step 3: Mesure sur un vrai Docker** — ajouter à `internal/dockerx/integration_test.go` (même en-tête de build). `FinishedAt` et `Changes` sont couverts de bout en bout aux tâches 6 et 9 ; ce test ne fait que relever `SizeRootFs`.

```go
func TestMeasureSizeRootFs(t *testing.T) {
	name := "hublot-it-size"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "true").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	size, err := c.SizeRootFs(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MESURE : SizeRootFs = %d octets", size) // 0: the progress stays at 0 % (not an error)
}
```

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ -run MeasureSizeRootFs -v 2>&1 | grep -E 'MESURE|^(ok|FAIL|---)'` — Expected: `--- PASS`, une ligne `MESURE`.

- [x] **Step 4: Commit**

```bash
git add internal/dockerx
git commit -F - <<'EOF'
explorateur: dockerx FinishedAt, SizeRootFs et Changes ; faux client sûr en concurrence

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 2: Tests d'évasion et de résolution de `tarindex` (rouges)

**Où tester :** macOS et Linux (code portable, pas de Docker).

**Files:**
- Create: `internal/fsview/tarindex.go` (signatures seules, corps qui renvoient `errNotYet`)
- Create: `internal/fsview/tarindex_test.go`

**Interfaces:**
- Consumes: `dockerx.Fake` (`Archives`, `Calls()`, `ArchiveCloses`), `HostBind`, `CleanPath`, `ModeFromUnix` (plan 1).
- Produces (signatures que la tâche 3 remplit) :

```go
type IndexLimits struct {
	MaxBytes    int64         // bytes read from GET /archive of "/"
	MaxDuration time.Duration // the whole build, GetArchive included
	MaxHeaders  int           // tar entries: bounds the agent's memory
}
var DefaultIndexLimits = IndexLimits{MaxBytes: 4 << 30, MaxDuration: 5 * time.Minute, MaxHeaders: 1_000_000}
type index struct {
	entries   map[string]Entry    // clean container path → entry, "/" included
	children  map[string][]string // directory → names, in archive order
	hardlinks map[string]string   // hard link → its target, a regular file
}
func buildIndex(ctx context.Context, r io.Reader, lim IndexLimits, read *atomic.Int64) (*index, error)
func progress(read, total int64) float64 // read/total, at most 0.99; 0 when total <= 0
type tarFS struct { docker dockerx.Client; id string; mounts []dockerx.Mount; idx *index; stale func() }
func newTarFS(docker dockerx.Client, id string, mounts []dockerx.Mount, idx *index, stale func()) *tarFS
// tarFS implements FS: List, Stat, Open, StatNoLinks, CheckArchive, Close;
// plus CheckOpen(ctx, p) error: the checks of Open, from the index only (review I2)
```

> **À comprendre :** ici, « s'échapper », c'est demander à Docker autre chose que le chemin entièrement résolu : le lien lui-même, un `..` lexical, la cible brute. Docker résoudrait ce chemin à sa façon, et ce ne serait plus celui qu'on a vérifié. Le faux Docker sert donc `HOST-SECRET` à chacun de ces chemins piégés. `TestTarTrapsAreArmed` prouve qu'il les sert vraiment ; les autres tests prouvent qu'aucun n'est demandé, et qu'aucun octet `HOST-SECRET` n'est lu, même quand une erreur est renvoyée.

- [x] **Step 1: Stub compilable** — `internal/fsview/tarindex.go` : en-tête de paquet, les déclarations ci-dessus, `var errNotYet = errors.New("tarindex : pas encore implémenté (tâche 3)")`, `var _ FS = (*tarFS)(nil)`. `buildIndex` renvoie `nil, errNotYet` ; `progress` renvoie `-1` ; `newTarFS` remplit les champs ; chaque méthode de `tarFS`, `CheckOpen` compris, renvoie `errNotYet` (`Close` renvoie `nil`).

- [x] **Step 2: Écrire les tests** — `internal/fsview/tarindex_test.go` (sans étiquette de build ; noms de helpers choisis pour ne pas heurter ceux de `procfs_test.go`, compilé sous Linux) :

```go
package fsview

// Tests of tarindex (spec §5.2, §10), written before the code. The index is
// built from tars generated in memory; Open goes through a fake Docker whose
// archives are keyed by the exact path asked. At every path a naive
// implementation would ask for (the link itself, a lexical "..", a raw
// target), the fake holds an archive of HOST-SECRET: TestTarTrapsAreArmed
// proves the fake serves it, the other tests prove it is never asked for.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var tctx = context.Background()

var tarID = strings.Repeat("cd", 32)

type hdr struct {
	name, link, body string
	typ              byte
	mode             int64
}

func tdir(name string) hdr            { return hdr{name: name, typ: tar.TypeDir, mode: 0o755} }
func tfile(name, body string) hdr     { return hdr{name: name, typ: tar.TypeReg, mode: 0o644, body: body} }
func tlink(name, target string) hdr   { return hdr{name: name, typ: tar.TypeSymlink, mode: 0o777, link: target} }
func thard(name, target string) hdr   { return hdr{name: name, typ: tar.TypeLink, mode: 0o644, link: target} }
func tnode(name string, typ byte) hdr { return hdr{name: name, typ: typ, mode: 0o644} }

func mkTar(t *testing.T, hs ...hdr) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, h := range hs {
		th := &tar.Header{Name: h.name, Typeflag: h.typ, Linkname: h.link, Mode: h.mode,
			Size: int64(len(h.body)), ModTime: time.Unix(1_700_000_000, 0), Uid: 1000, Gid: 1000}
		if err := tw.WriteHeader(th); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, h.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// image is the fake container, named as GET /archive of "/" names it.
func image() []hdr {
	return []hdr{
		tdir("/"), tdir("/etc/"), tfile("/etc/passwd", "root:x:0:0"),
		tdir("/usr/"), tdir("/usr/lib/"), tfile("/usr/lib/os-release", "NAME=Alpine"),
		tdir("/usr/share/"), tfile("/usr/share/x", "SHARE-X"),
		tlink("/etc/os-release", "../usr/lib/os-release"), // relative: from the link's directory
		tlink("/abs", "/usr/lib/os-release"),              // absolute: from the container root
		tlink("/up", "../../../../usr/lib/os-release"),    // ".." blocked at the root
		tlink("/lib", "usr/lib"),                          // link on an intermediate directory
		tlink("/d", "/usr/lib"),                           // ".." after a linked directory
		tlink("/e", "d/../share/x"),                       // ".." inside a target, after a linked directory
		tlink("/c1", "c2"), tlink("/c2", "c3"), tlink("/c3", "/etc/passwd"),
		tlink("/loop1", "loop2"), tlink("/loop2", "loop1"),
		tlink("/dangling", "/nope"), tlink("/empty", ""),
		tdir("/bin/"), {name: "/bin/busybox", typ: tar.TypeReg, mode: 0o4755, body: "BUSYBOX"},
		thard("/bin/sh", "/bin/busybox"), // hard link: linkname = first occurrence
		tdir("/dev/"), tnode("/dev/null", tar.TypeChar), tnode("/dev/sda", tar.TypeBlock),
		tnode("/run.fifo", tar.TypeFifo),
		// Spec §5.2 step 7: nothing under /proc and /sys is indexed.
		tdir("/proc/"), tfile("/proc/kallsyms", "HOST-SECRET"), tdir("/sys/"), tfile("/sys/kernel/x", "HOST-SECRET"),
	}
}

// fileTar is what GET /archive of one file returns: one entry, its base name.
func fileTar(t *testing.T, p, body string) []byte { return mkTar(t, tfile(path.Base(p), body)) }

var realFiles = map[string]string{
	"/etc/passwd": "root:x:0:0", "/usr/lib/os-release": "NAME=Alpine",
	"/usr/share/x": "SHARE-X", "/bin/busybox": "BUSYBOX",
}

// traps: what a naive implementation would send to GetArchive.
var traps = []string{
	"/etc/os-release", "/abs", "/up", "/lib/os-release", "/lib", "/d/../share/x", "/share/x",
	"/c1", "/c2", "/c3", "/e", "/bin/sh", "/a/x", "/../../../../usr/lib/os-release",
	"/usr/lib/../../../../usr/lib/os-release", "../usr/lib/os-release", "usr/lib/os-release",
	"/proc/kallsyms", "/sys/kernel/x",
}

func trapDocker(t *testing.T) *dockerx.Fake {
	f := &dockerx.Fake{Archives: map[string][]byte{}}
	for p, body := range realFiles {
		f.Archives[tarID+":"+p] = fileTar(t, p, body)
	}
	for _, p := range traps {
		f.Archives[tarID+":"+p] = fileTar(t, p, "HOST-SECRET")
	}
	return f
}

type tarView struct {
	*tarFS
	docker *dockerx.Fake
	stale  *atomic.Int32
}

func newTarView(t *testing.T, mounts []dockerx.Mount, hs ...hdr) tarView {
	t.Helper()
	idx, err := buildIndex(tctx, bytes.NewReader(mkTar(t, hs...)), DefaultIndexLimits, new(atomic.Int64))
	if err != nil {
		t.Fatalf("buildIndex : %v", err)
	}
	f := trapDocker(t)
	stale := new(atomic.Int32)
	return tarView{newTarFS(f, tarID, mounts, idx, func() { stale.Add(1) }), f, stale}
}

func readTar(fsys FS, p string) ([]byte, error) {
	rc, err := fsys.Open(tctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// noLeak: no HOST-SECRET in what was read, error or not, and GetArchive was
// called exactly for want (none when want is empty).
func noLeak(t *testing.T, v tarView, b []byte, want ...string) {
	t.Helper()
	if bytes.Contains(b, []byte("HOST-SECRET")) {
		t.Errorf("ÉVASION : contenu piégé lu : %q", b)
	}
	exp := make([]string, len(want))
	for i, w := range want {
		exp[i] = tarID + ":" + w
	}
	if calls := v.docker.Calls(); !slices.Equal(calls, exp) {
		t.Errorf("ÉVASION : GetArchive %v, attendu %v", calls, exp)
	}
}

func TestTarTrapsAreArmed(t *testing.T) {
	f := trapDocker(t)
	for _, p := range traps {
		rc, err := f.GetArchive(tctx, tarID, p)
		if err != nil {
			t.Fatalf("piège %s : %v", p, err)
		}
		tr := tar.NewReader(rc)
		if _, err := tr.Next(); err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tr.Next(); err != io.EOF { // to the end of the tar
			t.Fatalf("piège %s : %v, attendu io.EOF", p, err)
		}
		rc.Close()
		if string(b) != "HOST-SECRET" {
			t.Fatalf("piège %s désarmé : %q", p, b)
		}
	}
}

func TestTarOpenResolves(t *testing.T) {
	cases := []struct{ path, resolved, body string }{
		{"/etc/os-release", "/usr/lib/os-release", "NAME=Alpine"},
		{"/abs", "/usr/lib/os-release", "NAME=Alpine"},
		{"/up", "/usr/lib/os-release", "NAME=Alpine"},
		{"/lib/os-release", "/usr/lib/os-release", "NAME=Alpine"},
		{"/d/../share/x", "/usr/share/x", "SHARE-X"}, // parent of the resolved target, never /share/x
		{"/e", "/usr/share/x", "SHARE-X"},            // the same "..", inside a link target
		{"/c1", "/etc/passwd", "root:x:0:0"},
		{"/bin/sh", "/bin/busybox", "BUSYBOX"},
		{"/../../etc/passwd", "/etc/passwd", "root:x:0:0"},
		{"/etc/./passwd", "/etc/passwd", "root:x:0:0"},
		{"//etc//passwd", "/etc/passwd", "root:x:0:0"},
		{"/usr/lib/../../../../usr/lib/os-release", "/usr/lib/os-release", "NAME=Alpine"},
	}
	for _, c := range cases {
		v := newTarView(t, nil, image()...)
		b, err := readTar(v, c.path)
		noLeak(t, v, b, c.resolved)
		if err != nil || string(b) != c.body {
			t.Errorf("mauvaise erreur : Open(%s) = %q, %v ; attendu %q", c.path, b, err, c.body)
		}
	}
}

func TestTarStat(t *testing.T) {
	v := newTarView(t, nil, image()...)
	if e, err := v.Stat(tctx, "/etc/os-release"); err != nil || e.Type != TypeFile || e.Size != int64(len("NAME=Alpine")) {
		t.Errorf("lien suivi : %+v %v", e, err)
	}
	// Hard link (spec §5.2 step 6): a file, with the size and mode of its target.
	if e, err := v.Stat(tctx, "/bin/sh"); err != nil || e.Type != TypeFile || e.Size != 7 || e.Mode != fs.ModeSetuid|0o755 {
		t.Errorf("lien physique : %+v %v", e, err)
	}
	for _, p := range []string{"/", "/d", "/etc/", "/lib/", "/d/"} {
		if e, err := v.Stat(tctx, p); err != nil || e.Type != TypeDir {
			t.Errorf("Stat(%s) : %+v %v, attendu un dossier", p, e, err)
		}
	}
	for _, p := range []string{"/proc/kallsyms", "/sys/kernel/x", "/nope", "/dangling", "/empty"} {
		if _, err := v.Stat(tctx, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat(%s) : %v, attendu fs.ErrNotExist", p, err)
		}
	}
	for _, p := range []string{"/etc/passwd/", "/c3/", "/etc/passwd/x", "/etc/passwd/.."} {
		if _, err := v.Stat(tctx, p); !errors.Is(err, ErrNotDir) {
			t.Errorf("Stat(%s) : %v, attendu ErrNotDir", p, err)
		}
	}
	noLeak(t, v, nil) // Stat never asks Docker
}

func TestTarList(t *testing.T) {
	v := newTarView(t, nil, image()...)
	root, truncated, err := v.List(tctx, "/")
	if err != nil || truncated {
		t.Fatalf("List(/) : %v %v", truncated, err)
	}
	byName := map[string]Entry{}
	for _, e := range root {
		byName[e.Name] = e
	}
	if e := byName["up"]; e.Type != TypeSymlink || e.LinkTarget != "../../../../usr/lib/os-release" {
		t.Errorf("cible brute attendue : %+v", e)
	}
	if e := byName["proc"]; e.Type != TypeDir {
		t.Errorf("/proc doit être montré comme un dossier : %+v", e)
	}
	if e := byName["run.fifo"]; e.Type != TypeOther {
		t.Errorf("FIFO : %+v", e)
	}
	for _, d := range []string{"/proc", "/sys"} {
		if entries, _, err := v.List(tctx, d); err != nil || len(entries) != 0 {
			t.Errorf("ÉVASION : List(%s) = %v, %v ; attendu vide", d, entries, err)
		}
	}
	if lib, _, err := v.List(tctx, "/lib"); err != nil || len(lib) != 1 || lib[0].Name != "os-release" {
		t.Errorf("List(/lib) : %+v %v", lib, err)
	}
	if _, _, err := v.List(tctx, "/etc/passwd"); !errors.Is(err, ErrNotDir) {
		t.Errorf("List d'un fichier : %v", err)
	}
	if _, _, err := v.List(tctx, "/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("List absent : %v", err)
	}
	noLeak(t, v, nil)
}

func TestTarListTruncates(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/big/")}
	for i := 0; i <= MaxEntries; i++ {
		hs = append(hs, tfile(fmt.Sprintf("/big/f%05d", i), ""))
	}
	v := newTarView(t, nil, hs...)
	if entries, truncated, err := v.List(tctx, "/big"); err != nil || !truncated || len(entries) != MaxEntries {
		t.Fatalf("%d entrées, tronqué %v, %v", len(entries), truncated, err)
	}
}

// One single counter of 40 hops for the whole resolution (spec §5.2 step 5).
func TestTarHopBudget(t *testing.T) {
	hs := []hdr{tdir("/"), tdir("/real/"), tfile("/real/file", "OK"), tlink("/real/g1", "file"), tlink("/s1", "/real")}
	for i := 2; i <= 21; i++ {
		hs = append(hs, tlink(fmt.Sprintf("/real/g%d", i), fmt.Sprintf("g%d", i-1)))
	}
	for i := 2; i <= 20; i++ {
		hs = append(hs, tlink(fmt.Sprintf("/s%d", i), fmt.Sprintf("/s%d", i-1)))
	}
	v := newTarView(t, nil, hs...)
	if e, err := v.Stat(tctx, "/s20/g20"); err != nil || e.Type != TypeFile { // 20 + 20 = 40
		t.Errorf("40 sauts : %+v %v", e, err)
	}
	if _, err := v.Stat(tctx, "/real/g21"); err != nil { // control: 21 hops alone resolve
		t.Errorf("21 sauts : %v", err)
	}
	if _, err := v.Stat(tctx, "/s20/g21"); !errors.Is(err, syscall.ELOOP) { // 20 + 21: one counter
		t.Errorf("41 sauts : %v, attendu ELOOP", err)
	}
	if _, err := v.Stat(tctx, "/loop1"); !errors.Is(err, syscall.ELOOP) {
		t.Errorf("boucle : %v", err)
	}
	b, err := readTar(v, "/loop1")
	noLeak(t, v, b)
	if !errors.Is(err, syscall.ELOOP) {
		t.Errorf("Open d'une boucle : %v", err)
	}
}

func TestTarOpenRefusesNonRegular(t *testing.T) {
	for _, p := range []string{"/dev/null", "/dev/sda", "/run.fifo", "/etc", "/lib", "/"} {
		v := newTarView(t, nil, image()...)
		b, err := readTar(v, p)
		noLeak(t, v, b) // refused from the index, before any GetArchive
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) : %v, attendu ErrNotRegular", p, err)
		}
	}
}

// The entry Docker sends must be the regular file the index announced; a
// docker cp since the build changes it.
func TestTarOpenChecksTheArchive(t *testing.T) {
	cases := map[string]func(t *testing.T) []byte{
		"lien à la place": func(t *testing.T) []byte { return mkTar(t, tlink("passwd", "/proc/self/root/etc/shadow")) },
		"autre nom":       func(t *testing.T) []byte { return mkTar(t, tfile("shadow", "HOST-SECRET")) },
		"dossier":         func(t *testing.T) []byte { return mkTar(t, tdir("passwd/"), tfile("passwd/x", "HOST-SECRET")) },
	}
	for name, archive := range cases {
		v := newTarView(t, nil, image()...)
		v.docker.Archives[tarID+":/etc/passwd"] = archive(t)
		b, err := readTar(v, "/etc/passwd")
		noLeak(t, v, b, "/etc/passwd")
		if !errors.Is(err, ErrContainerChanged) || v.stale.Load() != 1 {
			t.Errorf("%s : %v, index retiré %d fois", name, err, v.stale.Load())
		}
		if n := v.docker.ArchiveCloses.Load(); n != 1 {
			t.Errorf("%s : archive fermée %d fois", name, n)
		}
	}
}

// Read to io.EOF of the tar: a second entry or a cut archive is an error,
// never a silent success (a download would save a truncated file as whole).
func TestTarOpenReadsToEOF(t *testing.T) {
	full := mkTar(t, tfile("passwd", "root:x:0:0"))
	cases := map[string][]byte{
		"deux entrées": mkTar(t, tfile("passwd", "root:x:0:0"), tfile("extra", "HOST-SECRET")),
		"tronquée":     full[:512+5],
	}
	for name, archive := range cases {
		v := newTarView(t, nil, image()...)
		v.docker.Archives[tarID+":/etc/passwd"] = archive
		b, err := readTar(v, "/etc/passwd")
		noLeak(t, v, b, "/etc/passwd")
		if err == nil {
			t.Errorf("%s : lu sans erreur (%q)", name, b)
		}
	}
	v := newTarView(t, nil, image()...) // control: the intact archive
	if b, err := readTar(v, "/etc/passwd"); err != nil || string(b) != "root:x:0:0" || v.docker.ArchiveCloses.Load() != 1 {
		t.Errorf("archive intacte : %q %v", b, err)
	}
}

// Review I3: the first occurrence of a hard link is a path of the archive,
// but a later entry can turn one of its directories into a symbolic link.
// Docker would follow it: Open must refuse, from the index.
func TestTarHardLinkThroughSymlink(t *testing.T) {
	v := newTarView(t, nil,
		tdir("/"), tdir("/etc/"), tdir("/a/"), tfile("/a/x", "HOST-SECRET"),
		tlink("/a", "/etc"),     // the directory /a becomes a link
		thard("/h", "/a/x"),     // first occurrence: /a/x, now behind the link
	)
	b, err := readTar(v, "/h")
	noLeak(t, v, b)
	if !errors.Is(err, ErrNotRegular) {
		t.Errorf("Open(/h) : %v, attendu ErrNotRegular", err)
	}
	if err := v.CheckOpen(tctx, "/h"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("CheckOpen(/h) : %v", err)
	}
}

// Review I2: &check=1 and HEAD on a file judge from the index only (spec
// §5.4: no GetArchive).
func TestTarCheckOpen(t *testing.T) {
	v := newTarView(t, nil, image()...)
	for _, p := range []string{"/etc/os-release", "/bin/sh", "/e"} {
		if err := v.CheckOpen(tctx, p); err != nil {
			t.Errorf("CheckOpen(%s) : %v", p, err)
		}
	}
	for _, p := range []string{"/dev/null", "/run.fifo", "/etc"} {
		if err := v.CheckOpen(tctx, p); !errors.Is(err, ErrNotRegular) {
			t.Errorf("CheckOpen(%s) : %v, attendu ErrNotRegular", p, err)
		}
	}
	if err := v.CheckOpen(tctx, "/loop1"); !errors.Is(err, syscall.ELOOP) {
		t.Errorf("CheckOpen(/loop1) : %v", err)
	}
	noLeak(t, v, nil)
}

func TestTarStatNoLinks(t *testing.T) {
	v := newTarView(t, nil, image()...)
	for _, p := range []string{"/etc/passwd", "/bin/sh", "/"} { // a hard link is no symbolic link
		if _, err := v.StatNoLinks(tctx, p); err != nil {
			t.Errorf("StatNoLinks(%s) : %v", p, err)
		}
	}
	for _, p := range []string{"/etc/os-release", "/lib/os-release", "/lib", "/c1"} {
		if _, err := v.StatNoLinks(tctx, p); !errors.Is(err, ErrLinkInPath) {
			t.Errorf("StatNoLinks(%s) : %v, attendu ErrLinkInPath", p, err)
		}
	}
	for _, p := range []string{"/nope", "/d/../share"} { // cleaned lexically: /share
		if _, err := v.StatNoLinks(tctx, p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("StatNoLinks(%s) : %v", p, err)
		}
	}
	noLeak(t, v, nil)
}

func TestTarCheckArchive(t *testing.T) {
	bad := newTarView(t, []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}}, image()...)
	if err := bad.CheckArchive(tctx, "/etc"); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "bind /proc -> /host/proc") {
		t.Errorf("bind de l'hôte : %v", err)
	}
	ok := newTarView(t, []dockerx.Mount{{Type: "bind", Source: "/srv/data", Destination: "/data"}}, image()...)
	if err := ok.CheckArchive(tctx, "/etc"); err != nil {
		t.Errorf("bind ordinaire : %v", err)
	}
	noLeak(t, bad, nil)
	noLeak(t, ok, nil)
}

func TestBuildIndexNames(t *testing.T) {
	v := newTarView(t, nil,
		tdir("./"), tdir("./a/"), tfile("./a/x", "1"),
		tfile("b/y", "22"), // relative name, parent never declared
		tfile("/c", "333"),
		tfile("/a/x", "4444"), // again: the last one wins, listed once
		tfile("/f", ""), tfile("/f/z", "HOST-SECRET"), // under a file: unreachable
		thard("/h1", "/nope"), thard("/h2", "/a"), // hard link to nothing, to a directory
	)
	if e, err := v.Stat(tctx, "/a/x"); err != nil || e.Size != 4 {
		t.Errorf("/a/x : %+v %v", e, err)
	}
	if a, _, err := v.List(tctx, "/a"); err != nil || len(a) != 1 {
		t.Errorf("doublon listé : %+v %v", a, err)
	}
	if e, err := v.Stat(tctx, "/b"); err != nil || e.Type != TypeDir {
		t.Errorf("parent implicite : %+v %v", e, err)
	}
	if e, err := v.Stat(tctx, "/b/y"); err != nil || e.Size != 2 {
		t.Errorf("/b/y : %+v %v", e, err)
	}
	if e, err := v.Stat(tctx, "/c"); err != nil || e.Size != 3 {
		t.Errorf("/c : %+v %v", e, err)
	}
	if _, err := v.Stat(tctx, "/f/z"); !errors.Is(err, ErrNotDir) {
		t.Errorf("/f/z : %v", err)
	}
	for _, p := range []string{"/h1", "/h2"} {
		if e, err := v.Stat(tctx, p); err != nil || e.Type != TypeOther {
			t.Errorf("Stat(%s) : %+v %v", p, e, err)
		}
		b, err := readTar(v, p)
		noLeak(t, v, b)
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) : %v", p, err)
		}
	}
}

type blockingReader struct{ ctx context.Context }

func (r blockingReader) Read([]byte) (int, error) { <-r.ctx.Done(); return 0, r.ctx.Err() }

func TestBuildIndexLimits(t *testing.T) {
	var hs []hdr
	for i := 0; i < 40; i++ {
		hs = append(hs, tfile(fmt.Sprintf("/f%d", i), strings.Repeat("x", 600)))
	}
	b := mkTar(t, hs...) // 40 × 1 536 + 1 024 octets
	lim := DefaultIndexLimits
	lim.MaxBytes = 4096
	if _, err := buildIndex(tctx, bytes.NewReader(b), lim, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("plafond d'octets : %v", err)
	}
	lim = DefaultIndexLimits
	lim.MaxHeaders = 10
	if _, err := buildIndex(tctx, bytes.NewReader(b), lim, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("plafond d'entrées : %v", err)
	}
	var read atomic.Int64 // control: within the limits, everything is read and counted
	if _, err := buildIndex(tctx, bytes.NewReader(b), DefaultIndexLimits, &read); err != nil || read.Load() < int64(len(b))-1024 || read.Load() > int64(len(b)) {
		t.Errorf("lecture complète : %d octets sur %d, %v", read.Load(), len(b), err)
	}
	dctx, cancel := context.WithTimeout(tctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := buildIndex(dctx, blockingReader{dctx}, DefaultIndexLimits, new(atomic.Int64)); !errors.Is(err, ErrTooLarge) || time.Since(start) > 2*time.Second {
		t.Errorf("plafond de durée : %v après %s", err, time.Since(start))
	}
	cctx, ccancel := context.WithCancel(tctx)
	ccancel() // evicted from the cache: cancelled, not too large
	if _, err := buildIndex(cctx, bytes.NewReader(b), DefaultIndexLimits, new(atomic.Int64)); err == nil || errors.Is(err, ErrTooLarge) {
		t.Errorf("annulée : %v", err)
	}
	cut := len(b)/2 + 100 // mid-block
	if idx, err := buildIndex(tctx, bytes.NewReader(b[:cut]), DefaultIndexLimits, new(atomic.Int64)); err == nil || idx != nil || errors.Is(err, ErrTooLarge) {
		t.Errorf("archive tronquée : index %v, %v (attendu une erreur, jamais un index partiel)", idx != nil, err)
	}
}

func TestProgress(t *testing.T) {
	for _, c := range []struct {
		read, total int64
		want        float64
	}{{0, 0, 0}, {10, 0, 0}, {50, 100, 0.5}, {99, 100, 0.99}, {100, 100, 0.99}, {500, 100, 0.99}} {
		if got := progress(c.read, c.total); got != c.want {
			t.Errorf("progress(%d, %d) = %v, attendu %v", c.read, c.total, got, c.want)
		}
	}
}
```

- [x] **Step 3: Vérifier qu'ils échouent pour la bonne raison**

Run: `go vet ./internal/fsview/ 2>&1 | tail -n 3 && go test ./internal/fsview/ -run 'Tar|BuildIndex|Progress' 2>&1 | grep -cE '^\s+--- FAIL'; go test ./internal/fsview/ -run 'TarTrapsAreArmed' 2>&1 | tail -n 1`
Expected: `vet` muet ; une douzaine de `--- FAIL` (`buildIndex : tarindex : pas encore implémenté`, ou `progress = -1`) ; `TestTarTrapsAreArmed` **passe** (`ok`) : le piège est armé. Les autres tests du paquet restent verts : `go test ./internal/fsview/ -skip 'Tar|BuildIndex|Progress' 2>&1 | tail -n 1` → `ok`.

- [x] **Step 4: Commit (tests rouges, ne pas pousser)**

```bash
git add internal/fsview/tarindex.go internal/fsview/tarindex_test.go
git commit -F - <<'EOF'
explorateur: tests d'évasion et de résolution de tarindex, écrits avant le code (rouges)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 3: `tarindex` : construction de l'index, résolution, vue `tarFS`

**Où tester :** macOS et Linux.

**Files:**
- Modify: `internal/fsview/tarindex.go` (remplacer les stubs)

**Interfaces:**
- Consumes: les signatures de la tâche 2 ; `changed(format, …)` de `cgroup.go` (renvoie une erreur `ErrContainerChanged`) ; `HostBind`.
- Produces: celles de la tâche 2, implémentées ; `(*index).resolve(p string) (string, error)` ; `maxHops = 40`.

- [x] **Step 1: Construction** — dans `tarindex.go`, retirer `errNotYet` et écrire :

```go
// maxHops: one single counter for a whole resolution (spec §5.2 step 5).
const maxHops = 40

var errTooManyBytes = errors.New("plafond d'octets atteint")

// errTooSlow marks a too_large of the time cap: it expires (Q11), the
// bytes and entries caps stay.
var errTooSlow = errors.New("plafond de durée")

// countingReader counts the bytes of GET /archive (progress, MaxBytes) and
// stops at the end of ctx even if the reader below ignores it.
type countingReader struct {
	ctx context.Context
	r   io.Reader
	n   *atomic.Int64
	max int64
}

func (c *countingReader) Read(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(b)
	if c.n.Add(int64(n)) > c.max {
		return n, errTooManyBytes
	}
	return n, err
}

// buildIndex reads the tar of "/" to io.EOF and keeps the headers only
// (spec §5.2 step 3). Any other error fails the whole build: never a
// partial index.
func buildIndex(ctx context.Context, r io.Reader, lim IndexLimits, read *atomic.Int64) (*index, error) {
	x := &index{
		entries:   map[string]Entry{"/": {Name: "/", Type: TypeDir, Mode: 0o755}},
		children:  map[string][]string{},
		hardlinks: map[string]string{},
	}
	tr := tar.NewReader(&countingReader{ctx: ctx, r: r, n: read, max: lim.MaxBytes})
	for n := 0; ; n++ {
		h, err := tr.Next()
		if err == io.EOF {
			return x, nil
		}
		switch {
		case errors.Is(err, errTooManyBytes):
			return nil, fmt.Errorf("%w : index au-delà de %d octets lus", ErrTooLarge, lim.MaxBytes)
		case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)):
			return nil, fmt.Errorf("%w : index au-delà de %s (%w)", ErrTooLarge, lim.MaxDuration, errTooSlow)
		case err != nil:
			return nil, fmt.Errorf("lecture de l'archive de / : %w", err)
		case n >= lim.MaxHeaders:
			return nil, fmt.Errorf("%w : index au-delà de %d entrées", ErrTooLarge, lim.MaxHeaders)
		}
		x.add(h)
	}
}

// add records one header. GET /archive of "/" spells names "/bin/"; "./bin"
// and "bin" are accepted too.
func (x *index) add(h *tar.Header) {
	p := CleanPath(h.Name)
	if strings.HasPrefix(p, "/proc/") || strings.HasPrefix(p, "/sys/") {
		return // spec §5.2 step 7: never indexed, so never opened
	}
	e := Entry{Name: path.Base(p), Size: h.Size, Mode: ModeFromUnix(uint32(h.Mode)), UID: h.Uid, GID: h.Gid, ModTime: h.ModTime}
	hardTarget := ""
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeCont, tar.TypeGNUSparse:
		e.Type = TypeFile
	case tar.TypeDir:
		e.Type, e.Size = TypeDir, 0
	case tar.TypeSymlink:
		e.Type, e.Size, e.LinkTarget = TypeSymlink, 0, h.Linkname
	case tar.TypeLink: // spec §5.2 step 6: a file with the size and mode of its target
		target := CleanPath(h.Linkname)
		if t, ok := x.entries[target]; ok && t.Type == TypeFile && target != p {
			if first, ok := x.hardlinks[target]; ok {
				target = first
			}
			e.Type, e.Size, e.Mode = TypeFile, t.Size, t.Mode
			hardTarget = target
		} else {
			e.Type, e.Size = TypeOther, 0 // unknown target: Open refuses it
		}
	default: // device, FIFO…
		e.Type, e.Size = TypeOther, 0
	}
	if p == "/" {
		if e.Type == TypeDir {
			e.Name = "/"
			x.entries["/"] = e
		}
		return
	}
	parent := path.Dir(p)
	if !x.ensureDir(parent) {
		return // under a non-directory: no resolution reaches it
	}
	if _, seen := x.entries[p]; !seen {
		x.children[parent] = append(x.children[parent], e.Name)
	}
	delete(x.hardlinks, p)
	if hardTarget != "" {
		x.hardlinks[p] = hardTarget
	}
	x.entries[p] = e
}

// ensureDir makes d a directory of the index, creating the parents the
// archive did not declare. false: something on the way is not a directory.
func (x *index) ensureDir(d string) bool {
	if e, ok := x.entries[d]; ok {
		return e.Type == TypeDir
	}
	if !x.ensureDir(path.Dir(d)) {
		return false
	}
	x.entries[d] = Entry{Name: path.Base(d), Type: TypeDir, Mode: 0o755}
	x.children[path.Dir(d)] = append(x.children[path.Dir(d)], path.Base(d))
	return true
}

func progress(read, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return min(0.99, float64(read)/float64(total)) // volumes are read but not in SizeRootFs
}
```

- [x] **Step 2: Résolution (logique de sécurité, à recopier telle quelle)**

```go
// resolve walks p component by component in the index (spec §5.2 step 5)
// and returns the path it designates, every link resolved: the only kind of
// path that ever goes to GetArchive. cur never contains a link, so ".." is
// the parent of the resolved target, and stays at "/" on the root.
func (x *index) resolve(p string) (string, error) {
	todo := components(p)
	mustDir := strings.HasSuffix(p, "/")
	cur, hops := "/", 0
	for len(todo) > 0 {
		c := todo[0]
		todo = todo[1:]
		if c == ".." {
			cur = path.Dir(cur)
			continue
		}
		next := childPath(cur, c)
		e, ok := x.entries[next]
		if !ok {
			return "", notExist(p)
		}
		if e.Type == TypeSymlink {
			if hops++; hops > maxHops {
				return "", &fs.PathError{Op: "resolve", Path: p, Err: syscall.ELOOP}
			}
			if e.LinkTarget == "" {
				return "", notExist(p)
			}
			if strings.HasPrefix(e.LinkTarget, "/") {
				cur = "/" // absolute: from the container root, never the host's
			} // relative: from cur, the link's directory
			if strings.HasSuffix(e.LinkTarget, "/") && len(todo) == 0 {
				mustDir = true
			}
			todo = append(components(e.LinkTarget), todo...)
			continue
		}
		if len(todo) > 0 && e.Type != TypeDir {
			return "", fmt.Errorf("%w : %s", ErrNotDir, p)
		}
		cur = next
	}
	if mustDir && x.entries[cur].Type != TypeDir {
		return "", fmt.Errorf("%w : %s", ErrNotDir, p)
	}
	return cur, nil
}

func components(p string) []string {
	var out []string
	for _, c := range strings.Split(p, "/") {
		if c != "" && c != "." {
			out = append(out, c)
		}
	}
	return out
}

func childPath(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

func notExist(p string) error { return &fs.PathError{Op: "resolve", Path: p, Err: fs.ErrNotExist} }
```

- [x] **Step 3: La vue `tarFS`**

```go
// tarFS is the view of a stopped container (spec §5.2): every lookup in the
// index; only Open asks Docker, for the resolved path of a regular file.
func (t *tarFS) List(_ context.Context, dir string) ([]Entry, bool, error) {
	r, err := t.idx.resolve(dir)
	if err != nil {
		return nil, false, err
	}
	if t.idx.entries[r].Type != TypeDir {
		return nil, false, fmt.Errorf("%w : %s", ErrNotDir, dir)
	}
	names := t.idx.children[r]
	truncated := len(names) > MaxEntries
	if truncated {
		names = names[:MaxEntries]
	}
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		out = append(out, t.idx.entries[childPath(r, n)])
	}
	return out, truncated, nil
}

// Stat follows links, like any resolved path; List shows the links themselves.
func (t *tarFS) Stat(_ context.Context, p string) (Entry, error) {
	r, err := t.idx.resolve(p)
	if err != nil {
		return Entry{}, err
	}
	return t.idx.entries[r], nil
}

// regularPath is the path Open sends to GetArchive for p: resolved, a
// regular file. For a hard link, its first occurrence, which must itself
// resolve to itself (review I3: a later entry may have turned one of its
// directories into a link, which Docker would follow).
func (t *tarFS) regularPath(p string) (string, error) {
	r, err := t.idx.resolve(p)
	if err != nil {
		return "", err
	}
	if t.idx.entries[r].Type != TypeFile {
		return "", fmt.Errorf("%w : %s", ErrNotRegular, p)
	}
	if first, ok := t.idx.hardlinks[r]; ok {
		_, chained := t.idx.hardlinks[first]
		if got, err := t.idx.resolve(first); err != nil || got != first || chained || t.idx.entries[first].Type != TypeFile {
			return "", fmt.Errorf("%w : %s (lien physique vers %s)", ErrNotRegular, p, first)
		}
		r = first
	}
	return r, nil
}

// CheckOpen: every check of Open, without asking Docker (&check=1, HEAD).
func (t *tarFS) CheckOpen(_ context.Context, p string) error {
	_, err := t.regularPath(p)
	return err
}

func (t *tarFS) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	r, err := t.regularPath(p)
	if err != nil {
		return nil, err
	}
	rc, err := t.docker.GetArchive(ctx, t.id, r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(rc)
	h, err := tr.Next()
	if err != nil {
		rc.Close()
		return nil, fmt.Errorf("archive de %s : %w", r, err)
	}
	if h.Typeflag != tar.TypeReg || path.Base(CleanPath(h.Name)) != path.Base(r) {
		rc.Close()
		t.stale()
		return nil, changed("%s a changé depuis l'indexation", r)
	}
	return &entryReader{tr: tr, rc: rc}, nil
}

// entryReader reads the single entry of a file's archive, then requires the
// end of the tar.
type entryReader struct {
	tr *tar.Reader
	rc io.ReadCloser
}

func (e *entryReader) Read(b []byte) (int, error) {
	n, err := e.tr.Read(b)
	if err == io.EOF {
		if _, next := e.tr.Next(); next != io.EOF {
			if next == nil {
				next = errors.New("archive d'un fichier : plus d'une entrée")
			}
			return n, next
		}
	}
	return n, err
}

func (e *entryReader) Close() error { return e.rc.Close() }

// StatNoLinks: CleanPath(p), with no symbolic link anywhere on it (spec §5.4).
func (t *tarFS) StatNoLinks(_ context.Context, p string) (Entry, error) {
	cur := "/"
	for _, c := range components(CleanPath(p)) {
		if t.idx.entries[cur].Type != TypeDir {
			return Entry{}, fmt.Errorf("%w : %s", ErrNotDir, cur)
		}
		cur = childPath(cur, c)
		e, ok := t.idx.entries[cur]
		if !ok {
			return Entry{}, notExist(p)
		}
		if e.Type == TypeSymlink {
			return Entry{}, fmt.Errorf("%w : %s", ErrLinkInPath, cur)
		}
	}
	return t.idx.entries[cur], nil
}

// CheckArchive replays the host-bind rule (spec §5.2 step 7): Views.For
// already refused such a container, this guards the view on its own.
func (t *tarFS) CheckArchive(ctx context.Context, _ string) error {
	return HostBind(ctx, t.docker, t.mounts)
}

func (t *tarFS) Close() error { return nil }
```

- [x] **Step 4: Mesure de la mémoire (revue I5)** — ajouter à `tarindex_test.go` (importer `runtime`) :

```go
// Review I5: bytes per index entry, against the 50 MB goal of spec §5.2 step 4.
func TestMeasureIndexMemory(t *testing.T) {
	const n = 50_000
	hs := []hdr{tdir("/")}
	for i := range n {
		hs = append(hs, tfile(fmt.Sprintf("/usr/share/doc/pkg%03d/file-%06d.txt", i%1000, i), ""))
	}
	b := mkTar(t, hs...)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	idx, err := buildIndex(tctx, bytes.NewReader(b), DefaultIndexLimits, new(atomic.Int64))
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	per := float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / float64(len(idx.entries))
	t.Logf("MESURE : %.0f octets par entrée, soit %.0f Mo pour 1 000 000 d'entrées", per, per)
	runtime.KeepAlive(idx)
}
```

- [x] **Step 5: Vérifier**

Run: `go test ./internal/fsview/ -run MeasureIndexMemory -v 2>&1 | grep MESURE && go test -race ./internal/fsview/ 2>&1 | tail -n 3 && go vet ./internal/fsview/ && GOOS=linux go vet ./internal/fsview/ 2>&1 | tail -n 3`
Expected: `ok` ; aucun avertissement. Mutations à rejouer (le test doit rougir, puis revenir) : supprimer le contrôle `if len(todo) > 0 && e.Type != TypeDir` → `TestTarStat` échoue (`/etc/passwd/..` devient `/etc`) ; passer `maxHops` à 41 → `TestTarHopBudget` échoue ; retirer le `resolve(first)` de `regularPath` → `TestTarHardLinkThroughSymlink` échoue.

- [x] **Step 6: Commit**

```bash
git add internal/fsview/tarindex.go
git commit -F - <<'EOF'
explorateur: tarindex, index des en-têtes de GET /archive et résolution dans l'index

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 4: `Views` : cache des index, binds de l'hôte, fin de `ErrStopped` et de `SetCgroupReaderForTests`

**Où tester :** macOS (unitaires) ; Linux root pour les tests `procfs` modifiés.

**Files:**
- Rename: `internal/fsview/for.go` → `internal/fsview/views.go` ; `internal/fsview/for_test.go` → `internal/fsview/views_test.go`
- Modify: `internal/fsview/fsview.go`, `root_linux.go`, `procfs_other.go`, `procfs_archive_linux_test.go`, `integration_test.go`
- Modify: `internal/api/server.go`, `server_test.go`, `fserr.go`, `fserr_test.go`, `fs_test.go`, `fs_download_linux_test.go`
- Modify: spec §4, §5.2 (étapes 2, 3, 4, 7), §5.4 puce (a)

**Interfaces:**
- Consumes: `buildIndex`, `newTarFS`, `progress`, `IndexLimits` (tâches 2-3) ; `HostBind` ; `dockerx.Client.SizeRootFs`, `Inspect` (`FinishedAt`).
- Produces:

```go
var ErrIndexing = errors.New("indexation en cours")       // in fsview.go; ErrStopped removed
type IndexingError struct{ Progress float64 }               // Unwrap() == ErrIndexing; Progress in [0, 0.99]
type Options struct {
	Limits     IndexLimits   // zero fields: DefaultIndexLimits
	MaxCached  int           // 0: 3
	FirstWait  time.Duration // the request that starts a build waits that long; 0: no wait
	IdleTTL    time.Duration // 0: 10 min
	Grace      time.Duration // 0: 30 s
	SlowRetry  time.Duration // 0: 5 min
	ReadCgroup func(pid int) ([]byte, error) // nil: /proc/<pid>/cgroup
}
func DefaultOptions() Options // {DefaultIndexLimits, 3, time.Second, 10 min, 30 s, 5 min, nil}
func NewViews(docker dockerx.Client, opt Options) *Views
func (v *Views) For(ctx context.Context, id string) (FS, error)
func (v *Views) Forget(id string)
// root_linux.go: openRoot(pid int, containerID string, readCgroup func(int) ([]byte, error));
// openProcFS(docker dockerx.Client, info dockerx.ContainerInfo, readCgroup func(int) ([]byte, error)) (FS, error)
// api: Deps.Views *fsview.Views; fixture.views *fsview.Views (tests)
```

- [x] **Step 1: Tests qui échouent** — `git mv internal/fsview/for.go internal/fsview/views.go && git mv internal/fsview/for_test.go internal/fsview/views_test.go`. Remplacer **tout** le contenu de `views_test.go` par :

```go
package fsview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

var vID = strings.Repeat("ef", 32)

func stoppedInfo(id, name string, mounts ...dockerx.Mount) dockerx.ContainerInfo {
	return dockerx.ContainerInfo{ID: id, Name: name, FinishedAt: "2026-10-09T10:00:00Z", Mounts: mounts}
}

func stoppedDocker(t *testing.T, mounts ...dockerx.Mount) *dockerx.Fake {
	info := stoppedInfo(vID, "db", mounts...)
	return &dockerx.Fake{
		Infos: map[string]dockerx.ContainerInfo{"db": info, vID: info},
		Archives: map[string][]byte{
			vID + ":/":           mkTar(t, image()...),
			vID + ":/etc/passwd": fileTar(t, "/etc/passwd", "root:x:0:0"),
		},
		Sizes:   map[string]int64{vID: 1 << 20},
		Volumes: map[string]map[string]string{},
	}
}

func (v *Views) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.builds)
}

func waitCount(t *testing.T, v *Views, want int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); v.count() != want; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("index en cache : %d, attendu %d", v.count(), want)
		}
	}
}

// ready polls For until the index is built.
func ready(t *testing.T, v *Views, id string) FS {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		fsys, err := v.For(tctx, id)
		var ie *IndexingError
		if !errors.As(err, &ie) {
			if err != nil {
				t.Fatalf("For(%s) : %v", id, err)
			}
			return fsys
		}
		if time.Now().After(deadline) {
			t.Fatalf("index jamais prêt : %v", err)
		}
	}
}

func archiveCalls(f *dockerx.Fake, id, p string) int {
	n := 0
	for _, c := range f.Calls() {
		if c == id+":"+p {
			n++
		}
	}
	return n
}

// gated holds GET /archive of "/" after free bytes until open is closed,
// and honours ctx like the real client.
type gated struct {
	*dockerx.Fake
	open chan struct{}
	free int
}

func (g *gated) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	rc, err := g.Fake.GetArchive(ctx, id, p)
	if err != nil || p != "/" {
		return rc, err
	}
	return &gateReader{ctx: ctx, open: g.open, rc: rc, free: g.free}, nil
}

type gateReader struct {
	ctx  context.Context
	open chan struct{}
	rc   io.ReadCloser
	free int
}

func (r *gateReader) Read(b []byte) (int, error) {
	if r.free > 0 {
		b = b[:min(len(b), r.free)]
		n, err := r.rc.Read(b)
		r.free -= n
		return n, err
	}
	select {
	case <-r.open:
		return r.rc.Read(b)
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}

func (r *gateReader) Close() error { return r.rc.Close() }

// Spec §5.2 step 7 and Q9 (confirmed): no index, no tar of "/", for a
// container whose mounts may expose the host.
func TestViewsHostBindBuildsNothing(t *testing.T) {
	vol := func(driver string) dockerx.Mount {
		return dockerx.Mount{Type: "volume", Name: "v", Driver: driver, Destination: "/v"}
	}
	refused := []struct {
		m    dockerx.Mount
		opts map[string]string
		name string
	}{
		{dockerx.Mount{Type: "bind", Source: "/", Destination: "/host"}, nil, "bind / -> /host"},
		{dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"}, nil, "bind /proc -> /host/proc"},
		{dockerx.Mount{Type: "bind", Source: "/sys", Destination: "/host/sys"}, nil, "bind /sys -> /host/sys"},
		{dockerx.Mount{Type: "bind", Source: "/sys/fs/cgroup", Destination: "/cg"}, nil, "bind /sys/fs/cgroup -> /cg"},
		{dockerx.Mount{Type: "bind", Source: "/dev", Destination: "/dev"}, nil, "bind /dev -> /dev"},
		{vol("local"), map[string]string{"o": "bind", "device": "/proc"}, "bind /proc -> /v (volume v)"},
		{vol("local"), map[string]string{"type": "fuse.sshfs"}, "volume v (type=fuse.sshfs) -> /v"},
		{vol("rexray"), map[string]string{}, `pilote "rexray" inconnu`},
		{vol("local"), nil, "options illisibles"}, // no options for v: unreadable
	}
	for _, c := range refused {
		f := stoppedDocker(t, c.m)
		if c.opts != nil {
			f.Volumes["v"] = c.opts
		}
		v := NewViews(f, Options{FirstWait: time.Second})
		_, err := v.For(tctx, "db")
		if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s : %v", c.name, err)
		}
		if n := v.count(); n != 0 || len(f.Calls()) != 0 {
			t.Errorf("ÉVASION : %s : %d index lancé(s), GetArchive %v", c.name, n, f.Calls())
		}
	}
	for _, m := range []dockerx.Mount{ // controls: indexed
		{Type: "bind", Source: "/srv/data", Destination: "/data"},
		{Type: "bind", Source: "/dev/log", Destination: "/dev/log"},
	} {
		f := stoppedDocker(t, m)
		ready(t, NewViews(f, Options{}), "db")
		if n := archiveCalls(f, vID, "/"); n != 1 {
			t.Errorf("%+v : %d GET /archive de /", m, n)
		}
	}
}

func TestViewsIndexingThenReady(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})}
	v := NewViews(g, Options{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := v.For(tctx, "db")
			var ie *IndexingError
			if !errors.As(err, &ie) || !errors.Is(err, ErrIndexing) || ie.Progress < 0 || ie.Progress > 0.99 {
				t.Errorf("pendant l'indexation : %v", err)
			}
		}()
	}
	wg.Wait()
	close(g.open)
	fsys := ready(t, v, "db")
	if _, _, err := fsys.List(tctx, "/etc"); err != nil {
		t.Fatal(err)
	}
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Errorf("une seule construction par clé : %d GET /archive de /", n)
	}
}

func TestViewsProgressCapped(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{}), free: 4096}
	g.Sizes[vID] = 1024 // volumes are in the archive, not in SizeRootFs
	v := NewViews(g, Options{})
	defer close(g.open)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		_, err := v.For(tctx, "db")
		var ie *IndexingError
		if !errors.As(err, &ie) {
			t.Fatalf("attendu indexing : %v", err)
		}
		if ie.Progress > 0.99 {
			t.Fatalf("progression %v au-delà de 0,99", ie.Progress)
		}
		if ie.Progress == 0.99 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("progression bloquée à %v", ie.Progress)
		}
	}
}

func TestViewsCacheKey(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{})
	ready(t, v, "db")
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 1 {
		t.Fatalf("en cache : %d constructions", n)
	}
	info := f.Infos["db"] // restarted then stopped: new FinishedAt
	info.FinishedAt = "2026-10-09T11:00:00Z"
	f.Infos["db"], f.Infos[vID] = info, info
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 || v.count() != 1 {
		t.Errorf("nouvelle clé : %d constructions, %d index en cache (attendu 2 et 1)", n, v.count())
	}
}

// addStopped adds stopped containers named n, of ID strings.Repeat(n[1:], 64).
func addStopped(t *testing.T, f *dockerx.Fake, names ...string) {
	for _, n := range names {
		id := strings.Repeat(n[1:], 64)
		info := stoppedInfo(id, n)
		f.Infos[n], f.Infos[id] = info, info
		f.Archives[id+":/"] = mkTar(t, image()...)
	}
}

func TestViewsLRU(t *testing.T) {
	f := stoppedDocker(t)
	names := []string{"db", "c2", "c3", "c4"}
	addStopped(t, f, names[1:]...)
	v := NewViews(f, Options{MaxCached: 3})
	for _, n := range names {
		ready(t, v, n)
	}
	if v.count() != 3 {
		t.Fatalf("%d index en cache, attendu 3", v.count())
	}
	ready(t, v, "db") // the least recently used was evicted: built again
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("db : %d constructions, attendu 2", n)
	}
	ready(t, v, "c4")
	if n := archiveCalls(f, strings.Repeat("4", 64), "/"); n != 1 {
		t.Errorf("c4 : %d constructions, attendu 1", n)
	}
}

// Review I1: a build in progress is never evicted; four containers polled
// at once all end up ready.
func TestViewsFourPolledAtOnce(t *testing.T) {
	f := stoppedDocker(t)
	addStopped(t, f, "c2", "c3", "c4")
	g := &gated{Fake: f, open: make(chan struct{})}
	v := NewViews(g, Options{MaxCached: 3})
	names := []string{"db", "c2", "c3", "c4"}
	for _, n := range names {
		if _, err := v.For(tctx, n); !errors.Is(err, ErrIndexing) {
			t.Fatalf("%s : %v", n, err)
		}
	}
	if n := archiveCalls(f, strings.Repeat("4", 64), "/"); n != 0 || v.count() != 3 {
		t.Fatalf("c4 : %d construction(s), %d index ; attendu 0 et 3 (aucune construction évincée)", n, v.count())
	}
	close(g.open)
	seen := map[string]bool{}
	for deadline := time.Now().Add(5 * time.Second); len(seen) < len(names); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("jamais prêts : %v", seen)
		}
		for _, n := range names {
			_, err := v.For(tctx, n)
			if err == nil {
				seen[n] = true
			} else if !errors.Is(err, ErrIndexing) {
				t.Fatalf("%s : %v", n, err)
			}
		}
	}
}

// R20: three builds finish but their requests went away. The fourth
// container gets a slot once Grace has passed, not after IdleTTL.
func TestViewsUnseenIndexesExpire(t *testing.T) {
	f := stoppedDocker(t)
	addStopped(t, f, "c2", "c3", "c4")
	v := NewViews(f, Options{MaxCached: 3, Grace: 300 * time.Millisecond})
	for _, n := range []string{"db", "c2", "c3"} {
		v.For(tctx, n) // 202, then never asked again
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		v.mu.Lock()
		done := 0
		for _, b := range v.builds {
			if finished(b) {
				done++
			}
		}
		v.mu.Unlock()
		if done == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("constructions jamais finies")
		}
	}
	c4 := strings.Repeat("4", 64)
	if _, err := v.For(tctx, "c4"); !errors.Is(err, ErrIndexing) || archiveCalls(f, c4, "/") != 0 {
		t.Fatalf("pendant Grace : %v, %d construction(s) de c4 (attendu 202 et 0)", err, archiveCalls(f, c4, "/"))
	}
	time.Sleep(400 * time.Millisecond)
	ready(t, v, "c4")
}

// Review I5: an index unused for IdleTTL is dropped.
func TestViewsIdleTTL(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{IdleTTL: 50 * time.Millisecond})
	ready(t, v, "db")
	waitCount(t, v, 0)
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

func TestViewsTooLargeSticky(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{Limits: IndexLimits{MaxBytes: 1024}, FirstWait: 5 * time.Second})
	for range 2 {
		if _, err := v.For(tctx, "db"); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("attendu too_large : %v", err)
		}
	}
	if n := archiveCalls(f, vID, "/"); n != 1 {
		t.Errorf("too_large reste en cache : %d constructions", n)
	}
}

func TestViewsDurationCap(t *testing.T) {
	g := &gated{Fake: stoppedDocker(t), open: make(chan struct{})} // never opened
	v := NewViews(g, Options{Limits: IndexLimits{MaxDuration: 50 * time.Millisecond}, FirstWait: 5 * time.Second,
		SlowRetry: 200 * time.Millisecond})
	for range 2 {
		if _, err := v.For(tctx, "db"); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("attendu too_large : %v", err)
		}
	}
	if n := archiveCalls(g.Fake, vID, "/"); n != 1 {
		t.Fatalf("gardé pendant SlowRetry : %d constructions", n)
	}
	time.Sleep(250 * time.Millisecond)
	v.For(tctx, "db") // Q11: the time cap expires, the bytes cap does not (TestViewsTooLargeSticky)
	if n := archiveCalls(g.Fake, vID, "/"); n != 2 {
		t.Errorf("après SlowRetry : %d constructions, attendu 2", n)
	}
}

func TestViewsErrorsAreRetried(t *testing.T) {
	f := stoppedDocker(t)
	tarOfRoot := f.Archives[vID+":/"]
	delete(f.Archives, vID+":/")
	v := NewViews(f, Options{FirstWait: 5 * time.Second})
	if _, err := v.For(tctx, "db"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Fatalf("attendu not_found : %v", err)
	}
	waitCount(t, v, 0) // not kept
	f.Archives[vID+":/"] = tarOfRoot
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

// docker start while dockerd builds the tar: the index would mix two states.
type startsDuringBuild struct {
	*dockerx.Fake
	started atomic.Bool
}

func (s *startsDuringBuild) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	s.started.Store(true)
	return s.Fake.GetArchive(ctx, id, p)
}

func (s *startsDuringBuild) Inspect(ctx context.Context, id string) (dockerx.ContainerInfo, error) {
	info, err := s.Fake.Inspect(ctx, id)
	if s.started.Load() {
		info.Running, info.Pid = true, 4242
	}
	return info, err
}

func TestViewsStartedDuringBuild(t *testing.T) {
	d := &startsDuringBuild{Fake: stoppedDocker(t)}
	v := NewViews(d, Options{FirstWait: 5 * time.Second})
	if _, err := v.For(tctx, "db"); !errors.Is(err, ErrContainerChanged) {
		t.Fatalf("attendu container_changed : %v", err)
	}
	waitCount(t, v, 0)
}

func TestViewsForget(t *testing.T) {
	f := stoppedDocker(t)
	v := NewViews(f, Options{})
	ready(t, v, "db")
	v.Forget(vID)
	if v.count() != 0 {
		t.Fatal("Forget n'a rien retiré")
	}
	ready(t, v, "db")
	if n := archiveCalls(f, vID, "/"); n != 2 {
		t.Errorf("%d constructions, attendu 2", n)
	}
}

func TestViewsInspectErrors(t *testing.T) {
	v := NewViews(&dockerx.Fake{}, Options{})
	if _, err := v.For(tctx, "nope"); !errors.Is(err, dockerx.ErrNotFound) {
		t.Errorf("inconnu : %v", err)
	}
	down := NewViews(&dockerx.Fake{Err: fmt.Errorf("%w : dial", dockerx.ErrUnavailable)}, Options{})
	if _, err := down.For(tctx, "web"); !errors.Is(err, dockerx.ErrUnavailable) {
		t.Errorf("Docker arrêté : %v", err)
	}
	// Running without a pid: the stopped path, never procfs.
	f := stoppedDocker(t)
	info := f.Infos["db"]
	info.Running = true
	f.Infos["db"], f.Infos[vID] = info, info
	ready(t, NewViews(f, Options{}), "db")
}
```

Run: `go test ./internal/fsview/ 2>&1 | tail -n 3` — Expected: FAIL à la compilation (`NewViews`, `Options`, `IndexingError` indéfinis).

- [x] **Step 2: Retirer `SetCgroupReaderForTests` (écart c)**
  - `root_linux.go` : supprimer `SetCgroupReaderForTests` et l'import `testing`. `openRoot(pid int, containerID string, read func(int) ([]byte, error))` : `if read == nil { read = readCgroup }` en tête, puis `read(pid)` à la place de `readCgroup(pid)`. `OpenRoot` passe `nil`. `openProcFS(docker, info, read)` passe `read` à `openRoot`. La variable `readCgroup` reste : `root_linux_test.go` la remplace encore.
  - `procfs_other.go` : `openProcFS(dockerx.Client, dockerx.ContainerInfo, func(int) ([]byte, error)) (FS, error)`.
  - `procfs_archive_linux_test.go`, `TestForWiresMountsIntoCheckArchive` : supprimer `restore := SetCgroupReaderForTests(…)` et `t.Cleanup(restore)` ; remplacer `For(context.Background(), docker, "c")` par `NewViews(docker, Options{ReadCgroup: func(int) ([]byte, error) { return []byte("0::/docker/" + id + "\n"), nil }}).For(context.Background(), "c")`.
  - `integration_test.go` (fsview) : dans `openFor`, `For(ctx, dc, id)` devient `NewViews(dc, Options{}).For(ctx, id)`. Dans `TestIntegrationForStoppedContainer`, supprimer la partie `ErrStopped` (la tâche 6 la remplace) ; garder seulement le cas `hublot-it-absent` → `dockerx.ErrNotFound` ; renommer le test `TestIntegrationForAbsentContainer`.

- [x] **Step 3: Implémenter `Views`**

`fsview.go` : supprimer `ErrStopped` ; ajouter dans le bloc `var` `ErrIndexing = errors.New("indexation en cours")` et, après le bloc (importer `fmt`) :

```go
// IndexingError: the index of a stopped container is being built (spec §5.2
// step 2, §6 202 indexing). Progress goes from 0 to 0.99.
type IndexingError struct{ Progress float64 }

func (e *IndexingError) Error() string {
	return fmt.Sprintf("%v (%d %%)", ErrIndexing, int(e.Progress*100))
}

func (e *IndexingError) Unwrap() error { return ErrIndexing }
```

`views.go`, contenu complet (logique de concurrence, à recopier telle quelle) :

```go
package fsview

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

// Views opens the file view of a container (spec §4): procfs while it runs,
// tarindex otherwise. One Views per agent: it keeps the indexes of stopped
// containers (spec §5.2 steps 1, 2, 4).
type Views struct {
	docker dockerx.Client
	opt    Options
	mu     sync.Mutex
	builds map[string]*build // key: full ID + "\x00" + State.FinishedAt
	order  []string          // keys, least recently used first
}

// Options of NewViews; DefaultOptions are the agent's.
type Options struct {
	Limits    IndexLimits   // zero fields: DefaultIndexLimits
	MaxCached int           // indexes kept, builds in progress included; 0: 3
	FirstWait time.Duration // the request that starts a build waits that long before 202; 0: no wait
	IdleTTL   time.Duration // an index unused that long is dropped (review I5); 0: 10 min
	Grace     time.Duration // a finished index no request saw becomes evictable after that (R20); 0: 30 s
	SlowRetry time.Duration // a too_large of the time cap expires after that (Q11); 0: 5 min
	// ReadCgroup replaces the reading of /proc/<pid>/cgroup (spec §5.1 step 3).
	// nil in production; tests give one to a process of their own.
	ReadCgroup func(pid int) ([]byte, error)
}

// DefaultOptions and NewViews: see Interfaces. NewViews replaces every zero
// or negative field of opt.Limits by DefaultIndexLimits', MaxCached by 3,
// IdleTTL by 10 min, Grace by 30 s, SlowRetry by 5 min, then returns
// &Views{docker, opt, builds: map[string]*build{}}.

type build struct {
	id     string
	done   chan struct{} // closed once idx, err and doneAt are set
	cancel context.CancelFunc
	read   atomic.Int64
	total  atomic.Int64 // SizeRootFs; 0 while unknown
	idx    *index
	err    error
	doneAt time.Time
	served  bool        // under v.mu: its result reached a request at least once
	touched time.Time   // under v.mu: last request for it
	idle    *time.Timer // under v.mu: drops the finished index after IdleTTL unused
}

func finished(b *build) bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// For: id may be a name or a short ID; errors of Inspect pass through.
func (v *Views) For(ctx context.Context, id string) (FS, error) {
	info, err := v.docker.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if info.Running && info.Pid != 0 {
		return openProcFS(v.docker, info, v.opt.ReadCgroup)
	}
	// Spec §5.2 step 7: a container whose mounts may expose the host gets
	// no index at all, not even the tar of "/".
	if err := HostBind(ctx, v.docker, info.Mounts); err != nil {
		return nil, err
	}
	key := info.ID + "\x00" + info.FinishedAt
	idx, err := v.index(ctx, key, info)
	if err != nil {
		return nil, err
	}
	return newTarFS(v.docker, info.ID, info.Mounts, idx, func() { v.drop(key, nil) }), nil
}

// Forget drops every index of container id (full ID): plan Explorateur 3
// calls it after a write on a stopped container (spec §5.2 step 1).
func (v *Views) Forget(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, b := range v.builds {
		if b.id == id {
			v.removeLocked(k)
		}
	}
}

// drop removes key if it still holds b (any build when b is nil).
func (v *Views) drop(key string, b *build) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if cur, ok := v.builds[key]; ok && (b == nil || cur == b) {
		v.removeLocked(key)
	}
}

func (v *Views) index(ctx context.Context, key string, info dockerx.ContainerInfo) (*index, error) {
	v.mu.Lock()
	b, ok := v.builds[key]
	if ok && finished(b) && errors.Is(b.err, errTooSlow) && time.Since(b.doneAt) > v.opt.SlowRetry {
		v.removeLocked(key) // Q11: the time cap may have met a slow moment of Docker
		ok = false
	}
	if !ok {
		for k, old := range v.builds {
			if old.id == info.ID {
				v.removeLocked(k) // restarted since: stale
			}
		}
		if b = v.startLocked(key, info); b == nil {
			v.mu.Unlock()
			return nil, &IndexingError{} // every slot holds a build in progress (review I1)
		}
	}
	v.touchLocked(key)
	v.mu.Unlock()
	if !ok && v.opt.FirstWait > 0 { // only the request that started it: polls never hold a guard slot
		timer := time.NewTimer(v.opt.FirstWait)
		defer timer.Stop()
		select {
		case <-b.done:
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	if !finished(b) {
		return nil, &IndexingError{Progress: progress(b.read.Load(), b.total.Load())}
	}
	v.mu.Lock()
	b.served = true
	v.mu.Unlock()
	if b.err != nil {
		return nil, b.err
	}
	return b.idx, nil
}

// startLocked starts the build of key, or returns nil when no slot can be
// freed. Review I1: a build in progress is never evicted, nor, for Grace, an
// index no request has seen yet; otherwise four containers polled at once
// would evict each other forever. R20: past Grace, an unseen index (its
// request went away) is evictable too. The victim is the least recently used.
func (v *Views) startLocked(key string, info dockerx.ContainerInfo) *build {
	for len(v.builds) >= v.opt.MaxCached {
		i := slices.IndexFunc(v.order, func(k string) bool {
			b := v.builds[k]
			return finished(b) && (b.served || time.Since(b.touched) > v.opt.Grace)
		})
		if i < 0 {
			return nil
		}
		v.removeLocked(v.order[i])
	}
	ctx, cancel := context.WithTimeout(context.Background(), v.opt.Limits.MaxDuration)
	b := &build{id: info.ID, done: make(chan struct{}), cancel: cancel}
	v.builds[key] = b
	v.order = append(v.order, key)
	go v.run(ctx, key, b, info)
	return b
}

// touchLocked records a request for key: last use, idle timer re-armed,
// end of v.order (most recently used).
func (v *Views) touchLocked(key string) {
	b := v.builds[key]
	b.touched = time.Now()
	if b.idle != nil {
		b.idle.Reset(v.opt.IdleTTL)
	}
	if i := slices.Index(v.order, key); i >= 0 {
		v.order = append(slices.Delete(v.order, i, i+1), key)
	}
}

// removeLocked stops the idle timer and cancels the build of key, then
// forgets it (a missing key is a no-op).
func (v *Views) removeLocked(key string) {
	if b, ok := v.builds[key]; ok {
		if b.idle != nil {
			b.idle.Stop()
		}
		b.cancel()
		delete(v.builds, key)
	}
	if i := slices.Index(v.order, key); i >= 0 {
		v.order = slices.Delete(v.order, i, i+1)
	}
}

func (v *Views) run(ctx context.Context, key string, b *build, info dockerx.ContainerInfo) {
	defer b.cancel()
	go func() { // progress only: an unknown size shows 0 %
		if size, err := v.docker.SizeRootFs(ctx, info.ID); err == nil {
			b.total.Store(size)
		}
	}()
	idx, err := v.buildFor(ctx, b, info)
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		err = &IndexingError{} // removed meanwhile (restart, Forget): its waiters ask again, never a 503
	}
	b.idx, b.err, b.doneAt = idx, err, time.Now()
	close(b.done)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.builds[key] != b {
		return
	}
	if err != nil && !errors.Is(err, ErrTooLarge) {
		v.removeLocked(key) // not kept: the next access builds again
		return
	}
	if err != nil {
		return // a too_large holds no index: sticky until FinishedAt changes (Q11, R21)
	}
	// Review I5: spec §5.2 step 4 counts memory at rest; an index unused for
	// IdleTTL goes away.
	b.idle = time.AfterFunc(v.opt.IdleTTL, func() { v.drop(key, b) })
}

func (v *Views) buildFor(ctx context.Context, b *build, info dockerx.ContainerInfo) (*index, error) {
	rc, err := v.docker.GetArchive(ctx, info.ID, "/")
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w : index au-delà de %s (%w)", ErrTooLarge, v.opt.Limits.MaxDuration, errTooSlow)
		}
		return nil, err
	}
	defer rc.Close()
	idx, err := buildIndex(ctx, rc, v.opt.Limits, &b.read)
	if err != nil {
		return nil, err
	}
	// Started, or restarted and stopped, during the build: the archive may
	// mix two states of the container.
	now, err := v.docker.Inspect(ctx, info.ID)
	if err != nil {
		return nil, err
	}
	if (now.Running && now.Pid != 0) || now.FinishedAt != info.FinishedAt {
		return nil, changed("le conteneur %.12s a démarré pendant l'indexation", info.ID)
	}
	return idx, nil
}
```

(ajouter `"fmt"` aux imports.)

`api` (pour compiler) :
- `server.go` : dans `Deps`, `Views *fsview.Views // nil: fsview.NewViews(Docker, fsview.DefaultOptions()); also the index cache` ; le commentaire d'`OpenFS` dit « nil means Views.For ». Dans `New` : `if d.Views == nil { d.Views = fsview.NewViews(d.Docker, fsview.DefaultOptions()) }` puis `if d.OpenFS == nil { d.OpenFS = d.Views.For }`.
- `server_test.go` : champ `views *fsview.Views // nil: the default of New` dans `fixture` ; `init` passe `Views: f.views`.
- `fs_download_linux_test.go` : supprimer le `t.Cleanup(fsview.SetCgroupReaderForTests(…))` ; construire `f := &fixture{docker: fake, views: fsview.NewViews(fake, fsview.Options{ReadCgroup: func(int) ([]byte, error) { return []byte("0::/docker/" + webID + "\n"), nil }})}`. `OpenFS` reste nil : le test passe toujours par le câblage par défaut de `New`.
- `fserr.go` : supprimer le cas `ErrStopped` (501). `fserr_test.go` : supprimer sa ligne. `fs_test.go`, `TestFSListErrors` : supprimer le bloc `f.openErr = … ErrStopped` (5 lignes) ; le `f := newFixture(t, false)` qui le précède reste, pour le cas `nope`.

- [x] **Step 4: Spec** — dans `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` :
  - §4 : « Un constructeur `fsview.For(ctx, docker, containerID)` renvoie `procfs` si le conteneur tourne, `tarindex` sinon. » → « Un `fsview.Views` par agent, qui garde le cache des index (§5.2) : `Views.For(ctx, containerID)` renvoie `procfs` si le conteneur tourne, `tarindex` sinon. »
  - §5.2 étape 2 : ajouter « La requête qui lance la construction attend au plus 1 s : un petit conteneur répond sans `202`. Les requêtes suivantes répondent tout de suite. »
  - §5.2 étape 3 : après « (valeurs proposées : 4 Gio, 5 minutes) », insérer « , et en nombre d'entrées (1 000 000) » ; à la fin de l'étape : « Après la fin du tar, un second `Inspect` doit montrer le conteneur arrêté avec le même `FinishedAt`, sinon l'index est jeté (`container_changed`). »
  - §5.2 étape 4 : ajouter « Les constructions en cours comptent dans les 3, mais ne sont jamais évincées ; un index qu'aucune requête n'a encore vu ne l'est qu'après 30 s sans visite. Si aucune place ne se libère, la requête reçoit `indexing` sans rien lancer. Un index inutilisé pendant 10 min est retiré (mémoire au repos) ; un `too_large`, qui ne contient pas d'index, n'est pas concerné. Un `too_large` d'octets ou d'entrées reste en cache jusqu'au prochain arrêt ; celui du plafond de durée expire après 5 min ; toute autre erreur est retentée au prochain accès. »
  - §5.2 étape 7 : après la phrase sur le volume `local` en `o=bind`, insérer « Comme pour le tar d'un dossier d'un conteneur actif (§5.4 a), sont refusés aussi les types de montage autres que bind, volume, tmpfs et image, les volumes `local` dont `type=` est un pseudo-système ou FUSE, les volumes d'un pilote tiers et ceux dont les options ne se lisent pas : `GET /archive` de `/` les traverse tous, et un volume qui ne répond pas bloquerait dockerd. »
  - §5.4 puce (a) : « Ces extensions ne valent pas, pour l'instant, pour l'explorateur d'un conteneur arrêté (§5.2) ; » → « Ces extensions valent aussi pour l'explorateur d'un conteneur arrêté (§5.2 étape 7) ; ».

- [x] **Step 5: Vérifier**

Run:
```bash
go test -race ./cmd/... ./internal/... 2>&1 | tail -n 12
GOOS=linux go vet ./... && GOOS=linux go vet -tags integration ./... 2>&1 | tail -n 3
GOOS=linux go list -deps ./cmd/hublot | grep -cx testing
scripts/test-linux.sh -race ./internal/fsview/ ./internal/api/ 2>&1 | tail -n 5
```
Expected: tout `ok` ; `vet` muet ; `grep -c` affiche `0` (écart c soldé ; il affichait `1` avant cette tâche) ; sous Linux, `ok` pour les deux paquets.

- [x] **Step 6: Commit**

```bash
git add internal/fsview internal/api docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: Views, cache des index des conteneurs arrêtés, binds de l'hôte avant toute construction

ErrStopped et SetCgroupReaderForTests disparaissent : testing n'est plus lié au binaire.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 5: Routes de fichiers sur un conteneur arrêté : `202 indexing`, `&check=1` sans `GetArchive`

**Où tester :** macOS.

**Files:**
- Modify: `internal/api/fserr.go`, `internal/api/fserr_test.go`, `internal/api/fs_download.go`, `internal/fsview/fsview.go`
- Create: `internal/api/fs_stopped_test.go`
- Modify: spec §6

**Interfaces:**
- Consumes: `fsview.IndexingError`, `fsview.ErrIndexing`, `fsview.NewViews`, `fsview.Options`, `fixture.views` (tâche 4) ; `tarFS.CheckOpen` (tâche 3).
- Produces: réponse `202 {"error":"indexing","detail":"…","progress":<0..0.99>}` ; `type OpenChecker interface { CheckOpen(ctx context.Context, path string) error }` dans `fsview.go` (facultatif : `procfs` et `MemFS` ne l'implémentent pas et gardent `Open` + `Close`).

- [x] **Step 1: Tests qui échouent** — `internal/api/fs_stopped_test.go` :

```go
package api

// File routes on a stopped container through the real default wiring of New
// (Views.For, tarindex), with a fake Docker.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
	"github.com/levy-cmd/hublot-kit/internal/fsview"
)

var dbID = strings.Repeat("d0", 32)

type stEntry struct {
	name, body, link string
	typ              byte
}

func stTar(t *testing.T, es ...stEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range es {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.link, Mode: 0o644, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		io.WriteString(tw, e.body)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// stTarNames reads a tar to io.EOF; any other error fails.
func stTarNames(t *testing.T, b []byte) []string {
	t.Helper()
	var names []string
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("tar : %v", err)
		}
		names = append(names, h.Name)
	}
}

func stoppedFake(t *testing.T, mounts ...dockerx.Mount) *dockerx.Fake {
	info := dockerx.ContainerInfo{ID: dbID, Name: "db", FinishedAt: "2026-10-09T10:00:00Z", Mounts: mounts}
	return &dockerx.Fake{
		Infos: map[string]dockerx.ContainerInfo{"db": info, dbID: info},
		Archives: map[string][]byte{
			dbID + ":/": stTar(t,
				stEntry{name: "/", typ: tar.TypeDir}, stEntry{name: "/etc/", typ: tar.TypeDir},
				stEntry{name: "/etc/hostname", body: "db\n", typ: tar.TypeReg},
				stEntry{name: "/etc/alias", link: "hostname", typ: tar.TypeSymlink},
				stEntry{name: "/conf", link: "etc", typ: tar.TypeSymlink}),
			dbID + ":/etc/hostname": stTar(t, stEntry{name: "hostname", body: "db\n", typ: tar.TypeReg}),
			dbID + ":/etc": stTar(t, stEntry{name: "etc/", typ: tar.TypeDir}, stEntry{name: "etc/hostname", body: "db\n", typ: tar.TypeReg}),
		},
		Volumes: map[string]map[string]string{},
	}
}

// gatedAPI holds GET /archive of "/" until open is closed.
type gatedAPI struct {
	*dockerx.Fake
	open chan struct{}
}

func (g *gatedAPI) GetArchive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	if p == "/" {
		select {
		case <-g.open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.Fake.GetArchive(ctx, id, p)
}

func stoppedFixture(t *testing.T, docker dockerx.Client, fake *dockerx.Fake, opt fsview.Options) (*fixture, *http.Cookie) {
	t.Helper()
	f := &fixture{docker: fake, views: fsview.NewViews(docker, opt)}
	f.init(t, false, docker, nil) // nil: Views.For, the default wiring
	return f, f.session(t)
}

var stoppedRoutes = []string{
	"/fs?path=/etc", "/fs/file?path=/etc/hostname", "/fs/download?path=/etc",
	"/fs/download?path=/etc&check=1", "/fs/download?path=/etc/hostname",
}

func TestStoppedContainerRoutes(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	f, c := stoppedFixture(t, g, fake, fsview.Options{})
	for _, u := range stoppedRoutes {
		rec := f.do("GET", "/api/containers/db"+u, "", c)
		var body struct {
			Error    string   `json:"error"`
			Progress *float64 `json:"progress"`
		}
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusAccepted || body.Error != "indexing" || body.Progress == nil || *body.Progress > 0.99 ||
			rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s pendant l'indexation : %d %s", u, rec.Code, rec.Body)
		}
	}
	if rec := f.do("HEAD", "/api/containers/db/fs/download?path=/etc", "", c); rec.Code != http.StatusAccepted {
		t.Errorf("HEAD pendant l'indexation : %d", rec.Code)
	}
	close(g.open)
	var rec *httptest.ResponseRecorder
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		rec = f.do("GET", "/api/containers/db/fs?path=/etc", "", c)
		if rec.Code != http.StatusAccepted || time.Now().After(deadline) {
			break
		}
	}
	l := decodeListing(t, rec)
	if len(l.Entries) != 2 {
		t.Fatalf("listing : %+v", l)
	}
	// Review I2: check=1 and HEAD on a file judge from the index (spec §5.4: no GetArchive).
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/etc/alias&check=1", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("check=1 d'un fichier : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("HEAD", "/api/containers/db/fs/download?path=/etc/hostname", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("HEAD d'un fichier : %d", rec.Code)
	}
	if calls := fake.Calls(); slices.Contains(calls, dbID+":/etc/hostname") {
		t.Errorf("check=1 a lu le fichier : %v", calls)
	}
	rec = f.do("GET", "/api/containers/db/fs/file?path=/etc/alias", "", c)
	if rec.Code != http.StatusOK || rec.Body.String() != "db\n" {
		t.Errorf("aperçu par un lien : %d %q", rec.Code, rec.Body)
	}
	if calls := fake.Calls(); slices.Contains(calls, dbID+":/etc/alias") || !slices.Contains(calls, dbID+":/etc/hostname") {
		t.Errorf("ÉVASION : seul le chemin résolu part vers Docker : %v", calls)
	}
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/etc&check=1", "", c); rec.Code != http.StatusNoContent {
		t.Errorf("check=1 : %d %s", rec.Code, rec.Body)
	}
	rec = f.do("GET", "/api/containers/db/fs/download?path=/etc", "", c)
	if rec.Code != http.StatusOK || !slices.Contains(stTarNames(t, rec.Body.Bytes()), "etc/hostname") {
		t.Errorf("tar du dossier : %d", rec.Code)
	}
	if rec := f.do("GET", "/api/containers/db/fs/download?path=/conf&check=1", "", c); rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "link_in_path" {
		t.Errorf("dossier atteint par un lien : %d %s", rec.Code, rec.Body)
	}
}

func TestStoppedHostBindRefusesEveryRoute(t *testing.T) {
	fake := stoppedFake(t, dockerx.Mount{Type: "bind", Source: "/proc", Destination: "/host/proc"})
	f, c := stoppedFixture(t, fake, fake, fsview.Options{})
	for _, u := range stoppedRoutes {
		rec := f.do("GET", "/api/containers/db"+u, "", c)
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_path" || !strings.Contains(rec.Body.String(), "bind /proc -> /host/proc") {
			t.Errorf("%s : %d %s", u, rec.Code, rec.Body)
		}
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("ÉVASION : GetArchive %v", calls)
	}
}

func TestStoppedTooLarge(t *testing.T) {
	fake := stoppedFake(t)
	f, c := stoppedFixture(t, fake, fake, fsview.Options{Limits: fsview.IndexLimits{MaxBytes: 512}, FirstWait: 5 * time.Second})
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "too_large" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

// Only the request that starts a build waits (FirstWait): polls answer at
// once, so they never pile up in the guard (4 slots per container).
func TestStoppedPollsDoNotWait(t *testing.T) {
	fake := stoppedFake(t)
	g := &gatedAPI{Fake: fake, open: make(chan struct{})}
	defer close(g.open)
	f, c := stoppedFixture(t, g, fake, fsview.Options{FirstWait: 300 * time.Millisecond})
	start := time.Now()
	if rec := f.do("GET", "/api/containers/db/fs?path=/", "", c); rec.Code != http.StatusAccepted {
		t.Fatalf("première requête : %d", rec.Code)
	}
	if d := time.Since(start); d < 300*time.Millisecond { // control: the starter does wait
		t.Fatalf("la requête qui lance la construction doit attendre FirstWait : %s", d)
	}
	for range 3 {
		start = time.Now()
		rec := f.do("GET", "/api/containers/db/fs?path=/", "", c)
		if d := time.Since(start); rec.Code != http.StatusAccepted || d > 150*time.Millisecond {
			t.Errorf("relance : %d en %s, attendu 202 immédiat", rec.Code, d)
		}
	}
}

func TestIndexingErrorBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeFSError(rec, &fsview.IndexingError{Progress: 0.42})
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"progress":0.42`) || errCode(t, rec) != "indexing" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if !errors.Is(&fsview.IndexingError{}, fsview.ErrIndexing) {
		t.Error("IndexingError doit envelopper ErrIndexing")
	}
}
```

Dans `fserr_test.go`, ajouter la ligne `{&fsview.IndexingError{Progress: 0.5}, http.StatusAccepted, "indexing"},` avant le cas `errors.New("autre")`.

Run: `go test ./internal/api/ -run 'Stopped|Indexing|FSListErrors|FSErrorStatus' 2>&1 | grep -E '^(---|ok|FAIL)' | head -n 12`
Expected: FAIL — 500 `internal` au lieu de 202 (le code ne connaît pas encore `ErrIndexing`). `TestStoppedHostBindRefusesEveryRoute` passe déjà (tâche 4) : c'est le contrôle du câblage.

- [x] **Step 2: Implémenter** — `fserr.go` :

```go
// indexingJSON is the 202 of spec §6: the error body plus the progress.
type indexingJSON struct {
	Error    string  `json:"error"`
	Detail   string  `json:"detail,omitempty"`
	Progress float64 `json:"progress"`
}

func writeFSError(w http.ResponseWriter, err error) {
	var ie *fsview.IndexingError
	if errors.As(err, &ie) {
		writeJSON(w, http.StatusAccepted, indexingJSON{Error: "indexing", Detail: err.Error(), Progress: ie.Progress})
		return
	}
	status, code := fsErrorStatus(err)
	// … unchanged
}
```

et dans `fsErrorStatus`, après les deux cas de la garde : `case errors.Is(err, fsview.ErrIndexing): return http.StatusAccepted, "indexing"`.

`fsview.go` : `OpenChecker` (voir Interfaces), commenté « every check of Open without reading: a view that would have to ask Docker implements it (spec §5.4, &check=1) ». `fs_download.go`, branche fichier : en mode `check`, si `fsys` implémente `fsview.OpenChecker`, appeler `CheckOpen(ctx, p)` et renvoyer `opened{}` ; sinon le code actuel (`Open` puis `Close`) reste.

- [x] **Step 3: Spec §6** — supprimer « Conteneur arrêté, en attendant l'index (Explorateur 2) : 501 `internal`, temporaire (Q4). » ; après « Statuts HTTP : … `internal` 500. », ajouter « `indexing` 202, corps `{"error":"indexing","detail":"…","progress":0.42}` (progression plafonnée à 0,99), sur toutes les routes de fichiers, `&check=1` et `HEAD` compris. »

- [x] **Step 4: Vérifier**

Run: `go test -race ./internal/api/ 2>&1 | tail -n 3 && scripts/test-linux.sh -race ./internal/api/ 2>&1 | tail -n 3`
Expected: `ok` partout.

- [x] **Step 5: Commit**

```bash
git add internal/api docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: 202 indexing et sa progression sur les routes de fichiers d'un conteneur arrêté

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 6: Conteneur arrêté de bout en bout sur un vrai Docker

**Où tester :** Docker Desktop depuis le Mac (fichiers `integration` sans `linux`) ; puis `scripts/test-linux.sh`.

**Files:**
- Create: `internal/fsview/integration_tarindex_test.go`, `internal/api/integration_stopped_test.go`

**Interfaces:**
- Consumes: `fsview.NewViews`, `Options`, `IndexingError` ; `api` : `fixture`, `session`, `decodeListing`, `stTarNames` (tâche 5).
- Produces: rien.

> **À comprendre :** ces tests ne montent rien de l'hôte (règle C1 : les binds dangereux ne se testent qu'avec le faux client). Ils vérifient ce que le faux ne peut pas : la forme réelle des noms et des `linkname` de Docker (liens physiques compris), les volumes dans l'archive, le cas « jamais démarré ».

- [x] **Step 1: Tests `fsview`** — `internal/fsview/integration_tarindex_test.go` :

```go
//go:build integration

package fsview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func dockerCmd(t *testing.T, args ...string) string {
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

// indexed polls For until the index is built (60 s for a cold image pull).
func indexed(t *testing.T, v *Views, name string) FS {
	t.Helper()
	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		fsys, err := v.For(context.Background(), name)
		var ie *IndexingError
		if !errors.As(err, &ie) {
			if err != nil {
				t.Fatalf("For(%s) : %v", name, err)
			}
			return fsys
		}
		if time.Now().After(deadline) {
			t.Fatalf("index jamais prêt : %v", err)
		}
	}
}

func readFS(t *testing.T, fsys FS, p string) string {
	t.Helper()
	rc, err := fsys.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("Open(%s) : %v", p, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("lecture de %s : %v", p, err)
	}
	return string(b)
}

func TestIntegrationTarIndexStopped(t *testing.T) {
	name, vol := "hublot-it-tar", "hublot-it-tar-vol"
	exec.Command("docker", "rm", "-f", name).Run()
	exec.Command("docker", "volume", "rm", "-f", vol).Run()
	t.Cleanup(func() {
		exec.Command("docker", "rm", "-f", name).Run()
		exec.Command("docker", "volume", "rm", "-f", vol).Run()
	})
	dockerCmd(t, "run", "--name", name, "-v", vol+":/data", "alpine:3", "sh", "-c",
		"echo bonjour > /root/note.txt && chmod 4755 /root/note.txt && ln /root/note.txt /root/hard && "+
			"ln -s ../root/note.txt /etc/rel && ln -s /root/note.txt /abs && mkfifo /root/fifo && echo vol > /data/v.txt")
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fsys := indexed(t, NewViews(dc, Options{}), name)
	for _, p := range []string{"/root/note.txt", "/root/hard"} { // one of them is the tar's hard link
		if e, err := fsys.Stat(ctx, p); err != nil || e.Type != TypeFile || e.Size != 8 || e.Mode&0o7777 == 0 {
			t.Errorf("Stat(%s) : %+v %v", p, e, err)
		}
		if got := readFS(t, fsys, p); got != "bonjour\n" {
			t.Errorf("Open(%s) : %q", p, got)
		}
	}
	for _, p := range []string{"/etc/rel", "/abs"} {
		if got := readFS(t, fsys, p); got != "bonjour\n" {
			t.Errorf("Open(%s) : %q", p, got)
		}
	}
	if _, err := fsys.Open(ctx, "/root/fifo"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("FIFO : %v", err)
	}
	if got := readFS(t, fsys, "/data/v.txt"); got != "vol\n" { // the volume is in the archive (spike)
		t.Errorf("volume : %q", got)
	}
	if entries, _, err := fsys.List(ctx, "/proc"); err != nil || len(entries) != 0 {
		t.Errorf("/proc : %v %v", entries, err)
	}
	if _, err := fsys.StatNoLinks(ctx, "/etc/rel"); !errors.Is(err, ErrLinkInPath) {
		t.Errorf("StatNoLinks : %v", err)
	}
	if err := fsys.CheckArchive(ctx, "/root"); err != nil {
		t.Errorf("CheckArchive : %v", err)
	}
}

func TestIntegrationTarIndexCreated(t *testing.T) {
	name := "hublot-it-created"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	dockerCmd(t, "create", "--name", name, "alpine:3", "true") // never started: FinishedAt is zero
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	fsys := indexed(t, NewViews(dc, Options{}), name)
	if _, _, err := fsys.List(context.Background(), "/etc"); err != nil {
		t.Fatal(err)
	}
}
```

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/fsview/ -run TarIndex -v 2>&1 | grep -E '^(---|ok|FAIL)|_test.go'`
Expected: `--- PASS` ×2. Un échec sur le lien physique dit comment Docker écrit son `linkname` : corriger `add`, ne pas assouplir le test.

- [x] **Step 2: Test `api`** — `internal/api/integration_stopped_test.go` :

```go
//go:build integration

package api

import (
	"net/http"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestIntegrationStoppedAndChanges(t *testing.T) {
	name := "hublot-it-api-stopped"
	exec.Command("docker", "rm", "-f", name).Run()
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	if out, err := exec.Command("docker", "run", "--name", name, "alpine:3", "sh", "-c", "echo bonjour > /root/note.txt").CombinedOutput(); err != nil {
		t.Fatalf("docker run : %v %s", err, out)
	}
	dc, err := dockerx.New()
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{}
	f.init(t, false, dc, nil) // real Views, DefaultOptions
	c := f.session(t)
	base := "/api/containers/" + name
	rec := f.do("GET", base+"/fs?path=/root", "", c)
	for deadline := time.Now().Add(60 * time.Second); rec.Code == http.StatusAccepted && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		rec = f.do("GET", base+"/fs?path=/root", "", c)
	}
	decodeListing(t, rec)
	if rec := f.do("GET", base+"/fs/file?path=/root/note.txt", "", c); rec.Code != http.StatusOK || rec.Body.String() != "bonjour\n" {
		t.Errorf("aperçu : %d %q", rec.Code, rec.Body)
	}
	rec = f.do("GET", base+"/fs/download?path=/root", "", c)
	if rec.Code != http.StatusOK || !slices.Contains(stTarNames(t, rec.Body.Bytes()), "root/note.txt") {
		t.Errorf("tar : %d", rec.Code)
	}
}
```

La tâche 9 ajoute à la fin de ce test la vérification de `/changes`.

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/api/ -run IntegrationStopped -v 2>&1 | grep -E '^(---|ok|FAIL)'` puis `scripts/test-linux.sh -tags integration ./internal/fsview/ ./internal/api/ 2>&1 | tail -n 5`
Expected: `PASS` ; sous Linux, `ok` (les tests procfs du plan 1 tournent aussi).

- [x] **Step 3: Commit**

```bash
git add internal/fsview/integration_tarindex_test.go internal/api/integration_stopped_test.go
git commit -F - <<'EOF'
explorateur: conteneur arrêté de bout en bout sur un vrai Docker (liens physiques, volume, jamais démarré)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 7: Vivacité du process par `poll` du pidfd (écart b)

**Où tester :** Linux : `scripts/test-linux.sh`. Le test ne dépend plus des droits : root ou non, même résultat.

**Files:**
- Modify: `internal/fsview/root_linux.go`, `internal/fsview/procfs_archive_linux.go`, `internal/fsview/root_linux_test.go`
- Modify: spec §5.1 étape 5

**Interfaces:**
- Consumes: `changed()`.
- Produces: `alive(pidfd, pid int) error` ; seam `pidfdExited func(pidfd int) (bool, error)`.

> **À comprendre :** le plan 1 testait la vie par `pidfd_send_signal(pidfd, 0)`. Un `EPERM` (droits, ou profil seccomp) y passait pour une mort. Or un pidfd devient lisible quand son process se termine (noyau ≥ 5.3, déjà requis par §5.1). `poll` sans attente répond donc sans aucun contrôle de droit. Un zombie compte désormais pour terminé, ce qui est plus juste : sa racine disparaît.

- [x] **Step 1: Tests qui échouent** — ajouter à `root_linux_test.go` (importer `os/exec`, `time`, `golang.org/x/sys/unix` si absents) :

```go
func TestAliveSeam(t *testing.T) {
	old := pidfdExited
	t.Cleanup(func() { pidfdExited = old })
	for _, c := range []struct {
		exited      bool
		err         error
		ok, changed bool
	}{
		{false, nil, true, false},
		{true, nil, false, true},
		{false, unix.EBADF, false, false}, // poll failed: internal, never container_changed
	} {
		pidfdExited = func(int) (bool, error) { return c.exited, c.err }
		if err := alive(3, 42); (err == nil) != c.ok || errors.Is(err, ErrContainerChanged) != c.changed {
			t.Errorf("%+v : %v", c, err)
		}
	}
}

// Real processes. PID 1 cannot be signalled without root (the old EPERM):
// alive. A zombie still received signal 0: it must now count as exited.
func TestAliveReal(t *testing.T) {
	pidfd, err := unix.PidfdOpen(1, 0)
	if err != nil {
		t.Fatalf("pidfd_open(1) : %v", err)
	}
	defer unix.Close(pidfd)
	if err := alive(pidfd, 1); err != nil {
		t.Errorf("PID 1 : %v", err)
	}
	cmd := exec.Command("sleep", "30")
	must(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	cfd, err := unix.PidfdOpen(cmd.Process.Pid, 0)
	must(t, err)
	defer unix.Close(cfd)
	if err := alive(cfd, cmd.Process.Pid); err != nil {
		t.Fatalf("vivant : %v", err)
	}
	// Not reaped: a zombie. A pidfd polls readable once its process exits,
	// before the reap (Linux >= 5.3, already required by spec §5.1).
	must(t, cmd.Process.Kill())
	for deadline := time.Now().Add(5 * time.Second); !errors.Is(alive(cfd, cmd.Process.Pid), ErrContainerChanged); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("un zombie compte encore comme vivant")
		}
	}
}
```

Run: `GOOS=linux go vet ./internal/fsview/ 2>&1 | tail -n 3` — Expected: `undefined: pidfdExited`, `undefined: alive`.

- [x] **Step 2: Implémenter** — dans `root_linux.go` :

```go
// pidfdExited polls pidfd without waiting: readable once the process has
// exited, zombie included. A seam for the tests of alive.
var pidfdExited = func(pidfd int) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, err
		}
		if fds[0].Revents&unix.POLLNVAL != 0 {
			return false, unix.EBADF
		}
		return n > 0 && fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0, nil
	}
}

// alive is spec §5.1 step 5 on the process pidfd pins: exited →
// container_changed; poll failed → an error of its own (500 internal).
func alive(pidfd, pid int) error {
	exited, err := pidfdExited(pidfd)
	if err != nil {
		return fmt.Errorf("poll du pidfd du process %d : %w", pid, err)
	}
	if exited {
		return changed("le process %d s'est terminé", pid)
	}
	return nil
}
```

Dans `openRoot`, le bloc `if err := unix.PidfdSendSignal(…)` devient `if err := alive(pidfd, pid); err != nil { unix.Close(fd); return fail(err) }`. Dans `CheckArchive`, le bloc équivalent devient `if err := alive(p.pidfd, p.pid); err != nil { return err }`. Plus aucun `PidfdSendSignal` : `grep -rn PidfdSendSignal internal | grep -v _test` est vide.

- [x] **Step 3: Spec §5.1 étape 5** — « `pidfd_send_signal(pidfd, 0)` doit réussir : le process est toujours vivant, donc le PID n'a pas été réattribué entre les étapes 2 et 4, et `rootfd` est bien sa racine. Sinon → fermer, `container_changed`. » → « Le `pidfd` ne doit pas être lisible (`poll` sans attente ; il le devient quand le process se termine, zombie compris) : le process est toujours vivant, donc le PID n'a pas été réattribué entre les étapes 2 et 4, et `rootfd` est bien sa racine. Lisible → fermer, `container_changed` ; échec de `poll` → fermer, `internal`. `pidfd_send_signal(pidfd, 0)` n'est pas utilisé : il répond `EPERM` sans droit de signaler (ou sous un profil seccomp). » Adapter de même la mention de §5.4 (b) « qui doit être vivant après la lecture » si elle cite le signal.

- [x] **Step 4: Vérifier**

Run: `scripts/test-linux.sh ./internal/fsview/ -run 'Alive|OpenRoot|CheckArchive' -v 2>&1 | grep -E '^(---|ok|FAIL)'` puis, en non-root, `docker run --rm --pid=host -u 1000 -v "$PWD":/src -w /src -e GOCACHE=/tmp/gc -e GOPATH=/tmp/gp hublot-test go test ./internal/fsview/ -run Alive 2>&1 | tail -n 3`
Expected: tout PASS, aucun SKIP, dans les deux cas. Contrôle de l'ancienne implémentation (à rejouer une fois) : avec le signal 0, `TestAliveReal` échoue sur le zombie.

- [x] **Step 5: Commit**

```bash
git add internal/fsview docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: vivacité du process par poll du pidfd, sans pidfd_send_signal

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 8: Destination Docker sous un lien : identifiant de montage lié au montage Docker (écart a)

**Où tester :** macOS (logique portable) ; `scripts/test-linux.sh` (le conteneur de test monte `/var/run/docker.sock`, et `/var/run` → `/run` dans l'image `golang`) ; intégration Linux.

**Files:**
- Modify: `internal/fsview/mounts.go`, `internal/fsview/mounts_test.go`, `internal/fsview/procfs_archive_linux.go`, `internal/fsview/procfs_archive_linux_test.go`, `internal/fsview/integration_test.go`
- Modify: spec §5.4 puce (b), §13 (dernière ligne)

**Interfaces:**
- Consumes: `scanMountinfo`, `CheckVisibleMounts`, `PseudoMountInMounts`, `PseudoMountUnder`, `alive` (tâche 7), `procFS.root`, `pathrs.OpenatInRoot`.
- Produces: `type MountIdent struct{ MountID, Dev, Ino uint64 }` ; `type mountLine struct{ ID uint64; Root, Point, FSType string }` (champs 1, 4, 5 et type) ; `scanMountinfo(mountinfo string, visit func(mountLine) bool)` ; `LocateMounts(mountinfo string, mounts []dockerx.Mount, ident func(p string) (MountIdent, error)) []dockerx.Mount` ; `(*procFS).mountIdent(p string) (MountIdent, error)`.

> **À comprendre :** prenons `-v /var/run/docker.sock:/var/run/docker.sock` dans une image où `/var/run` → `/run`. Docker monte en `/run/docker.sock`, que `mountinfo` montre. `CheckVisibleMounts` ne trouve pas `/var/run/docker.sock` et refuse tout tar. Retrouver le montage par son identifiant (`statx` `STATX_MNT_ID`, noyau ≥ 5.8) ne suffit pas : si le PID 1 s'est chrooté sur un point de montage, des liens plantés dedans peuvent mener chaque destination vers une racine de montage visible (revue I4). Le montage trouvé doit donc **être** ce montage Docker : sa racine (`mountinfo`, champ 4) doit terminer la `Source` d'un bind, ou finir par `/<Name>/_data` pour un volume. De plus, P ne peut pas être `/`, et un même P ne peut pas servir deux destinations.

- [x] **Step 1: Tests qui échouent** — `mounts_test.go` (portable ; importer `io/fs`) :

```go
func TestLocateMounts(t *testing.T) {
	mi := strings.Join([]string{
		"20 1 0:20 / / rw - overlay overlay rw",
		"42 20 0:5 /docker.sock /run/docker.sock rw - tmpfs tmpfs rw",
		"43 20 8:1 /var/lib/docker/volumes/v/_data /run/x rw - ext4 /dev/sda1 rw",
		"44 43 0:4 / /run/x/proc rw - proc proc rw",
	}, "\n")
	ids := map[string]MountIdent{
		"/var/run/docker.sock": {42, 5, 7}, "/run/docker.sock": {42, 5, 7}, // same mount, same inode
		"/var/run/sock2":       {42, 5, 7}, // a second destination onto that mount
		"/var/run/plain":       {20, 20, 99}, "/": {20, 20, 2}, // not a mount: on "/", another inode
		"/data":                {20, 20, 2}, // links planted after a chroot lead /data to the root
		"/data2":               {43, 30, 2}, // ... or /data2 to the root of another mount
		"/var/run/x":           {43, 30, 2}, "/run/x": {43, 30, 2},
		"/var/run/stale":       {77, 1, 1}, // ID absent from the table
		"/var/run/other":       {42, 5, 8}, // same mount, another inode
	}
	ident := func(p string) (MountIdent, error) {
		if id, ok := ids[p]; ok {
			return id, nil
		}
		return MountIdent{}, fs.ErrNotExist
	}
	sock := func(dst string) dockerx.Mount {
		return dockerx.Mount{Type: "bind", Source: "/var/run/docker.sock", Destination: dst}
	}

	in := []dockerx.Mount{sock("/var/run/docker.sock")}
	got := LocateMounts(mi, in, ident)
	if got[0].Destination != "/run/docker.sock" || in[0].Destination != "/var/run/docker.sock" {
		t.Fatalf("localisé : %v (entrée %v, qui ne doit pas changer)", got, in)
	}
	if err := CheckVisibleMounts(mi, got); err != nil {
		t.Fatalf("après localisation : %v", err)
	}
	vol := []dockerx.Mount{{Type: "volume", Name: "v", Destination: "/var/run/x"}}
	if _, _, dst, bad := PseudoMountInMounts(mi, LocateMounts(mi, vol, ident)); !bad || dst != "/run/x" {
		t.Errorf("le point retrouvé nourrit la règle « tout le conteneur » : %v %v", bad, dst)
	}
	// Fail closed: each of these stays where it is, and the tar is refused.
	refused := map[string][]dockerx.Mount{
		"pas un montage":       {sock("/var/run/plain")},
		"ID inconnu":           {sock("/var/run/stale")},
		"autre inode":          {sock("/var/run/other")},
		"introuvable":          {sock("/var/run/absent")},
		"chroot vers /":        {{Type: "bind", Source: "/srv", Destination: "/data"}},
		"chroot vers un autre montage": {{Type: "bind", Source: "/srv", Destination: "/data2"}},
		"autre source":         {{Type: "bind", Source: "/srv/other.sock", Destination: "/var/run/docker.sock"}},
		"autre volume":         {{Type: "volume", Name: "w", Destination: "/var/run/x"}},
		"tmpfs":                {{Type: "tmpfs", Destination: "/var/run/docker.sock"}},
		"même point deux fois": {sock("/var/run/docker.sock"), sock("/var/run/sock2")},
		"point déjà visible":   {sock("/run/docker.sock"), sock("/var/run/docker.sock")},
	}
	for name, ms := range refused {
		if err := CheckVisibleMounts(mi, LocateMounts(mi, ms, ident)); !errors.Is(err, ErrForbiddenPath) {
			t.Errorf("ÉVASION : %s accepté", name)
		}
	}
	// A visible destination is never resolved; an unreadable ID locates nothing.
	LocateMounts(mi, []dockerx.Mount{sock("/run/docker.sock")}, func(string) (MountIdent, error) {
		t.Error("ident appelé pour une destination visible")
		return MountIdent{}, nil
	})
	if got := LocateMounts("x 20 0:20 / / rw - overlay overlay rw", in, ident); got[0].Destination != "/var/run/docker.sock" {
		t.Errorf("ID illisible : %v", got)
	}
}
```

`procfs_archive_linux_test.go` (importer `path`, `path/filepath` si absents) :

```go
// Deferred issue (a) of plan 1.
func TestCheckArchiveDestinationUnderSymlink(t *testing.T) {
	p, cmd := childView(t)
	// Control first: under a symlink but not a mount, still refused.
	dir := t.TempDir()
	must(t, os.Mkdir(filepath.Join(dir, "real"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "real", "f"), nil, 0o644))
	must(t, os.Symlink("real", filepath.Join(dir, "link")))
	p.mounts = []dockerx.Mount{{Type: "bind", Source: "/x/f", Destination: filepath.Join(dir, "link", "f")}}
	if err := p.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "ne montre pas la destination") {
		t.Fatalf("lien sans montage : %v", err)
	}
	// The real case: scripts/test-linux.sh mounts the Docker socket at
	// /var/run/docker.sock, and /var/run -> /run in the golang image. Its
	// Source is the mount's root as mountinfo shows it (host layouts differ).
	mi, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", cmd.Process.Pid))
	must(t, err)
	root := ""
	for _, line := range strings.Split(string(mi), "\n") {
		if f := strings.Fields(line); len(f) > 4 && f[4] == "/run/docker.sock" {
			root = f[3]
		}
	}
	target, err := os.Readlink("/var/run")
	if err == nil && !path.IsAbs(target) {
		target = path.Join("/var", target)
	}
	if err != nil || path.Clean(target) != "/run" || root == "" || root == "/" {
		t.Skip("précondition absente : socket Docker monté sous /var/run -> /run (lancer par scripts/test-linux.sh)")
	}
	p.mounts = []dockerx.Mount{{Type: "bind", Source: root, Destination: "/var/run/docker.sock"}}
	if err := p.CheckArchive(ctx, "/etc"); err != nil {
		t.Fatalf("socket Docker sous /var/run -> /run : %v", err)
	}
}
```

`integration_test.go` (fsview, `integration && linux` ; importer `path`) :

```go
func TestIntegrationDockerSocketUnderVarRun(t *testing.T) {
	needRoot(t)
	name := "hublot-it-sock"
	id := runTarget(t, name, "-v", "/var/run/docker.sock:/var/run/docker.sock", "alpine:3", "sleep", "300")
	out, err := exec.Command("docker", "exec", name, "readlink", "/var/run").Output() // not docker(): it would Fatalf
	l := strings.TrimSpace(string(out))
	if err == nil && !path.IsAbs(l) {
		l = path.Join("/var", l)
	}
	if err != nil || path.Clean(l) != "/run" {
		t.Skipf("précondition absente : /var/run n'est pas un lien vers /run dans alpine:3 (%q, %v)", l, err)
	}
	if err := openFor(t, id).CheckArchive(context.Background(), "/etc"); err != nil {
		t.Fatalf("tar de /etc refusé à cause du socket Docker : %v", err)
	}
}
```

Run: `go test ./internal/fsview/ -run LocateMounts 2>&1 | tail -n 3` — Expected: FAIL à la compilation (`MountIdent`, `LocateMounts` indéfinis).

- [x] **Step 2: Implémenter** — `mounts.go` :
  - `scanMountinfo(mountinfo string, visit func(mountLine) bool)` : `ID` par `strconv.ParseUint(f[0], 10, 64)`, `Root` = `f[3]` et `Point` = `f[4]`, tous deux par `unescapeMountinfo`. Un ID illisible ou un échappement invalide arrête le parcours, `return line, "…", true` (échec fermé, comme aujourd'hui). Adapter ses deux appelants, `dangerousMount` et `CheckVisibleMounts` ; aucun test ne l'appelle directement.
  - Ajouter :

```go
// MountIdent is what a container path resolves to: the mount it is on
// (statx STATX_MNT_ID, mountinfo field 1), its device and inode.
type MountIdent struct{ MountID, Dev, Ino uint64 }

// LocateMounts returns a copy of mounts where each destination the mount
// table does not show verbatim is replaced by the mount point P of the very
// same mount, when all of this holds; otherwise it stays, and
// CheckVisibleMounts refuses it (fail closed):
//   - the destination, resolved in the container (ident: pathrs, links
//     followed, never out of the root), is on mount n; P resolves to mount
//     n with the same device and inode;
//   - review I4: mount n is this Docker mount (sameMount), so links planted
//     after a chroot cannot map it onto another mount;
//   - P is not "/", and no other Docker mount shows, or is mapped to, P.
func LocateMounts(mountinfo string, mounts []dockerx.Mount, ident func(p string) (MountIdent, error)) []dockerx.Mount {
	lines := map[uint64]mountLine{}
	points := map[string]bool{}
	if _, _, bad := scanMountinfo(mountinfo, func(l mountLine) bool {
		lines[l.ID], points[l.Point] = l, true
		return false
	}); bad {
		return mounts
	}
	taken := map[string]bool{}
	for _, m := range mounts {
		if d := CleanPath(m.Destination); points[d] {
			taken[d] = true
		}
	}
	out := slices.Clone(mounts)
	for i, m := range out {
		dst := CleanPath(m.Destination)
		if points[dst] {
			continue
		}
		want, err := ident(dst)
		if err != nil {
			continue
		}
		l, ok := lines[want.MountID]
		if !ok || l.Point == "/" || taken[l.Point] || !sameMount(m, l.Root) {
			continue
		}
		if got, err := ident(l.Point); err != nil || got != want {
			continue
		}
		out[i].Destination, taken[l.Point] = l.Point, true
	}
	return out
}

// sameMount ties a mountinfo root (field 4) to a Docker mount: the end of a
// bind's Source, or the data directory of a named volume. Anything else
// (tmpfs, a bind of a host mount root shown as "/") cannot be tied: false.
func sameMount(m dockerx.Mount, root string) bool {
	switch m.Type {
	case "bind":
		return root != "/" && strings.HasSuffix(CleanPath(m.Source), root)
	case "volume":
		return m.Name != "" && strings.HasSuffix(root, "/"+m.Name+"/_data")
	}
	return false
}
```

`procfs_archive_linux.go` (importer `pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"`, comme `procfs_linux.go`) :

```go
// mountIdent resolves name in the container root (pathrs: links followed,
// never out of it) and returns its mount ID, device and inode.
func (p *procFS) mountIdent(name string) (MountIdent, error) {
	h, err := pathrs.OpenatInRoot(p.root, name)
	if err != nil {
		return MountIdent{}, err
	}
	defer h.Close()
	var st unix.Statx_t
	if err := unix.Statx(int(h.Fd()), "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_INO|unix.STATX_MNT_ID, &st); err != nil {
		return MountIdent{}, err
	}
	if st.Mask&unix.STATX_MNT_ID == 0 {
		return MountIdent{}, errors.New("statx sans STATX_MNT_ID (noyau antérieur à 5.8)")
	}
	return MountIdent{MountID: st.Mnt_id, Dev: unix.Mkdev(st.Dev_major, st.Dev_minor), Ino: st.Ino}, nil
}

// CheckArchive (spec §5.2 step 7, §5.4). The checks that read nothing in the
// container come first; ident (pathrs + statx) may meet a FUSE mount there.
func (p *procFS) CheckArchive(ctx context.Context, dir string) error {
	if p.pidfd < 0 {
		return errors.New("contrôle des montages impossible : vue sans process")
	}
	if err := HostBind(ctx, p.docker, p.mounts); err != nil {
		return err
	}
	raw, err := readMountinfo(p.pid)
	if err != nil {
		return changed("lecture de /proc/%d/mountinfo : %v", p.pid, err)
	}
	if err := alive(p.pidfd, p.pid); err != nil {
		return err
	}
	mi := string(raw)
	if mp, typ, dst, bad := PseudoMountInMounts(mi, p.mounts); bad {
		return fmt.Errorf("%w : %s monté sur %s, sous le montage %s du conteneur : aucun tar", ErrForbiddenPath, typ, mp, dst)
	}
	if mp, typ, bad := PseudoMountUnder(mi, dir); bad {
		return fmt.Errorf("%w : %s monté sur %s, le tar de %s l'inclurait", ErrForbiddenPath, typ, mp, CleanPath(dir))
	}
	mounts := LocateMounts(mi, p.mounts, p.mountIdent)
	if err := CheckVisibleMounts(mi, mounts); err != nil {
		return err
	}
	if mp, typ, dst, bad := PseudoMountInMounts(mi, mounts); bad {
		return fmt.Errorf("%w : %s monté sur %s, sous le montage %s du conteneur : aucun tar", ErrForbiddenPath, typ, mp, dst)
	}
	return nil
}
```

`HostBind` garde `p.mounts` : il juge les sources, pas les destinations.

- [x] **Step 3: Spec** — §5.4 puce (b) : « La table des montages doit montrer la racine `/` et chaque destination d'`Inspect.Mounts` telle quelle, sinon refus (PID 1 chrooté, destination sous un lien). » → « La table des montages doit montrer la racine `/` et chaque destination d'`Inspect.Mounts`, telle quelle ou retrouvée par identifiant de montage. Pour la retrouver : la destination résolue dans le conteneur (`statx` `STATX_MNT_ID`) est sur le montage n ; `mountinfo` montre n au point P, et P résolu donne le même montage, le même périphérique et le même inode. La racine du montage n (champ 4) doit terminer la `Source` du bind, ou finir par `/<nom>/_data` pour un volume. P n'est pas `/`, et il ne sert qu'une seule destination. Sinon refus (PID 1 chrooté). Les contrôles qui ne lisent rien dans le conteneur passent avant cette résolution. » §13, dernière ligne : remplacer « ; tout tar de dossier d'un tel conteneur est refusé (fail closed). Une comparaison par identifiant de montage est prévue plus tard » par « : la destination est retrouvée par identifiant de montage, lié au montage Docker (§5.4 b) ; sans preuve, refus (fail closed) ».

- [x] **Step 4: Vérifier**

Run: `go test -race ./internal/fsview/ 2>&1 | tail -n 3 && scripts/test-linux.sh -race ./internal/fsview/ -run 'CheckArchive|Locate' -v 2>&1 | grep -E '^(---|ok|FAIL)' && scripts/test-linux.sh -tags integration ./internal/fsview/ -run 'DockerSocket|CheckArchive' 2>&1 | tail -n 3`
Expected: `ok` ; sous Linux, `TestCheckArchiveDestinationUnderSymlink` **PASS** (pas SKIP). Si `TestIntegrationDockerSocketUnderVarRun` échoue sur Docker Desktop, la racine du montage ne termine pas la `Source` d'`Inspect` (socket mandataire) : relever les deux valeurs et en discuter avant tout assouplissement. Mutations : retirer `sameMount` → « chroot vers un autre montage », « autre source » et « autre volume » rougissent (« chroot vers / » reste refusé par `P ≠ "/"`) ; retirer le second `ident` → « autre inode » rougit.

- [x] **Step 5: Commit**

```bash
git add internal/fsview docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
explorateur: destination Docker sous un lien retrouvée par identifiant de montage, liée au montage Docker

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 9: `GET /api/containers/{id}/changes` (F5)

**Où tester :** macOS ; intégration Docker Desktop.

**Files:**
- Create: `internal/api/changes.go`, `internal/api/changes_test.go`
- Modify: `internal/api/server.go` (route), `internal/api/integration_stopped_test.go` (ajout de la vérification de `/changes`), spec §6 (ligne de la table)

**Interfaces:**
- Consumes: `dockerx.Client.Changes`, `containerRef`, `writeFSError`, `session`.
- Produces: `GET /api/containers/{id}/changes` → `200 [{"path","kind"}]` trié par chemin, `[]` si vide ; erreurs `not_found`, `docker_unavailable`.

- [x] **Step 1: Tests qui échouent** — `changes_test.go` :

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestChanges(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	f.docker.Diffs = map[string][]dockerx.Change{"web": {
		{Path: "/z", Kind: "added"}, {Path: "/a/b", Kind: "deleted"}, {Path: "/a", Kind: "modified"},
	}}
	if rec := f.do("GET", "/api/containers/web/changes", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	rec := f.do("GET", "/api/containers/web/changes", "", c)
	var got []dockerx.Change
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	want := []dockerx.Change{{Path: "/a", Kind: "modified"}, {Path: "/a/b", Kind: "deleted"}, {Path: "/z", Kind: "added"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("trié par chemin : %+v", got)
	}
	f.docker.Diffs["empty"] = nil
	if rec := f.do("GET", "/api/containers/empty/changes", "", c); rec.Body.String() != "[]\n" {
		t.Errorf("aucun changement : %q, attendu []", rec.Body)
	}
}

func TestChangesErrors(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	if rec := f.do("GET", "/api/containers/nope/changes", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("inconnu : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/.hidden/changes", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
	f.docker.Err = fmt.Errorf("%w : dial", dockerx.ErrUnavailable)
	if rec := f.do("GET", "/api/containers/web/changes", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker arrêté : %d %s", rec.Code, rec.Body)
	}
}
```

Run: `go test ./internal/api/ -run Changes 2>&1 | tail -n 3` — Expected: FAIL (404 JSON de la route inconnue).

- [x] **Step 2: Implémenter** — `changes.go` : `const changesTimeout = 30 * time.Second` (commentaire : Docker calcule le diff de la couche, lent sur une grosse ; pas de garde, le diff ne traverse pas les montages). `func (s *server) changes(w, r)` :
  1. `id := r.PathValue("id")` ; si `!containerRef.MatchString(id)` → `404 not_found`, sans appel à Docker ;
  2. `ctx` borné par `changesTimeout` ; `cs, err := s.d.Docker.Changes(ctx, id)` ; erreur → `writeFSError` ;
  3. `nil` → `[]dockerx.Change{}` ; `sort.Slice` par `Path` ; `writeJSON(w, 200, cs)`.

`server.go` : `mux.Handle("GET /api/containers/{id}/changes", session(s.changes))` après les routes de fichiers. À la fin de `TestIntegrationStoppedAndChanges` (`integration_stopped_test.go`, importer `encoding/json`) :

```go
	rec = f.do("GET", base+"/changes", "", c)
	var cs []dockerx.Change
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &cs) != nil ||
		!slices.Contains(cs, dockerx.Change{Path: "/root/note.txt", Kind: "added"}) {
		t.Errorf("changes : %d %s", rec.Code, rec.Body)
	}
```

- [x] **Step 3: Spec §6**, ligne `/changes` : « F5 `[{path, kind: "added"|"modified"|"deleted"}]` » → « F5 `[{path, kind: "added"|"modified"|"deleted"}]`, trié par chemin, liste complète ; diff de la couche du conteneur (volumes exclus), sans garde FUSE, délai 30 s ».

- [x] **Step 4: Vérifier**

Run: `go test -race ./internal/api/ 2>&1 | tail -n 3 && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/api/ -run IntegrationStopped 2>&1 | tail -n 3`
Expected: `ok` ×2.

- [x] **Step 5: Commit**

```bash
git add internal/api docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
diff: route GET /changes (F5), triée par chemin

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 10: Interface : explorateur d'un conteneur arrêté, progression de l'indexation

**Où tester :** macOS (Vitest, `check`, `lint`, `build`) ; essai manuel avec `scripts/dev-agent.sh`.

**Files:**
- Modify: `web/src/lib/api.ts`, `web/src/lib/api.test.ts`, `web/src/lib/files.ts`, `web/src/lib/files.test.ts`, `web/src/lib/i18n/fr.ts`, `web/src/lib/i18n/en.ts`
- Modify: `web/src/lib/components/FileBrowser.svelte`, `FilePreview.svelte`, `web/src/routes/c/[id]/+page.svelte`

**Interfaces:**
- Consumes: `202 {"error":"indexing","progress"}` (tâche 5).
- Produces: `ApiError.progress?: number` ; `indexingPercent(progress?: number): number` ; `fsErrorMessage(code: string, detail: string | undefined, stopped: boolean): string` ; prop `stopped: boolean` de `FileBrowser` et `FilePreview`.

- [x] **Step 1: Tests qui échouent** — `api.test.ts` :

```ts
describe('202 indexing (spec §6)', () => {
	const indexing = () =>
		new Response(JSON.stringify({ error: 'indexing', detail: 'indexation en cours (42 %)', progress: 0.42 }), { status: 202 });
	it('listDir rejects with the progress', async () => {
		stubFetch(async () => indexing());
		await expect(listDir('db', '/')).rejects.toMatchObject({ status: 202, code: 'indexing', progress: 0.42 });
	});
	it('previewFile rejects instead of showing a binary file', async () => {
		stubFetch(async () => indexing());
		await expect(previewFile('db', '/etc/hostname')).rejects.toMatchObject({ code: 'indexing' });
	});
	it('checkDownload rejects: the browser never saves the JSON', async () => {
		stubFetch(async () => indexing());
		await expect(checkDownload(downloadUrl('db', '/etc'))).rejects.toMatchObject({ code: 'indexing' });
	});
});
```

`files.test.ts` (importer `indexingPercent`, `fsErrorMessage`, et `t`, `errorMessage` depuis `./i18n/index.ts`) :

```ts
describe('indexingPercent', () => {
	it('never shows 100 before the end', () => {
		expect([undefined, 0, 0.42, 0.99, 1, 7, -1].map(indexingPercent)).toEqual([0, 0, 42, 99, 99, 99, 0]);
	});
});

describe('fsErrorMessage', () => {
	it('names the mount of a stopped container that cannot be explored', () => {
		expect(fsErrorMessage('forbidden_path', 'bind /proc -> /host/proc', true)).toBe(
			`${t('files.stoppedUnavailable')} (bind /proc -> /host/proc)`
		);
	});
	it('keeps the pseudo filesystem message of a running container', () => {
		expect(fsErrorMessage('forbidden_path', undefined, false)).toBe(t('files.inaccessible'));
	});
	it('explains the index cap of a stopped container', () => {
		expect(fsErrorMessage('too_large', undefined, true)).toBe(t('files.stoppedTooLarge'));
		expect(fsErrorMessage('not_found', undefined, true)).toBe(errorMessage('not_found'));
	});
});
```

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 | tail -n 8` — Expected: FAIL (`listDir` résout avec le JSON ; `indexingPercent` absent).

- [x] **Step 2: `api.ts`, `files.ts`, i18n**
  - `ApiError` : 4ᵉ paramètre `public progress?: number`. `errorOf` : `const progress = typeof body?.progress === 'number' ? body.progress : undefined;` passé au constructeur.
  - `api()` : avant `if (res.status === 204)`, `// 202 is res.ok but never a result: the index is being built (spec §6).` puis `if (res.status === 202) throw await errorOf(res);`. `get()` : `if (res.status === 202 || !res.ok) throw await errorOf(res);`.
  - `files.ts` :

```ts
/** Percent shown while a stopped container is indexed: never 100 before the end. */
export function indexingPercent(progress?: number): number {
	return Math.min(99, Math.max(0, Math.floor((progress ?? 0) * 100)));
}

/**
 * Message for a failed listing or preview. A stopped container whose mounts
 * expose the host is refused whole (spec §5.2 step 7): the detail names the
 * mount, shown as text (escaped by Svelte).
 */
export function fsErrorMessage(code: string, detail: string | undefined, stopped: boolean): string {
	if (code === 'forbidden_path') {
		if (!stopped) return t('files.inaccessible');
		return detail ? `${t('files.stoppedUnavailable')} (${detail})` : t('files.stoppedUnavailable');
	}
	if (code === 'too_large' && stopped) return t('files.stoppedTooLarge');
	return errorMessage(code);
}
```

  - i18n : supprimer `container.stoppedSoon` (fr et en). Ajouter :

| Clé | fr | en |
|---|---|---|
| `files.indexing` | `Indexation du conteneur arrêté` | `Indexing the stopped container` |
| `files.indexingHint` | `Première ouverture : l'agent lit les en-têtes de l'archive du conteneur. Les suivantes sont immédiates.` | `First opening: the agent reads the headers of the container archive. Later ones are immediate.` |
| `files.stoppedUnavailable` | `Explorateur indisponible pour ce conteneur arrêté : un de ses montages pourrait exposer l'hôte. Les changements restent consultables.` | `File explorer unavailable for this stopped container: one of its mounts could expose the host. Changes remain available.` |
| `files.stoppedTooLarge` | `Conteneur trop volumineux pour être indexé (au plus 4 Gio, volumes compris, 5 minutes ou 1 000 000 d'entrées). Démarrez-le pour l'explorer.` | `Container too large to index (at most 4 GiB, volumes included, 5 minutes or 1,000,000 entries). Start it to explore it.` |

- [x] **Step 3: Composants**

`FileBrowser.svelte` :
- Prop : `let { id, stopped = false }: { id: string; stopped?: boolean } = $props();` ; état `let indexing = $state<number | null>(null); // percent while the index is built`.
- Effet de listing : déclarer `let retry: ReturnType<typeof setTimeout> | undefined;` après `let current = true;`. Dans `.then`, `indexing = null;` en tête. Le `.catch`, après le cas 401 et `listing = null;`, devient :

```ts
				if (e instanceof ApiError && e.code === 'indexing') {
					indexing = indexingPercent(e.progress);
					retry = setTimeout(() => reloads++, 1000); // ask again until the index is ready
					return;
				}
				indexing = null;
				error = e instanceof ApiError ? fsErrorMessage(e.code, e.detail, stopped) : errorMessage('unknown');
```

  et le nettoyage : `return () => { current = false; clearTimeout(retry); };`. Le cas `status === 501` disparaît.
- Balisage : avant `{#if error}`, insérer un premier cas ; `{#if error}` devient `{:else if error}` :

```svelte
		{#if indexing !== null}
			<p role="status">{t('files.indexing')} : {indexing} %</p>
			<progress max="100" value={indexing}></progress>
			<p class="note">{t('files.indexingHint')}</p>
```

- `<FilePreview {id} path={file} entry={selected} {stopped} />`.

`FilePreview.svelte` : même prop `stopped`, même état `indexing`, même minuterie `retry` (nettoyée avec `URL.revokeObjectURL`). Dans le `.catch` : cas `indexing` identique ; sinon `error = code === 'too_large' ? t('files.tooLarge') : fsErrorMessage(code, e instanceof ApiError ? e.detail : undefined, stopped);` (le 501 disparaît). Afficher `{t('files.indexing')} : {indexing} %` et la barre comme dans `FileBrowser`. Le bouton « Recharger » de l'état d'erreur reste.

`+page.svelte` : remplacer le bloc `{#if container.state === 'running' || …}…{:else}…{/if}` par `<FileBrowser id={container.id} stopped={container.state !== 'running' && container.state !== 'paused'} />`.

- [x] **Step 4: Vérifier**

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 | tail -n 5 && npm run check 2>&1 | tail -n 3 && npm run lint 2>&1 | tail -n 3 && npm run build 2>&1 | tail -n 2 && grep -rn 'stoppedSoon\|501' src | wc -l`
Expected: tests verts, `check` et `lint` sans erreur, build OK, `0`.

Essai manuel : `make web && scripts/dev-agent.sh`, puis dans un autre terminal `docker run --name hk-big node:22 true`. Ouvrir `hk-big` : barre de progression, puis le listing sans recharger la page. Ensuite `mkdir -p /tmp/hk-bind && docker run --name hk-bind -v /tmp/hk-bind:/data alpine:3 true` doit s'explorer (`/tmp` est partagé par Docker Desktop, `/srv` ne l'est pas). Ne **pas** essayer un bind de `/proc` contre le vrai démon (règle C1). Relever la durée d'indexation de `node:22` pour la tâche 13.

- [x] **Step 5: Commit**

```bash
git add web/src
git commit -F - <<'EOF'
explorateur: interface d'un conteneur arrêté, progression de l'indexation, 202 reconnu par api.ts

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 11: Interface : onglet Changements et marques du diff (F5)

**Où tester :** macOS.

**Files:**
- Create: `web/src/lib/changes.ts`, `web/src/lib/changes.test.ts`, `web/src/lib/components/ChangesList.svelte`
- Modify: `web/src/lib/api.ts`, `web/src/lib/i18n/fr.ts`, `en.ts`, `web/src/lib/components/FileBrowser.svelte`, `web/src/routes/c/[id]/+page.svelte`

**Interfaces:**
- Consumes: `GET /changes` (tâche 9).
- Produces: `type Change = { path: string; kind: 'added' | 'modified' | 'deleted' }` et `listChanges(id: string): Promise<Change[]>` dans `api.ts` ; dans `changes.ts` : `changeMarks`, `deletedIn`, `countChanges`, `kindLabel`, `kindLetter`, `CHANGES_SHOWN = 1000`.

- [x] **Step 1: Tests qui échouent** — `changes.test.ts` :

```ts
import { describe, expect, it } from 'vitest';
import type { Change } from './api';
import { changeMarks, countChanges, deletedIn, kindLetter } from './changes';

const cs: Change[] = [
	{ path: '/etc', kind: 'modified' },
	{ path: '/etc/motd', kind: 'deleted' },
	{ path: '/etc/new.conf', kind: 'added' },
	{ path: '/etc/ssl/old.pem', kind: 'deleted' },
	{ path: '/gone', kind: 'deleted' }
];

describe('changes', () => {
	it('marks each path with its kind', () => {
		const m = changeMarks(cs);
		expect(m.get('/etc/new.conf')).toBe('added');
		expect(m.get('/etc')).toBe('modified');
		expect(m.get('/usr')).toBeUndefined();
	});
	it('lists the deleted children of a folder, not its grandchildren', () => {
		expect(deletedIn(cs, '/etc')).toEqual(['motd']);
		expect(deletedIn(cs, '/')).toEqual(['gone']);
		expect(deletedIn(cs, '/etc/ssl')).toEqual(['old.pem']);
		expect(deletedIn(cs, '/e')).toEqual([]);
	});
	it('counts per kind', () => {
		expect(countChanges(cs)).toEqual({ added: 1, modified: 1, deleted: 3 });
	});
	it('gives a letter per kind', () => {
		expect(['added', 'modified', 'deleted'].map((k) => kindLetter(k as Change['kind']))).toEqual(['A', 'M', 'D']);
	});
});
```

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run src/lib/changes.test.ts 2>&1 | tail -n 4` — Expected: FAIL (module absent).

- [x] **Step 2: Logique** — `api.ts` : le type `Change` et `export const listChanges = (id: string) => api<Change[]>(\`/containers/${encodeURIComponent(id)}/changes\`);`. `changes.ts` (imports : `type Change` de `./api.ts`, `t` de `./i18n/index.ts`) :
  - `type ChangeKind = Change['kind']` ; `CHANGES_SHOWN = 1000` (Q12) ;
  - `changeMarks(changes): Map<string, ChangeKind>` : chemin → type ;
  - `deletedIn(changes, dir): string[]` : préfixe `dir === '/' ? '/' : dir + '/'`, garde les `deleted` dont le reste ne contient pas de `/`, renvoie ce reste (enfants directs seulement) ;
  - `countChanges(changes): Record<ChangeKind, number>` ;
  - `kindLetter(kind): 'A' | 'M' | 'D'` ; `kindLabel(kind)` : `t('changes.added' | 'changes.modified' | 'changes.deleted')`.

i18n :

| Clé | fr | en |
|---|---|---|
| `changes.hint` | `Différences avec l'image, calculées par Docker (hors volumes).` | `Differences from the image, computed by Docker (volumes excluded).` |
| `changes.empty` | `Aucun changement par rapport à l'image.` | `No change from the image.` |
| `changes.added` | `ajouté` | `added` |
| `changes.modified` | `modifié` | `modified` |
| `changes.deleted` | `supprimé` | `deleted` |
| `changes.more` | `changements de plus, non affichés` | `more changes, not shown` |
| `changes.deletedHere` | `supprimé(s) dans ce dossier depuis l'image :` | `deleted in this folder since the image:` |

- [x] **Step 3: Composants**

`ChangesList.svelte` (nouveau ; prop `id`) : même squelette d'effet que `FileBrowser` (`const wanted = id`, `void reloads`, drapeau `current`, 401 → `/login`). États : erreur (`role="alert"` + bouton Recharger qui fait `reloads++`), chargement (`files.loading`), vide (`changes.empty`). Sinon : `changes.hint` en tête ; une ligne de décompte `{t('changes.added')} : n · …` via `{@const n = countChanges(changes)}` ; une `<ul>` des `CHANGES_SHOWN` premiers changements. Chaque `<li>` est une grille `1.5rem 5.5rem minmax(0, 1fr)` avec `<abbr class="mark {kind}" title={kindLabel(kind)}>{kindLetter(kind)}</abbr>`, le libellé en clair, puis le chemin. Le chemin est `<s>` pour un supprimé, sinon `<a href={'?' + new URLSearchParams({ path })}>` (l'explorateur ouvre un fichier ainsi). Au-delà de `CHANGES_SHOWN`, `{changes.length - CHANGES_SHOWN} {t('changes.more')}`. Couleurs : ajouté `#1b7f3b`, modifié `#8a5a00`, supprimé `#b00020`. Le chemin vient du conteneur : du texte, jamais `{@html}`.

`FileBrowser.svelte` : importer `listChanges`, `type Change` et `changeMarks`, `deletedIn`, `kindLabel`, `kindLetter`. État `changes = $state<Change[]>([])`, `marks = $derived(changeMarks(changes))`, `deleted = $derived(deletedIn(changes, dir))`. Un second `$effect`, qui ne dépend **que** de `id` (pas de `reloads` : la relance d'indexation toutes les secondes ne doit pas redemander le diff), appelle `listChanges(id)` avec son drapeau `current` et un `.catch(() => {})` : en cas d'échec, pas de marques (décision du plan).

Dans la ligne du tableau, juste après `{#each entries as e, i (i)}`, `{@const k = marks.get(joinPath(dir, e.name))}`, et après le bouton du nom : `{#if k}<abbr class="mark {k}" title={kindLabel(k)}>{kindLetter(k)}</abbr>{/if}`. Après le tableau : `{#if deleted.length}<p class="note">{deleted.length} {t('changes.deletedHere')} {deleted.join(', ')} — <a href="?tab=changes">{t('container.tab.changes')}</a></p>{/if}`. Styles `.mark`, `.added`, `.modified` comme dans `ChangesList`.

`+page.svelte` : `let tab = $derived(page.url.searchParams.get('tab') === 'changes' ? 'changes' : 'files');`. Les deux premiers `<span class="tab">` deviennent des liens :

```svelte
			<a class="tab" class:active={tab === 'files'} aria-current={tab === 'files' ? 'page' : undefined} href="?">{t('container.tab.files')}</a>
			<a class="tab" class:active={tab === 'changes'} aria-current={tab === 'changes' ? 'page' : undefined} href="?tab=changes">{t('container.tab.changes')}</a>
```

et le contenu : `{#if tab === 'changes'}<ChangesList id={container.id} />{:else}<FileBrowser … />{/if}`.

- [x] **Step 4: Vérifier**

Run: `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 | tail -n 5 && npm run check 2>&1 | tail -n 3 && npm run lint 2>&1 | tail -n 3 && npm run build 2>&1 | tail -n 2`
Expected: tout vert. Essai manuel (agent de dev) : onglet Changements d'un conteneur modifié, lien vers un fichier ajouté, marque `A` dans l'explorateur, retour arrière du navigateur, largeur téléphone (390 px) sans débordement horizontal.

- [x] **Step 5: Commit**

```bash
git add web/src
git commit -F - <<'EOF'
diff: onglet Changements et marques du diff dans l'explorateur (F5)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 12: Parcours Playwright unique et CI (matrice overlay2 / containerd, job e2e)

**Où tester :** Docker Desktop (script e2e) ; GitHub Actions (après push, à la fin de la phase).

**Files:**
- Create: `web/playwright.config.ts`, `web/e2e/parcours.e2e.ts`, `scripts/e2e.sh`
- Modify: `web/package.json`, `web/package-lock.json`, `.github/workflows/ci.yml`, `.gitignore` (`web/test-results/`, `web/playwright-report/`), spec §10

**Interfaces:**
- Consumes: l'agent complet ; `reset-token` (« Nouveau jeton d'accès (affiché une seule fois) : <jeton> ») ; le champ `#token` et le bouton `submit` de `/login` ; le lien du nom de conteneur dans `/` ; les boutons `root/` et `note.txt` de l'explorateur ; `.preview` ; `abbr.mark.added` ; l'onglet « Changements ».
- Produces: `scripts/e2e.sh` (variables `HUBLOT_URL`, `HUBLOT_TOKEN` pour Playwright).

- [x] **Step 1: Dépendance** — `cd web && export PATH=/opt/homebrew/bin:$PATH && npm i -D -E @playwright/test 2>&1 | tail -n 2 && npx playwright install chromium 2>&1 | tail -n 1`. La version exacte installée est figée dans `package.json`.

- [x] **Step 2: Config et parcours**

`web/playwright.config.ts` :

```ts
import { defineConfig, devices } from '@playwright/test';

// The single journey of spec §10, run by scripts/e2e.sh against a real agent.
// *.e2e.ts, never *.spec.ts: Vitest would pick those up.
export default defineConfig({
	testDir: 'e2e',
	testMatch: '*.e2e.ts',
	timeout: 60_000,
	expect: { timeout: 15_000 },
	retries: 0,
	reporter: 'list',
	use: { baseURL: process.env.HUBLOT_URL ?? 'http://127.0.0.1:7879', locale: 'fr-FR', trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], locale: 'fr-FR' } }]
});
```

`web/e2e/parcours.e2e.ts` :

```ts
import { expect, test } from '@playwright/test';

// Spec §10: login → list → open a file → see the diff. scripts/e2e.sh starts
// the agent and the stopped container hublot-e2e, whose /root/note.txt was
// added after the image: this also runs tarindex end to end.
test('connexion, liste, fichier, diff', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN;
	if (!token) throw new Error('HUBLOT_TOKEN absent : lancer par scripts/e2e.sh');
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await page.getByRole('link', { name: 'hublot-e2e', exact: true }).click();
	await page.getByRole('button', { name: 'root/', exact: true }).click(); // after the index is built
	await page.getByRole('button', { name: 'note.txt', exact: true }).click();
	await expect(page.locator('.preview')).toContainText('bonjour hublot');
	await expect(page.getByRole('row', { name: /note\.txt/ }).locator('abbr.mark.added')).toBeVisible();
	await page.getByRole('link', { name: 'Changements', exact: true }).click();
	await expect(page.getByRole('listitem').filter({ hasText: '/root/note.txt' })).toContainText('ajouté');
});
```

- [x] **Step 3: `scripts/e2e.sh`** (exécutable : `chmod +x`) :

```bash
#!/usr/bin/env bash
# Parcours Playwright unique (spec §10) contre un vrai agent et un vrai Docker.
# Agent dans un conteneur (mêmes droits que scripts/dev-agent.sh, port 7879),
# conteneur arrêté hublot-e2e, puis Playwright ; tout est nettoyé à la fin.
# Prérequis : Docker, Node ≥ 22.17 (sur le Mac : export PATH=/opt/homebrew/bin:$PATH),
# `cd web && npx playwright install chromium` fait une fois.
set -euo pipefail
cd "$(dirname "$0")/.."
make web >/dev/null
arch=$(docker version -f '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o bin/hublot-linux ./cmd/hublot
cleanup() {
  docker rm -f hublot-e2e hublot-e2e-agent >/dev/null 2>&1 || true
  docker volume rm -f hublot-e2e-data >/dev/null 2>&1 || true
}
cleanup
trap cleanup EXIT
docker run --name hublot-e2e alpine:3 sh -c 'echo "bonjour hublot" > /root/note.txt'
tok=$(docker run --rm -v hublot-e2e-data:/data -v "$PWD/bin/hublot-linux:/hublot:ro" alpine:3 \
  /hublot reset-token --data-dir /data | sed -n 's/^.* : //p')
test -n "$tok"
docker run -d --name hublot-e2e-agent \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD/bin/hublot-linux:/hublot:ro" -v hublot-e2e-data:/data \
  -p 127.0.0.1:7879:7878 \
  alpine:3 /hublot --listen 0.0.0.0:7878 --data-dir /data >/dev/null
for _ in $(seq 1 40); do curl -sf -o /dev/null http://127.0.0.1:7879/login && break; sleep 0.5; done
curl -sf -o /dev/null http://127.0.0.1:7879/login || { echo "l'agent ne répond pas" >&2; docker logs hublot-e2e-agent 2>&1 | tail -n 20; exit 1; }
cd web && HUBLOT_URL=http://127.0.0.1:7879 HUBLOT_TOKEN="$tok" npx playwright test
```

Run: `export PATH=/opt/homebrew/bin:$PATH && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock scripts/e2e.sh 2>&1 | tail -n 8` puis `cd web && npx vitest run 2>&1 | tail -n 3`
Expected: `1 passed` ; Vitest n'a pas ramassé `parcours.e2e.ts` (même nombre de fichiers de test qu'avant). En cas d'échec : `docker logs hublot-e2e-agent | tail -n 20` avant le nettoyage (retirer temporairement le `trap`).

- [x] **Step 4: CI** — `.github/workflows/ci.yml` :
  - Job `integration` : ajouter, sous `runs-on`,

```yaml
    strategy:
      fail-fast: false
      matrix:
        storage: [overlay2, containerd]
```

  et, avant l'étape `go test`,

```yaml
      - name: Stockage Docker (${{ matrix.storage }})
        run: |
          want=${{ matrix.storage == 'containerd' && 'true' || 'false' }}
          sudo mkdir -p /etc/docker
          [ -f /etc/docker/daemon.json ] || echo '{}' | sudo tee /etc/docker/daemon.json >/dev/null
          sudo jq --argjson v "$want" '.features["containerd-snapshotter"] = $v' /etc/docker/daemon.json | sudo tee /etc/docker/daemon.json.new >/dev/null
          sudo mv /etc/docker/daemon.json.new /etc/docker/daemon.json
          sudo systemctl restart docker
          status=$(docker info -f '{{json .DriverStatus}}')
          echo "$(docker info -f '{{.Driver}}') $status"
          if [ "$want" = true ]; then echo "$status" | grep -q io.containerd.snapshotter
          else ! echo "$status" | grep -q io.containerd.snapshotter; fi
```

  (fusion avec le `daemon.json` existant du runner, qui peut porter d'autres réglages).
  - Nouveau job `e2e` (`runs-on: ubuntu-latest`, `needs: [go, web]`, commentaire `# Spec §10: the single Playwright journey, real agent and real Docker.`) : `actions/checkout@v7`, `actions/setup-go@v7` (`go-version-file: go.mod`), `actions/setup-node@v7` (Node 24, cache npm de `web/package-lock.json`, comme le job `web`), puis `npm ci` et `npx playwright install --with-deps chromium` dans `web/`, enfin `scripts/e2e.sh` depuis la racine.

  Si une règle de protection de `main` exige le contexte `integration`, la mettre à jour : il devient `integration (overlay2)` et `integration (containerd)`.

- [x] **Step 5: Spec §10** — puce « Interface » : après « un seul parcours Playwright : connexion → liste → ouvrir un fichier → voir le diff », ajouter « (`web/e2e/parcours.e2e.ts`, lancé par `scripts/e2e.sh` contre un agent réel et un conteneur arrêté) ». Puce « Intégration » : « matrice overlay2 / containerd configurée par `daemon.json` » → « matrice overlay2 / containerd (`features.containerd-snapshotter` dans `daemon.json`, vérifiée par `docker info`) ».

- [x] **Step 6: Vérifier et commiter**

Run: `python3 -I -c "import sys,yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo yaml-ok; cd web && export PATH=/opt/homebrew/bin:$PATH && npm run check 2>&1 | tail -n 3 && npm run lint 2>&1 | tail -n 3`
Expected: `yaml-ok` (si PyYAML manque, `ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml")' && echo yaml-ok`) ; `check` et `lint` sans erreur.

```bash
git add web/playwright.config.ts web/e2e web/package.json web/package-lock.json scripts/e2e.sh .github/workflows/ci.yml .gitignore docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
git commit -F - <<'EOF'
ci: parcours Playwright unique, matrice overlay2 / containerd pour l'intégration

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 13: Vérification sur les VM, mesure et clôture

**Où tester :** `hk-ubuntu` (AppArmor), `hk-fedora` (SELinux), Docker Desktop.

**Files:**
- Modify: spec §5.2 (paragraphe « Spike » : mesure), `docs/superpowers/plans/2026-10-09-explorateur-2-tarindex-diff.md` (cases cochées, constats)

- [x] **Step 1: Suite complète sur les deux VM**

Run (pour chaque VM) : `limactl start hk-ubuntu >/dev/null && limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 | grep -E '^(ok|FAIL|---)|MESURE' | tail -n 30`, puis idem `hk-fedora`.
Expected: `ok` pour chaque paquet. `TestCheckArchiveDestinationUnderSymlink` et `TestIntegrationDockerSocketUnderVarRun` passent (pas de SKIP). Lignes `MESURE` relevées.

- [x] **Step 2: Mesures** — avec l'agent de dev, indexer un conteneur arrêté de `node:22` : nombre d'entrées (`docker export hk-big | tar -t | wc -l`) et durée (premier 202 → premier 200 dans l'onglet Réseau). Reprendre la ligne `MESURE` de `TestMeasureIndexMemory` (tâche 3 : octets par entrée). Ajouter au paragraphe « Spike » de §5.2 : « Mesuré (plan Explorateur 2, Docker Desktop) : `node:22`, <entrées> entrées, <durée> s ; index : <octets> octets par entrée, soit <Mo> Mo pour 1 000 000 d'entrées. » Si un index de 1 000 000 d'entrées dépasse 300 Mo, baisser `MaxHeaders` en conséquence, dans le code et la spec.

- [x] **Step 3: Revue de branche** — demander la revue complète de la branche (sous-agent indépendant + Codex), corriger, puis cocher les cases de ce plan et y noter les constats.

- [x] **Step 4: Commit**

```bash
git add docs/superpowers
git commit -F - <<'EOF'
docs: Explorateur 2/3 vérifié sur hk-ubuntu et hk-fedora, mesure d'indexation

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

#### Constats de la tâche 13

- hk-ubuntu (AppArmor) et hk-fedora (SELinux) : `go test -race -tags integration ./cmd/... ./internal/...` donne `ok` pour chaque paquet. `TestCheckArchiveDestinationUnderSymlink` et `TestIntegrationDockerSocketUnderVarRun` passent sur les deux, sans SKIP.
- Mesure : `TestMeasureIndexMemory` donne 237 octets par entrée sur les deux VM, soit 237 Mo pour 1 000 000 d'entrées, sous le seuil de 300 Mo : `MaxHeaders` reste à 1 000 000.
- `node:22` sur hk-fedora : 33 538 entrées (`docker export | tar -t | wc -l`) ; l'index lui-même se construit en 1,0 s depuis un fichier tar, l'export du démon prend 7 à 8 s. Le délai 202 → 200 dans l'onglet Réseau n'a pas été relevé (pas d'essai manuel de l'interface).
- Step 3 (revue de branche) : la revue finale (Opus + Codex) a conclu « prête à fusionner avec corrections » ; les corrections sont appliquées dans ce commit. La CI de la PR reste à observer.
