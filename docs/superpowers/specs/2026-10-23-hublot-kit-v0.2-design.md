# Hublot-Kit v0.2 — Complément de spec

- **Date** : 2026-10-23
- **Statut** : toutes les décisions de l'utilisateur du 2026-10-23 sont appliquées (v0.2 §0.1), et aucune question ne reste ouverte. Correctifs de revue du 2026-10-23 intégrés (registre)
- **Référence** : la spec v0.1 (`2026-10-07-hublot-kit-v0.1-design.md`) reste la référence pour tout ce que ce document ne change pas. Ses numéros de section (§5.1, §7…) sont cités tels quels ; les sections de ce document sont notées « v0.2 §N ».
- **Plan** : `docs/superpowers/plans/2026-10-23-v0.2.md` ; registre : `.claude/handoff/phase-v0.2.registre.md`.

## 0. Décisions et question ouverte

### 0.1 Décisions de l'utilisateur (2026-10-23)

Q55, Q56, Q57, Q59 et Q60 sont tranchées par l'utilisateur ; pour Q58 et Q61 à Q68, il a adopté la recommandation. Le texte de la spec les applique ; chaque renvoi « (Qnn) » désigne la décision ci-dessous. La numérotation suit celle des plans précédents (Q54 était la dernière).

| # | Sujet | Décision |
|---|---|---|
| Q55 | F8 (process et ressources), F9 (démarrer, arrêter, redémarrer), F10 (terminal web), promises pour la v0.2 par la spec v0.1 | reportées après la v0.2 |
| Q56 | encart « lecture seule » de la maquette (« sécurisé par défaut ») | l'interface affiche l'état réel ; la lecture seule n'est **pas** le défaut (`--read-only=false`, §8, inchangé). Encart : « Lecture seule : aucune écriture dans les conteneurs » quand l'option est active, « Écriture permise, toujours confirmée » sinon ; jamais « l'hôte » |
| Q57 | prendre ou supprimer une photo quand l'agent tourne en `--read-only`, sachant son coût sur la production (photo d'un conteneur actif non atomique, jusqu'à 1 Gio lu, environ 150 Mo de mémoire au pire ; export d'un conteneur arrêté qui lit aussi ses volumes et le verrouille jusqu'à 5 min, v0.2 §5.7) | **permis**, y compris en lecture seule, après une boîte de confirmation qui affiche ce coût. La photo des métadonnées tirée d'un index déjà construit est proposée en premier quand elle est possible |
| Q58 | volumes et binds dans un snapshot | exclus : le snapshot couvre le système de fichiers propre au conteneur ; chaque point de montage est noté sans être parcouru |
| Q59 | contenu des snapshots | aucun contenu conservé ; pas de diff ligne à ligne entre deux photos. Contre le conteneur actuel : le contenu actuel, et un diff unifié seulement quand l'ancienne empreinte égale celle de la version de l'image (v0.2 §5.7) |
| Q60 | labels | liste blanche `com.docker.compose.*` et `org.opencontainers.image.*` (en clair s'ils passent aussi §7) ; les autres montrent leur nom avec une valeur masquée, sans révélation |
| Q61 | fichiers dans la recherche globale | seulement dans l'index déjà construit du conteneur ou de l'image ouvert ; jamais de construction ni de parcours, jamais plusieurs conteneurs |
| Q62 | couches zstd | refusées (`unsupported_image`) ; pas de nouvelle dépendance |
| Q63 | téléchargement des fichiers d'une image | non ; aperçu seul, 2 Mio |
| Q64 | thème sans choix mémorisé | sombre, même si le système demande le clair |
| Q65 | diff unifié | `@codemirror/merge` (`unifiedMergeView`) |
| Q66 | rétention des snapshots | 20 par conteneur, 1 Gio au total, aucune suppression automatique (`too_large`), snapshots d'un conteneur supprimé gardés et marqués |
| Q67 | publication | `v0.2.0-rc.1` privée, puis `v0.2.0`, chaque étape sur ARRÊT |
| Q68 | polices | Archivo et JetBrains Mono, déjà auto-hébergées |

### 0.2 Questions ouvertes

Aucune. Q57, reformulée après la revue, a été tranchée par l'utilisateur (2026-10-23), et figure au tableau ci-dessus.

## 1. Objet et périmètre

Une nouvelle interface, « Graphite & Émeraude », sur la disposition de la maquette de l'utilisateur. Elle s'accompagne de nouvelles fonctions de l'agent, toutes en **lecture seule côté Docker** : aucune ne crée, ne démarre, ne modifie ni ne supprime quoi que ce soit dans Docker. La seule écriture permise reste celle de §5.3 (F4).

| # | Fonctionnalité | Section |
|---|---|---|
| F11 | Disposition de la maquette, thème sombre par défaut et thème clair, bascule mémorisée | v0.2 §3, §4 |
| F12 | Recherche globale (Ctrl+K, Cmd+K sur macOS) | v0.2 §5.8 |
| F13 | Projets Compose : colonne, filtre « Projet » | v0.2 §5.1 |
| F14 | Labels dans le détail | v0.2 §5.2 |
| F15 | Chiffres clés : fichiers, taille de l'image, taille du conteneur, dernière sortie | v0.2 §5.3 |
| F16 | Page Images : liste, taille, date, couches, conteneurs qui l'utilisent, fichiers en lecture seule | v0.2 §5.4, §5.5 |
| F17 | Diff de contenu d'un fichier entre l'image et le conteneur (panneau du bas) | v0.2 §5.6 |
| F18 | Page Snapshots : photo de l'état des fichiers, comparaison | v0.2 §5.7 |
| F19 | Page Secrets : inventaire des secrets de tous les conteneurs | v0.2 §5.9 |

Hors v0.2 : F8 à F10 (Q55), et tout ce que §2 exclut déjà.

## 2. Invariants repris sans changement

- Accès aux fichiers d'un conteneur : uniquement par §5 (pidfd et cgroup, `pathrs-lite`, fichiers réguliers, refus `fstatfs` des pseudo-systèmes, chemins d'écriture sans lien). Le parcours d'un snapshot (v0.2 §5.7) passe par les mêmes fonctions de `fsview`, sans aucun `filepath.Join` ni `os.Open` sur un chemin du conteneur.
- Une image se lit sans créer de conteneur (v0.2 §5.5). `docker create`, `docker run`, `docker pull`, `docker commit` et `docker cp` vers un conteneur sont exclus.
- Aucun secret ne sort de l'agent hors de `env/reveal`. Ce sont les règles de §7 (`internal/secrets`), étendues aux labels, à l'historique d'une image et à sa configuration.
- `{@html}` reste interdit ; tout contenu venu d'un conteneur ou d'une image passe par l'échappement de Svelte.
- L'agent n'ouvre toujours aucune connexion sortante.
- Messages de l'agent en anglais ; interface en FR et EN, avec les mêmes clés (test existant).

## 3. Disposition (F11)

Elle reproduit la maquette. Les écarts sont justifiés par ce qui existe réellement.

**Barre du haut** (toutes les pages sauf `/login`)
- À gauche : l'anneau du hublot, « Hublot-Kit » et la **vraie version** (`/api/meta` → `version`, par exemple `0.2.0` ou `dev`). La « v1.8.0 » de la maquette disparaît.
- Au centre : le champ « Rechercher un conteneur, une image, un fichier… » avec l'indication `Ctrl K` (`⌘ K` sur macOS). Le champ ouvre la palette (v0.2 §5.8) ; il ne fait aucune recherche par lui-même.
- À droite :
  - l'état de Docker : « Connecté » ou « Docker injoignable », avec l'adresse (`/api/meta` → `docker.host`, par exemple `unix:///var/run/docker.sock`) ;
  - l'icône bouclier : pleine avec le libellé « Lecture seule » si `readOnly`, sinon contour avec « Écriture permise » ; l'état se lit toujours en texte, dans l'infobulle et pour les lecteurs d'écran ;
  - la bascule du thème (`aria-pressed`) ;
  - le **menu du serveur**, qui remplace le menu utilisateur « Livaille » (Hublot-Kit n'a pas de comptes, seulement un jeton). Il contient : l'adresse de l'agent (`location.host`), « Serveurs » (`/servers`), « Paramètres » (`/settings`) et « Se déconnecter » (`POST /api/logout`).

**Barre latérale**
- Entrées : Conteneurs (compteur), Images (compteur), Diff avec l'image, Recherche, Logs, Secrets, Snapshots, Paramètres.
- Une entrée n'apparaît qu'une fois sa page livrée par sa tranche : aucun lien mort.
- « Diff avec l'image » et « Logs » ouvrent le panneau correspondant du conteneur choisi (`/?c=<id>&panel=diff` ou `panel=logs`). Sans conteneur choisi, la liste demande d'en choisir un.
- « Recherche » ouvre la palette.
- En bas, un encart qui dit l'état réel (Q56), puis la marque et la version.
- Repliable en icônes sous 80 rem, en tiroir sous 48 rem.

**Page Conteneurs** (`/`)
- Titre « Conteneurs », sous-titre « Voir, explorer et analyser vos conteneurs, même arrêtés, même sans shell ». Indicateur du nombre de conteneurs et bouton « Actualiser ».
- Onglets à compteurs : Tous, En cours, Arrêtés. L'onglet « Images » de la maquette mène à `/images` avec son compteur : une image n'est pas un conteneur et ne se filtre pas dans ce tableau.
- Une recherche locale, puis les listes « Statut » et « Projet » (v0.2 §5.1). Les filtres vivent dans l'URL (`?state=&project=&q=`), sauf la recherche, qui n'y figure pas (un nom peut être sensible).
- Tableau : Nom (avec icône), Image, Statut (pastille et texte), Ports, Uptime, Actions.
  - **Uptime** : le `Status` de Docker, analysé puis traduit (`Up 3 hours (healthy)` → « depuis 3 heures, sain » ; `Exited (137) 2 days ago` → « arrêté il y a 2 jours, code 137 »). Le code de sortie n'est affiché que s'il est non nul. Si le texte n'est pas reconnu, il est montré tel quel. Le vocabulaire de Docker est fini : `HumanDuration` de `go-units`.
  - **Actions** : Explorer, Logs, Comparer ; elles ouvrent le panneau du bas.
- La ligne choisie est surlignée (fond `--selection` et liseré émeraude de 3 px). Clavier : flèches, Début, Fin, Entrée pour choisir, `/` pour la recherche locale (mécanique `rowStep` reprise de « Quai »).

**Panneau droit** (détail du conteneur choisi)
- Nom, icône, badge d'état.
- Image (avec un bouton de copie), Créé le, Dernière sortie (v0.2 §5.3).
- Trois chiffres : Fichiers, Taille de l'image, Taille du conteneur (v0.2 §5.3), chargés à part et sans bloquer le reste.
- Onglets :
  - Aperçu : commande masquée avec copie, ports, labels en puces ;
  - Logs ;
  - Diff : la liste F5 ;
  - Fichiers : l'explorateur F2 à F4 ;
  - Secrets : la partie « environnement » de la radiographie F7, avec révélation unitaire. Les réseaux, montages, redémarrages, etc. restent dans Aperçu, sous un repli « Détails ».
- Boutons « Explorer les fichiers », « Comparer avec l'image », « Voir les logs » : ils ouvrent le panneau du bas correspondant.
- Le bouton de copie de la commande copie la **forme masquée** ; une valeur masquée n'est jamais dans le presse-papiers.

**Panneaux du bas** (conteneur choisi), côte à côte à partir de 90 rem, en onglets en dessous
1. « Système de fichiers » : l'arbre F2, avec la bascule Explorer / Diff (marques F5 dans l'arbre, comme en v0.1).
2. « Diff avec l'image » : compteurs Ajoutés, Modifiés, Supprimés, et la liste F5 avec des badges.
3. Le contenu du fichier choisi : l'aperçu F2, ou le diff unifié image contre conteneur (F17, v0.2 §5.6), avec les numéros de ligne.
4. « Logs en direct » : le flux F6 avec horodatage, niveau et message, la bascule « Suivi automatique » (le défilement suit la fin ; la désactiver ne met pas le flux en pause) et « Télécharger les logs ». Le **niveau** est déduit du texte (`ERROR`, `WARN`, `INFO`, `DEBUG`, ou un champ JSON `level`) ; à défaut, on affiche le flux (`stderr` ou `stdout`). La déduction est heuristique, et l'infobulle le dit.

**Pages**
- `/login` ;
- `/` : conteneurs, détail, panneaux ;
- `/c/[id]` : page pleine d'un conteneur, gardée pour les liens et le téléphone, mêmes onglets que le panneau droit ;
- `/images` et `/images/[id]` ;
- `/secrets` ;
- `/snapshots` ;
- `/settings` ;
- `/servers`.

**Téléphone** : la liste, puis le détail en page pleine ; les panneaux du bas deviennent les onglets de `/c/[id]`. L'édition reste pensée pour l'ordinateur (§9).

**Paramètres** (`/settings`) : la langue et le thème (préférences du navigateur), puis, en lecture : la version de l'agent, la lecture seule, Docker (adresse, version du démon, version d'API), le dossier de données, les plafonds des snapshots. Aucun réglage de l'agent ne s'écrit depuis l'interface en v0.2 : ce sont des options de démarrage (§8).

## 4. Identité « Graphite & Émeraude » (F11)

Fond anthracite, surfaces en gris ardoise, émeraude pour ce qui est actif (sélection, focus, onglet ouvert, en marche, bouton principal), ambre discret pour les alertes. Une interface industrielle, sobre, orientée DevOps. Pas d'indigo, pas de dégradé, pas d'ombre portée décorative.

### 4.1 Jetons

Sombre par défaut (Q64). Le clair s'active par la bascule, mémorisée dans `localStorage` (`hublot-theme`, seule valeur stockée), et appliquée avant le premier affichage par le script en ligne de `app.html`, dont l'agent calcule l'empreinte CSP (§7, mécanique reprise de « Quai »).

| Jeton | Sombre | Clair | Usage |
|---|---|---|---|
| `--fond` | `#0F1215` | `#E8ECF0` | fond de page |
| `--surface` | `#171B20` | `#F7F9FA` | barres, panneaux, tableau |
| `--releve` | `#1F252B` | `#EEF2F5` | champs, en-têtes de tableau, survol, blocs de code |
| `--trait` | `#2A3138` | `#D3DAE0` | séparateurs décoratifs (non porteurs d'information) |
| `--trait-fort` | `#6B7784` | `#7A8693` | bords de champs et de boutons |
| `--texte` | `#E5E9EC` | `#141A1F` | texte |
| `--texte-doux` | `#9AA4AE` | `#4E5A65` | texte secondaire, indications |
| `--emeraude` | `#10B981` | `#10B981` | remplissage : bouton principal, pastille « en marche » |
| `--sur-emeraude` | `#04130D` | `#04130D` | texte sur l'émeraude |
| `--emeraude-texte` | `#34D399` | `#036B4D` | texte et icône actifs, liens, puce d'état seule |
| `--focus` | `#34D399` | `#036B4D` | anneau de focus (2 px, décalé de 2 px), liseré de la ligne choisie |
| `--selection` | `#123329` | `#D3EFE3` | fond de la ligne choisie et de l'onglet ouvert |
| `--ambre` | `#F2B544` | `#F2B544` | remplissage : pastille d'alerte (pause, redémarrage, sortie anormale) |
| `--sur-ambre` | `#1A1203` | `#1A1203` | texte sur l'ambre |
| `--ambre-texte` | `#F2B544` | `#8A5300` | texte d'alerte, puce d'alerte seule |
| `--rouge-texte` | `#F28B82` | `#B3261E` | erreur de l'interface, suppression dans un diff |
| `--diff-ajout` | `#11291F` | `#DDF3E8` | fond d'une ligne ajoutée |
| `--diff-suppr` | `#3A1A1A` | `#FBE3E1` | fond d'une ligne supprimée |

**Règles d'emploi**
- « Même émeraude » dans les deux thèmes : `--emeraude` est identique. Sur fond clair, le texte et les puces seules prennent `--emeraude-texte`, plus sombre, pour tenir le contraste (`#10B981` sur `#F7F9FA` ne donne que 2,40:1).
- Une pastille d'état est un remplissage plus un texte (`--sur-emeraude` sur `--emeraude`) ; une puce seule, sans texte dans la puce, prend la variante `-texte`. L'état se lit **toujours** en texte : la couleur le double, elle ne le remplace pas.
- Arrêté proprement (code 0) : texte doux, pas de couleur. Code non nul, `dead`, `OOMKilled` : ambre. Rouge : erreurs de l'interface et suppressions dans un diff.
- Rayon de 6 px (champs, boutons, panneaux) et de 999 px pour les pastilles ; filets de 1 px ; aucune ombre, sauf celle de la palette et des menus (une seule, `0 8px 24px rgb(0 0 0 / .35)` en sombre, `.12` en clair), qui flottent réellement.
- Animations : 120 ms au plus sur le survol et l'ouverture des menus ; `prefers-reduced-motion` les coupe.

### 4.2 Contraste (WCAG 2.x, calculé)

Ratios calculés par la formule de luminance relative de WCAG (script reproductible dans le plan, tranche 1, étape 1). Texte : 4,5:1 au moins. Éléments non textuels porteurs d'information (bords de champs, focus, puces) : 3:1 au moins.

| Paire | Sombre | Clair |
|---|---|---|
| texte sur fond / surface / relevé | 15,39 / 14,17 / 12,67 | 14,77 / 16,61 / 15,58 |
| texte sur sélection | 11,23 | 14,39 |
| texte doux sur fond / surface / relevé / sélection | 7,42 / 6,83 / 6,11 / 5,42 | 5,95 / 6,69 / 6,27 / 5,79 |
| émeraude-texte sur fond / surface / relevé / sélection | 9,77 / 9,00 / 8,05 / 7,13 | 5,50 / 6,19 / 5,80 / 5,36 |
| sur-émeraude sur émeraude (bouton, pastille) | 7,50 | 7,50 |
| sur-ambre sur ambre (pastille) | 10,13 | 10,13 |
| ambre-texte sur surface / relevé / sélection | 9,45 / 8,45 / 7,49 | 5,99 / 5,62 / 5,19 |
| rouge-texte sur surface / relevé / sélection | 7,24 / 6,47 / 5,74 | 6,19 / 5,81 / 5,36 |
| texte sur diff-ajout / diff-suppr | 12,64 / 12,81 | 15,08 / 14,35 |
| émeraude-texte sur diff-ajout ; rouge-texte sur diff-suppr | 8,03 ; 6,55 | 5,62 ; 5,35 |
| trait-fort sur fond / surface / relevé (≥ 3) | 4,11 / 3,79 / 3,39 | 3,13 / 3,51 / 3,30 |
| focus sur fond / surface / relevé / sélection (≥ 3) | 9,77 / 9,00 / 8,05 / 7,13 | 5,50 / 6,19 / 5,80 / 5,36 |
| émeraude (remplissage) sur surface (≥ 3) | 6,82 | 2,40 : **jamais seule** en clair, toujours avec son texte |

Un test Vitest (`web/src/lib/contrast.test.ts`) relit les jetons dans `app.css` et refait ces calculs : un jeton modifié qui passe sous le seuil fait échouer `make test`.

### 4.3 Typographie et forme

- Archivo variable pour l'interface (largeur 100 %, titres à 87,5 %) ; JetBrains Mono pour le technique : identifiants, images, ports, chemins, empreintes, logs, contenu de fichier (Q68).
- Échelle : 0,75 / 0,8125 / 0,875 / 1 / 1,25 / 1,5 rem ; titre de page 1,5 rem ; chiffres tabulaires partout.
- Casse normale ; pas de surtitres en capitales espacées.
- Icônes : un jeu SVG maison en ligne (trait de 1,5 px, 16 et 20 px), dans `web/src/lib/icons/` ; aucune bibliothèque d'icônes.

### 4.4 Ce qui est repris de la tranche 1 « Quai » (c963718, `origin/phase/interface`)

Le thème « Quai » est abandonné. On reprend, fichier par fichier (`git show c963718:<chemin>`), ce qui suit.

| Repris | Adaptation |
|---|---|
| `web/src/lib/theme.ts` et `theme.test.ts` (clé `hublot-theme`, `parseStored`, `toggled`, `setTheme`) | `effectiveTheme(null, …)` renvoie `'dark'` (Q64) ; le test change en conséquence |
| Script d'amorçage en ligne de `app.html` (empreinte CSP calculée par `internal/web`, déjà en place depuis la v0.1) | le défaut devient `dark` ; `<meta name="color-scheme" content="dark light">` |
| `web/static/fonts/archivo-latin-wdth.woff2`, `jetbrains-mono-latin-wght.woff2`, `Archivo-OFL.txt`, `JetBrainsMono-OFL.txt` | aucune |
| `stateTone` et `rowStep` de `containers.ts`, avec leurs tests | `stateTone` prend aussi le code de sortie : `exited` avec le code 0 → `idle` (texte doux), sinon `warn` (ambre), comme `dead` ; le ton `bad` (rouge) n'est plus utilisé pour un état |
| Page de connexion (structure, accessibilité) et clavier de la liste | restylées |
| Méthode : contrastes calculés et consignés, état toujours en texte, aucun CDN | inchangée |

Ne sont pas repris : la palette et les jetons « Quai », la plaque (`Plaque.svelte`, `plaqueId`), le jaune de marquage, le document `2026-10-23-interface-design.md` (remplacé par cette section). La branche `phase/interface` est gardée telle quelle (on ne supprime pas les branches de phase).

## 5. Fonctions de l'agent

### 5.1 Projets Compose (F13)

- `GET /api/containers` ajoute `project` et `service`, lus dans les labels de la liste Docker (`com.docker.compose.project`, `com.docker.compose.service`), absents hors Compose. Aucun appel de plus : `ContainerList` renvoie déjà les labels. Ce sont les deux seuls labels exposés par la liste.
- Le filtre « Projet » est fait côté interface : la liste « Projet » propose les projets présents, plus « Sans projet ».
- Ces valeurs passent par la règle des labels (v0.2 §5.2). Un nom de projet qui serait masqué n'est pas renvoyé, et le conteneur compte alors comme « sans projet ».

### 5.2 Labels (F14) — décision

La v0.1 les excluait de la radiographie : ils portent souvent des secrets (plan Observation, « Radiographie en un seul `Inspect` »). En v0.2, ils sortent **uniquement** par `secrets.Labels`, avec cette règle :

1. une valeur est montrée en clair seulement si sa clé commence par `com.docker.compose.` ou `org.opencontainers.image.`, **et** si `secrets.Masked(clé, valeur)` (§7) est faux ;
2. tout autre label est montré par sa clé, avec la valeur `null` et `masked: true` ;
3. aucune révélation des labels en v0.2 (Q60).

**Pourquoi une liste blanche plutôt que le masquage de §7 seul.** §7 reconnaît un secret à des mots dans le **nom** (`PASSWORD`, `TOKEN`, `KEY`…) ou à des formes dans la valeur (URL à identifiants, `NOM=valeur`…). Or les labels sont de la configuration d'outils tiers, dont les clés ne contiennent pas ces mots. Exemple courant : `traefik.http.middlewares.auth.basicauth.users=admin:$apr1$…`. Ni « users » ni « basicauth » ne sont des mots de §7, et la valeur n'a ni `://`, ni `=`, ni `Bearer` : §7 la laisserait passer, et l'empreinte apr1 se casse hors ligne. Ce cas est figé par un test rouge (plan, tranche 2). D'autres outils mettent dans des labels des DSN, des commandes avec mot de passe ou des clés de webhook. Les labels de Compose, eux, sont écrits par Compose (projet, service, numéro, dossier de travail, fichiers de configuration, empreinte de configuration, version, image, dépendances) ; ceux de l'OCI décrivent l'image (source, révision, version, licences). Le masquage de §7 reste appliqué même sur la liste blanche : un label posé à la main sous `com.docker.compose.` peut contenir n'importe quoi.

Coût : on ne voit pas en clair les labels de Traefik, Watchtower, etc. Leur clé reste visible, ce qui suffit pour savoir qu'un routeur ou une règle existe ; leur valeur se lit avec `docker inspect` sur l'hôte.

### 5.3 Chiffres clés (F15)

| Chiffre | Origine | Coût | Route |
|---|---|---|---|
| Dernière sortie | `State.FinishedAt` et `State.ExitCode` de l'`Inspect` déjà fait par `GET /api/containers/{id}` ; `0001-01-01T00:00:00Z` → « jamais » | nul | `GET /api/containers/{id}` (champ `finishedAt` ajouté) |
| Taille du conteneur | `SizeRw` (couche inscriptible) et `SizeRootFs` (total), `Inspect` avec `size=true` | mesuré : 11 à 16 ms par conteneur (Docker Desktop, couches inscriptibles petites) contre 2 à 9 ms sans `size` ; dockerd parcourt la couche inscriptible, le coût croît avec son nombre de fichiers. La liste avec `size=1` coûte 565 ms pour 39 conteneurs, contre 69 ms : **jamais** dans la liste | `GET /api/containers/{id}/figures`, chargée à part ; délai de 30 s ; mise en cache 60 s par ID |
| Taille de l'image | `Size` de `GET /images/{id}/json` (image du conteneur, par son ID) | quelques ms ; cache par ID d'image (immuable) | même route |
| Fichiers | (1) l'index d'un conteneur arrêté **déjà construit** (`Views`, sans jamais lancer de construction) ; (2) sinon, le dernier snapshot du conteneur (v0.2 §5.7), avec sa date ; (3) sinon, `null`, et un bouton « Compter » qui lance un comptage borné (v0.2 §5.7, `GET /api/containers/{id}/count`) | (1) et (2) : nuls. (3) : un parcours `procfs` sans lecture de contenu (conteneur actif) ou un export (conteneur arrêté) ; plafonds de v0.2 §5.7 | `figures` renvoie `files: {count, source: "index"|"snapshot"|"count", at}` ou `null` |

La réponse de `figures` : `{"sizeRw": 1234, "sizeRootFs": 56789, "imageSize": 55555, "files": {"count": 517, "source": "index", "at": "…"} | null}`. Une taille que Docker ne donne pas (`null` dans l'API de Docker) reste `null`. Pendant l'export d'un conteneur arrêté, dockerd verrouille ce conteneur, et l'`Inspect` attend (registre Interface, v0.1.1) : `figures` répond alors après l'export, ou `504` au délai. Le panneau affiche « — » sans bloquer le reste.

### 5.4 Images : liste et détail (F16)

- `GET /api/images` : `[{id, tags, size, created, containers: [{id, name, state}]}]`. Source : `GET /images/json` (mesuré : 172 ms pour 74 étiquettes) ; les conteneurs viennent de la liste des conteneurs, par `ImageID`. Les images intermédiaires sont exclues ; les images sans étiquette sont gardées (`tags: []`) et montrées par leur ID court.
- `GET /api/images/{id}` (`{id}` : l'ID complet, 64 caractères hexadécimaux, préfixe `sha256:` accepté ; jamais un nom, qui peut changer de cible entre deux requêtes) : `{id, tags, size, created, os, architecture, variant, config: {env, cmd, entrypoint, commandMasked, workingDir, user, exposedPorts, labels}, layers: [{createdBy, created, size, empty}], containers}`.
  - `config.env` passe par `secrets.Env` (valeurs masquées `null`) ;
  - `cmd` et `entrypoint` par `secrets.Args` ;
  - `labels` par `secrets.Labels` (v0.2 §5.2) ;
  - `layers` vient de `GET /images/{id}/history`. L'historique contient les commandes de construction, avec les arguments de construction (`ARG`) en préfixe : `|2 GH_PAT=… NPM_AUTH=… /bin/sh -c …` (constructeur classique) ou `RUN |2 GH_PAT=… NPM_AUTH=… /bin/sh -c …` (BuildKit). Or `GH_PAT` et `NPM_AUTH` ne contiennent aucun mot de §7 : `secrets.Line` seul les laisserait passer. Règle, dans `secrets.History(createdBy string) string` :
    1. si la ligne commence par `|N ` ou `RUN |N ` (N entier), les mots qui suivent, jusqu'au premier ` /bin/sh -c ` ou ` /bin/bash -c `, sont des affectations : **toutes leurs valeurs** sont remplacées par `••••••`, quel que soit le nom (le nom reste visible). Une valeur peut contenir des blancs, et le nombre N ne suffit donc pas à trouver la fin des affectations. Si aucun ` /bin/sh -c ` ni ` /bin/bash -c ` ne suit, tout le reste de la ligne après `|N` est masqué (échec fermé) ;
    2. puis la ligne entière passe par `secrets.Line` (un `RUN curl -H "Authorization: Bearer …"` y est masqué).

    Tests rouges : `|2 GH_PAT=s3cr3t-a NPM_AUTH=s3cr3t-b /bin/sh -c npm ci`, la même ligne préfixée par `RUN `, une valeur à blancs, une ligne sans shell, un `Bearer`.
  - Aucune révélation dans les images en v0.2 : la révélation d'une variable reste celle d'un conteneur (`env/reveal`).
- « Couches » = les entrées de l'historique. Le nombre de couches réelles est celui des entrées non vides (`empty: false`).

### 5.5 Images : fichiers en lecture seule (F16)

**Principe : aucun conteneur créé.** `docker create` serait une écriture côté Docker. L'agent lit l'image par `GET /images/{id}/get` (l'export de `docker save`), qui est une lecture, et en construit un index des couches, avec les mêmes refus que l'index tar d'un conteneur arrêté (§5.2).

**Format mesuré** (Docker 29.7.2 sur Docker Desktop et 29.8.2 sur hk-ubuntu, magasin containerd dans les deux cas) : une archive tar à la disposition OCI (`blobs/sha256/<hex>`, `index.json`, `oci-layout`) plus un `manifest.json` « docker save », qui donne la configuration et les couches **dans l'ordre**, de la plus basse à la plus haute. Les blobs arrivent **avant** `manifest.json`. Toutes les couches mesurées sont en gzip ; avec l'ancien pilote overlay2, `docker save` les écrit non compressées (format à vérifier en CI, matrice overlay2 de §10).

**Construction de l'index** (`internal/fsview/imageindex.go`)
1. `ImageInspect(id)` donne l'ID de référence. L'export se fait **par ID**, jamais par nom.
2. Lecture en flux de l'archive externe, sous un contexte à délai ; le contexte est testé à chaque tour de boucle (chaque en-tête, externe ou de couche). Pour chaque blob `blobs/sha256/<64 hex>` :
   - son SHA-256 est calculé au passage, sur **tous** ses octets physiques, et doit égaler son nom, sinon `unsupported_image` ;
   - nature, d'après ses 512 premiers octets (`bufio.Reader.Peek`) : gzip (`1f 8b`) ; zstd (`28 b5 2f fd`) → `unsupported_image` (Q62) ; tar non compressé si l'en-tête ustar est valide (`ustar` à l'octet 257, somme de contrôle de l'en-tête juste) ; tout autre blob (JSON de configuration, manifestes) est lu et jeté ;
   - une couche est lue par `archive/tar` **en n'appelant que `Next()`** : le contenu des entrées n'est jamais lu par l'agent. `Next()` saute le reste physique de l'entrée en cours, c'est-à-dire les octets réellement présents dans le flux. À l'inverse, `io.Copy(io.Discard, tr)` sur une entrée creuse (sparse, PAX ou GNU) de taille logique 2⁶⁰ rendrait des zéros sans rien lire du flux, et aucun plafond d'octets ne l'arrêterait. Les octets décompressés comptent sous le `gzip.Reader`, donc sur ce que `Next()` lit réellement ;
   - de chaque en-tête, on ne garde qu'une forme compacte : `{nom, cible, type, taille, mode, uid, gid, mtime, rang}`, où `rang` est la position de l'entrée dans sa couche. `PAXRecords`, `Xattrs`, `Uname` et `Gname` sont jetés. Un nom ou une cible de lien de plus de **4 Kio** → `unsupported_image` (échec fermé : la mémoire par entrée reste bornée) ;
   - une couche garde sa liste d'en-têtes compacts jusqu'à l'arrivée du manifeste (les blobs arrivent avant lui).
   - **En-têtes spéciaux (PAX, noms longs GNU) : plafonnés avant leur lecture complète.** `archive/tar` de Go 1.27 les lit par `readSpecialFile`, soit `io.ReadAll(io.LimitReader(r, maxSpecialFileSize+1))` avec `maxSpecialFileSize = 1 << 20` (`format.go:149`, `reader.go:870`). Un en-tête de plus d'1 Mio est donc refusé (`ErrFieldTooLong` → `unsupported_image`) après au plus 1 Mio + 1 octet lus en mémoire, et chaque encodage creux est borné de même (`format.go:153`). Ce plafond borne la **mémoire** par en-tête. Il ne borne pas une **chaîne** d'en-têtes spéciaux : `Next()` les enchaîne dans une seule boucle interne (`reader.go:80` à `140`), chacun sous 1 Mio, sans rendre la main. C'est le lecteur compteur, placé **sous** `archive/tar` (et sous `gzip`), qui l'arrête : il teste le contexte et les plafonds d'octets (lus et décompressés) à chaque `Read`, donc aussi au milieu d'un seul `Next()`. Le compteur par couche n'est pas nécessaire : la mémoire est bornée par en-tête, et les octets par les plafonds globaux.
   
   Tout autre nom de fichier dans l'archive externe (hors `index.json`, `oci-layout`, `manifest.json`, `repositories`) est sauté par `Next()`.
3. `manifest.json` (64 Kio au plus) : exactement une entrée, et chaque couche de la forme `blobs/sha256/<64 hex>` et présente parmi les blobs lus, sinon `unsupported_image`. Une image multi-plateforme dont plusieurs plateformes sont présentes localement donne plusieurs entrées : elle est refusée en v0.2 (cas rare ; limite de §13).
4. Fusion des couches dans l'ordre du manifeste, de la plus basse à la plus haute, dans un `index` de §5.2 (même `CleanPath`, même `add`, mêmes liens physiques, mêmes entrées `/proc/` et `/sys/` ignorées). Pour chaque couche :
   - d'abord ses blanchiments, appliqués aux couches inférieures : `.wh.<nom>` supprime `<nom>` **et tout son sous-arbre** ; `.wh..wh..opq` vide le dossier de toutes ses entrées inférieures. Un blanchiment dont le `<nom>` est vide, `.` ou `..` (`.wh.`, `.wh..`, `.wh...`) est ignoré : il ne supprime ni le dossier, ni son parent ;
   - puis ses entrées, dans l'ordre de la couche. Une entrée qui **remplace** un chemin d'un autre type supprime d'abord l'ancien. En particulier, un dossier remplacé par un fichier, un lien ou un autre type perd **tous ses descendants**, ceux des couches inférieures compris : ils ne doivent plus être visibles ni résolus. Un même chemin présent deux fois dans une couche : la dernière occurrence gagne, et c'est son `rang` qui est retenu ;
   - les entrées `.wh.*` ne sont jamais montrées.

   Pour chaque chemin visible, l'index garde son **origine** : `{empreinte de la couche, rang}` de l'entrée qui le porte (ou, pour un lien physique, de sa cible). Un nom qui sort de la racine (`../../x`) est ramené dedans par `CleanPath`, comme en §5.2 : l'index n'est qu'une table en mémoire, rien n'est écrit sur le disque, et aucun chemin de l'hôte n'est touché.
5. **Plafonds** (constantes `ImageLimits`) :
   - octets lus de l'archive externe : 4 Gio ;
   - octets décompressés, toutes couches confondues : 16 Gio (bombe de décompression) ;
   - entrées de toutes les couches avant fusion : 1 000 000 ;
   - durée : 5 min ;
   - 4 Kio par nom et par cible de lien ;
   - 256 Mio de noms et de cibles gardés, toutes couches confondues.

   Au-delà → `too_large` (ou `unsupported_image` pour un nom trop long), mis en cache comme en §5.2 étape 4.
6. Cache : la clé est l'ID de l'image, qui est immuable. Les index d'images partagent les **3 places** du cache de `Views` (§5.2 étape 4), pour tenir l'objectif de mémoire au repos ; mêmes règles d'éviction, de retrait après 10 min sans visite, et même singleflight. Pendant la construction : `202 indexing` avec la progression (octets lus / taille de l'image, plafonnée à 0,99).

**Lecture d'un fichier (extraction)** : on ne peut pas se positionner dans un flux. `Open(path)` résout le chemin dans l'index (liens suivis, §5.2 étape 5) et prend son origine `{couche, rang}`. Une entrée caractère, bloc ou FIFO → `not_regular`, sans rien relancer. Sinon, l'agent relance `GET /images/{id}/get`, saute les autres blobs par `Next()`, et décompresse la seule couche utile jusqu'à l'entrée de ce **rang** : pas son nom, car un chemin peut figurer deux fois dans une couche.
- **Contenu lu** : au plus 2 Mio + 1 octet (au-delà, `too_large`, comme l'aperçu de §6). Pour `image-match` (v0.2 §5.7), le contenu passe en flux dans le HMAC, sans être gardé, jusqu'à 256 Mio (au-delà, la réponse vaut `match: false`).
- **Empreinte** : après l'entrée, la lecture continue jusqu'à la **fin de ce blob**, et son SHA-256 doit égaler son nom ; sinon, rien n'est servi (`unsupported_image`). Les blobs suivants ne sont pas lus : la connexion est fermée. Le coût est celui de la couche utile entière, plus les blobs qui la précèdent dans l'archive.
- **Plafonds par extraction** : 4 Gio lus de l'archive externe, 16 Gio décompressés, 2 min.
- **Limites globales** : une extraction à la fois par image, et **deux au plus pour tout l'agent**. Au-delà, la requête reçoit `202 indexing` sans rien lancer, et la relance suivante retente.
- **Extraction longue** : la première requête attend au plus 1 s, puis `202 indexing` avec la progression. L'extraction a son propre contexte, détaché de la requête et borné par la durée de 2 min.
- **Client parti** : si aucune requête n'a demandé ce résultat depuis 15 s, l'extraction est annulée et son créneau est rendu.
- **Résultat** : le contenu extrait (au plus 2 Mio) reste en mémoire 60 s, pour la relance suivante, puis il est oublié. Le résultat d'un `image-match` (un booléen) reste 5 min.
- Pas de téléchargement des fichiers d'une image en v0.2 (Q63).

**Coût mesuré** (export seul, puis export et lecture des en-têtes de toutes les couches avec décompression, en Python ; Go sera au moins aussi rapide) :

| Image | Taille Docker | Export (compressé) | Contenu décompressé | Entrées | Durée (export, décompression, index) |
|---|---|---|---|---|---|
| `alpine:3.20` | 13,7 Mo | 4 Mo | 9 Mo | 517 | 0,1 s |
| `nginx:1.27-alpine` | 151 Mo | 45 Mo | 98 Mo | 4 502 | 0,7 s |
| `node:24.15.0-bookworm-slim` | 345 Mo | 82 Mo | 245 Mo | 7 049 | 1,6 s |
| `postgres:16-alpine` | 411 Mo | 115 Mo | 288 Mo | 3 999 | 1,9 s |
| `golang:1.27` | 1,32 Go | 320 Mo | 900 Mo | 37 224 | 5,2 s (export seul, sans décompression : 3,7 s) |
| `quay.io/keycloak/keycloak:26.3` | 1,42 Go | 490 Mo | 903 Mo | 11 162 | 6,8 s |
| `hublot-test` (hk-ubuntu, 1,37 Go) | | 327 Mo | | | export seul : 2,4 s |

- **Disque** : nul. Rien n'est écrit, l'archive est lue en flux.
- **Mémoire** : celle d'un index de §5.2 (237 octets par entrée mesurés, soit environ 9 Mo pour `golang:1.27`), plus les listes d'en-têtes compacts par couche jusqu'à l'arrivée du manifeste, du même ordre. Bornée par trois plafonds : 1 000 000 d'entrées, 4 Kio par nom ou cible, et 256 Mio de noms et de cibles au total. Au pire, cela fait environ 256 Mo de noms, plus une centaine d'octets de champs par entrée ; en pratique, un chemin fait environ 60 octets.
- **Lecture d'un fichier** : jusqu'à la fin de la couche qui le porte, blobs précédents compris ; au pire, un export complet (fichier dans la dernière couche envoyée). Les durées ci-dessus sont des bornes hautes.
- **Côté démon** : l'export d'une image ne verrouille pas les conteneurs (contrairement à `GET /archive`), mais il occupe un cœur et le disque pendant sa durée.

**Recherche et liste** : `GET /api/images/{id}/fs?path=`, `/fs/file?path=`, `/fs/search?q=` (v0.2 §5.8), mêmes formes de réponse que les routes de conteneur (§6). Aucune route d'écriture ni de téléchargement.

### 5.6 Diff de contenu avec l'image (F17)

Pour un chemin de la liste F5 d'un conteneur :
- **modifié** : à gauche, la version de l'image du conteneur (`GET /api/images/{imageId}/fs/file?path=`, v0.2 §5.5) ; à droite, celle du conteneur (aperçu F2) ; diff unifié avec numéros de ligne (`@codemirror/merge`, Q65) ;
- **ajouté** : le contenu du conteneur seul ; **supprimé** : celui de l'image seule.

Pas de diff si l'un des deux côtés est binaire (octet nul dans les 8 premiers Ko, §6), dépasse 2 Mio, ou n'est pas un fichier régulier : un message le dit. Le diff est calculé dans le navigateur, sans route nouvelle. Le chemin de F5 est celui du conteneur ; il est résolu dans l'index de l'image avec les règles de §5.2 étape 5.

### 5.7 Snapshots (F18)

**Ce qu'est un snapshot** : un **manifeste** de l'état des fichiers d'un conteneur à un instant T, gardé dans le dossier de données de l'agent. **Aucun contenu de fichier n'y est copié** (sinon les secrets y seraient dupliqués). Lecture seule côté Docker.

**Périmètre** : le système de fichiers propre au conteneur (image et couche inscriptible). Volumes, binds, tmpfs, `/proc`, `/sys`, `/dev`, et tout point de montage rencontré, sont notés comme entrées de type `mount` ou `inaccessible`, sans être parcourus (Q58).

**Entrée du manifeste** (une ligne JSON par entrée, dans l'ordre des composants du chemin) : `{path, type: file|dir|symlink|other|mount|inaccessible, size, mode, uid, gid, mtime, link?, mac?, macSkipped?}`.
- `link` : la cible brute d'un lien, comme `Entry.LinkTarget`.
- `mac` : seulement pour un fichier régulier ; `HMAC-SHA256(clé de l'agent, contenu)`, en hexadécimal.
- `macSkipped` : `"too_large"` (fichier de plus de 256 Mio), `"budget"` (plafond d'octets atteint), `"sparse"` (entrée creuse d'une archive) ou `"unreadable"` (erreur de lecture, lien apparu pendant le parcours).

**En-tête** (première ligne) : `{id, containerId, containerName, project, image, imageId, createdAt, finishedAt, state: running|stopped, source: procfs|archive|index, keyId, entries, hashedBytes, partial, limits}`.

**Empreintes et force brute.** Un SHA-256 nu du contenu d'un petit fichier secret se devine hors ligne : un code à 6 chiffres (10⁶ essais) tombe instantanément, et un mot de passe de 8 caractères alphanumériques (62⁸, soit 2·10¹⁴ essais) en quelques heures sur un GPU qui fait de l'ordre de 10¹⁰ SHA-256 par seconde. Or `/run/secrets/db_password` et ses pareils sont exactement des fichiers courts. Parade, en trois couches :
1. **HMAC-SHA256 avec une clé de l'agent** : 32 octets de `crypto/rand` dans `<data-dir>/snapshot.key`, en 0600. Sans la clé, aucune force brute n'est possible.
   - **Création atomique** : la clé est écrite dans un fichier temporaire du dossier de données (`O_CREAT|O_EXCL|O_NOFOLLOW`, 0600), passée par `fsync`, puis publiée par `linkat` sous le nom `snapshot.key`, qui échoue si le nom existe déjà (deux créations concurrentes : une seule gagne, et l'autre relit la gagnante), et enfin le fichier temporaire est supprimé. Aucune clé à moitié écrite n'est jamais visible.
   - **Lecture** : ouverture `O_NOFOLLOW` ; `fstat` du descripteur : fichier régulier, propriétaire de l'agent, aucun droit pour le groupe ni pour les autres, et **exactement 32 octets** (lus jusqu'à la fin). Sinon, la clé est refusée (les règles du dossier de données de §8 s'appliquent aussi), et snapshots et comparaisons de contenu répondent `internal`, avec un détail qui le dit.
   - **Rotation** : la sous-commande `hublot rotate-snapshot-key` (agent arrêté, comme `reset-token`) écrit une clé neuve de la même façon, puis la publie par `renameat` sur `snapshot.key`. Les snapshots déjà pris gardent leur `keyId` : comparés à un snapshot de la nouvelle clé, ils ne se comparent que par les métadonnées (`contentCompared: false`).
   - `keyId` = les 8 premiers octets de `HMAC(clé, "hublot-snapshot-key-id")`, qui ne révèlent rien de la clé.
2. **Les empreintes ne sortent jamais de l'agent** : aucune réponse d'API ne contient de `mac`, seulement des verdicts de comparaison. Il n'y a pas d'export de manifeste en v0.2. Un test vérifie qu'aucune réponse ne contient un `mac`.
3. Limite assumée : la clé et les manifestes vivent dans le même dossier (root, 0700). Qui lit ce dossier a les deux, mais il est root sur l'hôte, et peut donc déjà lire les conteneurs. La parade protège ce qui **sort** du dossier par morceaux (un manifeste copié dans un ticket, une sauvegarde partielle). Supprimer `snapshot.key`, ou en changer, rend les comparaisons de contenu avec les anciens snapshots impossibles : la doc le dit.

**Dossier `snapshots/`** : ouvert par `openat(dossier de données, "snapshots", O_DIRECTORY|O_NOFOLLOW)`, puis contrôlé par `fstat` (dossier, propriétaire de l'agent, 0700, aucun droit pour le groupe ni pour les autres). Un lien ou un autre propriétaire → refus, `internal`. Toutes les opérations qui suivent (création, `renameat`, `unlinkat`, lecture) se font relativement à ce descripteur, jamais par un chemin construit.

Les noms de fichiers du conteneur sont écrits dans les manifestes. Un nom peut être sensible ; il est déjà visible dans l'explorateur, et le dossier est réservé à root. La doc le dit.

**Prise** — `POST /api/containers/{id}/snapshots` (Origin exigé, §7 ; permise en lecture seule, Q57). Une prise à la fois pour tout l'agent ; une seconde demande → `409 conflict`. L'interface demande d'abord une confirmation qui dit le coût (ci-dessous). Journal : `snapshot.create` (intention, puis `ok` ou `error`), `container` = l'ID du conteneur, `path` = l'ID du snapshot.
- La réponse a la forme de `indexing` : `202 {"error": "indexing", "detail": "snapshot in progress", "progress": 0, "snapshot": "<uuid>"}`. L'avancement se suit dans `GET /api/snapshots`.
- **Conteneur actif** (`Views.For` → `procfs`) : un parcours dans `fsview`, sur la racine épinglée par §5.1 étapes 1 à 5. Aucune re-résolution par `OpenatInRoot`, qui suivrait un lien posé pendant le parcours. Chaque chemin visité (dossier ou fichier) s'ouvre par `openNoLinks` (`openat2` avec `RESOLVE_IN_ROOT | RESOLVE_NO_SYMLINKS | RESOLVE_NO_MAGICLINKS`, en `O_PATH`, comme les écritures de §5.3 étape 2). Un lien apparu dans la chaîne depuis le listing → `ErrLinkInPath` : l'entrée est notée `macSkipped: "unreadable"`, sans rien suivre. Puis, sur ce descripteur :
  - `fstatfs` d'abord (§5.1 étape 9) : un pseudo-système → entrée `inaccessible`, non lue ;
  - l'identifiant de montage (`STATX_MNT_ID`), comparé à celui de la racine : s'il diffère → entrée `mount`, non parcourue et non lue ;
  - un dossier est relu par `Reopen(O_RDONLY|O_DIRECTORY)` ; ses noms passent par `fstatat(AT_SYMLINK_NOFOLLOW)` et `readlinkat`, comme `List` (10 000 entrées par dossier au plus, sinon le snapshot est `partial`) ;
  - un fichier : `fstat`, puis tout ce qui n'est pas `S_IFREG` est noté `other`, jamais ouvert ; un fichier régulier est rouvert par `Reopen(O_RDONLY|O_NONBLOCK|O_NOATIME)` (sans `O_NOATIME` si le noyau répond `EPERM`) et lu en flux dans le HMAC, avec un délai de 10 s par lecture (FUSE muet, §13 → `macSkipped: "unreadable"`) ;
  - les liens sont notés avec leur cible brute, jamais suivis.

  Le parcours est en profondeur, les noms de chaque dossier triés : les entrées sortent dans l'**ordre des composants**, qui sert aussi à la comparaison, et s'écrivent au fil du parcours dans le fichier temporaire. Un seul dossier est ouvert par niveau, et la prise passe sous la garde des opérations de fichiers (4 par conteneur). La photo **n'est pas atomique** : un fichier modifié pendant le parcours peut être vu avant ou après sa modification. L'en-tête porte l'heure de début et celle de fin.
- **Conteneur arrêté** : mêmes refus préalables que l'explorateur (§5.2 étape 7 : un bind de l'hôte → `forbidden_path`, sans export). Ensuite, `GET /archive` de `/` est lu en flux, contexte testé à chaque en-tête.
  - Chaque en-tête donne une entrée.
  - Le contenu d'un fichier régulier (`TypeReg`) passe dans le HMAC seulement si sa taille annoncée tient sous le plafond par fichier et dans le budget restant ; la lecture est bornée par cette taille (`io.LimitReader`). Une entrée creuse (sparse) n'est pas hachée (`macSkipped: "sparse"`), car sa taille logique ne dit rien des octets lus. Tout le reste est sauté par `Next()`.
  - Les entrées sous `/proc`, `/sys` et sous une destination de montage (`Inspect.Mounts`, `HostConfig.Tmpfs`) sont sautées sans être notées ; la destination elle-même est une entrée `mount`. Docker les met quand même dans l'archive : l'export **lit aussi les volumes** du conteneur, et cela compte dans le plafond d'octets lus.
  - Les entrées sont triées en mémoire dans l'ordre des composants avant l'écriture.
  - Pendant tout l'export, dockerd **verrouille le conteneur** (registre Interface, v0.1.1) : `docker start`, `docker inspect`, logs, radiographie et écritures sur ce conteneur attendent, jusqu'au plafond de durée.
  - Un second `Inspect` à la fin (même `FinishedAt`, toujours arrêté), sinon le snapshot est jeté (`container_changed`).
- **Photo des métadonnées** (conteneur arrêté seulement) : si l'index de ce conteneur est déjà construit (§5.2, même clé `ID + FinishedAt`), la photo peut en être tirée, sans export ni verrou ni empreintes (`source: "index"`, aucun `mac`). L'interface la propose en premier quand elle est possible. Sa comparaison ne voit que tailles, dates, droits, liens, ajouts et suppressions (`unknown` pour un fichier de même taille et de même date).
- **Plafonds** (constantes `snapshot.Limits`) :
  - 500 000 entrées → abandon, `too_large` : un manifeste incomplet fausserait « supprimé » ;
  - 1 Gio haché au total, puis les fichiers suivants sont notés sans `mac` (`macSkipped: "budget"`, `partial: true`) ;
  - 256 Mio par fichier haché (`macSkipped: "too_large"`) ;
  - conteneur arrêté : 4 Gio lus de l'archive (volumes compris) et 5 min, puis abandon, `too_large` ; le verrou de dockerd dure donc 5 min au plus ;
  - conteneur actif : 10 min, puis abandon, `too_large`.
- **Écriture** : `snapshots/<id>.jsonl.gz` (fichier 0600), créé relativement au descripteur du dossier dans un fichier temporaire, passé par `fsync`, puis publié par `renameat`. Un échec ne laisse aucun fichier partiel (le fichier temporaire est supprimé, et les restes d'un arrêt brutal sont nettoyés au démarrage).

**Coût et plafonds**
- **Hachage** : HMAC-SHA256 seul, mesuré à 0,31 s pour 900 Mio (Python et OpenSSL sur Apple M, soit environ 2,9 Gio/s ; SHA-256 nu : 0,30 s). Le `crypto/sha256` de Go utilise les instructions SHA d'arm64 et d'amd64 quand elles existent ; sans elles, il faut compter environ 0,5 Gio/s, soit 2 s par Gio. La mesure combinée (export de `golang:1.27`, décompression et HMAC) varie de 3,9 à 5,4 s d'un passage à l'autre, avec ou sans HMAC : trop bruitée pour isoler l'écart. La lecture domine.
- **Conteneur arrêté** : environ la durée de l'export (§5.2 : 7 à 8 s pour `node:22`, 33 538 entrées), plus ses volumes, jusqu'à 5 min de verrou.
- **Conteneur actif** : un `openat2`, un `fstatfs`, un `statx` et un `Reopen` par entrée ; jusqu'à 1 Gio lu sur le disque de production, un fichier à la fois. À mesurer en tranche sur les VM (objectif : moins de 30 s pour 40 000 fichiers et 1 Gio).
- **Mémoire** : conteneur actif, bornée (écriture au fil du parcours) ; conteneur arrêté et comparaison « contre l'actuel », toutes les entrées en mémoire : environ 300 octets par entrée (à mesurer), soit environ 150 Mo au plafond de 500 000.
- **Disque** : environ 200 octets par entrée en JSON, de l'ordre de 60 une fois compressé (à mesurer), soit environ 2 Mo pour 37 000 entrées et 30 Mo au plafond.

**Rétention** (Q66) : 20 snapshots par conteneur, 1 Gio au total ; au-delà, la prise est refusée (`413 too_large`, avec un détail qui le dit). Aucune suppression automatique. Les snapshots d'un conteneur supprimé sont gardés et marqués « conteneur supprimé ».

**Suppression** — `DELETE /api/snapshots/{sid}` (Origin exigé), journalisée (`snapshot.delete`, intention puis résultat). Le fichier est supprimé par `unlinkat` sur le descripteur du dossier `snapshots`. Permise en lecture seule, comme la prise (Q57).

**Liste** — `GET /api/snapshots` : les en-têtes, sans `mac` ni `keyId`, plus la prise en cours et sa progression.

**Comparaison** — `GET /api/snapshots/{sid}/compare?with=<sid2>|current`
- Résultat : `{a, b, contentCompared: bool, results: [{path, kind, fields}], truncated}`. `kind` vaut `added`, `deleted`, `modified` (empreinte différente), `meta` (métadonnées seules : `size`, `mode`, `uid`, `gid`, `mtime`, `link`), `type` (changement de type) ou `unknown` (taille et date égales, mais au moins une empreinte manquante : on ne peut rien dire du contenu). Dans l'ordre des composants ; calculée par une fusion en flux des deux manifestes (mémoire constante) ; 10 000 résultats au plus, puis `truncated`. Les dossiers dont seul le `mtime` change sont rangés dans `meta` et masqués par défaut dans l'interface.
- **Entre deux snapshots** : ajouté, supprimé, modifié, sans diff de contenu, puisque rien n'est conservé (Q59). `contentCompared` est faux si les `keyId` diffèrent.
- **Contre le conteneur actuel** (`with=current`) : une prise en mémoire (même parcours, mêmes plafonds, même coût que la prise, rien n'est écrit), en `202 indexing` avec la progression ; elle compte comme la prise unique de l'agent ; résultat gardé 5 min. Pour un fichier `modified`, l'interface montre le **contenu actuel** (aperçu F2). Elle montre un **vrai diff unifié** dans un seul cas : `GET /api/snapshots/{sid}/image-match?path=` répond `{"match": true}`, c'est-à-dire quand le `mac` de l'ancienne version égale le HMAC, calculé à la demande, de la version de ce chemin dans l'image du conteneur (extraction de v0.2 §5.5). L'ancien contenu est alors exactement celui de l'image. La route ne renvoie qu'un booléen.
- Deux snapshots de conteneurs différents se comparent (par exemple avant et après une recréation par Compose, qui change l'ID) : l'interface le propose quand le nom ou le couple projet et service est le même.

**Comptage** — `GET /api/containers/{id}/count` : le même parcours (ou export), sans HMAC ni écriture ; `202 indexing` pendant le calcul, résultat gardé 5 min. Il alimente le chiffre « Fichiers » (v0.2 §5.3).

### 5.8 Recherche globale (F12)

Ouverte par Ctrl+K (Cmd+K sur macOS ; `preventDefault`, car Firefox y place sa propre recherche), par le champ de la barre du haut, ou par l'entrée « Recherche ».

**Ce qu'elle cherche**
1. Conteneurs (nom, ID court, image, projet, service), images (étiquettes, ID court), projets Compose et pages de l'interface : **dans le navigateur**, sur les listes déjà chargées (`/api/containers`, `/api/images`). Aucune route nouvelle, coût nul côté agent.
2. Fichiers : **seulement** dans l'index déjà construit du conteneur ou de l'image actuellement ouvert (Q61) : `GET /api/containers/{id}/fs/search?q=` ou `GET /api/images/{id}/fs/search?q=`.
   - La route ne lance **jamais** de construction. Sans index en cache : `{"indexed": false, "results": []}`, et la palette propose « Ouvrir l'explorateur ».
   - Pour un conteneur actif (`procfs`), il n'y a pas d'index : `indexed: false`.
   - Recherche par sous-chaîne, sans tenir compte de la casse, sur le chemin complet ; requête de 2 à 200 caractères, sinon `400 invalid_query` ; 200 résultats au plus, puis `truncated` ; parcours de l'index en mémoire, sous la garde des routes de fichiers.

**Fuites possibles, et parades**
- Un nom de fichier peut être sensible (`/srv/backup-db-prod-2026.sql.gz`, `/run/secrets/stripe_live_key`). La recherche ne renvoie que des noms que l'explorateur montre déjà à la même session, pour un seul conteneur à la fois, jamais en balayage de tous les conteneurs.
- Rien n'est mémorisé : ni historique de recherche, ni `localStorage`, ni requête dans l'URL de la page.
- L'agent ne journalise pas la requête. Elle part dans l'URL de l'appel d'API (`?q=`) : un reverse proxy qui journalise les URL la verrait. La doc le dit.
- Les valeurs masquées (env, commande, labels) ne sont jamais indexées par la palette, qui ne lit que des noms.

### 5.9 Inventaire des secrets (F19)

`GET /api/secrets` : pour chaque conteneur, `{id, name, project, service, state, env: [noms], args: [arguments masqués], labels: [clés], unavailable}`.
- `env` : les **noms** des variables que `secrets.Masked` masque ;
- `args` : les arguments de la commande et du point d'entrée dans leur forme masquée par `secrets.Args` (ceux qui contiennent `••••••`), comme dans la radiographie ;
- `labels` : les clés des labels masqués par `secrets.Labels` ;
- `unavailable: true` quand l'`Inspect` n'a pas répondu dans les 3 s (conteneur verrouillé par un export, par exemple).

Règles :
- **Jamais de valeur dans la réponse.** Un test donne des valeurs témoins (`s3cr3t-…`) dans l'environnement, la commande et les labels, et vérifie qu'aucune n'apparaît dans le corps.
- Révélation par la route existante seulement (`POST /api/containers/{id}/env/reveal`), journalisée (`env.reveal`), permise en lecture seule (Q28) ; la commande et les labels ne se révèlent pas. Côté interface, une valeur révélée est oubliée dans les mêmes cas que dans la radiographie (§7).
- Coût : un `Inspect` par conteneur, 4 en parallèle au plus, 3 s chacun. Mesuré : 2 à 9 ms par `Inspect`, soit environ 0,1 s pour 39 conteneurs. Pas de cache : la page se recharge à la demande.
- La page montre les conteneurs triés par nombre de secrets, avec un filtre par projet. Une ligne se déplie sur ses variables ; chacune a son bouton « Révéler ».

### 5.10 `/api/meta`

`{version, readOnly, docker: {host, connected, serverVersion, apiVersion}}`.
- `host` : l'adresse que le client Docker utilise (`DOCKER_HOST` ou `unix:///var/run/docker.sock`), passée par `secrets.Line` (une adresse `ssh://user@hôte` ou `tcp://user:pass@hôte` serait masquée) ;
- `connected` : un `Ping` de 2 s au plus ;
- les versions viennent du `Ping` et de la version négociée.

Pour une session seulement, comme toutes les routes (§6).

## 6. API (ajouts à §6)

| Méthode | Route | Rôle |
|---|---|---|
| GET | `/api/meta` | ajoute `docker` (v0.2 §5.10) |
| GET | `/api/containers` | ajoute `project`, `service` (v0.2 §5.1) |
| GET | `/api/containers/{id}` | ajoute `labels` (`[{key, value, masked}]`, v0.2 §5.2) et `finishedAt` |
| GET | `/api/containers/{id}/figures` | v0.2 §5.3 |
| GET | `/api/containers/{id}/count` | v0.2 §5.7 ; `202 indexing` |
| GET | `/api/containers/{id}/fs/search?q=` | v0.2 §5.8 ; `400 invalid_query` |
| GET | `/api/images` | v0.2 §5.4 |
| GET | `/api/images/{id}` | v0.2 §5.4 ; `404 not_found` (ID inconnu ou mal formé) |
| GET | `/api/images/{id}/fs?path=` | v0.2 §5.5 ; `202 indexing`, `too_large`, `unsupported_image` |
| GET | `/api/images/{id}/fs/file?path=` | v0.2 §5.5 ; `202 indexing` (extraction), `not_regular`, `too_large` (plus de 2 Mio) |
| GET | `/api/images/{id}/fs/search?q=` | v0.2 §5.8 |
| POST | `/api/containers/{id}/snapshots` | v0.2 §5.7 ; Origin ; `202` à la forme de `indexing` (plus `snapshot`), `409 conflict`, `413 too_large`, `forbidden_path` ; permise en lecture seule (Q57) |
| GET | `/api/snapshots` | v0.2 §5.7 |
| DELETE | `/api/snapshots/{sid}` | v0.2 §5.7 ; Origin ; `204`, `404 not_found` |
| GET | `/api/snapshots/{sid}/compare?with=` | v0.2 §5.7 ; `202 indexing` pour `current` |
| GET | `/api/snapshots/{sid}/image-match?path=` | v0.2 §5.7 ; `{match}` ; `202 indexing` pendant l'extraction |
| GET | `/api/secrets` | v0.2 §5.9 |

Nouveaux codes :
- `unsupported_image` (422) : couche zstd, manifeste à plusieurs entrées, empreinte de blob fausse, couche absente ;
- `invalid_query` (400).

Le sens de `indexing` (202) s'élargit : un travail en cours (index, extraction, comparaison, comptage, prise), avec `progress`. `{sid}` est un UUID v4 en minuscules, sinon `not_found`. Les routes d'images et de snapshots sont sous la garde des routes de fichiers (§13 : 4 opérations, 10 s), avec pour clé l'ID de l'image ou du conteneur.

## 7. Sécurité (ajouts à §7)

- **Journal d'audit** : nouvelles actions `snapshot.create` et `snapshot.delete`. L'intention est écrite avant, le résultat après ; si l'intention ne s'écrit pas, rien n'est fait.
- **Dossier de données** : il contient en plus `snapshot.key` (0600, exactement 32 octets, écrit par `linkat` ou `renameat`) et `snapshots/` (0700, ouvert sans suivre de lien). Les mêmes contrôles qu'au démarrage s'appliquent (§8 : ni lien, ni autre propriétaire, ni droits pour le groupe ou les autres). Nouvelle sous-commande `hublot rotate-snapshot-key` (v0.2 §5.7).
- **Lecture seule** : `--read-only` reste appliqué dans `files` pour les écritures dans les conteneurs. Les snapshots écrivent dans le dossier de l'agent, pas dans Docker ; ils sont permis en lecture seule, après une confirmation qui affiche leur coût (Q57).
- **Données non fiables** : les couches d'image viennent de registres tiers. Elles ne sont jamais écrites sur le disque, ni extraites, ni exécutées. Le contenu des entrées n'est jamais lu pour l'index (`Next()` seul : une entrée creuse ne peut pas faire tourner l'agent sans fin), les en-têtes sont réduits à une forme compacte (noms et cibles de 4 Kio au plus, PAX et attributs étendus jetés), tout est plafonné (v0.2 §5.5), et chaque blob est vérifié par son empreinte, à l'index comme à l'extraction.
- **Secrets** : `secrets.Labels` (v0.2 §5.2) ; l'historique d'une image passe par `secrets.History` (v0.2 §5.4), sa configuration par `secrets.Args` et `secrets.Env` ; la palette et l'inventaire ne voient que des noms.
- **CSP** : inchangée. L'unique script en ligne reste celui du thème.

## 8. Tests (ajouts à §10)

- **Sécurité d'abord** (tests rouges commités seuls, avant le code), pour chaque tranche qui touche §5 ou les secrets :
  - `imageindex` : blanchiment de fichier et de dossier opaque, blanchiments `.wh.`, `.wh..`, `.wh...` sans effet, dossier remplacé par un fichier ou un lien (descendants disparus), nom `../`, nom absolu, doublon d'un chemin dans une couche (le dernier rang gagne, à l'index comme à l'extraction), lien physique vers une couche inférieure, lien symbolique vers `/etc/passwd` ou `../../..` résolu dans l'index seulement, entrée creuse de taille logique 2⁶⁰ (`TestImageIndexSparseBomb` : l'index se termine sans la lire, sous le plafond de durée), en-tête PAX de 4 Mio refusé sans pic de mémoire (`TestImageIndexHugePAXHeader`), chaîne de mille en-têtes PAX de 900 Kio arrêtée par le plafond d'octets décompressés et par le contexte au milieu d'un seul `Next()` (`TestImageIndexPAXChain`), nom ou cible de plus de 4 Kio, couche non compressée (ustar à l'octet 257), couche zstd → `unsupported_image`, bombe gzip → `too_large`, empreinte de blob fausse (à l'index et à l'extraction), manifeste à deux entrées, couche manquante, FIFO ou périphérique → `not_regular`, plafond d'entrées ; extraction : plafonds, deux au plus pour l'agent, annulée quand le client est parti ;
  - parcours du snapshot sur une racine temporaire : liens piégés jamais suivis, lien posé dans la chaîne entre le listing et l'ouverture (`openNoLinks` → `unreadable`, rien de suivi), FIFO jamais ouverte (le test ne doit pas bloquer), périphérique jamais ouvert, dossier sur un pseudo-système non lu, point de montage non parcouru, aucune lecture hors de la racine, `O_NOATIME` (l'heure d'accès d'un fichier lu ne change pas) ; archive : entrée creuse non hachée, taille annoncée au-delà du plafond non lue ;
  - HMAC : un `mac` n'égale jamais le SHA-256 nu ; clé créée en 0600 par `linkat`, refusée si c'est un lien, si elle est ouverte au groupe, ou si elle ne fait pas exactement 32 octets ; deux créations concurrentes donnent la même clé ; rotation : `keyId` change, et les comparaisons de contenu avec les anciens snapshots tombent à `contentCompared: false` ; `snapshots/` remplacé par un lien → refus ;
  - aucune réponse d'API ne contient de `mac` ;
  - fuite de secrets : labels (le cas Traefik), historique d'image (`|2 GH_PAT=… NPM_AUTH=…`, avec et sans `RUN `, valeur à blancs, ligne sans shell, `Bearer`), inventaire ;
  - aucune valeur témoin dans aucune réponse.
- **Intégration** : une image de test construite en CI avec des blanchiments, un dossier opaque et un lien physique entre couches ; snapshots d'un conteneur actif et d'un conteneur arrêté ; matrice overlay2 et containerd (§10).
- **VM** (`hk-ubuntu`, `hk-fedora`) : exploration d'images et snapshots des cinq cibles de §8, sans refus AppArmor ni AVC.
- **Interface** : Vitest pour la logique (statut traduit, palette, contraste des jetons, niveau des logs, entrées du diff) ; le parcours Playwright est étendu (palette → conteneur → diff de contenu ; page Images ; snapshot puis comparaison).

## 9. Risques (ajouts à §13)

| Risque | Parade |
|---|---|
| Couche d'image malveillante (noms piégés, décompression explosive, des millions d'entrées) | rien n'est écrit ni extrait ; `CleanPath` ; plafonds en octets lus et décompressés, en entrées et en durée ; empreinte de chaque blob vérifiée ; tests rouges |
| Export d'image long sur un gros magasin (plusieurs Go) | progression, cache par ID d'image, plafonds ; l'export ne verrouille aucun conteneur |
| Lire un fichier d'image coûte la lecture de l'archive jusqu'à la fin de sa couche (jusqu'à environ 7 s mesurées pour un export entier) | extraction en `202` avec progression, plafonnée (4 Gio, 16 Gio décompressés, 2 min), deux au plus pour l'agent, annulée sans client, contenu gardé 60 s ; limite assumée pour la v0.2 |
| Image multi-plateforme avec plusieurs plateformes en local | refus `unsupported_image` ; limite assumée pour la v0.2 |
| Format de `docker save` différent selon le magasin (containerd ou overlay2) ou la version de Docker | seule la forme `blobs/sha256/<hex>` est acceptée ; matrice CI overlay2 et containerd ; refus fermé sinon |
| Snapshot d'un conteneur arrêté : l'export lit aussi les volumes et dockerd verrouille le conteneur pendant toute sa durée | plafonds de 4 Gio lus et de 5 min ; photo des métadonnées tirée de l'index déjà construit, sans export ; confirmation qui dit le coût ; une prise à la fois ; Q57 |
| Snapshot d'un conteneur actif : non atomique, jusqu'à 1 Gio lu sur le disque de production | heures de début et de fin dans l'en-tête ; plafond de 1 Gio haché, puis `partial` ; un fichier à la fois, `O_NOATIME` ; Q57 |
| Pic mémoire d'un snapshot d'archive ou d'une comparaison « contre l'actuel » | plafond de 500 000 entrées (environ 150 Mo) ; comparaison entre deux photos en flux |
| Snapshot : noms de fichiers et HMAC sur le disque de l'hôte | dossier root 0700, HMAC à clé, empreintes jamais servies ; limite dite dans la doc |
| Clé de snapshot perdue ou supprimée | les comparaisons de contenu deviennent impossibles (`contentCompared: false`), les métadonnées restent comparables |
| Labels de tiers porteurs de secrets non reconnus par §7 | liste blanche (v0.2 §5.2) ; sans révélation |
| Historique d'image porteur de secrets de construction | `secrets.History` sur chaque `createdBy` (valeurs après `|N` toutes masquées, puis `secrets.Line`) ; les formes non reconnues de §13 restent non masquées, comme pour la commande |
| Nom de fichier sensible dans la recherche | un seul index à la fois, déjà construit, rien de mémorisé ; la requête peut paraître dans les journaux d'un reverse proxy, ce que la doc dit |
| Statut de Docker non reconnu par l'analyseur (autre langue, autre version) | affiché tel quel |
| Niveau de log déduit à tort | marqué comme heuristique ; le flux `stdout` ou `stderr` reste visible |

## 10. Planning

Plan : `docs/superpowers/plans/2026-10-23-v0.2.md`, en tranches verticales. La nouvelle interface sur les fonctions existantes vient d'abord, puis une fonction nouvelle par tranche ; la publication se fait sur ARRÊT (Q67).
