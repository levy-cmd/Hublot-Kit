# Système de design de l'interface : « Quai »

Le hublot regarde dans les conteneurs d'un port. L'interface emprunte au terminal à conteneurs : béton, encre, acier, marquage au sol jaune, peinture d'état. Une seule audace : la plaque. Le reste se tait.

Sources : registre `.claude/handoff/phase-interface.registre.md` (parti pris, interdits) ; jetons dans `web/src/app.css`.

## Jetons de couleur

Chaque jeton existe en clair (« Quai ») et en sombre (« quai de nuit »). Le sombre suit `prefers-color-scheme` ; la bascule de la barre force l'un ou l'autre et le mémorise (`localStorage`, clé `hublot-theme`, seul réglage stocké). Un script en ligne de `app.html` l'applique avant le premier affichage ; l'agent calcule son empreinte CSP au démarrage.

| Jeton | Clair | Sombre | Usage |
|---|---|---|---|
| `--fond` | #E3E6E4 béton | #1E2624 asphalte | fond de page |
| `--fond-releve` | #ECEEED | #26302E | champs, blocs de code |
| `--survol` | #D9DDDB | #26302E | ligne survolée |
| `--encre` | #1B2422 | #DDE3E0 | texte, filets forts |
| `--texte-doux` | #56625F | #9AA6A3 | texte secondaire |
| `--trait` | #6B7876 acier | #6B7876 | bords des champs et boutons |
| `--trait-fin` | #C3C9C6 | #34403D | séparateurs de lignes |
| `--jaune` / `--sur-jaune` | #F2C230 / #1B2422 | idem | sélection, focus, onglet ouvert |
| `--vert` / `--vert-texte` | #2E7D5B / #24654A | #5DB88A | en marche |
| `--ambre` / `--ambre-texte` | #C98A1B / #7A520C | #E0A63A | en pause, redémarre |
| `--rouille` / `--rouille-texte` | #9C3B26 | #E3876B | arrêté, mort, erreur |
| `--acier` / `--acier-texte` | #6B7876 / #56625F | #9AA6A3 | créé, autre état |
| `--porte` / `--porte-texte` | #1B2422 / #E3E6E4 | #2C3734 / #E8ECEA | la plaque |

Les couleurs « peinture » du registre (`--vert`, `--ambre`, `--rouille`) servent aux liserés et aux pastilles. En clair, le vert et l'ambre n'atteignent pas 4,5:1 sur le béton : le texte d'état prend la variante `-texte`, plus sombre.

### Contraste (WCAG 2, calculé)

| Paire | Clair | Sombre |
|---|---|---|
| encre sur fond | 12,64 | 11,88 |
| texte doux sur fond | 5,05 | 6,15 |
| texte doux sur champ (indication) | 5,45 | 5,41 |
| vert-texte sur fond | 5,50 | 6,40 |
| ambre-texte sur fond | 5,50 | 7,13 |
| rouille-texte sur fond | 5,45 | 5,85 |
| états sur ligne survolée (min.) | 4,99 | 5,14 |
| encre sur jaune (sélection, onglet) | 9,48 | 9,48 |
| texte de plaque sur porte | 12,64 | 10,34 |
| libellé de plaque sur porte | 7,38 | 4,91 |
| pastille en marche | 5,00 (blanc sur vert) | 6,40 |
| pastille pause | 5,40 (encre sur ambre) | 7,13 |
| pastille arrêté | 6,85 (blanc sur rouille) | 5,85 |
| pastille autre | 4,59 (blanc sur acier) | 6,15 |
| bord de champ (non textuel, ≥ 3) | 3,65 | 3,37 |
| focus : cerne encre / jaune sur fond | 12,64 | 9,23 |

## Typographie

- **Archivo** variable (poids 100–900, largeur 62–125 %), sous-ensemble latin, `web/static/fonts/archivo-latin-wdth.woff2` (90 Kio). Interface en largeur normale ; titres à 75–87,5 % ; plaque à 62 %, graisse 800.
- **JetBrains Mono** variable, sous-ensemble latin (40 Kio), seulement pour le technique : image, ports, hôte, chemins, condensés, logs, contenu de fichier.
- Licences OFL jointes (`Archivo-OFL.txt`, `JetBrainsMono-OFL.txt`). Aucun CDN. La flèche « → » des ports n'est pas dans le sous-ensemble latin : elle vient de la police de secours.
- Échelle : 0,8125 / 0,875 / 1 / 1,25 / 1,75 rem, titres de page 2,25 rem, nom sur la plaque `clamp(2,25 rem, 4vw + 1 rem, 4,25 rem)`.
- Casse normale partout. Majuscules seulement sur la plaque (`text-transform`), jamais d'espacement large.
- Chiffres tabulaires sur toute la page.

## Espace et forme

- Espacements : 0,25 / 0,5 / 0,75 / 1 / 1,5 / 2 / 3 rem (`--e1` à `--e7`) ; gouttière `clamp(1rem, 3vw, 2rem)`.
- Rayon : 2 px (`--rayon`), partout. Seule forme ronde : l'anneau du hublot dans la marque.
- Pas d'ombre, pas de dégradé, pas de carte. La structure vient des filets : 1 px encre sous les en-têtes, 1 px fin entre les lignes.

## Composants

- **Barre** : marque (anneau + « Hublot-Kit », Archivo 75 %), hôte en mono, navigation (onglet courant souligné d'encre 3 px), « Lecture seule », bascule clair/sombre (`aria-pressed`), déconnexion en simple mot. Sous 40 rem : deux rangées.
- **Liste** : tableau dense (lignes de 2,375 rem), liseré gauche de 4 px à la couleur de l'état, état en texte coloré. Flèches haut/bas, Début, Fin passent d'une ligne à l'autre ; « / » donne la recherche. La ligne au focus clavier devient une bande jaune. Sous 40 rem, chaque ligne devient un bloc : nom et état, image, statut, ports.
- **Plaque** (page d'un conteneur) : porte peinte (`--porte`), marquage au pochoir. Identifiant court en groupes de 6 et 5, le dernier caractère encadré comme le chiffre de contrôle ISO 6346 ; nom en très grand ; un tableau image / état sous un filet, l'état en pastille peinte.
- **Onglets** : texte ; l'onglet ouvert est une bande jaune (même code que la sélection).
- **Boutons** : contour acier 1 px ; primaire en encre pleine. **Champs** : fond relevé, bord acier.
- **Focus** : contour jaune 3 px, cerné d'encre en clair (le jaune seul ne tranche pas sur le béton).

## Règles

- L'état se lit toujours en texte ; la couleur le double, ne le remplace pas.
- Jaune = « vous êtes ici » (focus, sélection, onglet ouvert), rien d'autre.
- Aucune animation d'entrée ; `prefers-reduced-motion` coupe toute transition.
- Interdits du registre : cartes arrondies, ombres douces, dégradés, surtitres en capitales espacées, « → » ajouté aux liens, méta liées par « · ».
- Les contenus des onglets (explorateur, diff, logs, radiographie) n'ont reçu que les jetons ; leur refonte est la tranche 2.
