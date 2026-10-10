# SDD ledger — plan: docs/superpowers/plans/2026-10-10-observation-logs-radiographie.md

Spec: docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
Revue par tâche : sous-agent + Codex en lecture seule (sans --write).
Q24–Q38 confirmées le 2026-10-10 (Q34 logs_unavailable, Q27 sans révélation, Q28 révélation permise en lecture seule).

## Scan préalable

Scan de cohérence du plan (HEAD 2576a19, seul le plan diffère de `a6ffdeb`, base des diffs : `git diff a6ffdeb HEAD -- . ':!docs'` est vide). Méthode : lecture des 5 794 lignes, recoupement des signatures, des listes `Files`/`git add` et des contextes de hunks entre tâches ; les 12 « remplacer » de la tâche 9 comptés dans la spec (chacun apparaît 1 fois) ; baseline Vitest = 62 tests (62 + 8 en tâche 7 = 70, + 7 en tâche 8 = 77, comme annoncé) ; `LogsView.svelte` extrait et compilé en mode runes (aucun avertissement, `paused` est bien `$state`). Pas de recompilation Go (consigne).

### 1. Paires de tâches (interfaces et fichiers partagés)

| # | Tâches | Produit → consomme | Constat |
|---|---|---|---|
| 1 | 1 → 9 | T1 : comportement tmpfs, tests `TestIntegrationWriteTmpfsUnderLink`, `...CheckWriteRootTmpfsUnderLink`, `...CheckArchiveTmpfsUnderLink`, `TestCheckArchiveTmpfsJail` → T9 l.5603-5625, 5735-5745 (§5.3, §5.4 b, §13) et l.5761 (noms des tests) | Concordant : les noms de l.5761 existent en T1 (l.209, 341, 368, 488) ; textes de T9 conformes aux gardes de `sameMount`/`runtimeMount` (l.745-768) |
| 2 | 1 → 3, 5, 6 (même paquet `api`) | T1 ajoute `integration_write_linux_test.go` (`dockerCLI`, linux seul) ; T3/T5/T6 ajoutent `logs_test.go`, `container_test.go`, `integration_observation_test.go` | Concordant : aucun nom en double (`dockerRun` de T6 l.4227 est portable, `dockerCLI` est `integration && linux`) ; aucun `mux`, `sse`, `slot`, `bucket` préexistant |
| 3 | 2 → 3 | `LogsOptions{Tail,Since,Follow}`, `LogLine`, `Client.Logs`, `ReadLogs(r,tty,max,emit)`, `MaxLogLine`, `ErrNoLogs`, `ContainerInfo.Tty`, `Fake.LogStreams/LogsErr/LogsGate/LogsCloses/LogsCalls()` (l.1002) → `logsStream`, `logsDownload`, tests (l.2785, 2902, 2005, 2059) | Concordant (signatures, types `atomic.Int32`, `[]LogsOptions`) |
| 4 | 2 → 5 (même `dockerx.go`) | T2 ajoute `Tty`, `LogDriver`, le bloc `if c.Config != nil` ; T5 ajoute `Details`, `Mount.Mode/RW`, `fromInspectDetails` | Concordant : les contextes des hunks de T5 (l.4005-4008, 4061-4075) reprennent exactement la sortie de T2 (l.1388-1418) ; imports `strconv`, `network` ajoutés par T5, `fmt`/`io`/`cerrdefs` déjà présents |
| 5 | 2 → 6 | `Client.Logs`, `ErrNoLogs` → `TestIntegrationObservation` (conteneur `--log-driver none`, l.4337) | Concordant |
| 6 | 3 → 5 (même `server.go`) | T3 : `inspectByRef`, champs `logLimits`/`streams`, routes l.2956-2957 ; T5 : routes l.3983-3984 | Concordant : le hunk de T5 (`@@ -111`) tombe sur les lignes d'origine +5 de T3 (hunks l.2933 et 2943 ajoutent 3 + 2 lignes) ; `inspectByRef(r) (ContainerInfo, error)` identique à l.1821 |
| 7 | 3 → 6 | `openSSE`, `sseClient.next/all/cancel`, `lineOf`, `failureOf` (l.1895-1999) → l.4277, 4296, 4323, 4337 | Concordant (fichier sans tag de build, utilisable par le test `integration`) |
| 8 | 3 → 7 | Contrat SSE : `id` = `<ts>/<rang>`, données `{ts,stream,line,truncated?}`, `dropped {count}`, `failure {error,detail}`, `end`, `evicted`, `?after=`, `Last-Event-ID`, routes `/logs` et `/logs/download`, code `logs_unavailable` (l.2511-2512) → `parseLine`, `parseCount`, `parseFailure`, `streamEnd`, `logsUrl`, `LogsView` (l.4776-4863, 4572-4597) | Concordant (regex d'ID l.2571 ↔ `e.lastEventId` ; `unauthorized` → `login: true` ↔ l.2826) |
| 9 | 3 → 9 | Plafonds 8 flux, 10 000/1 000 par s, 15 s, 30 s, `tail` 0 à 10 000, 1 Mio, 422 | Concordant avec les constantes (l.2553, 2558, 2566) et les lignes §6/§13 de T9 (l.5651-5709, 5748-5752) |
| 10 | 4 → 5 | `secrets.Env`, `Args`, `Line`, `Lookup`, `Var{Name,Value *string,Masked}`, `Mask` (l.2990) → `toRadioJSON`, `revealEnv` (l.3902-3912, 3964) | Concordant (JSON `value:null` ↔ `[]secrets.Var`) |
| 11 | 4 → 9 | Règles de masquage → §7 et lignes §13 (l.5711-5721, 5753-5754) | Concordant avec R11 à R15 (mots dans un chemin lisibles, `Pwd`, `\=`, décodage `%XX`) |
| 12 | 5 → 6 | `radioJSON`, `auditLines`, `env/reveal` (l.3876, 3646, 3984) → l.4256-4273 | Concordant (champs `State.Running`, `Ports[0].Public`, `LogDriver`, `Networks[0].IP`, `Env[].Masked/Value`) |
| 13 | 5 → 8 | JSON de `radioJSON` (l.3876-3898, `mountJSON`, `stateJSON`, `restartJSON`, `dockerx.Network`, `dockerx.Port`) → types `Radiography`, `EnvVar`, `Network`, `Mount` (l.5046-5080), `getContainer`, `revealEnv` | Concordant champ par champ (clés, `omitempty` ↔ `?`) |
| 14 | 5 → 9 | Routes `GET /containers/{id}`, `POST /env/reveal`, action d'audit `env.reveal` → §6/§7 (l.5663-5685, 5723-5733) | Concordant (journal avant l'envoi, `not_found` pour corps invalide, permis en lecture seule : l.3946-3975) |
| 15 | 7 → 8 (mêmes `api.ts`, `fr.ts`, `en.ts`, `+page.svelte`, `api.test.ts`) | T7 : `TABS` à 3 entrées, import de `LogsView`, clés `logs.*`, import de `logsUrl`/`logsDownloadUrl` → T8 : `TABS` à 4, clés `xray.*`, retrait de `container.soon` | Concordant : contextes de T8 (l.4943, 5039-5042, 5242-5253, 5404-5406) reprennent la sortie de T7 ; `container.soon` n'est plus utilisé après T8 (seule occurrence hors i18n : `+page.svelte` l.59-60 de l'état actuel, remplacée en T7 puis T8) ; 25 clés `xray.*` identiques en fr et en |
| 16 | 6, 7, 8 → 9 | Noms de tests et de parcours cités en l.5761 | Concordant |
| 17 | 3 + 5 → 8 (parcours jetable) | Libellés de l'interface (`En direct`, `En pause`, `Reprendre`, `Fin du flux`, `Révéler`, `masquée`, `Télécharger les logs`, `Radiographie`) et classes (`.status`, `.line.stdout`, `.env li`, `.xray`) → l.5485-5530 | Concordant avec fr.ts (l.4743-4760, 5305-5337) et les composants |

17 lignes de paires.

### 2. Cohérence interne de chaque tâche

| Tâche | Tests ↔ code | Fichiers créés ↔ touchés ↔ `git add` | Constat |
|---|---|---|---|
| 1 (l.180-987) | 6 fichiers de test + 6 fichiers de code, tous listés l.185-186 ; mutations (l.976) citent des lignes littérales (`l.Point == "/" \|\|`, `&& !runtimeMount(l.Point)`, `return underTmpfs(target, points)`) | `git add internal/fsview internal/files internal/api/integration_write_linux_test.go` (l.981) couvre les 12 fichiers | Cohérent. Aucun appelant de `newTarFS`/`CheckWriteRoot` oublié (grep sur le dépôt : seuls ceux modifiés) |
| 2 (l.991-1807) | `TestReadLogs*`, `TestFakeLogs`, `TestWrapLogs`, `TestFromInspectTtyAndLogDriver`, `TestLogsRealDocker` ↔ `logs.go`, `fake.go`, `dockerx.go` | `git add internal/dockerx` (l.1801) couvre `logs.go`, `logs_test.go`, `dockerx.go`, `fake.go`, `integration_test.go` | Cohérent. Les imports de `fake.go` (`bytes`, `fmt`, `io`, `slices`, `sync/atomic`) existent déjà. Le seul autre `Client` (les `putter` de `files_test.go`) embarque `*dockerx.Fake` |
| 3 (l.1811-2977) | 22 tests ↔ `logs.go` (638 lignes de test, 409 de code, comptes des en-têtes de hunk exacts) ; mutations littérales (`rc.Close()`, `e.deadline()`, `if len(s.live) == 0 {`) | `git add internal/api cmd/hublot/main.go` (l.2971) couvre `logs.go`, `logs_test.go`, `server.go`, `fserr.go`, `main.go` | Cohérent. `main.go` importe déjà `net` et `context` (le hunk l.2481 n'ajoute pas d'import) |
| 4 (l.2981-3481) | `secrets_test.go` (212 l.) ↔ `secrets.go` (236 l.) ; cas de `TestArgs` correspondent aux règles R1, R11, R12, R14, R15 | `git add internal/secrets` (l.3475) : répertoire nouveau, 2 fichiers | Cohérent. Mutation n° 3 (l.3470) non littérale (voir 3.) |
| 5 (l.3485-4183) | `container_test.go`, `server_test.go` (champ `auditLog`), `dockerx_test.go` (import `network` ; `netip`, `reflect` déjà importés) ↔ `container.go`, `server.go`, `dockerx.go` | `git add internal/dockerx internal/api` (l.4177) couvre les 6 fichiers | Cohérent. Mutations littérales (`Error: secrets.Line(d.Error)`, `Cmd`/`Entrypoint` bruts) |
| 6 (l.4187-4365) | Un seul test, pas d'étape rouge (explication l.4198) | `git add internal/api/integration_observation_test.go` (l.4359) = le seul fichier créé | Cohérent ; écart à la règle « étape 2 = échec » de Global l.111, justifié par le texte |
| 7 (l.4369-4916) | `logs.test.ts` (8 `it` au total avec `api.test.ts` et `i18n.test.ts`) ↔ `logs.ts`, `api.ts`, `LogsView`, `index.ts` ; 62 → 70 tests vérifié | `git add web/src` (l.4910) couvre les 10 fichiers de l.4374-4376 ; `fr.ts`/`en.ts` : 18 clés `logs.*` + `error.logs_unavailable` des deux côtés | Cohérent |
| 8 (l.4920-5590) | `xray.test.ts` (5 `it`) + `api.test.ts` (2) ↔ `xray.ts`, `api.ts`, `XrayView` ; 70 → 77 vérifié | `git add web/src` (l.5584) couvre les 7 fichiers de l.4925-4927 ; les deux fichiers jetables (créés puis `rm`, l.5577) sont hors `web/src`, et `git status` (l.5583) le contrôle | Cohérent, avec les écarts 1 à 3 de la liste 3 |
| 9 (l.5594-5774) | 12 chaînes « remplacer » : chacune présente 1 fois dans la spec | `git add docs/superpowers` (l.5768) : spec + plan | Écarts 4 et 5 de la liste 3 |

### 3. Défauts qu'un relecteur relèverait, et contradictions avec les Global Constraints

1. **Test sans assertion** : tâche 8, `xray.test.ts` « matches the JSON of GET /api/containers/{id} » (l.5016-5024) construit un littéral `Radiography` puis affirme `r.state.running` vrai, la valeur qu'il vient d'écrire ; Vitest ne vérifie pas les types, `npm run check` seul les contrôle. Aucun lien avec le JSON réel.
2. **Régression de comportement** : tâche 8, `stopped={!container.state.running}` (l.5455) remplace `state !== 'running' && state !== 'paused'`. Un conteneur `restarting` a `State.Running = true` mais `Pid = 0` : `Views.For` (`views.go:113`, `Running && Pid != 0`) sert alors l'index d'archive, alors que l'interface le présente comme actif (messages « arrêté » et note de confirmation d'écriture faux).
3. **Global l.115 (« bouton Recharger sur les états d'erreur »)** : `XrayView` affiche `error` (échec de révélation) sans bouton (l.5169). **Global l.115 (« textes dans fr.ts et en.ts »)** : `gw` (l.5203) et `tmpfs` (l.5218) en dur ; `stdout`/`stderr` en dur dans `LogsView` (l.4644-4645, termes techniques).
4. **`git add` incomplet** : tâche 9, étape 3 (l.5763) demande de « corriger » après la revue de branche, mais l'étape 4 ne stage que `docs/superpowers` (l.5768) : une correction de code ne serait pas commitée.
5. **Incohérence de libellé** : tâche 9, l.5763 et l.5794 demandent les réponses aux « questions Q24 à Q35 », alors que Q36 à Q38 existent (l.100-105) et que le ledger les donne confirmées (Q24 à Q38).
6. **Mutations avant commit** : tâches 1 à 5 (l.976, 1796, 2966, 3470, 4172) rejouent les mutations à l'étape 4, donc sur des changements **non commités**, avec « puis annuler » mais sans commande. En tâches 2 à 5, `logs.go`, `secrets.go`, `container.go` et leurs tests sont **non suivis** : `git checkout -- <fichier>` échoue (pathspec) ou, pour un fichier suivi modifié (T1 : `mounts.go`, `files.go` ; T2 : `dockerx.go`, `fake.go` ; T3 : `server.go`, `fserr.go`, `main.go` ; T5 : `dockerx.go`, `server.go`), jetterait **toute l'étape 3**. Aucun `git checkout`, `restore` ou `stash` n'est écrit dans le plan. Parade : `git add` des chemins de la tâche avant de muter, puis `git checkout -- <fichier muté>`.
7. **Mutation non littérale** : tâche 4, l.3470, 3e mutation (« `strings.Contains(s[loc[1]:], Mask)` au lieu du masque en fin de mot ») : cette expression n'existe pas dans `leaks` (l.3404 utilise `strings.LastIndex(after, Mask)` puis `alnum`) ; l'exécutant doit la réécrire.
8. **Duplications** (sans gravité) : `sse.deadline()` (l.2680) et `timedWriter.Write` (l.2880) répètent `SetWriteDeadline(time.Now().Add(sseWriteTimeout))` ; `goto('/login').catch(() => {})` répété 4 fois (l.4594, 4610, 5120 et la page existante) ; fr/en : `xray.identity` = `xray.image` et `xray.command` = `xray.cmd` (mêmes valeurs, l.5254-5268, 5305-5319) ; la boucle « PID 1 chroote un instant après le démarrage » (25 × 200 ms) copiée de `TestIntegrationCheckWriteRoot` (l.354-359 et 323-331) ; `TestFakeLogs` (l.1341) teste le faux, pas le code livré.
9. **Contradictions avec les Global Constraints** :
   - l.112 (« aucun secret ne quitte l'agent non masqué ») est absolue alors que les logs ne sont pas masqués (Décisions l.68, ligne §13 l.5746) ; la contrainte devrait se limiter à l'environnement, à la commande et à l'audit.
   - l.117 (« Co-Authored-By: Claude Opus 5.5 ») diffère de l'attribution demandée à la session (« Claude Sonnet 5.5 ») : les 9 heredocs (l.985, 1805, 2975, 3479, 4181, 4914, 5588, 5772 et celui de la tâche 6, l.4363) portent Opus 5.5.
   - l.109 (« dépendances autorisées… aucune autre ») ne cite pas `github.com/containerd/errdefs`, que le plan liste (l.21) et que `dockerx.go:13` importe déjà : pas de dépendance nouvelle, liste à compléter.
   - Aucune violation de l.110 : pas de `filepath.Join`/`path.Join`/`os.Open` sur un chemin de conteneur (`path.Base(args[0])` en T4 porte sur une commande, `CleanPath` sur des chaînes d'index).
   - Aucune étape de revue (sous-agent + Codex) n'est écrite dans les tâches 1 à 8 : la règle de CLAUDE.md repose sur l'en-tête (l.5) et le ledger.

### 4. Ordre des commits

- Le plan ne commite **qu'une fois par tâche**, à l'étape 5 (l.981, 1801, 2971, 3475, 4177, 4359, 4910, 5584, 5768), après l'étape 4 : tests et code partent **ensemble**. Aucun commit de tests seuls : l.7 (« un commit de tests puis un commit de code par tâche ») décrit la copie de vérification, pas ce que l'exécutant fera, et la phrase l.139 (« ne pas pousser entre l'étape 1 et l'étape 3 ») n'a rien à interdire puisque rien n'est commité entre les deux. Les tests d'évasion (T1) et de masquage (T4, T5) sont donc **écrits** avant le code, pas **commités** avant. Pour que l'historique le montre, il faudrait commiter à la fin de l'étape 2.
- **Aucun commit intermédiaire rouge** dans le plan tel qu'écrit : chaque commit de tâche est complet (interfaces changées avec tous leurs appelants et faux). Tâche 6 : commit de test seul, après le code qu'il vérifie. Tâche 9 : documentation.
- À ne pas pousser : **aucun couple de tâches**. Seulement, à l'intérieur d'une tâche, rien entre les étapes 1 et 3 si l'exécutant choisit de commiter les tests (tâches 1 à 5, 7, 8), car le paquet ne compile pas ou la suite est rouge (l.139).

### 5. Annulations de mutations par `git checkout`

- Le plan n'écrit aucun `git checkout`, `git restore` ni `git stash` (recherche exhaustive des lignes `git `). Les annulations de mutations sont dites « puis annuler » (l.976, 2966, 3470, 4172) ou « Mutation : supprimer… » (l.1796), sans commande, sur des changements non commités : voir le défaut 6 de la liste 3 (fichiers non suivis en T2 à T5 ; fichiers suivis dont `git checkout` jetterait l'étape 3 entière en T1, T2, T3, T5).
- L'étape 5 de la tâche 8 (fichiers jetables) crée puis supprime par `rm` (l.5577) deux fichiers non suivis : aucun `git checkout`, et `git status --short web/e2e scripts` (l.5583) garde le contrôle.

## Arbitrages du scan préalable
- Ruling: P1 (T8 l.5016-5024) — test « matches the JSON » qui relit son propre littéral : le faire porter sur une vraie conversion (ou le retirer s'il ne peut rien prouver) — si faux : un test vide de plus.
- Ruling: P2 (T8 l.5455) — `stopped` doit suivre le même critère que l'agent (views.go:113 : Running && Pid>0), pas `!running`, pour qu'un conteneur `restarting` soit traité comme arrêté — si faux : un état affiché qui contredit la vue servie.
- Ruling: P3 (T8 l.5169, 5203, 5218) — bouton « Recharger » sur l'erreur de révélation (contrainte globale) ; libellés `gw` et `tmpfs` dans fr.ts/en.ts — si faux : aucun.
- Ruling: P4 (T9 l.5763-5794) — le `git add` de la T9 inclut aussi les corrections de code éventuelles ; « Q24 à Q35 » → « Q24 à Q38 » — si faux : un commit incomplet.
- Ruling: P5 (T1-T5) — les mutations ne s'annulent ni par `git checkout` (fichiers non suivis, ou étape 3 entière jetée) : rééditer puis vérifier `git diff` ; la 3e mutation de T4 est décrite en prose : l'implémenteur la rend littérale — si faux : un commit qui ne compile pas.
- Ruling: P6 — CLAUDE.md exige les tests d'évasion AVANT le code : pour les tâches de sécurité (T1 tmpfs, T4 secrets, T5 révélation, T3 limites/fin des flux), commit des tests rouges d'abord, puis le code ; ne pas pousser entre les deux. Les autres tâches gardent un commit — si faux : quelques commits rouges de plus en local.
- Ruling: P7 — l.112 (aucun secret ne quitte l'agent) à nuancer en T9 : les logs ne sont pas masqués (§13) ; attribution « Opus 5.5 » (consigne de la session principale) ; `containerd/errdefs` déjà importé par le dépôt, donc autorisé — si faux : une phrase trop absolue.
- Task 1: Codex : aucun constat ; Opus I1 : tmpfs non déclaré amené par un rbind sert de leurre (sans et avec prison, confirmé par test jetable) → tour 1
- Task 1: fix round 1/5 (I1 addressed — tmpfs lié seulement s'il est fils d'une ligne « / » ; tests unitaire + intégration Docker du leurre ; commits 36e2059..d95523b)
- Task 1: POUR LA T9 : écrire en §5.3 et §5.4 (b) la garde « parent = ligne / » ; §13 : faux refus d'un tmpfs déclaré sous un autre tmpfs déclaré ou sous un volume/bind (ex. compose `tmpfs: [/var/run, /var/lock]` → toute écriture refusée, échec fermé) ; M1 (commentaire de tarFS.CheckWriteRoot)
- Task 1: minor (deferred): accepter comme parent un tmpfs déclaré déjà retrouvé (Opus R1, facultatif) ; scénario B sans test Docker réel ; mutation 3 du plan non compilable telle quelle
- Task 1: complete (commits 2576a19..d95523b, review clean après 1 tour — Codex aucun constat)
- Task 2: Ruling: Codex I « trame stdin (type 0) traitée comme stdout, tampon partagé » — identique à moby stdcopy (Stdin retombe sur Stdout) ; l'API des logs n'émet jamais de stdin — minor (deferred) — si faux : une trame stdin d'un démon hostile s'affiche comme stdout.
- Task 2: minor (deferred): max négatif fait paniquer add (logs.go:95) ; readRaw octet par octet (logs.go:210) ; message du démon (trame 3) non assaini (logs.go:138) ; tests manquants (trame > 32 Kio, ligne de plusieurs Mio, flux 4+, horodatage malformé)
- Task 2: complete (commits d95523b..e289631, review clean — sous-agent ✅, Codex 1 I arbitré)
- Task 3: Ruling: Opus I1 « téléchargement interrompu = fichier complet » est imposé par la consigne, mais contredit la contrainte globale du téléchargement (JSON avant le premier octet, ErrAbortHandler après) → corrigé — si faux : aucun.
- Task 3: minor (deferred): arrêt de l'agent sans test ; sortie en code 1 si un client ne lit plus ; délai HTTP/2 qui coupe un flux sans écriture en cours ; plafond de 8 pas strict pendant 30 s ; ID de ligne en double (rare)
- Task 3: POUR LA T6 : couvrir sur vrai Docker le départ du client d'un flux et la reprise sur de vraies lignes
- Task 3: Ruling: Codex Minor « Cache-Control: no-cache » non appliqué — le middleware /api/ pose déjà no-store (plus strict), vérifié par l'implémenteur — si faux : aucun.
- Task 3: fix round 1/5 (3 addressed — erreur de téléchargement JSON avant le 1er octet / abort après, délai d'inactivité 1 min, flux silencieux à l'arrêt ; commits 6eb19ba..2c90892)
- Task 3: minor (deferred): F3 (flux silencieux à l'arrêt) sans test dédié ; échéance du téléchargement avant le 1er octet → 500 internal (504 serait plus juste)
- Task 3: complete (commits e289631..2c90892, review clean après 1 tour)
- Task 4: tour 1 envoyé (I1 quadratique, I2 repli trompé, I3 --pwd/Pwd, Unicode Cf/homoglyphes, Minor JSON/blanc échappé/Bearer/URL a@b)
- Parallélisme (demande de l'utilisateur, 2026-10-10) : T7 (web, onglet Logs, dépend seulement de T3) en worktree isolé pendant le correctif de T4 puis T5 (Go) ; fusion de la branche du worktree dans phase/observation avant T8.
