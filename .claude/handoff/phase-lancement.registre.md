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
