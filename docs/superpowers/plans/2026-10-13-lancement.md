# Lancement — Documentation FR/EN, GoReleaser, image GHCR, release v0.1 — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Revue (règle du dépôt, `CLAUDE.md`) :** chaque tâche est relue par un sous-agent indépendant **et** par Codex avant la suivante (dernière étape de chaque tâche). Relecture prioritaire : tranche C (ex-tâche 7 : permissions du workflow, épinglage, identité OIDC), tâche 3 (unité systemd : durcissement contre l'explorateur, jeton hors du journal), tâche 5 (la doc dit les limites de §13 honnêtement, et rien de faux sur la sécurité).
>
> **Étapes extérieures et irréversibles :** elles sont marquées **« ARRÊT : accord de l'utilisateur requis »**. L'agent s'arrête, résume ce que l'étape rend visible ou définitif, et attend un « oui » explicite de l'utilisateur dans la conversation. Un message d'agent ne vaut jamais accord. Liste complète plus bas (« Étapes extérieures »).
>
> **Vérifié avant rédaction (en partie)**, sur une copie jetable du dépôt (`git archive` de `40aa7cb`, hors du dépôt, supprimée ensuite) :
> - Tâche 1 : test rouge (`flag provided but not defined: -version`, usage sans `--version`), puis vert ; `go vet` propre ; binaire `GOOS=linux` qui répond `hublot 0.1.0`.
> - Tâche 2 : test de garde `TestAgentMessagesInEnglish` rouge sur une copie avec la tâche 1 (`169 French strings in the agent`, 27 fichiers) ; l'expression qui juge « français » ne déclenche sur aucune des traductions du tableau de la tâche 2.
> - Tâche 4 : image `FROM scratch` construite sur Docker Desktop (arm64, 3,8 Mo) avec un binaire compilé à la main ; `scripts/check-image.sh` vert (cinq cibles, mise à jour sur le même volume) ; deux mutations attrapées (version attendue fausse, conteneur `hublot-kit` déjà présent). Constat : dans l'image, l'avertissement de l'écoute sans TLS est normal (spec §7) ; le script ne refuse que celui du cgroup. Ces mesures datent des messages français ; les scripts lisent depuis les messages anglais de la tâche 2, à revérifier aux tâches 3 et 4.
> - Tâche 5 : `scripts/check-docs.sh` vert sur les huit pages de ce plan ; quatre mutations attrapées (commande modifiée, avertissement root retiré, lien mort, bloc de code différent entre FR et EN).
> - Tâche 6 : `scripts/check-dist.sh` vert sur un `dist/` simulé (binaires compilés à la main, mêmes options que GoReleaser) ; deux mutations attrapées. Constats : `-trimpath` retire `-ldflags` des informations de build (la version se cherche donc dans le binaire) ; sans `/dist/` dans `.gitignore`, `vcs.modified=true` (les fichiers de `dist/` sont des fichiers non suivis).
> - Revue du plan (2026-10-13) : `hk_check_writes` (édition et envoi) vert sur Docker Desktop dans l'image `scratch`, binaire aux messages anglais ; `check-dist.sh` refuse un SBOM SPDX sans le module de l'agent et un SBOM absent de `checksums.txt` (deux mutations) ; `release.yml` en `workflow_dispatch` avec `guard` et `web`, `actionlint` propre.
> - Tâches 6 et 7 : `actionlint` 1.7.12 sans remarque sur `ci.yml` et `release.yml`. Versions des actions et des outils relevées le 2026-10-13 par `gh api` (Q40).
> - **Non vérifié** : la traduction elle-même (tâche 2), GoReleaser lui-même (absent du Mac ; premier passage en CI, tâche 6), la signature et la publication (tâche 9), les VM `hk-ubuntu` et `hk-fedora` (tâches 3, 4, 8 : unité systemd, AppArmor, SELinux, `sudo` sur Fedora).

**Goal:** Publier Hublot-Kit v0.1 : une documentation publique en anglais et en français, des archives Linux amd64 et arm64 avec leurs sommes de contrôle, leurs SBOM et leurs signatures, une unité systemd vérifiée, et l'image multi-architecture `ghcr.io/levy-cmd/hublot-kit:0.1`, signée, avec son SBOM ; chaque maillon de la chaîne est vérifié avant toute publication.

**Architecture:** GoReleaser construit tout dans GitHub Actions (`release.yml`, lancé à la main sur un tag `v*` confirmé, Q54) : binaires statiques avec l'interface embarquée (construite par `make web` dans un job sans droits), archives avec l'unité systemd, `checksums.txt`, SBOM SPDX par syft, signatures cosign sans clé (jeton OIDC de GitHub Actions), image `FROM scratch` multi-architecture par buildx, signée par son empreinte, avec une attestation SBOM de BuildKit ; la release reste en brouillon, un second job la relit (`scripts/verify-release.sh`). Avant tout tag, le job CI `release-snapshot` fait toute la chaîne sans rien publier, et lance l'image amd64 avec la commande exacte de la spec §8 ; les VM testent l'image arm64 et l'unité systemd. Une pré-version `v0.1.0-rc.1` éprouve la signature et GHCR en privé ; la v0.1.0 ne devient publique qu'après, étape par étape, avec l'accord de l'utilisateur.

**Tech Stack:** Go 1.27 (option `--version`, messages de l'agent en anglais), GoReleaser OSS ≥ 2.18 (`dockers_v2`, `sboms`, `signs`, `docker_signs`), syft, cosign 3.x (keyless, bundles `.sigstore.json`), Docker buildx (BuildKit, attestation SBOM), GitHub Actions et GHCR, systemd, bash. Aucune dépendance Go ni d'interface nouvelle.

**Spec:** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§7 « le README le dit en tête », §8 commande recommandée, image, alternative binaire et systemd, prérequis hôte, §10 CI, §12 « Lancement : docs FR/EN, GoReleaser, image GHCR, release → v0.1 publique », §13 risques que la doc reprend). Plan précédent (format, Q1 à Q38) : `docs/superpowers/plans/2026-10-10-observation-logs-radiographie.md`.

## Décisions de l'utilisateur (2026-10-13)

Appliquées telles quelles, sans les rediscuter :

1. La branche `phase/lancement` part de `main` **après** la fusion de la PR #5 (Observation).
2. Chaîne d'approvisionnement : sommes de contrôle, SBOM (syft), signature cosign sans clé (OIDC de GitHub Actions) des binaires **et** de l'image. Image multi-architecture linux/amd64 et linux/arm64 sur GHCR.
3. Documentation : `README.md` en anglais (vitrine GitHub), `README.fr.md` en français, et un dossier `docs/` (installation, sécurité, limites) en anglais et en français. Pas de site. Le README dit **en tête** que l'agent a, de fait, les droits root sur l'hôte (spec §7, §8).
4. systemd : une unité systemd dans la release (binaire lancé en root hors conteneur), **vérifiée** sur `hk-ubuntu` (AppArmor) et `hk-fedora` (SELinux), comme le reste.
5. Langue (Q49, décision du 2026-10-13) : les messages de l'agent passent en anglais (sortie, aide, `slog`, `detail` d'API) ; l'interface, les commentaires, les commits et les docs du projet restent tels quels.

## Ce que les plans précédents ont préparé

- `var version = "dev"` dans `cmd/hublot/main.go`, rempli par `-ldflags "-X main.version=…"` ; `/api/meta` renvoie `{version, readOnly}`.
- Chaque option a sa variable `HUBLOT_*` (`internal/config`) : l'image fixe `HUBLOT_LISTEN=0.0.0.0:7878` et `HUBLOT_DATA_DIR=/data` (§8) sans changer le code.
- `prepareDataDir` : dossier créé en 0700, refus d'un lien, d'un autre propriétaire, d'un dossier ouvert en écriture au groupe ou à tous. Un volume Docker neuf (`/data`, 0755, root) et `StateDirectory=` de systemd (0700, root) passent tous les deux.
- Avertissements au démarrage : écoute non locale sans TLS (attendu dans l'image), espace de noms cgroup privé (ne doit jamais paraître avec la commande de §8).
- `make web` construit l'interface et la copie dans `internal/web/dist` (embarquée) ; le job CI `binary` vérifie déjà `/login` et la CSP sur un binaire réel.
- `scripts/dev-agent.sh` et `scripts/e2e.sh` lancent déjà l'agent avec les droits de §8 ; `scripts/test-linux.sh` sert sur les VM (`limactl shell --workdir "$PWD" hk-ubuntu -- sudo …`).
- Spike n°3 et 3b : cibles mesurées (ordinaire, `--privileged`, `apparmor=unconfined`, autre uid) ; `scripts/release-lib.sh` les reprend.

## Décisions de ce plan (la spec ne tranche pas)

- **Une seule source pour la commande d'installation : la spec §8.** `scripts/check-image.sh` la lit dans la spec (de `docker run -d --name hublot-kit` à `ghcr.io/levy-cmd/hublot-kit:0.1`) et ne remplace que la référence d'image ; `scripts/check-docs.sh` exige que `README.md`, `README.fr.md` et les deux pages d'installation la reprennent à l'octet près. Tester l'image, c'est donc tester la commande de la doc. → Tâches 4, 5.
- **`hublot --version`**, avant la lecture de la configuration : `hublot <version>`, code 0, sans toucher au dossier de données. Sert aux contrôles de `dist/` et à la doc. → Tâche 1.
- **Des scripts de vérification versionnés**, réutilisables par les mainteneurs : `check-systemd.sh`, `check-image.sh`, `check-dist.sh`, `check-docs.sh`, `verify-release.sh`, fonctions communes dans `release-lib.sh`. Chacun est le « test » de sa tâche : écrit d'abord, vu en échec, puis vert. → Tâches 3 à 7.
- **Les builds lourds tournent en CI.** Sur le Mac (disque à 91 %) : seulement un binaire et une image `FROM scratch` de quelques Mo, supprimée aussitôt. GoReleaser ne s'installe pas sur le Mac. Les disques des VM Lima vivent aussi sur le disque du Mac : chaque image chargée y est supprimée après usage. → Tâches 4, 6, 8.
- **Ce qui part vers les VM vient de la CI** (artefact `release-snapshot` : image arm64 en `docker save`, archives) : les VM testent exactement ce que GoReleaser a produit. → Tâche 8.
- **Release en brouillon**, relue par le job `verify` sur une machine neuve, publiée à la main. → Tâches 7, 9, 10.
- **Le jeton n'entre jamais dans le journal système** : la doc systemd fait `hublot reset-token` avant le premier démarrage ; `check-systemd.sh` le vérifie. → Tâches 3, 5.
- **`sudo /usr/local/bin/hublot`** dans la doc : sur Fedora et RHEL, le `secure_path` de `sudo` n'inclut pas `/usr/local/bin`. → Tâches 3, 5.

## Décisions (questions de la rédaction, tranchées par l'utilisateur le 2026-10-13)

Une ligne par question ; les tâches indiquées appliquent la réponse.

- **Q39 — Go de la release** : recommandation adoptée. `release.yml` prend le dernier correctif de 1.27 (`go-version: "1.27.x"`, `check-latest: true`, sans cache) ; `go.mod` et les autres jobs ne changent pas. → Tâche 7.
- **Q40 — Versions et épinglage** : recommandation adoptée. Actions tierces épinglées par empreinte de commit (tableau ci-dessous), actions de GitHub par version majeure ; GoReleaser `~> v2.18`, cosign v3.1.3 ; relevé refait à l'étape 1 de la tâche 6. → Tâches 6, 7.
- **Q41 — Unité systemd** : recommandation adoptée. `hublot-kit.service`, root, `/usr/local/bin/hublot`, `StateDirectory=hublot-kit` (0700), `127.0.0.1:7878`, réglages par `systemctl edit` ; durcissement limité à `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp` (exclus : `ProtectProc`, `ProcSubset`, `PrivateUsers`, `CapabilityBoundingSet`, `ProtectKernelTunables`, `PrivateDevices`, `SystemCallFilter`) ; une directive qui casse un contrôle est retirée et notée. → Tâches 3, 5.
- **Q42 — Image de base** : recommandation adoptée, `FROM scratch`. → Tâche 4.
- **Q43 — Étiquettes de l'image** : recommandation adoptée. `0.1.0` et `0.1`, pas de `latest` ; une pré-version n'a que sa propre étiquette. → Tâches 6, 9, 10.
- **Q44 — Signatures et SBOM** : recommandation adoptée. Bundles cosign de `checksums.txt` et de chaque archive, index de l'image signé par empreinte ; SBOM SPDX (syft) par archive, attestation SBOM (BuildKit) sur l'image. → Tâches 6, 7.
- **Q45 — Release en brouillon** : recommandation adoptée ; publication manuelle (ARRÊT) après le job `verify`. → Tâches 7, 9, 10.
- **Q46 — Pré-version** : recommandation adoptée. `v0.1.0-rc.1` sur `phase/lancement` avant la fusion, `v0.1.0` sur `main` après ; un échec donne `rc.2`, jamais un tag déplacé ; brouillon de la rc supprimé sur accord. → Tâches 9, 10.
- **Q47 — Dépôt privé** : recommandation adoptée. Audit à la tâche 8, dépôt rendu public juste avant la publication de la v0.1.0 (tâche 10, ARRÊT) ; le nom du dépôt dans Rekor dès la rc est accepté. → Tâches 8, 9, 10.
- **Q48 — Captures d'écran** : oui, deux PNG dans `docs/images/` ; l'étape n'est plus facultative. → Tâche 5.
- **Q49 — Langue de l'agent** : **changement**. Tout texte humain que l'agent affiche, journalise (`slog`) ou renvoie dans un `detail` d'API passe en anglais ; l'interface (FR/EN), les commentaires du code, les commits et les docs du projet ne changent pas ; la limite « messages en français » sort de la doc. → Tâche 2 (nouvelle, avant la doc), tâches 3, 4, 5.
- **`detail` affichés tels quels par l'interface** (revue du plan, 2026-10-13) : les 4 cas (`files.ts:78`, `files.ts:95`, `write.ts:66`, `write.ts:72`) restent en anglais dans les deux langues : ils sont dynamiques (chemin, montage, message de Docker), et les traduire demanderait des codes de détail dans l'API (§6), environ une journée, hors v0.1. → Tâche 2.
- **Q50 — amd64** : recommandation adoptée ; amd64 testé par la CI seulement. → Tâches 6, 8.
- **Q51 — Notes de release** : recommandation adoptée ; en-tête GoReleaser en anglais, puis le journal des commits. → Tâche 6.
- **Q52 — Paquets `.deb` / `.rpm`** : recommandation adoptée ; pas en v0.1.
- **Q53 — Signalement des vulnérabilités** : recommandation adoptée ; signalement privé de GitHub activé au passage en public. → Tâches 5, 10.
- **Q54 — Verrou technique de publication** (revue du plan, 2026-10-13) : un tag `v*` poussé ne publie plus rien. Un environnement `release` avec relecteur obligatoire était le premier choix, mais il n'est pas disponible ici : `gh api repos/levy-cmd/Hublot-Kit` → `"private": true` ; `gh api user --jq .plan` → `null` (le jeton n'a pas le droit `user`) ; la doc de GitHub (« Deployments and environments ») dit : « If you are on a GitHub Free, GitHub Pro, or GitHub Team plan, required reviewers are only available for public repositories », et un compte personnel n'a pas d'offre Enterprise. Retenu : `release.yml` ne se lance que par `workflow_dispatch` (`gh workflow run release.yml --ref <tag> -f confirm=<tag>`), et un job `guard` sans droits refuse une ref qui n'est pas un tag `vX.Y.Z[-rc.N]`, une confirmation différente du tag, ou un autre auteur que le propriétaire. Conséquence : GitHub n'enregistre un `workflow_dispatch` que si le fichier est sur `main` ; une petite PR qui ne contient que `release.yml` est fusionnée avant la rc (tâche 9, étape 0), ce qui garde Q46. Une fois le dépôt public, l'environnement avec relecteur pourra s'ajouter. → Tâches 7, 9, 10.

### Versions épinglées (relevé du 2026-10-13)

| Action ou outil | Version | Commit |
|---|---|---|
| `goreleaser/goreleaser-action` | v7.3.0 | `a4f02ab5355a589de60d917a659ce617ed19b3de` |
| `sigstore/cosign-installer` | v4.1.2 | `6f9f17788090df1f26f669e9d70d6ae9567deba6` |
| `anchore/sbom-action` (`download-syft`) | v0.24.3 | `66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c` |
| `docker/setup-buildx-action` | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| `docker/login-action` | v4.6.0 | `dbcb813823bdd20940b903addbd779551569679f` |
| `actions/upload-artifact` | v7.0.2 | `cf430e030ddbb5b0abf93d22962f4752f3646cd9` |
| GoReleaser | v2.18.3 | (contrainte `~> v2.18`) |
| cosign | v3.1.3 | (`cosign-release` de l'installeur) |
| syft | v1.54.1 | (installé par `download-syft`) |

## Historique du plan

- **2026-10-13 — rédaction** (`35285a7`), décisions de l'utilisateur sur Q39 à Q53 (`24fad02`), deux tours de revue (Opus et Codex) : `eae7277`, `ee58f7c`.
- **Révision 2026-10-15 — tranches verticales** (décision de l'utilisateur ; règles « Orchestration » de `CLAUDE.md`). Raison : les anciennes tâches 6 à 8 découpaient la release en couches (GoReleaser, puis workflow, puis vérification VM de tout) ; une fonctionnalité se livre désormais de bout en bout, code, doc et vérification sur la cible ensemble. Ce qui reste fixe : objectifs, critères d'acceptation (tableau ci-dessous, aucun retiré), Décisions, étapes ARRÊT. Ce qui change : seul le découpage des tâches 6 à 8.
  - Tâche 6 : socle GoReleaser, presque fait (`f05b69f`) ; renvoi, rien n'est redécrit.
  - **Tranche A**, installer avec Docker depuis une release : partie image de la tâche 6, section Docker de la doc vérifiée contre `.goreleaser.yaml`, partie Docker de la vérification VM de l'ancienne tâche 8, job CI `docs` (après la fusion de la tâche 5), test de la garde cgroup (remplace la mutation bloquée de la tâche 4).
  - **Tranche B**, installer avec systemd depuis une release : partie archives de la tâche 6, section systemd de la doc, partie systemd de la vérification VM de l'ancienne tâche 8.
  - **Tranche C**, vérifier une release : l'ancienne tâche 7 et la section de vérification de la doc.
  - Tâche 8 : son reste, spec et audit `gitleaks`. Tâches 9 (rc.1) et 10 (v0.1.0) inchangées, sauf leurs renvois.
  - Ordre : fusion de la tâche 5 (faite : `9d872c7`) → fin de la tâche 6 → A → B (les deux touchent `.goreleaser.yaml`, donc en séquence) → C → 8 → 9 → 10.
  - Contrat (leçon consignée au registre) : chaque tranche liste ses fichiers possédés **et** les fichiers qu'elle lit chez une autre (l'archive de la tranche B lit `README.fr.md`, par exemple) ; un changement d'un fichier lu passe par son écrivain.
  - Renvois : dans les sections écrites avant la révision (Décisions, tâches 1 à 5), « tâche 7 » désigne la tranche C, et « tâche 8 » les tranches A et B (vérification VM) ou la tâche 8 (spec, audit).

## Critères d'acceptation des anciennes tâches 6 à 8 (révision 2026-10-15)

Aucun n'est retiré ; chacun a une seule tâche ou tranche qui le prouve.

| Critère (ancienne tâche) | Où il est prouvé |
|---|---|
| `goreleaser check` et `release --snapshot --clean --skip=sign` verts en CI (6) | tâche 6, étape 5 |
| `dist/` : archives au contenu exact, `checksums.txt` juste, SBOM SPDX valide avec le module de l'agent et présent dans `checksums.txt`, binaires statiques, `-trimpath`, `vcs.modified=false`, version, interface embarquée (6) | tâche 6 (écrit ; prouvé à l'étape 5) ; relu par la tranche B |
| `/dist/` ignoré par git ; versions des actions relevées et épinglées (6) | tâche 6 (écrit ; prouvé à l'étape 5 : `vcs.modified=false`, actions épinglées exécutées) |
| Image multi-architecture poussée dans le registre local de la CI, plateformes et SBOM validés par `jq` (6) | tâche 6 (écrit ; prouvé à l'étape 5) ; tranche A, étape 7 |
| Étiquettes du snapshot `<V>-amd64` / `<V>-arm64` vérifiées avant `check-image.sh` (6) | tâche 6 (écrit ; prouvé à l'étape 5) ; tranche A, étape 7 |
| Image amd64 lancée en CI par la commande exacte de §8 : cinq cibles, édition, envoi, mise à jour (6) | tâche 6, étape 5 ; tranche A, étape 7 (sur son dernier commit) |
| Job CI `docs` vert (5, puis contrat T5/T6) | tranche A, étapes 3 et 7 |
| Étiquettes `X.Y.Z` et `X.Y`, jamais `latest` (6, Q43) | tranche A (bloc A de `check-docs.sh`) ; tâches 9 et 10 sur GHCR |
| Garde cgroup de `check-image.sh` éprouvée (mutation de la tâche 4, bloquée) | tranche A, étapes 1 à 4 (`test-release-lib.sh`) |
| Artefact CI pris au run du commit testé, commit final compris (8) | tranches A (étapes 7 et 8) et B (étapes 4 et 5) |
| Image arm64 de la CI sur `hk-ubuntu` et `hk-fedora`, sans refus AppArmor ni AVC (8) | tranche A, étape 8 |
| Archive arm64 de la CI, sommes contrôlées, installée par l'unité sur les deux VM, écritures et SELinux `Enforcing` compris (8) | tranche B, étape 5 |
| Doc Docker et systemd conformes à `.goreleaser.yaml` et aux VM (nouveau, tranches) | tranches A (étapes 5 et 8) et B (étapes 3 et 5) |
| `verify-release.sh` rouge sans release ; `actionlint` propre ; actions tierces épinglées ; rien de Node dans le job qui signe ; garde rejoué en local (7) | tranche C, étapes 2 et 4 |
| Verrou : seul un lancement confirmé sur un tag publie ; job `verify` : signatures, sommes, SBOM SPDX des archives et de l'image, plateformes (7) | tranche C (écrit) ; tâche 9, étapes 2 et 3 (éprouvé) |
| Doc « Verify the release » conforme à `verify-release.sh` (identité, émetteur, couverture de la signature) (7, revue) | tranche C, étapes 1 et 5 |
| Spec §8, §10, §13 à jour (8) | tâche 8, étape 1 |
| Audit avant publication : `gitleaks`, `.claude/handoff/*`, adresses des commits (8) | tâche 8, étape 2 |
| Revue de toute la branche (8) | tâche 8, étape 3 |

## Global Constraints

- **Dépôt et image** : module `github.com/levy-cmd/hublot-kit` ; dépôt GitHub `levy-cmd/Hublot-Kit` (casse exacte : c'est elle qui figure dans l'identité OIDC des signatures) ; image `ghcr.io/levy-cmd/hublot-kit`, commande de §8 sur `ghcr.io/levy-cmd/hublot-kit:0.1`.
- **Prérequis hôte (spec §8, à reprendre tels quels)** : « Linux amd64 ou arm64, noyau 5.6+ (lecture 5.6+ ; écriture (édition et envoi) sur un conteneur actif 5.8+, car `CheckWriteRoot` exige `STATX_MNT_ID`), Docker Engine avec API 1.44+, stockage overlay2 ou containerd. »
- **Sécurité dite d'abord** : les deux README disent l'accès root dans leurs 12 premières lignes ; la doc ne dit jamais que `:ro` protège le socket (§7) ; jamais de `--privileged` recommandé (§8).
- **Aucune dépendance Go ni d'interface nouvelle.** GoReleaser, syft, cosign et buildx ne tournent qu'en CI (et cosign dans une VM, tâche 9).
- **Accès aux fichiers d'un conteneur (CLAUDE.md, §5)** : ce plan ne change aucune logique ; la tâche 2 ne touche, dans `internal/fsview` et `internal/files`, que le texte des messages d'erreur (aucun appel, aucune condition).
- **Tests d'abord** : l'étape 1 de chaque tâche écrit le test ou le script de vérification, l'étape 2 le voit échouer pour la bonne raison.
- **Disque du Mac (91 %)** : pas de GoReleaser ni d'image lourde sur le Mac ; images locales `FROM scratch` seulement, supprimées aussitôt ; sur les VM, `docker rmi` après chaque contrôle.
- **Scripts** : bash, `set -euo pipefail`, commentaires en français ; `check-docs.sh` et `check-dist.sh` tournent aussi avec le bash 3.2 de macOS. Sur le Mac : `export PATH=/opt/homebrew/bin:$PATH` avant `npm`, `DOCKER_HOST=unix://$HOME/.docker/run/docker.sock` pour Docker Desktop. Sorties filtrées par `| tail -n 20`.
- **Langue** : tout texte que l'agent émet est en anglais (tâche 2, garde `TestAgentMessagesInEnglish`) ; commentaires du code, commits, spec et plans en français (CLAUDE.md).
- **Commits** en français, terminés par une ligne vide puis `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Jamais** de tag poussé, de release publiée, d'image poussée sur GHCR, de changement de visibilité sans l'étape ARRÊT correspondante ; jamais de tag déplacé ni de `push --force` sur un tag.

## Review Focus

1. **Un utilisateur Fedora ou RHEL suit la doc systemd** : `sudo hublot …` répond « command not found » (`secure_path`). → Tâche 3 : `check-systemd.sh` appelle `/usr/local/bin/hublot` comme la doc et tourne sur `hk-fedora` ; tâche 5 : la doc écrit le chemin complet.
2. **Le jeton d'accès finit dans un journal lisible par d'autres** (premier démarrage sous systemd : journal lisible par `adm` et `systemd-journal`). → Tâche 3 : `check-systemd.sh` cherche le jeton dans `journalctl -u hublot-kit` et échoue s'il y est.
3. **Mise à jour : le conteneur est recréé** ; l'utilisateur s'attend à garder son jeton, sans nouveau jeton affiché. → Tâche 4 : `check-image.sh` recrée le conteneur sur le même volume et se reconnecte avec le même jeton.
4. **Après une 0.1.1, l'utilisateur vérifie `:0.1`** avec la commande de la doc : l'identité doit accepter tout tag `v0.1.N` et refuser une rc. → Tâche 5 : expression régulière de la doc ; tâche 10 : la commande de la doc lancée telle quelle sur `:0.1`.
5. **Un inconnu sans compte GitHub** tire `:0.1`, lance la commande de §8 et vérifie la signature. → Tâche 10 : `docker logout`, puis tirage anonyme, `check-image.sh` sur `:0.1` sans modification, et les commandes `cosign` de la doc telles quelles.

## Étapes extérieures et irréversibles

| Étape | Tâche | Ce qu'elle rend visible ou définitif |
|---|---|---|
| Pousser le tag `v0.1.0-rc.1` ; lancer `release.yml` sur ce tag | 9, étapes 1 et 3 | tag sur GitHub ; puis (lancement) image `0.1.0-rc.1` poussée sur GHCR (paquet privé), release brouillon, entrées permanentes dans Rekor (nom du dépôt, workflow, tag) |
| Pousser le tag `v0.1.0`, puis lancer `release.yml` sur ce tag | 10, étape 2 | idem, images `0.1.0` et `0.1` |
| Rendre le dépôt public, activer le signalement privé de vulnérabilités | 10, étape 5 | tout l'historique, les plans, les adresses e-mail des commits |
| Publier la release `v0.1.0` | 10, étape 6 | release visible, marquée « Latest » |
| Rendre public le paquet GHCR | 10, étape 7 | `docker pull` anonyme ; ne se fait que dans l'interface web de GitHub |
| Supprimer le brouillon de la rc (facultatif) | 10, étape 9 | suppression définitive du brouillon |
| Supprimer de GHCR la version d'image d'une release dont le job `verify` échoue alors que le paquet est déjà public (toute release lancée après la tâche 10, étape 7) | 9, étape 3 (consigne en cas d'échec) | suppression définitive de cette version (interface web du paquet, ou `gh api -X DELETE` avec `delete:packages`) ; jusque-là, l'image refusée est tirable par tous. Le brouillon de release reste privé |

Ne sont pas des étapes ARRÊT : les deux lancements refusés de la tâche 9, étape 2 (seul le garde tourne) ; pousser la branche `phase/lancement` et ouvrir sa PR en brouillon (privées, comme aux phases précédentes) ; la petite PR de `release.yml` (tâche 9, étape 0) ; les fusions restent faites par l'utilisateur. Pousser un tag seul ne publie rien (Q54) : c'est le lancement de `release.yml` qui publie.

## Où tester

| Repère | Commande (racine du dépôt) |
|---|---|
| **macOS** | `go test ./cmd/... ./internal/... 2>&1 \| tail -n 20` ; `scripts/check-docs.sh` |
| **Docker Desktop (image locale)** | `export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock && scripts/check-image.sh <image> <version> 2>&1 \| tail -n 12` |
| **CI** | `gh pr checks --watch` (PR en brouillon ouverte à la tâche 6) ; un job : `gh run view <id> --log-failed \| tail -n 40` |
| **VM Ubuntu / Fedora** | `limactl start hk-fedora` au besoin ; `limactl copy <fichier> hk-ubuntu:/tmp/` ; `limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/check-image.sh … 2>&1 \| tail -n 12` (idem `check-systemd.sh`, idem `hk-fedora`) |
| **AppArmor / SELinux** | `limactl shell hk-ubuntu -- sudo journalctl -k --since -15min \| grep -c 'apparmor="DENIED"'` (attendu `0`) ; `limactl shell hk-fedora -- sudo ausearch -m avc -ts recent` (attendu `<no matches>`) |

## Structure des fichiers

```
cmd/hublot/main.go, main_test.go       --version (tâche 1)
cmd/hublot/messages_test.go            garde : aucune chaîne française dans l'agent (tâche 2)
cmd/, internal/ (27 fichiers)          messages en anglais (tâche 2)
packaging/systemd/hublot-kit.service   unité systemd livrée dans les archives (tâche 3)
scripts/release-lib.sh                 fonctions communes : commande de §8, cinq cibles, connexion, contrôles (tâche 3)
scripts/check-systemd.sh               installe l'unité comme la doc, explore, rotation du jeton, désinstalle (tâche 3)
Dockerfile                             image FROM scratch, contexte fourni par GoReleaser (tâche 4)
scripts/check-image.sh                 commande exacte de §8 sur une image, cinq cibles, mise à jour (tâche 4)
README.md, README.fr.md                vitrine EN, version FR (tâche 5)
docs/en/{install,security,limitations}.md, docs/fr/{installation,securite,limites}.md   (tâche 5)
scripts/check-docs.sh                  commande de §8 à l'octet près, FR/EN alignés, liens, root en tête (tâche 5)
.goreleaser.yaml                       binaires, archives, checksums, SBOM, signatures, image, release brouillon (tâche 6 ; correctifs : A dockers_v2, B archives, C signatures)
scripts/check-dist.sh                  contenu de dist/ (tâche 6)
.gitignore                             /dist/ (tâche 6)
.github/workflows/ci.yml               release-snapshot (tâche 6), job docs (tranche A)
scripts/test-release-lib.sh            gardes de release-lib, dont la garde cgroup (tâche 4, tranche A)
scripts/check-docs.sh                  blocs « Tranche A / B / C » : la doc contre .goreleaser.yaml, release.yml, verify-release.sh
.github/workflows/release.yml          lancement confirmé sur un tag, guard, web, release, verify (tranche C)
scripts/verify-release.sh              signatures, sommes, SBOM, plateformes d'une release (tranche C)
docs/superpowers/specs/…-design.md     §8, §10, §13 (tâche 8), §12 (tâche 10)
```

## Avant la tâche 1 : la branche

- [x] La branche `phase/lancement` existe déjà (partie de `main` après la PR #5, plan commité dessus) :

```bash
gh pr view 5 --json state --jq .state            # attendu : MERGED ; sinon, attendre l'utilisateur
git fetch origin
git switch phase/lancement && git pull --ff-only
git merge-base --is-ancestor origin/main HEAD && echo "main inclus"   # attendu : main inclus
```

---

### Task 1: `hublot --version`

**Où tester :** macOS.

**Files:**
- Modify: `cmd/hublot/main.go` (usage, début de `run`)
- Test: `cmd/hublot/main_test.go` (`TestRunHelp`, nouveau `TestRunVersion`)

**Interfaces:**
- Consumes: `var version` (rempli par `-X main.version=…`).
- Produces: `hublot --version` (ou `-version`, seul argument) écrit `hublot <version>\n` sur la sortie standard, code 0, avant toute lecture de `HUBLOT_*` et sans toucher au dossier de données. `check-dist.sh` (tâche 6) l'appelle sur le binaire de l'architecture courante ; la doc (tâche 5) le cite.

- [x] **Step 1: Écrire les tests** — dans `cmd/hublot/main_test.go`, ajouter `--version` à la liste de `TestRunHelp` et le test `TestRunVersion` (partie test du diff ci-dessous) :

```diff
diff --git a/cmd/hublot/main_test.go b/cmd/hublot/main_test.go
index e4d2897..c859be9 100644
--- a/cmd/hublot/main_test.go
+++ b/cmd/hublot/main_test.go
@@ -51,7 +51,7 @@ func TestRunHelp(t *testing.T) {
 		if code := run(context.Background(), []string{flag}, noenv, &out, &errb); code != 0 {
 			t.Fatalf("%s: want exit 0, got %d (stderr %q)", flag, code, errb.String())
 		}
-		for _, want := range []string{"--listen", "--data-dir", "--read-only", "--tls-cert", "--tls-key", "--public-origin", "reset-token"} {
+		for _, want := range []string{"--listen", "--data-dir", "--read-only", "--tls-cert", "--tls-key", "--public-origin", "--version", "reset-token"} {
 			if !strings.Contains(out.String(), want) {
 				t.Errorf("%s: usage lacks %q: %q", flag, want, out.String())
 			}
@@ -59,6 +59,33 @@ func TestRunHelp(t *testing.T) {
 	}
 }
 
+func TestRunVersion(t *testing.T) {
+	old := version
+	version = "1.2.3"
+	t.Cleanup(func() { version = old })
+	for _, flag := range []string{"--version", "-version"} {
+		var out, errb bytes.Buffer
+		// The data directory is never touched: --version answers before
+		// reading the configuration (a file stands where it would go).
+		blocker := filepath.Join(t.TempDir(), "file")
+		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
+			t.Fatal(err)
+		}
+		env := func(k string) string {
+			if k == "HUBLOT_DATA_DIR" {
+				return filepath.Join(blocker, "data")
+			}
+			return ""
+		}
+		if code := run(context.Background(), []string{flag}, env, &out, &errb); code != 0 {
+			t.Fatalf("%s: code %d, stderr %q", flag, code, errb.String())
+		}
+		if out.String() != "hublot 1.2.3\n" || errb.Len() != 0 {
+			t.Errorf("%s: out %q, stderr %q", flag, out.String(), errb.String())
+		}
+	}
+}
+
 func TestRunDataDirUnwritable(t *testing.T) {
 	parent := t.TempDir()
 	blocker := filepath.Join(parent, "file")
```

- [x] **Step 2: Les voir échouer**

Run: `go test ./cmd/hublot/ 2>&1 | tail -n 8`
Expected: FAIL — `TestRunHelp: -h: usage lacks "--version"` et `TestRunVersion: --version: code 2, stderr "configuration : flag provided but not defined: -version\n"`.

- [x] **Step 3: Écrire le code** (`cmd/hublot/main.go`) :

```diff
diff --git a/cmd/hublot/main.go b/cmd/hublot/main.go
index 0b36f18..690349c 100644
--- a/cmd/hublot/main.go
+++ b/cmd/hublot/main.go
@@ -34,6 +34,7 @@ var selfCgroup = func() ([]byte, error) { return os.ReadFile("/proc/self/cgroup"
 
 const usage = `Usage : hublot [options]
         hublot reset-token [--data-dir DIR]
+        hublot --version
 
 Options (chacune a un équivalent HUBLOT_*) :
   --listen ADDR          adresse d'écoute (défaut 127.0.0.1:7878)
@@ -55,6 +56,11 @@ func main() {
 }
 
 func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
+	// Before the configuration: --version never reads HUBLOT_* nor touches the data directory.
+	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
+		fmt.Fprintf(stdout, "hublot %s\n", version)
+		return 0
+	}
 	resetToken := len(args) > 0 && args[0] == "reset-token"
 	if resetToken {
 		args = args[1:]
```

- [x] **Step 4: Les voir passer**

Run: `gofmt -l cmd && go vet ./cmd/hublot && go test ./cmd/... ./internal/... 2>&1 | tail -n 12`
Expected: `gofmt` n'affiche rien ; `ok` pour chaque paquet.

Run: `d=$(mktemp -d) && CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=0.1.0" -o "$d/hublot" ./cmd/hublot && go version -m "$d/hublot" | grep GOOS; rm -rf "$d"`
Expected: `build	GOOS=linux` (le binaire Linux compile ; il ne s'exécute pas sur le Mac).

- [x] **Step 5: Commit**

```bash
git add cmd/hublot/main.go cmd/hublot/main_test.go
git commit -F - <<'EOF'
agent : option --version

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 6: Relecture (CLAUDE.md)** — un sous-agent indépendant, sans le contexte de l'implémentation, puis Codex, relisent le commit contre cette tâche. Points : `--version` répond avant `config.Load` ; `--version` avec d'autres arguments reste une erreur (code 2) ; message d'usage. Chaque correction retenue est un commit `correctif de revue : …` ; la tâche 2 ne commence qu'après les deux avis.

---

### Task 2: Messages de l'agent en anglais (Q49)

**Où tester :** macOS, puis `scripts/test-linux.sh` (les fichiers `*_linux.go` ne compilent que sous Linux), puis `scripts/e2e.sh` sur Docker Desktop.

**Files:**
- Create: `cmd/hublot/messages_test.go`
- Modify (code, 27 fichiers, 169 chaînes) : `cmd/hublot/main.go` ; `internal/api/changes.go`, `container.go`, `fs.go`, `fs_preview.go`, `fs_write.go`, `fsguard.go`, `logs.go` ; `internal/auth/token.go` ; `internal/config/config.go` ; `internal/dockerx/dockerx.go`, `fake.go`, `logs.go` ; `internal/files/files.go` ; `internal/fsview/cgroup.go`, `fsview.go`, `memfs.go`, `mountpoint_linux.go`, `mounts.go`, `procfs_archive_linux.go`, `procfs_linux.go`, `procfs_other.go`, `pseudofs_linux.go`, `root_linux.go`, `tarindex.go`, `views.go` ; `internal/web/web.go`
- Modify (tests qui comparent ces messages, 27 assertions dans 8 fichiers) : `cmd/hublot/main_test.go`, `internal/fsview/mounts_test.go`, `internal/fsview/views_test.go`, `internal/fsview/procfs_archive_linux_test.go`, `internal/files/files_test.go`, `internal/api/logs_test.go`, `internal/api/fs_write_test.go`, `internal/api/integration_write_linux_test.go` ; données de test de l'interface, par cohérence : `web/src/lib/write.test.ts`, `web/src/lib/logs.test.ts`
- Modify (scripts qui lisent la sortie de l'agent) : `scripts/e2e.sh`, `.github/workflows/ci.yml` (job `binary`)
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§8)

**Interfaces:**
- Consumes: `hublot --version` (tâche 1), le message d'usage qu'elle a complété.
- Produces: les lignes exactes que lisent les tâches suivantes et la doc : `Access token (shown only once): <jeton>` (premier lancement), `New access token (shown only once): <jeton>` (`reset-token`), `WARNING: listening on <adresse> without TLS. …`, `WARNING: private cgroup namespace (/proc/self/cgroup = 0::/): …`, `Hublot-Kit <version> listening on <adresse>`. `scripts/check-systemd.sh` (tâche 3) et `scripts/check-image.sh` (tâche 4) extraient le jeton par `sed -n 's/^New access token (shown only once): //p'` et `sed -n 's/^Access token (shown only once): //p'`.

> **À comprendre :** tout texte humain que l'agent affiche (sortie standard et d'erreur, aide), journalise (`slog`, déjà en anglais) ou renvoie dans le `detail` d'une erreur d'API passe en anglais. Les **codes** d'erreur de §6 ne changent pas, ni l'interface (dictionnaires FR/EN), ni les commentaires du code, les commits, la spec et les plans, qui restent en français (CLAUDE.md). Inventaire (relevé sur `main` après la PR #5, outil `go/ast` sur les fichiers non-test de `cmd/` et `internal/`) : **169 chaînes dans 27 fichiers** — 133 messages à traduire, 35 chaînes de format à la typographie française (`%w : %s` → `%w: %s`), et le message d'usage. Le test de garde de l'étape 1 les retrouve toutes, et empêche le retour d'une chaîne française.
>
> **Signalé (l'interface montre un `detail` du serveur tel quel) :** `web/src/lib/files.ts` (`downloadErrorMessage` pour `forbidden_path`, `fsErrorMessage` pour l'explorateur indisponible d'un conteneur arrêté) et `web/src/lib/write.ts` (`writeErrorMessage` pour `read_only` et `forbidden_path`) ajoutent le `detail` entre parenthèses après le message traduit, échappé par Svelte (`files.ts:78`, `files.ts:95`, `write.ts:66`, `write.ts:72`). Après cette tâche, ce détail technique (le montage qui bloque un tar, le refus de Docker) s'affiche en anglais dans l'interface française ; avant, il s'affichait en français dans l'interface anglaise. Rien à changer dans l'interface : le détail nomme une cause technique (chemin, montage) que l'utilisateur doit pouvoir chercher. Aucun autre `detail` n'est affiché (les flux de logs, `streamEnd`, ne lisent que le code).

- [x] **Step 1: Écrire les tests**

Le test de garde, `cmd/hublot/messages_test.go` :

```go
package main

// The agent speaks English (plan Lancement, Q49): every human text it
// prints, logs or returns in an API "detail" is English. This guard parses
// the production code of cmd/ and internal/ and refuses any string literal
// that looks French: an accented letter, a space before a colon (French
// typography: "%w : %s"), or a common French word. Code comments, tests and
// the interface dictionaries (web/) are not concerned.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var frenchText = regexp.MustCompile(`(?i)[àâçéèêëîïôûùüœ]|\s:(\s|$)|\b(le|la|les|des|du|de|un|une|est|pas|sans|sur|ou|et|au|aux|ce|dans|pour|par|avec|jeton|dossier|lien|chemin|fichier|conteneur|montage|refus|ensemble|journal|indisponible|lecture|seule|trop|volumineux|indexation|cours|plafond|atteint|certificat|octets|client Docker)\b`)

func TestAgentMessagesInEnglish(t *testing.T) {
	var found []string
	for _, dir := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.ImportSpec, *ast.Field: // import paths, struct tags
					return false
				case *ast.BasicLit:
					if s, err := strconv.Unquote(x.Value); x.Kind == token.STRING && err == nil && frenchText.MatchString(s) {
						found = append(found, fset.Position(x.Pos()).String()+" "+strconv.Quote(s))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) > 0 {
		t.Errorf("%d French strings in the agent:\n%s", len(found), strings.Join(found, "\n"))
	}
}
```

Les assertions qui citent un message, remplacées par leur version anglaise : 27 assertions dans 8 fichiers Go (relevé par `go/ast` : toute chaîne d'un test, hors messages de `t.Fatalf`/`t.Errorf` et noms de cas, qui est un morceau d'un message de l'agent), plus 2 données Vitest. Les numéros de ligne sont ceux de `main` après la PR #5 ; les assertions marquées « absent » restent négatives.

| Fichier : ligne (test) | Avant | Après |
|---|---|---|
| `cmd/hublot/main_test.go:33` (`TestResetTokenSubcommand`) | `"jeton"` | `"New access token"` |
| `cmd/hublot/main_test.go:86` (`TestRunStartsAndStops`), `:100` (absent, `TestRunListenFailureKeepsNoOrphanToken`), `:170` (absent, `runRefused`) | `"Jeton d'accès"` | `"Access token"` |
| `cmd/hublot/main_test.go:170` (absent, `runRefused`) | `"Nouveau jeton"` | `"New access token"` |
| `cmd/hublot/main_test.go:198`, `:209` (`TestRunRefusesSymlinkDataDir`) | `"lien symbolique"` | `"symbolic link"` |
| `cmd/hublot/main_test.go:220` (`TestRunRefusesDataDirThatIsAFile`) | `"pas un dossier"` | `"not a directory"` |
| `cmd/hublot/main_test.go:239` (`TestRunRefusesDataDirOwnedByAnotherUser`, root seulement) | `"appartient"` | `"owned by uid"` |
| `cmd/hublot/main_test.go:263` (absent, `TestRunBadTLSCert`) | `"écoute sur"` | `"listening on"` |
| `internal/fsview/mounts_test.go:50` (`TestHostBind`) | `"options illisibles"` | `"unreadable options"` |
| `internal/fsview/mounts_test.go:125` (`TestPseudoMountUnder`) | `"illisible"` | `"unreadable"` |
| `internal/fsview/mounts_test.go:161` (`TestPseudoMountUnderEdgeCases`) | `"échappement"` | `"escape"` |
| `internal/fsview/mounts_test.go:201` (`TestCheckVisibleMounts`) | `"ne montre pas la destination /link/m"` | `"does not show the destination /link/m"` |
| `internal/fsview/mounts_test.go:207` (`TestCheckVisibleMounts`, chaîne brute) | `` `pas de racine "/"` `` | `` `has no "/" root` `` |
| `internal/fsview/views_test.go:137` (`TestViewsHostBindBuildsNothing`, chaîne brute) | `` `pilote "rexray" inconnu` `` | `` `unknown driver "rexray"` `` |
| `internal/fsview/views_test.go:138` (idem) | `"options illisibles"` | `"unreadable options"` |
| `internal/fsview/procfs_archive_linux_test.go:100` (`TestCheckArchiveContainerWideRule`) | `"sous le montage /sys"` | `"under the container mount /sys"` |
| `internal/fsview/procfs_archive_linux_test.go:157` (`TestCheckArchiveDestinationNotVisible`) | `"ne montre pas la destination /link/m"` | `"does not show the destination /link/m"` |
| `internal/fsview/procfs_archive_linux_test.go:171` (`TestCheckArchiveDestinationUnderSymlink`) | `"ne montre pas la destination"` | `"does not show the destination"` |
| `internal/fsview/procfs_archive_linux_test.go:218` (`TestCheckWriteRoot`) | `"racine"` | `"root"` |
| `internal/fsview/procfs_archive_linux_test.go:266` (`TestCheckWriteRootJail`) | `"racine épinglée"` | `"root pinned"` |
| `internal/files/files_test.go:393` (`TestPutFailureIsAudited`) | `"refusé"` | `"refused"` |
| `internal/api/fs_write_test.go:350` (`TestWriteTimeoutDetail`) | `"peut encore aboutir"` | `"may still complete"` |
| `internal/api/logs_test.go:472` (`TestLogsDownloadSlots`) | `"flux de logs"` | `"log streams"` |
| `internal/api/integration_write_linux_test.go:89` (`TestIntegrationWriteRunning`) | `"point de montage"` | `"mount point"` |
| `web/src/lib/write.test.ts:54` | `'délai dépassé'` | `'timed out'` |
| `web/src/lib/logs.test.ts:69` | `"session terminée"` | `"session ended"` |

Restent tels quels : `--cgroupns=host` (`TestRunWarnsPrivateCgroupNamespace`, `cgroup_test.go`), `"make web"` (`web_test.go`), les noms de cas et les messages d'échec des tests (en français, comme les commentaires). Contrôle après coup : `grep -rnE '"(jeton|Jeton|lien symbolique|pas un dossier|appartient|illisible|échappement|racine|refusé|flux de logs|point de montage|peut encore aboutir)' --include='*_test.go' cmd internal` ne doit plus rien trouver dans une assertion.

- [x] **Step 2: Les voir échouer**

Run: `go test ./cmd/... ./internal/... 2>&1 | grep -E '^(--- FAIL|FAIL|ok)|French strings in the agent' | head -n 20`
Expected: `169 French strings in the agent` (`TestAgentMessagesInEnglish` ; vérifié à la rédaction sur une copie avec la tâche 1) ; dans `cmd/hublot`, `--- FAIL:` pour `TestResetTokenSubcommand`, `TestRunStartsAndStops`, `TestRunRefusesSymlinkDataDir`, `TestRunRefusesDataDirThatIsAFile` (sur macOS, `TestRunRefusesDataDirOwnedByAnotherUser` se saute : il faut root) ; dans `internal/fsview`, `TestHostBind`, `TestPseudoMountUnder`, `TestPseudoMountUnderEdgeCases`, `TestCheckVisibleMounts`, `TestViewsHostBindBuildsNothing` ; `TestPutFailureIsAudited` (`internal/files`) ; `TestWriteTimeoutDetail` et `TestLogsDownloadSlots` (`internal/api`). Les autres paquets : `ok`.

Run: `scripts/test-linux.sh -tags integration ./cmd/hublot/ ./internal/fsview/ ./internal/api/ 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | head -n 20`
Expected: en plus, sous Linux root : `TestRunRefusesDataDirOwnedByAnotherUser`, `TestCheckArchiveContainerWideRule`, `TestCheckArchiveDestinationNotVisible`, `TestCheckArchiveDestinationUnderSymlink`, `TestCheckWriteRoot`, `TestCheckWriteRootJail` et `TestIntegrationWriteRunning` en échec (lancer aussi `./cmd/hublot/` pour le premier).

- [x] **Step 3: Traduire**

1. Le message d'usage de `cmd/hublot/main.go` devient :

```go
const usage = `Usage: hublot [options]
       hublot reset-token [--data-dir DIR]
       hublot --version

Options (each has a HUBLOT_* equivalent):
  --listen ADDR          listen address (default 127.0.0.1:7878)
  --data-dir DIR         data directory (default /var/lib/hublot-kit)
  --read-only            disable every write
  --tls-cert FILE        TLS certificate (with --tls-key)
  --tls-key FILE         TLS key (with --tls-cert)
  --public-origin URL    public origin behind a reverse proxy

Subcommand:
  reset-token            create a new access token and invalidate the old one
`
```

2. Dans `run`, l'aide `fail` imprime `what, ":", err` par `Fprintln`, ce qui donne `token : …` (le test de garde ne voit pas ce `":"` isolé) : la remplacer par

```go
		fmt.Fprintf(stderr, "%s: %v\n", what, err)
```

3. Chaque chaîne du tableau ci-dessous, à l'identique (repérer par `grep -nF` sur un fragment). Trois chaînes contiennent `"/"`, écrit `\"/\"` dans le source (`mounts.go`, deux fois ; `procfs_archive_linux.go`, une fois) : le tableau les montre décodées, et `grep -F` ne les trouve pas telles quelles ; chercher un fragment sans guillemets (`grep -n "n'a pas de racine" internal/fsview/mounts.go`, `grep -n 'du process n.est pas celui' internal/fsview/procfs_archive_linux.go`) et garder l'échappement `\"/\"` dans la traduction. Règle pour les chaînes de format sans texte français (`%w : %s`, `%w : %v`, `%w : %q`, `%w : %s:%s`, `%w : %s : %s`, `%w : bind %s -> %s`, `%w : volume %s (type=%s) -> %s`, `%w : bind %s -> %s (volume %s)`, `%w : %s (If-Match)`, `%w : volume %s`, `fstatfs %s : %w`, `pidfd_open(%d) : %v`) : retirer l'espace avant le deux-points (`%w: %s`…). Les commentaires voisins restent en français.

**`cmd/hublot/main.go`** (22 chaîne(s))

| Avant | Après |
|---|---|
| `configuration :` | `configuration:` |
| `dossier de données %s : %v\n` | `data directory %s: %v\n` |
| `reset-token :` | `reset-token:` |
| `Nouveau jeton d'accès (affiché une seule fois) : %s\n` | `New access token (shown only once): %s\n` |
| `certificat TLS :` | `TLS certificate:` |
| `ATTENTION : écoute sur %s sans TLS. À réserver à un conteneur publié sur 127.0.0.1 ou à un reverse proxy HTTPS.\n` | `WARNING: listening on %s without TLS. Only for a container published on 127.0.0.1 or behind an HTTPS reverse proxy.\n` |
| `écoute :` | `listen:` |
| `jeton :` | `token:` |
| `Jeton d'accès (affiché une seule fois) : %s\n` | `Access token (shown only once): %s\n` |
| <code>Le jeton affiché plus haut reste valide ; en cas de doute, lancez `hublot reset-token`.</code> | <code>The token printed above stays valid; if in doubt, run `hublot reset-token`.</code> |
| `jeton` | `token` |
| `journal d'audit` | `audit log` |
| `client Docker` | `Docker client` |
| `ATTENTION : espace de noms cgroup privé (/proc/self/cgroup = 0::/) : l'explorateur répondra container_changed. Lancez l'agent avec --cgroupns=host.` | `WARNING: private cgroup namespace (/proc/self/cgroup = 0::/): the explorer will answer container_changed. Start the agent with --cgroupns=host.` |
| `Hublot-Kit %s écoute sur %s\n` | `Hublot-Kit %s listening on %s\n` |
| `serveur :` | `server:` |
| `arrêt :` | `shutdown:` |
| `c'est un lien symbolique : refusé ; indiquez directement un dossier dédié, par exemple /var/lib/hublot-kit` | `it is a symbolic link: refused; give a dedicated directory directly, for example /var/lib/hublot-kit` |
| `ce n'est pas un dossier` | `not a directory` |
| `appartient à l'uid %d, pas à l'utilisateur courant (uid %d) ; utilisez un dossier dédié, par exemple /var/lib/hublot-kit` | `owned by uid %d, not by the current user (uid %d); use a dedicated directory, for example /var/lib/hublot-kit` |
| `accessible en écriture au groupe ou à tous (%v) ; utilisez un dossier dédié, par exemple /var/lib/hublot-kit` | `writable by group or others (%v); use a dedicated directory, for example /var/lib/hublot-kit` |

**`internal/api/changes.go`** (1 chaîne(s))

| Avant | Après |
|---|---|
| `identifiant de conteneur invalide` | `invalid container ID` |

**`internal/api/container.go`** (3 chaîne(s))

| Avant | Après |
|---|---|
| `%w : variable inconnue` | `%w: unknown variable` |
| `données après l'objet JSON` | `data after the JSON object` |
| `journal d'audit : rien n'est révélé` | `audit log: nothing revealed` |

**`internal/api/fs.go`** (2 chaîne(s))

| Avant | Après |
|---|---|
| `identifiant de conteneur invalide` | `invalid container ID` |
| `chemin invalide` | `invalid path` |

**`internal/api/fs_preview.go`** (1 chaîne(s))

| Avant | Après |
|---|---|
| `%w : aperçu limité à %d octets` | `%w: preview limited to %d bytes` |

**`internal/api/fs_write.go`** (5 chaîne(s) ; et 1 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `%w : corps multipart attendu (%v)` | `%w: multipart body expected (%v)` |
| `%w : plusieurs champs file` | `%w: several file fields` |
| `%w : aucun champ file` | `%w: no file field` |
| `%w ; l'écriture peut encore aboutir : rechargez le fichier pour le vérifier` | `%w; the write may still complete: reload the file to check` |

**`internal/api/fsguard.go`** (2 chaîne(s))

| Avant | Après |
|---|---|
| `trop d'opérations de fichiers en cours sur ce conteneur (montage FUSE bloqué ?)` | `too many file operations in progress on this container (stuck FUSE mount?)` |
| `délai dépassé (montage FUSE bloqué ?)` | `timed out (stuck FUSE mount?)` |

**`internal/api/logs.go`** (5 chaîne(s))

| Avant | Après |
|---|---|
| `trop de flux de logs ouverts` | `too many open log streams` |
| `%w (%d au plus, tous des téléchargements)` | `%w (%d at most, all downloads)` |
| `%w : identifiant de conteneur invalide` | `%w: invalid container ID` |
| `flux remplacé par un plus récent (%d flux de logs au plus)` | `stream replaced by a newer one (%d log streams at most)` |
| `session terminée` | `session ended` |

**`internal/auth/token.go`** (2 chaîne(s))

| Avant | Après |
|---|---|
| <code>%s est un lien symbolique : refusé ; supprimez-le puis relancez `hublot reset-token`</code> | <code>%s is a symbolic link: refused; remove it, then run `hublot reset-token` again</code> |
| <code>%s corrompu : relancez `hublot reset-token`</code> | <code>%s is corrupt: run `hublot reset-token` again</code> |

**`internal/config/config.go`** (14 chaîne(s))

| Avant | Après |
|---|---|
| `adresse d'écoute` | `listen address` |
| `dossier de données` | `data directory` |
| `désactive toute écriture` | `disable every write` |
| `certificat TLS` | `TLS certificate` |
| `clé TLS` | `TLS key` |
| `origine publique derrière un reverse proxy` | `public origin behind a reverse proxy` |
| `arguments positionnels non autorisés: %v` | `positional arguments not allowed: %v` |
| `--tls-cert/HUBLOT_TLS_CERT et --tls-key/HUBLOT_TLS_KEY vont ensemble` | `--tls-cert/HUBLOT_TLS_CERT and --tls-key/HUBLOT_TLS_KEY go together` |
| `schéma http ou https attendu` | `http or https scheme expected` |
| `hôte manquant` | `missing host` |
| `authentification utilisateur non autorisée` | `user credentials not allowed` |
| `zone ID IPv6 non autorisée dans Host` | `IPv6 zone ID not allowed in Host` |
| `une origine n'a ni chemin, ni requête, ni fragment` | `an origin has no path, query or fragment` |

**`internal/dockerx/dockerx.go`** (9 chaîne(s) ; et 4 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `introuvable dans Docker` | `not found in Docker` |
| `refusé par Docker` | `refused by Docker` |
| `extraction refusée par Docker` | `extraction refused by Docker` |
| `Docker indisponible` | `Docker unavailable` |
| `%w : type de changement inconnu %d pour %s` | `%w: unknown change kind %d for %s` |

**`internal/dockerx/logs.go`** (4 chaîne(s) ; et 2 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `le pilote de logs de ce conteneur ne permet pas la lecture` | `the logging driver of this container cannot be read back` |
| `%w : type de trame de logs inconnu %d` | `%w: unknown log frame type %d` |

**`internal/files/files.go`** (20 chaîne(s) ; et 4 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `agent en lecture seule` | `agent is read-only` |
| `le fichier a changé ou existe déjà` | `the file changed or already exists` |
| `nom de fichier invalide` | `invalid file name` |
| `%w : édition limitée à %d octets` | `%w: edits limited to %d bytes` |
| `%w : %s est un point de montage du conteneur : modifiez sa source` | `%w: %s is a mount point of the container: edit its source` |
| `%w : envoi limité à %d octets` | `%w: uploads limited to %d bytes` |
| `%w : un envoi exige If-None-Match: *` | `%w: an upload requires If-None-Match: *` |
| `%w : %s existe` | `%w: %s exists` |
| `requête terminée avant l'écriture, rien n'est écrit : %w` | `request ended before the write, nothing written: %w` |
| `journal d'audit, rien n'est écrit : %w` | `audit log, nothing written: %w` |
| `%w : %s dépasse %d octets` | `%w: %s exceeds %d bytes` |
| `écriture refusée pour tout le conteneur : %w` | `write refused for the whole container: %w` |
| `%w : %s : %s est monté au démarrage du conteneur, Docker n'y écrit pas` | `%w: %s: %s is mounted when the container starts, Docker does not write there` |
| `%w : %s est géré et monté par Docker` | `%w: %s is managed and mounted by Docker` |
| `%w : %s est monté (%s %s) : modifiez sa source` | `%w: %s is mounted (%s %s): edit its source` |
| `%w : %s est sous le tmpfs %s, Docker n'y écrit pas` | `%w: %s is under the tmpfs %s, Docker does not write there` |

**`internal/fsview/cgroup.go`** (2 chaîne(s) ; et 1 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| ` ; lancez l'agent avec --cgroupns=host` | `; start the agent with --cgroupns=host` |

**`internal/fsview/fsview.go`** (7 chaîne(s))

| Avant | Après |
|---|---|
| `pas un dossier` | `not a directory` |
| `pas un fichier régulier` | `not a regular file` |
| `chemin interdit` | `forbidden path` |
| `lien symbolique dans le chemin` | `symbolic link in the path` |
| `le conteneur a changé` | `the container changed` |
| `trop volumineux` | `too large` |
| `indexation en cours` | `indexing in progress` |

**`internal/fsview/mountpoint_linux.go`** (1 chaîne(s))

| Avant | Après |
|---|---|
| `%w : point de montage indécidable` | `%w: cannot tell whether this is a mount point` |

**`internal/fsview/mounts.go`** (16 chaîne(s) ; et 4 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `%w : volume %s -> %s : pilote %q inconnu, refusé` | `%w: volume %s -> %s: unknown driver %q, refused` |
| `%w : volume %s -> %s : options illisibles (pas de client Docker)` | `%w: volume %s -> %s: unreadable options (no Docker client)` |
| `%w : volume %s -> %s : options illisibles (%v)` | `%w: volume %s -> %s: unreadable options (%v)` |
| `%w : montage de type %q inconnu -> %s, refusé` | `%w: mount of unknown type %q -> %s, refused` |
| `ligne de mountinfo illisible` | `unreadable mountinfo line` |
| `identifiant de montage illisible` | `unreadable mount ID` |
| `identifiant du montage parent illisible` | `unreadable parent mount ID` |
| `échappement de mountinfo invalide` | `invalid mountinfo escape` |
| `%w : la table des montages n'a pas de racine "/" : la racine du process n'est pas celle du conteneur (chroot du PID 1), écriture refusée` | `%w: the mount table has no "/" root: the process root is not the container's (chroot of PID 1), write refused` |
| `%w : la table des montages n'a pas de racine "/" : tar refusé` | `%w: the mount table has no "/" root: tar refused` |
| `%w : la table des montages ne montre pas la destination %s (racine du process déplacée, ou destination sous un lien) : tar refusé` | `%w: the mount table does not show the destination %s (process root moved, or destination under a link): tar refused` |

**`internal/fsview/procfs_archive_linux.go`** (9 chaîne(s))

| Avant | Après |
|---|---|
| `statx sans STATX_MNT_ID (noyau antérieur à 5.8)` | `statx without STATX_MNT_ID (kernel older than 5.8)` |
| `lecture de /proc/%d/mountinfo : %v` | `reading /proc/%d/mountinfo: %v` |
| `%w : le montage "/" du process n'est pas celui de la racine épinglée à l'ouverture de la vue (chroot du process) : écriture refusée` | `%w: the "/" mount of the process is not the one of the root pinned when the view was opened (chroot of the process): write refused` |
| `%w (écriture refusée : le process n'est peut-être pas dans la racine du conteneur, chroot du PID 1)` | `%w (write refused: the process may not be in the container's root, chroot of PID 1)` |
| `contrôle des montages impossible : vue sans process` | `mount check impossible: view without a process` |
| `%w : %s monté sur %s, sous le montage %s du conteneur : aucun tar` | `%w: %s mounted on %s, under the container mount %s: no tar` |
| `%w : %s monté sur %s, le tar de %s l'inclurait` | `%w: %s mounted on %s, the tar of %s would include it` |

**`internal/fsview/procfs_other.go`** (1 chaîne(s))

| Avant | Après |
|---|---|
| `explorateur : Linux uniquement (sur macOS, lancez l'agent dans un conteneur)` | `explorer: Linux only (on macOS, run the agent in a container)` |

**`internal/fsview/pseudofs_linux.go`** (2 chaîne(s) ; et 1 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `%w : %s est sur %s` | `%w: %s is on %s` |

**`internal/fsview/root_linux.go`** (10 chaîne(s) ; et 1 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `poll du pidfd du process %d : %w` | `poll of the pidfd of process %d: %w` |
| `le process %d s'est terminé` | `process %d has exited` |
| `lecture du cgroup du pid %d : %v` | `reading the cgroup of pid %d: %v` |
| `le pid %d n'est pas dans le cgroup du conteneur %.12s` | `pid %d is not in the cgroup of container %.12s` |
| `ouverture de /proc/%d/root : %v` | `opening /proc/%d/root: %v` |
| `ouverture de /proc/%d/root : %w ; causes mesurées (spec §8) : ` | `opening /proc/%d/root: %w; measured causes (spec §8): ` |
| `--cap-add SYS_PTRACE absent, ou cible non confinée (--privileged, apparmor=unconfined) ` | `--cap-add SYS_PTRACE missing, or unconfined target (--privileged, apparmor=unconfined) ` |
| `sans --security-opt apparmor=unconfined sur l'agent` | `without --security-opt apparmor=unconfined on the agent` |
| `ouverture de /proc/%d/root : %w` | `opening /proc/%d/root: %w` |

**`internal/fsview/tarindex.go`** (16 chaîne(s) ; et 6 chaîne(s) de format `… : …` → `…: …`)

| Avant | Après |
|---|---|
| `plafond d'octets atteint` | `byte cap reached` |
| `plafond de durée` | `time cap` |
| `%w : index au-delà de %d octets lus` | `%w: index beyond %d bytes read` |
| `%w : index au-delà de %s (%w)` | `%w: index beyond %s (%w)` |
| `lecture de l'archive de / : %w` | `reading the archive of /: %w` |
| `%w : index au-delà de %d entrées` | `%w: index beyond %d entries` |
| `%w : %s (lien physique vers %s)` | `%w: %s (hard link to %s)` |
| `archive de %s : %w` | `archive of %s: %w` |
| `%s a changé depuis l'indexation` | `%s changed since indexing` |
| `archive d'un fichier : plus d'une entrée` | `archive of a file: more than one entry` |

**`internal/fsview/views.go`** (2 chaîne(s))

| Avant | Après |
|---|---|
| `%w : index au-delà de %s (%w)` | `%w: index beyond %s (%w)` |
| `le conteneur %.12s a démarré pendant l'indexation` | `container %.12s started during indexing` |

**`internal/web/web.go`** (1 chaîne(s))

| Avant | Après |
|---|---|
| <code>interface non compilée : lancez `make web`</code> | <code>interface not built: run `make web`</code> |

**`internal/dockerx/fake.go`** : 5 chaîne(s) de format `%w : %s` (ou `%w : %v`…) → `%w: %s`.

**`internal/fsview/memfs.go`** : 3 chaîne(s) de format `%w : %s` (ou `%w : %v`…) → `%w: %s`.

**`internal/fsview/procfs_linux.go`** : 3 chaîne(s) de format `%w : %s` (ou `%w : %v`…) → `%w: %s`.

4. Les scripts qui lisent la sortie : dans `scripts/e2e.sh`, `sed -n 's/^.* : //p'` devient `sed -n 's/^New access token (shown only once): //p'` ; dans le job `binary` de `.github/workflows/ci.yml`, `grep -q "jeton"` devient `grep -q "New access token"`.

- [x] **Step 4: Les voir passer**

Run: `gofmt -l cmd internal && go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/... 2>&1 | tail -n 12`
Expected: rien pour `gofmt` ; `ok` pour chaque paquet, `TestAgentMessagesInEnglish` compris.

Run: `scripts/test-linux.sh -race -tags integration -count=1 ./cmd/... ./internal/... 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | tail -n 12`
Expected: `ok` pour chaque paquet.

Run: `export PATH=/opt/homebrew/bin:$PATH DOCKER_HOST=unix://$HOME/.docker/run/docker.sock && (cd web && npx vitest run 2>&1 | tail -n 4) && scripts/e2e.sh 2>&1 | tail -n 5`
Expected: Vitest vert ; le parcours Playwright passe (le jeton est bien extrait de la nouvelle ligne).

Run: `make build >/dev/null && d=$(mktemp -d) && ./bin/hublot reset-token --data-dir "$d/data"; ./bin/hublot --nope; ./bin/hublot --help | head -n 3; rm -rf "$d"`
Expected: `New access token (shown only once): <jeton>`, `configuration: flag provided but not defined: -nope`, puis les trois lignes `Usage: hublot [options]` … `hublot --version`.

- [x] **Step 5: Reporter dans la spec** (§8 ; chaque passage est unique, vérifier par `grep -c`)

1. Remplacer :

   ```text
   porte le détail « lancez l'agent avec --cgroupns=host ».
   ```

   par :

   ```text
   porte le détail « start the agent with --cgroupns=host ».
   ```

2. Remplacer :

   ```text
   **Contrôle au démarrage** :
   ```

   par :

   ```text
   **Langue de l'agent** : tout texte humain que l'agent affiche (sortie standard et d'erreur, aide), journalise (`slog`) ou renvoie dans le `detail` d'une erreur d'API est en anglais ; les codes d'erreur de §6 ne changent pas. L'interface est traduite (§9) et affiche certains `detail` tels quels, entre parenthèses (montage qui bloque un tar, refus de Docker en lecture seule) : ils sont en anglais dans les deux langues. Un test (`cmd/hublot/messages_test.go`) refuse toute chaîne française dans le code de l'agent (plan Lancement, Q49).

   **Contrôle au démarrage** :
   ```

- [x] **Step 6: Commit**

```bash
git add cmd internal web/src/lib/write.test.ts web/src/lib/logs.test.ts scripts/e2e.sh .github/workflows/ci.yml docs/superpowers/specs
git commit -F - <<'EOF'
agent : messages en anglais (sortie, aide, journal, detail de l'API)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 7: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex. Points : plus aucune chaîne française dans ce que l'agent émet (garde verte, et relecture des `fmt.Fprint*` qui assemblent des morceaux) ; codes d'erreur inchangés ; traductions exactes (un `detail` qui nomme un montage le nomme toujours) ; les scripts et la CI lisent la nouvelle ligne du jeton ; commentaires toujours en français. Corrections en commits séparés ; la tâche 3 attend les deux avis.

---

### Task 3: Unité systemd, vérifiée sur les deux VM

**Où tester :** `hk-ubuntu` (AppArmor) et `hk-fedora` (SELinux), avec un binaire arm64 compilé sur le Mac.

**Files:**
- Create: `scripts/release-lib.sh`, `scripts/check-systemd.sh`, `packaging/systemd/hublot-kit.service`

**Interfaces:**
- Consumes: `hublot reset-token` (écrit `New access token (shown only once): <jeton>`, tâche 2), `POST /api/login` (`{"token"}`, `Origin` exigé, `401` si refusé), `GET /api/meta`, `GET /api/containers/{nom}/fs?path=`, `GET /api/containers/{nom}/fs/file?path=`.
- Produces: `scripts/release-lib.sh` (à sourcer) : `HK_TARGETS`, `HK_BASE`, `hk_fail MESSAGE`, `hk_spec_command` (la commande de §8), `hk_start_targets`, `hk_remove_targets`, `hk_wait_login`, `hk_login JETON COOKIES`, `hk_check_meta COOKIES [VERSION]`, `hk_check_targets COOKIES` — utilisés par `check-image.sh` (tâche 4) et `check-docs.sh` (tâche 5). `scripts/check-systemd.sh ARCHIVE VERSION` (version obligatoire), lancé en root, utilisé par les tâches 8 et 9. L'unité `hublot-kit.service`, que l'archive de la tâche 6 embarque à sa racine.

> **À comprendre :** l'unité lance le binaire en root, comme le dit §8 : il lit `/proc/<pid>/root` des conteneurs et parle au socket. Les directives de durcissement ne servent qu'à limiter les erreurs ; toute directive qui cache `/proc` des autres process ou retire des capacités casserait l'explorateur (Q41). Le script installe exactement comme la doc de la tâche 5 (`install`, unité, `reset-token` **avant** le premier démarrage, `enable --now`), puis désinstalle tout, même en cas d'échec ; il refuse de toucher une machine où Hublot-Kit est déjà installé. Il **écrit** aussi (`hk_check_writes` : une édition `PUT fs/file` avec `If-Match`, puis un envoi `POST fs/upload` avec `If-None-Match: *`, relus par `docker exec`) : c'est sous `ProtectSystem=strict`, `NoNewPrivileges` et, sur Fedora, `unconfined_service_t` que l'écriture doit passer. Le sous-agent de revue l'a fait à la main sur `hk-ubuntu`, avec succès ; Fedora reste à éprouver. Le journal est lu une fois, depuis le démarrage du test, puis cherché (sous `pipefail`, `journalctl | grep -q` peut finir en SIGPIPE et masquer une fuite).

- [x] **Step 1: Écrire les fonctions communes et le script de vérification**

`scripts/release-lib.sh` :

```bash
# Fonctions communes de scripts/check-image.sh et scripts/check-systemd.sh
# (plan Lancement). À sourcer, pas à lancer. Bash 3.2 (macOS) compris.
#
# Cinq cibles alpine:3, comme le spike n°3 et 3b (spec §8, §11) : ordinaire,
# --privileged, apparmor=unconfined, uid 1000, et un conteneur arrêté. Plus
# une édition et un envoi dans la cible ordinaire (spec §5.3), relus par
# docker exec.

HK_TARGETS="hk-check-normal hk-check-priv hk-check-unconf hk-check-uid hk-check-stopped"
HK_BASE=http://127.0.0.1:7878

hk_fail() { echo "ÉCHEC : $*" >&2; exit 1; }

# La commande `docker run` de la spec §8, à l'octet près (celle de la doc :
# scripts/check-docs.sh vérifie que la doc la reprend telle quelle).
hk_spec_command() {
  awk '/^docker run -d --name hublot-kit/{on=1} on{print} on&&/ghcr\.io\/levy-cmd\/hublot-kit:0\.1$/{exit}' \
    docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
}

hk_start_targets() {
  for c in $HK_TARGETS; do
    if docker container inspect "$c" >/dev/null 2>&1; then hk_fail "le conteneur $c existe déjà"; fi
  done
  docker pull -q alpine:3 >/dev/null
  docker run -d --name hk-check-normal alpine:3 sleep 600 >/dev/null
  docker exec hk-check-normal sh -c 'echo before > /root/hk-edit.txt'
  docker run -d --name hk-check-priv --privileged alpine:3 sleep 600 >/dev/null
  docker run -d --name hk-check-unconf --security-opt apparmor=unconfined alpine:3 sleep 600 >/dev/null
  docker run -d --name hk-check-uid --user 1000:1000 alpine:3 sleep 600 >/dev/null
  docker run --name hk-check-stopped alpine:3 true
}

hk_remove_targets() {
  # shellcheck disable=SC2086
  docker rm -f $HK_TARGETS >/dev/null 2>&1 || true
}

hk_wait_login() {
  for _ in $(seq 1 60); do
    if curl -sf -o /dev/null "$HK_BASE/login"; then return 0; fi
    sleep 0.5
  done
  hk_fail "l'agent ne répond pas sur $HK_BASE/login"
}

# hk_login JETON FICHIER_COOKIES
hk_login() {
  curl -sf -c "$2" -H "Origin: $HK_BASE" -H 'Content-Type: application/json' \
    -d "{\"token\":\"$1\"}" -o /dev/null "$HK_BASE/api/login" || hk_fail "connexion refusée"
}

# hk_check_meta FICHIER_COOKIES [VERSION]
hk_check_meta() {
  meta=$(curl -sf -b "$1" "$HK_BASE/api/meta") || hk_fail "/api/meta"
  echo "meta : $meta"
  if [ -n "${2:-}" ]; then
    case $meta in *"\"version\":\"$2\""*) ;; *) hk_fail "version attendue $2" ;; esac
  fi
}

# hk_check_targets FICHIER_COOKIES : /etc de chaque cible actif listé, un
# fichier lu ; le conteneur arrêté répond 202 (indexation) puis 200.
hk_check_targets() {
  for c in hk-check-normal hk-check-priv hk-check-unconf hk-check-uid; do
    out=$(curl -s -b "$1" -w '\n%{http_code}' "$HK_BASE/api/containers/$c/fs?path=/etc")
    code=${out##*$'\n'}
    case "$code $out" in 200*'"name":"hostname"'*) ;; *) hk_fail "$c : $out" ;; esac
    code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" "$HK_BASE/api/containers/$c/fs/file?path=/etc/os-release")
    [ "$code" = 200 ] || hk_fail "$c : lecture de /etc/os-release → $code"
    echo "ok $c"
  done
  code=
  for _ in $(seq 1 60); do
    code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" "$HK_BASE/api/containers/hk-check-stopped/fs?path=/etc")
    [ "$code" = 200 ] && break
    sleep 0.5
  done
  [ "$code" = 200 ] || hk_fail "hk-check-stopped : $code"
  echo "ok hk-check-stopped"
}

# hk_check_writes FICHIER_COOKIES : édition (PUT fs/file, If-Match = ETag de
# l'aperçu) puis envoi (POST fs/upload, multipart, If-None-Match: *) dans le
# conteneur actif hk-check-normal ; chaque écriture est relue par docker exec.
hk_check_writes() {
  url="$HK_BASE/api/containers/hk-check-normal"
  etag=$(curl -s -D - -o /dev/null -b "$1" "$url/fs/file?path=/root/hk-edit.txt" | tr -d '\r' | sed -n 's/^[Ee][Tt][Aa][Gg]: //p')
  [ -n "$etag" ] || hk_fail "aperçu de /root/hk-edit.txt sans ETag"
  code=$(printf 'after\n' | curl -s -o /dev/null -w '%{http_code}' -b "$1" -X PUT -H "Origin: $HK_BASE" \
    -H "If-Match: $etag" -H 'Content-Type: application/octet-stream' --data-binary @- "$url/fs/file?path=/root/hk-edit.txt")
  [ "$code" = 200 ] || hk_fail "édition (PUT fs/file) : $code"
  [ "$(docker exec hk-check-normal cat /root/hk-edit.txt)" = after ] || hk_fail "édition absente du conteneur"
  echo "ok édition (PUT fs/file)"
  up=$(mktemp)
  printf 'uploaded\n' > "$up"
  code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" -H "Origin: $HK_BASE" -H 'If-None-Match: *' \
    -F "file=@$up;filename=hk-upload.txt" "$url/fs/upload?dir=/root")
  rm -f "$up"
  [ "$code" = 201 ] || hk_fail "envoi (POST fs/upload) : $code"
  [ "$(docker exec hk-check-normal cat /root/hk-upload.txt)" = uploaded ] || hk_fail "envoi absent du conteneur"
  echo "ok envoi (POST fs/upload)"
}
```

`scripts/check-systemd.sh` :

```bash
#!/usr/bin/env bash
# Vérifie l'unité systemd de la release (spec §8, alternative : binaire lancé
# en root hors conteneur) avec le binaire d'une archive, en suivant les
# commandes de docs/en/install.md : install, unité, reset-token avant le
# premier démarrage (le jeton n'entre jamais dans le journal), enable --now.
# Puis l'explorateur sur les cinq cibles de scripts/release-lib.sh, une
# édition et un envoi (sous le durcissement de l'unité, et SELinux sur Fedora),
# la rotation du jeton, et la désinstallation complète. Refuse si Hublot-Kit est
# déjà installé sur la machine.
#
# Usage (root, sur hk-ubuntu ou hk-fedora) :
#   sudo scripts/check-systemd.sh /tmp/hublot-kit_0.1.0_linux_arm64.tar.gz [VERSION_ATTENDUE]
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
archive=${1:?usage : scripts/check-systemd.sh ARCHIVE.tar.gz [VERSION_ATTENDUE]}
want=${2:-}
[ "$(id -u)" = 0 ] || hk_fail "à lancer en root"
for p in /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service /var/lib/hublot-kit; do
  [ ! -e "$p" ] || hk_fail "$p existe déjà : Hublot-Kit est installé, rien n'est touché"
done
work=$(mktemp -d)
jar=$(mktemp)
cleanup() {
  systemctl disable --now hublot-kit >/dev/null 2>&1 || true
  rm -f /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service
  rm -rf /var/lib/hublot-kit "$work" "$jar"
  systemctl daemon-reload
  hk_remove_targets
}
trap cleanup EXIT

tar -xzf "$archive" -C "$work"
# Les commandes de docs/en/install.md, section « Binary and systemd ».
install -m 0755 "$work/hublot" /usr/local/bin/hublot
install -m 0644 "$work/hublot-kit.service" /etc/systemd/system/hublot-kit.service
# Seuls les avertissements sur notre unité comptent (verify parle aussi de ses dépendances).
out=$(systemd-analyze verify /etc/systemd/system/hublot-kit.service 2>&1) || hk_fail "systemd-analyze : $out"
case $out in *hublot-kit*) hk_fail "systemd-analyze : $out" ;; esac
tok=$(/usr/local/bin/hublot reset-token | sed -n 's/^New access token (shown only once): //p')
[ -n "$tok" ] || hk_fail "reset-token n'a pas donné de jeton"
since=$(date '+%Y-%m-%d %H:%M:%S')
systemctl daemon-reload
systemctl enable --now hublot-kit

hk_start_targets
hk_wait_login
[ "$(stat -c %a /var/lib/hublot-kit)" = 700 ] || hk_fail "/var/lib/hublot-kit n'est pas en 0700"
hk_login "$tok" "$jar"
hk_check_meta "$jar" "$want"
hk_check_targets "$jar"
hk_check_writes "$jar"
# Journal capturé d'abord : sous pipefail, `journalctl | grep -q` peut finir en
# SIGPIPE (141) et masquer une fuite.
journal=$(journalctl -u hublot-kit --since "$since" --no-pager)
case $journal in *"$tok"*) hk_fail "le jeton est dans le journal" ;; esac
case $journal in *WARNING*) hk_fail "avertissement au démarrage (127.0.0.1 attendu, cgroup de l'hôte)" ;; esac
for prop in NoNewPrivileges=yes ProtectSystem=strict PrivateTmp=yes User= ; do
  systemctl show hublot-kit -p "${prop%%=*}" | grep -qx "$prop" || hk_fail "propriété $prop : $(systemctl show hublot-kit -p "${prop%%=*}")"
done
if command -v getenforce >/dev/null && [ "$(getenforce)" = Enforcing ]; then
  ctx=$(ps -o label= -p "$(systemctl show hublot-kit -p MainPID --value)")
  echo "SELinux : $ctx"
  case $ctx in *unconfined_service_t*) ;; *) hk_fail "contexte SELinux inattendu : $ctx" ;; esac
fi

# Rotation : nouveau jeton, redémarrage, l'ancien est refusé.
new=$(/usr/local/bin/hublot reset-token | sed -n 's/^New access token (shown only once): //p')
systemctl restart hublot-kit
hk_wait_login
code=$(curl -s -o /dev/null -w '%{http_code}' -H "Origin: $HK_BASE" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$tok\"}" "$HK_BASE/api/login")
[ "$code" = 401 ] || hk_fail "ancien jeton après rotation : $code"
hk_login "$new" "$jar"
echo "SYSTEMD OK : $archive"
```

Run: `chmod +x scripts/check-systemd.sh && bash -n scripts/release-lib.sh && bash -n scripts/check-systemd.sh && echo ok`
Expected: `ok`.

- [x] **Step 2: Le voir échouer** — une archive sans unité, comme avant la tâche :

```bash
export PATH=/opt/homebrew/bin:$PATH
make web >/dev/null
tmp=$(mktemp -d)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=0.0.0-local" -o "$tmp/hublot" ./cmd/hublot
cp LICENSE README.md "$tmp/"
tar -czf "$tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz" -C "$tmp" hublot LICENSE README.md
limactl copy "$tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz" hk-ubuntu:/tmp/
limactl shell --workdir "$PWD" hk-ubuntu -- sudo scripts/check-systemd.sh /tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz 0.0.0-local 2>&1 | tail -n 5
```

Expected: échec, `install: cannot stat '/tmp/tmp.…/hublot-kit.service': No such file or directory` ; puis `limactl shell hk-ubuntu -- ls /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service` → `No such file or directory` pour les deux (le nettoyage a tourné).

- [x] **Step 3: Écrire l'unité** — `packaging/systemd/hublot-kit.service` :

```ini
# Hublot-Kit agent, run as root outside a container (spec §8, alternative).
# Install: see docs/en/install.md ("Binary and systemd").
# Settings: `systemctl edit hublot-kit`, then e.g.
#   [Service]
#   Environment=HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com
# Every flag has a HUBLOT_* equivalent (`hublot --help`).
#
# The agent talks to the Docker socket and reads container files through
# /proc/<pid>/root: it is root on the host in practice. The sandboxing below
# only limits mistakes; directives that would hide /proc of other processes
# (ProtectProc, PrivateUsers…) or drop capabilities would break the explorer.

[Unit]
Description=Hublot-Kit, Docker container explorer
Documentation=https://github.com/levy-cmd/Hublot-Kit
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hublot
Restart=on-failure
RestartSec=5s
Environment=HUBLOT_LISTEN=127.0.0.1:7878
Environment=HUBLOT_DATA_DIR=/var/lib/hublot-kit
StateDirectory=hublot-kit
StateDirectoryMode=0700
NoNewPrivileges=yes
ProtectSystem=strict
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
```

- [x] **Step 4: La voir passer sur les deux VM** (même shell qu'à l'étape 2 : `$tmp` contient le binaire)

```bash
cp packaging/systemd/hublot-kit.service "$tmp/"
tar -czf "$tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz" -C "$tmp" hublot LICENSE README.md hublot-kit.service
limactl start hk-fedora >/dev/null 2>&1 || true
for vm in hk-ubuntu hk-fedora; do
  limactl copy "$tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz" "$vm:/tmp/"
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-systemd.sh /tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz 0.0.0-local 2>&1 | tail -n 11
  limactl shell "$vm" -- rm -f /tmp/hublot-kit_0.0.0-local_linux_arm64.tar.gz
done
limactl shell hk-ubuntu -- sudo journalctl -k --since -15min | grep -c 'apparmor="DENIED"'
limactl shell hk-fedora -- sudo ausearch -m avc -ts recent 2>&1 | tail -n 3
rm -rf "$tmp"
```

Expected: sur chaque VM, `meta : {"readOnly":false,"version":"0.0.0-local"}`, `ok hk-check-normal`, `ok hk-check-priv`, `ok hk-check-unconf`, `ok hk-check-uid`, `ok hk-check-stopped`, `ok édition (PUT fs/file)`, `ok envoi (POST fs/upload)`, puis `SYSTEMD OK : …` ; sur `hk-fedora`, en plus, `SELinux : system_u:system_r:unconfined_service_t:s0` ; `0` refus AppArmor ; `<no matches>` pour les AVC.

Si un contrôle échoue à cause d'une directive de durcissement (lecture d'une cible, socket, dossier de données) : retirer **cette directive seulement**, relancer, et noter le constat (directive, VM, message) sous « Constats » à la fin du plan. Ne jamais ajouter de capacité ni de `--privileged`-like.

- [x] **Step 5: Commit**

```bash
git add scripts/release-lib.sh scripts/check-systemd.sh packaging/systemd/hublot-kit.service
git commit -F - <<'EOF'
release : unité systemd, vérifiée sur hk-ubuntu et hk-fedora

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 6: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex. Points : chaque directive de l'unité contre l'explorateur (`/proc/<pid>/root`, `CAP_SYS_PTRACE`, socket sous `ProtectSystem=strict`) ; le jeton jamais dans le journal ; le nettoyage en cas d'échec (`trap`) ; le refus d'une installation existante ; `systemd-analyze verify` ; bash portable. Corrections en commits séparés ; la tâche 4 attend les deux avis.

---

### Task 4: Image de release, vérifiée par la commande exacte de la doc

**Où tester :** Docker Desktop, puis `hk-ubuntu` et `hk-fedora` (image arm64 construite dans la VM, quelques Mo).

**Files:**
- Create: `scripts/check-image.sh`, `Dockerfile`

**Interfaces:**
- Consumes: `release-lib.sh` (tâche 3) ; la commande de la spec §8, lue par `hk_spec_command`.
- Produces: `scripts/check-image.sh IMAGE [VERSION]`, utilisé par le job CI `release-snapshot` (tâche 6) et les tâches 8, 9, 10. `Dockerfile` : contexte `linux/<arch>/hublot` (celui que fournit `dockers_v2` de GoReleaser, tâche 6), image `FROM scratch`.

> **À comprendre :** `check-image.sh` ne contient pas la commande : il la lit dans la spec, qui fait foi, et n'y remplace que `ghcr.io/levy-cmd/hublot-kit:0.1` ; la tâche 5 force la doc à reprendre la même commande. Dans l'image, l'agent écoute sur `0.0.0.0:7878` (publié sur `127.0.0.1`) et affiche l'avertissement « sans TLS » : c'est prévu par §7. Seul l'avertissement de l'espace de noms cgroup fait échouer le script. Le script écrit aussi (édition et envoi, `hk_check_writes` de la tâche 3) : l'image `scratch` tourne en root, et l'écriture passe par Docker (`PUT /archive`) ; vérifié à la rédaction sur Docker Desktop, avec un binaire aux messages anglais de la tâche 2.

- [x] **Step 1: Écrire le script de vérification** — `scripts/check-image.sh` :

```bash
#!/usr/bin/env bash
# Vérifie une image de release avec la commande exacte de la spec §8, celle de
# la doc (plan Lancement). Seule la référence d'image change : IMAGE remplace
# ghcr.io/levy-cmd/hublot-kit:0.1. Puis : jeton lu dans `docker logs`, connexion,
# /api/meta, l'explorateur sur les cinq cibles de scripts/release-lib.sh, une
# édition et un envoi, et la recréation du conteneur sur le même volume.
# Nettoie tout à la fin ; refuse de toucher un conteneur hublot-kit ou un
# volume hublot-data déjà présents.
#
# Usage : scripts/check-image.sh IMAGE [VERSION_ATTENDUE]   (en root sur les VM)
#   scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 0.1.0-rc.1
#   scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1           (commande inchangée)
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
image=${1:?usage : scripts/check-image.sh IMAGE [VERSION_ATTENDUE]}
want=${2:-}

cmd=$(hk_spec_command)
[ -n "$cmd" ] || hk_fail "commande docker run introuvable dans la spec §8"
cmd=${cmd//ghcr.io\/levy-cmd\/hublot-kit:0.1/$image}

if docker container inspect hublot-kit >/dev/null 2>&1; then hk_fail "un conteneur hublot-kit existe déjà"; fi
if docker volume inspect hublot-data >/dev/null 2>&1; then hk_fail "un volume hublot-data existe déjà"; fi
jar=$(mktemp)
cleanup() {
  docker rm -f hublot-kit >/dev/null 2>&1 || true
  docker volume rm -f hublot-data >/dev/null 2>&1 || true
  hk_remove_targets
  rm -f "$jar"
}
trap cleanup EXIT

hk_start_targets
printf '+ %s\n' "$cmd"
sh -c "$cmd" >/dev/null
hk_wait_login
logs=$(docker logs hublot-kit 2>&1)
tok=$(printf '%s\n' "$logs" | sed -n 's/^Access token (shown only once): //p')
[ -n "$tok" ] || hk_fail "jeton absent de docker logs : $logs"
# L'avertissement « without TLS » est attendu dans l'image (0.0.0.0 publié sur 127.0.0.1, spec §7) ;
# celui de l'espace de noms cgroup ne doit jamais paraître (spec §8).
case $logs in *"private cgroup namespace"*) hk_fail "avertissement cgroup au démarrage : $logs" ;; esac
hk_login "$tok" "$jar"
hk_check_meta "$jar" "$want"
hk_check_targets "$jar"
hk_check_writes "$jar"

# Mise à jour (docs, « Upgrade ») : conteneur recréé sur le même volume, même
# jeton, et aucun nouveau jeton affiché.
docker rm -f hublot-kit >/dev/null
sh -c "$cmd" >/dev/null
hk_wait_login
case $(docker logs hublot-kit 2>&1) in *"Access token"*) hk_fail "nouveau jeton après recréation : le volume n'a pas gardé l'empreinte" ;; esac
hk_login "$tok" "$jar"
echo "ok mise à jour (même volume, même jeton)"
echo "IMAGE OK : $image"
```

Run: `chmod +x scripts/check-image.sh && bash -n scripts/check-image.sh && echo ok`
Expected: `ok`.

- [x] **Step 2: Le voir échouer** — pas encore de `Dockerfile`, donc pas d'image :

```bash
export DOCKER_HOST=unix://$HOME/.docker/run/docker.sock PATH=/opt/homebrew/bin:$PATH
make web >/dev/null
ctx=$(mktemp -d) && mkdir -p "$ctx/linux/arm64"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=0.0.0-local" -o "$ctx/linux/arm64/hublot" ./cmd/hublot
docker build -q -f Dockerfile -t hk-local-test:arm64 "$ctx" 2>&1 | tail -n 2
scripts/check-image.sh hk-local-test:arm64 0.0.0-local 2>&1 | tail -n 3
```

Expected: `docker build` échoue (Dockerfile introuvable), puis `check-image.sh` échoue au `docker run` (`Unable to find image 'hk-local-test:arm64' locally` … `pull access denied`) ; `docker ps -a --format '{{.Names}}' | grep -c 'hk-check\|hublot-kit'` → `0` (nettoyé).

- [x] **Step 3: Écrire le Dockerfile** — `Dockerfile` :

```dockerfile
# Release image, built by GoReleaser (dockers_v2, .goreleaser.yaml): the build
# context holds linux/amd64/hublot and linux/arm64/hublot, already compiled
# (static, CGO_ENABLED=0). It does not build on its own: see scripts/check-image.sh
# and the release-snapshot CI job.
#
# FROM scratch: nothing but the binary. The agent runs as root on purpose
# (spec §7, §8): it reads /proc/<pid>/root of the containers.
FROM scratch
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/hublot /hublot
ENV HUBLOT_LISTEN=0.0.0.0:7878 HUBLOT_DATA_DIR=/data
EXPOSE 7878
ENTRYPOINT ["/hublot"]
```

- [x] **Step 4: Le voir passer sur Docker Desktop, puis sur les deux VM** (même shell qu'à l'étape 2 : `$ctx` contient le binaire)

```bash
docker build -q -f Dockerfile -t hk-local-test:arm64 "$ctx" >/dev/null
scripts/check-image.sh hk-local-test:arm64 0.0.0-local 2>&1 | tail -n 11
docker rmi -f hk-local-test:arm64 >/dev/null
limactl start hk-fedora >/dev/null 2>&1 || true
for vm in hk-ubuntu hk-fedora; do
  limactl copy "$ctx/linux/arm64/hublot" "$vm:/tmp/hublot"
  limactl shell --workdir "$PWD" "$vm" -- sh -c 'mkdir -p /tmp/hk-ctx/linux/arm64 && mv /tmp/hublot /tmp/hk-ctx/linux/arm64/ && sudo docker build -q -f Dockerfile -t hk-local-test:arm64 /tmp/hk-ctx >/dev/null'
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-image.sh hk-local-test:arm64 0.0.0-local 2>&1 | tail -n 11
  limactl shell "$vm" -- sh -c 'sudo docker rmi -f hk-local-test:arm64 >/dev/null; rm -rf /tmp/hk-ctx'
done
limactl shell hk-ubuntu -- sudo journalctl -k --since -15min | grep -c 'apparmor="DENIED"'
limactl shell hk-fedora -- sudo ausearch -m avc -ts recent 2>&1 | tail -n 3
rm -rf "$ctx"
```

Expected, partout : la commande affichée (`+ docker run -d --name hublot-kit \` … `hk-local-test:arm64`), `meta : {"readOnly":false,"version":"0.0.0-local"}`, les cinq `ok hk-check-…`, `ok édition (PUT fs/file)`, `ok envoi (POST fs/upload)`, `ok mise à jour (même volume, même jeton)`, `IMAGE OK : hk-local-test:arm64` ; `0` refus AppArmor ; `<no matches>` pour les AVC. Mesuré à la rédaction sur Docker Desktop : vert, écritures comprises.

- [x] **Step 5: Commit**

```bash
git add scripts/check-image.sh Dockerfile
git commit -F - <<'EOF'
release : image FROM scratch, vérifiée par la commande exacte de la spec §8

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 6: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex. Points : seule la référence d'image change dans la commande ; aucun conteneur ni volume de l'utilisateur écrasé ; nettoyage complet ; `FROM scratch` en root voulu (§7, §8) ; l'image ne contient que le binaire. Corrections en commits séparés ; la tâche 5 attend les deux avis.

---

### Task 5: Documentation publique, en anglais et en français

**Où tester :** macOS (`scripts/check-docs.sh`, bash 3.2), puis le job CI `docs`.

**Files:**
- Create: `scripts/check-docs.sh`, `docs/images/containers.png`, `docs/images/explorer.png`, `README.fr.md`, `docs/en/install.md`, `docs/en/security.md`, `docs/en/limitations.md`, `docs/fr/installation.md`, `docs/fr/securite.md`, `docs/fr/limites.md`
- Modify: `README.md` (réécrit en anglais ; les « Notes d'exploitation » passent dans `docs/*/install*.md`, section « Data directory »), `.github/workflows/ci.yml` (job `docs`)

**Interfaces:**
- Consumes: les lignes anglaises de l'agent (tâche 2 : `Access token (shown only once): …`) ; `hk_spec_command` (tâche 3) ; les commandes de `check-systemd.sh` (tâche 3) et le comportement de l'image (tâche 4), que la doc décrit ; les noms d'artefacts de la tâche 6 (`hublot-kit_<V>_linux_<ARCH>.tar.gz`, `checksums.txt`, `*.sigstore.json`, `*.sbom.json`) et l'identité du workflow de la tâche 7 (`.github/workflows/release.yml@refs/tags/v<V>`).
- Produces: les ancres que la release cite (tâche 6) : `docs/en/install.md#verify-the-release`, `docs/en/install.md`, `docs/fr/installation.md`, `docs/en/security.md` ; `README.fr.md`, inclus dans les archives (tâche 6).

> **À comprendre :** la doc est un livrable de sécurité : elle dit en tête que l'agent est root sur l'hôte, et reprend honnêtement les limites de §13 (`limitations.md`). `check-docs.sh` garde trois choses que la relecture rate facilement : la commande de §8 copiée à l'octet près, les blocs de code identiques entre FR et EN (les commandes ne se traduisent pas), et les liens relatifs. Les pages FR sont une traduction fidèle des pages EN, mêmes titres dans le même ordre.

- [x] **Step 1: Écrire le script de vérification** — `scripts/check-docs.sh` :

```bash
#!/usr/bin/env bash
# Contrôles de la documentation publique (plan Lancement, spec §8 et §12) :
# - chaque page existe en anglais et en français, avec le même nombre de titres
#   et exactement les mêmes blocs de code (les commandes ne se traduisent pas) ;
# - la commande `docker run` de README.md, README.fr.md et des pages
#   d'installation est celle de la spec §8, à l'octet près ;
# - les deux README disent l'accès root dans leurs 12 premières lignes ;
# - tout lien relatif mène à un fichier qui existe.
# Usage : scripts/check-docs.sh   (bash 3.2 compris)
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
errors=0
err() { echo "ERREUR : $*" >&2; errors=$((errors + 1)); }
block() { awk '/^docker run -d --name hublot-kit/{on=1} on{print} on&&/ghcr\.io\/levy-cmd\/hublot-kit:0\.1$/{exit}' "$1"; }
code_blocks() { awk '/^ *```/{inside=!inside; next} inside' "$1"; }

want=$(hk_spec_command)
[ -n "$want" ] || err "commande docker run introuvable dans la spec §8"

pairs="README.md:README.fr.md docs/en/install.md:docs/fr/installation.md docs/en/security.md:docs/fr/securite.md docs/en/limitations.md:docs/fr/limites.md"
for p in $pairs; do
  en=${p%%:*} fr=${p##*:}
  ok=1
  for f in "$en" "$fr"; do [ -f "$f" ] || { err "$f absent"; ok=0; }; done
  [ $ok = 1 ] || continue
  [ "$(grep -c '^#' "$en")" = "$(grep -c '^#' "$fr")" ] || err "$en et $fr : nombre de titres différent"
  [ "$(code_blocks "$en")" = "$(code_blocks "$fr")" ] || err "$en et $fr : blocs de code différents"
done

for f in README.md README.fr.md docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  [ "$(block "$f")" = "$want" ] || err "$f : la commande docker run diffère de la spec §8"
done
for f in README.md README.fr.md; do
  [ -f "$f" ] || continue
  head -n 12 "$f" | grep -q '^> \*\*.*root' || err "$f : l'accès root n'est pas dit en tête (12 premières lignes)"
done

for f in README.md README.fr.md docs/en/*.md docs/fr/*.md; do
  [ -f "$f" ] || continue
  dir=$(dirname "$f")
  for link in $(grep -o '](\([^)#]*\)' "$f" | sed 's/^](//' | grep -v '^[a-z][a-z]*:' | grep -v '^$' || true); do
    [ -e "$dir/$link" ] || err "$f : lien mort $link"
  done
done

[ $errors = 0 ] || { echo "$errors erreur(s)" >&2; exit 1; }
echo "DOCS OK"
```

- [x] **Step 2: Le voir échouer**

Run: `chmod +x scripts/check-docs.sh && scripts/check-docs.sh 2>&1 | tail -n 10`
Expected: `ERREUR : README.fr.md absent`, les six pages de `docs/en` et `docs/fr` absentes, `ERREUR : README.md : la commande docker run diffère de la spec §8`, puis `8 erreur(s)`.

- [x] **Step 3: Écrire la documentation** — les huit fichiers ci-dessous, tels quels.

`README.md` :

````markdown
# Hublot-Kit

> **Security: installing Hublot-Kit gives it root on the host.**
> It talks to the Docker socket and reads the files of your containers. Keep it on `127.0.0.1` and reach it
> through an SSH tunnel or an HTTPS reverse proxy. Read [the security model](docs/en/security.md) first.

[Lire en français](README.fr.md)

Explore the inside of the Docker containers of a server from a browser, without a terminal. Pick a container and see
its files, what changed since it was created, its logs and its configuration.

![Container list](docs/images/containers.png)
![File explorer with the changes since creation](docs/images/explorer.png)

## What it does (v0.1)

- **Containers**: name, image, status, ports, uptime; filter and search.
- **Files**: the whole tree of a running or a stopped container, text and image preview, size, owner, permissions.
- **Download** a file, or a folder as a tar archive.
- **Edit** a text file or **upload** a file, after a confirmation that names the container and the path.
- **Changes**: what was added, modified or deleted since the container was created.
- **Logs**: live stream, search, pause and resume, download.
- **X-ray**: image, state, command, environment (secrets masked, revealed one at a time and logged), ports, networks,
  mounts.
- **Read-only mode** (`--read-only`): no write at all.

One server, one user, an interface in English and French, a single static binary with the interface embedded, no
database, no telemetry. Processes and resources, start/stop and a web terminal are planned for v0.2.

## Quick start

Requirements: Linux amd64 or arm64, kernel 5.6 or later (5.8 to edit files of a running container), Docker Engine
with API 1.44 or later, overlay2 or containerd storage. The same command works on AppArmor (Ubuntu, Debian) and
SELinux (Fedora, RHEL) hosts; [why each option is needed](docs/en/install.md#why-each-option).

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

The access token is printed once, on the first start, on the line that starts with `Access token`.

```sh
docker logs hublot-kit
```

From your computer, open a tunnel, then browse to <http://127.0.0.1:7878> and paste the token.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

Prefer a plain binary run by systemd? See [Binary and systemd](docs/en/install.md#binary-and-systemd).

## Documentation

- [Installation](docs/en/install.md): Docker, binary and systemd, reverse proxy, token, upgrade, uninstall, verifying
  the signatures and SBOMs of a release.
- [Security model](docs/en/security.md): what the agent can do, what protects it, how to expose it.
- [Known limitations](docs/en/limitations.md): what v0.1 does not do, or does not detect.

## Development

Requirements: Go 1.27 or later, Node 24 (22.17 or later), Docker. The Go code that touches `/proc` only runs on Linux:
`make test-linux` runs it as root in a `golang` container.

```sh
make test
make build
make test-linux
```

On macOS (Docker Desktop), `/proc/<pid>/root` only exists inside Docker's virtual machine, so the agent runs in a
container: `make web && scripts/dev-agent.sh`. The interface in development mode proxies to the agent on
`127.0.0.1:7878`: `cd web && npm run dev`. The technical spec is in
[docs/superpowers/specs](docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md) (in French).

## License

[MIT](LICENSE)
````

`README.fr.md` :

````markdown
# Hublot-Kit

> **Sécurité : installer Hublot-Kit revient à lui donner les droits root sur l'hôte.**
> Il parle au socket Docker et lit les fichiers de vos conteneurs. Gardez-le sur `127.0.0.1` et accédez-y par un
> tunnel SSH ou un reverse proxy HTTPS. Lisez d'abord [le modèle de sécurité](docs/fr/securite.md).

[Read in English](README.md)

Explorer l'intérieur des conteneurs Docker d'un serveur depuis un navigateur, sans terminal. Choisissez un conteneur
et voyez ses fichiers, ce qui a changé depuis sa création, ses logs et sa configuration.

![Liste des conteneurs](docs/images/containers.png)
![Explorateur de fichiers avec les changements depuis la création](docs/images/explorer.png)

## Ce qu'il fait (v0.1)

- **Conteneurs** : nom, image, statut, ports, durée de fonctionnement ; filtre et recherche.
- **Fichiers** : toute l'arborescence d'un conteneur actif ou arrêté, aperçu texte et image, taille, propriétaire,
  droits.
- **Téléchargement** d'un fichier, ou d'un dossier en archive tar.
- **Édition** d'un fichier texte ou **envoi** d'un fichier, après une confirmation qui rappelle le conteneur et le
  chemin.
- **Changements** : ce qui a été ajouté, modifié ou supprimé depuis la création du conteneur.
- **Logs** : flux en direct, recherche, pause et reprise, téléchargement.
- **Radiographie** : image, état, commande, environnement (secrets masqués, révélés un par un et journalisés), ports,
  réseaux, montages.
- **Lecture seule** (`--read-only`) : aucune écriture.

Un serveur, un utilisateur, une interface en français et en anglais, un seul binaire statique avec l'interface
embarquée, pas de base de données, aucune télémétrie. Les process et ressources, le démarrage et l'arrêt, et un
terminal web sont prévus pour la v0.2.

## Démarrage rapide

Prérequis : Linux amd64 ou arm64, noyau 5.6 ou plus récent (5.8 pour éditer les fichiers d'un conteneur actif),
Docker Engine avec l'API 1.44 ou plus récente, stockage overlay2 ou containerd. La même commande sert sur les hôtes
AppArmor (Ubuntu, Debian) et SELinux (Fedora, RHEL) ; [à quoi sert chaque option](docs/fr/installation.md#à-quoi-sert-chaque-option).

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

Le jeton d'accès s'affiche une seule fois, au premier lancement, sur la ligne qui commence par `Access token` (les
messages de l'agent sont en anglais).

```sh
docker logs hublot-kit
```

Depuis votre ordinateur, ouvrez un tunnel, puis allez sur <http://127.0.0.1:7878> et collez le jeton.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

Vous préférez un binaire lancé par systemd ? Voir [Binaire et systemd](docs/fr/installation.md#binaire-et-systemd).

## Documentation

- [Installation](docs/fr/installation.md) : Docker, binaire et systemd, reverse proxy, jeton, mise à jour,
  désinstallation, vérification des signatures et des SBOM d'une release.
- [Modèle de sécurité](docs/fr/securite.md) : ce que l'agent peut faire, ce qui le protège, comment l'exposer.
- [Limites connues](docs/fr/limites.md) : ce que la v0.1 ne fait pas, ou ne détecte pas.

## Développement

Prérequis : Go 1.27 ou plus récent, Node 24 (22.17 ou plus récent), Docker. Le code Go qui touche `/proc` ne tourne
que sous Linux : `make test-linux` le lance en root dans un conteneur `golang`.

```sh
make test
make build
make test-linux
```

Sur macOS (Docker Desktop), `/proc/<pid>/root` n'existe que dans la machine virtuelle de Docker : l'agent tourne donc
dans un conteneur, `make web && scripts/dev-agent.sh`. L'interface en mode développement passe par un proxy vers
l'agent sur `127.0.0.1:7878` : `cd web && npm run dev`. La spec technique est dans
[docs/superpowers/specs](docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md).

## Licence

[MIT](LICENSE)
````

`docs/en/install.md` :

````markdown
# Installing Hublot-Kit

> **Hublot-Kit is root on the host in practice.** Read [the security model](security.md) before installing, and
> [the known limitations](limitations.md) before relying on it.

## Requirements

- Linux amd64 or arm64.
- Kernel 5.6 or later to read files; 5.8 or later to edit or upload files in a running container.
- Docker Engine with API 1.44 or later (`docker version` shows it), overlay2 or containerd storage.
- Not supported in v0.1: rootless Docker, `userns-remap`, Podman, Kubernetes.

## Docker (recommended)

The same command works on AppArmor hosts (Ubuntu, Debian) and SELinux hosts (Fedora, RHEL). It was measured on
Ubuntu with AppArmor and on Fedora with SELinux enforcing.

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

`0.1` follows the 0.1.x patch releases; `0.1.0` never moves. There is no `latest` tag.

### Why each option

| Option | Why |
|---|---|
| `--pid=host` | to see `/proc/<pid>` of the containers, which is how the agent reads their files. |
| `--cgroupns=host` | required: the agent proves that a process belongs to a container through its cgroup path. In a private cgroup namespace it cannot, and every file request answers `container_changed`; the agent prints a warning at startup. |
| `--cap-add SYS_PTRACE` | to read the files of a container that runs as another user, or that holds capabilities the agent lacks. |
| `--security-opt apparmor=unconfined` | AppArmor hosts: without it, containers started with `--privileged` or `apparmor=unconfined` cannot be explored (the error names this option). No effect on other hosts. |
| `--security-opt label=disable` | SELinux hosts: lets the agent read other containers and use the Docker socket. Docker already applies it to a container started with `--pid=host`, but that is not a documented contract. Do not use any other `label=` option: it would cut the agent off the socket. No effect on other hosts. |
| `-v /var/run/docker.sock:…` | the agent's only way to Docker. Mounting it `:ro` protects nothing. |
| `-v hublot-data:/data` | the hash of the access token and the audit log. |
| `-p 127.0.0.1:7878:7878` | reachable from the server only; use an SSH tunnel or a reverse proxy. |

`--privileged` is not needed. The agent is already root on the host in practice: lifting its AppArmor confinement,
or running it as `spc_t` under SELinux, changes which containers it can read, not what it can do.

### First login

The access token is printed once, on the first start, on the line that starts with `Access token`.

```sh
docker logs hublot-kit
```

From your computer, open a tunnel, browse to <http://127.0.0.1:7878> and paste the token.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

### Options

Every flag has an environment variable. With Docker, add flags after the image name, or variables with `-e` before
it.

| Flag | Variable | Default |
|---|---|---|
| `--listen` | `HUBLOT_LISTEN` | `127.0.0.1:7878` (`0.0.0.0:7878` in the image) |
| `--data-dir` | `HUBLOT_DATA_DIR` | `/var/lib/hublot-kit` (`/data` in the image) |
| `--read-only` | `HUBLOT_READ_ONLY` | `false`: with `true`, no write at all, and the write buttons disappear |
| `--tls-cert`, `--tls-key` | `HUBLOT_TLS_CERT`, `HUBLOT_TLS_KEY` | empty: serve HTTPS directly with a certificate you provide |
| `--public-origin` | `HUBLOT_PUBLIC_ORIGIN` | empty: the public URL behind a reverse proxy |

`hublot --help` lists them; `hublot --version` prints the version.

## Behind an HTTPS reverse proxy

Keep the port on `127.0.0.1`, point the proxy at `http://127.0.0.1:7878`, and give the agent its public origin, for
example `-e HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com` before the image name. Without it, a proxy that changes
the scheme makes every request other than GET fail with `forbidden_origin`, login included, and the session cookie
is not marked `Secure`.

The logs tab is a Server-Sent Events stream: the proxy must not buffer it (the agent sends `X-Accel-Buffering: no`,
which nginx honours) and must allow long reads (the stream sends a ping every 15 seconds). The agent never trusts
`X-Forwarded-*` headers.

Without a proxy, `--tls-cert` and `--tls-key` serve HTTPS directly. v0.1 does not obtain certificates by itself.

## Binary and systemd

The alternative to the container: the static binary, run as root by systemd. Check the archive first
([Verify the release](#verify-the-release)). Use `amd64` or `arm64`.

```sh
V=0.1.0
ARCH=amd64
curl -fLO "https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V/hublot-kit_${V}_linux_${ARCH}.tar.gz"
tar -xzf "hublot-kit_${V}_linux_${ARCH}.tar.gz"
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service
sudo /usr/local/bin/hublot reset-token
sudo systemctl daemon-reload
sudo systemctl enable --now hublot-kit
```

`hublot reset-token` before the first start prints the token on your terminal. Started first, the agent would print
it in the system journal, which members of the `adm` or `systemd-journal` groups can read.

On SELinux hosts, `install` gives the binary the right label; a binary moved with `mv` from a home directory keeps a
label that systemd refuses to run (`sudo restorecon /usr/local/bin/hublot` fixes it).

The unit listens on `127.0.0.1:7878`, keeps its data in `/var/lib/hublot-kit` (mode 0700) and runs with
`NoNewPrivileges`, `ProtectSystem=strict` and `PrivateTmp`. It does not hide `/proc` or drop capabilities: the
explorer needs them.

### Settings

Change options with a drop-in, never by editing the unit:

```sh
sudo systemctl edit hublot-kit
```

```ini
[Service]
Environment=HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com
Environment=HUBLOT_READ_ONLY=true
```

```sh
sudo systemctl restart hublot-kit
```

## Access token

A new token invalidates the old one once the agent restarts; until then, a running agent still accepts the old one.
After 5 failed logins in a minute from one address, logins from that address are refused for the rest of the minute.

With Docker:

```sh
docker exec hublot-kit /hublot reset-token
docker restart hublot-kit
```

With systemd:

```sh
sudo /usr/local/bin/hublot reset-token
sudo systemctl restart hublot-kit
```

## Data directory

`--data-dir` must be dedicated to Hublot-Kit. It holds `token.sha256` (the hash of the token, never the token) and
`audit.log`. Missing, it is created with mode 0700. Existing, its mode is kept, but the agent refuses to start if it
is a symbolic link, if it belongs to another user, or if its group or others can write to it (`/tmp`, for example).
Its files are never opened through a symbolic link. The audit log is not rotated in v0.1.

## Upgrade

With Docker, pull the image, remove the container and run the command of [Docker](#docker-recommended) again; the
`hublot-data` volume keeps the token.

```sh
docker pull ghcr.io/levy-cmd/hublot-kit:0.1
docker rm -f hublot-kit
```

With systemd, download and verify the new archive, then replace the binary and restart:

```sh
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo systemctl restart hublot-kit
```

## Uninstall

With Docker:

```sh
docker rm -f hublot-kit
docker volume rm hublot-data
```

With systemd:

```sh
sudo systemctl disable --now hublot-kit
sudo rm -rf /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service /etc/systemd/system/hublot-kit.service.d
sudo systemctl daemon-reload
sudo rm -rf /var/lib/hublot-kit
```

## Verify the release

Releases are built and signed by GitHub Actions, with keyless [cosign](https://docs.sigstore.dev/) signatures tied to
the release workflow of this repository. You need cosign 3.0 or later.

What a valid signature proves: the file or the image was produced by that workflow, run on that tag, and has not
changed since. What it does not prove: that the code of the tag was reviewed, or that an SBOM is complete (it lists
what syft, for the archives, and BuildKit, for the image, found in the binary). No SLSA provenance is published in
v0.1.

### Archives

`checksums.txt` lists the SHA-256 of the archives and of their SPDX SBOMs (`.sbom.json`); its signature covers them.
The `.sigstore.json` bundles are not listed: each one carries its own certificate, checked by `cosign verify-blob`.
Each archive also has its own bundle.

```sh
V=0.1.0
ARCH=amd64
BASE="https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V"
curl -fLO "$BASE/hublot-kit_${V}_linux_${ARCH}.tar.gz"
curl -fLO "$BASE/checksums.txt"
curl -fLO "$BASE/checksums.txt.sigstore.json"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

### Image

The image is signed by digest, for linux/amd64 and linux/arm64, and carries an SBOM attestation.

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1
docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:0.1 --format '{{ json .SBOM }}'
```

To run exactly the image you verified, replace `:0.1` in the command of [Docker](#docker-recommended) with the digest
that `cosign verify` prints (`ghcr.io/levy-cmd/hublot-kit@sha256:…`).
````

`docs/fr/installation.md` :

````markdown
# Installer Hublot-Kit

> **Hublot-Kit est, de fait, root sur l'hôte.** Lisez [le modèle de sécurité](securite.md) avant d'installer, et
> [les limites connues](limites.md) avant de vous y fier.

## Prérequis

- Linux amd64 ou arm64.
- Noyau 5.6 ou plus récent pour lire les fichiers ; 5.8 ou plus récent pour éditer ou envoyer des fichiers dans un
  conteneur actif.
- Docker Engine avec l'API 1.44 ou plus récente (`docker version` l'indique), stockage overlay2 ou containerd.
- Non pris en charge en v0.1 : Docker rootless, `userns-remap`, Podman, Kubernetes.

## Docker (recommandé)

La même commande sert sur les hôtes AppArmor (Ubuntu, Debian) et SELinux (Fedora, RHEL). Elle a été mesurée sur
Ubuntu avec AppArmor et sur Fedora avec SELinux en mode `enforcing`.

```sh
docker run -d --name hublot-kit \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined \
  --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v hublot-data:/data \
  -p 127.0.0.1:7878:7878 \
  ghcr.io/levy-cmd/hublot-kit:0.1
```

`0.1` suit les correctifs 0.1.x ; `0.1.0` ne bouge jamais. Il n'y a pas d'étiquette `latest`.

### À quoi sert chaque option

| Option | Raison |
|---|---|
| `--pid=host` | voir `/proc/<pid>` des conteneurs : c'est par là que l'agent lit leurs fichiers. |
| `--cgroupns=host` | obligatoire : l'agent prouve qu'un process appartient à un conteneur par son chemin de cgroup. Dans un espace de noms cgroup privé, il ne le peut pas, et chaque requête de fichier répond `container_changed` ; l'agent affiche un avertissement au démarrage. |
| `--cap-add SYS_PTRACE` | lire les fichiers d'un conteneur qui tourne sous un autre utilisateur, ou qui détient des capacités que l'agent n'a pas. |
| `--security-opt apparmor=unconfined` | hôtes AppArmor : sans elle, les conteneurs lancés en `--privileged` ou `apparmor=unconfined` ne s'explorent pas (l'erreur nomme cette option). Sans effet ailleurs. |
| `--security-opt label=disable` | hôtes SELinux : permet à l'agent de lire les autres conteneurs et d'utiliser le socket Docker. Docker l'applique déjà à un conteneur lancé avec `--pid=host`, mais ce n'est pas un contrat documenté. N'utilisez aucune autre option `label=` : elle couperait l'agent du socket. Sans effet ailleurs. |
| `-v /var/run/docker.sock:…` | le seul accès de l'agent à Docker. Le monter en `:ro` ne protège rien. |
| `-v hublot-data:/data` | l'empreinte du jeton d'accès et le journal d'audit. |
| `-p 127.0.0.1:7878:7878` | joignable depuis le serveur seulement ; passez par un tunnel SSH ou un reverse proxy. |

`--privileged` n'est pas nécessaire. L'agent est déjà root sur l'hôte de fait : lever son confinement AppArmor, ou le
faire tourner en `spc_t` sous SELinux, change les conteneurs qu'il peut lire, pas ce qu'il peut faire.

### Première connexion

Le jeton d'accès s'affiche une seule fois, au premier lancement, sur la ligne qui commence par `Access token` (les
messages de l'agent sont en anglais).

```sh
docker logs hublot-kit
```

Depuis votre ordinateur, ouvrez un tunnel, allez sur <http://127.0.0.1:7878> et collez le jeton.

```sh
ssh -L 7878:127.0.0.1:7878 user@server
```

### Options

Chaque option a sa variable d'environnement. Avec Docker, ajoutez les options après le nom de l'image, ou les
variables avec `-e` avant lui.

| Option | Variable | Défaut |
|---|---|---|
| `--listen` | `HUBLOT_LISTEN` | `127.0.0.1:7878` (`0.0.0.0:7878` dans l'image) |
| `--data-dir` | `HUBLOT_DATA_DIR` | `/var/lib/hublot-kit` (`/data` dans l'image) |
| `--read-only` | `HUBLOT_READ_ONLY` | `false` : avec `true`, aucune écriture, et les boutons d'écriture disparaissent |
| `--tls-cert`, `--tls-key` | `HUBLOT_TLS_CERT`, `HUBLOT_TLS_KEY` | vides : servir directement en HTTPS avec un certificat que vous fournissez |
| `--public-origin` | `HUBLOT_PUBLIC_ORIGIN` | vide : l'URL publique derrière un reverse proxy |

`hublot --help` les liste ; `hublot --version` affiche la version.

## Derrière un reverse proxy HTTPS

Gardez le port sur `127.0.0.1`, faites pointer le proxy vers `http://127.0.0.1:7878`, et donnez à l'agent son origine
publique, par exemple `-e HUBLOT_PUBLIC_ORIGIN=https://hublot.mondomaine.fr` avant le nom de l'image. Sans elle, un
proxy qui change le schéma fait échouer toute requête autre que GET par `forbidden_origin`, connexion comprise, et le
cookie de session n'est pas marqué `Secure`.

L'onglet Logs est un flux Server-Sent Events : le proxy ne doit pas le mettre en tampon (l'agent envoie
`X-Accel-Buffering: no`, que nginx respecte) et doit accepter les lectures longues (le flux envoie un battement toutes
les 15 secondes). L'agent ne croit jamais les en-têtes `X-Forwarded-*`.

Sans proxy, `--tls-cert` et `--tls-key` servent directement en HTTPS. La v0.1 n'obtient pas de certificat elle-même.

## Binaire et systemd

L'alternative au conteneur : le binaire statique, lancé en root par systemd. Vérifiez d'abord l'archive
([Vérifier la release](#vérifier-la-release)). Utilisez `amd64` ou `arm64`.

```sh
V=0.1.0
ARCH=amd64
curl -fLO "https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V/hublot-kit_${V}_linux_${ARCH}.tar.gz"
tar -xzf "hublot-kit_${V}_linux_${ARCH}.tar.gz"
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service
sudo /usr/local/bin/hublot reset-token
sudo systemctl daemon-reload
sudo systemctl enable --now hublot-kit
```

`hublot reset-token` avant le premier démarrage affiche le jeton dans votre terminal. Démarré d'abord, l'agent
l'écrirait dans le journal du système, que lisent les membres des groupes `adm` ou `systemd-journal`.

Sur un hôte SELinux, `install` donne au binaire la bonne étiquette ; un binaire déplacé par `mv` depuis un dossier
personnel garde une étiquette que systemd refuse de lancer (`sudo restorecon /usr/local/bin/hublot` la corrige).

L'unité écoute sur `127.0.0.1:7878`, garde ses données dans `/var/lib/hublot-kit` (droits 0700) et tourne avec
`NoNewPrivileges`, `ProtectSystem=strict` et `PrivateTmp`. Elle ne masque pas `/proc` et ne retire aucune capacité :
l'explorateur en a besoin.

### Réglages

Changez les options par un fichier complémentaire (drop-in), jamais en modifiant l'unité :

```sh
sudo systemctl edit hublot-kit
```

```ini
[Service]
Environment=HUBLOT_PUBLIC_ORIGIN=https://hublot.example.com
Environment=HUBLOT_READ_ONLY=true
```

```sh
sudo systemctl restart hublot-kit
```

## Jeton d'accès

Un nouveau jeton invalide l'ancien une fois l'agent relancé ; d'ici là, un agent déjà lancé accepte encore l'ancien.
Après 5 échecs de connexion en une minute depuis une adresse, les connexions depuis cette adresse sont refusées
jusqu'à la fin de la minute.

Avec Docker :

```sh
docker exec hublot-kit /hublot reset-token
docker restart hublot-kit
```

Avec systemd :

```sh
sudo /usr/local/bin/hublot reset-token
sudo systemctl restart hublot-kit
```

## Dossier de données

`--data-dir` doit être dédié à Hublot-Kit. Il contient `token.sha256` (l'empreinte du jeton, jamais le jeton) et
`audit.log`. Absent, il est créé en 0700. Existant, ses droits ne sont pas modifiés, mais l'agent refuse de démarrer
si c'est un lien symbolique, s'il appartient à un autre utilisateur, ou s'il est accessible en écriture au groupe ou à
tous (`/tmp`, par exemple). Ses fichiers ne sont jamais ouverts à travers un lien symbolique. Le journal d'audit n'est
pas tourné en v0.1.

## Mise à jour

Avec Docker, tirez l'image, supprimez le conteneur et relancez la commande de [Docker](#docker-recommandé) ; le volume
`hublot-data` garde le jeton.

```sh
docker pull ghcr.io/levy-cmd/hublot-kit:0.1
docker rm -f hublot-kit
```

Avec systemd, téléchargez et vérifiez la nouvelle archive, puis remplacez le binaire et relancez :

```sh
sudo install -m 0755 hublot /usr/local/bin/hublot
sudo systemctl restart hublot-kit
```

## Désinstallation

Avec Docker :

```sh
docker rm -f hublot-kit
docker volume rm hublot-data
```

Avec systemd :

```sh
sudo systemctl disable --now hublot-kit
sudo rm -rf /usr/local/bin/hublot /etc/systemd/system/hublot-kit.service /etc/systemd/system/hublot-kit.service.d
sudo systemctl daemon-reload
sudo rm -rf /var/lib/hublot-kit
```

## Vérifier la release

Les releases sont construites et signées par GitHub Actions, avec des signatures [cosign](https://docs.sigstore.dev/)
sans clé (keyless), liées au workflow de release de ce dépôt. Il faut cosign 3.0 ou plus récent.

Ce que prouve une signature valide : le fichier ou l'image sort de ce workflow, lancé sur ce tag, et n'a pas changé
depuis. Ce qu'elle ne prouve pas : que le code du tag a été relu, ni qu'un SBOM est complet (il liste ce que syft,
pour les archives, et BuildKit, pour l'image, ont trouvé dans le binaire). Aucune provenance SLSA n'est publiée en
v0.1.

### Archives

`checksums.txt` donne le SHA-256 des archives et de leurs SBOM SPDX (`.sbom.json`) ; sa signature les couvre. Les
bundles `.sigstore.json` n'y figurent pas : chacun porte son propre certificat, contrôlé par `cosign verify-blob`.
Chaque archive a aussi son propre bundle.

```sh
V=0.1.0
ARCH=amd64
BASE="https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V"
curl -fLO "$BASE/hublot-kit_${V}_linux_${ARCH}.tar.gz"
curl -fLO "$BASE/checksums.txt"
curl -fLO "$BASE/checksums.txt.sigstore.json"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt
```

### Image

L'image est signée par son empreinte, pour linux/amd64 et linux/arm64, et porte une attestation SBOM.

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1
docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:0.1 --format '{{ json .SBOM }}'
```

Pour lancer exactement l'image vérifiée, remplacez `:0.1` dans la commande de [Docker](#docker-recommandé) par
l'empreinte qu'affiche `cosign verify` (`ghcr.io/levy-cmd/hublot-kit@sha256:…`).
````

`docs/en/security.md` :

````markdown
# Security model

## Hublot-Kit is root on the host

The agent talks to the Docker socket, which is root access to the host, and with `--pid=host` it reads any
container's files through `/proc/<pid>/root`. Whoever can log in to Hublot-Kit, or take over the agent, is root on
the server. The measures below limit mistakes and exposure; they do not protect against a compromised agent.

Treat the access token like the root password of the server.

## Exposure

- The agent listens on `127.0.0.1:7878` by default. On any other address without TLS it prints a warning at
  startup (legitimate inside a container published on `127.0.0.1`, or behind a reverse proxy).
- Reach it through an SSH tunnel, or through an HTTPS reverse proxy with `--public-origin` set
  ([installation](install.md#behind-an-https-reverse-proxy)).
- The agent makes no outbound connection: no telemetry, no update check.

## Login and session

- The token is 32 random bytes, printed once. Only its SHA-256 hash is stored (`token.sha256`), and it is compared in
  constant time. `hublot reset-token` replaces it.
- 5 failed logins per minute and per IP address, then `rate_limited`. Every failure is written to the audit log.
- The session lives in memory for 12 hours. Its cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` with TLS or an
  `https` public origin.
- Every request other than GET or HEAD must carry an `Origin` header equal to the trusted origin (`--public-origin`,
  or the `Host` of the request). `X-Forwarded-*` headers are never trusted.
- DNS rebinding: without `--public-origin`, the trusted origin comes from `Host`, so a rebound name passes the
  `Origin` check; the `SameSite=Strict` cookie and the token are then what protect you, and a rebound page can use
  up the login attempts of `127.0.0.1` for a minute. Exposed beyond `127.0.0.1`, set `--public-origin`.

## Content of the containers

- Files, logs, file names and variables are always escaped by the interface; `{@html}` is forbidden in the code and
  enforced by a lint rule. The preview only serves `text/plain` or a PNG, JPEG, GIF or WebP image, with `nosniff`;
  SVG and HTML are shown as text, never rendered. Downloads are always `attachment`.
- Headers: a strict `Content-Security-Policy` (scripts limited to the agent and the hashes of the embedded
  `index.html`, `frame-ancestors 'none'`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.

## Reading files

- The agent pins the container's process with a pidfd, checks its cgroup, and opens every path with
  `filepath-securejoin` inside the container's root: a symbolic link can never lead outside the container.
- Pseudo-filesystems (`/proc`, `/sys` and the like) are listed but cannot be opened: they would be read with the
  agent's host privileges. Device nodes, FIFOs and sockets are refused: a container's root could create a node of a
  host device.
- A container that bind-mounts the host (`-v /:/host`) shows the host in the explorer, and its files can be read:
  the user is root anyway. Downloading a folder of such a container is refused.

## Writing files

- `--read-only` disables every write, in a single place in the code.
- Every write goes through a confirmation that names the container and the path, then through Docker
  (`PUT /archive`), never directly. The path must contain no symbolic link at all.
- Writes are refused in a container that bind-mounts `/`, `/proc`, `/sys` (or below) or `/dev` of the host, and
  where Docker does not write what the container sees (`/etc/hosts`, tmpfs, mount points).
- The audit log records the intent before the write and the result after it. If the intent cannot be written,
  nothing is written in the container.

## Secrets

- The X-ray masks a variable when its name contains `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`,
  `CREDENTIAL`, `PRIVATE` or `DSN`, when its value holds a URL with credentials, or a password in a known form
  (connection strings, `--requirepass …`, `Authorization: Bearer …`). The command and the error message are masked
  part by part.
- A masked value is revealed one at a time, and each reveal is written to the audit log (the name, never the value)
  before the value is sent.
- This is a heuristic: [some forms are not recognised](limitations.md#secret-masking). Logs are never masked.

## Audit log

`audit.log` in the data directory, one JSON line per event: failed logins, revealed variables, edited and uploaded
files, with the IP address of the TCP connection. It is append-only and is not rotated in v0.1.

## Docker socket

Mounting the socket `:ro` protects nothing: it is a socket, not a file. A Docker socket proxy that only allows the
endpoints Hublot-Kit uses (`_ping`; containers: list, inspect, changes, logs, archive; volumes: inspect) narrows what the
agent can ask Docker, but with `--pid=host` the agent stays root on the host.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting (the **Security** tab of the repository, **Report a
vulnerability**) rather than a public issue.
````

`docs/fr/securite.md` :

````markdown
# Modèle de sécurité

## Hublot-Kit est root sur l'hôte

L'agent parle au socket Docker, qui donne l'accès root à l'hôte, et avec `--pid=host` il lit les fichiers de
n'importe quel conteneur par `/proc/<pid>/root`. Qui peut se connecter à Hublot-Kit, ou prendre la main sur l'agent,
est root sur le serveur. Les mesures ci-dessous limitent les erreurs et l'exposition ; elles ne protègent pas contre
un agent compromis.

Traitez le jeton d'accès comme le mot de passe root du serveur.

## Exposition

- L'agent écoute sur `127.0.0.1:7878` par défaut. Sur toute autre adresse sans TLS, il affiche un avertissement au
  démarrage (légitime dans un conteneur publié sur `127.0.0.1`, ou derrière un reverse proxy).
- Accédez-y par un tunnel SSH, ou par un reverse proxy HTTPS avec `--public-origin`
  ([installation](installation.md#derrière-un-reverse-proxy-https)).
- L'agent n'ouvre aucune connexion sortante : ni télémétrie, ni recherche de mise à jour.

## Connexion et session

- Le jeton fait 32 octets aléatoires, affichés une seule fois. Seule son empreinte SHA-256 est stockée
  (`token.sha256`), et la comparaison se fait en temps constant. `hublot reset-token` le remplace.
- 5 échecs de connexion par minute et par adresse IP, puis `rate_limited`. Chaque échec est écrit dans le journal
  d'audit.
- La session vit en mémoire pendant 12 heures. Son cookie est `HttpOnly`, `SameSite=Strict`, et `Secure` avec TLS ou
  une origine publique en `https`.
- Toute requête autre que GET ou HEAD doit porter un en-tête `Origin` égal à l'origine de confiance
  (`--public-origin`, ou le `Host` de la requête). Les en-têtes `X-Forwarded-*` ne sont jamais crus.
- Rebinding DNS : sans `--public-origin`, l'origine de confiance vient de `Host`, donc un nom rebindé passe le
  contrôle d'`Origin` ; ce sont alors le cookie `SameSite=Strict` et le jeton qui vous protègent, et une page rebindée
  peut épuiser les tentatives de connexion de `127.0.0.1` pendant une minute. Exposé hors de `127.0.0.1`, renseignez
  `--public-origin`.

## Contenu des conteneurs

- Fichiers, logs, noms de fichiers et variables sont toujours échappés par l'interface ; `{@html}` est interdit dans
  le code, et une règle de lint le fait respecter. L'aperçu ne sert que du `text/plain` ou une image PNG, JPEG, GIF ou
  WebP, avec `nosniff` ; SVG et HTML s'affichent comme du texte, jamais rendus. Les téléchargements sont toujours en
  `attachment`.
- En-têtes : une `Content-Security-Policy` stricte (scripts limités à l'agent et aux empreintes de l'`index.html`
  embarqué, `frame-ancestors 'none'`), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.

## Lecture des fichiers

- L'agent épingle le process du conteneur par un pidfd, contrôle son cgroup, et ouvre chaque chemin avec
  `filepath-securejoin` dans la racine du conteneur : un lien symbolique ne peut jamais mener hors du conteneur.
- Les pseudo-systèmes de fichiers (`/proc`, `/sys` et assimilés) sont listés mais ne s'ouvrent pas : ils seraient lus
  avec les privilèges de l'agent sur l'hôte. Les nœuds de périphérique, FIFO et sockets sont refusés : le root d'un
  conteneur pourrait créer un nœud d'un périphérique de l'hôte.
- Un conteneur qui monte l'hôte (`-v /:/host`) montre l'hôte dans l'explorateur, et ses fichiers se lisent :
  l'utilisateur est root de toute façon. Le téléchargement d'un dossier d'un tel conteneur est refusé.

## Écriture des fichiers

- `--read-only` désactive toute écriture, en un seul endroit du code.
- Chaque écriture passe par une confirmation qui rappelle le conteneur et le chemin, puis par Docker
  (`PUT /archive`), jamais directement. Le chemin ne doit contenir aucun lien symbolique.
- L'écriture est refusée dans un conteneur qui monte `/`, `/proc`, `/sys` (ou dessous) ou `/dev` de l'hôte, et là où
  Docker n'écrit pas ce que voit le conteneur (`/etc/hosts`, tmpfs, points de montage).
- Le journal d'audit note l'intention avant l'écriture et le résultat après. Si l'intention ne s'écrit pas, rien
  n'est écrit dans le conteneur.

## Secrets

- La radiographie masque une variable quand son nom contient `PASSWORD`, `PASSWD`, `PASS`, `TOKEN`, `KEY`, `SECRET`,
  `CREDENTIAL`, `PRIVATE` ou `DSN`, quand sa valeur contient une URL avec identifiants, ou un mot de passe sous une
  forme connue (chaînes de connexion, `--requirepass …`, `Authorization: Bearer …`). La commande et le message
  d'erreur sont masqués partie par partie.
- Une valeur masquée se révèle une à une, et chaque révélation est écrite dans le journal d'audit (le nom, jamais la
  valeur) avant l'envoi de la valeur.
- C'est une heuristique : [certaines formes ne sont pas reconnues](limites.md#masquage-des-secrets). Les logs ne sont
  jamais masqués.

## Journal d'audit

`audit.log` dans le dossier de données, une ligne JSON par événement : connexions échouées, variables révélées,
fichiers édités et envoyés, avec l'adresse IP de la connexion TCP. Il est en ajout seul et n'est pas tourné en v0.1.

## Socket Docker

Monter le socket en `:ro` ne protège rien : c'est un socket, pas un fichier. Un proxy de socket Docker qui n'autorise
que les points d'API utilisés par Hublot-Kit (`_ping` ; conteneurs : liste, inspection, changements, logs, archive ;
volumes : inspection) réduit ce que l'agent peut demander à Docker, mais avec `--pid=host` l'agent reste root sur
l'hôte.

## Signaler une vulnérabilité

Merci d'utiliser le signalement privé de vulnérabilités de GitHub (onglet **Security** du dépôt, **Report a
vulnerability**) plutôt qu'un ticket public.
````

`docs/en/limitations.md` :

````markdown
# Known limitations (v0.1)

v0.1 is a first public version. This page lists what it does not do, and the cases it knowingly does not detect.
The technical spec ([§13, in French](../superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md)) has every detail.

## Scope

- One Docker server, one user. Multiple servers or users: later.
- Not supported: rootless Docker, `userns-remap`, Podman, Kubernetes.
- Not in v0.1: processes and resources, start/stop/restart, web terminal (planned for v0.2); certificates from
  Let's Encrypt (bring your own); rotation of the audit log.

## Host

- Kernel 5.6 or later to read; 5.8 or later to edit or upload in a running container. Docker API 1.44 or later,
  overlay2 or containerd storage.
- Only the cgroup layouts of Docker are recognised: with the cgroupfs driver and a custom `--cgroup-parent`, a
  running container answers `container_changed` and cannot be explored. With the systemd driver it works.
- Measured on Ubuntu (AppArmor) and Fedora (SELinux enforcing). Other security modules were not measured.

## Files

- Pseudo-filesystems (`/proc`, `/sys`…) appear as folders that cannot be opened. A pseudo-filesystem type that
  the agent does not know would not be refused; device nodes stay refused in any case.
- A stopped container is read from an index of its full archive, built on first access: this can take seconds on a
  large image, with a progress bar. The index is capped at 4 GiB read, 5 minutes and 1,000,000 entries
  (`too_large`); three indexes are kept in memory.
- A stopped container that bind-mounts `/`, `/proc`, `/sys` (or below) or `/dev` of the host cannot be explored (the
  error names the mount): its archive would contain the host. Its logs, changes and X-ray stay available.
- A host folder bind-mounted into a stopped container, with a `/proc`, `/sys`, FUSE or NFS mount below it, is not
  detected: indexing may then make Docker read it, and a stuck FUSE or NFS server can block `dockerd`.
- A FUSE mount owned by a running container that stops answering: requests to that container fail after 10
  seconds, at most 4 at a time, and the stuck reads stay blocked inside the agent until it restarts.
- File names that are not valid UTF-8 are listed, but cannot be opened from the interface.
- Downloading a folder of a running container is refused when the archive would cross a host bind, a
  pseudo-filesystem or a FUSE mount, and always for `/`. A mount added between the check and the download (it needs
  `CAP_SYS_ADMIN` in the container) or a "hard" NFS mount below the folder is not refused, and can block `dockerd`.

## Writes

- Not atomic: Docker deletes the file and creates a new one. Hard links to the old file are broken, a process that
  keeps it open still sees the old content, and a write cut after one minute leaves the file lost or truncated.
- Between the checks and the write, a process of the container can still change the file or put a symbolic link in
  the path; Docker then resolves the path inside the container. It can reach another mount of the container, such
  as a bind of a host folder other than `/`, `/proc`, `/sys` or `/dev`.
- Refused where Docker does not write what the container sees: `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`,
  mount points, tmpfs, `/dev`, `/proc`, `/sys`. A tmpfs declared under another tmpfs, a volume or a bind makes every
  write to that running container refused (fail closed).
- Mounts that `docker inspect` does not show (Swarm configs and secrets, `--init`, CDI or `--gpus`): an edit in a
  running container is refused, but an upload, or any write to a stopped container, is not checked.
- A write under a stuck FUSE or "hard" NFS mount can block `dockerd`, and the container stays locked for writes until
  the agent restarts; the interface may then wrongly say that the write may have happened.
- A chroot jail mounted by the container itself (`CAP_SYS_ADMIN`) is not seen.

## Logs

- Logs are shown as `docker logs` shows them: secrets written by the application are not masked.
- Beyond 1,000 lines per second, extra lines are not displayed but counted; the download has them all.
- At most 8 streams for the agent; when they are all taken, the oldest stream gives its place and says so.
- Resuming after a cut can replay a few lines that share a timestamp, count dropped lines twice, or skip lines after
  a log rotation or a clock going backwards.
- A logging driver Docker cannot read back (`none`, `syslog`…) gives `logs_unavailable`.

## Secret masking

The X-ray masks secrets by name and by known forms. Not recognised in v0.1, among others:

- JSON values (`{"token": "…"}`), `user:password@host` without `://`, `password = …` with spaces, a variable named
  `DB_PWD`;
- in the command: `mysql -pSECRET`, secrets in a script or in a file passed as an argument, `--password -s3cr3t`,
  `--pwd s3cr3t` as two arguments, keys with non-ASCII or invisible characters, `Authorization: Token …` or
  `ApiKey …`;
- some masks leave a remainder visible, and `env PWD=/srv` is masked although it is not a secret.

The command itself cannot be revealed in v0.1 (`docker inspect` can).
````

`docs/fr/limites.md` :

````markdown
# Limites connues (v0.1)

La v0.1 est une première version publique. Cette page dit ce qu'elle ne fait pas, et les cas qu'elle ne détecte pas,
en connaissance de cause. La spec technique ([§13](../superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md)) donne
tous les détails.

## Périmètre

- Un serveur Docker, un utilisateur. Plusieurs serveurs ou utilisateurs : plus tard.
- Non pris en charge : Docker rootless, `userns-remap`, Podman, Kubernetes.
- Absents de la v0.1 : process et ressources, démarrer / arrêter / redémarrer, terminal web (prévus pour la v0.2) ;
  certificats Let's Encrypt (fournissez le vôtre) ; rotation du journal d'audit.

## Hôte

- Noyau 5.6 ou plus récent pour lire ; 5.8 ou plus récent pour éditer ou envoyer dans un conteneur actif. API Docker
  1.44 ou plus récente, stockage overlay2 ou containerd.
- Seules les dispositions de cgroups de Docker sont reconnues : avec le pilote cgroupfs et un `--cgroup-parent`
  personnalisé, un conteneur actif répond `container_changed` et ne s'explore pas. Avec le pilote systemd, cela
  marche.
- Mesuré sur Ubuntu (AppArmor) et Fedora (SELinux `enforcing`). Les autres modules de sécurité n'ont pas été mesurés.

## Fichiers

- Les pseudo-systèmes de fichiers (`/proc`, `/sys`…) apparaissent comme des dossiers qui ne s'ouvrent pas. Un type de
  pseudo-système que l'agent ne connaît pas ne serait pas refusé ; les nœuds de périphérique restent refusés quoi
  qu'il arrive.
- Un conteneur arrêté se lit par un index de son archive complète, construit au premier accès : cela peut prendre des
  secondes sur une grosse image, avec une barre de progression. L'index est plafonné à 4 Gio lus, 5 minutes et
  1 000 000 d'entrées (`too_large`) ; trois index sont gardés en mémoire.
- Un conteneur arrêté qui monte `/`, `/proc`, `/sys` (ou dessous) ou `/dev` de l'hôte ne s'explore pas (l'erreur
  nomme le montage) : son archive contiendrait l'hôte. Ses logs, ses changements et sa radiographie restent
  disponibles.
- Un dossier de l'hôte monté dans un conteneur arrêté, avec un montage `/proc`, `/sys`, FUSE ou NFS en dessous, n'est
  pas détecté : l'indexation peut alors faire lire ce montage à Docker, et un serveur FUSE ou NFS bloqué peut bloquer
  `dockerd`.
- Un montage FUSE d'un conteneur actif qui ne répond plus : les requêtes vers ce conteneur échouent après 10 secondes,
  4 au plus en même temps, et les lectures bloquées le restent dans l'agent jusqu'à son redémarrage.
- Les noms de fichiers qui ne sont pas en UTF-8 valide sont listés, mais ne s'ouvrent pas depuis l'interface.
- Le téléchargement d'un dossier d'un conteneur actif est refusé quand l'archive traverserait un montage de l'hôte,
  un pseudo-système ou un FUSE, et toujours pour `/`. Un montage ajouté entre le contrôle et le téléchargement (il
  faut `CAP_SYS_ADMIN` dans le conteneur) ou un montage NFS « dur » sous le dossier n'est pas refusé, et peut bloquer
  `dockerd`.

## Écritures

- Pas atomiques : Docker supprime le fichier et en crée un nouveau. Les liens physiques vers l'ancien sont rompus, un
  process qui le garde ouvert voit encore l'ancien contenu, et une écriture coupée après une minute laisse le fichier
  perdu ou tronqué.
- Entre les contrôles et l'écriture, un process du conteneur peut encore modifier le fichier ou poser un lien
  symbolique dans le chemin ; Docker résout alors le chemin dans le conteneur. Il peut atteindre un autre montage du
  conteneur, comme le montage d'un dossier de l'hôte autre que `/`, `/proc`, `/sys` ou `/dev`.
- Refusées là où Docker n'écrit pas ce que voit le conteneur : `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`,
  points de montage, tmpfs, `/dev`, `/proc`, `/sys`. Un tmpfs déclaré sous un autre tmpfs, un volume ou un montage
  fait refuser toute écriture dans ce conteneur actif (échec fermé).
- Montages que `docker inspect` ne montre pas (configs et secrets Swarm, `--init`, CDI ou `--gpus`) : une édition
  dans un conteneur actif est refusée, mais un envoi, ou toute écriture dans un conteneur arrêté, n'est pas contrôlé.
- Une écriture sous un montage FUSE bloqué ou NFS « dur » peut bloquer `dockerd`, et le conteneur reste verrouillé
  pour l'écriture jusqu'au redémarrage de l'agent ; l'interface peut alors dire à tort que l'écriture a peut-être eu
  lieu.
- Une prison chroot montée par le conteneur lui-même (`CAP_SYS_ADMIN`) n'est pas vue.

## Logs

- Les logs s'affichent comme `docker logs` les montre : les secrets écrits par l'application ne sont pas masqués.
- Au-delà de 1 000 lignes par seconde, les lignes en trop ne s'affichent pas mais sont comptées ; le téléchargement
  les contient toutes.
- 8 flux au plus pour l'agent ; quand ils sont tous pris, le flux le plus ancien cède sa place et le dit.
- La reprise après une coupure peut rejouer quelques lignes de même horodatage, compter deux fois des lignes
  écartées, ou sauter des lignes après une rotation du journal ou un recul de l'horloge.
- Un pilote de logs que Docker ne sait pas relire (`none`, `syslog`…) donne `logs_unavailable`.

## Masquage des secrets

La radiographie masque les secrets par leur nom et par des formes connues. Non reconnus en v0.1, entre autres :

- les valeurs JSON (`{"token": "…"}`), `user:motdepasse@hôte` sans `://`, `password = …` avec des blancs, une
  variable nommée `DB_PWD` ;
- dans la commande : `mysql -pSECRET`, un secret dans un script ou dans un fichier passé en argument,
  `--password -s3cr3t`, `--pwd s3cr3t` en deux arguments, des clés avec des caractères non ASCII ou invisibles,
  `Authorization: Token …` ou `ApiKey …` ;
- certains masques laissent voir un reste de la valeur, et `env PWD=/srv` est masqué alors que ce n'est pas un
  secret.

La commande elle-même ne se révèle pas en v0.1 (`docker inspect` le peut).
````

- [x] **Step 4: Captures d'écran (Q48)** — les deux PNG que citent les README (`docs/images/containers.png`, `docs/images/explorer.png`). Agent lancé comme dans `scripts/e2e.sh` (conteneur `hublot-e2e`, aucune donnée réelle), interface en anglais ; deux captures Playwright jetables (non commitées) en 1280 × 800 : la liste des conteneurs, puis l'explorateur de `hublot-e2e` sur `/root` avec les marques du diff. 300 Ko au plus chacune (`sips -Z 1280`, puis `pngquant` s'il est installé) ; aucun nom d'hôte, chemin personnel ou jeton visible.

Run: `ls -l docs/images/*.png | awk '{print $5, $9}'`
Expected: deux fichiers, chacun sous 307 200 octets.

- [x] **Step 5: Le voir passer, puis le job CI**

Run: `scripts/check-docs.sh 2>&1 | tail -n 3`
Expected: `DOCS OK`.

Mutation (puis annulée) : remplacer `--cap-add SYS_PTRACE` par `--cap-add SYS_ADMIN` dans `README.fr.md` → `ERREUR : README.md et README.fr.md : blocs de code différents` et `ERREUR : README.fr.md : la commande docker run diffère de la spec §8`. `git checkout README.fr.md`.

Révision 2026-10-15 (contrat T5/T6 du registre) : le job `docs` n'est pas ajouté ici, `ci.yml` appartenant à la tâche 6 ; il arrive avec la tranche A, après la fusion de cette tâche (version définitive : tranche A, étape 3). Pour mémoire, la forme d'origine :

```yaml
  # Plan Lancement : la documentation publique (commande de la spec §8, FR et EN alignés, liens).
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - run: scripts/check-docs.sh
```

- [x] **Step 6: Commit**

```bash
git add README.md README.fr.md docs/en docs/fr scripts/check-docs.sh
git add docs/images
git commit -F - <<'EOF'
docs : README en anglais et en français, installation, sécurité et limites en FR et EN

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 7: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex, avec la spec ouverte. Points : chaque phrase de sécurité contre §7 (rien de plus fort que ce que le code fait) ; chaque limite contre §13 (rien d'omis qui puisse surprendre un utilisateur : écriture non atomique, binds de l'hôte, FUSE et NFS, masquage heuristique, logs non masqués) ; options et valeurs par défaut contre `internal/config` ; commandes de la doc contre `check-systemd.sh` et `check-image.sh` ; traduction fidèle. Corrections en commits séparés ; la tâche 6 attend les deux avis.

---

### Task 6: Socle GoReleaser et job CI `release-snapshot` (presque faite)

**Révision 2026-10-15 :** les étapes 1 à 4 sont faites (`f05b69f` : `.goreleaser.yaml`, job `release-snapshot` de `.github/workflows/ci.yml`, `scripts/check-dist.sh`, `/dist/` dans `.gitignore`) ; leur contenu est dans le dépôt, et le texte d'origine dans `ee58f7c` de ce plan. Il reste les étapes 5 et 6 ci-dessous, à l'agent de la tâche 6 : CI verte sur la branche (T5 comprise) et relecture. Ce socle sert les trois tranches : l'image du snapshot à la tranche A, les archives à la tranche B, la signature (`signs`, `docker_signs`, `release`) à la tranche C.

**Fichiers possédés (jusqu'à la fin de la tâche) :** `.goreleaser.yaml`, `.github/workflows/ci.yml`, `scripts/check-dist.sh`, `.gitignore`.

**Fichiers lus (chez d'autres) :** `Dockerfile` et `scripts/check-image.sh` (tâche 4, puis tranche A), `packaging/systemd/hublot-kit.service` (tâche 3, puis tranche B), `README.md`, `README.fr.md`, `LICENSE` (mis dans les archives ; tâche 5), `Makefile` (`make web`), `scripts/release-lib.sh`.

**Interfaces (produites, consommées par les tranches) :** `dist/` (archives `hublot-kit_<V>_linux_{amd64,arm64}.tar.gz` au contenu exact `LICENSE README.fr.md README.md hublot hublot-kit.service`, `checksums.txt`, SBOM SPDX par archive, `metadata.json`) ; en snapshot, images locales `ghcr.io/levy-cmd/hublot-kit:<V>-amd64` et `…-arm64` ; artefact CI `release-snapshot` (`hublot-kit-arm64.tar`, archives, `checksums.txt`, `metadata.json`).

- [x] **Step 5: La CI sur la branche, T5 comprise** — `f05b69f` est poussé et la PR brouillon #6 existe ; la branche contient maintenant la tâche 5 fusionnée (`9d872c7`). Relancer la CI sur le dernier commit et la suivre :

```bash
git switch phase/lancement && git pull --ff-only
sha=$(git rev-parse HEAD)
run=$(gh run list --workflow ci.yml --commit "$sha" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
if [ -n "$run" ]; then gh run rerun "$run"; else git push; sleep 15; run=$(gh run list --workflow ci.yml --commit "$sha" --limit 1 --json databaseId --jq '.[0].databaseId'); fi
gh run watch "$run" --exit-status 2>&1 | tail -n 12
```

Expected: tous les jobs verts, dont `release-snapshot` (le job `docs` arrive avec la tranche A). Dans son journal (`gh run view "$run" --log | grep -E 'DIST OK|IMAGE OK|ok hublot-kit_|ok hk-check|mise à jour'`) : `DIST OK : <V>`, les deux `ok hublot-kit_<V>_linux_…`, les cinq `ok hk-check-…`, `ok édition …`, `ok envoi …`, `ok mise à jour …`, `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:<V>-amd64`, et l'étape « Étiquettes des images du snapshot ». C'est cette exécution qui prouve `check-dist.sh`, les étiquettes du snapshot et l'image multi-architecture (tableau des critères).

Si GoReleaser refuse la configuration, corriger et noter le constat. Points fragiles connus (`dockers_v2` est encore « provisoire » chez GoReleaser) : une étiquette vide (`X.Y` d'une pré-version) — si elle est refusée, remplacer la liste `tags` par deux entrées `dockers_v2` dont la seconde a `disable: "{{ .Prerelease }}"` ; `sbom` en gabarit ; l'emplacement du binaire dans le contexte (`$TARGETPLATFORM/hublot`). Chaque correction : commit `correctif : …`, puis `gh pr checks --watch`.

- [x] **Step 6: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex. Points : aucun secret ni jeton dans le job de PR ; le registre local seul reçoit une image ; contenu exact des archives ; SBOM qui voit les modules Go ; drapeaux de compilation (`CGO_ENABLED=0`, `-trimpath`, `vcs.modified=false`) ; étiquettes `X.Y.Z` / `X.Y`, jamais `latest`. Corrections en commits séparés ; la tranche A attend les deux avis.

---

### Tranche A : installer avec Docker depuis une release

**Critère d'acceptation (fixe) :** la commande de la spec §8, telle que la doc la donne, lance l'image que produit la chaîne de release (image multi-architecture de `.goreleaser.yaml`, construite par la CI) sur Ubuntu (AppArmor) et Fedora (SELinux) en arm64, et sur le runner Ubuntu en amd64 : lecture des cinq cibles, édition et envoi relus par `docker exec`, mise à jour sur le même volume avec le même jeton, sans refus AppArmor ni AVC ; la doc Docker cite exactement l'image et l'étiquette que publie `.goreleaser.yaml`.

**Où tester :** macOS (scripts), CI (`release-snapshot`, `docs`), `hk-ubuntu`, `hk-fedora`.

**Fichiers possédés :** `scripts/release-lib.sh`, `scripts/check-image.sh`, `scripts/test-release-lib.sh`, `Dockerfile`, `.github/workflows/ci.yml` (job `docs` ; partie image de `release-snapshot` si un correctif l'exige), `.goreleaser.yaml` (section `dockers_v2` seulement, si un correctif l'exige), `scripts/check-docs.sh` (bloc « Tranche A »), et dans la doc : `README.md`, `README.fr.md` (démarrage rapide), `docs/en/install.md`, `docs/fr/installation.md` (sections Docker, options, reverse proxy, et les parties Docker de jeton, mise à jour et désinstallation). Un seul écrivain par fichier : la tranche B ne commence qu'après la fusion de A.

**Fichiers lus (chez d'autres) :** `.goreleaser.yaml` hors `dockers_v2` (tâche 6 ; `images`, `tags`), la spec §8 (commande, lue par `hk_spec_command`), les autres sections de `docs/*/install*.md` et `docs/*/security*`, `limitations`, que `check-docs.sh` contrôle en entier (tâche 5), `scripts/check-dist.sh` et le job `release-snapshot` (tâche 6), l'artefact CI `release-snapshot`.

**Interfaces:**
- Consumes: tâche 6 (images du snapshot `ghcr.io/levy-cmd/hublot-kit:<V>-amd64` / `-arm64`, artefact `release-snapshot`), tâche 4 (`check-image.sh IMAGE [VERSION]`), tâche 5 (doc ; `check-docs.sh`).
- Produces: `hk_check_startup_logs JOURNAL` dans `scripts/release-lib.sh` (échoue sur l'avertissement cgroup ou le détail « start the agent with --cgroupns=host », passe sur « without TLS »), appelée par `check-image.sh` ; le job CI `docs` (`scripts/check-docs.sh`, `scripts/test-release-lib.sh`) ; le bloc « Tranche A » de `check-docs.sh`, dont les tranches B et C suivent le modèle (variable `gr=.goreleaser.yaml`).

> **À comprendre :** le test unitaire de la garde cgroup remplace la mutation « retirer `--cgroupns=host` de la commande de §8 », que le contrôle de permissions a bloquée à la tâche 4 (registre) : au lieu de muter la spec, un faux journal d'agent passe par la garde que `check-image.sh` applique à `docker logs`. Pour cela, la garde, aujourd'hui en ligne dans `check-image.sh`, devient une fonction de `release-lib.sh`. La partie image de la chaîne (dockers_v2, snapshot, registre local, image amd64 lancée par la CI) est faite à la tâche 6 : cette tranche la vérifie de bout en bout et y aligne la doc.

- [x] **Step 1: Écrire les tests** — la garde cgroup dans `scripts/test-release-lib.sh`, et le bloc « Tranche A » de `scripts/check-docs.sh`, inséré juste avant la ligne finale `[ $errors = 0 ] || …` :

La partie test du diff ci-dessous (fichier `scripts/test-release-lib.sh`), et :

```bash
# Tranche A (plan Lancement) : l'image que cite la doc est celle que publie
# .goreleaser.yaml (dockers_v2), avec l'étiquette X.Y et sans latest (Q43).
gr=.goreleaser.yaml
img=$(awk '/^dockers_v2:/{d=1} d&&/images:/{gsub(/.*\[|\].*/,""); print; exit}' "$gr")
[ "$img" = ghcr.io/levy-cmd/hublot-kit ] || err "$gr : image « $img », la doc cite ghcr.io/levy-cmd/hublot-kit"
grep -qF -- '- "{{ if not .Prerelease }}{{ .Major }}.{{ .Minor }}{{ end }}"' "$gr" || err "$gr : pas d'étiquette X.Y, que cite la doc (:0.1)"
! grep -qE '^ +- "?latest"?$' "$gr" || err "$gr : étiquette latest (Q43)"
for f in README.md README.fr.md docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  for ref in $(grep -oE 'ghcr\.io/[a-z0-9._/-]+(:[A-Za-z0-9._-]+|@sha256)?' "$f" | sort -u); do
    case $ref in "$img:0.1" | "$img@sha256") ;; *) err "$f : image « $ref », attendu $img:0.1 (ou $img@sha256:…)" ;; esac
  done
done
```

- [x] **Step 2: Les voir échouer**

Run: `scripts/test-release-lib.sh 2>&1 | grep -E 'ÉCHEC|échec'`
Expected (vérifié à la rédaction) : `ÉCHEC : hk_check_startup_logs absente de scripts/release-lib.sh`, `ÉCHEC : garde cgroup : refus pour une autre raison : … hk_check_startup_logs: command not found`, `ÉCHEC : garde cgroup (détail procfs) : refus pour une autre raison : …`, `ÉCHEC : garde cgroup : avertissement TLS seul refusé`, `4 échec(s)`.

Run: `scripts/check-docs.sh | tail -n 1` (après la fusion de la tâche 5)
Expected: `DOCS OK` d'emblée (la doc de la tâche 5 est déjà alignée) ; le bloc mord, vérifié à la rédaction par mutation de `.goreleaser.yaml` : image renommée → `ERREUR : .goreleaser.yaml : image « ghcr.io/levy-cmd/hublot »…` ; étiquette `latest` ajoutée → `ERREUR : .goreleaser.yaml : étiquette latest (Q43)`.

- [x] **Step 3: Écrire le code** — la garde devient une fonction (diff complet, tests compris ; s'applique sur `phase/lancement` après la tâche 6 : `git apply --check` vérifié à la rédaction) :

```diff
diff --git a/scripts/check-image.sh b/scripts/check-image.sh
index 24b018f..2ff4682 100755
--- a/scripts/check-image.sh
+++ b/scripts/check-image.sh
@@ -56,9 +56,7 @@ tok=$(printf '%s\n' "$logs" | sed -n 's/^Access token (shown only once): //p')
 # Les journaux ne s'affichent que masqués (hk_redact) : le jeton n'entre
 # jamais dans la sortie, ni dans celle de la CI.
 [ -n "$tok" ] || hk_fail "jeton absent de docker logs : $(printf '%s\n' "$logs" | hk_redact)"
-# L'avertissement « without TLS » est attendu dans l'image (0.0.0.0 publié sur 127.0.0.1, spec §7) ;
-# celui de l'espace de noms cgroup ne doit jamais paraître (spec §8).
-case $logs in *"private cgroup namespace"*) hk_fail "avertissement cgroup au démarrage : $(printf '%s\n' "$logs" | hk_redact)" ;; esac
+hk_check_startup_logs "$logs"
 hk_login "$tok" "$jar"
 hk_check_meta "$jar" "$want"
 hk_check_targets "$jar"
diff --git a/scripts/release-lib.sh b/scripts/release-lib.sh
index f12826e..9721272 100644
--- a/scripts/release-lib.sh
+++ b/scripts/release-lib.sh
@@ -59,6 +59,17 @@ hk_redact() {
   sed -E 's/(shown only once):.*/\1: [masqué]/; s/[A-Za-z0-9_-]{43,}/[masqué]/g'
 }
 
+# hk_check_startup_logs JOURNAL : échoue si l'agent signale un espace de noms
+# cgroup privé (avertissement de démarrage, ou détail « start the agent with
+# --cgroupns=host » d'une erreur procfs, spec §8) ; l'avertissement « without
+# TLS » est attendu dans l'image (0.0.0.0 publié sur 127.0.0.1, spec §7).
+hk_check_startup_logs() {
+  case $1 in
+    *"private cgroup namespace"* | *"--cgroupns=host"*)
+      hk_fail "avertissement cgroup au démarrage : $(printf '%s\n' "$1" | hk_redact)" ;;
+  esac
+}
+
 # Conteneurs créés par ce script, les seuls que hk_remove_targets supprime :
 # un conteneur hk-check-* qui existait avant le test n'est jamais touché.
 HK_CREATED=
diff --git a/scripts/test-release-lib.sh b/scripts/test-release-lib.sh
index 771b526..88ad084 100755
--- a/scripts/test-release-lib.sh
+++ b/scripts/test-release-lib.sh
@@ -75,5 +75,32 @@ case $out in
   *) bad "hk_redact : $out" ;;
 esac
 
+# Garde du démarrage (hk_check_startup_logs, appelée par check-image.sh) : un
+# faux journal d'agent avec l'avertissement cgroup, ou le détail procfs « start
+# the agent with --cgroupns=host », échoue sans afficher le jeton ;
+# l'avertissement « without TLS » seul passe. Remplace la mutation « retirer
+# --cgroupns=host » de la commande de §8, que le contrôle de permissions a bloquée.
+(. scripts/release-lib.sh && type hk_check_startup_logs > /dev/null 2>&1) || bad "hk_check_startup_logs absente de scripts/release-lib.sh"
+cg=$(printf 'WARNING: listening on 0.0.0.0:7878 without TLS.\nWARNING: private cgroup namespace (/proc/self/cgroup = 0::/): the explorer will answer container_changed. Start the agent with --cgroupns=host.\nAccess token (shown only once): %s\n' "$tok")
+if (. scripts/release-lib.sh && hk_check_startup_logs "$cg") > "$tmp/out" 2>&1; then
+  bad "garde cgroup : journal avec l'avertissement accepté"
+elif ! grep -q 'avertissement cgroup' "$tmp/out"; then
+  bad "garde cgroup : refus pour une autre raison : $(tail -n 1 "$tmp/out")"
+elif grep -qF "$tok" "$tmp/out"; then
+  bad "garde cgroup : le jeton est affiché"
+else
+  echo "ok garde cgroup : $(tail -n 1 "$tmp/out" | cut -c1-60)"
+fi
+detail='file route error: the container changed: pid 42 is not in the cgroup of container 0123456789ab; start the agent with --cgroupns=host'
+if (. scripts/release-lib.sh && hk_check_startup_logs "$detail") > "$tmp/out" 2>&1; then
+  bad "garde cgroup : détail procfs accepté"
+elif ! grep -q 'avertissement cgroup' "$tmp/out"; then
+  bad "garde cgroup (détail procfs) : refus pour une autre raison : $(tail -n 1 "$tmp/out")"
+else
+  echo "ok garde cgroup (détail procfs)"
+fi
+(. scripts/release-lib.sh && hk_check_startup_logs 'WARNING: listening on 0.0.0.0:7878 without TLS.') > /dev/null 2>&1 &&
+  echo "ok garde cgroup : avertissement TLS seul accepté" || bad "garde cgroup : avertissement TLS seul refusé"
+
 [ "$fails" = 0 ] || { echo "$fails échec(s)" >&2; exit 1; }
 echo "RELEASE-LIB OK"
```

Le job `docs` de `.github/workflows/ci.yml`, à la fin de `jobs:` (contrat T5/T6 du registre : ajouté par l'écrivain de `ci.yml`, après la fusion de la tâche 5) :

```yaml
  # Plan Lancement : documentation publique (commande de §8, FR et EN alignés,
  # liens, et ce que la doc cite de .goreleaser.yaml), gardes des scripts.
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - run: bash scripts/check-docs.sh
      - run: bash scripts/test-release-lib.sh
```

- [x] **Step 4: Les voir passer**

Run: `scripts/test-release-lib.sh | tail -n 4 && scripts/check-docs.sh | tail -n 1 && go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 2>&1 | tail -n 3`
Expected: `ok garde cgroup : …`, `ok garde cgroup (détail procfs)`, `ok garde cgroup : avertissement TLS seul accepté`, `RELEASE-LIB OK`, `DOCS OK`, rien pour `actionlint`. Mutation (vérifiée à la rédaction, puis annulée) : la garde réduite au seul motif `private cgroup namespace` → `ÉCHEC : garde cgroup : détail procfs accepté`.

- [x] **Step 5: La doc Docker** — relire les sections possédées (EN et FR) contre `.goreleaser.yaml`, l'image locale de la tâche 4 et le code : image et étiquette, options, messages exacts (`Access token (shown only once): …`, `WARNING: listening on … without TLS`), mise à jour sur le même volume. Corriger s'il le faut (mêmes blocs de code en FR et EN) ; `scripts/check-docs.sh | tail -n 1` → `DOCS OK`.

- [x] **Step 6: Commit**

```bash
git add scripts/release-lib.sh scripts/check-image.sh scripts/test-release-lib.sh scripts/check-docs.sh .github/workflows/ci.yml
git add README.md README.fr.md docs/en/install.md docs/fr/installation.md 2>/dev/null || true
git commit -F - <<'EOF'
release : tranche A, installer avec Docker depuis une release (garde cgroup testée, doc vérifiée contre .goreleaser.yaml, job docs)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

- [x] **Step 7: Pousser, puis la CI de CE commit** — le job `docs` et `release-snapshot` doivent tourner sur le commit de l'étape 6 :

```bash
git push
sha=$(git rev-parse HEAD)
run=
for _ in $(seq 1 30); do
  run=$(gh run list --workflow ci.yml --commit "$sha" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
  [ -n "$run" ] && break
  sleep 5
done
test -n "$run" || echo "aucun run CI pour $sha"
gh run watch "$run" --exit-status 2>&1 | tail -n 12
```

Expected: `docs` et `release-snapshot` verts. Dans le journal de `release-snapshot` (`gh run view "$run" --log | grep -E 'IMAGE OK|ok hk-check|ok édition|ok envoi|mise à jour|-amd64|-arm64'`) : l'étape « Étiquettes des images du snapshot » (`…:<V>-amd64`, `…:<V>-arm64`), puis pour l'image amd64 lancée par la commande de §8 : les cinq `ok hk-check-…`, `ok édition (PUT fs/file)`, `ok envoi (POST fs/upload)`, `ok mise à jour …`, `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:<V>-amd64`.

- [x] **Step 8: L'image arm64 de la CI, sur les deux VM** — artefact du run de l'étape 7 (même commit, `--commit`) :

```bash
run=$(gh run list --workflow ci.yml --branch phase/lancement --commit "$(git rev-parse HEAD)" --status success --limit 1 --json databaseId --jq '.[0].databaseId')
test -n "$run" || echo "aucun run CI vert pour ce commit : pousser, attendre gh pr checks --watch"
snap=$(mktemp -d)
gh run download "$run" -n release-snapshot -D "$snap"
v=$(jq -r .version "$snap/metadata.json"); echo "$v"
limactl start hk-fedora >/dev/null 2>&1 || true
for vm in hk-ubuntu hk-fedora; do
  limactl shell "$vm" -- sh -c 'uname -r; sudo docker version -f "{{.Server.Version}} API {{.Server.APIVersion}}"; command -v getenforce >/dev/null && getenforce || true'
  limactl copy "$snap/hublot-kit-arm64.tar" "$vm:/tmp/"
  limactl shell "$vm" -- sudo docker load -q -i /tmp/hublot-kit-arm64.tar
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-image.sh "ghcr.io/levy-cmd/hublot-kit:$v-arm64" "$v" 2>&1 | tail -n 5
  limactl shell "$vm" -- sh -c "sudo docker rmi -f ghcr.io/levy-cmd/hublot-kit:$v-arm64 >/dev/null; rm -f /tmp/hublot-kit-arm64.tar"
done
limactl shell hk-ubuntu -- sudo journalctl -k --since -30min | grep -c 'apparmor="DENIED"'
limactl shell hk-fedora -- sudo ausearch -m avc -ts recent 2>&1 | tail -n 3
rm -rf "$snap"
```

Expected, sur chaque VM : `ok édition (PUT fs/file)`, `ok envoi (POST fs/upload)`, `ok mise à jour …`, `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:<V>-arm64` ; `0` refus AppArmor ; `<no matches>` ; Fedora en `Enforcing`. Noter noyau, version de Docker et API de chaque VM, commit et run, au registre (`.claude/handoff/phase-lancement.registre.md`), avec les verdicts de revue. **Un correctif** (doc, script, VM en échec) : nouveau commit, puis de nouveau les étapes 7 et 8 sur ce commit ; la CI et les VM portent toujours sur le dernier commit.

- [x] **Step 9: Relecture (CLAUDE.md, « Orchestration »)** — `relecteur` puis Codex (`codex:codex-rescue`, lecture seule), concordants ou arbitrés au registre. Points : CI et VM portent sur le dernier commit ; la garde refuse aussi le détail procfs et ne laisse jamais passer le jeton ; le bloc A de `check-docs.sh` lit bien `.goreleaser.yaml` (pas une copie) ; la commande de la doc reste celle de §8 ; résultats VM complets. La tranche B attend les deux avis et la fusion de A.

---

### Tranche B : installer avec systemd depuis une release

**Critère d'acceptation (fixe) :** l'archive que produit la chaîne de release (`hublot-kit_<V>_linux_<arch>.tar.gz` de `.goreleaser.yaml`, construite par la CI) s'installe en suivant la section systemd de la doc, sur Ubuntu (AppArmor) et Fedora (SELinux `enforcing`, `unconfined_service_t`) : unité vérifiée par `systemd-analyze`, données en 0700, jeton jamais dans le journal, lecture des cinq cibles, édition et envoi sous le durcissement de l'unité, rotation du jeton, désinstallation complète ; la doc systemd cite exactement le nom d'archive et l'unité que produit `.goreleaser.yaml`.

**Où tester :** macOS (`check-docs.sh`), CI (`release-snapshot` : `check-dist.sh`), `hk-ubuntu`, `hk-fedora`.

**Fichiers possédés (après la fusion de A) :** `packaging/systemd/hublot-kit.service`, `scripts/check-systemd.sh`, `scripts/check-dist.sh` (si un correctif l'exige), `.goreleaser.yaml` (section `archives` seulement, si un correctif l'exige), `scripts/release-lib.sh` (si un correctif l'exige), `scripts/check-docs.sh` (bloc « Tranche B »), et dans `docs/en/install.md`, `docs/fr/installation.md` : les sections « Binary and systemd » / « Binaire et systemd », « Settings » / « Réglages », et les parties systemd de jeton, mise à jour et désinstallation.

**Fichiers lus (chez d'autres) :** `.goreleaser.yaml` hors `archives` (`project_name`, `checksum`), `README.md`, `README.fr.md`, `LICENSE` (contenus dans l'archive ; tranche A, tâche 5), `scripts/release-lib.sh` et le bloc A de `check-docs.sh` (tranche A), l'artefact CI `release-snapshot`.

**Interfaces:**
- Consumes: tâche 6 (archives dans l'artefact `release-snapshot`, contenu vérifié par `check-dist.sh`), tâche 3 (`check-systemd.sh ARCHIVE VERSION`, version obligatoire), tranche A (bloc A de `check-docs.sh`, variable `gr`).
- Produces: le bloc « Tranche B » de `check-docs.sh` ; la preuve VM que l'archive de la CI s'installe comme la doc le dit.

- [x] **Step 1: Écrire le test** — le bloc « Tranche B » de `scripts/check-docs.sh`, après le bloc A :

```bash
# Tranche B (plan Lancement) : l'archive et l'unité que cite la doc sont
# celles que produit .goreleaser.yaml (archives).
proj=$(sed -n 's/^project_name: *//p' "$gr")
nt=$(awk '/^archives:/{a=1} a&&/name_template:/{sub(/.*name_template: */,""); gsub(/"/,""); print; exit}' "$gr")
[ "$proj" = hublot-kit ] && [ "$nt" = '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}' ] ||
  err "$gr : archives « $nt » (projet $proj), la doc attend hublot-kit_\${V}_linux_\${ARCH}.tar.gz"
grep -A1 -F 'src: packaging/systemd/hublot-kit.service' "$gr" | grep -q 'strip_parent: true' ||
  err "$gr : l'unité hublot-kit.service n'est pas à la racine de l'archive"
[ -f packaging/systemd/hublot-kit.service ] || err "packaging/systemd/hublot-kit.service absent"
for f in docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  grep -qF 'hublot-kit_${V}_linux_${ARCH}.tar.gz' "$f" || err "$f : nom d'archive de .goreleaser.yaml absent"
  grep -qxF 'sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service' "$f" ||
    err "$f : installation de l'unité absente"
  grep -qxF 'sudo /usr/local/bin/hublot reset-token' "$f" || err "$f : reset-token par chemin complet absent (sudo sur Fedora)"
done
```

- [x] **Step 2: Le voir passer, puis mordre**

Run: `scripts/check-docs.sh | tail -n 1`
Expected: `DOCS OK` (doc de la tâche 5 déjà alignée). Mutations de `.goreleaser.yaml` (vérifiées à la rédaction, puis annulées) : `name_template` en tirets → `ERREUR : .goreleaser.yaml : archives « {{ .ProjectName }}-{{ .Version }}-… »…` ; `strip_parent: false` → `ERREUR : .goreleaser.yaml : l'unité hublot-kit.service n'est pas à la racine de l'archive`.

- [x] **Step 3: La doc systemd** — relire les sections possédées (EN et FR) contre `check-systemd.sh`, l'unité et les résultats VM de la tâche 3 : `install`, unité, `sudo /usr/local/bin/hublot reset-token` avant le premier démarrage, `enable --now`, réglages par `systemctl edit`, SELinux (`restorecon`). Corriger s'il le faut ; `scripts/check-docs.sh | tail -n 1` → `DOCS OK`.

- [x] **Step 4: Commit, pousser, puis la CI de CE commit**

```bash
git add scripts/check-docs.sh docs/en/install.md docs/fr/installation.md
git commit -F - <<'EOF'
release : tranche B, installer avec systemd depuis une release (archive de la CI vérifiée sur les VM, doc vérifiée contre .goreleaser.yaml)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

```bash
git push
sha=$(git rev-parse HEAD)
run=
for _ in $(seq 1 30); do
  run=$(gh run list --workflow ci.yml --commit "$sha" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
  [ -n "$run" ] && break
  sleep 5
done
test -n "$run" || echo "aucun run CI pour $sha"
gh run watch "$run" --exit-status 2>&1 | tail -n 12
```

Expected: `docs` et `release-snapshot` verts (`check-dist.sh` : `ok hublot-kit_<V>_linux_amd64.tar.gz`, `…_arm64…`, `DIST OK : <V>`).

- [x] **Step 5: L'archive arm64 de la CI, sur les deux VM** — artefact du run de l'étape 4 (même commit), sommes contrôlées, Fedora en `Enforcing` exigé :

```bash
run=$(gh run list --workflow ci.yml --branch phase/lancement --commit "$(git rev-parse HEAD)" --status success --limit 1 --json databaseId --jq '.[0].databaseId')
test -n "$run" || echo "aucun run CI vert pour ce commit : pousser, attendre gh pr checks --watch"
snap=$(mktemp -d)
gh run download "$run" -n release-snapshot -D "$snap"
v=$(jq -r .version "$snap/metadata.json"); echo "$v"
(cd "$snap" && shasum -a 256 -c --ignore-missing checksums.txt)
limactl start hk-fedora >/dev/null 2>&1 || true
se=$(limactl shell hk-fedora -- getenforce | tr -d '\r'); echo "hk-fedora : $se"
[ "$se" = Enforcing ] || { echo "hk-fedora n'est pas en Enforcing : arrêter, SELinux n'est pas éprouvé" >&2; exit 1; }
for vm in hk-ubuntu hk-fedora; do
  limactl copy "$snap/hublot-kit_${v}_linux_arm64.tar.gz" "$vm:/tmp/"
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-systemd.sh "/tmp/hublot-kit_${v}_linux_arm64.tar.gz" "$v" 2>&1 | tail -n 5
  limactl shell "$vm" -- rm -f "/tmp/hublot-kit_${v}_linux_arm64.tar.gz"
done
limactl shell hk-ubuntu -- sudo journalctl -k --since -30min | grep -c 'apparmor="DENIED"'
limactl shell hk-fedora -- sudo ausearch -m avc -ts recent 2>&1 | tail -n 3
rm -rf "$snap"
```

Expected: `hk-fedora : Enforcing` ; sur chaque VM : `hublot-kit_<V>_linux_arm64.tar.gz: OK` (sommes), `ok édition (PUT fs/file)`, `ok envoi (POST fs/upload)`, sur `hk-fedora` `SELinux : …unconfined_service_t…`, puis `SYSTEMD OK : …` ; `0` refus AppArmor ; `<no matches>`.

Au registre : commit, run, résultats des deux VM, contexte SELinux, verdicts de revue. **Un correctif** : nouveau commit, puis de nouveau les étapes 4 et 5 sur ce commit.

- [x] **Step 6: Relecture (CLAUDE.md, « Orchestration »)** — `relecteur` puis Codex (lecture seule). Points : l'archive testée est celle du dernier commit, CI comprise (filtre `--commit`, sommes) ; le bloc B lit `.goreleaser.yaml` ; commandes de la doc identiques à celles de `check-systemd.sh`. La tranche C attend les deux avis.

---

### Tranche C : vérifier une release

**Où tester :** macOS (`actionlint`, étape rouge) ; le premier vrai passage est la pré-version (tâche 9).

**Critère d'acceptation (fixe) :** une release ne se publie que par le lancement manuel confirmé de `release.yml` sur un tag (job `guard`) ; elle est signée sans clé (archives, `checksums.txt`, image), porte des SBOM SPDX valides, et `scripts/verify-release.sh` la relit depuis une machine neuve ; la section « Verify the release » / « Vérifier la release » de la doc donne exactement les commandes, l'identité et l'émetteur que vérifie `verify-release.sh`, et dit ce que la signature couvre et ne couvre pas.

**Fichiers possédés (après la fusion de B) :** `.github/workflows/release.yml`, `scripts/verify-release.sh`, `.goreleaser.yaml` (sections `sboms`, `signs`, `docker_signs`, `release`, si un correctif l'exige), `scripts/check-docs.sh` (bloc « Tranche C »), et la section « Verify the release » / « Vérifier la release » de `docs/en/install.md` et `docs/fr/installation.md`.

**Fichiers lus (chez d'autres) :** `.goreleaser.yaml` hors signatures (noms d'archives, `checksum`, `dockers_v2` : tâche 6, tranches A et B), `Makefile` (`make web`, lancé par le job `web`), `scripts/check-dist.sh` (mêmes contrôles SBOM), les blocs A et B de `check-docs.sh`, `Dockerfile`.

**Files:**
- Create: `scripts/verify-release.sh`, `.github/workflows/release.yml`
- Modify: `scripts/check-docs.sh` (bloc « Tranche C »), section de vérification de la doc

**Interfaces:**
- Consumes: `.goreleaser.yaml` (tâche 6, sans hook `before`).
- Produces: un tag `v*` poussé ne publie rien ; `gh workflow run release.yml --ref <tag> -f confirm=<tag>` (job `guard` sans droits : ref = tag `vX.Y.Z[-rc.N]`, confirmation = tag, auteur = propriétaire ; Q54) produit : release GitHub **brouillon** (8 fichiers : 2 archives, 2 SBOM, `checksums.txt`, 3 bundles `.sigstore.json`), images `ghcr.io/levy-cmd/hublot-kit:<V>` (et `<X.Y>` hors pré-version) signées, avec attestation SBOM ; identité de signature `https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/<tag>`, émetteur `https://token.actions.githubusercontent.com`. `scripts/verify-release.sh TAG [ÉTIQUETTE…]`, lancé par le job `verify` et à la tâche 10.

> **À comprendre :** quatre jobs. `guard` (aucun droit) refuse tout lancement qui n'est pas le tag confirmé par le propriétaire : c'est le verrou technique (Q54 : un environnement avec relecteur n'est pas disponible sur ce dépôt privé). `web` (lecture seule) lance `make web`, donc `npm ci` et le build de l'interface, et passe `internal/web/dist` en artefact : **aucun code npm ne tourne dans le job qui signe et pousse** (d'où la fin du hook `before` de GoReleaser). Le job `release` a seul les droits d'écrire (`contents`, `packages`) et de signer (`id-token`) ; il récupère l'interface, sans Node ; aucun cache n'y entre (un cache empoisonné par une PR signerait un binaire piégé) ; les actions tierces sont épinglées par empreinte. Le job `verify` tourne sur une machine neuve et relit ce qui a été publié, comme le ferait un utilisateur. Lire une release **brouillon** demande le droit d'écriture sur le dépôt, d'où `contents: write` dans `verify`.

- [x] **Step 1: Écrire les tests** — le bloc « Tranche C » de `scripts/check-docs.sh` (après le bloc B), puis le script de vérification.

```bash
# Tranche C (plan Lancement) : les commandes de vérification de la doc
# visent ce que signent .goreleaser.yaml et release.yml, avec l'identité que
# vérifie scripts/verify-release.sh.
wf=.github/workflows/release.yml
[ -f "$wf" ] || err "$wf absent"
grep -qx 'name_template: checksums.txt' <(sed 's/^ *//' "$gr") || err "$gr : checksums.txt renommé"
[ "$(grep -c 'signature: "${artifact}.sigstore.json"' "$gr")" = 2 ] || err "$gr : bundles .sigstore.json (checksums et archives) attendus"
grep -qF 'identity="https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag"' scripts/verify-release.sh ||
  err "scripts/verify-release.sh : identité différente de celle de la doc"
for f in docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  grep -qF -- '--certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V"' "$f" ||
    err "$f : identité de signature des archives"
  grep -qF -- "--certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+\$'" "$f" ||
    err "$f : identité de signature de l'image"
  grep -qF -- '--certificate-oidc-issuer https://token.actions.githubusercontent.com' "$f" || err "$f : émetteur OIDC"
  grep -qF 'checksums.txt.sigstore.json' "$f" || err "$f : bundle de checksums.txt"
done
```

`scripts/verify-release.sh` :

```bash
#!/usr/bin/env bash
# Vérifie une release faite par .github/workflows/release.yml, brouillon
# compris (plan Lancement) : bundles cosign keyless de checksums.txt et des
# archives (identité : ce workflow, sur ce tag), sommes de contrôle, SBOM des
# archives (SPDX JSON bien formé, module de l'agent présent, empreinte dans
# checksums.txt signé), signature de l'image, plateformes, attestation SBOM de
# chaque plateforme (validée de même).
#
# Ce que les signatures couvrent : les archives et leurs SBOM (par
# checksums.txt signé, et chaque archive par son propre bundle), et l'index de
# l'image, qui désigne par empreinte les images de chaque plateforme et leurs
# attestations SBOM. Elles prouvent que ces fichiers sortent du workflow de
# release de ce dépôt, lancé sur ce tag, et n'ont pas changé depuis.
# Ce qu'elles ne couvrent pas : la relecture du code du tag ; l'exactitude et
# l'exhaustivité des SBOM (ce que syft et BuildKit ont trouvé dans le binaire) ;
# le texte de la page de release ; les bundles .sigstore.json eux-mêmes (absents
# de checksums.txt : chacun porte son certificat, vérifié par cosign) ; aucune
# provenance SLSA n'est publiée en v0.1.
#
# Usage : scripts/verify-release.sh vX.Y.Z[-rc.N] [IMAGE_TAG_EN_PLUS ...]
#   scripts/verify-release.sh v0.1.0-rc.1
#   scripts/verify-release.sh v0.1.0 0.1       (vérifie aussi ghcr.io/…:0.1)
# Prérequis : gh authentifié (lecture d'un brouillon : droit d'écriture sur le
# dépôt), cosign 3.x, jq, docker avec buildx, accès en lecture au paquet GHCR.
set -euo pipefail
tag=${1:?usage : scripts/verify-release.sh vX.Y.Z[-rc.N] [TAG_IMAGE ...]}
shift
v=${tag#v}
repo=levy-cmd/Hublot-Kit
image=ghcr.io/levy-cmd/hublot-kit
module=github.com/levy-cmd/hublot-kit
identity="https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag"
issuer=https://token.actions.githubusercontent.com
fail() { echo "ÉCHEC : $*" >&2; exit 1; }
blob() { cosign verify-blob --bundle "$1.sigstore.json" --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$1"; }
# spdx_ok : lit un document SPDX JSON sur l'entrée standard.
spdx_ok() {
  jq -e --arg m "$module" '(.spdxVersion | startswith("SPDX-")) and (.packages | length > 0)
    and any(.packages[]; .name == $m)' >/dev/null
}

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
gh release download "$tag" -R "$repo" -D "$dir"
cd "$dir"
blob checksums.txt
sha256sum -c --quiet checksums.txt || fail "checksums.txt"
for arch in amd64 arm64; do
  a=hublot-kit_${v}_linux_${arch}.tar.gz
  blob "$a"
  grep -q "  $a.sbom.json\$" checksums.txt || fail "$a.sbom.json absent de checksums.txt signé"
  spdx_ok < "$a.sbom.json" || fail "$a.sbom.json : SPDX JSON invalide ou sans $module"
  echo "ok $a"
done

for t in "$v" "$@"; do
  cosign verify --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$image:$t" >/dev/null ||
    fail "signature de $image:$t"
  raw=$(docker buildx imagetools inspect --raw "$image:$t")
  sbom=$(docker buildx imagetools inspect "$image:$t" --format '{{ json .SBOM }}')
  for p in amd64 arm64; do
    jq -e --arg a "$p" 'any(.manifests[]; .platform.os == "linux" and .platform.architecture == $a)' <<<"$raw" >/dev/null ||
      fail "$image:$t sans linux/$p"
    jq --arg p "linux/$p" '.[$p].SPDX' <<<"$sbom" | spdx_ok || fail "$image:$t : attestation SBOM linux/$p invalide ou sans $module"
  done
  echo "ok $image:$t"
done
echo "RELEASE OK : $tag"
```

- [x] **Step 2: Les voir échouer** — juste après le bloc C, **avant** d'écrire `verify-release.sh` : `scripts/check-docs.sh 2>&1 | grep ERREUR` → `ERREUR : .github/workflows/release.yml absent` et `ERREUR : scripts/verify-release.sh : identité différente de celle de la doc` (le script n'existe pas encore ; `grep` dit aussi `No such file or directory`). Une fois `verify-release.sh` écrit, la seconde erreur disparaît ; la première reste jusqu'à l'étape 3. Et aucune release n'existe :

Run: `chmod +x scripts/verify-release.sh && bash -n scripts/verify-release.sh && { scripts/verify-release.sh v0.0.0-absent 2>&1 | tail -n 2; }; scripts/verify-release.sh v0.0.0-absent >/dev/null 2>&1 || echo "échec attendu"`
Expected: `release not found`, puis `échec attendu`.

- [x] **Step 3: Écrire le workflow** — `.github/workflows/release.yml` :

```yaml
name: Release

# Plan Lancement. Pousser un tag ne publie rien : la release se lance à la main,
# par l'utilisateur, sur ce tag, en retapant son nom (verrou technique, Q54) :
#   gh workflow run release.yml --ref v0.1.0 -f confirm=v0.1.0
# guard vérifie la ref, la confirmation et l'auteur, sans aucun droit ; web
# construit l'interface sans droit d'écriture ; release construit, signe
# (cosign keyless) et publie : archives, checksums, SBOM, image multi-arch sur
# GHCR, et une release GitHub en BROUILLON ; verify relit ce qui a été publié
# depuis une machine neuve. La publication du brouillon reste manuelle.
# GitHub n'enregistre un workflow_dispatch que si ce fichier est sur la branche
# par défaut (main).

on:
  workflow_dispatch:
    inputs:
      confirm:
        description: "Tag to release, typed again (must equal the ref, e.g. v0.1.0)"
        required: true
        type: string

permissions: {}

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

jobs:
  guard:
    runs-on: ubuntu-latest
    permissions: {}
    steps:
      # Une relance (« Re-run failed jobs ») par un autre compte garde l'auteur
      # d'origine dans github.actor : triggering_actor est vérifié aussi.
      - name: La ref est le tag confirmé, lancé par le propriétaire
        env:
          REF_TYPE: ${{ github.ref_type }}
          REF_NAME: ${{ github.ref_name }}
          CONFIRM: ${{ inputs.confirm }}
          ACTOR: ${{ github.actor }}
          TRIGGER: ${{ github.triggering_actor }}
          OWNER: ${{ github.repository_owner }}
        run: |
          [ "$REF_TYPE" = tag ] || { echo "à lancer sur un tag (--ref vX.Y.Z), pas sur $REF_TYPE $REF_NAME" >&2; exit 1; }
          [ "$CONFIRM" = "$REF_NAME" ] || { echo "confirmation « $CONFIRM » différente du tag « $REF_NAME »" >&2; exit 1; }
          echo "$REF_NAME" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$' || { echo "tag inattendu : $REF_NAME" >&2; exit 1; }
          [ "$ACTOR" = "$OWNER" ] || { echo "lancé par $ACTOR, pas par $OWNER" >&2; exit 1; }
          [ "$TRIGGER" = "$OWNER" ] || { echo "relancé par $TRIGGER, pas par $OWNER" >&2; exit 1; }

  # npm ne tourne jamais là où l'on signe et pousse.
  web:
    needs: guard
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - uses: actions/setup-node@v7
        with:
          node-version: 24
      - run: make web
      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2
        with:
          name: web-dist
          path: internal/web/dist/
          if-no-files-found: error
          retention-days: 1

  release:
    needs: [guard, web]
    runs-on: ubuntu-latest
    permissions:
      contents: write # créer la release (brouillon) et y déposer les fichiers
      packages: write # pousser l'image sur ghcr.io/levy-cmd/hublot-kit
      id-token: write # cosign keyless : jeton OIDC de GitHub Actions
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
        with:
          name: web-dist
          path: internal/web/dist
      - run: test -f internal/web/dist/index.html
      # Dernier correctif de Go 1.27 ; aucun cache dans ce job (empoisonnement de cache).
      - uses: actions/setup-go@v7
        with:
          go-version: "1.27.x"
          check-latest: true
          cache: false
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.3
      - uses: anchore/sbom-action/download-syft@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
      - uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7.3.0
        with:
          distribution: goreleaser
          version: "~> v2.18"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  verify:
    needs: release
    runs-on: ubuntu-latest
    permissions:
      contents: write # lire une release encore en brouillon (réservé aux droits d'écriture)
      packages: read
    steps:
      - uses: actions/checkout@v7
        with:
          persist-credentials: false
      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.3
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
      - uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      # Une release finale est relue aussi sous son étiquette X.Y ; une pré-version, sous la sienne seulement.
      - run: |
          extra=
          case "$GITHUB_REF_NAME" in
            *-*) ;;
            *) v=${GITHUB_REF_NAME#v}; extra=${v%.*} ;;
          esac
          scripts/verify-release.sh "$GITHUB_REF_NAME" $extra
        env:
          GH_TOKEN: ${{ github.token }}
```

- [x] **Step 4: Le contrôler**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 2>&1 | tail -n 5`
Expected: aucune sortie (vérifié à la rédaction).

Run: `grep -nE 'uses: (docker|sigstore|anchore|goreleaser)/|uses: actions/(upload|download)-artifact' .github/workflows/*.yml | grep -vE '@[0-9a-f]{40} # v' ; echo "fin"`
Expected: seulement `fin` (toute action tierce, et les artefacts, épinglés par empreinte).

Run: `grep -c 'npm\|make web\|setup-node' <(awk '/^  release:/,/^  verify:/' .github/workflows/release.yml)`
Expected: `0` (rien de Node dans le job qui signe).

Le garde, rejoué en local avec le script exact du workflow :

```bash
guard=$(awk '/- name: La ref est le tag confirmé/{f=1} f&&/run: \|/{r=1;next} r&&!/^          /{exit} r{sub(/^          /,""); print}' .github/workflows/release.yml)
g() { env REF_TYPE="$1" REF_NAME="$2" CONFIRM="$3" ACTOR="$4" TRIGGER="$5" OWNER=levy-cmd bash -c "$guard" >/dev/null 2>&1 && echo "accepte $*" || echo "refuse  $*"; }
g tag v0.1.0-rc.1 v0.1.0-rc.1 levy-cmd levy-cmd
g tag v0.1.0-rc.1 v0.1.0-rc.9 levy-cmd levy-cmd
g branch phase/lancement phase/lancement levy-cmd levy-cmd
g tag v0.1.0-rc.1 v0.1.0-rc.1 levy-cmd autre
g tag v0.1.0-rc.1 v0.1.0-rc.1 autre levy-cmd
g tag v0.1 v0.1 levy-cmd levy-cmd
```

Expected (vérifié à la rédaction) : `accepte` pour la première ligne seulement ; `refuse` pour la confirmation fausse, la branche, la relance par un autre compte (`triggering_actor`), un autre auteur, et un tag `v0.1`.

- [x] **Step 5: La doc de vérification** — relire « Verify the release » / « Vérifier la release » (EN et FR) contre `verify-release.sh` et `release.yml` : commandes `cosign verify-blob` et `cosign verify`, identité (`release.yml@refs/tags/v$V`, expression régulière pour `:0.1`), émetteur, ce que `checksums.txt` liste (archives et SBOM, pas les bundles), ce que la signature couvre et ne couvre pas (en-tête de `verify-release.sh`). Corriger s'il le faut.

Run: `scripts/check-docs.sh | tail -n 1`
Expected: `DOCS OK`. Mutation (vérifiée à la rédaction, puis annulée) : identité de `verify-release.sh` en `refs/heads/` → `ERREUR : scripts/verify-release.sh : identité différente de celle de la doc`.

- [x] **Step 6: Commit**

```bash
git add scripts/verify-release.sh .github/workflows/release.yml scripts/check-docs.sh docs/en/install.md docs/fr/installation.md
git commit -F - <<'EOF'
release : workflow signé (cosign sans clé), release en brouillon et vérification

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git push && gh pr checks --watch 2>&1 | tail -n 12
```

Expected: tous les jobs verts (`release.yml` ne tourne pas sur une PR).

- [x] **Step 7: Relecture (CLAUDE.md, « Orchestration »)** — `relecteur` puis Codex (lecture seule). Points : le garde (ref, confirmation, auteur) avant tout job qui a des droits ; aucun code npm dans le job qui signe ; permissions minimales par job ; `persist-credentials: false` ; pas de cache ; épinglage ; identité et émetteur de `verify-release.sh` identiques à ceux de la doc ; le brouillon ; `concurrency` sans annulation ; ce qui se passe si `verify` échoue (la release reste brouillon, l'image privée reste poussée). Corrections en commits séparés ; la tâche 8 (spec et audit) attend les deux avis.

---

### Task 8: Spec et audit avant publication (reste de l'ancienne tâche 8)

**Où tester :** macOS ; `hk-ubuntu` pour `gitleaks`. Les vérifications VM de l'image et de l'archive de la CI sont passées aux tranches A et B.

**Fichiers possédés :** `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md`, ce plan (cases, constats).

**Files:**
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§8, §10, §13), `docs/superpowers/plans/2026-10-13-lancement.md` (cases cochées, constats)

- [x] **Step 1: Reporter dans la spec** — remplacer chaque passage par le suivant (chacun est unique dans la spec ; vérifier par `grep -c`).

1. §8, alternative — remplacer :

   ```text
   **Alternative** : binaire des releases GitHub + service systemd, lancé en root. Binaire lancé en root hors conteneur : vérifié sur Ubuntu et Fedora ; service systemd non testé.
   ```

   par :

   ```text
   **Alternative** : binaire des releases GitHub + service systemd, lancé en root. L'archive (`hublot-kit_<version>_linux_<arch>.tar.gz`) contient l'unité `hublot-kit.service` : `/usr/local/bin/hublot`, root, `127.0.0.1:7878`, `StateDirectory=hublot-kit` en 0700, `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp` ; ni `ProtectProc`, ni `PrivateUsers`, ni réduction des capacités, que l'explorateur exige. Le jeton se crée par `hublot reset-token` avant le premier démarrage, pour ne jamais passer par le journal du système. Binaire et unité vérifiés sur Ubuntu (AppArmor) et Fedora (SELinux `enforcing`, l'agent tourne en `unconfined_service_t`) par `scripts/check-systemd.sh` (plan Lancement).
   ```

2. §8, image et release — remplacer :

   ```text
   **Prérequis hôte** :
   ```

   par :

   ```text
   **Image et release** : `ghcr.io/levy-cmd/hublot-kit`, `FROM scratch` (le binaire seul, root, `HUBLOT_LISTEN=0.0.0.0:7878`, `HUBLOT_DATA_DIR=/data`), linux/amd64 et linux/arm64 ; étiquettes `X.Y.Z` et `X.Y`, jamais `latest`, une pré-version n'a que la sienne. Archives, `checksums.txt` et image signés par cosign sans clé (identité : `.github/workflows/release.yml` sur le tag) ; un SBOM SPDX par archive (syft) et une attestation SBOM sur l'image. `hublot --version` affiche la version. La commande ci-dessus est vérifiée telle quelle par `scripts/check-image.sh`, et la doc la reprend à l'octet près (`scripts/check-docs.sh`).

   **Prérequis hôte** :
   ```

3. §10, CI — remplacer :

   ```text
   - **CI** : `go vet`, `go test ./...`, `npm run check`, Vitest, Playwright, build multi-architecture.
   ```

   par :

   ```text
   - **CI** : `go vet`, `go test ./...`, `npm run check`, Vitest, Playwright, `scripts/check-docs.sh` (job `docs`), et la chaîne de release sans rien publier (job `release-snapshot` : GoReleaser en snapshot, `scripts/check-dist.sh`, image multi-architecture poussée dans un registre local éphémère avec son SBOM, image amd64 lancée avec la commande de §8 par `scripts/check-image.sh`). Une release est lancée à la main sur un tag `v*` (`workflow_dispatch`, garde : tag confirmé, propriétaire), puis construite, signée et relue par `.github/workflows/release.yml` (`scripts/verify-release.sh`), en brouillon ; l'interface est construite dans un job sans droits.
   ```

4. §13, première ligne — remplacer :

   ```text
   Repli : binaire lancé en root hors conteneur (vérifié sur Ubuntu et Fedora ; service systemd non testé)
   ```

   par :

   ```text
   Repli : binaire lancé en root hors conteneur, par l'unité systemd de la release (vérifiés sur Ubuntu et Fedora, plan Lancement)
   ```

Run: `scripts/check-docs.sh | tail -n 1 && grep -c 'service systemd non testé' docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md`
Expected: `DOCS OK` (la commande de §8 n'a pas bougé), puis `1` (il ne reste que le résultat historique du spike, §11).

- [x] **Step 2: Audit avant publication** (Q47) — rien n'est modifié ici ; les constats vont à l'utilisateur.

```bash
limactl shell --workdir "$PWD" hk-ubuntu -- sudo docker run --rm \
  -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' \
  -v "$PWD:/repo:ro" ghcr.io/gitleaks/gitleaks:v8.30.1 git /repo --no-banner 2>&1 | tail -n 15
limactl shell hk-ubuntu -- sudo docker rmi ghcr.io/gitleaks/gitleaks:v8.30.1 >/dev/null
git ls-files .claude
git log --all --format='%an <%ae>' | sort -u
git log --all --oneline -- '*.pdf' '.superpowers' | head -n 3
```

Expected: `no leaks found` (sinon, lister chaque fuite à l'utilisateur, sans rien réécrire) ; les quatre fichiers `.claude/handoff/*` (à relire : ils deviennent publics) ; la liste des auteurs et adresses (elles deviennent publiques) ; aucun PDF ni `.superpowers` dans l'historique.

- [ ] **Step 3: Revue de branche** — demander la revue complète de `phase/lancement` (sous-agent indépendant + Codex) contre ce plan et la spec ; corriger ; cocher les cases ; noter sous « Constats de la tâche 8 » : verdicts de la revue, résultat de l'audit (les résultats VM sont au registre, tranches A et B).

- [x] **Step 4: Commit**

```bash
git add docs/superpowers
git commit -F - <<'EOF'
docs : Lancement, spec §8, §10 et §13 à jour, audit avant publication

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git push && gh pr checks --watch 2>&1 | tail -n 12
```

- [ ] **Step 5: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex sur le commit de la tâche (remplacements de la spec fidèles au livré, constats exacts). La tâche 9 attend les deux avis **et** la réponse de l'utilisateur aux constats de l'audit.

---

### Task 9: Pré-version `v0.1.0-rc.1` — toute la chaîne, rien de public

**Où tester :** GitHub Actions, GHCR (privé), `hk-ubuntu`, `hk-fedora`.

**Files:**
- Modify: `docs/superpowers/plans/2026-10-13-lancement.md` (constats)

> **À comprendre :** c'est le premier passage de la signature, de GHCR et du brouillon. Le tag se pose sur le dernier commit de `phase/lancement` (Q46). Un tag poussé ne bouge plus : si la chaîne échoue, on corrige sur la branche et on recommence avec `v0.1.0-rc.2` (nouvel ARRÊT).

- [x] **Step 0: `release.yml` sur `main`** (Q54) — GitHub n'enregistre un `workflow_dispatch` que si le fichier est sur la branche par défaut. Une petite PR qui ne contient que ce fichier (pas une étape ARRÊT ; l'utilisateur la fusionne) :

```bash
git fetch origin && git switch -c ci/release-dispatch origin/main
git checkout phase/lancement -- .github/workflows/release.yml
git commit -F - <<'EOF'
ci : workflow de release, lancement manuel sur un tag confirmé

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git push -u origin ci/release-dispatch
gh pr create --base main --title "ci : workflow de release (lancement manuel)" --body "Requis pour workflow_dispatch (plan Lancement, tâche 9, Q54). Le job guard refuse toute ref qui n'est pas un tag vX.Y.Z[-rc.N] confirmé par son propriétaire ; les scripts et .goreleaser.yaml viennent du tag lancé."
```

Après la fusion : `gh workflow list | grep Release` → `Release  active`. Le fichier lancé est toujours celui du tag (`--ref`), pas celui de `main`.

- [x] **Step 1: ARRÊT : accord de l'utilisateur requis — pousser le tag `v0.1.0-rc.1`.** Pousser le tag ne publie rien (Q54) : il rend le tag visible dans le dépôt (privé). Après le « oui » explicite :

```bash
git switch phase/lancement && git pull --ff-only
gh pr checks 2>&1 | tail -n 12                    # tout vert avant de taguer
git tag -a v0.1.0-rc.1 -m "Hublot-Kit v0.1.0-rc.1"
git push origin v0.1.0-rc.1
```

- [x] **Step 2: Le verrou refuse les mauvais lancements** — d'abord le garde en local (même script que la tranche C, étape 4), puis deux vrais lancements refusés sur GitHub, chacun attendu jusqu'au bout et contrôlé : conclusion `failure`, et **seul le job `guard` a tourné** (`web`, `release`, `verify` sautés). Si un contrôle échoue, tout s'arrête : pas d'étape 3, et l'utilisateur est prévenu (un lancement non refusé a pu publier : vérifier `gh release list` et les versions du paquet GHCR).

```bash
guard=$(awk '/- name: La ref est le tag confirmé/{f=1} f&&/run: \|/{r=1;next} r&&!/^          /{exit} r{sub(/^          /,""); print}' .github/workflows/release.yml)
g() { env REF_TYPE="$1" REF_NAME="$2" CONFIRM="$3" ACTOR="$4" TRIGGER="$5" OWNER=levy-cmd bash -c "$guard" >/dev/null 2>&1 && echo "accepte $*" || echo "refuse  $*"; }
g tag v0.1.0-rc.1 v0.1.0-rc.1 levy-cmd levy-cmd
g tag v0.1.0-rc.1 v0.1.0-rc.9 levy-cmd levy-cmd
g branch phase/lancement phase/lancement levy-cmd levy-cmd
g tag v0.1.0-rc.1 v0.1.0-rc.1 levy-cmd autre
g tag v0.1.0-rc.1 v0.1.0-rc.1 autre levy-cmd
g tag v0.1 v0.1 levy-cmd levy-cmd
# hk_bad_run REF CONFIRMATION : lance release.yml, attend la fin du run, exige
# l'échec du seul garde.
hk_bad_run() {
  since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  gh workflow run release.yml --ref "$1" -f confirm="$2" || return 1
  id=
  for _ in $(seq 1 30); do
    id=$(gh run list --workflow release.yml --event workflow_dispatch --branch "$1" --limit 5 \
      --json databaseId,createdAt --jq "[.[] | select(.createdAt >= \"$since\")][0].databaseId // empty")
    [ -n "$id" ] && break
    sleep 2
  done
  [ -n "$id" ] || { echo "run introuvable pour $1" >&2; return 1; }
  if gh run watch "$id" --exit-status >/dev/null 2>&1; then echo "LE VERROU N'A PAS REFUSÉ $1 (run $id)" >&2; return 1; fi
  concl=$(gh run view "$id" --json conclusion --jq .conclusion)
  ran=$(gh run view "$id" --json jobs --jq '[.jobs[] | select(.conclusion != "skipped") | .name] | join(",")')
  if [ "$concl" != failure ] || [ "$ran" != guard ]; then echo "run $id : $concl, jobs lancés : $ran" >&2; return 1; fi
  echo "ok refus de $1 (run $id : seul guard a tourné, en échec)"
}
hk_bad_run v0.1.0-rc.1 v0.1.0-rc.9 && hk_bad_run phase/lancement phase/lancement && echo "VERROU OK"
gh release list | head -n 3
```

Expected: le tableau local comme à la tranche C, étape 4 ; `ok refus de v0.1.0-rc.1 …`, `ok refus de phase/lancement …`, `VERROU OK` ; `gh release list` vide.

- [x] **Step 3: ARRÊT : accord de l'utilisateur requis — lancer `release.yml` sur `v0.1.0-rc.1`**, seulement si l'étape 2 a affiché `VERROU OK`. Rappeler ce que cela fait : **l'image `ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1` est poussée sur GHCR** (paquet créé privé ; ARRÊT « pousser l'image sur GHCR » compris dans celui-ci), une release **brouillon** est créée, et des entrées **permanentes et publiques** sont inscrites dans le journal Rekor (nom du dépôt, chemin du workflow, tag), même dépôt privé. Après le « oui » explicite :

```bash
gh workflow run release.yml --ref v0.1.0-rc.1 -f confirm=v0.1.0-rc.1
sleep 10; gh run watch "$(gh run list --workflow release.yml --event workflow_dispatch --branch v0.1.0-rc.1 --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status 2>&1 | tail -n 15
```

Expected: `guard`, `web`, `release` puis `verify` verts ; le journal de `verify` finit par `RELEASE OK : v0.1.0-rc.1`. En cas d'échec : `gh run view --log-failed | tail -n 60`, corriger sur la branche (commit `correctif : …`, relecture), puis ARRÊT pour `v0.1.0-rc.2` (étapes 1 à 3 à nouveau). Si `verify` échoue alors que le paquet GHCR est déjà public (toute release lancée après la tâche 10, étape 7), l'image de la version refusée est tirable par tous : **ARRÊT : accord de l'utilisateur requis — supprimer cette version de GHCR** (suppression définitive ; interface web du paquet, ou `gh api -X DELETE /user/packages/container/hublot-kit/versions/<id>` avec le droit `delete:packages`), puis vérifier que `docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:<V>` échoue. Le brouillon de release, lui, reste privé.

- [ ] **Step 4: Contrôler ce qui a été créé** — le droit `read:packages` manque d'ordinaire au jeton de `gh` : `gh auth refresh -h github.com -s read:packages` (l'utilisateur valide dans son navigateur).

```bash
gh release view v0.1.0-rc.1 --json isDraft,isPrerelease,assets --jq '{isDraft, isPrerelease, assets: [.assets[].name]}'
gh api /users/levy-cmd/packages/container/hublot-kit --jq .visibility
gh api /users/levy-cmd/packages/container/hublot-kit/versions --jq '.[].metadata.container.tags'
```

Expected: `isDraft: true`, `isPrerelease: true`, 8 fichiers (`checksums.txt`, `checksums.txt.sigstore.json`, et pour amd64 et arm64 : l'archive, son `.sbom.json`, son `.sigstore.json`) ; `private` ; une seule étiquette, `["0.1.0-rc.1"]` (ni `0.1`, ni `latest`) — les versions sans étiquette sont les manifestes de plateforme, d'attestation et de signature.

- [ ] **Step 5: L'image rc, avec la commande de la doc, sur les deux VM** — connexion à GHCR en lecture, retirée à la fin :

```bash
rc=$(mktemp -d)
gh release download v0.1.0-rc.1 -p 'hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz' -D "$rc"
for vm in hk-ubuntu hk-fedora; do
  gh auth token | limactl shell "$vm" -- sudo docker login ghcr.io -u levy-cmd --password-stdin >/dev/null
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 0.1.0-rc.1 2>&1 | tail -n 3
  limactl copy "$rc/hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz" "$vm:/tmp/"
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-systemd.sh /tmp/hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz 0.1.0-rc.1 2>&1 | tail -n 2
  limactl shell "$vm" -- sh -c 'sudo docker rmi -f ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 >/dev/null; rm -f /tmp/hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz'
done
```

Expected: `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1` et `SYSTEMD OK : …` sur les deux VM.

- [ ] **Step 6: La signature et le SBOM vérifiés hors de GitHub** — cosign dans `hk-ubuntu` (binaire de la release de sigstore, contrôlé par sa somme) ; la connexion GHCR de l'étape 5 sert encore :

```bash
limactl shell hk-ubuntu -- sh -c 'cd /tmp && curl -fsSLO https://github.com/sigstore/cosign/releases/download/v3.1.3/cosign-linux-arm64 && curl -fsSL https://github.com/sigstore/cosign/releases/download/v3.1.3/cosign_checksums.txt | grep " cosign-linux-arm64$" | sha256sum -c - && sudo install -m 0755 cosign-linux-arm64 /usr/local/bin/cosign && rm cosign-linux-arm64'
limactl shell hk-ubuntu -- sudo cosign verify \
  --certificate-identity https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v0.1.0-rc.1 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 2>&1 | tail -n 4
limactl shell hk-ubuntu -- sudo docker buildx imagetools inspect ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 --format '{{ json .SBOM }}' | grep -o '"linux/[a-z0-9]*"' | sort -u
for f in checksums.txt checksums.txt.sigstore.json; do gh release download v0.1.0-rc.1 -p "$f" -D "$rc" --clobber; done
limactl copy "$rc/checksums.txt" "$rc/checksums.txt.sigstore.json" "$rc/hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz" hk-ubuntu:/tmp/
limactl shell hk-ubuntu -- sh -c 'cd /tmp && cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v0.1.0-rc.1 --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt && sha256sum --ignore-missing -c checksums.txt; rm -f checksums.txt* hublot-kit_*'
for vm in hk-ubuntu hk-fedora; do limactl shell "$vm" -- sudo docker logout ghcr.io >/dev/null; done
rm -rf "$rc"
```

Expected: `Verification for ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 --` suivi de `The cosign claims were validated` et du JSON ; `"linux/amd64"` et `"linux/arm64"` ; `Verified OK` puis `hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz: OK`. Une signature faite avec une autre identité doit échouer : relancer `cosign verify` avec `@refs/tags/v0.1.0-rc.2` → `none of the expected identities matched`.

- [ ] **Step 7: Constats, commit, puis prêt pour la fusion**

Noter sous « Constats de la tâche 9 » : durée du workflow, fichiers et étiquettes, version de cosign et format des signatures (bundle, référents OCI ou étiquette `sha256-…`), résultat sur chaque VM, tout correctif et toute rc supplémentaire.

```bash
git add docs/superpowers/plans/2026-10-13-lancement.md
git commit -F - <<'EOF'
docs : constats de la pré-version v0.1.0-rc.1

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git push && gh pr ready && gh pr checks --watch 2>&1 | tail -n 12
```

- [ ] **Step 8: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex sur les constats et tout correctif de la tâche. Ensuite, l'utilisateur fusionne la PR (pas l'agent).

---

### Task 10: v0.1.0 publique

**Où tester :** GitHub, GHCR, `hk-ubuntu`, `hk-fedora`, sans aucune connexion (vérification anonyme).

**Files:**
- Modify: `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md` (§12), `docs/superpowers/plans/2026-10-13-lancement.md` (constats), par une petite PR sur `main`

- [ ] **Step 1: La PR est fusionnée** — `gh pr view phase/lancement --json state --jq .state` → `MERGED` (fusion faite par l'utilisateur ; la branche est gardée, règle du dépôt). Puis :

```bash
git fetch origin && git switch main && git pull --ff-only
gh run list --workflow ci.yml --branch main --limit 1 --json conclusion --jq '.[0].conclusion'   # attendu : success
```

- [ ] **Step 2: ARRÊT : accord de l'utilisateur requis — pousser le tag `v0.1.0`** sur le commit de fusion de `main`, **puis lancer `release.yml` dessus** (le verrou a été éprouvé à la tâche 9, étape 2, et `release.yml` n'a pas changé depuis : `git diff v0.1.0-rc.1 v0.1.0 -- .github/workflows/release.yml` vide, sinon refaire l'étape 2 de la tâche 9 sur `v0.1.0` avant de lancer) : **images `0.1.0` et `0.1` poussées sur GHCR** (paquet encore privé), release **brouillon**, entrées Rekor. Après le « oui » explicite :

```bash
git tag -a v0.1.0 -m "Hublot-Kit v0.1.0"
git push origin v0.1.0
gh workflow run release.yml --ref v0.1.0 -f confirm=v0.1.0
sleep 10; gh run watch "$(gh run list --workflow release.yml --event workflow_dispatch --branch v0.1.0 --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status 2>&1 | tail -n 15
```

Expected: `guard`, `web`, `release` et `verify` verts, `RELEASE OK : v0.1.0`.

- [ ] **Step 3: Étiquettes et brouillon**

```bash
gh api /users/levy-cmd/packages/container/hublot-kit/versions --jq '.[] | select(.metadata.container.tags | length > 0) | .metadata.container.tags'
gh release view v0.1.0 --json isDraft,isPrerelease --jq '{isDraft, isPrerelease}'
```

Expected: `["0.1.0","0.1"]` sur une même version (même empreinte), `["0.1.0-rc.1"]` sur une autre ; `isDraft: true`, `isPrerelease: false`. Le job `verify` a relu les deux étiquettes : `gh run view "$(gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')" --log | grep -E 'ok ghcr.io'` → `ok ghcr.io/levy-cmd/hublot-kit:0.1.0` et `ok ghcr.io/levy-cmd/hublot-kit:0.1`.

- [ ] **Step 4: La commande de la doc, inchangée, sur les deux VM** (connexion GHCR en lecture, comme à la tâche 9 ; `check-image.sh` sans remplacement : `:0.1` est la référence de la spec)

```bash
for vm in hk-ubuntu hk-fedora; do
  gh auth token | limactl shell "$vm" -- sudo docker login ghcr.io -u levy-cmd --password-stdin >/dev/null
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1 0.1.0 2>&1 | tail -n 3
  limactl shell "$vm" -- sh -c 'sudo docker rmi -f ghcr.io/levy-cmd/hublot-kit:0.1 >/dev/null; sudo docker logout ghcr.io >/dev/null'
done
```

Expected: `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:0.1` sur les deux VM.

- [ ] **Step 5: ARRÊT : accord de l'utilisateur requis — rendre le dépôt public**, après réponse aux constats de l'audit (tâche 8, étape 2). Rappeler : tout l'historique, les plans, `.claude/handoff/*` et les adresses e-mail des commits deviennent publics. Après le « oui » explicite :

```bash
gh repo edit levy-cmd/Hublot-Kit --visibility public --accept-visibility-change-consequences
gh api -X PUT repos/levy-cmd/Hublot-Kit/private-vulnerability-reporting
gh repo view levy-cmd/Hublot-Kit --json visibility --jq .visibility
```

Expected: `PUBLIC`.

- [ ] **Step 6: ARRÊT : accord de l'utilisateur requis — publier la release `v0.1.0`.** Après le « oui » explicite :

```bash
gh release edit v0.1.0 --draft=false --latest
gh release view v0.1.0 --json isDraft,url --jq '{isDraft, url}'
```

Expected: `isDraft: false` et l'URL de la release.

- [ ] **Step 7: ARRÊT : accord de l'utilisateur requis — rendre public le paquet GHCR.** Pas d'API : l'utilisateur le fait dans l'interface (https://github.com/users/levy-cmd/packages/container/hublot-kit/settings → « Danger Zone » → « Change visibility » → « Public »). Vérifier ensuite :

Run: `gh api /users/levy-cmd/packages/container/hublot-kit --jq .visibility`
Expected: `public`.

- [ ] **Step 8: Vérification anonyme, comme un inconnu** — aucune connexion ni jeton, commandes de la doc telles quelles :

```bash
curl -fsSI https://github.com/levy-cmd/Hublot-Kit/releases/download/v0.1.0/checksums.txt | head -n 1
for vm in hk-ubuntu hk-fedora; do
  limactl shell "$vm" -- sudo docker logout ghcr.io >/dev/null 2>&1
  limactl shell --workdir "$PWD" "$vm" -- sudo scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1 0.1.0 2>&1 | tail -n 2
done
limactl shell hk-ubuntu -- cosign verify \
  --certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v0\.1\.[0-9]+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/levy-cmd/hublot-kit:0.1 2>&1 | head -n 3
limactl shell hk-ubuntu -- sh -c 'd=$(mktemp -d) && cd "$d" && V=0.1.0 && ARCH=arm64 && BASE="https://github.com/levy-cmd/Hublot-Kit/releases/download/v$V" && curl -fLO "$BASE/hublot-kit_${V}_linux_${ARCH}.tar.gz" && curl -fLO "$BASE/checksums.txt" && curl -fLO "$BASE/checksums.txt.sigstore.json" && cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V" --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt && sha256sum --ignore-missing -c checksums.txt; cd / && rm -rf "$d"'
limactl shell hk-ubuntu -- sh -c 'sudo docker rmi -f ghcr.io/levy-cmd/hublot-kit:0.1 >/dev/null; sudo rm -f /usr/local/bin/cosign'
limactl shell hk-fedora -- sudo docker rmi -f ghcr.io/levy-cmd/hublot-kit:0.1 >/dev/null
```

Expected: `HTTP/2 302` ; `IMAGE OK : ghcr.io/levy-cmd/hublot-kit:0.1` sur les deux VM (tirage anonyme) ; `Verification for ghcr.io/levy-cmd/hublot-kit:0.1 --` ; `Verified OK` puis `hublot-kit_0.1.0_linux_arm64.tar.gz: OK`. Le dernier bloc reprend les commandes de « Verify the release » avec `ARCH=arm64`.

- [ ] **Step 9: ARRÊT : accord de l'utilisateur requis — supprimer le brouillon de la rc** (facultatif, Q46 ; le tag et l'image `0.1.0-rc.1` restent). Après le « oui » explicite : `gh release delete v0.1.0-rc.1 --yes` (sans `--cleanup-tag`). Retirer le droit ajouté au jeton : `gh auth refresh -h github.com -r read:packages`.

- [ ] **Step 10: Clôture** — une petite PR sur `main` (branche `docs/lancement-constats`, fusionnée par l'utilisateur) :

1. Spec §12 — remplacer :

   ```text
   | Lancement | 11 à 12 | docs FR/EN, GoReleaser, image GHCR, release | v0.1 publique |
   ```

   par (date de l'étape 6) :

   ```text
   | Lancement | 11 à 12 | docs FR/EN, GoReleaser, image GHCR, release | v0.1 publique : v0.1.0 publiée le AAAA-MM-JJ (plan Lancement) |
   ```

2. Plan : cases cochées, « Constats de la tâche 10 » (heure de chaque étape ARRÊT et accord reçu, étiquettes, vérification anonyme, rappel des décisions Q39–Q53 du 2026-10-13 et de tout changement ultérieur).

```bash
git switch -c docs/lancement-constats origin/main
git add docs/superpowers
git commit -F - <<'EOF'
docs : v0.1.0 publiée, constats du Lancement

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
git push -u origin docs/lancement-constats
gh pr create --base main --title "docs : constats du Lancement (v0.1.0)" --body "Spec §12 et constats du plan Lancement."
```

- [ ] **Step 11: Relecture (CLAUDE.md)** — sous-agent indépendant puis Codex sur cette PR, avant sa fusion par l'utilisateur.

---

#### Constats de la rédaction

- Dépôt au 2026-10-13 : branche `phase/observation` (`40aa7cb`), PR #5 ouverte, aucun tag, dépôt **privé** ; `limactl list` : `hk-ubuntu` et `hk-fedora` en `aarch64` (vz).
- Aucun outil de release sur le Mac (`goreleaser`, `cosign`, `syft` absents) : tout passe par la CI ou une VM.
- `-trimpath` retire `-ldflags` des informations de build ; `vcs.modified` devient `true` dès qu'un fichier non suivi (comme `dist/`) est présent.
- Image `FROM scratch` arm64 sur Docker Desktop : 3,8 Mo ; `Env` = `HUBLOT_LISTEN=0.0.0.0:7878 HUBLOT_DATA_DIR=/data`, utilisateur vide (root) ; le volume nommé `hublot-data` neuf passe `prepareDataDir` ; recréer le conteneur garde le jeton.
- Inventaire des messages (Q49, `go/ast` sur les fichiers non-test de `cmd/` et `internal/`, après la PR #5 et la tâche 1) : 169 chaînes françaises dans 27 fichiers (133 messages, 35 chaînes de format `… : …`, l'usage), plus l'assemblage `what, ":", err` de `main.go`. Les `slog` sont déjà en anglais. Tests qui comparent un message : 8 fichiers Go (27 assertions, après la revue du plan), 2 fichiers Vitest (données seulement). L'interface affiche le `detail` du serveur tel quel dans 4 cas (`files.ts:78`, `files.ts:95`, `write.ts:66`, `write.ts:72`).
- Appels de `dockerx` au démon (pour la doc du proxy de socket) : `Ping`, `ContainerList`, `ContainerInspect`, `ContainerDiff`, `ContainerLogs`, `CopyFromContainer`, `CopyToContainer`, `VolumeInspect`.
- GoReleaser (doc consultée le 2026-10-13) : `dockers_v2` depuis 2.12 (`platforms` depuis 2.14), nom « provisoire », remplacera `dockers` et `docker_manifests` (dépréciés) en v3 ; en snapshot, une image par plateforme avec suffixe, rien n'est poussé ; images construites dans la phase de publication. Cosign v3 : `--bundle` obligatoire pour `sign-blob`, installeur v4 requis.

#### Constats des tranches A, B et C

- Au registre de la phase (`.claude/handoff/phase-lancement.registre.md`), une entrée par tranche (règles « Orchestration »).

#### Constats de la tâche 8

- **Spec** (étape 1) : les quatre remplacements du plan sont faits (§8 alternative, §8 image et release, §10 CI, §13 repli) ; `grep -c 'service systemd non testé'` → `1` (§11, résultat historique du spike) ; `DOCS OK`. En plus, pour décrire tout ce que livre le Lancement (registre) : §1 (dépôt `levy-cmd/Hublot-Kit`, casse exacte, et module Go), §2 (hors v0.1 : `.deb`/`.rpm`, `latest`, provenance SLSA), §3 (quatre décisions : langue de l'agent, distribution, chaîne d'approvisionnement, publication), §4 (fichiers de packaging et scripts de release), §6 (`X-Accel-Buffering: no` du flux de logs), §7 (aucune connexion sortante ; ancien jeton accepté jusqu'au redémarrage ; `Cache-Control: no-store` sur `/api/`), §8 (détail de `check-systemd.sh` et `check-image.sh`, drop-in `ReadWritePaths` du dossier de données déplacé, mise à jour, « Vérifier une release », règles du dossier de données, ce qui n'est éprouvé qu'à la rc), §12 (contenu du Lancement, avancement au 2026-10-15), §13 (quatre risques de release : verrou de lancement, dépendance npm embarquée, release défectueuse et image déjà sur GHCR, entrées Rekor publiques).
- **Spec contre doc publique** (EN et FR, mêmes blocs et mêmes titres d'après `check-docs.sh`) : aucune contradiction trouvée. Les écarts étaient des manques de la spec, que la doc disait déjà : les ajouts de §6, §7 et §8 ci-dessus (connexions sortantes, ancien jeton, en-têtes, dossier de données, mise à jour). La doc FR suit la doc EN point par point (limites relues en entier).
- **`check-docs.sh`** (Minor reporté de la tranche B) : le drop-in qui déplace `HUBLOT_DATA_DIR` doit porter `ReadWritePaths=` sur le même dossier, dans le même bloc `ini`, et la doc doit créer ce dossier avant (`sudo install -d -m 0700 <dossier>`) ; l'unité doit garder `ProtectSystem=strict`. Six mutations jouées sur une copie hors du dépôt (EN et FR), toutes attrapées : `ReadWritePaths` retiré, sur un autre dossier, déplacé dans un autre bloc `ini` ; création du dossier retirée ; drop-in du dossier retiré ; `ProtectSystem=strict` retiré de l'unité. Vert sous bash 3.2 (macOS).
- **Audit** (étape 2) :
  - `gitleaks` v8.30.1 (conteneur sur `hk-ubuntu`, dépôt monté en lecture seule, image supprimée ensuite) sur tout l'historique (`--all`, branches distantes comprises) : **238 commits analysés, `no leaks found`**, rapport JSON vide (`[]`). Les 240 commits hors fusions moins 2 qui ne font que supprimer (`dd5e116`, `9521bd3`) ; gitleaks ne lit que les lignes ajoutées.
  - Auteurs et adresses : `Livaille An-nour <51993811+levy-cmd@users.noreply.github.com>` (adresse « noreply » de GitHub, aucune adresse personnelle) et `GitHub <noreply@github.com>` (fusions faites dans l'interface web). Le nom `levy` figure aussi dans `.claude/handoff/spike-semaine-1.md` (`de: levy`).
  - Aucun PDF ni `.superpowers` dans l'historique ; aucun chemin `/Users/…`, `/private/…` ni adresse IP réelle (seulement des adresses d'exemple dans les tests).
  - Deviendraient publics, à trancher par l'utilisateur avant la tâche 10, étape 5 : `CLAUDE.md`, `.claude/agents/*` et `.claude/handoff/*` (cinq fichiers suivis, plus trois passations supprimées mais présentes dans l'historique) : orchestration par agents (Claude, Codex), revues, défauts trouvés puis corrigés, refus du classifieur de permissions ; le lien `claude.ai` du cahier des charges (spec, ligne 5 : privé, inaccessible au public, mais visible) ; des détails de la machine de développement (`/opt/homebrew/bin`, `$HOME/.docker/run/docker.sock`, disque du Mac à 91 %, noms et versions des VM `hk-ubuntu` et `hk-fedora`, `~/.lima/hk-ubuntu/lima.yaml`) ; l'état du compte GitHub dans le plan (compte `User`, offre Free, dépôt privé) ; toutes les branches poussées (dont `phase/observation-t7-wip`) et les PR, dont la PR brouillon #6.
- **Revue de branche** (étape 3) et relecture (étape 5) : à faire par la session principale (`relecteur` puis Codex).
- **Cases cochées** : avant la tâche 1, tâches 1 à 6, tranches A à C (validées au registre), et les étapes 1, 2 et 4 de cette tâche. La mutation « retirer `--cgroupns=host` » de la tâche 4 n'a pas été jouée (refusée par le classifieur de permissions) : le test de la garde cgroup de la tranche A la remplace (registre).

#### Constats de la tâche 9

- **Étape 0** (2026-10-15, accord de l'utilisateur transmis par la session principale, fusion faite par l'agent) : PR #7 (`ci/release-dispatch`, un seul fichier, `release.yml` identique à `phase/lancement`), CI verte (6 jobs), fusionnée par commit de fusion `c27d493` sur `main`. `gh workflow list` → `Release  active`. Branche `ci/release-dispatch` gardée.
- **Étape 1** : `phase/lancement` à jour (`c763472`), checks de la PR #6 verts (9 jobs). Tag annoté `v0.1.0-rc.1` (objet `ba883f2`) sur `c763472`, poussé.
- **Étape 2** : garde local conforme (seul le premier cas accepté). Lancements refusés : run 38783319487 (`--ref v0.1.0-rc.1`, confirmation `v0.1.0-rc.9`) → `failure`, `guard` seul en échec (« confirmation « v0.1.0-rc.9 » différente du tag « v0.1.0-rc.1 » »), `web`, `release`, `verify` sautés ; run 38783372584 (`--ref phase/lancement`) → `failure`, `guard` seul (« à lancer sur un tag (--ref vX.Y.Z), pas sur branch phase/lancement »). `VERROU OK` ; `gh release list` vide.
- **Étape 3** : run 38783489392, 18:11:00 → 18:14:36 UTC (3 min 36 s ; GoReleaser 2.18.3 : 1 min 45 s), image de runner `ubuntu24/20261004.327`. `guard`, `web`, `release`, `verify` verts ; `verify` finit par `RELEASE OK : v0.1.0-rc.1` (trois `Verified OK`, `ok` pour les deux archives, `cosign verify` de l'image, `ok ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1`). Valeurs réelles sous `workflow_dispatch` (journal de `guard`) : `REF_TYPE: tag`, `ACTOR: levy-cmd`, `TRIGGER: levy-cmd`, `OWNER: levy-cmd` ; l'API du run donne aussi `actor` et `triggering_actor` = `levy-cmd`. Droits du jeton : `guard` Metadata read seul ; `release` Contents write, Packages write (et OIDC, la signature keyless a réussi) ; `verify` Contents write, Packages read. L'artefact `web-dist` est passé à `release` (téléchargé, empreinte vérifiée, `index.html` présent). Image poussée : `ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1`, index `sha256:b9d209c5db635faf4293a9881a587954f6271da85ee9acc69b0d599e13c8bc5b`.
- **Étape 4** (partielle) : brouillon `isDraft: true`, `isPrerelease: true`, exactement les 8 fichiers attendus. Le jeton de `gh` n'a pas `read:packages` (`gh auth refresh -s read:packages` exige l'utilisateur dans son navigateur) : visibilité et étiquettes non lues par l'API. Paquet privé prouvé autrement : le manifeste `0.1.0-rc.1` en anonyme → 403, alors que le même accès anonyme à un paquet public (témoin `goreleaser/goreleaser`) → 200. Seule étiquette d'après GoReleaser : `0.1.0-rc.1` (non confirmée par l'API).
- **Étape 5** (partielle) : l'archive arm64, téléchargée du brouillon par `gh release download`, donne `SYSTEMD OK` sur `hk-ubuntu` et sur `hk-fedora` (SELinux `unconfined_service_t`). `check-image.sh` sur l'image GHCR : **en attente** de `read:packages` (connexion `docker login ghcr.io` avec le jeton de `gh`).
- **Étape 6** (partielle) : cosign v3.1.3 (somme contrôlée) dans `hk-ubuntu` : `cosign verify-blob` du bundle de `checksums.txt` → `Verified OK`, puis `hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz: OK` ; avec l'identité `@refs/tags/v0.1.0-rc.2` → `no matching CertificateIdentity found`. Format des signatures de fichiers : bundles `application/vnd.dev.sigstore.bundle.v0.3+json` (certificat, entrée `hashedrekord` 0.0.1, horodatage). `cosign verify` de l'image et l'inspection SBOM hors de GitHub : **en attente** de `read:packages` (faits dans `verify` sur GitHub, verts) ; le format de la signature d'image (référents OCI ou étiquette `sha256-…`) reste à lire.
- **Entrées Rekor** (publiques, permanentes) : `checksums.txt` logIndex 3267280145 (lu sur `rekor.sigstore.dev`), archive arm64 3267279982, archive amd64 3267282183, plus la signature de l'image.
- **Détail** : le brouillon affiche `targetCommitish: main` ; sans effet, le tag existe déjà et pointe sur `c763472`.
- Aucun correctif, aucune rc supplémentaire.

#### Constats de la tâche 10

- *à remplir*
