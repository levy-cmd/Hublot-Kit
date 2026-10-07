# Hublot-Kit

Interface web pour explorer l'intérieur des conteneurs Docker d'un serveur. Agent Go + interface SvelteKit embarquée.

## Sources de vérité
- Spec technique (fait foi) : `docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md`
- Résultats du spike : `docs/superpowers/spikes/2026-10-07-resultats-spike.md`
- Plans par phase : `docs/superpowers/plans/`

## Règles durables
- Accès aux fichiers d'un conteneur : uniquement par les mécanismes de la spec §5 (pidfd + cgroup, `pathrs-lite`, fichiers réguliers, refus `fstatfs` des pseudo-fs, chemins d'écriture sans lien). Jamais `filepath.Join` / `os.Open` sur un chemin du conteneur.
- Les tests d'évasion s'écrivent avant le code qu'ils protègent.
- Le code Linux (`/proc`, `openat2`, pidfd) se teste sous Linux : conteneur `golang`, ou VM Lima `hk-ubuntu` (AppArmor) / `hk-fedora` (SELinux). macOS ne suffit pas.
- Chaque tâche est relue par un sous-agent indépendant **et** par Codex avant de passer à la suivante.
- Commits et docs en français.
