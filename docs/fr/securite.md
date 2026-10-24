# Modèle de sécurité

## Hublot-Kit est root sur l'hôte

L'agent parle au socket Docker, qui donne l'accès root à l'hôte, et avec `--pid=host` il lit les fichiers de
n'importe quel conteneur par `/proc/<pid>/root`. Qui peut se connecter à Hublot-Kit, ou prendre la main sur l'agent,
est root sur le serveur. Les mesures ci-dessous limitent les erreurs et l'exposition ; elles ne protègent pas contre
un agent compromis.

Traitez le jeton d'accès comme le mot de passe root du serveur.

## Exposition

- L'agent écoute sur `127.0.0.1:7878` par défaut. Sur toute autre adresse sans TLS, il affiche
  `WARNING: listening on … without TLS` au démarrage (légitime dans un conteneur publié sur `127.0.0.1`, ou derrière un reverse proxy).
- Accédez-y par un tunnel SSH, ou par un reverse proxy HTTPS avec `--public-origin`
  ([installation](installation.md#derrière-un-reverse-proxy-https)).
- L'agent n'ouvre aucune connexion sortante : ni télémétrie, ni recherche de mise à jour.

## Connexion et session

- Le jeton fait 32 octets aléatoires, affichés une seule fois. Seule son empreinte SHA-256 est stockée
  (`token.sha256`), et la comparaison se fait en temps constant. `hublot reset-token` le remplace.
- 5 échecs de connexion par minute et par adresse IP, puis `rate_limited`. Chaque tentative est comptée avant la
  lecture de son jeton : une salve de requêtes n'en obtient pas plus. Les échecs sont écrits dans le journal d'audit,
  10 lignes par minute au plus par adresse (une adresse IPv6 compte pour tout son /64) ; au-delà, une ligne
  `suppressed` pour cette adresse, et une minute plus tard une ligne par adresse compte les échecs qui n'ont pas été
  écrits, même s'ils ont cessé.
- Une connexion doit envoyer son premier octet en 5 secondes, une requête être lue en 15 secondes (ses en-têtes en
  10), une édition ou un envoi, une fois connecté, en 10 minutes ; une connexion inactive est fermée au bout d'une
  minute. L'agent garde au plus 512 connexions ouvertes, 64 par adresse (`--max-conns-per-ip`). Un téléchargement, l'aperçu
  d'un fichier (de conteneur ou d'image) ou un flux de logs dont le client ne lit plus pendant 30 secondes est arrêté.
- La session vit en mémoire pendant 12 heures. Son cookie est `HttpOnly`, `SameSite=Strict`, et `Secure` avec TLS ou
  une origine publique en `https`.
- Toute requête autre que GET ou HEAD doit porter un en-tête `Origin` égal à l'origine de confiance
  (`--public-origin`, ou le `Host` de la requête). Les en-têtes `X-Forwarded-*` ne sont jamais crus.
- Rebinding DNS : sans `--public-origin`, l'origine de confiance vient de `Host`, donc un nom rebindé passe le
  contrôle d'`Origin` ; ce sont alors le cookie `SameSite=Strict` et le jeton qui vous protègent, et une page rebindée
  peut épuiser les tentatives de connexion de `127.0.0.1` pendant une minute, ou tenir ses 64 connexions. Exposé hors de `127.0.0.1`, renseignez
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
- Les labels suivent une liste blanche : seules les valeurs des labels `com.docker.compose.*` et
  `org.opencontainers.image.*` sont montrées, si le masquage ci-dessus n'y trouve rien et si ce ne sont pas des chemins
  de l'hôte (`config_files`, `working_dir`, `environment_file` de Compose). Les autres montrent leur nom,
  jamais leur valeur, et ne se révèlent pas : un label `basicauth.users` de Traefik porte une empreinte de mot de passe
  que les règles ci-dessus ne reconnaissent pas.
- Une valeur masquée se révèle une à une, et chaque révélation est écrite dans le journal d'audit (le nom, jamais la
  valeur) avant l'envoi de la valeur.
- La page Secrets (`GET /api/secrets`) ne renvoie que des noms : variables masquées, arguments par leur place et ce qui y a été détecté
  (jamais leur texte, qu'un masque partiel laisserait dépasser), clés des labels masqués. Elle n'a pas de révélation à elle : une valeur passe par la même demande
  journalisée que la radiographie, une variable à la fois, sous le contrôle d'`Origin`. Comme toute réponse de l'API,
  elle n'est pas gardée en cache (`Cache-Control: no-store`), et l'interface n'écrit aucune valeur dans le stockage du
  navigateur. Son coût est borné : quatre `docker inspect` à la fois pour tout l'agent, 3 s chacun, 15 s et
  1 000 conteneurs par inventaire ; au-delà, un conteneur est marqué indisponible.
- C'est une heuristique : [certaines formes ne sont pas reconnues](limites.md#masquage-des-secrets). Les logs ne sont
  jamais masqués.

## Snapshots

- Un snapshot ne garde aucun contenu de fichier : des métadonnées et, pour chaque fichier régulier, un
  HMAC-SHA256 de son contenu, avec une clé de l'agent (`snapshot.key`, 32 octets, 0600, dans le dossier de données).
  Un SHA-256 nu d'un petit fichier secret (un code, un mot de passe court) se devinerait hors ligne ; sans la clé,
  non. Ni les empreintes ni l'identifiant de la clé ne sortent de l'agent : aucune réponse de l'API n'en contient.
- La clé est créée de façon atomique (fichier temporaire, `fsync`, `linkat`) et relue sans suivre de lien ; elle est
  refusée si elle n'est pas un fichier régulier de l'utilisateur de l'agent, si le groupe ou les autres y ont un droit,
  ou si elle ne fait pas exactement 32 octets. `hublot rotate-snapshot-key` (agent arrêté) en écrit une nouvelle : les
  snapshots pris avec l'ancienne ne se comparent plus que par leurs métadonnées. Supprimer ou changer la clé a le même
  effet.
- Les manifestes vivent dans `snapshots/` (0700), ouvert sans suivre de lien et contrôlé par son descripteur ;
  chaque fichier y est écrit en 0600, relativement à ce descripteur, publié par `renameat`. **Les noms des fichiers du
  conteneur y sont écrits** : un nom peut être sensible ; il est déjà visible dans l'explorateur, et le dossier est
  réservé à root. La clé et les manifestes sont dans le même dossier : qui le lit a les deux, mais il est root sur
  l'hôte. La clé protège ce qui sort du dossier par morceaux (un manifeste copié dans un ticket, une sauvegarde
  partielle).
- Le parcours d'un conteneur en marche ouvre chaque chemin depuis la racine épinglée sans suivre aucun lien
  (`openat2` avec `RESOLVE_NO_SYMLINKS`) : un lien posé pendant le parcours n'est jamais suivi, le fichier est noté
  illisible. Il ne lit ni pseudo-système, ni FIFO, ni périphérique, ne traverse aucun point de montage, et lit avec
  `O_NOATIME` (les heures d'accès du disque de production ne changent pas). L'export d'un conteneur arrêté passe par les
  mêmes refus que l'explorateur (un bind de `/`, `/proc`, `/sys` ou `/dev` de l'hôte, jugé tel qu'écrit et tel que
  résolu sur l'hôte) ; son contenu n'est haché que sous les plafonds, jamais pour une entrée creuse.

## Journal d'audit

`audit.log` dans le dossier de données, une ligne JSON par événement : connexions échouées, variables révélées,
fichiers édités et envoyés, snapshots pris et supprimés. Une connexion échouée note aussi l'adresse IP de la connexion TCP (jamais
`X-Forwarded-For`). Il est en ajout seul et n'est pas tourné.

## Socket Docker

Monter le socket en `:ro` ne protège rien : c'est un socket, pas un fichier. Un proxy de socket Docker qui n'autorise
que les points d'API utilisés par Hublot-Kit (`_ping` ; conteneurs : liste, inspection, changements, logs, archive ;
volumes : inspection) réduit ce que l'agent peut demander à Docker, mais avec `--pid=host` l'agent reste root sur
l'hôte.

## Signaler une vulnérabilité

Merci d'utiliser le signalement privé de vulnérabilités de GitHub (onglet **Security** du dépôt, **Report a
vulnerability**) plutôt qu'un ticket public.
