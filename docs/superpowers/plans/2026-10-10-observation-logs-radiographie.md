# Observation — Logs en direct (F6) et radiographie (F7) — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (règle du dépôt, `CLAUDE.md`) :** chaque tâche est relue par un sous-agent indépendant **et** par Codex avant la suivante. Relecture prioritaire : tâche 1 (évasion, spec §13), tâches 4 et 5 (aucun secret ne quitte l'agent), tâche 3 (flux qui ne se ferme pas, goroutines et créneaux).
>
> **Compilé et vérifié avant rédaction.** Tout le code de ce plan a été écrit puis exécuté sur une copie isolée du dépôt (`git worktree` sur `a6ffdeb`, un commit de tests puis un commit de code par tâche, supprimée ensuite). Les blocs `diff` ci-dessous sont **extraits de cette copie** par `git diff`, pas recopiés ; ils ont ensuite été **réappliqués** dans l'ordre du plan (`git apply`, puis les remplacements de la tâche 9) sur une seconde copie propre de `a6ffdeb`, qui redonne exactement l'arbre vérifié (même `git write-tree`).
> - Go : `go vet ./...` et `go vet -tags integration ./...`, en macOS et en `GOOS=linux` ; `gofmt -l` vide ; `go test -race ./cmd/... ./internal/...` vert sur macOS ; `scripts/test-linux.sh -race -tags integration -count=1 ./cmd/... ./internal/...` vert (Docker Desktop 29.7.2, API 1.55, conteneur `golang` root) ; en `-v`, 368 PASS et un seul SKIP, préexistant et attendu sur Docker Desktop (`TestIntegrationDockerSocketUnderVarRun` : socket mandataire) ; aucun test de ce plan ne se saute ; `go list -deps ./cmd/hublot` ne contient pas `testing`.
> - Rouge avant le code, à chaque tâche (le commit de tests seul) : tâche 1, compilation (`not enough arguments in call to newTarFS`, et sous Linux `CheckWriteRoot returns 1 value`), deux tests de `files` en échec, et le défaut de §13 reproduit sur un vrai Docker (`TestIntegrationWriteTmpfsUnderLink` : envoi dans `/etc` → `403 forbidden_path`) ; tâches 2 à 5, compilation (`undefined: LogLine`, `undefined: logLimits`, `undefined: SecretName`, `unknown field Mode`) ; tâches 7 et 8, Vitest (3 et 2 fichiers en échec).
> - Mutations (une fois chacune, puis annulées), toutes attrapées : garde « P n'est pas `/` » retirée de `LocateMounts` → `TestLocateMountsTmpfs` (prison), `TestLocateMountsChrootToRootMountEndingSource`, `TestIntegrationCheckWriteRootTmpfsUnderLink` ; garde `runtimeMount` retirée → `TestLocateMountsTmpfs` (leurre `/dev/shm`) ; points retrouvés ignorés par `checkRoot` → `TestWriteRefusedUnderResolvedTmpfs` ; recollage des parties TTY retiré → `TestReadLogsTTYPartialMessages` ; `rc.Close()` retiré du flux → `TestLogsClientGone` ; règle d'URL retirée → `TestEnv`, `TestEnvNeverLeaks` (« FUITE ») ; commande non masquée → `TestContainerRadiography` (« FUITE ») ; échec du journal ignoré → `TestRevealEnvRefusals`.
> - Mesuré sur Docker Desktop (voir « Constats de la rédaction ») : format des logs (trames, horodatage, parties de 16 Kio, TTY), `--log-driver none` → 501, `since` inclusif, `/var/run` → `../run` dans `alpine:3`, arrêt de l'agent avec un flux ouvert (6 s et code 1 sans le correctif de la tâche 3, 1 s et code 0 avec).
> - Interface : Vitest 76 tests verts ; `npm run check` 0 erreur ; `npm run lint` propre ; `vite build` vert. Contre un vrai agent (même montage que `scripts/e2e.sh`) : le parcours unique passe, et le parcours jetable de la tâche 8 (non commité) passe : masquage, révélation, flux en direct, recherche, pause puis reprise sans doublon ni trou, téléchargement, conteneur arrêté, 390 px sans défilement horizontal.
> - **Revérifié après la revue 1** (même méthode : copie reconstruite depuis les blocs du plan `df000d7`, un commit par étape, corrections R1 à R9 appliquées en correctifs de la tâche concernée, puis réappliquées sur une seconde copie propre : même arbre ; copies supprimées). `go vet` macOS et Linux, avec et sans `integration` ; `gofmt` vide ; `go test -race` macOS ; `scripts/test-linux.sh -race -tags integration -count=1 -v` : 375 PASS, le seul SKIP préexistant ; `go test -race -count=30 -run 'Logs|StreamSlots'` stable ; Vitest 76, `check`, `lint`, `build` verts ; parcours unique et parcours jetable rejoués contre un vrai agent (arrêt : 0 s, code 0). Rouge avant le code inchangé à chaque tâche (compilation pour 1 à 5). Mutations nouvelles, toutes attrapées : repli fermé de `Args` retiré → `TestArgs` (« FUITE : mysql -ps3cr3t --password ») ; contrôle de session retiré → `TestLogsSessionEnds` ; délai d'écriture retiré → `TestLogsWriteDeadline` ; éviction retirée → `TestStreamSlots`, `TestLogsEvictsOldest` ; garde « P n'est pas `/` » retirée → `TestCheckArchiveTmpfsJail` (« tar accepté dans une prison tmpfs ») ; `State.Error` non masqué → `TestContainerRadiography` (« FUITE : s3cr3t-err »).
> - **Revérifié après la revue 2** (même méthode, copie reconstruite depuis `14845ec`, corrections R11 à R17 en correctifs de leur tâche, puis supprimée). `go vet` macOS et Linux, avec et sans `integration` ; `gofmt` vide ; `go test -race` macOS ; `-count=30` sur les logs ; `scripts/test-linux.sh -race -tags integration -count=1 -v` vert (376 PASS, le seul SKIP préexistant) ; Vitest 77, `check`, `lint`, `build` ; parcours jetable rejoué. Le cas Codex et les contrôles de I1 sont dans le même `TestArgs` et passent ensemble. Mutations nouvelles, attrapées : test de fuite rendu global (`strings.Contains(s[loc[1]:], Mask)`) → `TestArgs` (« FUITE : echo --password=••••••(cat /f) … ») ; exemption des chemins retirée → `TestArgs` (`--cert=/etc/ssl/private/x.pem` masqué).
> - **Non vérifié** : les VM `hk-ubuntu` et `hk-fedora` (tâche 9).

**Goal:** Depuis la page d'un conteneur, l'utilisateur suit ses logs en direct (stdout et stderr, recherche, pause, téléchargement, reprise après coupure, logs passés d'un conteneur arrêté) et lit sa radiographie (image, état, redémarrages, commande, variables d'environnement aux secrets masqués par l'agent et révélables une à une avec journal, ports, réseaux, montages). En préalable, un `--tmpfs` placé sous un lien ne bloque plus toute écriture dans le conteneur.

**Architecture:** `dockerx` gagne `Logs` (le flux brut de Docker, horodaté) et `ReadLogs`, qui démultiplexe les trames (ou lit le flux TTY), recolle les lignes que Docker coupe à 16 Kio et plafonne leur taille sans jamais lire une trame entière en mémoire ; `ContainerInfo` gagne `Tty`, `LogDriver` et `Details` (la radiographie). `api` sert `GET /logs` en SSE (`id` = horodatage et rang, événements `end`, `dropped`, `failure`, battement de cœur), avec un plafond de débit et de flux ouverts, et `GET /logs/download` ; `GET /api/containers/{id}` et `POST /env/reveal` passent par un nouveau paquet `secrets` (§4), seul à décider du masquage (§7). Pour le correctif, `LocateMounts` retrouve un tmpfs par identifiant de montage, `CheckWriteRoot` rend les points de tmpfs retrouvés (index pour un conteneur arrêté) et `files` refuse d'écrire dessous. L'interface gagne les onglets Logs (`EventSource`) et Radiographie, et l'en-tête de la page lit `GET /api/containers/{id}`.

**Tech Stack:** Go 1.27 (`net/http`, `encoding/binary`, `bufio`, `regexp`), `github.com/moby/moby/client` v0.6.1 (`ContainerLogs`), `github.com/moby/moby/api` v1.56.1 (types `container`, `network`), `github.com/containerd/errdefs` v1.0.0 (`IsNotImplemented`) ; aucune dépendance nouvelle (le démultiplexage est écrit, pas `stdcopy`). SvelteKit 3 / Svelte 5, Vitest, `EventSource` du navigateur ; aucune dépendance d'interface nouvelle.

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§2 F6 et F7, §3 « Flux temps réel », §4, §5.3 étape 2, §6, §7, §9, §10, §12 « Observation », §13). Plan précédent (format, décisions en vigueur, Q1 à Q23) : `docs/superpowers/plans/2026-10-09-explorateur-3-ecriture.md`.

## Décisions de revue

Revue du plan, premier tour (relecteur Opus et Codex, « Prêt avec réserves »). Une ligne par correction, avec la tâche qui l'applique.

- **R1 (Opus I1, Codex) :** le masquage de la commande laissait passer `PGPASSWORD="x"`, `PGPASSWORD='x'`, `--requirepass "x"`, `--env=DB_PASSWORD=x`, la fin de `--password=pa ss` et de `--password=s3"cr3t`, et `["--password=", "x"]` (mesuré). `Args` accepte les valeurs entre guillemets (une apostrophe ou un guillemet non fermé court jusqu'à la fin), cherche une affectation dans la valeur d'une autre, masque tout le reste de l'argument hors d'une ligne de shell (argument qui ne suit pas `-c`), masque l'argument qui suit `--nom=` vide, et **échoue fermé** : un mot de §7 resté sans rien de masqué après lui fait masquer l'argument entier (`Line`). Cas exacts écrits d'abord dans `TestArgs`, avec la vérification « FUITE ». → Tâche 4 ; Q27.
- **R2 (Opus I2) :** une valeur au nom neutre portait un secret (`ConnectionStrings__Default=…;Password=…;`, `…?password=…`, `JAVA_OPTS=-D…Password=…`). `Masked` ajoute `secretAssignment(value)` ; `TestEnvNeverLeaks` gagne ces cas et deux témoins qui restent visibles. → Tâches 4, 5, 9 ; Q36.
- **R3 (Opus I3) :** un flux survivait à la déconnexion et à l'expiration de la session. À chaque battement, `sessionValid` relit le cookie (`Sessions.Valid`) ; sinon `failure` `unauthorized`, puis fermeture ; l'interface renvoie à la connexion. Test d'abord : `TestLogsSessionEnds`. → Tâches 3, 7 ; Q37.
- **R4 (Opus I4) :** un client parti sans FIN gardait un créneau environ 15 min, et son `EventSource` en reprenait un autre. Chaque écriture du flux a un délai de 30 s (`SetWriteDeadline` par `http.ResponseController`) ; quand les 8 créneaux sont pris, le flux SSE le plus ancien cède le sien (`event: evicted`, sans reconnexion côté interface) ; un téléchargement n'est jamais évincé, et s'il ne reste que des téléchargements, le nouveau venu est refusé. Tests d'abord : `TestStreamSlots`, `TestLogsEvictsOldest`, `TestLogsDownloadSlots`, `TestLogsWriteDeadline`. → Tâches 3, 7 ; Q33, Q37.
- **R5 (Opus I5) :** le changement de `sameMount` vaut aussi pour le tar d'un dossier (§5.4 b) : un `--mount type=tmpfs` sous un lien n'y refuse plus tout tar. Tests d'évasion ajoutés à la tâche 1 : `TestCheckArchiveTmpfsJail` (prison, la garde « P n'est pas `/` » seule refuse ; mutation attrapée) et `TestIntegrationCheckArchiveTmpfsUnderLink` (alpine, tar de `/etc` accepté) ; §5.4 (b) mis à jour par la tâche 9. → Tâches 1, 9 ; Q38.
- **R6 (Codex) :** reprise après des lignes écartées par le limiteur, horodatages identiques : `TestLogsResumeAfterDrops` montre qu'aucune ligne envoyée n'est renvoyée et qu'aucune n'est perdue ; les lignes `dropped` reviennent à la reprise (elles n'avaient été qu'annoncées). Pas de changement de code ; limites écrites dans Q30 et §13. → Tâche 3 ; Q30.
- **R7 (Codex Minor) :** « journalisée » pour un téléchargement interrompu désignait le journal de l'agent (`slog`), pas l'audit. La prose suit le code : §7 ne journalise que `login.fail`, `env.reveal`, `file.write` et `file.upload` ; une lecture interrompue est un incident technique, pas une action à tracer. → Décisions.
- **R8 (Opus Minor 1) :** `tarFS.CheckWriteRoot` résout une destination tmpfs aussi loin qu'elle existe, puis y recolle le reste (`index.walk`, tiré de `resolve`) : `/dangling/x`, avec `/dangling → ../real` absent, donne `/real/x`, là où runc le créera. → Tâche 1 ; Q25.
- **R9 (Opus Minor 3, 4, 8) :** `TestLogsClientGone` ne prouve plus la libération du créneau par une seconde ouverture (instable sous charge, et sans objet avec l'éviction) : `TestStreamSlots` la prouve. Le lien de téléchargement des logs disparaît quand le flux est en erreur (sinon le navigateur enregistrait le JSON d'erreur). `State.Error` passe par `secrets.Line` (un message de Docker peut citer la commande). → Tâches 3, 5, 7.
- **R10 (Opus Minor 2, 5, 6, 7), non retenus :** `readRaw` est gardé (testé, mutation attrapée) ; les limites de reprise sont écrites (Q30, §13) ; la surface en plus de la radiographie (MAC, passerelle, alias, utilisateur) reste, elle est petite et utile au diagnostic ; pas de test Go de l'arrêt de l'agent : `run` crée son propre client Docker et ne peut pas tenir un flux ouvert sans démon, l'arrêt reste mesuré par le script jetable de la tâche 8.

Second tour (relecteur Opus et Codex, « Prêt avec réserves ») :

- **R11 (Opus I1) :** le repli fermé masquait en entier des commandes banales, sans révélation possible : `/opt/keycloak/bin/kc.sh`, un `sh -c` qui lit `/run/secrets/db_password`, `ssh-keyscan`, `echo 'reset your password'`, `keydb-server`, `--cert=/etc/ssl/private/x.pem`, `passwd root`, et `State.Error` dès qu'il contenait « key ». Un mot de §7 n'est plus suspect que dans son propre mot (sans blanc ni guillemet) : s'il s'agit d'un drapeau, ou si le mot contient `=` ou `:` après lui ; dans un chemin (`/` avant lui) ou un mot ordinaire, il reste lisible. Ces cas sont écrits d'abord, en contrôles de `TestArgs` et de `TestLine`. → Tâche 4 ; Q27.
- **R12 (Codex) :** le test de fuite de `Line` acceptait un masque n'importe où après le mot : un masque d'URL plus loin couvrait une valeur restée visible. Le lien est désormais **local** : la valeur de ce mot-là doit être masquée dans son propre mot (masque en fin de mot, suivi au plus de ponctuation), ou être le mot suivant d'un drapeau. Cas gardés : `echo --password=$SECRET postgres://u:p@db` (valeur masquée) et `echo --password=$(cat /f) postgres://u:p@db` (masqué en entier : le masque est suivi de `(cat`). Testés avec les contrôles de R11, dans le même `TestArgs`. → Tâche 4 ; Q27.
- **R13 (Opus I2) :** `REDIS_ARGS=--requirepass x`, la forme documentée de redis-stack, sortait en clair. `Masked = SecretName(name) || maskText(value) != value` : la valeur suit les trois règles de la commande (URL, affectation, drapeau), sans le repli fermé. Témoins visibles : `JAVA_OPTS=-Xmx512m -Dfile.encoding=UTF-8`, `NODE_OPTIONS=--max-old-space-size=4096`, `PATH=/usr/local/keycloak/bin:/usr/bin`, `PWD=/srv/app`. → Tâches 4, 9 ; Q36.
- **R14 (Opus Minor 1) :** `postgres -c ssl_passphrase_command=echo X` fuyait : tout argument après `-c` était lu comme du shell. Une ligne de shell est désormais l'argument qui suit le `-c` (ou `-lc`, `-ec`…) d'un shell (`sh`, `bash`, `ash`, `dash`, `zsh`, `ksh`) ; ailleurs, `nom=valeur` masque tout le reste. Cas `postgres` et `bash -lc` dans `TestArgs`. → Tâche 4.
- **R15 (Opus Minor 2 et 5, Codex Minor) :** après `--nom=` vide, l'argument suivant est toujours masqué, même s'il commence par `-` ; la frontière gauche d'une affectation est tout ce qui ne peut pas faire partie d'un nom (`:`, `/`, `{`, `[` compris) ; `Pwd=` (ODBC, .NET) est une clé secrète dans une affectation, jamais comme nom de variable (`PWD` est le dossier courant) ; `\=` compte comme `=` (`-DDB_PASSWORD\=x`) ; une valeur d'environnement est jugée aussi décodée de ses `%XX` (`?pass%77ord=x`). Limites écrites en §13 : `--nom -valeur`, `$(…)` suivi de texte, `password = …` avec des blancs, une variable nommée `DB_PWD`. → Tâches 4, 9.
- **R16 (Opus Minor 3 et 7) :** le téléchargement des logs pose aussi un délai d'écriture de 30 s (`timedWriter`, sous le `bufio`) : il n'est jamais évincé, un client parti sans FIN ne doit pas garder son créneau ; `TestLogsWriteDeadline` le couvre. La décision de fin de flux est extraite de `LogsView` en fonction pure, `streamEnd` (`end`, `failure`, `unauthorized` → connexion, `evicted` → pas de reconnexion), testée par Vitest. → Tâches 3, 7.
- **R17 (Opus Minor 4 et 6), non retenus :** le délai d'écriture reste une constante, et aucun test ne prouve qu'un client qui ne lit plus est coupé : il faudrait remplir le tampon d'envoi du noyau (taille variable selon le système), ce qui donne un test lent et instable, pour vérifier `SetWriteDeadline` de la bibliothèque standard ; le test garde la preuve que le délai est posé. `$(…)` suivi de texte n'est pas pris dans la valeur (rare, écrit en §13).
- **Arbitrage du contrôleur (Codex, éviction entre sessions) :** l'éviction de flux entre sessions (Codex) n'est pas retenue. La v0.1 n'a qu'un utilisateur (spec §2), avec une session exigée et un cookie `SameSite=Strict`, et un onglet évincé ne se reconnecte pas. Opus juge aussi qu'elle n'est pas exploitable. Une ligne en §13 (tâche 9) : à revoir si l'agent devient multi-utilisateur (v0.2).

## Ce que les plans précédents ont préparé

- `ContainerInfo.Tmpfs` (plan 3, Q21), `LocateMounts` et `CheckVisibleMounts` (revue du plan 1), `CheckWriteRoot` (revue du plan 3) et `files.writable` : la tâche 1 les étend sans les remplacer.
- `index.resolve` (plan 2) résout un chemin dans l'index d'un conteneur arrêté, liens suivis, jamais hors de la racine : la tâche 1 s'en sert pour un tmpfs d'un conteneur arrêté.
- `fsErrorStatus` / `writeFSError` (codes de §6), `containerRef` (identifiant sûr dans une URL de Docker), `fsTimeout` (10 s pour l'`Inspect`), `originChecker` (toute méthode autre que GET/HEAD, donc `POST /env/reveal`), `requireSession`.
- `audit.Event{Action, Container, Path, Result, Error, Remote}` et `Audit.Write` : `env.reveal` est déjà une action de §7.
- Interface : `api()` et `ApiError`, `getMeta`, `errorMessage`, `stateLabel`, les onglets par `?tab=`, le bouton « Recharger », `fr.ts` et `en.ts` aux mêmes clés (test).

## Décisions de ce plan (la spec ne tranche pas)

- **Lecteur de logs dans `dockerx`.** Le format des trames est celui de Docker : il vit à côté du client. `ReadLogs(r, tty, max, emit)` plutôt que `stdcopy` : il faut la ligne, son flux et son horodatage, recoller les parties d'une ligne longue et plafonner la mémoire. Le plafond est celui de l'appelant (16 Kio pour la vue, 1 Mio pour le téléchargement, Q32). → Tâche 2.
- **Docker toujours interrogé avec horodatage**, stdout et stderr ; la vue demande `Follow` (le flux d'un conteneur arrêté se termine de lui-même, mesuré), le téléchargement non. → Tâches 2, 3.
- **Événements SSE.** Une ligne : événement sans nom, `id: <horodatage RFC 3339 nano>/<rang>`, `data: {"ts","stream","line","truncated"?}`. Fin du flux de Docker : `event: end`. Lignes au-delà du débit : `event: dropped`, `{"count"}`. Erreur : `event: failure`, `{"error","detail"}` (`unauthorized` quand la session a pris fin, R3). Flux évincé par un plus récent : `event: evicted` (R4). Battement `: ping` toutes les 15 s, qui vérifie aussi la session. En-têtes `Content-Type: text/event-stream`, `Cache-Control: no-store` (déjà posé sur `/api/`), `X-Accel-Buffering: no` (nginx). → Tâche 3 ; Q29, Q30.
- **Pas de journal pour les logs.** §7 ne liste que `login.fail`, `env.reveal`, `file.write`, `file.upload`. Les logs ne passent pas par la garde des fichiers (`fsGuard`, qui vise un FUSE muet) mais par leurs propres créneaux (Q33). → Tâche 3.
- **Téléchargement.** `<ts> <stream> <ligne>` par ligne, tout ce que Docker garde, `application/octet-stream`, `attachment; filename="<nom>-<AAAAMMJJ-HHMMSS>.log"` (nom réduit à `[A-Za-z0-9_.-]`). Une erreur avant le premier octet est une erreur d'API ; après, le fichier s'arrête, et une ligne `slog` le note dans le journal de l'agent, pas dans l'audit (R7 : §7 ne trace pas les lectures). → Tâche 3.
- **Arrêt de l'agent.** Un flux SSE ne finit jamais seul : `srv.Shutdown` attendait 5 s puis échouait (mesuré : 6 s, code 1). `BaseContext` + `RegisterOnShutdown` annulent le contexte des requêtes à l'arrêt (mesuré : 1 s, code 0). Une écriture en cours n'en souffre pas : `PutArchive` a déjà son contexte détaché (plan 3, Q17). → Tâche 3.
- **Radiographie en un seul `Inspect`.** `ContainerInfo.Details` porte ce que F7 montre ; `Env`, `Cmd` et `Entrypoint` y sont bruts et ne sortent de l'agent que par `secrets`. Ports : les liaisons réelles d'un conteneur actif (0.0.0.0 et :: une seule fois, comme F1), sinon celles de la configuration, plus les ports exposés sans liaison. Réseaux triés par nom. **Ni les labels ni le journal du healthcheck** : F7 ne les demande pas et ils portent souvent des secrets (seul l'état de santé est montré). → Tâche 5.
- **Les logs ne sont pas masqués.** §7 masque l'environnement ; un secret écrit par l'application dans ses logs s'affiche comme avec `docker logs` (ligne §13, tâche 9).
- **En-tête de la page par `GET /api/containers/{id}`.** La page ne cherche plus le conteneur dans la liste : un seul appel, par identifiant, et l'onglet Radiographie réutilise la réponse. → Tâche 8.
- **Interface des logs.** `EventSource` (reconnexion native, §3) ; les lignes reçues sont ajoutées une fois par image (`requestAnimationFrame`) ; 5 000 lignes au plus dans la vue (`MAX_LINES`), les plus anciennes sortent et sont comptées ; recherche et filtres stdout/stderr côté navigateur ; horodatage en heure locale à la milliseconde, masquable ; la vue suit la fin sauf si l'utilisateur remonte ; quitter l'onglet ferme le flux. → Tâche 7.

## Questions ouvertes

Le plan suit la recommandation de chaque question ; une réponse différente change la tâche indiquée.

- **Q24 — Lier une ligne tmpfs à un `--tmpfs` déclaré.** §13 donne comme parade le refus de toute écriture (échec fermé) ; §5.4 (b) lie un bind ou un volume par sa source, mais un tmpfs n'en a pas : un lien posé dans le conteneur pourrait mener sa destination vers un autre tmpfs (leurre).
  - *Recommandation (retenue en attendant la confirmation de l'utilisateur) :* une destination tmpfs absente de la table est retrouvée sur une ligne de type `tmpfs` **entière** (racine du montage `/`), dont le point P n'est **ni `/`** (prison : PID 1 chrooté dans le tmpfs, mesuré avec un vrai chroot), **ni déjà pris** par une autre destination (leurre : un autre tmpfs déclaré, un bind d'un tmpfs de l'hôte), **ni sur ou sous `/dev`, `/proc`, `/sys`** (leurre : `/dev`, `/dev/shm` du runtime), et que P résolu donne le même montage, périphérique et inode. Toute écriture **sous P** est refusée (`forbidden_path`), sinon elle réussirait sans que le conteneur la voie (mesuré au plan 3). Reste un leurre monté par le conteneur lui-même (`CAP_SYS_ADMIN`) : conséquence possible, une écriture que le conteneur ne voit pas, jamais une évasion (ligne §13). Autre option : garder le refus de toute écriture, et un compose `tmpfs: [/var/run]` (courant) n'est jamais inscriptible. → Tâches 1, 9.
- **Q25 — Conteneur arrêté : tmpfs sous un lien.** `writable` compare les destinations au texte : sur un conteneur arrêté dont le tmpfs est `/var/run` (→ `../run`), un envoi dans `/run` passait, écrit dans le rootfs puis masqué au prochain démarrage.
  - *Recommandation (retenue) :* `tarFS.CheckWriteRoot` résout chaque destination tmpfs dans l'index (liens suivis, jamais hors de la racine, comme runc au démarrage) et `files` refuse d'écrire dessous ; une destination qui cesse d'exister en chemin est résolue aussi loin qu'elle va, le reste recollé (`index.walk`, R8 : `/dangling → ../real` puis `x` donne `/real/x`). Coût nul : l'index est déjà construit pour toute écriture sur un conteneur arrêté. → Tâche 1.
- **Q26 — URL avec identifiants.** §7 dit `://user:pass@`. Formes courantes sans `:` ou sans utilisateur : `https://<jeton>@github.com`, `redis://:motdepasse@cache`.
  - *Recommandation (retenue) :* masquer toute valeur où `://` est suivi, avant tout blanc, d'un `@` (règle gourmande, jusqu'au dernier `@` du mot). Faux positifs acceptés (`https://example.com/a@b`) : la valeur se révèle d'un clic. → Tâche 4.
- **Q27 — Secrets dans la commande et le point d'entrée.** §7 ne masque que l'environnement, mais F7 montre la commande, et `redis-server --requirepass …`, `mysqld --password=…`, `sh -c "PGPASSWORD=… psql postgres://u:p@h"` sont courants.
  - *Recommandation (retenue, revues 1 et 2 : R1, R11, R12, R14, R15) :* masquer partie par partie, en remplaçant par `••••••` : les identifiants d'une URL ; la valeur de `NOM=valeur`, `--nom=valeur` ou `NOM\=valeur`, guillemets compris, et, hors d'une ligne de shell (l'argument qui suit le `-c` d'un shell), tout le reste de l'argument ; une affectation cachée dans la valeur d'une autre (`--env=DB_PASSWORD=x`) ; l'argument qui suit `--nom=` vide, ou le mot ou l'argument qui suit `--nom` s'il n'est pas lui-même un drapeau ; `nom` selon la règle des noms de §7, ou `Pwd` dans une affectation. Puis **échec fermé, local** : un mot de §7 dont la valeur, dans son propre mot, n'est pas masquée (le mot est un drapeau, ou contient `=` ou `:` après lui) masque l'argument entier ; un mot de §7 dans un chemin ou un mot ordinaire reste lisible (`/opt/keycloak/bin/kc.sh`, `ssh-keyscan`, `passwd root`). `commandMasked: true` le dit à l'interface. Le message d'erreur de l'état suit la même règle (`secrets.Line`). Pas de révélation de la commande en v0.1 (`docker inspect` reste là) ; limites écrites en §13 (`mysql -pX`, un secret dans un script). Autres options : montrer la commande telle quelle (lettre de §7), ou la masquer en entier dès qu'un mot suspect y paraît (illisible). → Tâches 4, 5, 8.
- **Q28 — Révélation : ce qui est journalisé et permis.** §6 dit « journalisé » sans plus.
  - *Recommandation (retenue) :* ligne `{action: "env.reveal", container: <ID complet>, path: <nom>, result: "ok"}` écrite **avant** l'envoi ; la valeur n'est jamais journalisée ; un journal qui ne s'écrit pas → `500 internal`, rien n'est révélé (même règle que l'intention d'une écriture). Les refus (Origin, corps invalide, nom inconnu, casse différente) ne sont pas journalisés : rien n'a été révélé. Toute variable peut être demandée, masquée ou non. Permis en `--read-only` : c'est une lecture (§7 applique la lecture seule dans `files` seulement). → Tâches 5, 8, 9.
- **Q29 — Erreurs d'un flux SSE.** `EventSource` ne lit ni le statut ni le corps d'une réponse autre que 200 : un `404 not_found` ou `503 docker_unavailable` y serait muet.
  - *Recommandation (retenue) :* une fois la session admise, toujours `200 text/event-stream` ; toute erreur arrive dans le flux (`event: failure`, code de §6) et la fin du flux de Docker par `event: end` ; l'interface ferme alors l'`EventSource`, sinon il se reconnecterait sans fin. Un `401` (session expirée, avant le flux) : l'interface interroge `/api/meta` et renvoie à la connexion. Autre option : `fetch` + `ReadableStream`, qui lit le statut mais réécrit la reconnexion, contre §3 (« reconnexion native »). → Tâches 3, 7.
- **Q30 — Reprise après coupure, et pause.** §3 promet la reconnexion native, sans dire d'où elle reprend.
  - *Recommandation (retenue) :* l'ID d'une ligne est `<horodatage>/<rang>`, le rang comptant les lignes de même horodatage déjà lues. `since` de Docker est inclusif (mesuré) : la reprise demande `since=<horodatage>`, `tail=all`, et saute les `rang + 1` premières lignes de cet horodatage. `Last-Event-ID` (le navigateur) prime sur `?after=` (la page). La pause ferme le flux (aucune ressource gardée, aucun tampon sans borne dans le navigateur) ; reprendre rouvre avec `after` : les lignes manquées arrivent (sous le plafond de Q31). Un ID mal formé est ignoré (départ ordinaire) ; une ligne sans horodatage n'a pas d'ID. Limites écrites (R6, Opus Minor 5) : une ligne écartée par le limiteur revient à la reprise (elle n'avait été qu'annoncée : l'interface la compte deux fois) ; si `tail` coupe un groupe de lignes de même horodatage, la reprise peut en rejouer quelques-unes (mesuré : 2 004 lignes écrites d'un coup ont 2 004 horodatages distincts, donc rare). → Tâches 3, 7.
- **Q31 — Débit.** Un conteneur bavard peut écrire des dizaines de milliers de lignes par seconde ; le navigateur n'en affiche utilement que quelques milliers.
  - *Recommandation (retenue) :* par flux, un seau de 10 000 lignes (la queue initiale, 10 000 au plus, y tient), rempli de 1 000 lignes par seconde. Les lignes au-delà ne sont pas envoyées mais **comptées** et annoncées (`dropped`) avant la ligne suivante, à chaque battement et à la fin ; l'interface dit combien, et renvoie au téléchargement, qui a tout. → Tâches 3, 7.
- **Q32 — Taille d'une ligne.** Docker coupe une ligne de plus de 16 Kio en parties de même horodatage (mesuré : 16 384 + 16 384 + 7 232 octets pour 40 000).
  - *Recommandation (retenue) :* les parties sont recollées par flux ; vue en direct : 16 Kio (`truncated`, marqué « … » dans l'interface) ; téléchargement : 1 Mio, puis `[…]`. Une trame n'est jamais lue entière : un en-tête qui annonce 1 Gio n'alloue rien (test). → Tâches 2, 3, 7.
- **Q33 — Flux simultanés, départ du client.** Rien dans la spec.
  - *Recommandation (retenue, revue 1 : R4) :* 8 flux au plus pour l'agent, téléchargements compris (un utilisateur, quelques onglets). Quand ils sont tous pris, le flux SSE **le plus ancien cède** sa place (`evicted`) : un client parti sans FIN garde sa connexion environ 15 min (retransmissions TCP), et son `EventSource` se reconnecte ailleurs ; le flux le plus récent est presque toujours le vivant. Un téléchargement n'est jamais évincé ; s'il ne reste que des téléchargements, `internal` (503, ou `failure` dans le flux) qui dit pourquoi. Le créneau n'est rendu qu'une fois le flux de Docker fermé et la goroutine de lecture finie. Chaque écriture a un délai de 30 s. Battement `: ping` toutes les 15 s (mandataires) ; il vérifie aussi la session (Q37). L'interface ferme le flux en quittant l'onglet : en HTTP/1.1, un navigateur n'ouvre que 6 connexions par hôte. → Tâches 3, 7.
- **Q34 — Pilote de logs illisible** (`--log-driver none`, `syslog`, `gelf`…). Docker répond `501` « configured logging driver does not support reading » (mesuré) ; `wrap` en ferait `docker_unavailable` (« Docker ne répond pas » : faux).
  - *Recommandation (retenue, approuvée par Opus en revue 1 ; la décision revient à l'utilisateur, c'est une modification de §6) :* nouveau code `logs_unavailable` (422), traduit, ajouté à §6 par la tâche 9. `docker_unavailable` serait faux, `not_found` tromperait (le conteneur existe). Autre option : `not_found` avec un détail, moins clair à l'écran. → Tâches 2, 3, 7, 9.
- **Q35 — Parcours Playwright.** §10 fixe un seul parcours (connexion → liste → fichier → diff) et ne demande rien pour F6 et F7.
  - *Recommandation (retenue) :* ne pas l'étendre ; F6 et F7 sont couverts de bout en bout côté API par la tâche 6 (§10 : « F1 à F7 de bout en bout côté API ») et l'interface par un parcours jetable, non commité (tâche 8). Si l'utilisateur le veut : trois étapes de plus dans le parcours unique (onglet Logs du conteneur arrêté, onglet Radiographie avec une variable masquée), et `scripts/e2e.sh` crée le conteneur avec un secret. → Tâche 8.
- **Q36 — Secret dans la valeur d'une variable au nom neutre** (revue 1, I2). §7 juge le nom et les URL ; une chaîne de connexion .NET (`ConnectionStrings__Default=Server=db;Password=…;`), une URL JDBC (`…?user=u&password=…`), `JAVA_OPTS=-D…Password=…` passaient en clair.
  - *Recommandation (retenue, revue 2 : R13, R15) :* masquer aussi la variable dont la valeur, telle quelle ou décodée de ses `%XX`, contient quelque chose que les trois règles de la commande masqueraient (`Masked = SecretName(name) || maskText(value) != value`) : URL avec identifiants, affectation secrète (`…;Password=…;`, `…;Pwd=…;`, `?password=…`, `-D…Password\=…`), drapeau secret suivi d'une valeur (`REDIS_ARGS=--requirepass …`, forme documentée de redis-stack). Sans le repli fermé : une valeur au nom neutre qui contient seulement un mot de §7 reste visible. Témoins testés : `JAVA_OPTS=-Xmx512m -Dfile.encoding=UTF-8`, `NODE_OPTIONS=--max-old-space-size=4096`, `PATH=/usr/local/keycloak/bin:/usr/bin`, `PWD=/srv/app`. Restent en clair (§13) : JSON (`"token":"…"`), `user:motdepasse@hôte` sans `://`, `password = …` avec des blancs, une variable nommée `DB_PWD`. Autre option : la lettre de §7, et ces valeurs sortent. → Tâches 4, 5, 9.
- **Q37 — Fin d'un flux SSE** (revue 1, I3, I4). Un flux ne finissait qu'avec le client ou le conteneur : il survivait à la déconnexion et à l'expiration de la session, et un client parti sans FIN gardait son créneau environ 15 min.
  - *Recommandation (retenue) :* trois fins de plus. (1) Session : vérifiée à chaque battement (15 s), `failure` `unauthorized` puis fermeture, l'interface renvoie à la connexion. (2) Éviction : quand les 8 créneaux sont pris, le flux le plus ancien reçoit `evicted` et ferme ; l'interface ne se reconnecte pas et propose « Recharger » (un neuvième onglet ouvert ferme donc le plus ancien, et le dit). (3) Délai d'écriture de 30 s : un client qui ne lit plus ne bloque pas la goroutine. Autres options : vérifier la session à chaque ligne (coût sur un flux bavard, pour gagner au plus 15 s), ou refuser le nouveau flux au lieu d'évincer (les créneaux zombies bloquent alors l'utilisateur légitime). → Tâches 3, 7, 9.
- **Q38 — Le tmpfs retrouvé vaut-il pour le tar d'un dossier ?** (revue 1, I5). `LocateMounts` sert à l'écriture (`CheckWriteRoot`) et au tar (`CheckArchive`, §5.4 b) ; un `--mount type=tmpfs` (dans `Inspect.Mounts`) sous un lien faisait refuser tout tar du conteneur.
  - *Recommandation (retenue) :* oui, mêmes gardes que pour l'écriture (ligne `tmpfs` entière, P ni `/`, ni pris, ni sous `/dev`, `/proc`, `/sys`) : le contrôle du tar reste fermé pour une prison (`TestCheckArchiveTmpfsJail`) et cesse de refuser un compose ordinaire (`TestIntegrationCheckArchiveTmpfsUnderLink`) ; §5.4 (b) le dit. Autre option : un drapeau de `LocateMounts` qui laisse le tar sans tmpfs (refus de tout tar de ces conteneurs, comme avant). → Tâches 1, 9.

## Global Constraints

- **Module et dépendances Go** : module `github.com/levy-cmd/hublot-kit` ; dépendances autorisées (§3) : SDK Docker `github.com/moby/moby/client` (et `github.com/moby/moby/api` pour les types), `github.com/cyphar/filepath-securejoin` **v0.7.0** (`pathrs-lite`), `golang.org/x/sys` **v0.48.0**. Aucune autre. Interface : aucune dépendance nouvelle.
- **Accès aux fichiers d'un conteneur (CLAUDE.md, §5).** La tâche 1 ne lit le conteneur que par `pathrs` (`mountIdent`), `openat2` et l'index ; **jamais** `filepath.Join`, `path.Join` ni `os.Open` sur un chemin du conteneur. Le reste du plan ne touche pas aux fichiers.
- **Tests d'abord (CLAUDE.md)** : à chaque tâche, l'étape 1 écrit les tests, l'étape 2 les voit échouer, l'étape 3 écrit le code. Tests d'évasion (tâche 1) et de masquage (tâches 4 et 5) avant le code qu'ils protègent ; un piège a son contrôle (un cas qui doit passer) ; un test ne se saute que sur une condition exacte, nommée dans le message.
- **Aucun secret ne quitte l'agent non masqué par défaut** : ni dans une réponse (tests `noSecret`, « FUITE »), ni dans le journal d'audit (le nom, jamais la valeur), ni dans les journaux de l'agent (`slog` ne reçoit jamais une valeur d'environnement).
- **Code Linux** testé sous Linux (`scripts/test-linux.sh`, VM) ; `go test ./cmd/... ./internal/...` reste vert sur macOS. **Jamais** de conteneur qui monte le `/proc`, le `/sys` ou le `/` de l'hôte suivi d'un tar.
- **Erreurs API** : `{"error": "<code>", "detail": "<texte>"}`, codes de §6, plus `logs_unavailable` (Q34, ajouté à §6 par la tâche 9).
- **Interface** : `{@html}` interdit ; contenu du conteneur (lignes de logs, variables, commande) toujours échappé par Svelte ; bouton « Recharger » sur les états d'erreur ; grilles en `minmax(0,1fr)` ; textes dans `fr.ts` et `en.ts` (mêmes clés) ; imports `#lib/….ts` dans les `.svelte` ; consultation au téléphone (§9 : 390 px sans défilement horizontal).
- **Node ≥ 22.17** : sur le Mac, `export PATH=/opt/homebrew/bin:$PATH` avant `npm`/`npx`. **Docker Desktop** : `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock`. Disque presque plein : aucune image lourde ; les tests n'utilisent que `alpine:3`.
- **Commits** en français, terminés par une ligne vide puis `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Sorties filtrées** : chaque commande de vérification se termine par `| tail -n 20` (ou `-run` ciblé) ; un test qui imprime une ligne de logs longue la résume (`texts`).

## Review Focus

1. **L'utilisateur ferme l'onglet, se déconnecte, le réseau coupe sans prévenir, ou l'agent s'arrête pendant un flux** : le flux de Docker est fermé, le créneau rendu ou repris par le flux le plus récent, aucune goroutine ne reste ; l'agent s'arrête aussitôt (code 0). → Tâche 3 `TestLogsClientGone`, `TestStreamSlots`, `TestLogsEvictsOldest`, `TestLogsSessionEnds`, `TestLogsWriteDeadline` ; tâche 8, script jetable (arrêt mesuré : 1 s, code 0).
2. **Conteneur très bavard ou ligne énorme** (JSON de 40 Kio, 10 000 lignes par seconde, en-tête de trame mensonger) : rien n'est perdu sans le dire, la mémoire reste bornée. → Tâche 2 `TestReadLogsPartialMessages`, `TestReadLogsErrors` (1 Gio annoncé), `TestReadLogsCap` ; tâche 3 `TestLogsRateLimit`, `TestLogsLongLine`.
3. **Coupure puis reprise, ou pause puis reprise** : ni doublon ni trou, même quand plusieurs lignes partagent un horodatage. → Tâche 3 `TestLogsResume`, `TestLogsRateLimit` (rangs) ; tâche 6 (reprise sur un vrai Docker) ; tâche 8, parcours jetable (numéros de `tick` consécutifs).
4. **Secret là où on ne l'attend pas** : jeton seul dans une URL, `sh -c` avec mot de passe entre guillemets, `--requirepass`, chaîne de connexion sous un nom neutre, message d'erreur de Docker, journal d'audit, réponse d'un refus. → Tâche 4 `TestURLCredentials`, `TestArgs`, `TestEnvNeverLeaks` ; tâche 5 `TestContainerRadiography`, `TestRevealEnv`, `TestRevealEnvRefusals`.
5. **Compose `tmpfs: [/var/run]` sur une image où `/var/run` → `../run`** : le reste du conteneur s'écrit, `/run` est refusé, actif comme arrêté, et aucun leurre ne déplace le refus. → Tâche 1 `TestLocateMountsTmpfs`, `TestTarCheckWriteRootTmpfs`, `TestWriteRefusedUnderResolvedTmpfs`, `TestIntegrationWriteTmpfsUnderLink`, `TestIntegrationCheckWriteRootTmpfsUnderLink`.

## Ordre : chaque test d'évasion précède le code qu'il protège

| Protection (spec) | Tests écrits | Code protégé |
|---|---|---|
| Tmpfs sous un lien retrouvé sans prison ni leurre, écriture refusée sous le point retrouvé, actif et arrêté ; tar d'un dossier toujours fermé à la prison (§5.3 ét. 2, §5.4 b, §13) | Tâche 1, étape 1 | Tâche 1, étape 3 |
| Lecteur de logs : trame jamais lue entière, lignes recollées, taille plafonnée | Tâche 2, étape 1 | Tâche 2, étape 3 |
| Flux : session (à l'ouverture et à chaque battement), client parti → flux de Docker fermé, créneau rendu ou repris par le plus récent, délai d'écriture, débit, erreurs dans le flux | Tâche 3, étape 1 | Tâche 3, étape 3 |
| Masquage de §7 : noms, URL, commande ; rien ne fuit | Tâche 4, étape 1 | Tâche 4, étape 3 |
| Radiographie sans secret ; révélation : Origin, journal avant l'envoi, valeur jamais journalisée | Tâche 5, étape 1 | Tâche 5, étape 3 |
| Interface : lignes, filtres, URL, masques affichés tels que l'agent les donne | Tâches 7 et 8, étape 1 | Tâches 7 et 8, étape 3 |

**Ne pas pousser la branche entre l'étape 1 et l'étape 3 d'une tâche** : le paquet testé n'y compile pas.

## Où tester

| Repère | Commande (racine du dépôt) |
|---|---|
| **macOS** | `go test ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; interface : `cd web && export PATH=/opt/homebrew/bin:$PATH && npx vitest run 2>&1 \| tail -n 15 && npm run check 2>&1 \| tail -n 5 && npm run lint 2>&1 \| tail -n 5` |
| **Docker Desktop (portable : logs, Inspect)** | `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ ./internal/api/ -run 'Logs\|Observation' 2>&1 \| tail -n 20` |
| **Linux root** | `scripts/test-linux.sh -race ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; avec Docker : ajouter `-tags integration` |
| **VM Ubuntu / Fedora** | `limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration ./cmd/... ./internal/... 2>&1 \| tail -n 30` (idem `hk-fedora`) |
| **Parcours Playwright** | `export PATH=/opt/homebrew/bin:$PATH && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock scripts/e2e.sh 2>&1 \| tail -n 20` |

## Structure des fichiers

```
internal/fsview/mounts.go                    LocateMounts : tmpfs lié par sa ligne (sameMount, runtimeMount), withTmpfs, tmpfsPoints
internal/fsview/fsview.go                    WriteRootChecker.CheckWriteRoot rend les points de tmpfs
internal/fsview/procfs_archive_linux.go      procFS.CheckWriteRoot : points retrouvés par identifiant de montage
internal/fsview/tarindex.go, views.go        newTarFS(docker, info, …) ; index.walk ; tarFS.CheckWriteRoot : points résolus dans l'index
internal/files/files.go                      checkRoot(ctx, fsys, target), underTmpfs
internal/dockerx/logs.go (+ test)            LogsOptions, LogLine, Logs, ReadLogs, ErrNoLogs, MaxLogLine
internal/dockerx/dockerx.go                  Client.Logs ; ContainerInfo.Tty, LogDriver, Details ; Details, Network ; Mount.Mode, RW
internal/dockerx/fake.go                     LogStreams, LogsErr, LogsGate, LogsCloses, LogsCalls()
internal/api/logs.go (+ test)                GET /logs (SSE) et /logs/download, logLimits, streamSlots (éviction), sessionValid, inspectByRef
internal/api/container.go (+ test)           GET /api/containers/{id}, POST /env/reveal
internal/api/server.go, fserr.go             routes, créneaux des flux, logs_unavailable
internal/api/integration_observation_test.go F6 et F7 sur un vrai Docker (portable)
internal/secrets/secrets.go (+ test)         SecretName, URLCredentials, Masked, Env, Lookup, Line (fuite jugée localement), Args, Mask
cmd/hublot/main.go                           arrêt : contexte des requêtes annulé
web/src/lib/api.ts (+ test)                  logsUrl, logsDownloadUrl, LogLine ; Radiography, getContainer, revealEnv
web/src/lib/logs.ts (+ test)                 parseLine, parseCount, parseFailure, streamEnd, appendLines, filterLines, formatTs
web/src/lib/xray.ts (+ test)                 commandLine, portLabel, mountLabel, restartLabel, shortId, envValue
web/src/lib/i18n/fr.ts, en.ts, index.ts      clés logs.*, xray.*, error.logs_unavailable
web/src/lib/components/LogsView.svelte       onglet Logs (EventSource)
web/src/lib/components/XrayView.svelte       onglet Radiographie
web/src/routes/c/[id]/+page.svelte           onglets Logs et Radiographie, en-tête par GET /api/containers/{id}
docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md   §5.3, §5.4, §6, §7, §13 (tâche 9)
```

---

### Task 1: Correctif hérité — un `--tmpfs` sous un lien ne bloque plus l'écriture

**Où tester :** macOS (portables), `scripts/test-linux.sh` (Linux root, Docker).

**Files:**
- Modify: `internal/fsview/mounts.go`, `internal/fsview/fsview.go`, `internal/fsview/procfs_archive_linux.go`, `internal/fsview/tarindex.go`, `internal/fsview/views.go`, `internal/files/files.go`
- Test: `internal/fsview/mounts_test.go`, `internal/fsview/procfs_archive_linux_test.go`, `internal/fsview/tarindex_test.go`, `internal/fsview/integration_test.go`, `internal/files/files_test.go`, `internal/api/integration_write_linux_test.go`

**Interfaces:**
- Consumes: `LocateMounts`, `CheckVisibleMounts`, `scanMountinfo`, `mountLine`, `procFS.mountIdent`, `index.resolve`, `files.writable`, `files.under`.
- Produces: `fsview.WriteRootChecker` devient `CheckWriteRoot(ctx context.Context) (tmpfs []string, err error)`, implémenté par `procFS` (points retrouvés par identifiant de montage) et par `tarFS` (destinations résolues dans l'index aussi loin qu'elles existent) ; `(*index).walk(p string) (cur string, rest []string, err error)`, dont `resolve` est désormais le cas « tout existe » ; `newTarFS(docker dockerx.Client, info dockerx.ContainerInfo, idx *index, stale func()) *tarFS` ; `sameMount(m dockerx.Mount, l mountLine) bool` ; `runtimeMount(p string) bool`, `withTmpfs(mounts []dockerx.Mount, tmpfs []string) []dockerx.Mount`, `tmpfsPoints(ms []dockerx.Mount) []string` ; dans `files` : `checkRoot(ctx, fsys, target string) error` et `underTmpfs(target string, points []string) error`.

> **À comprendre :** compose `tmpfs: [/var/run]` sur une image où `/var/run` est un lien vers `../run` (alpine, Debian…) : runc monte le tmpfs là où le lien mène, sur `/run`, et la table des montages ne montre jamais `/var/run`. `CheckVisibleMounts` refusait donc **toute** écriture dans ce conteneur (échec fermé, §13). La tâche le retrouve comme `LocateMounts` retrouve déjà un bind ou un volume : par l'identifiant de montage de la destination résolue dans le conteneur (`pathrs`, jamais hors de la racine). Un tmpfs n'a pas de source qui le lie à une ligne : le lien est la forme de la ligne (type `tmpfs`, racine `/`), avec trois gardes contre les pièges (Q24) : P n'est pas `/` (prison), P n'est pas déjà pris (leurre : autre tmpfs, bind), P n'est pas sous `/dev`, `/proc`, `/sys` (leurre : montages du runtime). Retrouver le point ne suffit pas : Docker n'écrit jamais dans un tmpfs (mesuré au plan 3), donc `files` refuse d'écrire **sous** le point retrouvé, sinon l'écriture réussirait sans que le conteneur la voie. Pour un conteneur arrêté, l'index résout la destination comme runc le fera au démarrage, et une fin de chemin absente est recollée sous la partie résolue (Q25, R8). `LocateMounts` sert aussi au contrôle du tar d'un dossier (`CheckArchive`, §5.4 b) : un `--mount type=tmpfs` sous un lien n'y fait plus refuser tout tar, et la prison y reste refusée (Q38, R5).

- [ ] **Step 1: Tests d'évasion qui échouent** — prison, leurre, lien vers un autre montage ; et le défaut de §13 sur un vrai Docker :

```diff
diff --git a/internal/api/integration_write_linux_test.go b/internal/api/integration_write_linux_test.go
--- a/internal/api/integration_write_linux_test.go
+++ b/internal/api/integration_write_linux_test.go
@@ -111,3 +111,42 @@ func TestIntegrationWriteRunning(t *testing.T) {
 		t.Fatalf("fichier envoyé, lien pendant intact : %q", got)
 	}
 }
+
+// Spec §13: compose "tmpfs: [/var/run]" on alpine, where /var/run ->
+// ../run: the tmpfs sits at /run. Before the fix, every write to this
+// container was refused (the destination was missing from the table). Now
+// /etc is writable, and /run, where Docker would write unseen, is refused.
+func TestIntegrationWriteTmpfsUnderLink(t *testing.T) {
+	if os.Geteuid() != 0 {
+		t.Skip("root requis pour /proc/<pid>/root des conteneurs : scripts/test-linux.sh ou job CI privilégié")
+	}
+	name := "hublot-it-write-tmpfs-link"
+	exec.Command("docker", "rm", "-f", name).Run()
+	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
+	dockerCLI(t, "run", "-d", "--name", name, "--tmpfs", "/var/run", "alpine:3", "sleep", "300")
+	if got := dockerCLI(t, "exec", name, "readlink", "/var/run"); got != "../run" {
+		t.Skipf("précondition absente : /var/run -> %q dans alpine:3, attendu ../run", got)
+	}
+	dc, err := dockerx.New()
+	if err != nil {
+		t.Fatal(err)
+	}
+	f := &fixture{}
+	f.init(t, false, dc, nil)
+	c := f.session(t)
+	if rec := f.upload(t, c, name, "/etc", "tmpfs-ok.txt", []byte("ok\n"), "*"); rec.Code != http.StatusCreated {
+		t.Fatalf("envoi dans /etc : %d %s, attendu 201", rec.Code, rec.Body)
+	}
+	if got := dockerCLI(t, "exec", name, "cat", "/etc/tmpfs-ok.txt"); got != "ok" {
+		t.Fatalf("relu dans le conteneur : %q", got)
+	}
+	for _, dir := range []string{"/run", "/var/run"} {
+		rec := f.upload(t, c, name, dir, "x", []byte("x"), "*")
+		if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden_path" {
+			t.Errorf("envoi dans %s : %d %s, attendu 403 forbidden_path", dir, rec.Code, rec.Body)
+		}
+	}
+	if got := dockerCLI(t, "exec", name, "sh", "-c", "test ! -e /run/x && echo absent"); got != "absent" {
+		t.Errorf("fichier créé par un envoi refusé : %q", got)
+	}
+}
diff --git a/internal/files/files_test.go b/internal/files/files_test.go
--- a/internal/files/files_test.go
+++ b/internal/files/files_test.go
@@ -809,14 +809,15 @@ func TestEditRefusesMountPoint(t *testing.T) {
 	}
 }
 
-// rootView is MemFS plus the root check of a running container
-// (fsview.WriteRootChecker, implemented by procfs).
+// rootView is MemFS plus the root check of a view (fsview.WriteRootChecker,
+// implemented by procfs and tarindex): err, or the tmpfs mount points.
 type rootView struct {
 	*fsview.MemFS
-	err error
+	err    error
+	points []string
 }
 
-func (v rootView) CheckWriteRoot(context.Context) error { return v.err }
+func (v rootView) CheckWriteRoot(context.Context) ([]string, error) { return v.points, v.err }
 
 // Review I1: with a chrooted PID 1 the view shows another tree than the one
 // Docker writes in. The view says so: no edit, no upload, no audit line, no
@@ -839,3 +840,33 @@ func TestWriteRefusedWhenRootIsNotContainerRoot(t *testing.T) {
 		t.Fatalf("contrôle, racine du conteneur : %v", err)
 	}
 }
+
+// Spec §13: a --tmpfs under a link (/var/run -> ../run) is mounted at /run;
+// the view finds it there (CheckWriteRoot). Docker never writes into a
+// tmpfs: a write under /run would succeed where the container never sees
+// it. Refused before any intent; the rest of the container stays writable.
+func TestWriteRefusedUnderResolvedTmpfs(t *testing.T) {
+	r := newRig(t, false)
+	r.mem.AddDir("/run").AddFile("/run/app.pid", []byte("1\n"))
+	r.mem.Entries["/run/app.pid"] = fsview.Entry{Name: "app.pid", Type: fsview.TypeFile, Mode: 0o644}
+	view := rootView{MemFS: r.mem, points: []string{"/run"}}
+	r.w.Open = func(context.Context, string) (fsview.FS, error) { return view, nil }
+	c := ctr
+	c.Tmpfs = []string{"/var/run"}
+	if _, err := r.w.Edit(ctx, c, "/run/app.pid", []byte("2\n"), ETag([]byte("1\n"))); !errors.Is(err, fsview.ErrForbiddenPath) || !strings.Contains(err.Error(), "/run") {
+		t.Fatalf("édition sous le tmpfs résolu : %v, attendu ErrForbiddenPath nommant /run", err)
+	}
+	r.nothingWritten(t)
+	for _, dir := range []string{"/run", "/run/sub"} {
+		if _, err := r.w.Upload(ctx, c, dir, "x", []byte("x"), "*"); !errors.Is(err, fsview.ErrForbiddenPath) {
+			t.Fatalf("envoi dans %s : %v, attendu ErrForbiddenPath", dir, err)
+		}
+	}
+	r.nothingWritten(t)
+	if _, err := r.w.Upload(ctx, c, "/", "run2", []byte("x"), "*"); err != nil {
+		t.Fatalf("/run2 n'est pas sous /run : %v", err)
+	}
+	if _, err := r.w.Edit(ctx, c, "/etc/app.conf", []byte("x"), confTag); err != nil {
+		t.Fatalf("le reste du conteneur reste inscriptible : %v", err)
+	}
+}
diff --git a/internal/fsview/integration_test.go b/internal/fsview/integration_test.go
--- a/internal/fsview/integration_test.go
+++ b/internal/fsview/integration_test.go
@@ -15,6 +15,7 @@ import (
 	"os"
 	"os/exec"
 	"path"
+	"slices"
 	"strings"
 	"testing"
 	"time"
@@ -310,8 +311,8 @@ func TestIntegrationCheckWriteRoot(t *testing.T) {
 	if !ok {
 		t.Fatal("la vue procfs doit implémenter WriteRootChecker")
 	}
-	if err := w.CheckWriteRoot(ctx); err != nil {
-		t.Fatalf("conteneur ordinaire avec volume et tmpfs : %v", err)
+	if points, err := w.CheckWriteRoot(ctx); err != nil || !slices.Equal(points, []string{"/scratch"}) {
+		t.Fatalf("conteneur ordinaire avec volume et tmpfs : %v %v", points, err)
 	}
 	for name, args := range map[string][]string{
 		"volume": {"-v", "/jail"},
@@ -322,7 +323,7 @@ func TestIntegrationCheckWriteRoot(t *testing.T) {
 		var err error
 		for i := 0; i < 25; i++ { // PID 1 chroots a moment after the start
 			time.Sleep(200 * time.Millisecond)
-			err = openFor(t, id).(WriteRootChecker).CheckWriteRoot(ctx)
+			_, err = openFor(t, id).(WriteRootChecker).CheckWriteRoot(ctx)
 			if err != nil {
 				break
 			}
@@ -332,3 +333,46 @@ func TestIntegrationCheckWriteRoot(t *testing.T) {
 		}
 	}
 }
+
+// Spec §13: alpine has /var/run -> ../run; a --tmpfs /var/run (compose
+// "tmpfs: [/var/run]") is mounted at /run. CheckWriteRoot finds it by mount
+// ID and returns /run, where package files refuses to write. A PID 1
+// chrooted into that tmpfs, with a link var/run -> / planted in the jail,
+// leads the destination to the jail's root: refused.
+func TestIntegrationCheckWriteRootTmpfsUnderLink(t *testing.T) {
+	needRoot(t)
+	ctx := context.Background()
+	id := runTarget(t, "hublot-it-fs-tmpfs-link", "--tmpfs", "/var/run", "alpine:3", "sleep", "300")
+	if got := docker(t, "exec", id, "readlink", "/var/run"); got != "../run" {
+		t.Skipf("précondition absente : /var/run -> %q dans alpine:3, attendu ../run", got)
+	}
+	points, err := openFor(t, id).(WriteRootChecker).CheckWriteRoot(ctx)
+	if err != nil || !slices.Equal(points, []string{"/run"}) {
+		t.Fatalf("tmpfs sous un lien : %v %v, attendu [/run]", points, err)
+	}
+	jail := runTarget(t, "hublot-it-fs-tmpfs-jail", "--tmpfs", "/var/run:exec", "alpine:3", "sh", "-c",
+		"mkdir /run/lib /run/var && ln -s / /run/var/run && cp /lib/ld-musl-* /run/lib/ && cp /bin/busybox /run/ && exec chroot /run /busybox sleep 300")
+	for i := 0; i < 25; i++ { // PID 1 chroots a moment after the start
+		time.Sleep(200 * time.Millisecond)
+		if _, err = openFor(t, jail).(WriteRootChecker).CheckWriteRoot(ctx); err != nil {
+			break
+		}
+	}
+	if !errors.Is(err, ErrForbiddenPath) {
+		t.Errorf("ÉVASION : prison dans le tmpfs sous un lien acceptée (%v)", err)
+	}
+}
+
+// Review I5: a --mount type=tmpfs under a link is in Inspect.Mounts; the tar
+// of a directory (spec §5.4 b) locates it on /run like a write does, instead
+// of refusing every tar of the container.
+func TestIntegrationCheckArchiveTmpfsUnderLink(t *testing.T) {
+	needRoot(t)
+	id := runTarget(t, "hublot-it-fs-mount-tmpfs", "--mount", "type=tmpfs,dst=/var/run", "alpine:3", "sleep", "300")
+	if got := docker(t, "exec", id, "readlink", "/var/run"); got != "../run" {
+		t.Skipf("précondition absente : /var/run -> %q dans alpine:3, attendu ../run", got)
+	}
+	if err := openFor(t, id).CheckArchive(context.Background(), "/etc"); err != nil {
+		t.Fatalf("tar de /etc refusé à cause du tmpfs /var/run : %v", err)
+	}
+}
diff --git a/internal/fsview/mounts_test.go b/internal/fsview/mounts_test.go
--- a/internal/fsview/mounts_test.go
+++ b/internal/fsview/mounts_test.go
@@ -300,3 +300,86 @@ func TestLocateMountsChrootToRootMountEndingSource(t *testing.T) {
 		t.Errorf("ÉVASION : accepté (%v)", err)
 	}
 }
+
+// Spec §13: a --tmpfs whose destination passes through a link (compose
+// "tmpfs: [/var/run]", /var/run -> ../run) is mounted at /run, so the table
+// does not show /var/run. It is found by mount ID like a bind or a volume,
+// with its own tie: a whole tmpfs (fstype tmpfs, root "/"), never on "/",
+// never on a point another destination already holds, never on a mount
+// the runtime adds (/dev, /dev/shm, /proc, /sys).
+func TestLocateMountsTmpfs(t *testing.T) {
+	mi := strings.Join([]string{
+		"20 1 0:20 / / rw - overlay overlay rw",
+		"50 20 0:50 / /run rw - tmpfs tmpfs rw",
+		"51 20 0:51 / /scratch rw - tmpfs tmpfs rw",
+		"52 20 0:52 / /dev rw - tmpfs tmpfs rw",
+		"53 52 0:53 / /dev/shm rw - tmpfs shm rw",
+		"54 20 0:54 / /hostrun rw - tmpfs tmpfs rw",
+		"55 20 8:1 /var/lib/docker/volumes/v/_data /data rw - ext4 /dev/sda1 rw",
+		"56 20 0:56 /sub /part rw - tmpfs tmpfs rw",
+	}, "\n")
+	ids := map[string]MountIdent{
+		"/run": {50, 50, 1}, "/var/run": {50, 50, 1}, // the case of spec §13
+		"/scratch": {51, 51, 1}, "/to-scratch": {51, 51, 1}, // decoy: another declared tmpfs
+		"/dev": {52, 52, 1}, "/to-dev": {52, 52, 1}, // decoy: runtime mounts
+		"/dev/shm": {53, 53, 1}, "/to-shm": {53, 53, 1},
+		"/hostrun": {54, 54, 1}, "/to-hostrun": {54, 54, 1}, // a bind of a host tmpfs
+		"/data": {55, 30, 2}, "/to-data": {55, 30, 2}, // not a tmpfs
+		"/part": {56, 56, 1}, "/to-part": {56, 56, 1}, // a part of a tmpfs, not a whole one
+		"/": {20, 20, 2}, "/to-root": {20, 20, 2}, // jail: the link leads to the root
+		"/other-inode": {50, 50, 9}, // same mount, another inode
+	}
+	ident := func(p string) (MountIdent, error) {
+		if id, ok := ids[p]; ok {
+			return id, nil
+		}
+		return MountIdent{}, fs.ErrNotExist
+	}
+	tmpfs := func(dst string) dockerx.Mount { return dockerx.Mount{Type: "tmpfs", Destination: dst} }
+
+	got := LocateMounts(mi, []dockerx.Mount{tmpfs("/var/run"), tmpfs("/scratch")}, ident)
+	if got[0].Destination != "/run" || got[1].Destination != "/scratch" {
+		t.Fatalf("tmpfs sous un lien : %v, attendu /run", got)
+	}
+	if err := CheckVisibleMounts(mi, got); err != nil {
+		t.Fatalf("après localisation : %v", err)
+	}
+	// Fail closed: each of these stays where it is, and the write is refused.
+	refused := map[string][]dockerx.Mount{
+		"lien vers la racine (overlay)":       {tmpfs("/to-root")},
+		"leurre : un autre tmpfs déclaré":     {tmpfs("/to-scratch"), tmpfs("/scratch")},
+		"leurre : /dev du runtime":            {tmpfs("/to-dev")},
+		"leurre : /dev/shm du runtime":        {tmpfs("/to-shm")},
+		"lien vers un autre montage (bind)":   {tmpfs("/to-hostrun"), {Type: "bind", Source: "/run", Destination: "/hostrun"}},
+		"lien vers un volume":                 {tmpfs("/to-data"), {Type: "volume", Name: "v", Destination: "/data"}},
+		"partie d'un tmpfs":                   {tmpfs("/to-part")},
+		"autre inode":                         {tmpfs("/other-inode")},
+		"introuvable":                         {tmpfs("/absent")},
+		"deux destinations sur un même tmpfs": {tmpfs("/var/run"), tmpfs("/var/run2")},
+	}
+	ids["/var/run2"] = MountIdent{50, 50, 1}
+	for name, ms := range refused {
+		if err := CheckVisibleMounts(mi, LocateMounts(mi, ms, ident)); !errors.Is(err, ErrForbiddenPath) {
+			t.Errorf("ÉVASION : %s accepté", name)
+		}
+	}
+	// Jail: PID 1 chrooted into the tmpfs itself, with a link var/run -> /
+	// planted in it: the destination leads to the jail's root, a whole tmpfs
+	// on "/". Only the "P is not /" guard refuses it.
+	jail := "60 1 0:60 / / rw - tmpfs tmpfs rw"
+	jailIdent := func(p string) (MountIdent, error) {
+		if p == "/" || p == "/var/run" {
+			return MountIdent{60, 60, 1}, nil
+		}
+		return MountIdent{}, fs.ErrNotExist
+	}
+	if err := CheckVisibleMounts(jail, LocateMounts(jail, []dockerx.Mount{tmpfs("/var/run")}, jailIdent)); !errors.Is(err, ErrForbiddenPath) {
+		t.Errorf("ÉVASION : prison dans le tmpfs acceptée")
+	}
+	// The tie of a bind or a volume never accepts a tmpfs line, nor the
+	// reverse: a bind whose destination leads to /run stays refused.
+	bind := []dockerx.Mount{{Type: "bind", Source: "/srv/run", Destination: "/var/run"}}
+	if err := CheckVisibleMounts(mi, LocateMounts(mi, bind, ident)); !errors.Is(err, ErrForbiddenPath) {
+		t.Errorf("ÉVASION : un bind localisé sur un tmpfs")
+	}
+}
diff --git a/internal/fsview/procfs_archive_linux_test.go b/internal/fsview/procfs_archive_linux_test.go
--- a/internal/fsview/procfs_archive_linux_test.go
+++ b/internal/fsview/procfs_archive_linux_test.go
@@ -14,6 +14,7 @@ import (
 	"os/exec"
 	"path"
 	"path/filepath"
+	"slices"
 	"strings"
 	"testing"
 
@@ -55,6 +56,31 @@ func TestCheckArchiveMountTable(t *testing.T) {
 	}
 }
 
+// Review I5: the tar check (spec §5.4 b) locates a tmpfs of Inspect.Mounts
+// (--mount type=tmpfs) under a link too. A PID 1 chrooted into that tmpfs,
+// with a link planted to its root, leads the destination to "/": refused.
+// The table is faked (a whole tmpfs on "/", with the real root's mount ID);
+// the destination is a real link to "/" in the test machine.
+func TestCheckArchiveTmpfsJail(t *testing.T) {
+	p, _ := childView(t)
+	st, err := statxMountID(int(p.root.Fd()), 0, 0)
+	must(t, err)
+	link := filepath.Join(t.TempDir(), "var-run")
+	must(t, os.Symlink("/", link))
+	if _, err := p.mountIdent(link); err != nil {
+		t.Skipf("précondition absente : %s illisible dans la vue (%v) ; lancer par scripts/test-linux.sh", link, err)
+	}
+	old := readMountinfo
+	t.Cleanup(func() { readMountinfo = old })
+	readMountinfo = func(int) ([]byte, error) {
+		return []byte(fmt.Sprintf("%d 1 0:50 / / rw - tmpfs tmpfs rw\n", st.Mnt_id)), nil
+	}
+	p.mounts = []dockerx.Mount{{Type: "tmpfs", Destination: link}}
+	if err := p.CheckArchive(ctx, "/etc"); !errors.Is(err, ErrForbiddenPath) {
+		t.Fatalf("ÉVASION : tar accepté dans une prison tmpfs (%v)", err)
+	}
+}
+
 func TestCheckArchiveHostBind(t *testing.T) {
 	p, _ := childView(t)
 	p.mounts = []dockerx.Mount{{Type: "bind", Source: "/proc", Destination: "/host/proc"}}
@@ -180,7 +206,7 @@ func TestCheckArchiveDestinationUnderSymlink(t *testing.T) {
 // table is faked: a real chrooted child needs a binary inside the jail.
 func TestCheckWriteRoot(t *testing.T) {
 	p, _ := childView(t)
-	if err := p.CheckWriteRoot(ctx); err != nil {
+	if _, err := p.CheckWriteRoot(ctx); err != nil {
 		t.Fatalf("process non chrooté : %v", err)
 	}
 	old := readMountinfo
@@ -188,12 +214,12 @@ func TestCheckWriteRoot(t *testing.T) {
 	readMountinfo = func(int) ([]byte, error) {
 		return []byte("36 35 8:1 / /etc rw - ext4 /dev/sda1 rw\n"), nil
 	}
-	err := p.CheckWriteRoot(ctx)
+	_, err := p.CheckWriteRoot(ctx)
 	if !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "racine") {
 		t.Fatalf("racine déplacée : %v, attendu ErrForbiddenPath", err)
 	}
 	readMountinfo = func(int) ([]byte, error) { return nil, errors.New("illisible") }
-	if err := p.CheckWriteRoot(ctx); err == nil {
+	if _, err := p.CheckWriteRoot(ctx); err == nil {
 		t.Fatal("mountinfo illisible : doit échouer fermé")
 	}
 }
@@ -220,24 +246,24 @@ func TestCheckWriteRootJail(t *testing.T) {
 	p.tmpfs = []string{"/run"}
 
 	readMountinfo = table(st.Mnt_id, "/data", "/jail", "/run")
-	if err := p.CheckWriteRoot(ctx); err != nil {
-		t.Fatalf("conteneur ordinaire, tout est visible : %v", err)
+	if points, err := p.CheckWriteRoot(ctx); err != nil || !slices.Equal(points, []string{"/run"}) {
+		t.Fatalf("conteneur ordinaire, tout est visible : %v %v, attendu le tmpfs /run", points, err)
 	}
 	readMountinfo = table(st.Mnt_id, "/data", "/run") // chroot /jail in a volume
-	if err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "/jail") {
+	if _, err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "/jail") {
 		t.Fatalf("prison dans un volume : %v, attendu ErrForbiddenPath nommant /jail", err)
 	}
 	p.mounts = []dockerx.Mount{{Type: "volume", Destination: "/data"}}
 	p.tmpfs = []string{"/jail"}
 	readMountinfo = table(st.Mnt_id, "/data") // chroot /jail in a tmpfs
-	if err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "/jail") {
+	if _, err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "/jail") {
 		t.Fatalf("prison dans un tmpfs : %v, attendu ErrForbiddenPath nommant /jail", err)
 	}
 	// The root pinned when the view was opened is not the root of the table
 	// any more (chroot after the open).
 	p.tmpfs = nil
 	readMountinfo = table(st.Mnt_id+1000, "/data")
-	if err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "racine épinglée") {
+	if _, err := p.CheckWriteRoot(ctx); !errors.Is(err, ErrForbiddenPath) || !strings.Contains(err.Error(), "racine épinglée") {
 		t.Fatalf("racine épinglée différente : %v, attendu ErrForbiddenPath", err)
 	}
 }
diff --git a/internal/fsview/tarindex_test.go b/internal/fsview/tarindex_test.go
--- a/internal/fsview/tarindex_test.go
+++ b/internal/fsview/tarindex_test.go
@@ -132,7 +132,7 @@ func newTarView(t *testing.T, mounts []dockerx.Mount, hs ...hdr) tarView {
 	}
 	f := trapDocker(t)
 	stale := new(atomic.Int32)
-	return tarView{newTarFS(f, tarID, mounts, idx, func() { stale.Add(1) }), f, stale}
+	return tarView{newTarFS(f, dockerx.ContainerInfo{ID: tarID, Mounts: mounts}, idx, func() { stale.Add(1) }), f, stale}
 }
 
 func readTar(fsys FS, p string) ([]byte, error) {
@@ -587,3 +587,28 @@ func TestMeasureIndexMemory(t *testing.T) {
 		t.Fatalf("mesure implausible : %.0f octets par entrée", per)
 	}
 }
+
+// Spec §13, stopped container: runc mounts a --tmpfs at its destination
+// resolved in the root filesystem, links followed. CheckWriteRoot resolves
+// it the same way in the index (never out of the root), so package files
+// refuses /run for a "tmpfs: [/var/run]" with /var/run -> ../run. A path
+// that stops existing midway is resolved as far as it goes, the rest
+// appended (runc creates it there: /dangling -> ../real gives /real/x).
+func TestTarCheckWriteRootTmpfs(t *testing.T) {
+	idx, err := buildIndex(tctx, bytes.NewReader(mkTar(t,
+		tdir("/"), tdir("/run/"), tdir("/var/"), tlink("/var/run", "../run"),
+		tlink("/var/lock", "/../../run/lock"), tdir("/run/lock/"), tdir("/tmp/"),
+		tlink("/dangling", "../real"),
+	)), DefaultIndexLimits, new(atomic.Int64))
+	if err != nil {
+		t.Fatal(err)
+	}
+	info := dockerx.ContainerInfo{ID: tarID, Tmpfs: []string{"/var/run", "/absent/x", "/dangling/x", "/var/run/new/../sub"},
+		Mounts: []dockerx.Mount{{Type: "tmpfs", Destination: "/var/lock"}, {Type: "volume", Name: "v", Destination: "/tmp"}}}
+	var w WriteRootChecker = newTarFS(&dockerx.Fake{}, info, idx, func() {})
+	points, err := w.CheckWriteRoot(tctx)
+	want := []string{"/run/lock", "/run", "/absent/x", "/real/x", "/run/sub"}
+	if err != nil || !slices.Equal(points, want) {
+		t.Fatalf("tmpfs résolus par l'index : %v %v, attendu %v", points, err, want)
+	}
+}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go vet ./internal/fsview/ 2>&1 | tail -n 3 ; go test ./internal/files/ 2>&1 | tail -n 6`
Expected: `not enough arguments in call to newTarFS` ; `--- FAIL: TestWriteRefusedWhenRootIsNotContainerRoot` et `--- FAIL: TestWriteRefusedUnderResolvedTmpfs` (la vue de test n'implémente plus l'ancienne interface).

Run: `GOOS=linux go vet -tags integration ./internal/fsview/ 2>&1 | tail -n 3 ; scripts/test-linux.sh -tags integration ./internal/api/ -run TmpfsUnderLink 2>&1 | tail -n 5`
Expected: échec de compilation (`assignment mismatch: 2 variables but w.CheckWriteRoot returns 1 value`, ou d'abord `not enough arguments in call to newTarFS`) ; `--- FAIL: TestIntegrationWriteTmpfsUnderLink` — `envoi dans /etc : 403 {"error":"forbidden_path","detail":"… ne montre pas la destination /var/run …"}` : le défaut de §13, reproduit.

- [ ] **Step 3: Implémenter**

```diff
diff --git a/internal/files/files.go b/internal/files/files.go
--- a/internal/files/files.go
+++ b/internal/files/files.go
@@ -16,6 +16,7 @@ import (
 	"io/fs"
 	"log/slog"
 	"path"
+	"slices"
 	"strings"
 	"sync"
 	"time"
@@ -109,7 +110,7 @@ func (w *Writer) Edit(ctx context.Context, c dockerx.ContainerInfo, p string, co
 		return "", err
 	}
 	defer fsys.Close()
-	if err := checkRoot(ctx, fsys); err != nil {
+	if err := checkRoot(ctx, fsys, target); err != nil {
 		return "", err
 	}
 	e, err := fsys.StatNoLinks(ctx, target)
@@ -182,7 +183,7 @@ func (w *Writer) Upload(ctx context.Context, c dockerx.ContainerInfo, dir, name
 		return "", err
 	}
 	defer fsys.Close()
-	if err := checkRoot(ctx, fsys); err != nil {
+	if err := checkRoot(ctx, fsys, target); err != nil {
 		return "", err
 	}
 	d, err := fsys.StatNoLinks(ctx, parent)
@@ -244,12 +245,19 @@ func (w *Writer) put(ctx context.Context, action, id, target string, tarball []b
 }
 
 // checkRoot: the view must show the container's own root, else it checks a
-// tree Docker does not write in (a chrooted PID 1; review I1).
-func checkRoot(ctx context.Context, fsys fsview.FS) error {
-	if rc, ok := fsys.(fsview.WriteRootChecker); ok {
-		return rc.CheckWriteRoot(ctx)
+// tree Docker does not write in (a chrooted PID 1; review I1). target must
+// not be under a tmpfs where the view finds it (spec §13: a --tmpfs under a
+// link, /var/run -> ../run, is at /run): Docker would write unseen.
+func checkRoot(ctx context.Context, fsys fsview.FS, target string) error {
+	rc, ok := fsys.(fsview.WriteRootChecker)
+	if !ok {
+		return nil
 	}
-	return nil
+	points, err := rc.CheckWriteRoot(ctx)
+	if err != nil {
+		return err
+	}
+	return underTmpfs(target, points)
 }
 
 // readCapped reads the current file through the view (regular files only),
@@ -308,17 +316,23 @@ func (w *Writer) writable(ctx context.Context, c dockerx.ContainerInfo, target s
 	case "/etc/hosts", "/etc/hostname", "/etc/resolv.conf":
 		return fmt.Errorf("%w : %s est géré et monté par Docker", fsview.ErrForbiddenPath, target)
 	}
+	tmpfs := slices.Clone(c.Tmpfs)
 	for _, m := range c.Mounts {
 		d := fsview.CleanPath(m.Destination)
 		if d == target {
 			return fmt.Errorf("%w : %s est monté (%s %s) : modifiez sa source", fsview.ErrForbiddenPath, target, m.Type, m.Source+m.Name)
 		}
-		if m.Type == "tmpfs" && under(target, d) {
-			return fmt.Errorf("%w : %s est sous le tmpfs %s, Docker n'y écrit pas", fsview.ErrForbiddenPath, target, d)
+		if m.Type == "tmpfs" {
+			tmpfs = append(tmpfs, d)
 		}
 	}
-	for _, d := range c.Tmpfs {
-		if under(target, fsview.CleanPath(d)) {
+	return underTmpfs(target, tmpfs)
+}
+
+// underTmpfs refuses target on or under one of the tmpfs mount points.
+func underTmpfs(target string, points []string) error {
+	for _, d := range points {
+		if d := fsview.CleanPath(d); under(target, d) {
 			return fmt.Errorf("%w : %s est sous le tmpfs %s, Docker n'y écrit pas", fsview.ErrForbiddenPath, target, d)
 		}
 	}
diff --git a/internal/fsview/fsview.go b/internal/fsview/fsview.go
--- a/internal/fsview/fsview.go
+++ b/internal/fsview/fsview.go
@@ -54,8 +54,12 @@ type MountPointChecker interface {
 // container's root (procfs: the mount table of the pinned process shows "/").
 // With a chrooted PID 1 the view shows another tree than the one Docker
 // writes in: package files refuses every write then (review I1, plan 3).
+// It also returns where the container's tmpfs mounts are (Inspect.Mounts
+// of type tmpfs and --tmpfs), in the view's coordinates: a destination
+// under a link is found where it is really mounted (spec §13; procfs by
+// mount ID, tarindex through the index). Docker writes in none of them.
 type WriteRootChecker interface {
-	CheckWriteRoot(ctx context.Context) error
+	CheckWriteRoot(ctx context.Context) (tmpfs []string, err error)
 }
 
 // FS is a read view of a container's tree. Paths are container paths;
diff --git a/internal/fsview/mounts.go b/internal/fsview/mounts.go
--- a/internal/fsview/mounts.go
+++ b/internal/fsview/mounts.go
@@ -285,7 +285,7 @@ func LocateMounts(mountinfo string, mounts []dockerx.Mount, ident func(p string)
 			continue
 		}
 		l, ok := lines[want.MountID]
-		if !ok || l.Point == "/" || taken[l.Point] || !sameMount(m, l.Root) {
+		if !ok || l.Point == "/" || taken[l.Point] || !sameMount(m, l) {
 			continue
 		}
 		if got, err := ident(l.Point); err != nil || got != want {
@@ -296,15 +296,53 @@ func LocateMounts(mountinfo string, mounts []dockerx.Mount, ident func(p string)
 	return out
 }
 
-// sameMount ties a mountinfo root (field 4) to a Docker mount: the end of a
-// bind's Source, or the data directory of a named volume. Anything else
-// (tmpfs, a bind of a host mount root shown as "/") cannot be tied: false.
-func sameMount(m dockerx.Mount, root string) bool {
+// sameMount ties a mountinfo line to a Docker mount: its root (field 4) ends
+// the Source of a bind, or is the data directory of a named volume. A tmpfs
+// (spec §13: a --tmpfs under a link) has no source to tie: the line must be
+// a whole tmpfs (type tmpfs, root "/") outside the mounts the runtime adds
+// (/dev, /dev/shm, /proc, /sys), which a link could otherwise borrow.
+// Anything else (a bind of a host mount root shown as "/") cannot be tied:
+// false.
+func sameMount(m dockerx.Mount, l mountLine) bool {
 	switch m.Type {
 	case "bind":
-		return root != "/" && strings.HasSuffix(CleanPath(m.Source), root)
+		return l.Root != "/" && strings.HasSuffix(CleanPath(m.Source), l.Root)
 	case "volume":
-		return m.Name != "" && strings.HasSuffix(root, "/"+m.Name+"/_data")
+		return m.Name != "" && strings.HasSuffix(l.Root, "/"+m.Name+"/_data")
+	case "tmpfs":
+		return l.FSType == "tmpfs" && l.Root == "/" && !runtimeMount(l.Point)
 	}
 	return false
 }
+
+// runtimeMount: on or under /dev, /proc or /sys, which the runtime mounts in
+// every container (Docker never writes there either).
+func runtimeMount(p string) bool {
+	for _, d := range []string{"/dev", "/proc", "/sys"} {
+		if p == d || strings.HasPrefix(p, d+"/") {
+			return true
+		}
+	}
+	return false
+}
+
+// withTmpfs: the Docker mounts plus the --tmpfs destinations
+// (HostConfig.Tmpfs, absent from Inspect.Mounts), as tmpfs mounts.
+func withTmpfs(mounts []dockerx.Mount, tmpfs []string) []dockerx.Mount {
+	out := slices.Clone(mounts)
+	for _, d := range tmpfs {
+		out = append(out, dockerx.Mount{Type: "tmpfs", Destination: d})
+	}
+	return out
+}
+
+// tmpfsPoints: the cleaned destinations of the tmpfs mounts of ms.
+func tmpfsPoints(ms []dockerx.Mount) []string {
+	var out []string
+	for _, m := range ms {
+		if m.Type == "tmpfs" {
+			out = append(out, CleanPath(m.Destination))
+		}
+	}
+	return out
+}
diff --git a/internal/fsview/procfs_archive_linux.go b/internal/fsview/procfs_archive_linux.go
--- a/internal/fsview/procfs_archive_linux.go
+++ b/internal/fsview/procfs_archive_linux.go
@@ -10,7 +10,6 @@ import (
 	"slices"
 
 	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
-	"github.com/levy-cmd/hublot-kit/internal/dockerx"
 	"golang.org/x/sys/unix"
 )
 
@@ -58,42 +57,42 @@ func (p *procFS) mountIdent(name string) (MountIdent, error) {
 //     descriptor pinned when the view was opened (else the process changed
 //     root after the open);
 //   - every destination declared by Inspect, tmpfs included, appears as a
-//     mount point (CheckVisibleMounts): a PID 1 chrooted into a volume or a
-//     tmpfs of the container shows a "/" line, but its destination is gone.
+//     mount point (CheckVisibleMounts), verbatim or found by mount ID
+//     (LocateMounts: a --tmpfs under a link, spec §13): a PID 1 chrooted
+//     into a volume or a tmpfs of the container shows a "/" line, but its
+//     destination is gone.
 //
+// It returns the mount points of the tmpfs mounts, as located.
 // A jail the container mounts itself (CAP_SYS_ADMIN) is not seen: spec §13.
 // A view without process (NewProcFS, tests only) has nothing to read; every
 // real view comes from For with a pid.
-func (p *procFS) CheckWriteRoot(context.Context) error {
+func (p *procFS) CheckWriteRoot(context.Context) ([]string, error) {
 	if p.pidfd < 0 {
-		return nil
+		return nil, nil
 	}
 	raw, err := readMountinfo(p.pid)
 	if err != nil {
-		return changed("lecture de /proc/%d/mountinfo : %v", p.pid, err)
+		return nil, changed("lecture de /proc/%d/mountinfo : %v", p.pid, err)
 	}
 	if err := alive(p.pidfd, p.pid); err != nil {
-		return err
+		return nil, err
 	}
 	mi := string(raw)
 	if err := CheckRootMount(mi); err != nil {
-		return err
+		return nil, err
 	}
 	st, err := statxMountID(int(p.root.Fd()), 0, 0)
 	if err != nil {
-		return err
+		return nil, err
 	}
 	if !slices.Contains(rootMountIDs(mi), st.Mnt_id) {
-		return fmt.Errorf("%w : le montage \"/\" du process n'est pas celui de la racine épinglée à l'ouverture de la vue (chroot du process) : écriture refusée", ErrForbiddenPath)
+		return nil, fmt.Errorf("%w : le montage \"/\" du process n'est pas celui de la racine épinglée à l'ouverture de la vue (chroot du process) : écriture refusée", ErrForbiddenPath)
 	}
-	mounts := slices.Clone(p.mounts)
-	for _, d := range p.tmpfs {
-		mounts = append(mounts, dockerx.Mount{Type: "tmpfs", Destination: d})
-	}
-	if err := CheckVisibleMounts(mi, LocateMounts(mi, mounts, p.mountIdent)); err != nil {
-		return fmt.Errorf("%w (écriture refusée : le process n'est peut-être pas dans la racine du conteneur, chroot du PID 1)", err)
+	mounts := LocateMounts(mi, withTmpfs(p.mounts, p.tmpfs), p.mountIdent)
+	if err := CheckVisibleMounts(mi, mounts); err != nil {
+		return nil, fmt.Errorf("%w (écriture refusée : le process n'est peut-être pas dans la racine du conteneur, chroot du PID 1)", err)
 	}
-	return nil
+	return tmpfsPoints(mounts), nil
 }
 
 // CheckArchive (spec §5.2 step 7, §5.4). The checks that read nothing in the
diff --git a/internal/fsview/tarindex.go b/internal/fsview/tarindex.go
--- a/internal/fsview/tarindex.go
+++ b/internal/fsview/tarindex.go
@@ -37,12 +37,13 @@ type tarFS struct {
 	docker dockerx.Client
 	id     string
 	mounts []dockerx.Mount
+	tmpfs  []string // --tmpfs destinations of Inspect (not in mounts)
 	idx    *index
 	stale  func()
 }
 
-func newTarFS(docker dockerx.Client, id string, mounts []dockerx.Mount, idx *index, stale func()) *tarFS {
-	return &tarFS{docker: docker, id: id, mounts: mounts, idx: idx, stale: stale}
+func newTarFS(docker dockerx.Client, info dockerx.ContainerInfo, idx *index, stale func()) *tarFS {
+	return &tarFS{docker: docker, id: info.ID, mounts: info.Mounts, tmpfs: info.Tmpfs, idx: idx, stale: stale}
 }
 
 var _ FS = (*tarFS)(nil)
@@ -182,6 +183,19 @@ func progress(read, total int64) float64 {
 // path that ever goes to GetArchive. cur never contains a link, so ".." is
 // the parent of the resolved target, and stays at "/" on the root.
 func (x *index) resolve(p string) (string, error) {
+	cur, rest, err := x.walk(p)
+	if err != nil {
+		return "", err
+	}
+	if rest != nil {
+		return "", notExist(p)
+	}
+	return cur, nil
+}
+
+// walk is resolve that stops at the first missing component: it returns
+// the resolved path so far and the components left (nil when p exists).
+func (x *index) walk(p string) (cur string, rest []string, err error) {
 	todo := components(p)
 	mustDir := strings.HasSuffix(p, "/")
 	cur, hops := "/", 0
@@ -195,14 +209,14 @@ func (x *index) resolve(p string) (string, error) {
 		next := childPath(cur, c)
 		e, ok := x.entries[next]
 		if !ok {
-			return "", notExist(p)
+			return cur, append([]string{c}, todo...), nil
 		}
 		if e.Type == TypeSymlink {
 			if hops++; hops > maxHops {
-				return "", &fs.PathError{Op: "resolve", Path: p, Err: syscall.ELOOP}
+				return "", nil, &fs.PathError{Op: "resolve", Path: p, Err: syscall.ELOOP}
 			}
 			if e.LinkTarget == "" {
-				return "", notExist(p)
+				return "", nil, notExist(p)
 			}
 			if strings.HasPrefix(e.LinkTarget, "/") {
 				cur = "/" // absolute: from the container root, never the host's
@@ -214,14 +228,14 @@ func (x *index) resolve(p string) (string, error) {
 			continue
 		}
 		if len(todo) > 0 && e.Type != TypeDir {
-			return "", fmt.Errorf("%w : %s", ErrNotDir, p)
+			return "", nil, fmt.Errorf("%w : %s", ErrNotDir, p)
 		}
 		cur = next
 	}
 	if mustDir && x.entries[cur].Type != TypeDir {
-		return "", fmt.Errorf("%w : %s", ErrNotDir, p)
+		return "", nil, fmt.Errorf("%w : %s", ErrNotDir, p)
 	}
-	return cur, nil
+	return cur, nil, nil
 }
 
 func components(p string) []string {
@@ -372,4 +386,21 @@ func (t *tarFS) CheckArchive(ctx context.Context, _ string) error {
 	return HostBind(ctx, t.docker, t.mounts)
 }
 
+// CheckWriteRoot (stopped container): no process, the root is the one
+// Docker writes in. At start, runc mounts each tmpfs at its destination
+// resolved in the root filesystem, links followed (spec §13: /var/run ->
+// ../run puts it at /run): the index resolves it the same way, never out
+// of the root, as far as the path exists; runc creates the rest there
+// (review Minor 1: /dangling -> ../real, then x, gives /real/x). A path
+// the index refuses (loop, not a directory) stays as declared.
+func (t *tarFS) CheckWriteRoot(context.Context) ([]string, error) {
+	points := tmpfsPoints(withTmpfs(t.mounts, t.tmpfs))
+	for i, d := range points {
+		if cur, rest, err := t.idx.walk(d); err == nil {
+			points[i] = CleanPath(cur + "/" + strings.Join(rest, "/"))
+		}
+	}
+	return points, nil
+}
+
 func (t *tarFS) Close() error { return nil }
diff --git a/internal/fsview/views.go b/internal/fsview/views.go
--- a/internal/fsview/views.go
+++ b/internal/fsview/views.go
@@ -114,7 +114,7 @@ func (v *Views) For(ctx context.Context, id string) (FS, error) {
 	if err != nil {
 		return nil, err
 	}
-	return newTarFS(v.docker, info.ID, info.Mounts, idx, func() { v.drop(key, nil) }), nil
+	return newTarFS(v.docker, info, idx, func() { v.drop(key, nil) }), nil
 }
 
 // Forget drops every index of container id (full ID): plan Explorateur 3
```

- [ ] **Step 4: Vérifier**

Run: `go test ./internal/fsview/ ./internal/files/ 2>&1 | tail -n 3 && scripts/test-linux.sh -tags integration ./internal/fsview/ ./internal/files/ ./internal/api/ -run 'Tmpfs|CheckWriteRoot|LocateMounts|WriteRunning|WriteRefused' -v 2>&1 | grep -E '^(---|ok|FAIL)|SKIP|ÉVASION' | tail -n 20`
Expected: `ok` ; sous Linux, `PASS` pour `TestLocateMountsTmpfs`, `TestCheckWriteRoot`, `TestCheckWriteRootJail`, `TestCheckArchiveTmpfsJail`, `TestTarCheckWriteRootTmpfs`, `TestIntegrationCheckWriteRoot`, `TestIntegrationCheckWriteRootTmpfsUnderLink`, `TestIntegrationCheckArchiveTmpfsUnderLink`, `TestWriteRefusedUnderResolvedTmpfs`, `TestIntegrationWriteRunning`, `TestIntegrationWriteTmpfsUnderLink`, aucun SKIP, aucune ligne `ÉVASION`. Le motif `-run` ci-dessus attrape aussi `TestCheckArchiveTmpfsJail` (`Tmpfs`).

Mutations à rejouer (une fois, puis annuler) : retirer `l.Point == "/" ||` de `LocateMounts` → `TestLocateMountsTmpfs` (« prison dans le tmpfs acceptée »), `TestCheckArchiveTmpfsJail` (« tar accepté dans une prison tmpfs ») et `TestIntegrationCheckWriteRootTmpfsUnderLink` échouent ; retirer `&& !runtimeMount(l.Point)` → `TestLocateMountsTmpfs` (« leurre : /dev/shm du runtime accepté ») ; remplacer `return underTmpfs(target, points)` par `return nil` dans `checkRoot` → `TestWriteRefusedUnderResolvedTmpfs`.

- [ ] **Step 5: Commit**

```bash
git add internal/fsview internal/files internal/api/integration_write_linux_test.go
git commit -F - <<'EOF'
fsview, files : tmpfs sous un lien retrouvé par identifiant de montage, écriture refusée sous le point retrouvé

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 2: `dockerx.Logs` et le lecteur des flux de logs

**Où tester :** macOS (unitaires), Docker Desktop (intégration, portable).

**Files:**
- Create: `internal/dockerx/logs.go`, `internal/dockerx/logs_test.go`
- Modify: `internal/dockerx/dockerx.go`, `internal/dockerx/fake.go`
- Test: `internal/dockerx/integration_test.go`

**Interfaces:**
- Consumes: `wrap`, `client.ContainerLogs` (`client.ContainerLogsOptions{ShowStdout, ShowStderr, Timestamps, Follow, Tail, Since}`, SDK v0.6.1), `cerrdefs.IsNotImplemented`.
- Produces: `type LogsOptions struct{ Tail, Since string; Follow bool }` ; `type LogLine struct{ TS time.Time; Stream, Text string; Truncated bool }` ; `Client.Logs(ctx, id string, opt LogsOptions) (io.ReadCloser, error)` ; `ReadLogs(r io.Reader, tty bool, max int, emit func(LogLine) error) error` ; `MaxLogLine = 16 << 10` ; `ErrNoLogs` ; `wrapLogs` ; `ContainerInfo.Tty bool`, `ContainerInfo.LogDriver string` ; `Fake.LogStreams map[string][]byte`, `Fake.LogsErr`, `Fake.LogsGate chan struct{}`, `Fake.LogsCloses atomic.Int32`, `(*Fake).LogsCalls() []LogsOptions`.

> **À comprendre :** `GET /containers/{id}/logs` rend un flux d'octets. Sans TTY, il est **multiplexé** : chaque message est une trame de 8 octets d'en-tête (type : 1 stdout, 2 stderr, 3 erreur du démon ; taille en gros-boutiste) puis la charge, `"<horodatage> <texte>\n"`. Avec un TTY, pas de trame : un seul flux, des fins de ligne `\r\n`. Docker coupe une ligne de plus de 16 Kio en parties qui portent **le même horodatage** (mesuré) ; en TTY, ces parties sont collées l'une à l'autre avec leur horodatage au milieu. `ReadLogs` recolle les parties (en TTY, l'horodatage n'est retiré qu'à la frontière exacte de 16 384 octets, et seulement s'il est identique à celui de la ligne), plafonne le texte à `max` et lit une trame par morceaux de 32 Kio : un en-tête qui annonce 1 Gio n'alloue rien. Il ne lit jamais plus d'octets que le flux n'en a envoyé : une ligne en direct part dès son `\n`.

- [ ] **Step 1: Tests qui échouent** — formats mesurés, lignes longues, TTY, erreurs ; un vrai Docker pour les mesures :

```diff
diff --git a/internal/dockerx/integration_test.go b/internal/dockerx/integration_test.go
--- a/internal/dockerx/integration_test.go
+++ b/internal/dockerx/integration_test.go
@@ -7,6 +7,7 @@ import (
 	"bytes"
 	"context"
 	"errors"
+	"fmt"
 	"io"
 	"os/exec"
 	"slices"
@@ -232,3 +233,90 @@ func TestPutArchiveRealDocker(t *testing.T) {
 		t.Fatalf("dossier absent : %v, attendu ErrNotFound", err)
 	}
 }
+
+// collect reads a logs stream of container id to its end (or 20 s).
+func collect(t *testing.T, c Client, id string, opt LogsOptions, tty bool) []LogLine {
+	t.Helper()
+	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
+	defer cancel()
+	rc, err := c.Logs(ctx, id, opt)
+	if err != nil {
+		t.Fatalf("Logs(%s) : %v", id, err)
+	}
+	defer rc.Close()
+	var out []LogLine
+	if err := ReadLogs(rc, tty, MaxLogLine, func(l LogLine) error { out = append(out, l); return nil }); err != nil {
+		t.Fatalf("ReadLogs(%s) : %v", id, err)
+	}
+	return out
+}
+
+// texts: "stream:text" per line, a long text shortened to its length (the
+// test output stays readable). The first two lines are sorted: stdout and
+// stderr written at the same instant reach the log file in either order.
+func texts(ls []LogLine) []string {
+	var out []string
+	for _, l := range ls {
+		s := l.Stream + ":" + l.Text
+		if len(l.Text) > 64 {
+			s = fmt.Sprintf("%s:%d×%c tronquée=%v", l.Stream, len(l.Text), l.Text[0], l.Truncated)
+		}
+		out = append(out, s)
+	}
+	if len(out) >= 2 {
+		slices.Sort(out[:2])
+	}
+	return out
+}
+
+// F6 on a real daemon: stdout and stderr apart (multiplexed), one stream
+// with a TTY, a 40 000-byte line joined back, a stopped container's past
+// logs (Follow ends by itself), Since inclusive, a driver without reading.
+func TestLogsRealDocker(t *testing.T) {
+	plain, tty, none := "hublot-it-logs", "hublot-it-logs-tty", "hublot-it-logs-none"
+	exec.Command("docker", "rm", "-f", plain, tty, none).Run()
+	t.Cleanup(func() { exec.Command("docker", "rm", "-f", plain, tty, none).Run() })
+	script := `echo out1; echo err1 >&2; head -c 40000 /dev/zero | tr "\0" a; echo; echo out2`
+	for _, args := range [][]string{
+		{"run", "-d", "--name", plain, "alpine:3", "sh", "-c", script},
+		{"run", "-d", "-t", "--name", tty, "alpine:3", "sh", "-c", script},
+		{"run", "-d", "--name", none, "--log-driver", "none", "alpine:3", "true"},
+	} {
+		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
+			t.Fatalf("docker %v : %v %s", args, err, out)
+		}
+	}
+	exec.Command("docker", "wait", plain, tty).Run() // stopped: past logs only
+	c, err := New()
+	if err != nil {
+		t.Fatal(err)
+	}
+	ctx := context.Background()
+	info, err := c.Inspect(ctx, tty)
+	if err != nil || !info.Tty || info.LogDriver != "json-file" {
+		t.Fatalf("Inspect TTY : %+v %v", info, err)
+	}
+
+	long := fmt.Sprintf("stdout:%d×a tronquée=true", MaxLogLine)
+	got := collect(t, c, plain, LogsOptions{Tail: "all", Follow: true}, false)
+	if want := []string{"stderr:err1", "stdout:out1", long, "stdout:out2"}; !slices.Equal(texts(got), want) || got[0].TS.IsZero() {
+		t.Fatalf("sans TTY : %q, attendu %q", texts(got), want)
+	}
+	// Since is inclusive: from the timestamp of the long line, it comes again.
+	again := collect(t, c, plain, LogsOptions{Tail: "all", Since: got[2].TS.Format(time.RFC3339Nano), Follow: true}, false)
+	if want := []string{long, "stdout:out2"}; !slices.Equal(texts(again), want) {
+		t.Fatalf("Since inclusif : %q, attendu %q", texts(again), want)
+	}
+	if tail := collect(t, c, plain, LogsOptions{Tail: "1"}, false); !slices.Equal(texts(tail), []string{"stdout:out2"}) {
+		t.Fatalf("Tail 1 : %q", texts(tail))
+	}
+
+	got = collect(t, c, tty, LogsOptions{Tail: "all", Follow: true}, true)
+	if want := []string{"stdout:err1", "stdout:out1", long, "stdout:out2"}; !slices.Equal(texts(got), want) {
+		t.Fatalf("TTY : %q, attendu %q", texts(got), want)
+	}
+
+	if _, err := c.Logs(ctx, none, LogsOptions{Tail: "all"}); !errors.Is(err, ErrNoLogs) {
+		t.Fatalf("--log-driver none : %v, attendu ErrNoLogs", err)
+	}
+}
diff --git a/internal/dockerx/logs_test.go b/internal/dockerx/logs_test.go
new file mode 100644
--- /dev/null
+++ b/internal/dockerx/logs_test.go
@@ -0,0 +1,255 @@
+package dockerx
+
+// Tests of the logs reader (F6), written before the code. The byte layouts
+// are those measured on Docker 29.7 (plan Observation, "Constats"): one
+// frame per message, the timestamp at the head of every frame, a line over
+// 16 KiB cut into parts that repeat the same timestamp; with a TTY, no
+// frames, "\r\n" line ends, and the parts glued with their timestamps.
+
+import (
+	"bytes"
+	"context"
+	"encoding/binary"
+	"errors"
+	"fmt"
+	"io"
+	"slices"
+	"strings"
+	"testing"
+	"time"
+
+	cerrdefs "github.com/containerd/errdefs"
+	"github.com/moby/moby/api/types/container"
+)
+
+const (
+	ts1 = "2026-10-10T07:14:22.769046552Z"
+	ts2 = "2026-10-10T07:14:23.775474053Z"
+)
+
+// frame is one frame of Docker's multiplexed stream: type, 3 zero bytes,
+// big-endian size, payload.
+func frame(typ byte, payload string) []byte {
+	h := make([]byte, 8, 8+len(payload))
+	h[0] = typ
+	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
+	return append(h, payload...)
+}
+
+func frames(fs ...[]byte) io.Reader { return bytes.NewReader(bytes.Join(fs, nil)) }
+
+func readAll(t *testing.T, r io.Reader, tty bool) []LogLine {
+	t.Helper()
+	var out []LogLine
+	if err := ReadLogs(r, tty, MaxLogLine, func(l LogLine) error { out = append(out, l); return nil }); err != nil {
+		t.Fatalf("ReadLogs : %v", err)
+	}
+	return out
+}
+
+func mustTS(t *testing.T, s string) time.Time {
+	t.Helper()
+	ts, err := time.Parse(time.RFC3339Nano, s)
+	if err != nil {
+		t.Fatal(err)
+	}
+	return ts
+}
+
+func TestReadLogsMultiplexed(t *testing.T) {
+	got := readAll(t, frames(
+		frame(2, ts1+" err1\n"),
+		frame(1, ts1+" out1\n"),
+		frame(1, ts2+" nonl\n"), // Docker keeps a line without "\n" until it ends
+		frame(0, ts2+" stdin is stdout\n"),
+	), false)
+	want := []LogLine{
+		{TS: mustTS(t, ts1), Stream: "stderr", Text: "err1"},
+		{TS: mustTS(t, ts1), Stream: "stdout", Text: "out1"},
+		{TS: mustTS(t, ts2), Stream: "stdout", Text: "nonl"},
+		{TS: mustTS(t, ts2), Stream: "stdout", Text: "stdin is stdout"},
+	}
+	if !slices.Equal(got, want) {
+		t.Fatalf("got %+v", got)
+	}
+}
+
+// Measured: a 40 000-byte line comes as 16 384 + 16 384 + 7 232 bytes, each
+// part behind the same timestamp. It is one line, cut at MaxLogLine.
+func TestReadLogsPartialMessages(t *testing.T) {
+	a := strings.Repeat("a", 16384)
+	got := readAll(t, frames(
+		frame(1, ts1+" "+a),
+		frame(1, ts1+" "+a),
+		frame(1, ts1+" "+strings.Repeat("a", 7232)+"\n"),
+		frame(2, ts2+" short\n"),
+	), false)
+	if len(got) != 2 {
+		t.Fatalf("%d lignes, attendu 2", len(got))
+	}
+	if got[0].Text != strings.Repeat("a", MaxLogLine) || !got[0].Truncated || got[0].Stream != "stdout" {
+		t.Errorf("ligne longue : %d octets, tronquée %v", len(got[0].Text), got[0].Truncated)
+	}
+	if got[1] != (LogLine{TS: mustTS(t, ts2), Stream: "stderr", Text: "short"}) {
+		t.Errorf("ligne suivante : %+v", got[1])
+	}
+}
+
+// Parts of stdout and stderr may interleave: each stream joins its own.
+func TestReadLogsPartialsPerStream(t *testing.T) {
+	got := readAll(t, frames(
+		frame(1, ts1+" out-"),
+		frame(2, ts2+" err\n"),
+		frame(1, ts1+" end\n"),
+	), false)
+	if len(got) != 2 || got[0].Text != "err" || got[1].Text != "out-end" || got[1].Stream != "stdout" {
+		t.Fatalf("got %+v", got)
+	}
+}
+
+func TestReadLogsTTY(t *testing.T) {
+	got := readAll(t, strings.NewReader(ts1+" out1\r\n"+ts1+" err1\r\n"+ts2+" a\rb\r\n"), true)
+	want := []LogLine{
+		{TS: mustTS(t, ts1), Stream: "stdout", Text: "out1"},
+		{TS: mustTS(t, ts1), Stream: "stdout", Text: "err1"},
+		{TS: mustTS(t, ts2), Stream: "stdout", Text: "a\rb"}, // only the final \r goes
+	}
+	if !slices.Equal(got, want) {
+		t.Fatalf("got %+v", got)
+	}
+}
+
+// Measured with a TTY: "TS <16 384 bytes>TS <16 384 bytes>TS <rest>\r\n".
+// The timestamps glued at the 16 KiB boundaries are not part of the text.
+func TestReadLogsTTYPartialMessages(t *testing.T) {
+	a := strings.Repeat("a", 16384)
+	b := strings.Repeat("b", 16384)
+	in := ts1 + " " + a + ts1 + " " + b + ts1 + " ccc\r\n" + ts2 + " short\r\n"
+	got := readAll(t, strings.NewReader(in), true)
+	if len(got) != 2 || got[1].Text != "short" || got[1].TS != mustTS(t, ts2) {
+		t.Fatalf("got %d lignes : %+v", len(got), got[len(got)-1])
+	}
+	if got[0].Text != a || !got[0].Truncated || strings.Contains(got[0].Text, "Z ") {
+		t.Errorf("ligne longue : %d octets, tronquée %v", len(got[0].Text), got[0].Truncated)
+	}
+	// A line of exactly 16 KiB: its last part is empty, behind the same
+	// timestamp; the line is whole, not truncated.
+	got = readAll(t, strings.NewReader(ts1+" "+a+ts1+" \r\n"), true)
+	if len(got) != 1 || got[0].Text != a || got[0].Truncated {
+		t.Errorf("ligne de 16 Kio exactement : %d lignes, tronquée %v", len(got), len(got) > 0 && got[0].Truncated)
+	}
+	// Not at a boundary, the same text stays: it belongs to the line.
+	got = readAll(t, strings.NewReader(ts1+" at "+ts1+" done\r\n"), true)
+	if len(got) != 1 || got[0].Text != "at "+ts1+" done" {
+		t.Errorf("horodatage dans le texte : %+v", got)
+	}
+}
+
+// The cap is the caller's: the live view keeps MaxLogLine, the download
+// a larger one.
+func TestReadLogsCap(t *testing.T) {
+	for _, tty := range []bool{false, true} {
+		in := io.Reader(strings.NewReader(ts1 + " abcdef\r\n" + ts1 + " abcd\r\n"))
+		if !tty {
+			in = frames(frame(1, ts1+" abcdef\n"), frame(1, ts1+" abcd\n"))
+		}
+		var got []LogLine
+		ReadLogs(in, tty, 4, func(l LogLine) error { got = append(got, l); return nil })
+		if len(got) != 2 || got[0].Text != "abcd" || !got[0].Truncated || got[1].Text != "abcd" || got[1].Truncated {
+			t.Errorf("tty=%v : %+v", tty, got)
+		}
+	}
+}
+
+func TestReadLogsMessageEndsAtEOF(t *testing.T) {
+	got := readAll(t, frames(frame(1, ts1+" last words")), false)
+	if len(got) != 1 || got[0].Text != "last words" {
+		t.Fatalf("multiplexé : %+v", got)
+	}
+	got = readAll(t, strings.NewReader(ts1+" tty last"), true)
+	if len(got) != 1 || got[0].Text != "tty last" {
+		t.Fatalf("TTY : %+v", got)
+	}
+}
+
+// Several lines in one frame, a line without timestamp: kept, never lost.
+func TestReadLogsOddFrames(t *testing.T) {
+	got := readAll(t, frames(frame(1, ts1+" a\nb\n"), frame(1, "no timestamp\n")), false)
+	if len(got) != 3 || got[0].Text != "a" || got[1].Text != "b" || got[1].TS != mustTS(t, ts1) ||
+		got[2].Text != "no timestamp" || !got[2].TS.IsZero() {
+		t.Fatalf("got %+v", got)
+	}
+}
+
+// A frame of type 3 is an error of the daemon; a cut header or payload is
+// an error too, and a header announcing 1 GiB is never allocated.
+func TestReadLogsErrors(t *testing.T) {
+	if err := ReadLogs(frames(frame(3, "log file is gone")), false, MaxLogLine, func(LogLine) error { return nil }); err == nil || !strings.Contains(err.Error(), "log file is gone") {
+		t.Errorf("trame d'erreur : %v", err)
+	}
+	if err := ReadLogs(bytes.NewReader([]byte{1, 0, 0}), false, MaxLogLine, func(LogLine) error { return nil }); !errors.Is(err, io.ErrUnexpectedEOF) {
+		t.Errorf("en-tête coupé : %v", err)
+	}
+	huge := []byte{1, 0, 0, 0, 0x40, 0, 0, 0} // 1 GiB announced, 0 sent
+	allocs := testing.AllocsPerRun(1, func() {
+		ReadLogs(bytes.NewReader(huge), false, MaxLogLine, func(LogLine) error { return nil })
+	})
+	if allocs > 100 {
+		t.Errorf("%v allocations pour une trame annoncée de 1 Gio", allocs)
+	}
+	stop := errors.New("stop")
+	n := 0
+	err := ReadLogs(frames(frame(1, ts1+" a\n"), frame(1, ts1+" b\n")), false, MaxLogLine, func(LogLine) error { n++; return stop })
+	if !errors.Is(err, stop) || n != 1 {
+		t.Errorf("emit en erreur : %v après %d lignes", err, n)
+	}
+}
+
+func TestFromInspectTtyAndLogDriver(t *testing.T) {
+	got := fromInspect(container.InspectResponse{ID: "x", Config: &container.Config{Tty: true},
+		HostConfig: &container.HostConfig{LogConfig: container.LogConfig{Type: "json-file"}}})
+	if !got.Tty || got.LogDriver != "json-file" {
+		t.Fatalf("got %+v", got)
+	}
+}
+
+func TestWrapLogs(t *testing.T) {
+	err := wrapLogs(fmt.Errorf("configured logging driver does not support reading: %w", cerrdefs.ErrNotImplemented))
+	if !errors.Is(err, ErrNoLogs) || errors.Is(err, ErrUnavailable) {
+		t.Errorf("pilote sans lecture : %v", err)
+	}
+	if err := wrapLogs(fmt.Errorf("x: %w", cerrdefs.ErrNotFound)); !errors.Is(err, ErrNotFound) {
+		t.Errorf("introuvable : %v", err)
+	}
+}
+
+func TestFakeLogs(t *testing.T) {
+	f := &Fake{LogStreams: map[string][]byte{"abc": []byte("RAW")}, LogsGate: make(chan struct{})}
+	ctx, cancel := context.WithCancel(context.Background())
+	rc, err := f.Logs(ctx, "abc", LogsOptions{Tail: "10", Follow: true})
+	if err != nil {
+		t.Fatal(err)
+	}
+	b := make([]byte, 3)
+	if _, err := io.ReadFull(rc, b); err != nil || string(b) != "RAW" {
+		t.Fatalf("lu %q %v", b, err)
+	}
+	done := make(chan error, 1)
+	go func() { _, err := rc.Read(b); done <- err }()
+	select {
+	case <-done:
+		t.Fatal("la porte doit retenir la suite du flux")
+	case <-time.After(50 * time.Millisecond):
+	}
+	cancel()
+	if err := <-done; !errors.Is(err, context.Canceled) {
+		t.Fatalf("après annulation : %v", err)
+	}
+	rc.Close()
+	if f.LogsCloses.Load() != 1 || !slices.Equal(f.LogsCalls(), []LogsOptions{{Tail: "10", Follow: true}}) {
+		t.Fatalf("fermetures %d, appels %+v", f.LogsCloses.Load(), f.LogsCalls())
+	}
+	if _, err := f.Logs(context.Background(), "nope", LogsOptions{}); !errors.Is(err, ErrNotFound) {
+		t.Fatalf("absent : %v", err)
+	}
+}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go vet ./internal/dockerx/ 2>&1 | tail -n 3`
Expected: échec de compilation (`undefined: LogLine`, puis `LogsOptions`, `ReadLogs`…).

- [ ] **Step 3: Implémenter**

```diff
diff --git a/internal/dockerx/dockerx.go b/internal/dockerx/dockerx.go
--- a/internal/dockerx/dockerx.go
+++ b/internal/dockerx/dockerx.go
@@ -45,6 +45,10 @@ type ContainerInfo struct {
 	// Tmpfs: destinations of --tmpfs (HostConfig.Tmpfs), sorted. Absent from
 	// Mounts; Docker never writes into them (plan Explorateur 3, Q21).
 	Tmpfs []string
+	// Tty: the logs come as one raw stream, not multiplexed (F6).
+	Tty bool
+	// LogDriver: HostConfig.LogConfig.Type ("json-file", "local", "none"…).
+	LogDriver string
 }
 
 // Change is one entry of GET /containers/{id}/changes (F5).
@@ -99,6 +103,11 @@ type Client interface {
 	// Docker never replaces a directory by a file (noOverwriteDirNonDir) and
 	// keeps the UID/GID of the tar headers (no copyUIDGID).
 	PutArchive(ctx context.Context, id, dir string, content io.Reader) error
+	// Logs is GET /containers/{id}/logs with stdout, stderr and timestamps
+	// (F6): the raw stream, multiplexed unless the container has a TTY; read
+	// it with ReadLogs. The caller closes it; it also ends with ctx.
+	// ErrNoLogs when the logging driver cannot be read.
+	Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error)
 }
 
 type engine struct{ c *client.Client }
@@ -197,7 +206,11 @@ func fromInspect(c container.InspectResponse) ContainerInfo {
 			Type: string(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: m.Driver,
 		})
 	}
+	if c.Config != nil {
+		info.Tty = c.Config.Tty
+	}
 	if c.HostConfig != nil {
+		info.LogDriver = c.HostConfig.LogConfig.Type
 		for d := range c.HostConfig.Tmpfs {
 			info.Tmpfs = append(info.Tmpfs, d)
 		}
diff --git a/internal/dockerx/fake.go b/internal/dockerx/fake.go
--- a/internal/dockerx/fake.go
+++ b/internal/dockerx/fake.go
@@ -38,7 +38,17 @@ type Fake struct {
 	// failed ones included (read them with PutCalls).
 	PutErr error
 	puts   []Put
-	Err    error
+	// LogStreams answers Logs with these raw bytes, by id as given (absent:
+	// ErrNotFound). LogsErr, when set, is returned instead.
+	LogStreams map[string][]byte
+	LogsErr    error
+	// LogsGate, when not nil, holds every logs stream after its bytes (a
+	// container that keeps running) until it is closed (end of stream) or
+	// the context ends (ctx.Err()).
+	LogsGate   chan struct{}
+	LogsCloses atomic.Int32
+	logsCalls  []LogsOptions
+	Err        error
 }
 
 // Put is one PutArchive call: the tar is read whole.
@@ -118,6 +128,54 @@ func (f *Fake) PutArchive(_ context.Context, id, dir string, content io.Reader)
 	return f.PutErr
 }
 
+func (f *Fake) Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error) {
+	f.mu.Lock()
+	f.logsCalls = append(f.logsCalls, opt)
+	f.mu.Unlock()
+	if f.Err != nil {
+		return nil, f.Err
+	}
+	if f.LogsErr != nil {
+		return nil, f.LogsErr
+	}
+	b, ok := f.LogStreams[id]
+	if !ok {
+		return nil, fmt.Errorf("%w : %s", ErrNotFound, id)
+	}
+	return &logsStream{Reader: bytes.NewReader(b), ctx: ctx, gate: f.LogsGate, closes: &f.LogsCloses}, nil
+}
+
+// LogsCalls returns a copy of the options of every Logs call.
+func (f *Fake) LogsCalls() []LogsOptions {
+	f.mu.Lock()
+	defer f.mu.Unlock()
+	return slices.Clone(f.logsCalls)
+}
+
+// logsStream serves its bytes, then waits at the gate, if any.
+type logsStream struct {
+	*bytes.Reader
+	ctx    context.Context
+	gate   chan struct{}
+	closes *atomic.Int32
+}
+
+func (s *logsStream) Read(p []byte) (int, error) {
+	if s.Reader.Len() > 0 {
+		return s.Reader.Read(p)
+	}
+	if s.gate != nil {
+		select {
+		case <-s.gate:
+		case <-s.ctx.Done():
+			return 0, s.ctx.Err()
+		}
+	}
+	return 0, io.EOF
+}
+
+func (s *logsStream) Close() error { s.closes.Add(1); return nil }
+
 // PutCalls returns a copy of the PutArchive calls, read under the lock.
 func (f *Fake) PutCalls() []Put {
 	f.mu.Lock()
diff --git a/internal/dockerx/logs.go b/internal/dockerx/logs.go
new file mode 100644
--- /dev/null
+++ b/internal/dockerx/logs.go
@@ -0,0 +1,290 @@
+package dockerx
+
+import (
+	"bufio"
+	"bytes"
+	"context"
+	"encoding/binary"
+	"errors"
+	"fmt"
+	"io"
+	"strings"
+	"time"
+
+	cerrdefs "github.com/containerd/errdefs"
+	"github.com/moby/moby/client"
+)
+
+// MaxLogLine caps the text of one line of the live view (F6): the size of
+// the parts Docker cuts a long line into. The download keeps more.
+const MaxLogLine = 16 << 10
+
+// dockerPartial is the size of the parts Docker cuts a long line into
+// (measured on Docker 29.7: 16 384 bytes, each behind the same timestamp).
+const dockerPartial = 16 << 10
+
+// ErrNoLogs: the container's logging driver cannot be read (--log-driver
+// none, syslog…): Docker answers 501 "configured logging driver does not
+// support reading".
+var ErrNoLogs = errors.New("le pilote de logs de ce conteneur ne permet pas la lecture")
+
+// LogsOptions of GET /containers/{id}/logs: stdout, stderr and timestamps
+// are always asked for. Tail is a number of lines or "all"; Since, when
+// set, an RFC 3339 timestamp, inclusive; Follow keeps the stream open while
+// the container runs (a stopped container's stream ends by itself).
+type LogsOptions struct {
+	Tail   string
+	Since  string
+	Follow bool
+}
+
+// LogLine is one line of a container's logs, without its line end.
+type LogLine struct {
+	TS        time.Time // zero when Docker gave none
+	Stream    string    // "stdout" | "stderr" (a TTY merges both into stdout)
+	Text      string
+	Truncated bool // longer than MaxLogLine: the rest was dropped
+}
+
+func (e *engine) Logs(ctx context.Context, id string, opt LogsOptions) (io.ReadCloser, error) {
+	rc, err := e.c.ContainerLogs(ctx, id, client.ContainerLogsOptions{
+		ShowStdout: true, ShowStderr: true, Timestamps: true,
+		Follow: opt.Follow, Tail: opt.Tail, Since: opt.Since,
+	})
+	if err != nil {
+		return nil, wrapLogs(err)
+	}
+	return rc, nil
+}
+
+// wrapLogs: wrap, plus a driver Docker cannot read (NotImplemented).
+func wrapLogs(err error) error {
+	if cerrdefs.IsNotImplemented(err) {
+		return fmt.Errorf("%w : %v", ErrNoLogs, err)
+	}
+	return wrap(err)
+}
+
+// ReadLogs reads a logs stream asked with timestamps and calls emit for
+// each line, in order. Without a TTY the stream is Docker's multiplexed
+// format (8-byte header: stream type, 3 zero bytes, big-endian size);
+// with a TTY it is raw, one stream. A line Docker cut into parts is joined
+// back (per stream), its text capped at max bytes (Truncated beyond). A
+// frame is never read whole into memory, whatever size its header
+// announces. It stops at the first error of r or of emit, and returns nil
+// at the end of the stream.
+func ReadLogs(r io.Reader, tty bool, max int, emit func(LogLine) error) error {
+	if tty {
+		return readRaw(bufio.NewReaderSize(r, 64<<10), max, emit)
+	}
+	return readFrames(r, max, emit)
+}
+
+// lineBuf assembles the line of one stream.
+type lineBuf struct {
+	max       int
+	stream    string
+	ts        time.Time
+	open      bool // a part was read, its line has not ended yet
+	text      []byte
+	truncated bool
+}
+
+// add keeps at most max+1 bytes: one more for the "\r" of a TTY line end,
+// which flush removes before it judges the length.
+func (b *lineBuf) add(p []byte) {
+	if room := b.max + 1 - len(b.text); len(p) > room {
+		p, b.truncated = p[:room], true
+	}
+	b.text = append(b.text, p...)
+}
+
+func (b *lineBuf) flush(emit func(LogLine) error) error {
+	if !b.open {
+		return nil
+	}
+	text := strings.TrimSuffix(string(b.text), "\r")
+	if len(text) > b.max {
+		text, b.truncated = text[:b.max], true
+	}
+	l := LogLine{TS: b.ts, Stream: b.stream, Text: text, Truncated: b.truncated}
+	b.open, b.text, b.truncated = false, b.text[:0], false
+	return emit(l)
+}
+
+// splitTS splits "<RFC 3339 timestamp> <text>"; no timestamp: zero, p.
+func splitTS(p []byte) (time.Time, []byte) {
+	if i := bytes.IndexByte(p[:min(len(p), 64)], ' '); i > 0 {
+		if ts, err := time.Parse(time.RFC3339Nano, string(p[:i])); err == nil {
+			return ts, p[i+1:]
+		}
+	}
+	return time.Time{}, p
+}
+
+func readFrames(r io.Reader, max int, emit func(LogLine) error) error {
+	bufs := map[byte]*lineBuf{1: {max: max, stream: "stdout"}, 2: {max: max, stream: "stderr"}}
+	bufs[0] = bufs[1] // stdin, as stdcopy: on stdout
+	var hdr [8]byte
+	chunk := make([]byte, 32<<10)
+	for {
+		if _, err := io.ReadFull(r, hdr[:]); err != nil {
+			if err == io.EOF {
+				break
+			}
+			return err
+		}
+		size := int64(binary.BigEndian.Uint32(hdr[4:]))
+		if hdr[0] == 3 { // Systemerr: an error of the daemon, as text
+			msg, _ := io.ReadAll(io.LimitReader(r, min(size, 4<<10)))
+			return fmt.Errorf("%w : %s", ErrUnavailable, msg)
+		}
+		b, ok := bufs[hdr[0]]
+		if !ok {
+			return fmt.Errorf("%w : type de trame de logs inconnu %d", ErrUnavailable, hdr[0])
+		}
+		// The payload is read in chunks: only the head (the timestamp) and
+		// what fits in max is kept.
+		body := io.LimitReader(r, size)
+		first := true
+		for {
+			n, err := io.ReadFull(body, chunk)
+			if n > 0 {
+				if err := b.take(chunk[:n], first, emit); err != nil {
+					return err
+				}
+				first = false
+			}
+			if err == io.EOF || err == io.ErrUnexpectedEOF {
+				break
+			}
+			if err != nil {
+				return err
+			}
+		}
+		if n, _ := body.(*io.LimitedReader); n.N > 0 {
+			return io.ErrUnexpectedEOF // the stream ended inside a frame
+		}
+	}
+	for _, k := range []byte{1, 2} {
+		if err := bufs[k].flush(emit); err != nil {
+			return err
+		}
+	}
+	return nil
+}
+
+// take adds a piece of a frame's payload; head: the frame starts with it
+// (and with a timestamp, repeated by every part of a long line). A "\n"
+// ends the line: the rest of the frame is a new line of the same time.
+func (b *lineBuf) take(p []byte, head bool, emit func(LogLine) error) error {
+	if head {
+		ts, rest := splitTS(p)
+		if !b.open {
+			b.ts = ts
+		}
+		p = rest
+	}
+	for len(p) > 0 {
+		b.open = true
+		i := bytes.IndexByte(p, '\n')
+		if i < 0 {
+			b.add(p)
+			return nil
+		}
+		b.add(p[:i])
+		ts := b.ts
+		if err := b.flush(emit); err != nil {
+			return err
+		}
+		b.ts, p = ts, p[i+1:]
+	}
+	return nil
+}
+
+// readRaw reads a TTY stream: "<timestamp> <text>\r\n" per line; a long
+// line comes as parts of dockerPartial bytes, each behind the same
+// timestamp, glued together: that timestamp is removed exactly there. It
+// reads byte by byte from a buffer and never waits for more bytes than the
+// stream has sent: a live line is emitted as soon as its "\n" arrives.
+func readRaw(r *bufio.Reader, max int, emit func(LogLine) error) error {
+	b := &lineBuf{max: max, stream: "stdout"}
+	var head []byte   // start of a line, until its timestamp is known
+	var tsText []byte // the timestamp of the current line, as written, with its space
+	part := 0         // bytes of the current part
+	match := -1       // at a part boundary: bytes of tsText matched so far
+	atStart := true
+	feed := func(c byte) error {
+		b.open = true
+		if c == '\n' {
+			atStart = true
+			return b.flush(emit)
+		}
+		b.add([]byte{c})
+		if part++; part == dockerPartial && len(tsText) > 0 {
+			match = 0
+		}
+		return nil
+	}
+	feedAll := func(p []byte) error {
+		for _, c := range p {
+			if err := feed(c); err != nil {
+				return err
+			}
+		}
+		return nil
+	}
+	for {
+		c, err := r.ReadByte()
+		if err == io.EOF {
+			if err := feedAll(head); err != nil {
+				return err
+			}
+			if match > 0 {
+				if err := feedAll(tsText[:match]); err != nil {
+					return err
+				}
+			}
+			return b.flush(emit)
+		}
+		if err != nil {
+			return err
+		}
+		switch {
+		case atStart:
+			head = append(head, c)
+			if c != ' ' && c != '\n' && len(head) < 64 {
+				continue
+			}
+			atStart, b.open, part, tsText = false, true, 0, tsText[:0]
+			if ts, rest := splitTS(head); len(rest) == 0 && c == ' ' {
+				b.ts, tsText = ts, append(tsText, head...)
+			} else {
+				b.ts = time.Time{}
+				if err := feedAll(head); err != nil {
+					return err
+				}
+			}
+			head = head[:0]
+		case match >= 0:
+			if c == tsText[match] {
+				if match++; match == len(tsText) {
+					match, part = -1, 0
+				}
+				continue
+			}
+			pending := tsText[:match]
+			match = -1
+			if err := feedAll(pending); err != nil {
+				return err
+			}
+			if err := feed(c); err != nil {
+				return err
+			}
+		default:
+			if err := feed(c); err != nil {
+				return err
+			}
+		}
+	}
+}
```

- [ ] **Step 4: Vérifier**

Run: `go test -race ./internal/dockerx/ 2>&1 | tail -n 3 && DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration ./internal/dockerx/ -run LogsRealDocker -v 2>&1 | grep -E '^(---|ok|FAIL)' | tail -n 5`
Expected: `ok` ; `--- PASS: TestLogsRealDocker` (stdout et stderr séparés, ligne de 40 000 octets recollée et coupée à 16 Kio, TTY sur stdout, `Since` inclusif, `Tail 1`, `--log-driver none` → `ErrNoLogs`). Mutation : supprimer `if part++; part == dockerPartial && len(tsText) > 0 { match = 0 }` (garder `part++`) → `TestReadLogsTTYPartialMessages` échoue (« ligne de 16 Kio exactement »).

- [ ] **Step 5: Commit**

```bash
git add internal/dockerx
git commit -F - <<'EOF'
dockerx : Logs, lecteur des flux multiplexé et TTY, lignes longues recollées et plafonnées

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 3: Routes `GET /logs` (SSE) et `GET /logs/download`, arrêt propre de l'agent

**Où tester :** macOS (`httptest` et un vrai serveur HTTP local, faux Docker).

**Files:**
- Create: `internal/api/logs.go`, `internal/api/logs_test.go`
- Modify: `internal/api/server.go`, `internal/api/fserr.go`, `cmd/hublot/main.go`

**Interfaces:**
- Consumes: `dockerx.Client.Logs`, `dockerx.ReadLogs`, `dockerx.MaxLogLine`, `dockerx.ErrNoLogs`, `fsErrorStatus`, `writeFSError`, `apiError`, `containerRef`, `fsTimeout`, `requireSession`.
- Produces: `type logLimits struct{ MaxStreams, Burst int; Rate float64; DefaultTail, MaxTail int; Heartbeat time.Duration }`, `var defaultLogLimits` (lu par `New` : les tests le réduisent) ; `(*server).inspectByRef(r *http.Request) (dockerx.ContainerInfo, error)` (réutilisé par la tâche 5) ; `type streamSlots`, `newStreamSlots(max int) *streamSlots`, `(*streamSlots).take(live bool) (release func(), evicted <-chan struct{}, err error)` ; `(*server).sessionValid(r *http.Request) bool` ; `sseWriteTimeout = 30 * time.Second` ; `server.logLimits`, `server.streams` ; `errTooManyStreams` ; événements `evicted` et `failure` `unauthorized` ; routes `GET /api/containers/{id}/logs` et `/logs/download` ; code `logs_unavailable` (422) dans `fsErrorStatus`.

> **À comprendre :** un flux SSE est une réponse qui ne finit pas : `text/event-stream`, puis des blocs `champ: valeur` séparés par une ligne vide, poussés par `Flush`. `EventSource` se reconnecte seul et renvoie le dernier `id` reçu dans `Last-Event-ID` ; il ne sait pas lire une réponse d'erreur HTTP, d'où les erreurs dans le flux (Q29). Une goroutine lit Docker et pousse les lignes dans un canal ; la boucle principale attend une ligne, le battement ou la fin de la requête. En partant, elle annule le contexte, ferme le flux de Docker, vide le canal (la goroutine se termine) et seulement alors rend le créneau (Q33). Le seau de débit (Q31) compte les lignes refusées ; elles avancent quand même le rang. Le battement fait deux choses : `: ping`, et la relecture de la session (R3) ; une session finie ferme le flux par `failure` `unauthorized`. Chaque écriture pose un délai de 30 s (`SetWriteDeadline`, R4). Les créneaux (`streamSlots`) se comptent par flux ; quand ils sont pris, le flux SSE le plus ancien est évincé : son canal `evicted` se ferme, il envoie `event: evicted` et rend la main (Q33, Q37). Côté `main`, `srv.Shutdown` attendait des flux qui ne finissent jamais : l'arrêt annule désormais le contexte des requêtes.

- [ ] **Step 1: Tests qui échouent** — un vrai serveur HTTP lit le flux événement par événement, comme un navigateur :

```diff
diff --git a/internal/api/logs_test.go b/internal/api/logs_test.go
new file mode 100644
--- /dev/null
+++ b/internal/api/logs_test.go
@@ -0,0 +1,638 @@
+package api
+
+// Tests of the logs routes (F6, spec §6), written before the code. The fake
+// Docker serves raw logs streams (multiplexed frames, or raw with a TTY);
+// LogsGate keeps a stream open like a running container. A real HTTP
+// server reads the SSE stream as a browser would, event by event.
+
+import (
+	"bufio"
+	"context"
+	"encoding/binary"
+	"encoding/json"
+	"errors"
+	"io"
+	"net/http"
+	"net/http/httptest"
+	"slices"
+	"strings"
+	"sync/atomic"
+	"testing"
+	"time"
+
+	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+)
+
+const (
+	lts1 = "2026-10-10T07:14:22.769046552Z"
+	lts2 = "2026-10-10T07:14:23.775474053Z"
+	lts3 = "2026-10-10T07:14:24.000000001Z"
+)
+
+// mux is one frame of Docker's multiplexed logs stream.
+func mux(stream byte, payload string) []byte {
+	h := make([]byte, 8, 8+len(payload))
+	h[0] = stream
+	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
+	return append(h, payload...)
+}
+
+func muxAll(fs ...[]byte) []byte { return slices.Concat(fs...) }
+
+// sseEvent is one event of the stream; a comment line is Comment.
+type sseEvent struct {
+	ID, Event, Data, Comment string
+}
+
+// logEvent is the data of a line event.
+type logEvent struct {
+	TS        string `json:"ts"`
+	Stream    string `json:"stream"`
+	Line      string `json:"line"`
+	Truncated bool   `json:"truncated"`
+}
+
+// sseClient opens path on a real server with the session cookie; next
+// reads one event (or comment), failing after 5 s.
+type sseClient struct {
+	res    *http.Response
+	r      *bufio.Reader
+	cancel context.CancelFunc
+}
+
+func openSSE(t *testing.T, f *fixture, path string, c *http.Cookie, headers map[string]string) *sseClient {
+	t.Helper()
+	srv := httptest.NewServer(f.h)
+	t.Cleanup(srv.Close)
+	ctx, cancel := context.WithCancel(context.Background())
+	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+path, nil)
+	if c != nil {
+		req.AddCookie(c)
+	}
+	for k, v := range headers {
+		req.Header.Set(k, v)
+	}
+	res, err := srv.Client().Do(req)
+	if err != nil {
+		cancel()
+		t.Fatal(err)
+	}
+	t.Cleanup(func() { cancel(); res.Body.Close() })
+	return &sseClient{res: res, r: bufio.NewReader(res.Body), cancel: cancel}
+}
+
+func (s *sseClient) next(t *testing.T) (sseEvent, bool) {
+	t.Helper()
+	type result struct {
+		e  sseEvent
+		ok bool
+	}
+	done := make(chan result, 1)
+	go func() {
+		var e sseEvent
+		for {
+			line, err := s.r.ReadString('\n')
+			if err != nil {
+				done <- result{e, false}
+				return
+			}
+			line = strings.TrimSuffix(line, "\n")
+			switch {
+			case line == "":
+				if e != (sseEvent{}) {
+					done <- result{e, true}
+					return
+				}
+			case strings.HasPrefix(line, ":"):
+				done <- result{sseEvent{Comment: strings.TrimSpace(line[1:])}, true}
+				return
+			case strings.HasPrefix(line, "id: "):
+				e.ID = line[4:]
+			case strings.HasPrefix(line, "event: "):
+				e.Event = line[7:]
+			case strings.HasPrefix(line, "data: "):
+				e.Data = line[6:]
+			case strings.HasPrefix(line, "retry: "):
+			default:
+				done <- result{sseEvent{Event: "MALFORMED", Data: line}, true}
+				return
+			}
+		}
+	}()
+	select {
+	case r := <-done:
+		return r.e, r.ok
+	case <-time.After(5 * time.Second):
+		t.Fatal("aucun événement en 5 s")
+		return sseEvent{}, false
+	}
+}
+
+// all reads the events up to the end of the stream, comments left out.
+func (s *sseClient) all(t *testing.T) []sseEvent {
+	t.Helper()
+	var out []sseEvent
+	for {
+		e, ok := s.next(t)
+		if !ok {
+			return out
+		}
+		if e.Comment == "" {
+			out = append(out, e)
+		}
+	}
+}
+
+func lineOf(t *testing.T, e sseEvent) logEvent {
+	t.Helper()
+	if e.Event != "" {
+		t.Fatalf("événement %q, attendu une ligne : %+v", e.Event, e)
+	}
+	var l logEvent
+	if err := json.Unmarshal([]byte(e.Data), &l); err != nil {
+		t.Fatalf("data %q : %v", e.Data, err)
+	}
+	return l
+}
+
+func failureOf(t *testing.T, events []sseEvent) string {
+	t.Helper()
+	last := events[len(events)-1]
+	if last.Event != "failure" {
+		t.Fatalf("dernier événement %+v, attendu failure", last)
+	}
+	var body struct{ Error string }
+	json.Unmarshal([]byte(last.Data), &body)
+	return body.Error
+}
+
+// logsFixture: the fake "web" container with a stopped container's logs.
+func logsFixture(t *testing.T) (*fixture, *http.Cookie) {
+	t.Helper()
+	f := newFixture(t, false)
+	f.docker.LogStreams = map[string][]byte{webID: muxAll(
+		mux(1, lts1+" out1\n"),
+		mux(2, lts1+" err1\n"),
+		mux(1, lts2+" out2\n"),
+	)}
+	return f, f.session(t)
+}
+
+// smallLimits shrinks the log limits for one test.
+func smallLimits(t *testing.T, change func(*logLimits)) {
+	t.Helper()
+	old := defaultLogLimits
+	t.Cleanup(func() { defaultLogLimits = old })
+	change(&defaultLogLimits)
+}
+
+func TestLogsNeedSession(t *testing.T) {
+	f, _ := logsFixture(t)
+	for _, p := range []string{"/api/containers/web/logs", "/api/containers/web/logs/download"} {
+		if rec := f.do("GET", p, "", nil); rec.Code != http.StatusUnauthorized {
+			t.Errorf("%s sans session : %d", p, rec.Code)
+		}
+	}
+	if len(f.docker.LogsCalls()) != 0 {
+		t.Fatal("Docker appelé sans session")
+	}
+}
+
+// A stopped container: its past logs, then "end"; the stream closes and
+// the browser must not reconnect (the UI closes it on "end").
+func TestLogsStreamPastLines(t *testing.T) {
+	f, c := logsFixture(t)
+	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	if ct := s.res.Header.Get("Content-Type"); s.res.StatusCode != 200 || ct != "text/event-stream" {
+		t.Fatalf("statut %d, Content-Type %q", s.res.StatusCode, ct)
+	}
+	if s.res.Header.Get("Cache-Control") != "no-store" || s.res.Header.Get("X-Accel-Buffering") != "no" {
+		t.Errorf("en-têtes : %v", s.res.Header)
+	}
+	events := s.all(t)
+	if len(events) != 4 || events[3].Event != "end" {
+		t.Fatalf("événements : %+v", events)
+	}
+	want := []logEvent{{lts1, "stdout", "out1", false}, {lts1, "stderr", "err1", false}, {lts2, "stdout", "out2", false}}
+	for i, w := range want {
+		if got := lineOf(t, events[i]); got != w {
+			t.Errorf("ligne %d : %+v, attendu %+v", i, got, w)
+		}
+	}
+	// The ID of a line: its timestamp, and its rank among the lines of the
+	// same timestamp (resumption, spec §3).
+	if ids := []string{events[0].ID, events[1].ID, events[2].ID}; !slices.Equal(ids, []string{lts1 + "/0", lts1 + "/1", lts2 + "/0"}) {
+		t.Errorf("identifiants : %q", ids)
+	}
+	if calls := f.docker.LogsCalls(); !slices.Equal(calls, []dockerx.LogsOptions{{Tail: "500", Follow: true}}) {
+		t.Errorf("appels : %+v", calls)
+	}
+}
+
+func TestLogsTTY(t *testing.T) {
+	f, c := logsFixture(t)
+	tty := dockerx.ContainerInfo{ID: strings.Repeat("7", 64), Name: "tty", Running: true, Pid: 7, Tty: true}
+	f.docker.Infos["tty"] = tty
+	f.docker.LogStreams[tty.ID] = []byte(lts1 + " un\r\n" + lts2 + " deux\r\n")
+	events := openSSE(t, f, "/api/containers/tty/logs", c, nil).all(t)
+	if len(events) != 3 || lineOf(t, events[0]).Line != "un" || lineOf(t, events[1]).Stream != "stdout" {
+		t.Fatalf("TTY : %+v", events)
+	}
+}
+
+func TestLogsTail(t *testing.T) {
+	for q, want := range map[string]string{
+		"": "500", "?tail=20": "20", "?tail=0": "0", "?tail=99999": "10000", "?tail=abc": "500", "?tail=-1": "500",
+	} {
+		f, c := logsFixture(t)
+		openSSE(t, f, "/api/containers/web/logs"+q, c, nil).all(t)
+		if calls := f.docker.LogsCalls(); len(calls) != 1 || calls[0].Tail != want || calls[0].Since != "" {
+			t.Errorf("%q : %+v, attendu Tail %s", q, calls, want)
+		}
+	}
+}
+
+// Resumption after a cut (browser: Last-Event-ID; UI after a pause:
+// ?after=): Docker's since is inclusive, so the lines of the same timestamp
+// already sent are skipped by their rank.
+func TestLogsResume(t *testing.T) {
+	stream := muxAll(mux(1, lts2+" a\n"), mux(1, lts2+" b\n"), mux(2, lts3+" c\n"))
+	for name, h := range map[string]map[string]string{
+		"Last-Event-ID": {"Last-Event-ID": lts2 + "/0"},
+		"after":         nil,
+		"les deux":      {"Last-Event-ID": lts2 + "/0"},
+	} {
+		f, c := logsFixture(t)
+		f.docker.LogStreams[webID] = stream
+		path := "/api/containers/web/logs"
+		switch name {
+		case "after":
+			path += "?after=" + lts2 + "/0"
+		case "les deux":
+			path += "?after=" + lts1 + "/0" // the header, newer, wins
+		}
+		events := openSSE(t, f, path, c, h).all(t)
+		if len(events) != 3 || lineOf(t, events[0]).Line != "b" || events[0].ID != lts2+"/1" ||
+			lineOf(t, events[1]).Line != "c" || events[1].ID != lts3+"/0" {
+			t.Errorf("%s : %+v", name, events)
+		}
+		if calls := f.docker.LogsCalls(); len(calls) != 1 || calls[0].Since != lts2 || calls[0].Tail != "all" {
+			t.Errorf("%s : appels %+v", name, calls)
+		}
+	}
+	// A malformed ID is ignored: an ordinary start.
+	f, c := logsFixture(t)
+	openSSE(t, f, "/api/containers/web/logs?after=hier/0", c, nil).all(t)
+	if calls := f.docker.LogsCalls(); calls[0].Since != "" || calls[0].Tail != "500" {
+		t.Errorf("identifiant invalide : %+v", calls)
+	}
+}
+
+// Errors come inside the stream (EventSource cannot read an HTTP error),
+// as a "failure" event with the code of spec §6, then the stream closes.
+func TestLogsFailures(t *testing.T) {
+	cases := []struct {
+		name, path string
+		set        func(*dockerx.Fake)
+		code       string
+	}{
+		{"conteneur absent", "/api/containers/absent/logs", nil, "not_found"},
+		{"identifiant invalide", "/api/containers/..x/logs", nil, "not_found"},
+		{"pilote sans lecture", "/api/containers/web/logs", func(d *dockerx.Fake) { d.LogsErr = dockerx.ErrNoLogs }, "logs_unavailable"},
+		{"Docker indisponible", "/api/containers/web/logs", func(d *dockerx.Fake) { d.Err = dockerx.ErrUnavailable }, "docker_unavailable"},
+		{"erreur du démon en cours de flux", "/api/containers/web/logs", func(d *dockerx.Fake) {
+			d.LogStreams[webID] = muxAll(mux(1, lts1+" avant\n"), mux(3, "log file rotated"))
+		}, "docker_unavailable"},
+	}
+	for _, k := range cases {
+		f, c := logsFixture(t)
+		if k.set != nil {
+			k.set(f.docker)
+		}
+		s := openSSE(t, f, k.path, c, nil)
+		events := s.all(t)
+		if s.res.StatusCode != 200 || len(events) == 0 {
+			t.Errorf("%s : statut %d, %+v", k.name, s.res.StatusCode, events)
+			continue
+		}
+		if code := failureOf(t, events); code != k.code {
+			t.Errorf("%s : %s, attendu %s", k.name, code, k.code)
+		}
+	}
+}
+
+// Q33: the client leaves (tab closed, navigation): Docker's stream is closed,
+// even while the container keeps running. The slot itself: TestStreamSlots.
+func TestLogsClientGone(t *testing.T) {
+	f, c := logsFixture(t)
+	f.docker.LogsGate = make(chan struct{}) // running: the stream never ends
+	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	if e, _ := s.next(t); lineOf(t, e).Line != "out1" {
+		t.Fatalf("premier événement : %+v", e)
+	}
+	s.cancel()
+	deadline := time.Now().Add(2 * time.Second)
+	for f.docker.LogsCloses.Load() != 1 && time.Now().Before(deadline) {
+		time.Sleep(10 * time.Millisecond)
+	}
+	if f.docker.LogsCloses.Load() != 1 {
+		t.Fatal("le flux de Docker n'est pas fermé après le départ du client")
+	}
+}
+
+// Q33, Q37: slots are counted per stream; when they are all taken, the
+// oldest live stream gives its slot (a client gone without FIN keeps one
+// for ~15 min, and its EventSource reconnects on a new connection). A
+// download is never evicted; with only downloads, the newcomer is refused.
+func TestStreamSlots(t *testing.T) {
+	s := newStreamSlots(2)
+	relA, evA, err := s.take(true)
+	if err != nil {
+		t.Fatal(err)
+	}
+	relD, evD, _ := s.take(false)
+	relB, evB, err := s.take(true) // full: A, the oldest live stream, goes
+	if err != nil {
+		t.Fatal(err)
+	}
+	select {
+	case <-evA:
+	default:
+		t.Fatal("A doit être évincé")
+	}
+	for name, ev := range map[string]<-chan struct{}{"téléchargement": evD, "B": evB} {
+		select {
+		case <-ev:
+			t.Errorf("%s évincé à tort", name)
+		default:
+		}
+	}
+	relA() // the evicted stream returns: its slot is not counted twice
+	relA()
+	if _, _, err := s.take(false); err != nil { // B is evicted for it
+		t.Fatalf("téléchargement : %v", err)
+	}
+	if _, _, err := s.take(true); !errors.Is(err, errTooManyStreams) { // two downloads: refused
+		t.Fatalf("plein de téléchargements : %v", err)
+	}
+	relB()
+	relD()
+	if _, _, err := s.take(true); err != nil {
+		t.Fatalf("après départ du téléchargement : %v", err)
+	}
+}
+
+// Q33, Q37: through the routes, the newest stream wins; the evicted one
+// is told ("evicted", so the UI does not reconnect) and closed.
+func TestLogsEvictsOldest(t *testing.T) {
+	smallLimits(t, func(l *logLimits) { l.MaxStreams = 2 })
+	f, c := logsFixture(t)
+	f.docker.LogsGate = make(chan struct{})
+	a := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	a.next(t)
+	b := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	b.next(t)
+	third := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	if e, _ := third.next(t); lineOf(t, e).Line != "out1" {
+		t.Fatalf("troisième flux : %+v", e)
+	}
+	var last sseEvent
+	for {
+		e, ok := a.next(t)
+		if !ok {
+			break
+		}
+		last = e
+	}
+	if last.Event != "evicted" {
+		t.Fatalf("le plus ancien doit être évincé : dernier événement %+v", last)
+	}
+	close(f.docker.LogsGate)
+	if events := b.all(t); events[len(events)-1].Event != "end" {
+		t.Fatalf("B ne doit pas être évincé : %+v", events)
+	}
+}
+
+// Downloads count in the slots: one evicts a live stream; with every slot
+// held by downloads, a new stream is refused, and says why.
+func TestLogsDownloadSlots(t *testing.T) {
+	smallLimits(t, func(l *logLimits) { l.MaxStreams = 1 })
+	f, c := logsFixture(t)
+	f.docker.LogsGate = make(chan struct{})
+	srv := httptest.NewServer(f.h)
+	t.Cleanup(srv.Close)
+	download := func() chan int {
+		done := make(chan int, 1)
+		go func() {
+			req, _ := http.NewRequest("GET", srv.URL+"/api/containers/web/logs/download", nil)
+			req.AddCookie(c)
+			res, err := srv.Client().Do(req)
+			if err != nil {
+				done <- 0
+				return
+			}
+			io.Copy(io.Discard, res.Body)
+			res.Body.Close()
+			done <- res.StatusCode
+		}()
+		return done
+	}
+	waitCalls := func(n int) {
+		deadline := time.Now().Add(2 * time.Second)
+		for len(f.docker.LogsCalls()) < n && time.Now().Before(deadline) {
+			time.Sleep(5 * time.Millisecond)
+		}
+	}
+	a := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	a.next(t)
+	d := download() // takes A's slot
+	waitCalls(2)
+	if events := a.all(t); events[len(events)-1].Event != "evicted" {
+		t.Fatalf("flux évincé par un téléchargement : %+v", events)
+	}
+	refused := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if failureOf(t, refused) != "internal" || !strings.Contains(refused[len(refused)-1].Data, "flux de logs") {
+		t.Fatalf("créneaux pris par des téléchargements : %+v", refused)
+	}
+	close(f.docker.LogsGate)
+	if code := <-d; code != http.StatusOK {
+		t.Fatalf("téléchargement : %d", code)
+	}
+}
+
+// Review I3: logging out, or the session expiring, ends the streams it
+// opened: the next heartbeat sees it, says "unauthorized" and closes.
+func TestLogsSessionEnds(t *testing.T) {
+	smallLimits(t, func(l *logLimits) { l.Heartbeat = 20 * time.Millisecond })
+	f, c := logsFixture(t)
+	f.docker.LogsGate = make(chan struct{})
+	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	s.next(t)
+	if rec := f.do("POST", "/api/logout", "", c); rec.Code != http.StatusNoContent {
+		t.Fatalf("déconnexion : %d", rec.Code)
+	}
+	var last sseEvent
+	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); {
+		e, ok := s.next(t) // a ping every 20 ms: the loop stays bounded
+		if !ok {
+			break
+		}
+		if e.Comment == "" {
+			last = e
+		}
+	}
+	if last.Event != "failure" || failureOf(t, []sseEvent{last}) != "unauthorized" {
+		t.Fatalf("flux encore ouvert 2 s après la déconnexion, dernier événement %+v", last)
+	}
+	deadline := time.Now().Add(2 * time.Second)
+	for f.docker.LogsCloses.Load() != 1 && time.Now().Before(deadline) {
+		time.Sleep(10 * time.Millisecond)
+	}
+	if f.docker.LogsCloses.Load() != 1 {
+		t.Fatal("flux de Docker resté ouvert après la déconnexion")
+	}
+}
+
+// deadlineWriter counts the write deadlines the handler sets (through
+// http.ResponseController), then passes them on.
+type deadlineWriter struct {
+	http.ResponseWriter
+	n *atomic.Int32
+}
+
+func (d deadlineWriter) SetWriteDeadline(t time.Time) error {
+	d.n.Add(1)
+	return http.NewResponseController(d.ResponseWriter).SetWriteDeadline(t)
+}
+
+func (d deadlineWriter) Unwrap() http.ResponseWriter { return d.ResponseWriter }
+
+// Review I4: every write of a stream, and of a download, has a deadline: a
+// client that no longer reads cannot hold the goroutine nor the slot.
+func TestLogsWriteDeadline(t *testing.T) {
+	f, c := logsFixture(t)
+	n := new(atomic.Int32)
+	h := f.h
+	f.h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(deadlineWriter{w, n}, r) })
+	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if len(events) != 4 || n.Load() < int32(len(events)) {
+		t.Fatalf("%d délais pour %d événements", n.Load(), len(events))
+	}
+	// Review 2, Minor 3: the download too (it is never evicted).
+	n.Store(0)
+	if rec := f.do("GET", "/api/containers/web/logs/download", "", c); rec.Code != http.StatusOK || n.Load() == 0 {
+		t.Fatalf("téléchargement : %d, %d délais", rec.Code, n.Load())
+	}
+}
+
+// Q31: beyond the burst, at most Rate lines per second; the lines left out
+// are counted and announced ("dropped"), never silently lost.
+func TestLogsRateLimit(t *testing.T) {
+	smallLimits(t, func(l *logLimits) {
+		l.Burst, l.Rate = 3, 0
+	})
+	f, c := logsFixture(t)
+	var frames [][]byte
+	for i := 0; i < 5; i++ {
+		frames = append(frames, mux(1, lts1+" x\n"))
+	}
+	f.docker.LogStreams[webID] = muxAll(frames...)
+	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if len(events) != 5 || events[3].Event != "dropped" || events[3].Data != `{"count":2}` || events[4].Event != "end" {
+		t.Fatalf("événements : %+v", events)
+	}
+	// The ranks go on through the dropped lines: a resumption skips them.
+	if events[2].ID != lts1+"/2" {
+		t.Errorf("identifiant : %q", events[2].ID)
+	}
+}
+
+// Codex: resuming after lines the limiter dropped, with shared timestamps:
+// the lines sent are never sent again; the dropped ones come back (they
+// were only announced, never shown).
+func TestLogsResumeAfterDrops(t *testing.T) {
+	smallLimits(t, func(l *logLimits) { l.Burst, l.Rate = 2, 0 })
+	f, c := logsFixture(t)
+	f.docker.LogStreams[webID] = muxAll(mux(1, lts1+" a\n"), mux(2, lts1+" b\n"), mux(1, lts1+" c\n"), mux(1, lts2+" d\n"))
+	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if len(events) != 4 || events[1].ID != lts1+"/1" || events[2].Data != `{"count":2}` {
+		t.Fatalf("premier flux : %+v", events)
+	}
+	events = openSSE(t, f, "/api/containers/web/logs", c, map[string]string{"Last-Event-ID": events[1].ID}).all(t)
+	if len(events) != 3 || lineOf(t, events[0]).Line != "c" || events[0].ID != lts1+"/2" ||
+		lineOf(t, events[1]).Line != "d" || events[1].ID != lts2+"/0" || events[2].Event != "end" {
+		t.Fatalf("reprise : %+v", events)
+	}
+}
+
+// A silent stream gets a comment line now and then: proxies keep it open,
+// and a gone client is noticed.
+func TestLogsHeartbeat(t *testing.T) {
+	smallLimits(t, func(l *logLimits) { l.Heartbeat = 20 * time.Millisecond })
+	f, c := logsFixture(t)
+	f.docker.LogStreams[webID] = nil
+	f.docker.LogsGate = make(chan struct{})
+	s := openSSE(t, f, "/api/containers/web/logs", c, nil)
+	if e, _ := s.next(t); e.Comment != "ping" {
+		t.Fatalf("attendu un commentaire ping : %+v", e)
+	}
+}
+
+// A line is JSON in one data field: no raw CR or LF that would cut the
+// event, no markup that would matter (the UI escapes anyway).
+func TestLogsEventEscaping(t *testing.T) {
+	f, c := logsFixture(t)
+	f.docker.LogStreams[webID] = mux(1, lts1+" a\rb </script>\xff\n")
+	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if len(events) != 2 || events[0].Event == "MALFORMED" {
+		t.Fatalf("événements : %+v", events)
+	}
+	if l := lineOf(t, events[0]); l.Line != "a\rb </script>�" {
+		t.Fatalf("ligne : %q", l.Line)
+	}
+}
+
+func TestLogsDownload(t *testing.T) {
+	f, c := logsFixture(t)
+	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
+	if rec.Code != http.StatusOK {
+		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
+	}
+	h := rec.Header()
+	if h.Get("Content-Type") != "application/octet-stream" ||
+		!strings.HasPrefix(h.Get("Content-Disposition"), `attachment; filename="web-`) ||
+		!strings.HasSuffix(h.Get("Content-Disposition"), `.log"`) {
+		t.Errorf("en-têtes : %v", h)
+	}
+	want := lts1 + " stdout out1\n" + lts1 + " stderr err1\n" + lts2 + " stdout out2\n"
+	if rec.Body.String() != want {
+		t.Errorf("corps %q, attendu %q", rec.Body, want)
+	}
+	if calls := f.docker.LogsCalls(); !slices.Equal(calls, []dockerx.LogsOptions{{Tail: "all"}}) {
+		t.Errorf("appels : %+v", calls)
+	}
+	// Errors before the first byte are ordinary API errors.
+	f.docker.LogsErr = dockerx.ErrNoLogs
+	if rec := f.do("GET", "/api/containers/web/logs/download", "", c); rec.Code != http.StatusUnprocessableEntity || errCode(t, rec) != "logs_unavailable" {
+		t.Errorf("pilote sans lecture : %d %s", rec.Code, rec.Body)
+	}
+	if rec := f.do("GET", "/api/containers/absent/logs/download", "", c); rec.Code != http.StatusNotFound {
+		t.Errorf("absent : %d", rec.Code)
+	}
+}
+
+// The download keeps a long line whole up to maxDownloadLine, the live
+// view cuts it at dockerx.MaxLogLine.
+func TestLogsLongLine(t *testing.T) {
+	f, c := logsFixture(t)
+	long := strings.Repeat("a", dockerx.MaxLogLine+10)
+	f.docker.LogStreams[webID] = mux(1, lts1+" "+long+"\n")
+	events := openSSE(t, f, "/api/containers/web/logs", c, nil).all(t)
+	if l := lineOf(t, events[0]); len(l.Line) != dockerx.MaxLogLine || !l.Truncated {
+		t.Errorf("vue en direct : %d octets, tronquée %v", len(l.Line), l.Truncated)
+	}
+	rec := f.do("GET", "/api/containers/web/logs/download", "", c)
+	if !strings.Contains(rec.Body.String(), " stdout "+long+"\n") {
+		t.Errorf("téléchargement : %d octets", rec.Body.Len())
+	}
+}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go vet ./internal/api/ 2>&1 | tail -n 3`
Expected: échec de compilation (`undefined: logLimits`).

- [ ] **Step 3: Implémenter**

```diff
diff --git a/cmd/hublot/main.go b/cmd/hublot/main.go
--- a/cmd/hublot/main.go
+++ b/cmd/hublot/main.go
@@ -84,6 +84,12 @@ func run(ctx context.Context, args []string, getenv func(string) string, stdout,
 	}
 
 	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second}
+	// A logs stream (SSE, F6) never ends by itself: Shutdown cancels the
+	// context of every request, so the streams return, then waits for them.
+	base, cancelBase := context.WithCancel(context.Background())
+	defer cancelBase()
+	srv.BaseContext = func(net.Listener) context.Context { return base }
+	srv.RegisterOnShutdown(cancelBase)
 	if cfg.TLS() {
 		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
 		if err != nil {
diff --git a/internal/api/fserr.go b/internal/api/fserr.go
--- a/internal/api/fserr.go
+++ b/internal/api/fserr.go
@@ -38,12 +38,14 @@ func writeFSError(w http.ResponseWriter, err error) {
 // errors first (a missing socket is ENOENT too, it is not a missing file).
 func fsErrorStatus(err error) (int, string) {
 	switch {
-	case errors.Is(err, errBusy):
+	case errors.Is(err, errBusy), errors.Is(err, errTooManyStreams):
 		return http.StatusServiceUnavailable, "internal"
 	case errors.Is(err, errTimeout):
 		return http.StatusGatewayTimeout, "internal"
 	case errors.Is(err, dockerx.ErrNotFound):
 		return http.StatusNotFound, "not_found"
+	case errors.Is(err, dockerx.ErrNoLogs): // F6: --log-driver none, syslog… (Q34)
+		return http.StatusUnprocessableEntity, "logs_unavailable"
 	case errors.Is(err, files.ErrReadOnly), errors.Is(err, dockerx.ErrRefused): // Q15: Docker's read-only rootfs or volume
 		return http.StatusForbidden, "read_only"
 	case errors.Is(err, files.ErrConflict), errors.Is(err, dockerx.ErrPutInvalid): // the path changed after the checks
diff --git a/internal/api/logs.go b/internal/api/logs.go
new file mode 100644
--- /dev/null
+++ b/internal/api/logs.go
@@ -0,0 +1,409 @@
+package api
+
+import (
+	"bufio"
+	"context"
+	"encoding/json"
+	"errors"
+	"fmt"
+	"log/slog"
+	"net/http"
+	"regexp"
+	"slices"
+	"strconv"
+	"strings"
+	"sync"
+	"time"
+
+	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+)
+
+// logLimits of F6 (plan Observation, Q31 to Q33, Q37). Tests shrink them through
+// defaultLogLimits, read by New.
+type logLimits struct {
+	MaxStreams  int           // logs streams open at once for the agent, downloads included
+	Burst       int           // lines sent at once to one stream (the initial tail fits)
+	Rate        float64       // lines per second refilled after the burst
+	DefaultTail int           // lines of the past shown when the view opens
+	MaxTail     int           // most lines of the past the view may ask for
+	Heartbeat   time.Duration // a comment on a silent stream: proxies, gone clients
+}
+
+var defaultLogLimits = logLimits{
+	MaxStreams: 8, Burst: 10_000, Rate: 1000, DefaultTail: 500, MaxTail: 10_000, Heartbeat: 15 * time.Second,
+}
+
+// maxDownloadLine caps one line of the downloaded file: a line longer than
+// this ends with "[…]".
+const maxDownloadLine = 1 << 20
+
+// errTooManyStreams: every slot of logLimits.MaxStreams is taken by
+// downloads, which are never evicted.
+var errTooManyStreams = errors.New("trop de flux de logs ouverts")
+
+// sseWriteTimeout bounds each write of a stream (review I4): a client
+// that no longer reads cannot hold the goroutine.
+const sseWriteTimeout = 30 * time.Second
+
+// lineID is the SSE ID of a line: its timestamp and its rank among the
+// lines of that same timestamp. Docker's since is inclusive: resuming
+// asks from the timestamp and skips the ranks already seen.
+var lineID = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT[0-9:.]+Z)/(\d{1,9})$`)
+
+// resumePoint reads a line ID (Last-Event-ID first: the browser's own
+// reconnection, newer than the page's ?after=); ok false when none valid.
+func resumePoint(r *http.Request) (ts time.Time, rank int, ok bool) {
+	for _, id := range []string{r.Header.Get("Last-Event-ID"), r.URL.Query().Get("after")} {
+		m := lineID.FindStringSubmatch(id)
+		if m == nil {
+			continue
+		}
+		t, err := time.Parse(time.RFC3339Nano, m[1])
+		n, err2 := strconv.Atoi(m[2])
+		if err == nil && err2 == nil {
+			return t, n, true
+		}
+	}
+	return time.Time{}, 0, false
+}
+
+// tailOf: the tail parameter, DefaultTail when absent or invalid, at most
+// MaxTail.
+func (l logLimits) tailOf(r *http.Request) string {
+	n, err := strconv.Atoi(r.URL.Query().Get("tail"))
+	if err != nil || n < 0 {
+		n = l.DefaultTail
+	}
+	return strconv.Itoa(min(n, l.MaxTail))
+}
+
+// streamSlots: at most max logs streams at once (Q33, Q37). When every slot
+// is taken, the oldest live stream gives its slot to the newcomer: a client
+// gone without FIN keeps its connection for ~15 min (TCP retransmissions),
+// and its EventSource reconnects on a new one; the newest stream is almost
+// always the living one. A download is never evicted.
+type streamSlots struct {
+	mu   sync.Mutex
+	max  int
+	used int
+	live []*slot // oldest first
+}
+
+type slot struct {
+	evicted chan struct{} // closed on eviction
+	gone    bool          // evicted: its slot is no longer counted
+}
+
+func newStreamSlots(max int) *streamSlots { return &streamSlots{max: max} }
+
+// take reserves a slot (live: an SSE stream, evictable). release frees it,
+// once; evicted is closed if a newer stream takes the slot.
+func (s *streamSlots) take(live bool) (release func(), evicted <-chan struct{}, err error) {
+	s.mu.Lock()
+	defer s.mu.Unlock()
+	if s.used >= s.max {
+		if len(s.live) == 0 {
+			return nil, nil, fmt.Errorf("%w (%d au plus, tous des téléchargements)", errTooManyStreams, s.max)
+		}
+		old := s.live[0]
+		s.live, s.used, old.gone = s.live[1:], s.used-1, true
+		close(old.evicted)
+	}
+	s.used++
+	sl := &slot{evicted: make(chan struct{})}
+	if live {
+		s.live = append(s.live, sl)
+	}
+	var once sync.Once
+	return func() {
+		once.Do(func() {
+			s.mu.Lock()
+			defer s.mu.Unlock()
+			if sl.gone {
+				return
+			}
+			s.used--
+			if i := slices.Index(s.live, sl); i >= 0 {
+				s.live = slices.Delete(s.live, i, i+1)
+			}
+		})
+	}, sl.evicted, nil
+}
+
+// sessionValid: the session that opened the stream still exists (review
+// I3: logout and expiry end the streams at the next heartbeat).
+func (s *server) sessionValid(r *http.Request) bool {
+	c, err := r.Cookie(cookieName)
+	return err == nil && s.d.Sessions.Valid(c.Value)
+}
+
+// inspectByRef checks {id} and resolves the container by Inspect, bounded
+// like the file routes. Every later step uses info.ID, the full ID.
+func (s *server) inspectByRef(r *http.Request) (dockerx.ContainerInfo, error) {
+	id := r.PathValue("id")
+	if !containerRef.MatchString(id) {
+		return dockerx.ContainerInfo{}, fmt.Errorf("%w : identifiant de conteneur invalide", dockerx.ErrNotFound)
+	}
+	ctx, cancel := context.WithTimeout(r.Context(), fsTimeout)
+	defer cancel()
+	return s.d.Docker.Inspect(ctx, id)
+}
+
+// sse writes the events of one stream.
+type sse struct {
+	w http.ResponseWriter
+	f *http.ResponseController
+}
+
+// deadline: every write of a stream ends within sseWriteTimeout (a writer
+// that cannot set one, in tests, writes without).
+func (e sse) deadline() { e.f.SetWriteDeadline(time.Now().Add(sseWriteTimeout)) }
+
+func (e sse) event(name, id string, data any) error {
+	b, err := json.Marshal(data)
+	if err != nil {
+		return err
+	}
+	var sb strings.Builder
+	if id != "" {
+		sb.WriteString("id: " + id + "\n")
+	}
+	if name != "" {
+		sb.WriteString("event: " + name + "\n")
+	}
+	sb.WriteString("data: ")
+	sb.Write(b)
+	sb.WriteString("\n\n")
+	e.deadline()
+	if _, err := e.w.Write([]byte(sb.String())); err != nil {
+		return err
+	}
+	return e.f.Flush()
+}
+
+func (e sse) comment(text string) error {
+	e.deadline()
+	if _, err := e.w.Write([]byte(": " + text + "\n\n")); err != nil {
+		return err
+	}
+	return e.f.Flush()
+}
+
+// failure reports err in the stream with the code of spec §6.
+func (e sse) failure(err error) {
+	_, code := fsErrorStatus(err)
+	if code == "internal" || code == "docker_unavailable" {
+		slog.Warn("logs stream error", "code", code, "err", err)
+	}
+	e.event("failure", "", apiError{Error: code, Detail: err.Error()})
+}
+
+type logJSON struct {
+	TS        string `json:"ts,omitempty"`
+	Stream    string `json:"stream"`
+	Line      string `json:"line"`
+	Truncated bool   `json:"truncated,omitempty"`
+}
+
+// bucket: Burst lines at once, then Rate per second.
+type bucket struct {
+	tokens, max, rate float64
+	last              time.Time
+}
+
+func (b *bucket) take(now time.Time) bool {
+	b.tokens = min(b.max, b.tokens+now.Sub(b.last).Seconds()*b.rate)
+	b.last = now
+	if b.tokens < 1 {
+		return false
+	}
+	b.tokens--
+	return true
+}
+
+// logsStream is GET /logs (F6, spec §6): an SSE stream of the container's
+// logs, {ts, stream, line} per event. It answers 200 as soon as the session
+// passed; every error comes as a "failure" event (EventSource cannot read
+// an HTTP error), the end of the container's stream as "end".
+func (s *server) logsStream(w http.ResponseWriter, r *http.Request) {
+	h := w.Header()
+	h.Set("Content-Type", "text/event-stream")
+	h.Set("X-Accel-Buffering", "no") // nginx: no buffering of the stream
+	w.WriteHeader(http.StatusOK)
+	out := sse{w: w, f: http.NewResponseController(w)}
+	out.deadline()
+	out.f.Flush()
+	lim := s.logLimits
+
+	info, err := s.inspectByRef(r)
+	if err != nil {
+		out.failure(err)
+		return
+	}
+	release, evicted, err := s.streams.take(true)
+	if err != nil {
+		out.failure(err)
+		return
+	}
+	defer release()
+	opt := dockerx.LogsOptions{Tail: lim.tailOf(r), Follow: true}
+	since, skip, resume := resumePoint(r)
+	if resume {
+		opt = dockerx.LogsOptions{Tail: "all", Since: since.Format(time.RFC3339Nano), Follow: true}
+	}
+	ctx, cancel := context.WithCancel(r.Context())
+	defer cancel()
+	rc, err := s.d.Docker.Logs(ctx, info.ID, opt)
+	if err != nil {
+		out.failure(err)
+		return
+	}
+	lines := make(chan dockerx.LogLine, 64)
+	readErr := make(chan error, 1)
+	go func() {
+		defer close(lines)
+		readErr <- dockerx.ReadLogs(rc, info.Tty, dockerx.MaxLogLine, func(l dockerx.LogLine) error {
+			select {
+			case lines <- l:
+				return nil
+			case <-ctx.Done():
+				return ctx.Err()
+			}
+		})
+	}()
+	// Leaving (client gone, end, error): the reader stops, Docker's
+	// stream is closed, and only then the slot is free.
+	defer func() {
+		cancel()
+		rc.Close()
+		for range lines {
+		}
+	}()
+
+	tick := time.NewTicker(lim.Heartbeat)
+	defer tick.Stop()
+	b := &bucket{tokens: float64(lim.Burst), max: float64(lim.Burst), rate: lim.Rate, last: time.Now()}
+	var cur time.Time // timestamp of the last line read, and its rank
+	rank, dropped := -1, 0
+	flushDropped := func() error {
+		if dropped == 0 {
+			return nil
+		}
+		n := dropped
+		dropped = 0
+		return out.event("dropped", "", map[string]int{"count": n})
+	}
+	for {
+		select {
+		case <-r.Context().Done():
+			return
+		case <-evicted:
+			flushDropped()
+			out.event("evicted", "", apiError{Error: "internal", Detail: fmt.Sprintf("flux remplacé par un plus récent (%d flux de logs au plus)", lim.MaxStreams)})
+			return
+		case <-tick.C:
+			if !s.sessionValid(r) {
+				out.event("failure", "", apiError{Error: "unauthorized", Detail: "session terminée"})
+				return
+			}
+			if flushDropped() != nil || out.comment("ping") != nil {
+				return
+			}
+		case l, ok := <-lines:
+			if !ok {
+				if err := <-readErr; err != nil && r.Context().Err() == nil {
+					flushDropped()
+					out.failure(err)
+					return
+				}
+				if flushDropped() == nil {
+					out.event("end", "", struct{}{})
+				}
+				return
+			}
+			if l.TS.Equal(cur) {
+				rank++
+			} else {
+				cur, rank = l.TS, 0
+			}
+			if resume && l.TS.Equal(since) && rank <= skip {
+				continue // sent before the cut
+			}
+			if !b.take(time.Now()) {
+				dropped++
+				continue
+			}
+			if flushDropped() != nil {
+				return
+			}
+			id, ts := "", ""
+			if !l.TS.IsZero() {
+				ts = l.TS.Format(time.RFC3339Nano)
+				id = ts + "/" + strconv.Itoa(rank)
+			}
+			if out.event("", id, logJSON{TS: ts, Stream: l.Stream, Line: l.Text, Truncated: l.Truncated}) != nil {
+				return
+			}
+		}
+	}
+}
+
+// timedWriter puts a write deadline before each write of a download
+// (review 2, Minor 3): a download is never evicted, a client gone without
+// FIN must not keep its slot for ~15 min.
+type timedWriter struct {
+	w http.ResponseWriter
+	c *http.ResponseController
+}
+
+func (t timedWriter) Write(p []byte) (int, error) {
+	t.c.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
+	return t.w.Write(p)
+}
+
+// safeName keeps [A-Za-z0-9_.-] of a container name, for a file name.
+var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
+
+// logsDownload is GET /logs/download (F6): every line Docker keeps, as text,
+// "<ts> <stream> <line>" per line, as an attachment (spec §7). Errors before
+// the first byte are API errors; after it, the file is cut short.
+func (s *server) logsDownload(w http.ResponseWriter, r *http.Request) {
+	info, err := s.inspectByRef(r)
+	if err != nil {
+		writeFSError(w, err)
+		return
+	}
+	release, _, err := s.streams.take(false)
+	if err != nil {
+		writeFSError(w, err)
+		return
+	}
+	defer release()
+	rc, err := s.d.Docker.Logs(r.Context(), info.ID, dockerx.LogsOptions{Tail: "all"})
+	if err != nil {
+		writeFSError(w, err)
+		return
+	}
+	defer rc.Close()
+	name := unsafeName.ReplaceAllString(info.Name, "_") + "-" + time.Now().UTC().Format("20060102-150405") + ".log"
+	h := w.Header()
+	h.Set("Content-Type", "application/octet-stream")
+	h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
+	bw := bufio.NewWriterSize(timedWriter{w, http.NewResponseController(w)}, 64<<10)
+	err = dockerx.ReadLogs(rc, info.Tty, maxDownloadLine, func(l dockerx.LogLine) error {
+		if !l.TS.IsZero() {
+			bw.WriteString(l.TS.Format(time.RFC3339Nano) + " ")
+		}
+		bw.WriteString(l.Stream + " " + l.Text)
+		if l.Truncated {
+			bw.WriteString(" […]")
+		}
+		return bw.WriteByte('\n')
+	})
+	if err == nil {
+		err = bw.Flush()
+	}
+	if err != nil && r.Context().Err() == nil {
+		slog.Warn("logs download cut", "container", info.ID, "err", err)
+	}
+}
diff --git a/internal/api/server.go b/internal/api/server.go
--- a/internal/api/server.go
+++ b/internal/api/server.go
@@ -76,6 +76,9 @@ type server struct {
 	throttle *throttleLog
 	guard    *fsGuard
 	files    *files.Writer // the only writer (spec §7: read-only applied there)
+	// F6: limits of the logs streams, and one slot per stream open.
+	logLimits logLimits
+	streams   *streamSlots
 }
 
 // Spec §13 (FUSE): at most 4 file operations at once per container, each one
@@ -98,6 +101,8 @@ func New(d Deps) http.Handler {
 		guard:    newFSGuard(fsMaxPerContainer, fsTimeout),
 		files: &files.Writer{ReadOnly: d.ReadOnly, Docker: d.Docker, Audit: d.Audit,
 			Open: d.OpenFS, Forget: d.Views.Forget},
+		logLimits: defaultLogLimits,
+		streams:   newStreamSlots(defaultLogLimits.MaxStreams),
 	}
 	session := func(h http.HandlerFunc) http.Handler { return requireSession(d.Sessions, h) }
 
@@ -112,6 +117,8 @@ func New(d Deps) http.Handler {
 	mux.Handle("PUT /api/containers/{id}/fs/file", session(s.fsEdit))
 	mux.Handle("POST /api/containers/{id}/fs/upload", session(s.fsUpload))
 	mux.Handle("GET /api/containers/{id}/changes", session(s.changes))
+	mux.Handle("GET /api/containers/{id}/logs", session(s.logsStream))
+	mux.Handle("GET /api/containers/{id}/logs/download", session(s.logsDownload))
 	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
 		writeError(w, http.StatusNotFound, "not_found", "")
 	})
```

- [ ] **Step 4: Vérifier**

Run: `go test -race -count=20 ./internal/api/ -run Logs 2>&1 | tail -n 3 && go test ./cmd/... ./internal/... 2>&1 | tail -n 12`
Expected: `ok` (vingt passages sans course ni test instable). Mutations : retirer `rc.Close()` du `defer` de `logsStream` → `TestLogsClientGone` (« le flux de Docker n'est pas fermé après le départ du client ») ; neutraliser `sessionValid` → `TestLogsSessionEnds` (« flux encore ouvert 2 s après la déconnexion ») ; retirer `e.deadline()` de `event` → `TestLogsWriteDeadline` ; remplacer `if len(s.live) == 0 {` par `if true {` → `TestStreamSlots`, `TestLogsEvictsOldest`.

- [ ] **Step 5: Commit**

```bash
git add internal/api cmd/hublot/main.go
git commit -F - <<'EOF'
api : logs en direct (SSE) et téléchargement, plafonds de débit et de flux, arrêt propre de l'agent

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 4: Paquet `secrets` — le masquage de §7, tests d'abord

**Où tester :** macOS.

**Files:**
- Create: `internal/secrets/secrets.go`, `internal/secrets/secrets_test.go`

**Interfaces:**
- Consumes: rien (bibliothèque standard).
- Produces: `const Mask = "••••••"` ; `SecretName(name string) bool` ; `URLCredentials(v string) bool` ; `Masked(name, value string) bool` (nom secret, ou valeur où les règles de la commande trouvent un secret, Q36) ; `Line(s string) string` (un texte : ligne de shell, message de Docker) ; `type Var struct{ Name string; Value *string; Masked bool }` (JSON `{"name","value","masked"}`, `value` `null` si masquée) ; `Env(env []string) []Var` ; `Lookup(env []string, name string) (string, bool)` ; `Args(args []string) (out []string, masked bool)`.

> **À comprendre :** §7 masque une variable dont le **nom** contient l'un de neuf mots (sans tenir compte de la casse) ou dont la **valeur** contient une URL avec identifiants. La règle est une sous-chaîne : `KEYBOARD` et `COMPASS` sont masqués aussi, et c'est voulu (côté sûr ; la valeur se révèle). `Env` ne copie jamais une valeur masquée : `Value` reste `nil`, donc le JSON n'a rien à fuir. `Args` (Q27) travaille sur une copie et ne masque que des morceaux (`--password=••••••`, `postgres://••••••@db`), pour que la commande reste lisible ; une valeur peut être entre guillemets, ou cacher une autre affectation (`--env=DB_PASSWORD=x`). Les règles sont des heuristiques : d'où le repli fermé de `Line` (R1), jugé **localement** (R11, R12) : un mot de §7 dont la valeur, dans son propre mot, n'est pas masquée (drapeau, ou `=`/`:` après lui) masque l'argument entier ; un masque ailleurs dans la ligne ne compte pas ; un mot de §7 dans un chemin ou un mot ordinaire reste lisible, sinon la commande de Keycloak ou un `sh -c` qui lit `/run/secrets/…` disparaîtraient.

- [ ] **Step 1: Tests qui échouent**

```diff
diff --git a/internal/secrets/secrets_test.go b/internal/secrets/secrets_test.go
new file mode 100644
--- /dev/null
+++ b/internal/secrets/secrets_test.go
@@ -0,0 +1,212 @@
+package secrets
+
+// Tests of the masking of spec §7, written before the code (CLAUDE.md:
+// security tests first). The rule is server-side: no secret leaves the
+// agent unmasked unless revealed, one variable at a time, and audited.
+
+import (
+	"encoding/json"
+	"slices"
+	"strings"
+	"testing"
+)
+
+func TestSecretName(t *testing.T) {
+	for _, n := range []string{
+		"PASSWORD", "db_password", "MYSQL_ROOT_PASSWORD", "PASSWD", "PGPASS", "Passphrase",
+		"GITHUB_TOKEN", "api_key", "AWS_SECRET_ACCESS_KEY", "SECRET", "GOOGLE_APPLICATION_CREDENTIALS",
+		"PRIVATE_KEY_PATH", "SENTRY_DSN", "dsn",
+		"KEYBOARD", "COMPASS", // false positives the substring rule accepts (spec §7)
+	} {
+		if !SecretName(n) {
+			t.Errorf("%s : non masqué", n)
+		}
+	}
+	for _, n := range []string{"PATH", "HOME", "LANG", "NODE_ENV", "PORT", "HOSTNAME", "TZ", "PS1", "USER", ""} {
+		if SecretName(n) {
+			t.Errorf("%s : masqué à tort", n)
+		}
+	}
+}
+
+// Q26: any user information in a URL ("://…@" before a blank), with or
+// without a password: a token alone as user name is a secret too.
+func TestURLCredentials(t *testing.T) {
+	for _, v := range []string{
+		"postgres://app:s3cr3t@db:5432/app", "redis://:s3cr3t@cache:6379/0", "https://ghp_abc@github.com/o/r.git",
+		"amqp://u:p%40ss@mq", "x mysql://root:pw@db y", "HTTPS://U:P@H",
+		"https://example.com/a@b", // a false positive the safe-side rule accepts
+	} {
+		if !URLCredentials(v) {
+			t.Errorf("%q : non masqué", v)
+		}
+	}
+	for _, v := range []string{"http://example.com:8080/path", "admin@example.com", "https:// @x", "plain", ""} {
+		if URLCredentials(v) {
+			t.Errorf("%q : masqué à tort", v)
+		}
+	}
+}
+
+func TestEnv(t *testing.T) {
+	env := []string{"PATH=/usr/bin", "DB_PASSWORD=s3cr3t", "DATABASE_URL=postgres://u:pw@db/app", "EQ=a=b", "NOVAL", "Api_Key="}
+	got := Env(env)
+	str := func(s string) *string { return &s }
+	want := []Var{
+		{Name: "PATH", Value: str("/usr/bin")},
+		{Name: "DB_PASSWORD", Masked: true},
+		{Name: "DATABASE_URL", Masked: true},
+		{Name: "EQ", Value: str("a=b")},
+		{Name: "NOVAL", Value: str("")},
+		{Name: "Api_Key", Masked: true}, // empty, masked all the same: the name decides
+	}
+	if len(got) != len(want) {
+		t.Fatalf("%d variables, attendu %d", len(got), len(want))
+	}
+	for i := range want {
+		g, w := got[i], want[i]
+		if g.Name != w.Name || g.Masked != w.Masked || (g.Value == nil) != (w.Value == nil) || (g.Value != nil && *g.Value != *w.Value) {
+			t.Errorf("%d : %+v, attendu %+v", i, g, w)
+		}
+	}
+	b, _ := json.Marshal(got[1])
+	if string(b) != `{"name":"DB_PASSWORD","value":null,"masked":true}` {
+		t.Errorf("JSON d'une valeur masquée : %s (spec §7 : null avec masked: true)", b)
+	}
+}
+
+// The whole JSON of Env never carries a secret value.
+func TestEnvNeverLeaks(t *testing.T) {
+	secrets := []string{"s3cr3t-1", "s3cr3t-2", "s3cr3t-3", "s3cr3t-4", "s3cr3t-5", "s3cr3t-6", "s3cr3t-7", "s3cr3t-8", "s3cr3t-9", "s3cr3t-A", "s3cr3t-B"}
+	env := []string{"MYSQL_PASSWORD=" + secrets[0], "GH_TOKEN=" + secrets[1], "URL=https://bot:" + secrets[2] + "@x.io", "ssh_private_key=" + secrets[3],
+		// Q36: a secret assignment inside the value of a neutral name.
+		"ConnectionStrings__Default=Server=db;User Id=sa;Password=" + secrets[4] + ";",
+		"SPRING_DATASOURCE_URL=jdbc:postgresql://db/x?user=u&password=" + secrets[5],
+		"JAVA_OPTS=-Xmx512m -Djavax.net.ssl.keyStorePassword=" + secrets[6],
+		// Review 2: the three rules of the command (redis-stack's REDIS_ARGS),
+		// ODBC's Pwd=, an escaped "\=", a percent-encoded name.
+		"REDIS_ARGS=--requirepass " + secrets[7],
+		"ODBC=Driver={x};Server=db;Uid=u;Pwd=" + secrets[8] + ";",
+		`JAVA_TOOL_OPTIONS=-DDB_PASSWORD\=` + secrets[9],
+		"CALLBACK=https://x.io/cb?pass%77ord=" + secrets[10],
+	}
+	b, _ := json.Marshal(Env(env))
+	for _, s := range secrets {
+		if strings.Contains(string(b), s) {
+			t.Errorf("FUITE : %s dans %s", s, b)
+		}
+	}
+	// Controls: neutral values stay shown.
+	for _, kv := range []string{"JAVA_OPTS=-Xmx512m -Dfile.encoding=UTF-8", "NODE_OPTIONS=--max-old-space-size=4096", "PATH=/usr/local/keycloak/bin:/usr/bin", "PWD=/srv/app"} {
+		if Env([]string{kv})[0].Masked {
+			t.Errorf("%s : masqué à tort", kv)
+		}
+	}
+}
+
+func TestLookup(t *testing.T) {
+	env := []string{"A=1", "DB_PASSWORD=s3cr3t", "A=2", "NOVAL"}
+	if v, ok := Lookup(env, "DB_PASSWORD"); !ok || v != "s3cr3t" {
+		t.Errorf("DB_PASSWORD : %q %v", v, ok)
+	}
+	if v, ok := Lookup(env, "A"); !ok || v != "2" {
+		t.Errorf("doublon : %q %v, attendu la dernière valeur", v, ok)
+	}
+	if v, ok := Lookup(env, "NOVAL"); !ok || v != "" {
+		t.Errorf("sans '=' : %q %v", v, ok)
+	}
+	if _, ok := Lookup(env, "db_password"); ok {
+		t.Error("le nom est exact, casse comprise")
+	}
+	if _, ok := Lookup(env, ""); ok {
+		t.Error("nom vide")
+	}
+}
+
+// Q27: the command and entrypoint carry secrets too (redis-server
+// --requirepass, mysqld --password=, sh -c "PGPASSWORD=… psql URL").
+func TestArgs(t *testing.T) {
+	cases := []struct {
+		in, want []string
+		masked   bool
+	}{
+		{[]string{"redis-server", "--requirepass", "s3cr3t"}, []string{"redis-server", "--requirepass", Mask}, true},
+		{[]string{"mysqld", "--password=s3cr3t", "--port=3306"}, []string{"mysqld", "--password=" + Mask, "--port=3306"}, true},
+		{[]string{"sh", "-c", "PGPASSWORD=s3cr3t psql postgres://u:pw@db/x && echo ok"},
+			[]string{"sh", "-c", "PGPASSWORD=" + Mask + " psql postgres://" + Mask + "@db/x && echo ok"}, true},
+		{[]string{"sh", "-c", "redis-server --requirepass s3cr3t --port 6379 && x"},
+			[]string{"sh", "-c", "redis-server --requirepass " + Mask + " --port 6379 && x"}, true},
+		// Review I1 and Codex: quotes, a value with blanks or a quote inside, an
+		// assignment inside a value, "--x=" with its value in the next argument.
+		{[]string{"sh", "-c", `PGPASSWORD="s3cr3t" psql`}, []string{"sh", "-c", "PGPASSWORD=" + Mask + " psql"}, true},
+		{[]string{"sh", "-c", `PGPASSWORD='s3cr3t' psql`}, []string{"sh", "-c", "PGPASSWORD=" + Mask + " psql"}, true},
+		{[]string{"sh", "-c", `redis-server --requirepass "s3cr3t" --port 6379`},
+			[]string{"sh", "-c", "redis-server --requirepass " + Mask + " --port 6379"}, true},
+		{[]string{"sh", "-c", `x --password=s3"cr3t y`}, []string{"sh", "-c", "x --password=" + Mask}, true},
+		{[]string{"app", "--env=DB_PASSWORD=s3cr3t"}, []string{"app", "--env=DB_PASSWORD=" + Mask}, true},
+		{[]string{"app", "--password=s3 cr3t"}, []string{"app", "--password=" + Mask}, true},
+		{[]string{"app", `--password=s3"cr3t`}, []string{"app", "--password=" + Mask}, true},
+		{[]string{"app", "--password=", "s3cr3t"}, []string{"app", "--password=", Mask}, true},
+		// Fail closed: a word of §7 left with nothing masked after it in the
+		// argument masks the whole argument (false positives accepted).
+		{[]string{"sh", "-c", "mysql -ps3cr3t --password"}, []string{"sh", "-c", Mask}, true},
+		{[]string{"sh", "-c", "x --api-key -v"}, []string{"sh", "-c", Mask}, true},
+		// Review 2, Codex: the link is local. The value of this very word must be
+		// masked; a mask elsewhere in the line (the URL) does not cover it.
+		{[]string{"sh", "-c", "echo --password=$SECRET postgres://u:p@db"},
+			[]string{"sh", "-c", "echo --password=" + Mask + " postgres://" + Mask + "@db"}, true},
+		{[]string{"sh", "-c", "echo --password=$(cat /f) postgres://u:p@db"}, []string{"sh", "-c", Mask}, true},
+		// Review 2, Minor 1: "-c" is a shell line only after a shell.
+		{[]string{"postgres", "-c", "ssl_passphrase_command=echo s3cr3t"}, []string{"postgres", "-c", "ssl_passphrase_command=" + Mask}, true},
+		{[]string{"/bin/bash", "-lc", "redis-server --requirepass s3cr3t"}, []string{"/bin/bash", "-lc", "redis-server --requirepass " + Mask}, true},
+		// Review 2, Minor 2 and 5, Codex Minor: a value after "--x=" that starts
+		// with "-"; left boundaries ":" "{" "/"; an escaped "\=".
+		{[]string{"app", "--api-key=", "-s3cr3t"}, []string{"app", "--api-key=", Mask}, true},
+		{[]string{"sh", "-c", "x:password=s3cr3t {token=s3cr3t} /x/secret=s3cr3t"},
+			[]string{"sh", "-c", "x:password=" + Mask + " {token=" + Mask + " /x/secret=" + Mask}, true},
+		{[]string{"java", `-DDB_PASSWORD\=s3cr3t`, "-jar", "app.jar"}, []string{"java", `-DDB_PASSWORD\=` + Mask, "-jar", "app.jar"}, true},
+		// Review 2, I1: a word of §7 in a path, or in a plain word, is not a
+		// secret; these stay readable (controls of the fail-closed rule).
+		{[]string{"/opt/keycloak/bin/kc.sh", "start"}, []string{"/opt/keycloak/bin/kc.sh", "start"}, false},
+		{[]string{"sh", "-c", "cat /run/secrets/db_password > /tmp/p && exec app"}, []string{"sh", "-c", "cat /run/secrets/db_password > /tmp/p && exec app"}, false},
+		{[]string{"sh", "-c", "ssh-keyscan github.com >> ~/.ssh/known_hosts && exec app"}, []string{"sh", "-c", "ssh-keyscan github.com >> ~/.ssh/known_hosts && exec app"}, false},
+		{[]string{"sh", "-c", "echo 'reset your password' && exec app"}, []string{"sh", "-c", "echo 'reset your password' && exec app"}, false},
+		{[]string{"keydb-server", "/etc/keydb.conf"}, []string{"keydb-server", "/etc/keydb.conf"}, false},
+		{[]string{"app", "--cert=/etc/ssl/private/x.pem"}, []string{"app", "--cert=/etc/ssl/private/x.pem"}, false},
+		{[]string{"passwd", "root"}, []string{"passwd", "root"}, false},
+		{[]string{"app", "-api-key", "-v"}, []string{"app", "-api-key", "-v"}, false}, // a flag follows: no value
+		{[]string{"app", "--token"}, []string{"app", "--token"}, false},
+		{[]string{"nginx", "-g", "daemon off;"}, []string{"nginx", "-g", "daemon off;"}, false},
+		{[]string{"env", "API_TOKEN=abc", "run"}, []string{"env", "API_TOKEN=" + Mask, "run"}, true},
+		{nil, nil, false},
+	}
+	for _, k := range cases {
+		got, masked := Args(k.in)
+		if !slices.Equal(got, k.want) || masked != k.masked {
+			t.Errorf("%q : %q %v, attendu %q %v", k.in, got, masked, k.want, k.masked)
+		}
+		for _, a := range got {
+			if strings.Contains(a, "cr3t") || strings.Contains(a, ":pw@") || strings.Contains(a, "=abc") || strings.Contains(a, "(cat") {
+				t.Errorf("FUITE : %q", a)
+			}
+		}
+	}
+	in := []string{"x", "--password=s3cr3t"}
+	Args(in)
+	if in[1] != "--password=s3cr3t" {
+		t.Error("Args ne doit pas modifier son entrée")
+	}
+}
+
+// Review 2, I1: Docker's messages stay readable when a word of §7 is only a
+// word or a path; a secret value in them is masked.
+func TestLine(t *testing.T) {
+	for _, m := range []string{`exec: "/opt/keycloak/bin/kc.sh": permission denied`, "unable to read key file", ""} {
+		if got := Line(m); got != m {
+			t.Errorf("%q : %q, attendu inchangé", m, got)
+		}
+	}
+	if got := Line(`exec: "--password=s3cr3t": unknown`); strings.Contains(got, "cr3t") {
+		t.Errorf("FUITE : %q", got)
+	}
+}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go vet ./internal/secrets/ 2>&1 | tail -n 3`
Expected: `undefined: SecretName`.

- [ ] **Step 3: Implémenter**

```diff
diff --git a/internal/secrets/secrets.go b/internal/secrets/secrets.go
new file mode 100644
--- /dev/null
+++ b/internal/secrets/secrets.go
@@ -0,0 +1,236 @@
+// Package secrets masks the values the radiography shows (F7, spec §7): an
+// environment variable whose name says it is a secret, or whose value is a
+// URL with user information, never leaves the agent unless revealed.
+package secrets
+
+import (
+	"net/url"
+	"path"
+	"regexp"
+	"slices"
+	"strings"
+)
+
+// Mask replaces a masked part of a command argument.
+const Mask = "••••••"
+
+// words of spec §7, matched anywhere in a name, ignoring case.
+var words = []string{"PASSWORD", "PASSWD", "PASS", "TOKEN", "KEY", "SECRET", "CREDENTIAL", "PRIVATE", "DSN"}
+
+// SecretName: the name contains one of the words of spec §7, ignoring case
+// (KEYBOARD and COMPASS too: the rule errs on the safe side).
+func SecretName(name string) bool {
+	up := strings.ToUpper(name)
+	for _, w := range words {
+		if strings.Contains(up, w) {
+			return true
+		}
+	}
+	return false
+}
+
+// userinfo: "://", then an "@" before any blank (plan Observation, Q26:
+// a password or a token alone; greedy, to the last "@" of the word).
+var userinfo = regexp.MustCompile(`://\S*@`)
+
+// URLCredentials: v holds a URL with user information.
+func URLCredentials(v string) bool { return userinfo.MatchString(v) }
+
+// Var is one variable of the radiography. Value is nil when masked (spec
+// §7: null with masked: true).
+type Var struct {
+	Name   string  `json:"name"`
+	Value  *string `json:"value"`
+	Masked bool    `json:"masked"`
+}
+
+// Masked: the variable must not be shown as is (spec §7): a secret name,
+// or a value in which the rules of the command find something to mask
+// (plan Observation, Q36: a URL with user information, "…;Pwd=…;",
+// "?password=…", "-D…Password\=…", "--requirepass …"), the value read as
+// is and percent-decoded ("?pass%77ord=…").
+func Masked(name, value string) bool {
+	if SecretName(name) || maskText(value) != value {
+		return true
+	}
+	d, err := url.PathUnescape(value)
+	return err == nil && d != value && maskText(d) != d
+}
+
+// Env masks a Config.Env ("NAME=value"; "NAME" alone has an empty value),
+// in order.
+func Env(env []string) []Var {
+	out := make([]Var, 0, len(env))
+	for _, kv := range env {
+		name, value, _ := strings.Cut(kv, "=")
+		v := Var{Name: name, Masked: Masked(name, value)}
+		if !v.Masked {
+			v.Value = &value
+		}
+		out = append(out, v)
+	}
+	return out
+}
+
+// Lookup returns the value of variable name, exact name: the last one if
+// it appears twice (Docker removes duplicates when it creates a container).
+func Lookup(env []string, name string) (string, bool) {
+	value, found := "", false
+	for _, kv := range env {
+		if n, v, _ := strings.Cut(kv, "="); n == name && name != "" {
+			value, found = v, true
+		}
+	}
+	return value, found
+}
+
+// value: a word, possibly quoted or partly quoted ("x", 'x', a"b c"d); an
+// unclosed quote runs to the end (review I1: nothing after it leaks).
+const value = `(?:"[^"]*"?|'[^']*'?|[^\s;&|"'()])+`
+
+// assignment: NAME=value or --name=value inside a text (a shell line, a
+// connection string), after anything that cannot be part of a name (review
+// 2, Minor 5: ":", "/", "{", "[" too); "\=" (an escaped "=", Java
+// properties) counts.
+var assignment = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])(-{0,2}[A-Za-z0-9_.-]+)(\\?=)(` + value + `)`)
+
+// pwd: Pwd=, the password key of ODBC and .NET connection strings. Only
+// as an assignment inside a text: the variable PWD is the working
+// directory, not a secret.
+var pwd = regexp.MustCompile(`(?i)pwd`)
+
+// secretKey: the name of an assignment whose value is a secret.
+func secretKey(name string) bool { return SecretName(name) || pwd.MatchString(name) }
+
+// flagValue: a flag, blanks, then a word that is not a flag, inside a shell
+// line (sh -c "… --requirepass x …").
+var flagValue = regexp.MustCompile(`(^|\s)(-{1,2}[A-Za-z0-9_.-]+)(\s+)((?:"[^"]*"?|'[^']*'?|[^\s;&|"'()-])(?:"[^"]*"?|'[^']*'?|[^\s;&|"'()])*)`)
+
+// flag: an argument that is only a flag ("--password", or "--password="
+// whose value is the next argument).
+var flag = regexp.MustCompile(`^-{1,2}[A-Za-z0-9_.-]+=?$`)
+
+// shells and shellFlag: "sh -c", "bash -lc", "ash -ec": the next argument is
+// a shell line (review 2, Minor 1: "postgres -c name=value" is not one).
+var (
+	shells    = []string{"sh", "bash", "ash", "dash", "zsh", "ksh"}
+	shellFlag = regexp.MustCompile(`^-[A-Za-z]*c$`)
+)
+
+// whole: an argument NAME=value or --name=value, outside a shell line: the
+// value is the whole rest of the argument, blanks included.
+var whole = regexp.MustCompile(`(?s)^(-{0,2}[A-Za-z0-9_.-]+)=(.+)$`)
+
+// word: a word of spec §7, anywhere, ignoring case.
+var word = regexp.MustCompile(`(?i)` + strings.Join(words, "|"))
+
+// maskText masks a text: user information of URLs, then the value of each
+// secret assignment (an assignment of another name is searched inside its
+// value: --env=DB_PASSWORD=x), then the word after a secret flag.
+func maskText(s string) string {
+	s = userinfo.ReplaceAllString(s, "://"+Mask+"@")
+	s = assignment.ReplaceAllStringFunc(s, func(m string) string {
+		sub := assignment.FindStringSubmatch(m)
+		if secretKey(sub[2]) {
+			return sub[1] + sub[2] + sub[3] + Mask
+		}
+		return sub[1] + sub[2] + sub[3] + maskText(sub[4])
+	})
+	return flagValue.ReplaceAllStringFunc(s, func(m string) string {
+		sub := flagValue.FindStringSubmatch(m)
+		if !SecretName(sub[2]) || sub[4] == Mask {
+			return m
+		}
+		return sub[1] + sub[2] + sub[3] + Mask
+	})
+}
+
+// blank: what ends a token for leaks.
+const blank = " \t\r\n\"'"
+
+// alnum: a letter or a digit.
+var alnum = regexp.MustCompile(`[A-Za-z0-9]`)
+
+// leaks: a word of §7 whose own value is not masked (review 2: the link is
+// local, a mask elsewhere in the text does not cover it). In its token (a
+// run without blank or quote), a word is
+//   - covered when the token ends with a mask after it (only punctuation
+//     may follow: "--password=••••••;"), or when the token is a flag and
+//     the next token is a mask ("--requirepass ••••••");
+//   - harmless in a path ("/" before it in the token, no "=" or ":" after
+//     it) or in a plain word ("ssh-keyscan", "reset your password");
+//   - a leak when the token is a flag or holds "=" or ":" after the word.
+//
+// An argument that is only a flag holds no value: never a leak.
+func leaks(s string) bool {
+	if flag.MatchString(s) {
+		return false
+	}
+	for _, loc := range word.FindAllStringIndex(s, -1) {
+		st := strings.LastIndexAny(s[:loc[0]], blank) + 1
+		en := len(s)
+		if i := strings.IndexAny(s[loc[1]:], blank); i >= 0 {
+			en = loc[1] + i
+		}
+		tok, before, after := s[st:en], s[st:loc[0]], s[loc[1]:en]
+		if i := strings.LastIndex(after, Mask); i >= 0 && !alnum.MatchString(after[i+len(Mask):]) {
+			continue
+		}
+		isFlag, bound := strings.HasPrefix(tok, "-"), strings.ContainsAny(after, "=:")
+		if isFlag && !bound && strings.HasPrefix(strings.TrimLeft(s[en:], " \t"), Mask) {
+			continue
+		}
+		if strings.Contains(before, "/") && !bound {
+			continue
+		}
+		if isFlag || bound {
+			return true
+		}
+	}
+	return false
+}
+
+// Line masks one text (a message of Docker, a shell line): maskText, then,
+// if a word of §7 is left with nothing masked after it, the whole text.
+func Line(s string) string {
+	if m := maskText(s); !leaks(m) {
+		return m
+	}
+	return Mask
+}
+
+// Args masks a command or an entrypoint (plan Observation, Q27), in a copy.
+// The argument after a secret flag is masked whole: always after
+// "--password=", unless it is a flag itself after "--requirepass". The
+// argument after the "-c" of a shell (sh -c, bash -lc…) is a shell line:
+// Line. Any other argument NAME=value or --name=value keeps its name and
+// masks its whole value if the name is secret (blanks included), else the
+// value goes through the rules of Line. masked tells whether anything was
+// hidden.
+func Args(args []string) (out []string, masked bool) {
+	if args == nil {
+		return nil, false
+	}
+	out = make([]string, len(args))
+	for i, a := range args {
+		prev := ""
+		if i > 0 {
+			prev = args[i-1]
+		}
+		switch w := whole.FindStringSubmatch(a); {
+		case flag.MatchString(prev) && SecretName(prev) && (strings.HasSuffix(prev, "=") || !strings.HasPrefix(a, "-")):
+			out[i] = Mask // "--password=" then any value; "--password" then a non-flag
+		case i > 0 && shellFlag.MatchString(prev) && slices.Contains(shells, path.Base(args[0])), w == nil:
+			out[i] = Line(a)
+		case SecretName(w[1]):
+			out[i] = w[1] + "=" + Mask
+		default:
+			out[i] = w[1] + "=" + maskText(w[2])
+			if leaks(out[i]) {
+				out[i] = Mask
+			}
+		}
+		masked = masked || out[i] != a
+	}
+	return out, masked
+}
```

- [ ] **Step 4: Vérifier**

Run: `go test -race ./internal/secrets/ 2>&1 | tail -n 3`
Expected: `ok`. Mutations : `Masked` réduit à `SecretName(name)` → `TestEnv` et `TestEnvNeverLeaks` (« FUITE : s3cr3t-3 ») échouent ; `leaks` qui rend toujours `false` → `TestArgs` (« FUITE : "mysql -ps3cr3t --password" ») ; test de fuite global (`strings.Contains(s[loc[1]:], Mask)` au lieu du masque en fin de mot) → `TestArgs` (« FUITE : "echo --password=••••••(cat /f) …" ») ; exemption des chemins neutralisée → `TestArgs` (`--cert=/etc/ssl/private/x.pem` masqué à tort).

- [ ] **Step 5: Commit**

```bash
git add internal/secrets
git commit -F - <<'EOF'
secrets : masquage des variables et de la commande (spec §7)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 5: Radiographie — `Details`, `GET /api/containers/{id}`, `POST /env/reveal`

**Où tester :** macOS.

**Files:**
- Create: `internal/api/container.go`, `internal/api/container_test.go`
- Modify: `internal/dockerx/dockerx.go`, `internal/api/server.go`
- Test: `internal/dockerx/dockerx_test.go`, `internal/api/server_test.go` (le `fixture` garde son `*audit.Log`)

**Interfaces:**
- Consumes: `secrets.Env`, `secrets.Args`, `secrets.Line` (message d'erreur de l'état, R9), `secrets.Lookup`, `inspectByRef` (tâche 3), `writeFSError`, `audit.Event`, `Port` (F1).
- Produces: `dockerx.Details{Image, ImageID, Created, Status string; ExitCode int; Error, StartedAt string; OOMKilled bool; Health string; RestartCount int; RestartPolicy string; MaxRetry int; Entrypoint, Cmd []string; WorkingDir, User string; Env []string; Ports []Port; NetworkMode string; Networks []Network}` ; `dockerx.Network{Name, IP, Gateway, MAC string; Aliases []string}` ; `ContainerInfo.Details` ; `Mount.Mode string`, `Mount.RW bool` ; `fromInspectDetails`, `portsOf` ; routes `GET /api/containers/{id}` (`radioJSON`) et `POST /api/containers/{id}/env/reveal` (`{name}` → `{name, value}`).

> **À comprendre :** un seul `Inspect` donne tout. `Env`, `Cmd` et `Entrypoint` restent bruts dans `dockerx` (le code de l'agent en a besoin, par exemple pour révéler) et ne sortent que par `toRadioJSON`, qui passe par `secrets`. La révélation suit l'ordre d'une écriture : contrôles, journal, puis seulement la réponse ; le journal ne reçoit que le nom (Q28). Le contrôle d'`Origin` de §7 s'applique déjà à toute requête `POST`.

- [ ] **Step 1: Tests qui échouent** — aucune réponse ne contient un secret ; la révélation est journalisée sans sa valeur :

```diff
diff --git a/internal/api/container_test.go b/internal/api/container_test.go
new file mode 100644
--- /dev/null
+++ b/internal/api/container_test.go
@@ -0,0 +1,206 @@
+package api
+
+// Tests of the radiography (F7, spec §6, §7), written before the code: no
+// secret leaves the agent unmasked; a reveal needs the Origin check, is
+// audited before the value is sent, and never writes the value in the log.
+
+import (
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"os"
+	"strings"
+	"testing"
+
+	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+)
+
+// The secrets of the fake "web" container: none may appear in a response
+// that has not revealed it.
+var radioSecrets = []string{"s3cr3t-env", "s3cr3t-url", "s3cr3t-cmd", "s3cr3t-ep", "s3cr3t-err", "s3cr3t-cs"}
+
+func radioFixture(t *testing.T, readOnly bool) (*fixture, *http.Cookie) {
+	t.Helper()
+	f := newFixture(t, readOnly)
+	web := f.docker.Infos["web"]
+	web.Mounts = []dockerx.Mount{{Type: "bind", Source: "/srv/conf", Destination: "/etc/app", Mode: "ro"}}
+	web.Tmpfs = []string{"/run"}
+	web.LogDriver = "json-file"
+	web.Details = dockerx.Details{
+		Image: "nginx:1.27", ImageID: "sha256:feed", Created: "2026-10-10T07:00:00Z", Status: "running",
+		// Review Minor 8: Docker's message may quote the command.
+		Error:        `OCI runtime create failed: exec: "--password=s3cr3t-err": unknown`,
+		RestartCount: 2, RestartPolicy: "unless-stopped",
+		Entrypoint: []string{"/entry.sh", "--token", "s3cr3t-ep"},
+		Cmd:        []string{"redis-server", "--requirepass", "s3cr3t-cmd"},
+		Env: []string{"PATH=/usr/bin", "DB_PASSWORD=s3cr3t-env", "CACHE_URL=redis://:s3cr3t-url@cache:6379",
+			"PLAIN=visible", "DB=Server=db;Password=s3cr3t-cs;"},
+		Ports:       []dockerx.Port{{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"}},
+		NetworkMode: "bridge",
+		Networks:    []dockerx.Network{{Name: "front", IP: "172.18.0.2"}},
+	}
+	f.docker.Infos["web"], f.docker.Infos[webID] = web, web
+	return f, f.session(t)
+}
+
+func noSecret(t *testing.T, body string) {
+	t.Helper()
+	for _, s := range radioSecrets {
+		if strings.Contains(body, s) {
+			t.Errorf("FUITE : %s dans la réponse", s)
+		}
+	}
+}
+
+func TestContainerNeedsSession(t *testing.T) {
+	f, _ := radioFixture(t, false)
+	if rec := f.do("GET", "/api/containers/web", "", nil); rec.Code != http.StatusUnauthorized {
+		t.Fatalf("statut %d", rec.Code)
+	}
+	if rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, nil); rec.Code != http.StatusUnauthorized {
+		t.Fatalf("révélation sans session : %d", rec.Code)
+	}
+}
+
+func TestContainerRadiography(t *testing.T) {
+	f, c := radioFixture(t, false)
+	rec := f.do("GET", "/api/containers/web", "", c)
+	if rec.Code != http.StatusOK {
+		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
+	}
+	noSecret(t, rec.Body.String())
+	var got struct {
+		ID, Name, Image, ImageID, LogDriver string
+		State                               struct {
+			Status  string
+			Running bool
+		}
+		RestartCount    int
+		RestartPolicy   struct{ Name string }
+		Cmd, Entrypoint []string
+		CommandMasked   bool
+		Env             []struct {
+			Name   string
+			Value  *string
+			Masked bool
+		}
+		Ports    []dockerx.Port
+		Networks []struct{ Name, IP string }
+		Mounts   []struct {
+			Type, Source, Destination, Mode string
+			RW                              bool
+		}
+		Tmpfs []string
+	}
+	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
+		t.Fatal(err)
+	}
+	if got.ID != webID || got.Name != "web" || got.Image != "nginx:1.27" || got.ImageID != "sha256:feed" ||
+		got.State.Status != "running" || !got.State.Running || got.RestartCount != 2 ||
+		got.RestartPolicy.Name != "unless-stopped" || got.LogDriver != "json-file" {
+		t.Errorf("identité, état : %+v", got)
+	}
+	if !got.CommandMasked || strings.Join(got.Cmd, " ") != "redis-server --requirepass ••••••" ||
+		strings.Join(got.Entrypoint, " ") != "/entry.sh --token ••••••" {
+		t.Errorf("commande : %q %q %v", got.Entrypoint, got.Cmd, got.CommandMasked)
+	}
+	masked := map[string]bool{}
+	for _, v := range got.Env {
+		masked[v.Name] = v.Masked
+		if v.Masked != (v.Value == nil) {
+			t.Errorf("%s : masked %v mais value %v (spec §7 : null avec masked: true)", v.Name, v.Masked, v.Value)
+		}
+	}
+	if !masked["DB_PASSWORD"] || !masked["CACHE_URL"] || !masked["DB"] || masked["PATH"] || masked["PLAIN"] || len(got.Env) != 5 {
+		t.Errorf("masquage : %v", masked)
+	}
+	if len(got.Ports) != 1 || got.Ports[0].Public != 8080 || len(got.Networks) != 1 || got.Networks[0].IP != "172.18.0.2" {
+		t.Errorf("ports, réseaux : %+v %+v", got.Ports, got.Networks)
+	}
+	if len(got.Mounts) != 1 || got.Mounts[0].Mode != "ro" || got.Mounts[0].Destination != "/etc/app" || len(got.Tmpfs) != 1 {
+		t.Errorf("montages : %+v %v", got.Mounts, got.Tmpfs)
+	}
+}
+
+func TestContainerErrors(t *testing.T) {
+	f, c := radioFixture(t, false)
+	if rec := f.do("GET", "/api/containers/absent", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
+		t.Errorf("absent : %d %s", rec.Code, rec.Body)
+	}
+	if rec := f.do("GET", "/api/containers/..x", "", c); rec.Code != http.StatusNotFound {
+		t.Errorf("identifiant invalide : %d", rec.Code)
+	}
+	f.docker.Err = dockerx.ErrUnavailable
+	if rec := f.do("GET", "/api/containers/web", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
+		t.Errorf("Docker indisponible : %d %s", rec.Code, rec.Body)
+	}
+}
+
+func auditLines(t *testing.T, f *fixture) string {
+	t.Helper()
+	b, _ := os.ReadFile(f.auditP)
+	return string(b)
+}
+
+// Spec §6, §7: the value of one variable, journaled (env.reveal) before it
+// is sent; the value itself never goes to the log. Read-only mode does not
+// forbid it: it is a read.
+func TestRevealEnv(t *testing.T) {
+	f, c := radioFixture(t, true)
+	rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, c)
+	if rec.Code != http.StatusOK {
+		t.Fatalf("statut %d : %s", rec.Code, rec.Body)
+	}
+	var got struct{ Name, Value string }
+	json.Unmarshal(rec.Body.Bytes(), &got)
+	if got.Name != "DB_PASSWORD" || got.Value != "s3cr3t-env" {
+		t.Fatalf("révélé : %+v", got)
+	}
+	log := auditLines(t, f)
+	if !strings.Contains(log, `"action":"env.reveal","container":"`+webID+`","path":"DB_PASSWORD","result":"ok"`) {
+		t.Errorf("journal : %s", log)
+	}
+	noSecret(t, log)
+	// A variable shown anyway can be asked too, and is journaled the same.
+	if rec := f.do("POST", "/api/containers/web/env/reveal", `{"name":"PLAIN"}`, c); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "visible") {
+		t.Errorf("variable non masquée : %d %s", rec.Code, rec.Body)
+	}
+}
+
+func TestRevealEnvRefusals(t *testing.T) {
+	f, c := radioFixture(t, false)
+	const same = "http://example.com"
+	cases := []struct {
+		name, body, origin string
+		status             int
+		code               string
+	}{
+		{"Origin absente", `{"name":"DB_PASSWORD"}`, "", http.StatusForbidden, "forbidden_origin"},
+		{"Origin étrangère", `{"name":"DB_PASSWORD"}`, "http://evil.example", http.StatusForbidden, "forbidden_origin"},
+		{"variable absente", `{"name":"NOPE"}`, same, http.StatusNotFound, "not_found"},
+		{"casse différente", `{"name":"db_password"}`, same, http.StatusNotFound, "not_found"},
+		{"corps invalide", `{"name":`, same, http.StatusNotFound, "not_found"},
+		{"données en trop", `{"name":"DB_PASSWORD"}{}`, same, http.StatusNotFound, "not_found"},
+	}
+	for _, k := range cases {
+		r := httptest.NewRequest("POST", "http://example.com/api/containers/web/env/reveal", strings.NewReader(k.body))
+		if k.origin != "" {
+			r.Header.Set("Origin", k.origin)
+		}
+		r.AddCookie(c)
+		rec := httptest.NewRecorder()
+		f.h.ServeHTTP(rec, r)
+		if r := rec; r.Code != k.status || errCode(t, r) != k.code {
+			t.Errorf("%s : %d %s, attendu %d %s", k.name, r.Code, r.Body, k.status, k.code)
+		}
+		noSecret(t, rec.Body.String())
+	}
+	if log := auditLines(t, f); strings.Contains(log, "env.reveal") {
+		t.Errorf("un refus a été journalisé comme une révélation : %s", log)
+	}
+	// The audit log cannot be written: nothing is revealed.
+	f.auditLog.Close()
+	if r := f.do("POST", "/api/containers/web/env/reveal", `{"name":"DB_PASSWORD"}`, c); r.Code != http.StatusInternalServerError || strings.Contains(r.Body.String(), "s3cr3t") {
+		t.Errorf("journal en échec : %d %s", r.Code, r.Body)
+	}
+}
diff --git a/internal/api/server_test.go b/internal/api/server_test.go
--- a/internal/api/server_test.go
+++ b/internal/api/server_test.go
@@ -22,10 +22,11 @@ import (
 )
 
 type fixture struct {
-	h      http.Handler
-	token  string
-	auditP string
-	docker *dockerx.Fake // nil when the fixture talks to a real Docker
+	h        http.Handler
+	token    string
+	auditP   string
+	auditLog *audit.Log    // closed by a test to make audit writes fail
+	docker   *dockerx.Fake // nil when the fixture talks to a real Docker
 	// File routes: OpenFS returns fs (or openErr) and records the ID it got.
 	fs       *fsview.MemFS
 	views    *fsview.Views // nil: the default of New
@@ -88,7 +89,7 @@ func (f *fixture) init(t *testing.T, readOnly bool, docker dockerx.Client, openF
 		Sessions: auth.NewSessions(auth.SessionTTL, time.Now),
 		Limiter:  auth.NewLimiter(5, time.Minute, time.Now),
 	})
-	f.token, f.auditP = tok, ap
+	f.token, f.auditP, f.auditLog = tok, ap, al
 }
 
 func (f *fixture) do(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
diff --git a/internal/dockerx/dockerx_test.go b/internal/dockerx/dockerx_test.go
--- a/internal/dockerx/dockerx_test.go
+++ b/internal/dockerx/dockerx_test.go
@@ -14,6 +14,7 @@ import (
 
 	cerrdefs "github.com/containerd/errdefs"
 	"github.com/moby/moby/api/types/container"
+	"github.com/moby/moby/api/types/network"
 )
 
 func TestFromSummary(t *testing.T) {
@@ -235,3 +236,63 @@ func TestFakePutArchive(t *testing.T) {
 		t.Fatal("un appel en échec est enregistré aussi")
 	}
 }
+
+// F7: what the radiography shows of `docker inspect`. Env stays raw here:
+// package secrets masks it in the API (spec §7).
+func TestFromInspectDetails(t *testing.T) {
+	p80 := network.MustParsePort("80/tcp")
+	p53 := network.MustParsePort("53/udp")
+	p9000 := network.MustParsePort("9000/tcp")
+	in := container.InspectResponse{
+		ID: "abc", Name: "/web", Image: "sha256:feed", Created: "2026-10-10T07:00:00Z", RestartCount: 3,
+		State: &container.State{Status: "running", Running: true, Pid: 7, ExitCode: 0, StartedAt: "2026-10-10T07:00:01Z",
+			FinishedAt: "0001-01-01T00:00:00Z", OOMKilled: true, Health: &container.Health{Status: "unhealthy"}},
+		Config: &container.Config{Image: "nginx:1.27", Env: []string{"A=1", "DB_PASSWORD=x"}, Cmd: []string{"nginx", "-g", "daemon off;"},
+			Entrypoint: []string{"/docker-entrypoint.sh"}, WorkingDir: "/srv", User: "101",
+			ExposedPorts: network.PortSet{p80: {}, p53: {}, p9000: {}}},
+		HostConfig: &container.HostConfig{NetworkMode: "bridge",
+			RestartPolicy: container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 5},
+			PortBindings:  network.PortMap{p80: {{HostPort: "8080"}}}},
+		NetworkSettings: &container.NetworkSettings{
+			Ports: network.PortMap{
+				p80: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "8080"}, {HostIP: netip.MustParseAddr("::"), HostPort: "8080"}},
+				p53: {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "5353"}},
+			},
+			Networks: map[string]*network.EndpointSettings{
+				"front": {IPAddress: netip.MustParseAddr("172.18.0.2"), Gateway: netip.MustParseAddr("172.18.0.1"), Aliases: []string{"web"},
+					MacAddress: network.HardwareAddr{2, 66, 172, 18, 0, 2}},
+				"back": {},
+			},
+		},
+		Mounts: []container.MountPoint{{Type: "volume", Name: "data", Source: "/var/lib/docker/volumes/data/_data", Destination: "/data", Driver: "local", Mode: "z", RW: true}},
+	}
+	got := fromInspect(in).Details
+	want := Details{
+		Image: "nginx:1.27", ImageID: "sha256:feed", Created: "2026-10-10T07:00:00Z",
+		Status: "running", StartedAt: "2026-10-10T07:00:01Z", OOMKilled: true, Health: "unhealthy",
+		RestartCount: 3, RestartPolicy: "on-failure", MaxRetry: 5,
+		Entrypoint: []string{"/docker-entrypoint.sh"}, Cmd: []string{"nginx", "-g", "daemon off;"},
+		WorkingDir: "/srv", User: "101", Env: []string{"A=1", "DB_PASSWORD=x"},
+		NetworkMode: "bridge",
+		Ports: []Port{ // running: the real bindings, 0.0.0.0 and :: once; exposed without binding too
+			{Private: 53, Public: 5353, Type: "udp", IP: "127.0.0.1"},
+			{Private: 80, Public: 8080, Type: "tcp", IP: "0.0.0.0"},
+			{Private: 9000, Type: "tcp"},
+		},
+		Networks: []Network{{Name: "back"}, {Name: "front", IP: "172.18.0.2", Gateway: "172.18.0.1", MAC: "02:42:ac:12:00:02", Aliases: []string{"web"}}},
+	}
+	if !reflect.DeepEqual(got, want) {
+		t.Fatalf("got  %+v\nwant %+v", got, want)
+	}
+	if m := fromInspect(in).Mounts[0]; m.Mode != "z" || !m.RW {
+		t.Errorf("montage : %+v", m)
+	}
+	// Stopped: no real binding, the configured ones.
+	in.State.Running, in.NetworkSettings.Ports = false, nil
+	if ports := fromInspect(in).Details.Ports; !reflect.DeepEqual(ports, []Port{{Private: 53, Type: "udp"}, {Private: 80, Public: 8080, Type: "tcp"}, {Private: 9000, Type: "tcp"}}) {
+		t.Errorf("ports configurés : %+v", ports)
+	}
+	if d := fromInspect(container.InspectResponse{ID: "x"}).Details; !reflect.DeepEqual(d, Details{}) {
+		t.Errorf("Inspect vide : %+v", d)
+	}
+}
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `go vet ./internal/dockerx/ ./internal/api/ 2>&1 | tail -n 4`
Expected: échec de compilation (`fromInspect(in).Details undefined`, `unknown field Mode in struct literal of type dockerx.Mount`).

- [ ] **Step 3: Implémenter**

```diff
diff --git a/internal/api/container.go b/internal/api/container.go
new file mode 100644
--- /dev/null
+++ b/internal/api/container.go
@@ -0,0 +1,144 @@
+package api
+
+import (
+	"encoding/json"
+	"errors"
+	"fmt"
+	"io"
+	"log/slog"
+	"net/http"
+
+	"github.com/levy-cmd/hublot-kit/internal/audit"
+	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+	"github.com/levy-cmd/hublot-kit/internal/secrets"
+)
+
+type stateJSON struct {
+	Status     string `json:"status"`
+	Running    bool   `json:"running"`
+	ExitCode   int    `json:"exitCode"`
+	Error      string `json:"error,omitempty"`
+	StartedAt  string `json:"startedAt,omitempty"`
+	FinishedAt string `json:"finishedAt,omitempty"`
+	OOMKilled  bool   `json:"oomKilled"`
+	Health     string `json:"health,omitempty"`
+}
+
+type restartJSON struct {
+	Name     string `json:"name"`
+	MaxRetry int    `json:"maxRetry,omitempty"`
+}
+
+type mountJSON struct {
+	Type        string `json:"type"`
+	Name        string `json:"name,omitempty"`
+	Source      string `json:"source,omitempty"`
+	Destination string `json:"destination"`
+	Driver      string `json:"driver,omitempty"`
+	Mode        string `json:"mode,omitempty"`
+	RW          bool   `json:"rw"`
+}
+
+// radioJSON is GET /api/containers/{id} (F7). Env and the command are
+// masked by package secrets (spec §7): nothing secret leaves the agent
+// here; one variable at a time goes through POST /env/reveal.
+type radioJSON struct {
+	ID            string            `json:"id"`
+	Name          string            `json:"name"`
+	Image         string            `json:"image"`
+	ImageID       string            `json:"imageId"`
+	Created       string            `json:"created"`
+	State         stateJSON         `json:"state"`
+	RestartCount  int               `json:"restartCount"`
+	RestartPolicy restartJSON       `json:"restartPolicy"`
+	Entrypoint    []string          `json:"entrypoint"`
+	Cmd           []string          `json:"cmd"`
+	CommandMasked bool              `json:"commandMasked"`
+	WorkingDir    string            `json:"workingDir,omitempty"`
+	User          string            `json:"user,omitempty"`
+	Tty           bool              `json:"tty"`
+	LogDriver     string            `json:"logDriver"`
+	Env           []secrets.Var     `json:"env"`
+	Ports         []dockerx.Port    `json:"ports"`
+	NetworkMode   string            `json:"networkMode,omitempty"`
+	Networks      []dockerx.Network `json:"networks"`
+	Mounts        []mountJSON       `json:"mounts"`
+	Tmpfs         []string          `json:"tmpfs"`
+}
+
+func toRadioJSON(c dockerx.ContainerInfo) radioJSON {
+	d := c.Details
+	ep, epMasked := secrets.Args(d.Entrypoint)
+	cmd, cmdMasked := secrets.Args(d.Cmd)
+	out := radioJSON{
+		ID: c.ID, Name: c.Name, Image: d.Image, ImageID: d.ImageID, Created: d.Created,
+		State: stateJSON{Status: d.Status, Running: c.Running, ExitCode: d.ExitCode, Error: secrets.Line(d.Error),
+			StartedAt: d.StartedAt, FinishedAt: c.FinishedAt, OOMKilled: d.OOMKilled, Health: d.Health},
+		RestartCount: d.RestartCount, RestartPolicy: restartJSON{Name: d.RestartPolicy, MaxRetry: d.MaxRetry},
+		Entrypoint: nonNil(ep), Cmd: nonNil(cmd), CommandMasked: epMasked || cmdMasked,
+		WorkingDir: d.WorkingDir, User: d.User, Tty: c.Tty, LogDriver: c.LogDriver,
+		Env: secrets.Env(d.Env), Ports: nonNil(d.Ports), NetworkMode: d.NetworkMode,
+		Networks: nonNil(d.Networks), Mounts: []mountJSON{}, Tmpfs: nonNil(c.Tmpfs),
+	}
+	for _, m := range c.Mounts {
+		out.Mounts = append(out.Mounts, mountJSON{Type: m.Type, Name: m.Name, Source: m.Source,
+			Destination: m.Destination, Driver: m.Driver, Mode: m.Mode, RW: m.RW})
+	}
+	return out
+}
+
+// nonNil: an empty JSON array rather than null.
+func nonNil[T any](s []T) []T {
+	if s == nil {
+		return []T{}
+	}
+	return s
+}
+
+// container is GET /api/containers/{id} (F7).
+func (s *server) container(w http.ResponseWriter, r *http.Request) {
+	info, err := s.inspectByRef(r)
+	if err != nil {
+		writeFSError(w, err)
+		return
+	}
+	writeJSON(w, http.StatusOK, toRadioJSON(info))
+}
+
+// errNoVariable: the body names no variable of the container.
+var errNoVariable = fmt.Errorf("%w : variable inconnue", dockerx.ErrNotFound)
+
+// revealEnv is POST /api/containers/{id}/env/reveal (spec §6, §7): {name} →
+// {name, value}. The reveal is journaled (env.reveal, the name in path,
+// never the value) before the value is sent; if the journal cannot be
+// written, nothing is revealed. A read: the read-only mode allows it.
+func (s *server) revealEnv(w http.ResponseWriter, r *http.Request) {
+	var body struct {
+		Name string `json:"name"`
+	}
+	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
+	err := dec.Decode(&body)
+	if err == nil && dec.Decode(&struct{}{}) != io.EOF {
+		err = errors.New("données après l'objet JSON")
+	}
+	if err != nil {
+		writeFSError(w, fmt.Errorf("%w (%v)", errNoVariable, err))
+		return
+	}
+	info, err := s.inspectByRef(r)
+	if err != nil {
+		writeFSError(w, err)
+		return
+	}
+	value, ok := secrets.Lookup(info.Details.Env, body.Name)
+	if !ok {
+		writeFSError(w, errNoVariable)
+		return
+	}
+	if err := s.d.Audit.Write(audit.Event{Action: "env.reveal", Container: info.ID, Path: body.Name, Result: "ok"}); err != nil {
+		slog.Error("audit write", "err", err, "action", "env.reveal")
+		writeError(w, http.StatusInternalServerError, "internal", "journal d'audit : rien n'est révélé")
+		return
+	}
+	writeJSON(w, http.StatusOK, map[string]string{"name": body.Name, "value": value})
+}
diff --git a/internal/api/server.go b/internal/api/server.go
--- a/internal/api/server.go
+++ b/internal/api/server.go
@@ -111,6 +111,8 @@ func New(d Deps) http.Handler {
 	mux.HandleFunc("POST /api/logout", s.logout)
 	mux.Handle("GET /api/meta", session(s.meta))
 	mux.Handle("GET /api/containers", session(s.containers))
+	mux.Handle("GET /api/containers/{id}", session(s.container))
+	mux.Handle("POST /api/containers/{id}/env/reveal", session(s.revealEnv))
 	mux.Handle("GET /api/containers/{id}/fs", session(s.fsList))
 	mux.Handle("GET /api/containers/{id}/fs/file", session(s.fsFile))
 	mux.Handle("GET /api/containers/{id}/fs/download", session(s.fsDownload))
diff --git a/internal/dockerx/dockerx.go b/internal/dockerx/dockerx.go
--- a/internal/dockerx/dockerx.go
+++ b/internal/dockerx/dockerx.go
@@ -7,11 +7,13 @@ import (
 	"fmt"
 	"io"
 	"sort"
+	"strconv"
 	"strings"
 	"time"
 
 	cerrdefs "github.com/containerd/errdefs"
 	"github.com/moby/moby/api/types/container"
+	"github.com/moby/moby/api/types/network"
 	"github.com/moby/moby/client"
 )
 
@@ -49,6 +51,46 @@ type ContainerInfo struct {
 	Tty bool
 	// LogDriver: HostConfig.LogConfig.Type ("json-file", "local", "none"…).
 	LogDriver string
+	// Details: the rest of the radiography (F7).
+	Details Details
+}
+
+// Details is what the radiography (F7) shows of `docker inspect`, beyond
+// ContainerInfo. Env, Cmd and Entrypoint are raw: package secrets masks
+// them before they leave the agent (spec §7).
+type Details struct {
+	Image        string // as asked (Config.Image: "nginx:1.27")
+	ImageID      string // "sha256:…"
+	Created      string
+	Status       string // State.Status: created, running, paused, restarting, exited, dead
+	ExitCode     int
+	Error        string
+	StartedAt    string
+	OOMKilled    bool
+	Health       string // healthy, unhealthy, starting; "" without a health check
+	RestartCount int
+	// RestartPolicy: no, always, on-failure, unless-stopped; MaxRetry for on-failure.
+	RestartPolicy string
+	MaxRetry      int
+	Entrypoint    []string
+	Cmd           []string
+	WorkingDir    string
+	User          string
+	Env           []string // "NAME=value", raw
+	// Ports: the real bindings of a running container, else the configured
+	// ones; an exposed port without binding has no Public. Sorted.
+	Ports       []Port
+	NetworkMode string
+	Networks    []Network // sorted by name
+}
+
+// Network is one network the container is attached to.
+type Network struct {
+	Name    string   `json:"name"`
+	IP      string   `json:"ip,omitempty"`
+	Gateway string   `json:"gateway,omitempty"`
+	MAC     string   `json:"mac,omitempty"`
+	Aliases []string `json:"aliases,omitempty"`
 }
 
 // Change is one entry of GET /containers/{id}/changes (F5).
@@ -64,6 +106,8 @@ type Mount struct {
 	Source      string // host path of a bind
 	Destination string // path in the container
 	Driver      string // volume driver ("local"…)
+	Mode        string // options given by the user ("ro", "z"…)
+	RW          bool
 }
 
 // ErrNotFound: no such container (or no such path, for an archive).
@@ -204,11 +248,13 @@ func fromInspect(c container.InspectResponse) ContainerInfo {
 	for _, m := range c.Mounts {
 		info.Mounts = append(info.Mounts, Mount{
 			Type: string(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: m.Driver,
+			Mode: m.Mode, RW: m.RW,
 		})
 	}
 	if c.Config != nil {
 		info.Tty = c.Config.Tty
 	}
+	info.Details = fromInspectDetails(c)
 	if c.HostConfig != nil {
 		info.LogDriver = c.HostConfig.LogConfig.Type
 		for d := range c.HostConfig.Tmpfs {
@@ -219,6 +265,91 @@ func fromInspect(c container.InspectResponse) ContainerInfo {
 	return info
 }
 
+func fromInspectDetails(c container.InspectResponse) Details {
+	d := Details{ImageID: c.Image, Created: c.Created, RestartCount: c.RestartCount}
+	if s := c.State; s != nil {
+		d.Status, d.ExitCode, d.Error, d.StartedAt, d.OOMKilled = string(s.Status), s.ExitCode, s.Error, s.StartedAt, s.OOMKilled
+		if s.Health != nil {
+			d.Health = string(s.Health.Status)
+		}
+	}
+	var exposed network.PortSet
+	if cf := c.Config; cf != nil {
+		d.Image, d.Entrypoint, d.Cmd, d.WorkingDir, d.User, d.Env = cf.Image, cf.Entrypoint, cf.Cmd, cf.WorkingDir, cf.User, cf.Env
+		exposed = cf.ExposedPorts
+	}
+	bindings := network.PortMap{}
+	if h := c.HostConfig; h != nil {
+		d.RestartPolicy, d.MaxRetry = string(h.RestartPolicy.Name), h.RestartPolicy.MaximumRetryCount
+		d.NetworkMode = string(h.NetworkMode)
+		bindings = h.PortBindings
+	}
+	if n := c.NetworkSettings; n != nil {
+		if c.State != nil && c.State.Running && len(n.Ports) > 0 {
+			bindings = n.Ports
+		}
+		for name, ep := range n.Networks {
+			nw := Network{Name: name}
+			if ep != nil {
+				nw.Aliases = ep.Aliases
+				if ep.IPAddress.IsValid() {
+					nw.IP = ep.IPAddress.String()
+				}
+				if ep.Gateway.IsValid() {
+					nw.Gateway = ep.Gateway.String()
+				}
+				if len(ep.MacAddress) > 0 {
+					nw.MAC = ep.MacAddress.String()
+				}
+			}
+			d.Networks = append(d.Networks, nw)
+		}
+		sort.Slice(d.Networks, func(i, j int) bool { return d.Networks[i].Name < d.Networks[j].Name })
+	}
+	d.Ports = portsOf(bindings, exposed)
+	return d
+}
+
+// portsOf lists each binding once (0.0.0.0 and :: are one mapping, as in
+// fromSummary), then the exposed ports without binding.
+func portsOf(bindings network.PortMap, exposed network.PortSet) []Port {
+	var out []Port
+	seen := map[Port]bool{}
+	for p, bs := range bindings {
+		for _, b := range bs {
+			ip := ""
+			if b.HostIP.IsValid() {
+				ip = b.HostIP.String()
+				if b.HostIP.IsUnspecified() {
+					ip = "0.0.0.0"
+				}
+			}
+			public, _ := strconv.ParseUint(b.HostPort, 10, 16)
+			port := Port{Private: p.Num(), Public: uint16(public), Type: string(p.Proto()), IP: ip}
+			if !seen[port] {
+				seen[port] = true
+				out = append(out, port)
+			}
+		}
+	}
+	for p := range exposed {
+		if len(bindings[p]) == 0 {
+			out = append(out, Port{Private: p.Num(), Type: string(p.Proto())})
+		}
+	}
+	sort.Slice(out, func(i, j int) bool {
+		a, b := out[i], out[j]
+		if a.Private != b.Private {
+			return a.Private < b.Private
+		}
+		if a.Type != b.Type {
+			return a.Type < b.Type
+		}
+		return a.IP < b.IP
+	})
+	return out
+}
+
 func (e *engine) GetArchive(ctx context.Context, id, path string) (io.ReadCloser, error) {
 	res, err := e.c.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: path})
 	if err != nil {
```

- [ ] **Step 4: Vérifier**

Run: `go test -race ./internal/dockerx/ ./internal/api/ 2>&1 | tail -n 3`
Expected: `ok`. Mutations : passer `d.Cmd` et `d.Entrypoint` bruts à `radioJSON` → `TestContainerRadiography` (« FUITE : s3cr3t-cmd ») ; `Error: d.Error` brut → `TestContainerRadiography` (« FUITE : s3cr3t-err ») ; ignorer l'erreur du journal dans `revealEnv` → `TestRevealEnvRefusals` (« journal en échec : 200 … s3cr3t-env »).

- [ ] **Step 5: Commit**

```bash
git add internal/dockerx internal/api
git commit -F - <<'EOF'
api, dockerx : radiographie (F7) aux secrets masqués, révélation journalisée

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 6: F6 et F7 de bout en bout sur un vrai Docker

**Où tester :** Docker Desktop (portable), puis `scripts/test-linux.sh` (suite complète).

**Files:**
- Create: `internal/api/integration_observation_test.go`

**Interfaces:**
- Consumes: `openSSE`, `lineOf`, `failureOf` (tâche 3), `auditLines` (tâche 5), `radioJSON`, `fixture.init` avec un vrai `dockerx.New()`.
- Produces: rien (vérification, §10 : « F1 à F7 de bout en bout côté API »).

> **À comprendre :** ce test ne protège pas un code à venir : il vérifie, contre un vrai démon, ce que les faux des tâches 3 et 5 supposent (format du flux, `since`, fin du flux à l'arrêt, pilote `none`, ports réels, réseau). Il est portable : ni les logs ni l'`Inspect` ne lisent `/proc`.

- [ ] **Step 1: Écrire le test**

```diff
diff --git a/internal/api/integration_observation_test.go b/internal/api/integration_observation_test.go
new file mode 100644
--- /dev/null
+++ b/internal/api/integration_observation_test.go
@@ -0,0 +1,136 @@
+//go:build integration
+
+package api
+
+// F6 and F7 end to end (spec §10): real Docker, real HTTP handler, a real
+// HTTP client for the SSE stream. Portable: logs and Inspect need no
+// /proc access, Docker Desktop answers from macOS.
+
+import (
+	"encoding/json"
+	"net/http"
+	"os/exec"
+	"strings"
+	"testing"
+	"time"
+
+	"github.com/levy-cmd/hublot-kit/internal/dockerx"
+)
+
+func dockerRun(t *testing.T, args ...string) {
+	t.Helper()
+	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
+		t.Fatalf("docker %v : %v %s", args, err, out)
+	}
+}
+
+func TestIntegrationObservation(t *testing.T) {
+	live, past, none := "hublot-it-obs-live", "hublot-it-obs-past", "hublot-it-obs-none"
+	exec.Command("docker", "rm", "-f", live, past, none).Run()
+	t.Cleanup(func() { exec.Command("docker", "rm", "-f", live, past, none).Run() })
+	dockerRun(t, "run", "-d", "--name", live, "-e", "DB_PASSWORD=s3cr3t-env", "-e", "PLAIN=visible",
+		"-p", "127.0.0.1::80", "alpine:3", "sh", "-c",
+		`echo "start s3cr3t-in-logs"; i=0; while true; do i=$((i+1)); echo "tick $i"; echo "warn $i" >&2; sleep 0.2; done`)
+	dockerRun(t, "run", "--name", past, "alpine:3", "sh", "-c", "echo un; echo deux >&2")
+	dockerRun(t, "run", "-d", "--name", none, "--log-driver", "none", "alpine:3", "sleep", "300")
+	dc, err := dockerx.New()
+	if err != nil {
+		t.Fatal(err)
+	}
+	f := &fixture{}
+	f.init(t, false, dc, nil)
+	c := f.session(t)
+
+	// F7: masked by default, revealed on demand, journaled.
+	rec := f.do("GET", "/api/containers/"+live, "", c)
+	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "s3cr3t-env") {
+		t.Fatalf("radiographie : %d %s", rec.Code, rec.Body)
+	}
+	var radio radioJSON
+	json.Unmarshal(rec.Body.Bytes(), &radio)
+	if radio.Image != "alpine:3" || !radio.State.Running || len(radio.Ports) != 1 || radio.Ports[0].Public == 0 ||
+		radio.LogDriver != "json-file" || len(radio.Networks) != 1 || radio.Networks[0].IP == "" {
+		t.Errorf("radiographie : %+v", radio)
+	}
+	for _, v := range radio.Env {
+		if v.Name == "DB_PASSWORD" && (!v.Masked || v.Value != nil) {
+			t.Errorf("DB_PASSWORD non masqué : %+v", v)
+		}
+	}
+	rec = f.do("POST", "/api/containers/"+live+"/env/reveal", `{"name":"DB_PASSWORD"}`, c)
+	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "s3cr3t-env") {
+		t.Fatalf("révélation : %d %s", rec.Code, rec.Body)
+	}
+	if log := auditLines(t, f); !strings.Contains(log, `"action":"env.reveal"`) || strings.Contains(log, "s3cr3t-env") {
+		t.Errorf("journal : %s", log)
+	}
+
+	// F6, running container: past lines, then new ones as they come, both
+	// streams; then resumption from the last ID.
+	s := openSSE(t, f, "/api/containers/"+live+"/logs?tail=5", c, nil)
+	streams := map[string]bool{}
+	var lastID string
+	for i := 0; i < 12; i++ {
+		e, ok := s.next(t)
+		if !ok {
+			t.Fatal("flux fermé trop tôt")
+		}
+		if e.Comment != "" {
+			continue
+		}
+		l := lineOf(t, e)
+		streams[l.Stream] = true
+		lastID = e.ID
+	}
+	if !streams["stdout"] || !streams["stderr"] || lastID == "" {
+		t.Fatalf("flux en direct : %v, dernier ID %q", streams, lastID)
+	}
+	s.cancel()
+	r := openSSE(t, f, "/api/containers/"+live+"/logs", c, map[string]string{"Last-Event-ID": lastID})
+	e, _ := r.next(t)
+	idTime := func(id string) time.Time {
+		ts, _, _ := strings.Cut(id, "/")
+		v, _ := time.Parse(time.RFC3339Nano, ts)
+		return v
+	}
+	if e.ID == "" || e.ID == lastID || idTime(e.ID).Before(idTime(lastID)) {
+		t.Errorf("reprise : premier ID %q, attendu après %q", e.ID, lastID)
+	}
+	// The container stops: the stream ends with "end".
+	go exec.Command("docker", "stop", "-t", "1", live).Run()
+	deadline := time.Now().Add(15 * time.Second)
+	for time.Now().Before(deadline) {
+		e, ok := r.next(t)
+		if !ok {
+			t.Fatal("flux fermé sans événement end")
+		}
+		if e.Event == "end" {
+			break
+		}
+		if e.Event == "failure" {
+			t.Fatalf("échec à l'arrêt : %+v", e)
+		}
+	}
+
+	// F6, stopped container: its past logs, then end.
+	events := openSSE(t, f, "/api/containers/"+past+"/logs", c, nil).all(t)
+	if len(events) != 3 || events[2].Event != "end" {
+		t.Fatalf("conteneur arrêté : %+v", events)
+	}
+	got := []string{lineOf(t, events[0]).Stream + ":" + lineOf(t, events[0]).Line, lineOf(t, events[1]).Stream + ":" + lineOf(t, events[1]).Line}
+	if !(got[0] == "stdout:un" && got[1] == "stderr:deux" || got[0] == "stderr:deux" && got[1] == "stdout:un") {
+		t.Errorf("lignes : %q", got)
+	}
+	dl := f.do("GET", "/api/containers/"+past+"/logs/download", "", c)
+	if dl.Code != http.StatusOK || !strings.Contains(dl.Body.String(), " stdout un\n") || !strings.Contains(dl.Body.String(), " stderr deux\n") {
+		t.Errorf("téléchargement : %d %q", dl.Code, dl.Body)
+	}
+
+	// A driver Docker cannot read: a clear code, in the stream and on the download.
+	if code := failureOf(t, openSSE(t, f, "/api/containers/"+none+"/logs", c, nil).all(t)); code != "logs_unavailable" {
+		t.Errorf("--log-driver none : %s", code)
+	}
+	if rec := f.do("GET", "/api/containers/"+none+"/logs/download", "", c); rec.Code != http.StatusUnprocessableEntity {
+		t.Errorf("--log-driver none, téléchargement : %d %s", rec.Code, rec.Body)
+	}
+}
```

- [ ] **Step 2: Le lancer**

Run: `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock go test -tags integration -count=1 ./internal/api/ -run Observation -v 2>&1 | grep -E '^(---|ok|FAIL)' | tail -n 3`
Expected: `--- PASS: TestIntegrationObservation` (environ 3 s).

- [ ] **Step 3: Suite complète sous Linux**

Run: `scripts/test-linux.sh -race -tags integration -count=1 ./cmd/... ./internal/... 2>&1 | tail -n 12`
Expected: `ok` pour chaque paquet, `internal/secrets` compris.

- [ ] **Step 4: Commit**

```bash
git add internal/api/integration_observation_test.go
git commit -F - <<'EOF'
api : F6 et F7 de bout en bout sur un vrai Docker

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 7: Interface — onglet Logs

**Où tester :** macOS (Vitest, `check`, `lint`) ; l'onglet lui-même par le parcours jetable de la tâche 8.

**Files:**
- Create: `web/src/lib/logs.ts`, `web/src/lib/logs.test.ts`, `web/src/lib/components/LogsView.svelte`
- Modify: `web/src/lib/api.ts`, `web/src/lib/i18n/fr.ts`, `web/src/lib/i18n/en.ts`, `web/src/lib/i18n/index.ts`, `web/src/routes/c/[id]/+page.svelte`
- Test: `web/src/lib/api.test.ts`, `web/src/lib/i18n/i18n.test.ts`

**Interfaces:**
- Consumes: `GET /api/containers/{id}/logs` (événements de la tâche 3), `GET /logs/download`, `getMeta`, `ApiError`, `errorMessage`, `goto`.
- Produces: `type LogLine = { ts?: string; stream: 'stdout' | 'stderr'; line: string; truncated?: boolean }` ; `logsUrl(id, { tail?, after? })`, `logsDownloadUrl(id)` ; dans `logs.ts` : `MAX_LINES = 5000`, `ShownLine`, `parseLine`, `parseCount`, `parseFailure`, `streamEnd(name: 'end' | 'failure' | 'evicted', data?) → { status, code?, login? }` (R16), `appendLines(lines, batch, max?) → { lines, removed }`, `filterLines(lines, { query, stdout, stderr })`, `formatTs(ts, timeZone?)` ; `LogsView` (`{ id }`) ; onglet `?tab=logs` ; code `logs_unavailable` dans `API_ERROR_CODES` et ses traductions ; clé `logs.evicted`.

> **À comprendre :** `EventSource` rappelle `onmessage` pour chaque ligne (événement sans nom) et `addEventListener(nom)` pour `end`, `dropped`, `failure`. Son `onerror` sert à deux choses : `readyState === CONNECTING`, le navigateur se reconnecte seul (avec `Last-Event-ID`) ; `CLOSED`, une réponse non SSE (session expirée) : on demande `/api/meta` pour savoir si c'est un `401`. Les lignes arrivent par centaines : elles attendent dans `pending` et rejoignent `lines` une fois par image (`requestAnimationFrame`), sinon Svelte rendrait la liste à chaque ligne. La pause ferme le flux et garde `lastId` ; reprendre rouvre avec `after` (Q30). `failure` `unauthorized` (session finie) renvoie à la connexion ; `evicted` ferme le flux sans reconnexion et propose « Recharger » (Q37). Le lien de téléchargement disparaît quand le flux est en erreur : le navigateur enregistrerait sinon le JSON de l'erreur dans un `.log` (R9). L'effet ne lit pas `status` (il l'écrit) : sinon chaque changement d'état rouvrirait le flux.

- [ ] **Step 1: Tests qui échouent**

```diff
diff --git a/web/src/lib/api.test.ts b/web/src/lib/api.test.ts
--- a/web/src/lib/api.test.ts
+++ b/web/src/lib/api.test.ts
@@ -1,5 +1,5 @@
 import { afterEach, describe, expect, it, vi } from 'vitest';
-import { api, ApiError, checkDownload, downloadUrl, listDir, login, previewFile, saveFile, uploadFile } from './api';
+import { api, ApiError, checkDownload, downloadUrl, listDir, login, logsDownloadUrl, logsUrl, previewFile, saveFile, uploadFile } from './api';
 
 afterEach(() => vi.unstubAllGlobals());
 
@@ -151,3 +151,14 @@ describe('writes (F4)', () => {
 		await expect(saveFile('web', '/f', 'x', '"a"')).rejects.toMatchObject({ status: 409, code: 'conflict' });
 	});
 });
+
+describe('logs routes (F6)', () => {
+	it('builds the stream and download URLs', () => {
+		expect(logsUrl('web')).toBe('/api/containers/web/logs');
+		expect(logsUrl('web', { tail: 200 })).toBe('/api/containers/web/logs?tail=200');
+		expect(logsUrl('a b', { after: '2026-10-10T07:14:22.769Z/0' })).toBe(
+			'/api/containers/a%20b/logs?after=2026-10-10T07%3A14%3A22.769Z%2F0'
+		);
+		expect(logsDownloadUrl('web')).toBe('/api/containers/web/logs/download');
+	});
+});
diff --git a/web/src/lib/i18n/i18n.test.ts b/web/src/lib/i18n/i18n.test.ts
--- a/web/src/lib/i18n/i18n.test.ts
+++ b/web/src/lib/i18n/i18n.test.ts
@@ -15,6 +15,10 @@ describe('i18n', () => {
 		}
 	});
 
+	it('knows the code of an unreadable log driver (F6)', () => {
+		expect(API_ERROR_CODES).toContain('logs_unavailable');
+	});
+
 	it('unknown codes fall back to a generic message', () => {
 		expect(errorMessage('nope')).toBe(errorMessage('unknown'));
 	});
diff --git a/web/src/lib/logs.test.ts b/web/src/lib/logs.test.ts
new file mode 100644
--- /dev/null
+++ b/web/src/lib/logs.test.ts
@@ -0,0 +1,59 @@
+import { describe, expect, it } from 'vitest';
+import type { LogLine } from './api';
+import { appendLines, filterLines, formatTs, MAX_LINES, parseCount, parseFailure, parseLine, streamEnd } from './logs';
+
+const line = (text: string, stream: 'stdout' | 'stderr' = 'stdout'): LogLine => ({ ts: '2026-10-10T07:14:22.769046552Z', stream, line: text });
+
+describe('logs (F6)', () => {
+	it('parses a line event, refuses anything else', () => {
+		expect(parseLine('{"ts":"2026-10-10T07:14:22Z","stream":"stderr","line":"x","truncated":true}')).toEqual({
+			ts: '2026-10-10T07:14:22Z', stream: 'stderr', line: 'x', truncated: true
+		});
+		expect(parseLine('{"stream":"stdout","line":""}')).toEqual({ stream: 'stdout', line: '' });
+		for (const bad of ['', 'nope', '{}', '{"stream":"stdin","line":"x"}', '{"stream":"stdout","line":3}', 'null']) {
+			expect(parseLine(bad)).toBeNull();
+		}
+	});
+
+	it('parses the dropped and failure events', () => {
+		expect(parseCount('{"count":12}')).toBe(12);
+		expect(parseCount('{"count":"x"}')).toBe(0);
+		expect(parseCount('garbage')).toBe(0);
+		expect(parseFailure('{"error":"logs_unavailable","detail":"none"}')).toBe('logs_unavailable');
+		expect(parseFailure('garbage')).toBe('unknown');
+	});
+
+	it('keeps at most MAX_LINES, the newest, and counts the others', () => {
+		const first = appendLines([], [line('a'), line('b')], 3);
+		expect(first.lines.map((l) => l.line)).toEqual(['a', 'b']);
+		expect(first.lines.map((l) => l.key)).toEqual([0, 1]);
+		const next = appendLines(first.lines, [line('c'), line('d')], 3);
+		expect(next.lines.map((l) => l.line)).toEqual(['b', 'c', 'd']);
+		expect(next.removed).toBe(1);
+		expect(next.lines.map((l) => l.key)).toEqual([1, 2, 3]); // keys never reused
+		expect(MAX_LINES).toBe(5000);
+	});
+
+	it('filters by stream and by a case-insensitive text', () => {
+		const ls = appendLines([], [line('GET /health 200'), line('Error: boom', 'stderr'), line('get /api 500')]).lines;
+		expect(filterLines(ls, { query: '', stdout: true, stderr: true })).toHaveLength(3);
+		expect(filterLines(ls, { query: 'get', stdout: true, stderr: true }).map((l) => l.line)).toEqual(['GET /health 200', 'get /api 500']);
+		expect(filterLines(ls, { query: '', stdout: false, stderr: true }).map((l) => l.line)).toEqual(['Error: boom']);
+		expect(filterLines(ls, { query: '  boom ', stdout: true, stderr: false })).toEqual([]);
+	});
+
+	it('formats a timestamp to the millisecond', () => {
+		expect(formatTs('2026-10-10T07:14:22.769046552Z', 'UTC')).toBe('07:14:22.769');
+		expect(formatTs(undefined, 'UTC')).toBe('');
+		expect(formatTs('garbage', 'UTC')).toBe('');
+	});
+
+	// Q37: how each end of a stream is handled; none reconnects.
+	it('decides what an end of stream does', () => {
+		expect(streamEnd('end')).toEqual({ status: 'ended' });
+		expect(streamEnd('failure', '{"error":"unauthorized","detail":"session terminée"}')).toEqual({ status: 'error', code: 'unauthorized', login: true });
+		expect(streamEnd('failure', '{"error":"logs_unavailable"}')).toEqual({ status: 'error', code: 'logs_unavailable' });
+		expect(streamEnd('failure', 'garbage')).toEqual({ status: 'error', code: 'unknown' });
+		expect(streamEnd('evicted', '{"error":"internal"}')).toEqual({ status: 'error', code: 'evicted' });
+	});
+});
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `cd web && npx vitest run 2>&1 | tail -n 6`
Expected: `Test Files  3 failed` (`logs.test.ts` ne trouve pas `./logs`, `api.test.ts` ne trouve pas `logsUrl`, `i18n.test.ts` : `logs_unavailable` absent de `API_ERROR_CODES`).

- [ ] **Step 3: Implémenter**

```diff
diff --git a/web/src/lib/api.ts b/web/src/lib/api.ts
--- a/web/src/lib/api.ts
+++ b/web/src/lib/api.ts
@@ -135,3 +135,17 @@ export function uploadFile(id: string, dir: string, file: File): Promise<{ path:
 		headers: { 'If-None-Match': '*' }
 	});
 }
+
+// F6 (spec §6): an SSE stream, one event per line; "end", "dropped" and
+// "failure" events. after resumes after a line ID (pause, "follow again").
+export type LogLine = { ts?: string; stream: 'stdout' | 'stderr'; line: string; truncated?: boolean };
+
+export function logsUrl(id: string, opts: { tail?: number; after?: string } = {}): string {
+	const q = new URLSearchParams();
+	if (opts.tail !== undefined) q.set('tail', String(opts.tail));
+	if (opts.after) q.set('after', opts.after);
+	const query = q.toString();
+	return `/api/containers/${encodeURIComponent(id)}/logs${query ? '?' + query : ''}`;
+}
+
+export const logsDownloadUrl = (id: string) => `/api/containers/${encodeURIComponent(id)}/logs/download`;
diff --git a/web/src/lib/components/LogsView.svelte b/web/src/lib/components/LogsView.svelte
new file mode 100644
--- /dev/null
+++ b/web/src/lib/components/LogsView.svelte
@@ -0,0 +1,178 @@
+<script lang="ts">
+	import { goto } from '$app/navigation';
+	import { ApiError, getMeta, logsDownloadUrl, logsUrl, type LogLine } from '#lib/api.ts';
+	import { errorMessage, t } from '#lib/i18n/index.ts';
+	import { appendLines, filterLines, formatTs, parseCount, parseLine, streamEnd, type ShownLine } from '#lib/logs.ts';
+
+	let { id }: { id: string } = $props();
+
+	type Status = 'connecting' | 'live' | 'reconnecting' | 'paused' | 'ended' | 'error';
+	let status = $state<Status>('connecting');
+	let error = $state('');
+	let lines = $state<ShownLine[]>([]);
+	let dropped = $state(0); // announced by the agent (rate)
+	let removed = $state(0); // left the view (MAX_LINES)
+	let query = $state('');
+	let stdout = $state(true);
+	let stderr = $state(true);
+	let timestamps = $state(true);
+	let paused = $state(false);
+	let restarts = $state(0); // bumped by Resume / Follow again / Reload
+	let shown = $derived(filterLines(lines, { query, stdout, stderr }));
+	let box = $state<HTMLElement | null>(null);
+	let atBottom = true; // the view follows the end unless the user scrolled up
+
+	let lastId = ''; // ID of the last line received: where a resumption starts
+	let pending: LogLine[] = [];
+	let frame = 0;
+
+	// Lines arrive by hundreds: they are added once per animation frame.
+	function flush() {
+		frame = 0;
+		const r = appendLines(lines, pending);
+		pending = [];
+		lines = r.lines;
+		removed += r.removed;
+	}
+
+	$effect(() => {
+		const wanted = id;
+		void restarts;
+		if (paused) {
+			status = 'paused'; // the stream is closed; lastId keeps the place
+			return;
+		}
+		status = 'connecting';
+		error = '';
+		const es = new EventSource(logsUrl(wanted, lastId ? { after: lastId } : {}));
+		const stop = (next: Status) => {
+			es.close();
+			status = next;
+		};
+		es.onopen = () => {
+			status = 'live';
+		};
+		es.onmessage = (e: MessageEvent<string>) => {
+			const l = parseLine(e.data);
+			if (!l) return;
+			if (e.lastEventId) lastId = e.lastEventId;
+			pending.push(l);
+			if (!frame) frame = requestAnimationFrame(flush);
+		};
+		es.addEventListener('dropped', (e) => {
+			dropped += parseCount((e as MessageEvent<string>).data);
+		});
+		for (const name of ['end', 'failure', 'evicted'] as const) {
+			es.addEventListener(name, (e) => {
+				const r = streamEnd(name, (e as MessageEvent<string>).data);
+				stop(r.status);
+				if (r.login) goto('/login').catch(() => {});
+				if (r.code) error = r.code === 'evicted' ? t('logs.evicted') : errorMessage(r.code);
+			});
+		}
+		es.onerror = () => {
+			if (es.readyState === EventSource.CONNECTING) {
+				status = 'reconnecting'; // the browser retries with Last-Event-ID
+				return;
+			}
+			if (status === 'ended' || status === 'error') return;
+			stop('error');
+			// An HTTP error (401 after the session expired): EventSource cannot
+			// read it, /api/meta tells.
+			getMeta()
+				.then(() => (error = errorMessage('network')))
+				.catch((err) => {
+					if (err instanceof ApiError && err.status === 401) goto('/login').catch(() => {});
+					else error = errorMessage(err instanceof ApiError ? err.code : 'unknown');
+				});
+		};
+		return () => {
+			es.close(); // leaving the tab frees the connection (6 per host in HTTP/1.1)
+			if (frame) cancelAnimationFrame(frame);
+			frame = 0;
+			if (pending.length) flush();
+		};
+	});
+
+	function pause() {
+		paused = true;
+	}
+
+	function resume() {
+		paused = false;
+		restarts++;
+	}
+
+	function onScroll() {
+		if (box) atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 8;
+	}
+
+	$effect(() => {
+		void shown.length;
+		if (box && atBottom) box.scrollTop = box.scrollHeight;
+	});
+</script>
+
+<section class="logs">
+	<div class="tools">
+		<input type="search" placeholder={t('logs.search')} aria-label={t('logs.search')} bind:value={query} />
+		<label><input type="checkbox" bind:checked={stdout} /> stdout</label>
+		<label><input type="checkbox" bind:checked={stderr} /> stderr</label>
+		<label><input type="checkbox" bind:checked={timestamps} /> {t('logs.timestamps')}</label>
+		{#if status === 'paused'}
+			<button type="button" onclick={resume}>{t('logs.resume')}</button>
+		{:else if status === 'live' || status === 'connecting' || status === 'reconnecting'}
+			<button type="button" onclick={pause}>{t('logs.pause')}</button>
+		{/if}
+		{#if status !== 'error'}<a href={logsDownloadUrl(id)} download>{t('logs.download')}</a>{/if}
+	</div>
+	<p class="status" role="status">
+		{#if status === 'connecting'}{t('logs.connecting')}
+		{:else if status === 'live'}<span class="live">●</span> {t('logs.live')}
+		{:else if status === 'reconnecting'}{t('logs.reconnecting')}
+		{:else if status === 'paused'}{t('logs.paused')}
+		{:else if status === 'ended'}{t('logs.ended')}{/if}
+		· {shown.length} {t('logs.shown')}
+	</p>
+	{#if status === 'ended'}
+		<button type="button" onclick={resume}>{t('logs.follow')}</button>
+	{/if}
+	{#if error}
+		<p role="alert">{error}</p>
+		<button type="button" onclick={resume}>{t('files.reload')}</button>
+	{/if}
+	{#if dropped}<p class="note">{dropped} {t('logs.dropped')}</p>{/if}
+	{#if removed}<p class="note">{removed} {t('logs.removed')}</p>{/if}
+	<div class="lines" bind:this={box} onscroll={onScroll}>
+		{#if lines.length === 0 && status !== 'connecting'}
+			<p class="note">{t('logs.empty')}</p>
+		{:else if shown.length === 0 && lines.length > 0}
+			<p class="note">{t('logs.noMatch')}</p>
+		{/if}
+		{#each shown as l (l.key)}
+			<div class="line {l.stream}" class:nots={!timestamps}>
+				{#if timestamps}<time datetime={l.ts}>{formatTs(l.ts)}</time>{/if}
+				<span class="text">{l.line}</span>
+				{#if l.truncated}<abbr class="cut" title={t('logs.truncated')}>…</abbr>{/if}
+			</div>
+		{/each}
+	</div>
+</section>
+
+<style>
+	.tools { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; align-items: center; }
+	.tools input[type='search'] { flex: 1 1 12rem; min-width: 0; }
+	.status { margin: 0.5rem 0; font-size: 0.9rem; }
+	.live { color: #1b7f3b; }
+	.note { font-style: italic; }
+	.lines {
+		font-family: ui-monospace, monospace; font-size: 0.85rem; max-height: 70vh; overflow: auto;
+		border: 1px solid #ddd; padding: 0.25rem 0.5rem; background: #fafafa;
+	}
+	.line { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; gap: 0.75rem; white-space: pre-wrap; overflow-wrap: anywhere; }
+	.line.nots { grid-template-columns: minmax(0, 1fr) auto; }
+	.line time { color: #666; }
+	.line.stderr .text { color: #b00020; }
+	.cut { color: #8a5a00; text-decoration: none; }
+	[role='alert'] { color: #b00020; }
+</style>
diff --git a/web/src/lib/i18n/en.ts b/web/src/lib/i18n/en.ts
--- a/web/src/lib/i18n/en.ts
+++ b/web/src/lib/i18n/en.ts
@@ -114,7 +114,26 @@ export const en: Record<Key, string> = {
 	'error.forbidden_path': 'Forbidden path.',
 	'error.link_in_path': 'The path contains a symbolic link.',
 	'error.docker_unavailable': 'Docker is not responding. Check the daemon and the agent’s socket access.',
+	'error.logs_unavailable': "Unreadable logs: this container's logging driver cannot be read (none, syslog…).",
 	'error.internal': 'Agent internal error.',
+	'logs.search': 'Search the logs',
+	'logs.timestamps': 'Timestamps',
+	'logs.pause': 'Pause',
+	'logs.resume': 'Resume',
+	'logs.download': 'Download the logs',
+	'logs.connecting': 'Connecting to the stream…',
+	'logs.live': 'Live',
+	'logs.paused': 'Paused: the lines that arrive meanwhile will be caught up.',
+	'logs.reconnecting': 'Connection lost, reconnecting…',
+	'logs.ended': 'End of the stream: the container is not running, or has just stopped.',
+	'logs.evicted': 'Stream closed: a newer logs stream took its place (8 at most for the agent). Reload to open it again.',
+	'logs.follow': 'Follow again',
+	'logs.empty': 'No line.',
+	'logs.noMatch': 'No line matches.',
+	'logs.dropped': 'line(s) not shown, rate too high: download the logs to see them.',
+	'logs.removed': 'older line(s) left the view (5,000 at most): download the logs to see them.',
+	'logs.truncated': 'line cut at 16 KiB (whole in the download)',
+	'logs.shown': 'line(s) shown',
 	'changes.hint': 'Differences from the image, computed by Docker (volumes excluded).',
 	'changes.empty': 'No change from the image.',
 	'changes.added': 'added',
diff --git a/web/src/lib/i18n/fr.ts b/web/src/lib/i18n/fr.ts
--- a/web/src/lib/i18n/fr.ts
+++ b/web/src/lib/i18n/fr.ts
@@ -117,7 +117,26 @@ export const fr = {
 	'error.forbidden_path': 'Chemin interdit.',
 	'error.link_in_path': 'Le chemin contient un lien symbolique.',
 	'error.docker_unavailable': "Docker ne répond pas. Vérifiez que le démon tourne et que l'agent accède au socket.",
+	'error.logs_unavailable': "Logs illisibles : le pilote de logs de ce conteneur ne permet pas la lecture (none, syslog…).",
 	'error.internal': "Erreur interne de l'agent.",
+	'logs.search': 'Rechercher dans les logs',
+	'logs.timestamps': 'Horodatage',
+	'logs.pause': 'Pause',
+	'logs.resume': 'Reprendre',
+	'logs.download': 'Télécharger les logs',
+	'logs.connecting': 'Connexion au flux…',
+	'logs.live': 'En direct',
+	'logs.paused': 'En pause : les lignes arrivées entre-temps seront reprises.',
+	'logs.reconnecting': 'Connexion perdue, reconnexion…',
+	'logs.ended': "Fin du flux : le conteneur ne tourne pas, ou vient de s'arrêter.",
+	'logs.evicted': "Flux fermé : un flux de logs plus récent a pris sa place (8 au plus pour l'agent). Rechargez pour le rouvrir.",
+	'logs.follow': 'Suivre à nouveau',
+	'logs.empty': 'Aucune ligne.',
+	'logs.noMatch': 'Aucune ligne ne correspond.',
+	'logs.dropped': 'ligne(s) non affichée(s), débit trop élevé : téléchargez les logs pour les voir.',
+	'logs.removed': 'ligne(s) plus ancienne(s) retirée(s) de la vue (5 000 au plus) : téléchargez les logs pour les voir.',
+	'logs.truncated': 'ligne tronquée à 16 Kio (complète dans le téléchargement)',
+	'logs.shown': 'ligne(s) affichée(s)',
 	'changes.hint': "Différences avec l'image, calculées par Docker (hors volumes).",
 	'changes.empty': "Aucun changement par rapport à l'image.",
 	'changes.added': 'ajouté',
diff --git a/web/src/lib/i18n/index.ts b/web/src/lib/i18n/index.ts
--- a/web/src/lib/i18n/index.ts
+++ b/web/src/lib/i18n/index.ts
@@ -8,7 +8,7 @@ export type Lang = 'fr' | 'en';
 export const API_ERROR_CODES = [
 	'unauthorized', 'invalid_name', 'forbidden_origin', 'rate_limited', 'not_found', 'read_only',
 	'conflict', 'container_changed', 'indexing', 'too_large', 'not_regular', 'forbidden_path',
-	'link_in_path', 'docker_unavailable', 'internal', 'network', 'unknown'
+	'link_in_path', 'docker_unavailable', 'logs_unavailable', 'internal', 'network', 'unknown'
 ] as const;
 
 const dicts: Record<Lang, Record<Key, string>> = { fr, en };
diff --git a/web/src/lib/logs.ts b/web/src/lib/logs.ts
new file mode 100644
--- /dev/null
+++ b/web/src/lib/logs.ts
@@ -0,0 +1,83 @@
+import type { LogLine } from './api.ts';
+
+/** Lines kept by the view; older ones leave it (the download has them all). */
+export const MAX_LINES = 5000;
+
+/** A line of the view: key is local and never reused (keyed {#each}). */
+export type ShownLine = LogLine & { key: number };
+
+/** The data of a line event, or null if it is not one. */
+export function parseLine(data: string): LogLine | null {
+	let v: unknown;
+	try {
+		v = JSON.parse(data);
+	} catch {
+		return null;
+	}
+	if (typeof v !== 'object' || v === null) return null;
+	const o = v as Record<string, unknown>;
+	if ((o.stream !== 'stdout' && o.stream !== 'stderr') || typeof o.line !== 'string') return null;
+	const out: LogLine = { stream: o.stream, line: o.line };
+	if (typeof o.ts === 'string') out.ts = o.ts;
+	if (o.truncated === true) out.truncated = true;
+	return out;
+}
+
+/** The count of a "dropped" event (0 if unreadable). */
+export function parseCount(data: string): number {
+	try {
+		const n = JSON.parse(data)?.count;
+		return typeof n === 'number' && n > 0 ? n : 0;
+	} catch {
+		return 0;
+	}
+}
+
+/** The error code of a "failure" event. */
+export function parseFailure(data: string): string {
+	try {
+		const code = JSON.parse(data)?.error;
+		return typeof code === 'string' ? code : 'unknown';
+	} catch {
+		return 'unknown';
+	}
+}
+
+/** What the view does at an end of stream (Q29, Q37): never reconnect. */
+export type StreamEnd = { status: 'ended' | 'error'; code?: string; login?: boolean };
+
+/**
+ * "end": the container's stream ended. "failure": an error of §6;
+ * "unauthorized" (logout, expiry) sends to the login page. "evicted": a
+ * newer stream took the slot.
+ */
+export function streamEnd(name: 'end' | 'failure' | 'evicted', data = ''): StreamEnd {
+	if (name === 'end') return { status: 'ended' };
+	if (name === 'evicted') return { status: 'error', code: 'evicted' };
+	const code = parseFailure(data);
+	return code === 'unauthorized' ? { status: 'error', code, login: true } : { status: 'error', code };
+}
+
+/** Appends a batch, keeps the newest max lines; removed counts the others. */
+export function appendLines(lines: ShownLine[], batch: LogLine[], max = MAX_LINES): { lines: ShownLine[]; removed: number } {
+	let key = lines.length ? lines[lines.length - 1].key + 1 : 0;
+	const all = lines.concat(batch.map((l) => ({ ...l, key: key++ })));
+	const removed = Math.max(0, all.length - max);
+	return { lines: removed ? all.slice(removed) : all, removed };
+}
+
+export type LogFilter = { query: string; stdout: boolean; stderr: boolean };
+
+/** The lines of the checked streams that contain the query (case ignored). */
+export function filterLines(lines: ShownLine[], f: LogFilter): ShownLine[] {
+	const q = f.query.trim().toLowerCase();
+	return lines.filter((l) => (l.stream === 'stdout' ? f.stdout : f.stderr) && (q === '' || l.line.toLowerCase().includes(q)));
+}
+
+/** HH:MM:SS.mmm of an RFC 3339 timestamp, in timeZone (default: the browser's). */
+export function formatTs(ts: string | undefined, timeZone?: string): string {
+	if (!ts) return '';
+	const d = new Date(ts);
+	if (Number.isNaN(d.getTime())) return '';
+	return d.toLocaleTimeString('en-GB', { hour12: false, timeZone, hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 });
+}
diff --git a/web/src/routes/c/[id]/+page.svelte b/web/src/routes/c/[id]/+page.svelte
--- a/web/src/routes/c/[id]/+page.svelte
+++ b/web/src/routes/c/[id]/+page.svelte
@@ -7,6 +7,7 @@
 	import { errorMessage, t } from '#lib/i18n/index.ts';
 	import ChangesList from '#lib/components/ChangesList.svelte';
 	import FileBrowser from '#lib/components/FileBrowser.svelte';
+	import LogsView from '#lib/components/LogsView.svelte';
 
 	let id = $derived(page.params.id ?? '');
 	let container = $state<Container | null>(null);
@@ -14,7 +15,8 @@
 	let reloads = $state(0); // bumped by the Reload button
 	// From the layout's /api/meta: true until it answers (spec §9).
 	const readOnly = getContext<() => boolean>('readOnly');
-	let tab = $derived(page.url.searchParams.get('tab') === 'changes' ? 'changes' : 'files');
+	const TABS = ['files', 'changes', 'logs'] as const;
+	let tab = $derived(TABS.find((x) => x === page.url.searchParams.get('tab')) ?? 'files');
 
 	// The list gives the name, image and state; GET /api/containers/{id}
 	// arrives with the X-ray tab (phase Observation).
@@ -56,11 +58,13 @@
 		<nav class="tabs">
 			<a class="tab" class:active={tab === 'files'} aria-current={tab === 'files' ? 'page' : undefined} href="?">{t('container.tab.files')}</a>
 			<a class="tab" class:active={tab === 'changes'} aria-current={tab === 'changes' ? 'page' : undefined} href="?tab=changes">{t('container.tab.changes')}</a>
-			<span class="tab soon">{t('container.tab.logs')} ({t('container.soon')})</span>
+			<a class="tab" class:active={tab === 'logs'} aria-current={tab === 'logs' ? 'page' : undefined} href="?tab=logs">{t('container.tab.logs')}</a>
 			<span class="tab soon">{t('container.tab.xray')} ({t('container.soon')})</span>
 		</nav>
 		{#if tab === 'changes'}
 			<ChangesList id={container.id} />
+		{:else if tab === 'logs'}
+			<LogsView id={container.id} />
 		{:else}
 			<FileBrowser
 				id={container.id}
```

- [ ] **Step 4: Vérifier**

Run: `cd web && npx vitest run 2>&1 | tail -n 4 && npm run check 2>&1 | tail -n 2 && npm run lint 2>&1 | tail -n 4`
Expected: 77 tests verts une fois la tâche 8 faite (70 ici, dont `decides what an end of stream does`) ; `0 ERRORS 0 WARNINGS` ; `lint` sans message.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -F - <<'EOF'
web : onglet Logs (flux en direct, recherche, stdout/stderr, pause, reprise, téléchargement)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 8: Interface — onglet Radiographie, en-tête par `GET /api/containers/{id}`

**Où tester :** macOS (Vitest, `check`, `lint`, `build`), puis un vrai agent (parcours jetable).

**Files:**
- Create: `web/src/lib/xray.ts`, `web/src/lib/xray.test.ts`, `web/src/lib/components/XrayView.svelte`
- Modify: `web/src/lib/api.ts`, `web/src/lib/i18n/fr.ts`, `web/src/lib/i18n/en.ts`, `web/src/routes/c/[id]/+page.svelte`
- Test: `web/src/lib/api.test.ts`

**Interfaces:**
- Consumes: `GET /api/containers/{id}`, `POST /env/reveal` (tâche 5), `stateLabel`, `LogsView` (tâche 7).
- Produces: `type Radiography`, `EnvVar`, `Network`, `Mount` ; `getContainer(id)`, `revealEnv(id, name) → Promise<string>` ; dans `xray.ts` : `commandLine(args)`, `portLabel(p)`, `mountLabel(m)`, `restartLabel(policy, count)`, `shortId(id)`, `envValue(v, revealed)` ; `XrayView` (`{ radio }`) ; onglet `?tab=xray` ; la page lit `GET /api/containers/{id}` (plus la liste) ; la clé `container.soon` disparaît.

> **À comprendre :** l'interface ne masque rien : elle montre ce que l'agent envoie (`null` → `•••••• (masquée)`, et les `••••••` de la commande tels quels). Une valeur révélée vit dans l'état du composant, jamais dans `localStorage` ni dans l'URL, et « Masquer » l'oublie. `commandLine` met entre apostrophes ce qu'un shell couperait, pour qu'on lise la commande comme on la taperait.

- [ ] **Step 1: Tests qui échouent**

```diff
diff --git a/web/src/lib/api.test.ts b/web/src/lib/api.test.ts
--- a/web/src/lib/api.test.ts
+++ b/web/src/lib/api.test.ts
@@ -1,5 +1,7 @@
 import { afterEach, describe, expect, it, vi } from 'vitest';
-import { api, ApiError, checkDownload, downloadUrl, listDir, login, logsDownloadUrl, logsUrl, previewFile, saveFile, uploadFile } from './api';
+import {
+	api, ApiError, checkDownload, downloadUrl, getContainer, listDir, login, logsDownloadUrl, logsUrl, previewFile, revealEnv, saveFile, uploadFile
+} from './api';
 
 afterEach(() => vi.unstubAllGlobals());
 
@@ -162,3 +164,23 @@ describe('logs routes (F6)', () => {
 		expect(logsDownloadUrl('web')).toBe('/api/containers/web/logs/download');
 	});
 });
+
+describe('radiography (F7)', () => {
+	it('gets a container by an encoded id', async () => {
+		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ id: 'abc', name: 'web' })));
+		vi.stubGlobal('fetch', fetchMock);
+		await expect(getContainer('a b')).resolves.toMatchObject({ name: 'web' });
+		expect(fetchMock.mock.calls[0][0]).toBe('/api/containers/a%20b');
+	});
+
+	it('reveals one variable by a JSON POST', async () => {
+		const fetchMock = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ name: 'DB_PASSWORD', value: 's3cr3t' })));
+		vi.stubGlobal('fetch', fetchMock);
+		await expect(revealEnv('web', 'DB_PASSWORD')).resolves.toBe('s3cr3t');
+		const [url, init] = fetchMock.mock.calls[0];
+		expect(url).toBe('/api/containers/web/env/reveal');
+		expect(init?.method).toBe('POST');
+		expect(init?.body).toBe('{"name":"DB_PASSWORD"}');
+		expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json');
+	});
+});
diff --git a/web/src/lib/xray.test.ts b/web/src/lib/xray.test.ts
new file mode 100644
--- /dev/null
+++ b/web/src/lib/xray.test.ts
@@ -0,0 +1,47 @@
+import { describe, expect, it } from 'vitest';
+import type { Radiography } from './api';
+import { commandLine, envValue, mountLabel, portLabel, restartLabel, shortId } from './xray';
+
+describe('xray (F7)', () => {
+	it('shows a command as a shell would read it, masked parts kept', () => {
+		expect(commandLine(['nginx', '-g', 'daemon off;'])).toBe("nginx -g 'daemon off;'");
+		expect(commandLine(['redis-server', '--requirepass', '••••••'])).toBe('redis-server --requirepass ••••••');
+		expect(commandLine(['sh', '-c', "echo 'a'"])).toBe(`sh -c 'echo '\\''a'\\'''`);
+		expect(commandLine(['a', ''])).toBe("a ''");
+		expect(commandLine([])).toBe('');
+	});
+
+	it('labels ports, mounts and the restart policy', () => {
+		expect(portLabel({ private: 80, public: 8080, type: 'tcp', ip: '127.0.0.1' })).toBe('127.0.0.1:8080 → 80/tcp');
+		expect(portLabel({ private: 80, public: 8080, type: 'tcp', ip: '0.0.0.0' })).toBe('8080 → 80/tcp');
+		expect(portLabel({ private: 9000, type: 'tcp' })).toBe('9000/tcp');
+		const m = { type: 'bind', source: '/srv/conf', destination: '/etc/app', mode: 'ro', rw: false };
+		expect(mountLabel(m)).toBe('/srv/conf → /etc/app');
+		expect(mountLabel({ type: 'volume', name: 'data', source: '/var/lib/docker/volumes/data/_data', destination: '/data', rw: true })).toBe('data → /data');
+		expect(restartLabel({ name: 'on-failure', maxRetry: 5 }, 3)).toBe('on-failure (5) · 3');
+		expect(restartLabel({ name: 'no' }, 0)).toBe('no · 0');
+		expect(restartLabel({ name: '' }, 1)).toBe('no · 1');
+	});
+
+	it('shortens an image ID', () => {
+		expect(shortId('sha256:0123456789abcdef0123')).toBe('0123456789ab');
+		expect(shortId('')).toBe('');
+	});
+
+	it('shows a revealed value, never a masked one', () => {
+		const masked = { name: 'DB_PASSWORD', value: null, masked: true };
+		expect(envValue(masked, {})).toBeNull();
+		expect(envValue(masked, { DB_PASSWORD: 's3cr3t' })).toBe('s3cr3t');
+		expect(envValue({ name: 'PATH', value: '/bin', masked: false }, {})).toBe('/bin');
+	});
+
+	it('matches the JSON of GET /api/containers/{id}', () => {
+		const r: Radiography = {
+			id: 'x', name: 'web', image: 'nginx', imageId: 'sha256:a', created: '', restartCount: 0,
+			state: { status: 'running', running: true, exitCode: 0, oomKilled: false },
+			restartPolicy: { name: 'no' }, entrypoint: [], cmd: [], commandMasked: false, tty: false, logDriver: 'json-file',
+			env: [], ports: [], networks: [], mounts: [], tmpfs: []
+		};
+		expect(r.state.running).toBe(true);
+	});
+});
```

- [ ] **Step 2: Vérifier qu'ils échouent**

Run: `cd web && npx vitest run 2>&1 | tail -n 6`
Expected: `Test Files  2 failed` (`xray.test.ts` ne trouve pas `./xray` ; `api.test.ts` : `getContainer` et `revealEnv` absents).

- [ ] **Step 3: Implémenter**

```diff
diff --git a/web/src/lib/api.ts b/web/src/lib/api.ts
--- a/web/src/lib/api.ts
+++ b/web/src/lib/api.ts
@@ -149,3 +149,52 @@ export function logsUrl(id: string, opts: { tail?: number; after?: string } = {}
 }
 
 export const logsDownloadUrl = (id: string) => `/api/containers/${encodeURIComponent(id)}/logs/download`;
+
+// F7 (spec §6, §7): the radiography. A masked variable has value null; the
+// command's masked parts are "••••••" (the agent masks, the UI only shows).
+export type EnvVar = { name: string; value: string | null; masked: boolean };
+export type Network = { name: string; ip?: string; gateway?: string; mac?: string; aliases?: string[] };
+export type Mount = { type: string; name?: string; source?: string; destination: string; driver?: string; mode?: string; rw: boolean };
+export type Radiography = {
+	id: string;
+	name: string;
+	image: string;
+	imageId: string;
+	created: string;
+	state: {
+		status: string;
+		running: boolean;
+		exitCode: number;
+		error?: string;
+		startedAt?: string;
+		finishedAt?: string;
+		oomKilled: boolean;
+		health?: string;
+	};
+	restartCount: number;
+	restartPolicy: { name: string; maxRetry?: number };
+	entrypoint: string[];
+	cmd: string[];
+	commandMasked: boolean;
+	workingDir?: string;
+	user?: string;
+	tty: boolean;
+	logDriver: string;
+	env: EnvVar[];
+	ports: Port[];
+	networkMode?: string;
+	networks: Network[];
+	mounts: Mount[];
+	tmpfs: string[];
+};
+
+export const getContainer = (id: string) => api<Radiography>(`/containers/${encodeURIComponent(id)}`);
+
+// The agent journals the reveal (env.reveal) before it answers.
+export async function revealEnv(id: string, name: string): Promise<string> {
+	const res = await api<{ name: string; value: string }>(`/containers/${encodeURIComponent(id)}/env/reveal`, {
+		method: 'POST',
+		body: JSON.stringify({ name })
+	});
+	return res.value;
+}
diff --git a/web/src/lib/components/XrayView.svelte b/web/src/lib/components/XrayView.svelte
new file mode 100644
--- /dev/null
+++ b/web/src/lib/components/XrayView.svelte
@@ -0,0 +1,142 @@
+<script lang="ts">
+	import { goto } from '$app/navigation';
+	import { ApiError, revealEnv, type Radiography } from '#lib/api.ts';
+	import { stateLabel } from '#lib/containers.ts';
+	import { errorMessage, t } from '#lib/i18n/index.ts';
+	import { commandLine, envValue, mountLabel, portLabel, restartLabel, shortId } from '#lib/xray.ts';
+
+	let { radio }: { radio: Radiography } = $props();
+
+	let revealed = $state<Record<string, string>>({});
+	let busy = $state(''); // the variable being revealed
+	let error = $state('');
+
+	async function reveal(name: string) {
+		if (busy) return;
+		busy = name;
+		error = '';
+		const id = radio.id;
+		try {
+			const value = await revealEnv(id, name);
+			if (radio.id === id) revealed = { ...revealed, [name]: value };
+		} catch (e) {
+			if (e instanceof ApiError && e.status === 401) {
+				goto('/login').catch(() => {});
+				return;
+			}
+			error = errorMessage(e instanceof ApiError ? e.code : 'unknown');
+		} finally {
+			busy = '';
+		}
+	}
+
+	function hide(name: string) {
+		const next = { ...revealed };
+		delete next[name];
+		revealed = next;
+	}
+</script>
+
+<section class="xray">
+	<h2>{t('xray.identity')}</h2>
+	<dl>
+		<dt>{t('xray.image')}</dt><dd>{radio.image}</dd>
+		<dt>{t('xray.imageId')}</dt><dd><code title={radio.imageId}>{shortId(radio.imageId)}</code></dd>
+		<dt>{t('xray.created')}</dt><dd>{radio.created}</dd>
+	</dl>
+
+	<h2>{t('xray.state')}</h2>
+	<dl>
+		<dt>{t('xray.state')}</dt><dd>{stateLabel(radio.state.status)}</dd>
+		{#if !radio.state.running}<dt>{t('xray.exitCode')}</dt><dd>{radio.state.exitCode}</dd>{/if}
+		{#if radio.state.error}<dt>{t('xray.error')}</dt><dd class="bad">{radio.state.error}</dd>{/if}
+		{#if radio.state.startedAt}<dt>{t('xray.startedAt')}</dt><dd>{radio.state.startedAt}</dd>{/if}
+		{#if !radio.state.running && radio.state.finishedAt}<dt>{t('xray.finishedAt')}</dt><dd>{radio.state.finishedAt}</dd>{/if}
+		{#if radio.state.oomKilled}<dt>{t('xray.oom')}</dt><dd class="bad">{t('xray.yes')}</dd>{/if}
+		{#if radio.state.health}<dt>{t('xray.health')}</dt><dd>{radio.state.health}</dd>{/if}
+		<dt>{t('xray.restarts')}</dt><dd>{restartLabel(radio.restartPolicy, radio.restartCount)}</dd>
+	</dl>
+
+	<h2>{t('xray.command')}</h2>
+	<dl>
+		<dt>{t('xray.entrypoint')}</dt><dd><code>{commandLine(radio.entrypoint) || t('xray.none')}</code></dd>
+		<dt>{t('xray.cmd')}</dt><dd><code>{commandLine(radio.cmd) || t('xray.none')}</code></dd>
+		{#if radio.workingDir}<dt>{t('xray.workingDir')}</dt><dd><code>{radio.workingDir}</code></dd>{/if}
+		{#if radio.user}<dt>{t('xray.user')}</dt><dd><code>{radio.user}</code></dd>{/if}
+		<dt>{t('xray.tty')}</dt><dd>{radio.tty ? t('xray.yes') : t('xray.no')}</dd>
+		<dt>{t('xray.logDriver')}</dt><dd>{radio.logDriver}</dd>
+	</dl>
+	{#if radio.commandMasked}<p class="note">{t('xray.commandMasked')}</p>{/if}
+
+	<h2>{t('xray.env')}</h2>
+	<p class="note">{t('xray.envHint')}</p>
+	{#if error}<p role="alert">{error}</p>{/if}
+	{#if radio.env.length === 0}
+		<p>{t('xray.none')}</p>
+	{:else}
+		<ul class="env">
+			{#each radio.env as v, i (i)}
+				{@const value = envValue(v, revealed)}
+				<li>
+					<code class="name">{v.name}</code>
+					{#if value === null}
+						<span class="masked">•••••• ({t('xray.masked')})</span>
+						<button type="button" disabled={busy !== ''} onclick={() => reveal(v.name)}>{t('xray.reveal')}</button>
+					{:else}
+						<code class="value">{value}</code>
+						{#if v.masked}<button type="button" onclick={() => hide(v.name)}>{t('xray.hide')}</button>{/if}
+					{/if}
+				</li>
+			{/each}
+		</ul>
+	{/if}
+
+	<h2>{t('xray.ports')}</h2>
+	{#if radio.ports.length === 0}<p>{t('xray.none')}</p>{:else}
+		<ul>{#each radio.ports as p, i (i)}<li><code>{portLabel(p)}</code></li>{/each}</ul>
+	{/if}
+
+	<h2>{t('xray.networks')}</h2>
+	{#if radio.networkMode}<p>{t('xray.networkMode')} : <code>{radio.networkMode}</code></p>{/if}
+	{#if radio.networks.length > 0}
+		<ul>
+			{#each radio.networks as n (n.name)}
+				<li>
+					<code>{n.name}</code>
+					{#if n.ip}· {n.ip}{/if}
+					{#if n.gateway}· gw {n.gateway}{/if}
+					{#if n.mac}· {n.mac}{/if}
+					{#if n.aliases?.length}· {n.aliases.join(', ')}{/if}
+				</li>
+			{/each}
+		</ul>
+	{/if}
+
+	<h2>{t('xray.mounts')}</h2>
+	{#if radio.mounts.length === 0 && radio.tmpfs.length === 0}<p>{t('xray.none')}</p>{:else}
+		<ul>
+			{#each radio.mounts as m, i (i)}
+				<li><span class="kind">{m.type}</span> <code>{mountLabel(m)}</code>{#if !m.rw} ({t('xray.readOnly')}){/if}</li>
+			{/each}
+			{#each radio.tmpfs as d (d)}
+				<li><span class="kind">tmpfs</span> <code>{d}</code></li>
+			{/each}
+		</ul>
+	{/if}
+</section>
+
+<style>
+	dl { display: grid; grid-template-columns: minmax(0, 12rem) minmax(0, 1fr); gap: 0.25rem 1rem; }
+	dt { font-weight: 600; }
+	dd { margin: 0; overflow-wrap: anywhere; }
+	h2 { font-size: 1.05rem; margin: 1.25rem 0 0.5rem; }
+	ul { list-style: none; padding: 0; margin: 0; }
+	li { padding: 0.15rem 0; overflow-wrap: anywhere; }
+	.env li { display: flex; flex-wrap: wrap; gap: 0.5rem; align-items: baseline; border-bottom: 1px solid #eee; }
+	.env .name { font-weight: 600; }
+	.masked { opacity: 0.7; }
+	.kind { display: inline-block; min-width: 4rem; opacity: 0.8; }
+	.note { font-style: italic; }
+	.bad, [role='alert'] { color: #b00020; }
+	@media (max-width: 480px) { dl { grid-template-columns: minmax(0, 1fr); } }
+</style>
diff --git a/web/src/lib/i18n/en.ts b/web/src/lib/i18n/en.ts
--- a/web/src/lib/i18n/en.ts
+++ b/web/src/lib/i18n/en.ts
@@ -37,7 +37,6 @@ export const en: Record<Key, string> = {
 	'container.tab.changes': 'Changes',
 	'container.tab.logs': 'Logs',
 	'container.tab.xray': 'X-ray',
-	'container.soon': 'soon',
 	'files.indexing': 'Indexing the stopped container',
 	'files.indexingHint': 'First opening: the agent reads the headers of the container archive. Later ones are immediate.',
 	'files.stoppedUnavailable':
@@ -134,6 +133,39 @@ export const en: Record<Key, string> = {
 	'logs.removed': 'older line(s) left the view (5,000 at most): download the logs to see them.',
 	'logs.truncated': 'line cut at 16 KiB (whole in the download)',
 	'logs.shown': 'line(s) shown',
+	'xray.identity': 'Image',
+	'xray.image': 'Image',
+	'xray.imageId': 'Image ID',
+	'xray.created': 'Created',
+	'xray.state': 'State',
+	'xray.exitCode': 'Exit code',
+	'xray.startedAt': 'Started',
+	'xray.finishedAt': 'Stopped',
+	'xray.error': 'Error',
+	'xray.oom': 'Killed for lack of memory (OOM)',
+	'xray.health': 'Health',
+	'xray.restarts': 'Restarts (policy · count)',
+	'xray.command': 'Command',
+	'xray.entrypoint': 'Entrypoint',
+	'xray.cmd': 'Command',
+	'xray.commandMasked': 'Secret values of the command are masked.',
+	'xray.workingDir': 'Working directory',
+	'xray.user': 'User',
+	'xray.tty': 'TTY (stdout and stderr merged in the logs)',
+	'xray.logDriver': 'Logging driver',
+	'xray.env': 'Environment variables',
+	'xray.envHint': 'Secret values are masked by the agent; a revealed value is journaled.',
+	'xray.masked': 'masked',
+	'xray.reveal': 'Reveal',
+	'xray.hide': 'Hide',
+	'xray.ports': 'Ports',
+	'xray.networks': 'Networks',
+	'xray.networkMode': 'Network mode',
+	'xray.mounts': 'Volumes and mounts',
+	'xray.readOnly': 'read-only',
+	'xray.none': 'none',
+	'xray.yes': 'yes',
+	'xray.no': 'no',
 	'changes.hint': 'Differences from the image, computed by Docker (volumes excluded).',
 	'changes.empty': 'No change from the image.',
 	'changes.added': 'added',
diff --git a/web/src/lib/i18n/fr.ts b/web/src/lib/i18n/fr.ts
--- a/web/src/lib/i18n/fr.ts
+++ b/web/src/lib/i18n/fr.ts
@@ -35,7 +35,6 @@ export const fr = {
 	'container.tab.changes': 'Changements',
 	'container.tab.logs': 'Logs',
 	'container.tab.xray': 'Radiographie',
-	'container.soon': 'bientôt',
 	'files.indexing': 'Indexation du conteneur arrêté',
 	'files.indexingHint':
 		"Première ouverture : l'agent lit les en-têtes de l'archive du conteneur. Les suivantes sont immédiates.",
@@ -137,6 +136,39 @@ export const fr = {
 	'logs.removed': 'ligne(s) plus ancienne(s) retirée(s) de la vue (5 000 au plus) : téléchargez les logs pour les voir.',
 	'logs.truncated': 'ligne tronquée à 16 Kio (complète dans le téléchargement)',
 	'logs.shown': 'ligne(s) affichée(s)',
+	'xray.identity': 'Image',
+	'xray.image': 'Image',
+	'xray.imageId': "Identifiant de l'image",
+	'xray.created': 'Créé le',
+	'xray.state': 'État',
+	'xray.exitCode': 'Code de sortie',
+	'xray.startedAt': 'Démarré le',
+	'xray.finishedAt': 'Arrêté le',
+	'xray.error': 'Erreur',
+	'xray.oom': 'Tué faute de mémoire (OOM)',
+	'xray.health': 'Santé',
+	'xray.restarts': 'Redémarrages (politique · nombre)',
+	'xray.command': 'Commande',
+	'xray.entrypoint': "Point d'entrée",
+	'xray.cmd': 'Commande',
+	'xray.commandMasked': 'Des valeurs secrètes de la commande sont masquées.',
+	'xray.workingDir': 'Dossier de travail',
+	'xray.user': 'Utilisateur',
+	'xray.tty': 'TTY (stdout et stderr confondus dans les logs)',
+	'xray.logDriver': 'Pilote de logs',
+	'xray.env': "Variables d'environnement",
+	'xray.envHint': "Les valeurs secrètes sont masquées par l'agent ; une valeur révélée est journalisée.",
+	'xray.masked': 'masquée',
+	'xray.reveal': 'Révéler',
+	'xray.hide': 'Masquer',
+	'xray.ports': 'Ports',
+	'xray.networks': 'Réseaux',
+	'xray.networkMode': 'Mode réseau',
+	'xray.mounts': 'Volumes et montages',
+	'xray.readOnly': 'lecture seule',
+	'xray.none': 'aucun',
+	'xray.yes': 'oui',
+	'xray.no': 'non',
 	'changes.hint': "Différences avec l'image, calculées par Docker (hors volumes).",
 	'changes.empty': "Aucun changement par rapport à l'image.",
 	'changes.added': 'ajouté',
diff --git a/web/src/lib/xray.ts b/web/src/lib/xray.ts
new file mode 100644
--- /dev/null
+++ b/web/src/lib/xray.ts
@@ -0,0 +1,35 @@
+import type { EnvVar, Mount, Port, Radiography } from './api.ts';
+
+/** An argument as a POSIX shell reads it: quoted when it has to be. */
+function quote(arg: string): string {
+	if (arg !== '' && !/[\s'"\\$`!*?[\]{}()<>|&;#~]/.test(arg)) return arg;
+	return `'${arg.replaceAll("'", `'\\''`)}'`;
+}
+
+/** The entrypoint or command on one line (masked parts stay "••••••"). */
+export const commandLine = (args: string[]): string => args.map(quote).join(' ');
+
+/** "127.0.0.1:8080 → 80/tcp", "8080 → 80/tcp" on every address, "9000/tcp" exposed only. */
+export function portLabel(p: Port): string {
+	const inner = `${p.private}/${p.type}`;
+	if (!p.public) return inner;
+	return `${p.ip && p.ip !== '0.0.0.0' ? `${p.ip}:` : ''}${p.public} → ${inner}`;
+}
+
+/** "source → destination": the volume's name rather than its data path. */
+export const mountLabel = (m: Mount): string => `${m.type === 'volume' && m.name ? m.name : (m.source ?? '')} → ${m.destination}`;
+
+/** "on-failure (5) · 3": policy, retries for on-failure, restarts so far. */
+export function restartLabel(policy: Radiography['restartPolicy'], count: number): string {
+	const name = policy.name || 'no';
+	return `${name}${policy.maxRetry ? ` (${policy.maxRetry})` : ''} · ${count}`;
+}
+
+/** The 12 first hex digits of an image ID. */
+export const shortId = (id: string): string => id.replace(/^sha256:/, '').slice(0, 12);
+
+/** The value to show: the agent's, or the one revealed; null while masked. */
+export function envValue(v: EnvVar, revealed: Record<string, string>): string | null {
+	if (!v.masked) return v.value;
+	return Object.hasOwn(revealed, v.name) ? revealed[v.name] : null;
+}
diff --git a/web/src/routes/c/[id]/+page.svelte b/web/src/routes/c/[id]/+page.svelte
--- a/web/src/routes/c/[id]/+page.svelte
+++ b/web/src/routes/c/[id]/+page.svelte
@@ -2,35 +2,34 @@
 	import { goto } from '$app/navigation';
 	import { page } from '$app/state';
 	import { getContext } from 'svelte';
-	import { ApiError, listContainers, type Container } from '#lib/api.ts';
+	import { ApiError, getContainer, type Radiography } from '#lib/api.ts';
 	import { stateLabel } from '#lib/containers.ts';
 	import { errorMessage, t } from '#lib/i18n/index.ts';
 	import ChangesList from '#lib/components/ChangesList.svelte';
 	import FileBrowser from '#lib/components/FileBrowser.svelte';
 	import LogsView from '#lib/components/LogsView.svelte';
+	import XrayView from '#lib/components/XrayView.svelte';
 
 	let id = $derived(page.params.id ?? '');
-	let container = $state<Container | null>(null);
+	let container = $state<Radiography | null>(null);
 	let error = $state('');
 	let reloads = $state(0); // bumped by the Reload button
 	// From the layout's /api/meta: true until it answers (spec §9).
 	const readOnly = getContext<() => boolean>('readOnly');
-	const TABS = ['files', 'changes', 'logs'] as const;
+	const TABS = ['files', 'changes', 'logs', 'xray'] as const;
 	let tab = $derived(TABS.find((x) => x === page.url.searchParams.get('tab')) ?? 'files');
 
-	// The list gives the name, image and state; GET /api/containers/{id}
-	// arrives with the X-ray tab (phase Observation).
+	// GET /api/containers/{id} (F7): the header and the X-ray tab, secrets
+	// already masked by the agent.
 	$effect(() => {
 		const wanted = id;
 		void reloads;
 		let current = true;
 		error = '';
 		container = null;
-		listContainers()
-			.then((list) => {
-				if (!current) return;
-				container = list.find((c) => c.id === wanted || c.name === wanted) ?? null;
-				if (!container) error = errorMessage('not_found');
+		getContainer(wanted)
+			.then((c) => {
+				if (current) container = c;
 			})
 			.catch((e) => {
 				if (!current) return;
@@ -53,24 +52,26 @@
 		<button type="button" onclick={() => reloads++}>{t('files.reload')}</button>
 	{/if}
 	{#if container}
-		<h1>{container.name} <small class="state {container.state}">{stateLabel(container.state)}</small></h1>
+		<h1>{container.name} <small class="state {container.state.status}">{stateLabel(container.state.status)}</small></h1>
 		<p class="image">{container.image}</p>
 		<nav class="tabs">
 			<a class="tab" class:active={tab === 'files'} aria-current={tab === 'files' ? 'page' : undefined} href="?">{t('container.tab.files')}</a>
 			<a class="tab" class:active={tab === 'changes'} aria-current={tab === 'changes' ? 'page' : undefined} href="?tab=changes">{t('container.tab.changes')}</a>
 			<a class="tab" class:active={tab === 'logs'} aria-current={tab === 'logs' ? 'page' : undefined} href="?tab=logs">{t('container.tab.logs')}</a>
-			<span class="tab soon">{t('container.tab.xray')} ({t('container.soon')})</span>
+			<a class="tab" class:active={tab === 'xray'} aria-current={tab === 'xray' ? 'page' : undefined} href="?tab=xray">{t('container.tab.xray')}</a>
 		</nav>
 		{#if tab === 'changes'}
 			<ChangesList id={container.id} />
 		{:else if tab === 'logs'}
 			<LogsView id={container.id} />
+		{:else if tab === 'xray'}
+			<XrayView radio={container} />
 		{:else}
 			<FileBrowser
 				id={container.id}
 				name={container.name}
 				readOnly={readOnly()}
-				stopped={container.state !== 'running' && container.state !== 'paused'}
+				stopped={!container.state.running}
 			/>
 		{/if}
 	{/if}
@@ -84,7 +85,6 @@
 	.tabs { display: flex; flex-wrap: wrap; gap: 0.5rem; margin: 1rem 0; border-bottom: 1px solid #ddd; }
 	.tab { padding: 0.4rem 0.8rem; }
 	.tab.active { border-bottom: 2px solid currentColor; font-weight: 600; }
-	.tab.soon { opacity: 0.5; }
 	.state.running { color: #1b7f3b; }
 	.state.exited, .state.dead { color: #b00020; }
 	[role='alert'] { color: #b00020; }
```

- [ ] **Step 4: Vérifier**

Run: `cd web && npx vitest run 2>&1 | tail -n 4 && npm run check 2>&1 | tail -n 2 && npm run lint 2>&1 | tail -n 4 && npm run build 2>&1 | tail -n 3`
Expected: 77 tests verts ; `0 ERRORS 0 WARNINGS` ; `lint` sans message ; `✔ done`.

- [ ] **Step 5: Parcours jetable contre un vrai agent (non commité)** — créer `web/e2e/obs-jetable.e2e.ts` et `scripts/e2e-obs-jetable.sh`, les lancer, puis les supprimer :

```ts
import { expect, test } from '@playwright/test';

// Disposable journey (plan Observation, task 8): never committed.
test('logs et radiographie', async ({ page }) => {
	const token = process.env.HUBLOT_TOKEN!;
	await page.goto('/login');
	await page.locator('#token').fill(token);
	await page.locator('button[type=submit]').click();
	await page.getByRole('link', { name: 'hublot-e2e-obs', exact: true }).click();
	// X-ray: masked by default, revealed on demand.
	await page.getByRole('link', { name: 'Radiographie', exact: true }).click();
	await expect(page.locator('.env li').filter({ hasText: 'DB_PASSWORD' })).toContainText('masquée');
	expect(await page.content()).not.toContain('s3cr3t-e2e');
	expect(await page.content()).not.toContain('s3cr3t-cmd');
	await expect(page.locator('code').filter({ hasText: 'redis-server --requirepass •••••• ' })).toBeVisible();
	await page.locator('.env li').filter({ hasText: 'DB_PASSWORD' }).getByRole('button', { name: 'Révéler' }).click();
	await expect(page.locator('.env li').filter({ hasText: 'DB_PASSWORD' })).toContainText('s3cr3t-e2e');
	await expect(page.locator('.env li').filter({ hasText: 'PLAIN' })).toContainText('visible');
	// Logs: live, both streams, search, pause and resume.
	await page.getByRole('link', { name: 'Logs', exact: true }).click();
	await expect(page.locator('.status')).toContainText('En direct');
	await expect(page.locator('.line.stderr').first()).toBeVisible();
	await expect(page.locator('.line.stdout').filter({ hasText: '<b>pas du html</b>' }).first()).toBeVisible();
	await page.getByRole('searchbox').fill('WARN');
	await expect(page.locator('.line.stdout')).toHaveCount(0);
	await page.getByRole('searchbox').fill('');
	await page.getByRole('button', { name: 'Pause' }).click();
	await expect(page.locator('.status')).toContainText('En pause');
	const before = await page.locator('.line').count();
	await page.waitForTimeout(1500);
	expect(await page.locator('.line').count()).toBe(before); // frozen while paused
	await page.getByRole('button', { name: 'Reprendre' }).click();
	await expect(page.locator('.status')).toContainText('En direct');
	await expect.poll(() => page.locator('.line').count()).toBeGreaterThan(before + 3); // caught up
	// No duplicate after the resumption: the tick numbers are strictly increasing.
	const ticks = (await page.locator('.line.stdout .text').allTextContents()).filter((s) => s.startsWith('tick ')).map((s) => Number(s.slice(5)));
	for (let i = 1; i < ticks.length; i++) expect(ticks[i]).toBe(ticks[i - 1] + 1);
	// Download link.
	const [dl] = await Promise.all([page.waitForEvent('download'), page.getByRole('link', { name: 'Télécharger les logs' }).click()]);
	expect(dl.suggestedFilename()).toMatch(/^hublot-e2e-obs-\d{8}-\d{6}\.log$/);
	// Stopped container: past logs, then the end.
	await page.goto('/');
	await page.getByLabel('Afficher les conteneurs arrêtés').check();
	await page.getByRole('link', { name: 'hublot-e2e', exact: true }).click();
	await page.getByRole('link', { name: 'Logs', exact: true }).click();
	await expect(page.locator('.status')).toContainText('Fin du flux');
	await expect(page.locator('.line')).toContainText(['fin du e2e']);
	// 390 px: no horizontal scroll on the Logs and X-ray tabs.
	await page.setViewportSize({ width: 390, height: 800 });
	const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
	expect(await overflow()).toBeLessThanOrEqual(0);
	await page.getByRole('link', { name: 'Radiographie', exact: true }).click();
	await expect(page.locator('.xray')).toBeVisible();
	expect(await overflow()).toBeLessThanOrEqual(0);
});
```

```bash
#!/usr/bin/env bash
# Jetable (plan Observation, tâche 8) : scripts/e2e.sh sans `make web` (pas de npm ci),
# plus un conteneur actif pour les logs et la radiographie, puis l'arrêt de l'agent
# avec un flux ouvert. À lancer depuis la racine du dépôt ; SKIP_PW=1 saute Playwright.
set -euo pipefail
(cd web && npm run build >/dev/null)
rm -rf internal/web/dist && mkdir -p internal/web/dist && cp -R web/build/. internal/web/dist/ && touch internal/web/dist/.gitkeep
arch=$(docker version -f '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o bin/hublot-linux ./cmd/hublot
sse=$(mktemp)
cleanup() {
  docker rm -f hublot-e2e hublot-e2e-obs hublot-e2e-agent >/dev/null 2>&1 || true
  docker volume rm -f hublot-e2e-data >/dev/null 2>&1 || true
  rm -f "$sse"
}
cleanup
trap cleanup EXIT
docker run --name hublot-e2e alpine:3 sh -c 'echo "bonjour hublot" > /root/note.txt; echo "fin du e2e"'
docker run -d --name hublot-e2e-obs -e DB_PASSWORD=s3cr3t-e2e -e PLAIN=visible alpine:3 sh -c \
  'echo "<b>pas du html</b>"; i=0; while true; do i=$((i+1)); echo "tick $i"; echo "WARN $i" >&2; sleep 0.2; done; exec redis-server --requirepass s3cr3t-cmd' >/dev/null
tok=$(docker run --rm -v hublot-e2e-data:/data -v "$PWD/bin/hublot-linux:/hublot:ro" alpine:3 \
  /hublot reset-token --data-dir /data | sed -n 's/^.* : //p')
docker run -d --name hublot-e2e-agent \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD/bin/hublot-linux:/hublot:ro" -v hublot-e2e-data:/data \
  -p 127.0.0.1:7879:7878 \
  alpine:3 /hublot --listen 0.0.0.0:7878 --data-dir /data >/dev/null
for _ in $(seq 1 40); do curl -sf -o /dev/null http://127.0.0.1:7879/login && break; sleep 0.5; done
[ -n "${SKIP_PW:-}" ] || (cd web && HUBLOT_URL=http://127.0.0.1:7879 HUBLOT_TOKEN="$tok" npx playwright test 2>&1 | tail -n 15)
# Arrêt avec un flux de logs ouvert : l'agent doit s'arrêter aussitôt, code 0.
cookie=$(curl -s -i -X POST -H 'Origin: http://127.0.0.1:7879' -H 'Content-Type: application/json' \
  -d "{\"token\":\"$tok\"}" http://127.0.0.1:7879/api/login | sed -n 's/^Set-Cookie: \(hublot_session=[^;]*\).*/\1/p')
curl -s -N -H "Cookie: $cookie" 'http://127.0.0.1:7879/api/containers/hublot-e2e-obs/logs?tail=2' > "$sse" &
sleep 2
start=$(date +%s)
docker stop -t 10 hublot-e2e-agent >/dev/null
echo "MESURE arrêt de l'agent avec un flux ouvert : $(( $(date +%s) - start )) s, code $(docker inspect -f '{{.State.ExitCode}}' hublot-e2e-agent)"
echo "MESURE début du flux lu par curl :"; head -c 200 "$sse"; echo
```

Run: `export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock && bash scripts/e2e-obs-jetable.sh 2>&1 | tail -n 12 ; rm web/e2e/obs-jetable.e2e.ts scripts/e2e-obs-jetable.sh`
Expected: `2 passed` (le parcours unique et le jetable) ; `MESURE arrêt de l'agent avec un flux ouvert : 1 s, code 0` (6 s et code 1 sans le `RegisterOnShutdown` de la tâche 3) ; le début du flux montre des lignes `id: <horodatage>/0` et `data: {"ts":…,"stream":"stdout","line":"tick …"}`.

- [ ] **Step 6: Commit**

```bash
git status --short web/e2e scripts   # rien : le parcours jetable n'est pas commité
git add web/src
git commit -F - <<'EOF'
web : onglet Radiographie (secrets masqués, révélation), en-tête par GET /api/containers/{id}

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

### Task 9: Spec, vérification sur les VM et clôture

**Où tester :** `hk-ubuntu` (AppArmor), `hk-fedora` (SELinux), Docker Desktop.

**Files:**
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§5.3, §5.4, §6, §7, §13), `docs/superpowers/plans/2026-10-10-observation-logs-radiographie.md` (cases cochées, constats)

- [ ] **Step 1: Reporter les décisions dans la spec** — remplacer chaque passage par le suivant (chacun est unique dans la spec ; vérifier par `grep -c`).

1. §5.3 étape 2, tmpfs retrouvé (tâche 1, Q24, Q25) — remplacer :

   ```text
   chaque destination déclarée par `Inspect` (`Mounts`) et chaque `--tmpfs` (`HostConfig.Tmpfs`) apparaît comme point de montage, comme pour le contrôle complet des montages de §5.4 (b).
   ```

   par :

   ```text
   chaque destination déclarée par `Inspect` (`Mounts`) et chaque `--tmpfs` (`HostConfig.Tmpfs`) apparaît comme point de montage, telle quelle ou retrouvée par identifiant de montage, comme pour le contrôle complet des montages de §5.4 (b). Un tmpfs n'a pas de source à lier : il n'est retrouvé que sur une ligne `tmpfs` entière (racine du montage `/`), ni sur `/`, ni sur un point déjà pris par une autre destination, ni sur ou sous `/dev`, `/proc` ou `/sys` (montages du runtime). Toute écriture sous le point où un tmpfs est retrouvé est refusée (`forbidden_path`) : Docker n'y écrit pas. Conteneur arrêté : la destination d'un tmpfs est résolue dans l'index, liens suivis, comme runc la résout au démarrage, et l'écriture sous le chemin résolu est refusée de même.
   ```

2. §5.4 (b), tmpfs retrouvé pour le tar (revue I5, Q38) — remplacer :

   ```text
   La racine du montage n (champ 4) doit terminer la `Source` du bind, ou finir par `/<nom>/_data` pour un volume. P n'est pas `/`, et il ne sert qu'une seule destination.
   ```

   par :

   ```text
   La racine du montage n (champ 4) doit terminer la `Source` du bind, ou finir par `/<nom>/_data` pour un volume ; un tmpfs (`--mount type=tmpfs`), qui n'a pas de source, n'est retrouvé que sur une ligne `tmpfs` entière (racine `/`), ni sur ni sous `/dev`, `/proc` ou `/sys`. P n'est pas `/`, et il ne sert qu'une seule destination.
   ```

3. §6, code logs_unavailable (Q34) — remplacer :

   ```text
   `link_in_path` (écriture ou téléchargement de dossier : un lien dans la chaîne du chemin, §5.3 étape 2), `docker_unavailable`, `internal`.
   ```

   par :

   ```text
   `link_in_path` (écriture ou téléchargement de dossier : un lien dans la chaîne du chemin, §5.3 étape 2), `docker_unavailable`, `logs_unavailable` (pilote de logs que Docker ne sait pas relire : `none`, `syslog`…), `internal`.
   ```

4. §6, statuts (Q33, Q34) — remplacer :

   ```text
   `not_regular` et `link_in_path` 422,
   ```

   par :

   ```text
   `not_regular`, `link_in_path` et `logs_unavailable` 422,
   ```

5. §6, plafond des flux de logs (Q33) — remplacer :

   ```text
   un créneau n'est rendu qu'au vrai retour de l'opération, copie d'un téléchargement comprise.
   ```

   par :

   ```text
   un créneau n'est rendu qu'au vrai retour de l'opération, copie d'un téléchargement comprise. Logs : au plus 8 flux ouverts pour l'agent, téléchargements compris ; quand ils sont tous pris, le flux SSE le plus ancien cède sa place (événement `evicted`) ; s'ils ne sont tenus que par des téléchargements, 503 `internal` (dans le flux SSE, événement `failure`).
   ```

6. §6, route GET /api/containers/{id} (F7) — remplacer :

   ```text
   | GET | `/api/containers/{id}` | F7, env masquées par l'agent |
   ```

   par :

   ```text
   | GET | `/api/containers/{id}` | F7 : image, état, redémarrages, commande et point d'entrée (parties secrètes remplacées par `••••••`, `commandMasked`), `env` (`[{name, value, masked}]`, valeur `null` si masquée, §7), ports, réseaux, montages, tmpfs, pilote de logs |
   ```

7. §6, route POST /env/reveal (Q28) — remplacer :

   ```text
   | POST | `/api/containers/{id}/env/reveal` | `{name}` → valeur en clair, journalisé |
   ```

   par :

   ```text
   | POST | `/api/containers/{id}/env/reveal` | `{name}` (nom exact) → `{name, value}` en clair, journalisé (`env.reveal`) avant l'envoi : si le journal ne s'écrit pas, `internal` et rien n'est révélé ; variable absente ou corps invalide → `not_found` ; permis en lecture seule (c'est une lecture) |
   ```

8. §6, route GET /logs (Q29 à Q33) — remplacer :

   ```text
   | GET | `/api/containers/{id}/logs?tail=500` | F6 SSE : `{ts, stream: "stdout"|"stderr", line}` |
   ```

   par :

   ```text
   | GET | `/api/containers/{id}/logs?tail=500` | F6 SSE : `{ts, stream: "stdout"|"stderr", line, truncated?}` par ligne, `id` = `<ts>/<rang>` ; `tail` de 0 à 10 000 ; reprise après `Last-Event-ID` ou `?after=<id>` ; événements `end` (fin du flux : conteneur arrêté), `dropped` (`{count}`, lignes au-delà du débit), `evicted` (un flux plus récent a pris le créneau), `failure` (`{error, detail}`, codes de §6 ; `unauthorized` quand la session a pris fin, vu au battement de 15 s) ; toujours `200` une fois la session admise |
   ```

9. §6, route GET /logs/download (Q32) — remplacer :

   ```text
   | GET | `/api/containers/{id}/logs/download` | F6 fichier texte |
   ```

   par :

   ```text
   | GET | `/api/containers/{id}/logs/download` | F6 fichier texte, `<ts> <stream> <ligne>` par ligne, tout ce que Docker garde ; ligne de plus d'1 Mio coupée (`[…]`) ; `attachment` et `application/octet-stream` |
   ```

10. §7, secrets (Q26, Q27) — remplacer :

   ```text
   ou si sa valeur contient une URL avec identifiants (`://user:pass@`). Valeur masquée renvoyée : `null` avec `masked: true`.
   ```

   par :

   ```text
   ou si sa valeur contient une URL avec identifiants (`://` puis un `@` avant tout blanc : mot de passe, ou jeton seul en nom d'utilisateur). Valeur masquée renvoyée : `null` avec `masked: true`. Une valeur est masquée aussi quand les règles de la commande y trouvent un secret, telle quelle ou décodée de ses `%XX` (`…;Password=…;`, `…;Pwd=…;`, `?password=…`, `-D…Password\=…`, `--requirepass …`). La commande, le point d'entrée et le message d'erreur de l'état sont masqués partie par partie (`••••••`) : identifiants d'une URL ; valeur de `NOM=valeur` ou `--nom=valeur` (ou `\=`), guillemets compris, et tout le reste de l'argument hors d'une ligne de shell (l'argument qui suit le `-c` d'un shell) ; argument ou mot qui suit un drapeau `--nom` (ou tout argument après `--nom=`) ; quand le nom suit la même règle, ou vaut `Pwd` dans une affectation. Un mot de la liste dont la valeur, dans son propre mot, n'est pas masquée (drapeau, ou mot qui contient `=` ou `:` après lui) fait masquer l'argument entier ; dans un chemin ou un mot ordinaire (`/opt/keycloak`, `ssh-keyscan`), il reste lisible. Une valeur se révèle une à une (`env/reveal`, journalisé) ; la commande ne se révèle pas en v0.1.
   ```

11. §7, journal d'audit (Q28) — remplacer :

   ```text
   Actions : `login.fail`, `env.reveal`, `file.write`, `file.upload`.
   ```

   par :

   ```text
   Actions : `login.fail`, `env.reveal`, `file.write`, `file.upload`. Pour `env.reveal`, `path` porte le nom de la variable, jamais sa valeur.
   ```

12. §13, tmpfs sous un lien (tâche 1) — remplacer :

   ```text
   | `--tmpfs` placé sous un lien (ex. `tmpfs: [/var/run]` en compose, avec `/var/run` → `../run`) : la destination ne se retrouve pas dans la table des montages | toute écriture (édition et envoi) sur ce conteneur actif est refusée (`forbidden_path`), par échec fermé (§5.3 étape 2) |
   ```

   par :

   ```text
   | `--tmpfs` placé sous un lien (ex. `tmpfs: [/var/run]` en compose, avec `/var/run` → `../run`) : la destination ne se retrouve pas telle quelle dans la table des montages | retrouvée par identifiant de montage (conteneur actif) ou par l'index (conteneur arrêté) ; l'écriture est permise ailleurs, refusée sous le point retrouvé (§5.3 étape 2) ; sans preuve, refus de toute écriture (échec fermé) |
   | Tmpfs leurre monté par le conteneur lui-même (`CAP_SYS_ADMIN`) sur la cible d'un lien posé après le démarrage : le `--tmpfs` déclaré est lié à ce leurre | la conséquence possible est une écriture réussie que le conteneur ne voit pas, pas une évasion ; limite assumée pour la v0.1 |
   | Secrets écrits dans les logs par l'application | non masqués : §7 ne masque que l'environnement et la commande ; les logs s'affichent comme `docker logs` |
   | Masquage de la commande heuristique : `mysql -pSECRET`, un secret dans un script ou un fichier passé en argument | non masqués en v0.1 ; seuls les noms de §7 sont reconnus (§7) |
   | Conteneur qui écrit plus de 1 000 lignes par seconde | au-delà de 10 000 lignes d'avance, les lignes en trop ne sont pas affichées mais comptées (`dropped`) ; le téléchargement les contient |
   | Navigateur en HTTP/1.1 : 6 connexions par hôte, un flux de logs en tient une | l'interface ferme le flux en quittant l'onglet Logs ; l'agent plafonne à 8 flux (§6) |
   | Client parti sans FIN (veille, changement de réseau) : sa connexion reste ouverte côté agent environ 15 min | chaque écriture du flux a un délai de 30 s ; quand les 8 créneaux sont pris, le flux le plus ancien cède le sien (`evicted`) ; un autre onglet encore ouvert peut ainsi être fermé, il le dit |
   | Déconnexion ou expiration de la session pendant un flux | le flux s'arrête au battement suivant (15 s au plus) par `failure` `unauthorized` |
   | Reprise d'un flux : lignes de même horodatage coupées par `tail`, ou lignes annoncées `dropped` | une reprise peut rejouer quelques lignes du groupe coupé (horodatages mesurés distincts : rare) ; les lignes `dropped` reviennent à la reprise, comptées deux fois par l'interface |
   | Secret d'environnement hors des formes reconnues : JSON (`"token":"…"`), `user:motdepasse@hôte` sans `://`, `password = …` avec des blancs, variable nommée `DB_PWD` | non masqué en v0.1 ; la règle est celle de §7 |
   | Secret de commande hors des formes reconnues : `--password -s3cr3t` (valeur qui commence par `-` après un drapeau sans `=`), `PASSWORD=$(cat f)s3cr3t` (fin après une substitution) | non masqué en v0.1 (§7) |
   | Éviction d'un flux de logs par un autre : les créneaux sont communs à toutes les sessions | la v0.1 n'a qu'un utilisateur (§2), avec une session exigée et un cookie `SameSite=Strict`, et un onglet évincé ne se reconnecte pas : non exploitable ; à revoir si l'agent devient multi-utilisateur (v0.2) |
   ```

- [ ] **Step 2: Suite complète sur les deux VM**

Run (pour chaque VM) : `limactl start hk-ubuntu >/dev/null && limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/test-linux.sh -race -tags integration -count=1 ./cmd/... ./internal/... 2>&1 | grep -E '^(ok|FAIL|---)' | tail -n 30`, puis idem `hk-fedora`.
Expected: `ok` pour chaque paquet ; `TestIntegrationWriteTmpfsUnderLink`, `TestIntegrationCheckWriteRootTmpfsUnderLink`, `TestIntegrationCheckArchiveTmpfsUnderLink`, `TestCheckArchiveTmpfsJail`, `TestLogsRealDocker` et `TestIntegrationObservation` passent sans SKIP (vérifier par `-run 'Tmpfs|Logs|Observation' -v` ; seul `TestIntegrationDockerSocketUnderVarRun` peut se sauter, et seulement sur Docker Desktop). Sur `hk-fedora`, aucun AVC (`sudo ausearch -m avc -ts recent`, vide) ; sur `hk-ubuntu`, aucun `apparmor="DENIED"` dans `dmesg`. Vérifier en particulier que `/var/run` → `../run` dans `alpine:3` sur les deux (sinon le test se saute en le disant).

- [ ] **Step 3: Revue de branche** — demander la revue complète de la branche (sous-agent indépendant + Codex), corriger, puis cocher les cases de ce plan et noter les constats sous « Constats de la tâche 9 » : résultat des deux VM, AVC éventuels, verdict de la revue, réponses de l'utilisateur aux questions Q24 à Q35.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers
git commit -F - <<'EOF'
docs : Observation vérifiée sur hk-ubuntu et hk-fedora, spec de F6 et F7 complétée

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

#### Constats de la rédaction

- Copie isolée (`git worktree`, `a6ffdeb`) : tout le code des tâches 1 à 8 compile et passe, en macOS et sous Linux root (`scripts/test-linux.sh -race -tags integration -count=1 ./cmd/... ./internal/...`, Docker Desktop 29.7.2, API 1.55) ; seul SKIP, préexistant : `TestIntegrationDockerSocketUnderVarRun` (socket mandataire de Docker Desktop). Les blocs du plan, réappliqués sur une seconde copie propre, redonnent le même arbre (`git write-tree` identique), et la vérification complète y a été rejouée. Les VM n'ont pas été lancées.
- Format des logs (Docker Desktop, `curl` sur le socket) : sans TTY, une trame par message, type 1 (stdout) ou 2 (stderr), charge `"<horodatage RFC 3339 nano> <texte>\n"` ; une sortie sans `\n` attend le `\n` suivant (`printf nonl; sleep 1; echo` → une ligne `nonl`, à l'heure du `\n`). Avec `-t` : aucune trame, fins de ligne `\r\n`, stderr mêlé à stdout.
- Ligne de 40 000 octets : trois trames de 16 384, 16 384 et 7 232 octets de texte, **même horodatage** ; en TTY, `TS <16 384>TS <16 384>TS <7 232>\r\n` collés (40 133 octets en tout avec la ligne suivante).
- `--log-driver none` : `501 Not Implemented`, `{"message":"configured logging driver does not support reading"}` (→ Q34).
- Deux lignes écrites au même instant sur stdout et stderr arrivent dans l'un ou l'autre ordre (`err1` avant `out1`, deux fois sur deux) : les tests trient ces deux lignes.
- `since` est inclusif (`TestLogsRealDocker`) ; `Follow` sur un conteneur arrêté rend ses logs puis se termine ; `tail=1` rend la dernière ligne.
- `Config.Env` : Docker retire les doublons à la création (`-e A=1 -e A=2` → `A=2`) ; `-e NOVAL` (absente du client) reste `NOVAL`, sans `=`.
- Ports : conteneur actif, `NetworkSettings.Ports` donne 0.0.0.0 et :: pour une même liaison ; conteneur arrêté, `NetworkSettings.Ports` est vide et `HostConfig.PortBindings` porte `HostIp: ""`.
- `alpine:3` : `/var/run -> ../run` ; `--tmpfs /var/run` est monté sur `/run`. Avant la tâche 1, un envoi dans `/etc` de ce conteneur → `403 forbidden_path` (« ne montre pas la destination /var/run ») ; après, `201`, et `/run` comme `/var/run` → `403`. Un PID 1 chrooté dans ce tmpfs, avec un lien `var/run → /` posé dans la prison, est refusé.
- Arrêt de l'agent (`docker stop -t 10`) avec un flux de logs ouvert : 6 s et code 1 sans `RegisterOnShutdown` (`Shutdown` attend 5 s puis échoue), 1 s et code 0 avec.
- Build de l'interface : le nœud de la page conteneur passe à 38 Ko (22 Ko au plan 3 : onglets Logs et Radiographie) ; CodeMirror reste à part, chargé à la demande.
- Revue 1 (Opus, sur Docker Desktop) : `--mount type=tmpfs,dst=/var/run` sur alpine figure dans `Inspect.Mounts` et se monte sur `/run` ; 2 004 lignes écrites d'un coup ont 2 004 horodatages distincts. Ici : `--mount type=tmpfs` n'accepte pas `exec` par la ligne de commande (`tmpfs-options` refusé par l'analyse CSV de la CLI 29.7), d'où la prison du tar simulée par une table de montages et un vrai lien vers `/` (`TestCheckArchiveTmpfsJail`) plutôt que par un vrai chroot.
- Parcours jetable (tâche 8) : masquage à l'écran (`s3cr3t-e2e` et `s3cr3t-cmd` absents de la page), révélation, ligne `<b>pas du html</b>` affichée comme texte, filtre `WARN`, pause (nombre de lignes figé 1,5 s) puis reprise (numéros de `tick` consécutifs : ni doublon ni trou), téléchargement `hublot-e2e-obs-AAAAMMJJ-HHMMSS.log`, conteneur arrêté (« Fin du flux », `fin du e2e`), 390 px sans défilement horizontal.

#### Constats de la tâche 9

- Remplis par l'étape 3 : résultat des deux VM, AVC, verdict de la revue, réponses de l'utilisateur aux questions Q24 à Q35.
