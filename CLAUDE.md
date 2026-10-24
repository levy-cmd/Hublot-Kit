# Hublot-Kit

Interface web pour explorer l'intérieur des conteneurs Docker d'un serveur. Agent Go + interface SvelteKit embarquée.

## Sources de vérité
- Spec technique (fait foi) : `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md`
- Résultats du spike : `docs/superpowers/spikes/2026-10-07-resultats-spike.md`
- Plans par phase : `docs/superpowers/plans/`
- Registres de phase (`.claude/handoff/`) et définitions d'agents (`.claude/agents/`) : hors du dépôt public, dans le dépôt privé `levy-cmd/Hublot-Kit-registres` (`handoff/` et `agents/` à sa racine), seule source de vérité. Son clone durable est `~/Dev/Hublot-Kit-registres` ; dans le dépôt principal, `.claude/handoff` et `.claude/agents` sont des liens symboliques vers ses dossiers, ignorés par Git. Restauration (machine neuve, ou liens disparus), depuis la racine du dépôt : `gh repo clone levy-cmd/Hublot-Kit-registres ~/Dev/Hublot-Kit-registres` si le clone n'existe pas, puis `(set -e; mkdir -p .claude; for d in handoff agents; do if [ -d .claude/$d ] && [ ! -L .claude/$d ]; then diff -r .claude/$d ~/Dev/Hublot-Kit-registres/$d; rm -r .claude/$d; fi; ln -sfn ~/Dev/Hublot-Kit-registres/$d .claude/$d; done)`. Un vrai dossier `.claude/handoff` ou `.claude/agents` n'est retiré que s'il est identique à celui du clone (`diff -r`) ; sinon la commande s'arrête en erreur sans rien retirer, et l'écart se reporte d'abord dans le clone privé.

## Règles durables
- Accès aux fichiers d'un conteneur : uniquement par les mécanismes de la spec §5 (pidfd + cgroup, `pathrs-lite`, fichiers réguliers, refus `fstatfs` des pseudo-fs, chemins d'écriture sans lien). Jamais `filepath.Join` / `os.Open` sur un chemin du conteneur.
- Les tests d'évasion s'écrivent avant le code qu'ils protègent.
- Le code Linux (`/proc`, `openat2`, pidfd) se teste sous Linux : conteneur `golang`, ou VM Lima `hk-ubuntu` (AppArmor) / `hk-fedora` (SELinux). macOS ne suffit pas.
- Chaque tâche est relue par un sous-agent indépendant **et** par Codex avant de passer à la suivante.
- Commits et docs en français.

## Orchestration (tranches verticales, plan adaptatif)
- **Tranche verticale** : une fonctionnalité se livre de bout en bout (code, tests, doc, vérification sur la cible) ; elle n'est pas répartie en couches entre plusieurs tâches.
- **Fixe / révisable** : l'objectif métier et les critères d'acceptation, écrits dans le plan, ne changent pas sans l'accord de l'utilisateur. Le plan d'exécution peut évoluer selon les résultats ; chaque révision est consignée au registre de la phase (`.claude/handoff/<phase>.registre.md`, dans le dépôt privé) avec sa raison.
- **Écriture des registres** : seule la session coordinatrice écrit un registre, dans le clone durable `~/Dev/Hublot-Kit-registres`, puis fait commit et push dans le dépôt privé. Un worktree n'a pas `.claude/handoff` : un agent en worktree ne commite jamais de registre dans Hublot-Kit, il remonte l'entrée dans son rapport.
- **Garde de fusion** : avant toute fusion vers `main`, `git ls-files .claude` doit être vide. Une branche antérieure à `ace6524` (par exemple `phase/interface`) suit encore `.claude/` : y passer dans le dépôt principal remplace sans rien dire les liens par de vrais dossiers, puis les supprime au retour. Son `git rm -r --cached .claude` se fait donc dans un worktree jetable, jamais dans le dépôt principal ; si les liens ont disparu, la commande de restauration les recrée.
- **Contrat avant parallèle** : avant deux implémentations en parallèle, écrire au registre le contrat partagé (formats, interfaces) et la propriété des fichiers. Un fichier n'a qu'un seul écrivain à la fois ; le travail parallèle se fait en worktree ; la fusion revient au coordinateur, suivie de `make test`.
- **Terminé** veut dire : tests pertinents exécutés, revue `relecteur` et revue Codex (`codex:codex-rescue` en lecture seule) concordantes ou arbitrées au registre, CI verte si la branche est poussée.
- **Déléguer quand ça rapporte** (travail vraiment parallèle, contexte à isoler, regard indépendant), jamais pour multiplier les agents. Chaque délégation donne : objectif, périmètre et exclusions, contexte, critères d'acceptation, rapport attendu.
- **Adapter en cours de route** : réévaluer après chaque résultat significatif ; si une dépendance bloque, poursuivre les tâches indépendantes sans risque ; un désaccord sur une interface ou une règle métier se tranche au registre avant l'intégration ; un échec se diagnostique avant d'être corrigé.
- **Rapport de fin, sur preuves** : résultats métier livrés, tranches terminées, agents et apports, fichiers et contrats changés, tests exécutés avec leurs résultats réels, ce qui reste bloqué ou non vérifié.
- **Hygiène** : après un agent qui crée des branches, vérifier la branche du dépôt principal. Une mutation qui affaiblit une garde de sécurité se joue sur une copie (ou par un faux journal), jamais dans le dépôt : le classifieur la refuse, et on ne la contourne pas.
- **Suppressions** : un agent ne supprime que ce qu'il a créé lui-même dans sa tâche (conteneurs, volumes, images, fichiers, branches). Jamais de `docker image prune`, `docker system prune` ni de nettoyage global sans l'accord de l'utilisateur.
- **Correctifs de sécurité** : le dépôt est public. Une branche de correctif de faille ne se pousse qu'avec ses correctifs et `make test` au vert, jamais avec les seuls tests rouges, qui décriraient une attaque encore ouverte.
- **Rôles** : la session principale coordonne ; `implementeur` écrit (`.claude/agents/implementeur.md`) ; `relecteur` relit sans écrire (`.claude/agents/relecteur.md`) ; ces deux définitions sont dans le dépôt privé. Les prompts ne recopient pas ces règles, ils donnent la tâche, les fichiers possédés et le contrat.
