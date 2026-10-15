# Registre de la phase Lancement

Plan : `docs/superpowers/plans/2026-10-13-lancement.md`. Décisions de l'utilisateur : section « Décisions » du plan.

## Revue du plan
- Deux tours (Opus + Codex), puis une validation : 24fad02 → eae7277 → ee58f7c.
- Ruling: les 4 `detail` serveur affichés tels quels par l'interface restent en anglais. Ils sont dynamiques ; les traduire coûterait environ une journée et toucherait l'API §6.
- Ruling: un environnement GitHub avec relecteur obligatoire est impossible sur un dépôt privé avec une offre Free. `release.yml` part donc par `workflow_dispatch` seulement, derrière le job `guard`. `levy-cmd` est un compte `User` : la comparaison `actor == repository_owner` est juste.
- Minor reporté : « Re-run failed jobs » ne relance pas `guard`. Le risque est faible, le dépôt n'a qu'un propriétaire.

## Tâches
- Task 1 (`--version`) : d96d12f, 00fd972. Validée par Opus et Codex.
- Task 2 (messages en anglais) : 7ca7e92, b062bc9, a7835c7. Validée par Opus et Codex.
  - La preuve AST montre que seules des chaînes ont changé, sauf `main.go:129` (`Fprintln` → `Fprintf`, prévu par le plan).
  - Minor accepté : la garde, qui repose sur une liste de mots, laisse passer « acces refuse », « session terminee », « hote manquant ».
- Task 3 (unité systemd) : 63f5059, de66621, 8e38631. Validée par Opus et Codex.
  - Vérifiée sur hk-ubuntu et hk-fedora, en lecture et en écriture, sans AVC ni refus AppArmor.
  - `systemd-analyze security` : 8.9 EXPOSED. Le durcissement est limité par Q41.
  - Ruling: les drapeaux « créé par le script » de `check-systemd.sh` sont posés juste avant chaque création. La course avec un autre processus entre la garde et l'installation est acceptée : elle ne touche que la VM de vérification. Poser le drapeau après la création laisserait un fichier partiel si l'installation échouait (Important de Codex, écarté).
- Task 4 (image `scratch` et `check-image.sh`) : d249a89, 546c5c7. Validée par Opus et Codex.
  - Image de 3,8 Mo. Docker Desktop, hk-ubuntu et hk-fedora : lecture et écriture OK, sans AVC ni refus AppArmor.
  - La commande de §8 passe par un tableau d'arguments, plus par `sh -c`. `hk_spec_args` vérifie qu'elle fait 8 lignes, que la dernière est la référence d'image, et que ses caractères sont dans une liste blanche.
  - `scripts/test-release-lib.sh` joue 11 mutations et fait partie de `make test`. `hk_redact` masque le jeton, en supposant le format de 43 caractères en base64url.
  - Étiquettes OCI reportées à la tâche 6 (`dockers_v2`).
  - En attente de l'utilisateur : la mutation « retirer `--cgroupns=host` » n'a pas été jouée. Le classifieur de permissions a refusé la modification ; personne ne l'a contourné.

## Orchestration (2026-10-15, passage aux tranches verticales)
- Règles ajoutées à `CLAUDE.md` (section « Orchestration ») ; sous-agents de projet `implementeur` et `relecteur` dans `.claude/agents/`.
- État au moment du passage : la session précédente s'est arrêtée avec trois agents interrompus.
  - **T5 (docs)** : terminée sur `phase/lancement-t5` (2deb213), NON relue. Les deux revues ont été interrompues ; le worktree du relecteur est resté verrouillé.
  - **T6 (GoReleaser)** : un commit local NON poussé (f05b69f) ; la fin de la tâche n'est pas vérifiée (`goreleaser check`, snapshot, job CI).
- Propriété des fichiers (aucun recouvrement constaté) :
  - T5 possède `README*`, `docs/`, `scripts/check-docs.sh` ;
  - T6 possède `.goreleaser.yaml`, `.github/workflows/ci.yml`, `scripts/check-dist.sh`, `.gitignore`.
- Contrat T5/T6 : le job CI `docs` (`bash scripts/check-docs.sh` sur ubuntu-latest) s'ajoute à `ci.yml` APRÈS la fusion de T5. C'est l'écrivain de `ci.yml` qui l'ajoute. Les noms d'archives et l'image citée dans la doc doivent égaler ceux de `.goreleaser.yaml` ; `check-docs.sh` est à étendre si besoin.
- 2026-10-15, première application des règles :
  - **Task 5 (docs)** : 2deb213, 9ae591c (cinq limites de §13 ajoutées, IP seulement pour `login.fail`, sortie JSON de `cosign verify`, niveaux de titre contrôlés). Validée par le relecteur et par Codex ; fusionnée (9d872c7). `make test` et `check-docs.sh` sont verts.
    - Ruling: les captures filtrent `/api/containers` pour ne garder que les conteneurs de démonstration. Accepté : c'est la vraie interface, aucune donnée personnelle. À refaire depuis hk-ubuntu, sans filtrage, si l'occasion se présente.
  - **Task 6 (GoReleaser)** : f05b69f, poussé. `goreleaser check` et `actionlint` sont verts, ainsi qu'un snapshot local avec `README.fr.md`. Le job CI `release-snapshot` était rouge faute de `README.fr.md`, que l'archive exige : c'est une dépendance T6 → T5 ABSENTE du contrat. Leçon : le contrat liste aussi les fichiers qu'une tâche LIT chez une autre.
    - L'agent a ouvert une PR brouillon #6 sans consigne : écart à signaler, sans effet (dépôt privé).
    - Instabilité : `TestViewsUnseenIndexesExpire` (`internal/fsview`, délai de grâce de 300 ms sous `-race`) a échoué une fois en CI. À analyser à part.
  - **Garde cgroup de `check-image.sh`** : la mutation bloquée par les permissions est remplacée par un test unitaire (faux journal) dans la tranche A. L'utilisateur en est informé.
  - **Révision du plan en tranches A, B, C** : validée par Codex et par le relecteur après un correctif (ordre commit → CI → VM dans A et B, fichiers lus listés par tranche).
