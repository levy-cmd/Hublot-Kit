# Revue de la tâche 4 : paquet `secrets` (2c90892..854d549)

Méthode :
- Le code et les tests commités sont identiques, au caractère près, à ceux de la consigne (`diff` vide sur les deux fichiers).
- Copie du paquet dans le scratchpad (`sec4/`). Soixante-dix cas anciens rejoués (`probe_test.go`, `probe2_test.go`), puis environ 150 cas nouveaux (`probe3_test.go`, `p4_test.go`) et des mesures de temps (`perf_test.go`).
- Les corrections proposées pour I2 et m2 sont prototypées dans `sec4fix/`. La suite commitée y passe.
- Numéros de ligne : `internal/secrets/secrets.go` au commit 854d549.

### Spec Compliance

- ❌ **Écart sur un point** (I3) :
  - Q27 range `Pwd` parmi les clés secrètes « dans une affectation », et compte `--nom=valeur` comme une affectation.
  - Dans `Args`, la branche `whole` (l. 225) utilise `SecretName` et non `secretKey`.
  - Résultat : `["app","--pwd=s3cr3t"]` et `["app","Pwd=s3cr3t"]` sortent en clair, alors que `sh -c "app --pwd=s3cr3t"` et `OPTS=--pwd=s3cr3t` sont masqués.
- Tout le reste de la consigne est conforme. Interfaces exactes : `Mask`, `SecretName`, `URLCredentials`, `Masked`, `Line`, `Var`, `Env`, `Lookup`, `Args`. Les tests précèdent le code : c1c0f7e puis 854d549, avec la preuve rouge dans le rapport. Commits en français, avec le Co-Authored-By.
- ⚠️ **Non vérifiable dans ce diff :**
  - les appelants (tâche 5) : que `State.Error` passe par `Line` et que `commandMasked` reprenne le `masked` d'`Args` ;
  - qu'un délai ou un plafond borne le travail de masquage par requête (voir I1).

### Strengths

- `Env` ne copie jamais une valeur masquée : `Value` reste `nil` et le JSON vaut `null` (l. 63-71). Il n'y a donc rien à fuir par construction.
- Les formes courantes sont toutes masquées :
  - `redis-server --requirepass`, `mysqld --password=` ;
  - `PGPASSWORD="…"` et `'…'`, `--env=DB_PASSWORD=` ;
  - JDBC `?password=`, .NET `;Password=`, ODBC `;Pwd=`, JAAS `password="…"` ;
  - `-D…\=` ;
  - `REDIS_ARGS=--requirepass` ;
  - les URL dont le mot de passe contient `@` ou `:` (`postgres://u:p@ss:s3cr3t@db`, motif glouton jusqu'au dernier `@`) ;
  - `%40`, IPv6 ;
  - les `--opt` répétés (`--password a --password s3cr3t`, `--password= --password= s3cr3t`), car `prev` est lu dans `args` et non dans `out` ;
  - le base64 (`--token=czNj…==`, `--token czNj…== --x`) ;
  - les variantes `$'…'`, `${X:-…}`, `"$(…)"x`, `<(…)` ;
  - la casse mixte (`--PaSsWoRd`, `PaSsWoRd=`), `PASS_WORD`, `DB_PASSWD`, `--db-passwd` ;
  - le zéro largeur (`PASS​WORD`, par `PASS`), le K Kelvin et le s long (par `(?i)`).
- Les contrôles R11 tiennent : `kc.sh`, `/run/secrets/db_password`, `ssh-keyscan`, `passwd root`, `keydb-server`, `--cert=/etc/ssl/private/…`, et les messages de Docker qui contiennent un chemin.
- Chaque piège a son contrôle, et les mutations du rapport sont plausibles et armées.

### Issues

#### Critical

Aucun. Aucune forme courante de secret ne sort.

#### Important

**I1. Coût quadratique, déclenché par le contenu de l'image ou du conteneur (DoS de l'agent)**
- **Où :** `secrets.go:137` (`maskText` se rappelle sur chaque valeur imbriquée) et `secrets.go:165-190` (`leaks` remonte et redescend tout le jeton pour chaque mot trouvé).
- **Mesuré (darwin/arm64) :**

| Entrée | Temps |
|---|---|
| `Masked("LABELS", "k=v," × 1000)`, 4 Ko | 122 ms |
| `Masked("LABELS", "k=v," × 4000)`, 16 Ko | 2,8 s |
| `Masked("X", "a=" × 8192)`, 16 Ko | 5,6 s |
| `Masked("X", "a=" × 65536)`, 128 Ko | **7 min 29 s** |
| `Line("key" × 43690)`, 128 Ko | 5,8 s |
| `Args(["app","--attrs=" + "service.name=x," × 1000])`, 15 Ko | 0,64 s |

- **Pourquoi c'est Important :**
  - L'`ENV` et le `CMD` viennent de l'image (Docker Hub) ou de celui qui crée le conteneur. Une valeur de 128 Ko passe `execve` (`MAX_ARG_STRLEN`).
  - Chaque radiographie brûle alors un cœur pendant des minutes. Le contexte ne l'interrompt pas, et « Recharger » relance le calcul.
  - La récursion atteint aussi 64 000 niveaux de pile.
- **Correction :**
  - Plafond fermé : `if len(value) > 8<<10 { return true }` dans `Masked`, et `Mask` dans `Line` et `Args` au-delà du même seuil.
  - Et/ou une passe linéaire : trouver toutes les clés `NOM\?=` d'un coup, masquer la valeur des clés secrètes, puis sauter les clés incluses dans une valeur déjà masquée.
  - Ajouter un test de temps, par exemple `Masked("X", strings.Repeat("a=", 1<<16))` en moins de 100 ms.

**I2. Le repli fermé « local » est trompé par le masque d'une autre clé dans le même mot**
- **Où :** `secrets.go:176`. « Couvert » signifie qu'un masque, n'importe lequel, termine le jeton, et non que **la valeur de ce mot-là** est masquée, comme R12 l'exige.
- **Fuites mesurées :**
  - `["app","--opts=password:s3cr3t,token=abc"]` → `--opts=password:s3cr3t,token=••••••` ;
  - `["sh","-c","app password:s3cr3t,token=abc"]` → idem ;
  - `["app","-Dauth=password:s3cr3t;key=abc"]` → `…password:s3cr3t;key=••••••`.
- **Correction, prototypée (`sec4fix/`, `TestArgs` et `TestLine` verts, les trois cas deviennent `••••••`) :** le masque doit suivre directement le premier `=` ou `:` qui vient après le mot.

  ```go
  if j := strings.IndexAny(after, "=:"); j >= 0 && strings.HasPrefix(after[j+1:], Mask) && !alnum.MatchString(after[j+1+len(Mask):]) { continue }
  ```

  Ajouter ces cas à `TestArgs`.

**I3. `--pwd=` et `Pwd=` en argument : écart à Q27**
- **Où :** `secrets.go:225` (`case SecretName(w[1])`).
- **Fuites :** `["app","--pwd=s3cr3t"]` et `["app","Pwd=s3cr3t"]` sortent en clair. Les mêmes formes sont masquées dans `sh -c` et dans `Env`, et Q27 les range parmi les affectations secrètes.
- **Correction :** `secretKey(w[1])` pour une forme de drapeau (`-{1,2}pwd=`), ou partout. Dans ce dernier cas, `env PWD=/srv cmd` sera masqué ; à arbitrer. Ajouter le cas à `TestArgs`.
- Fréquence faible, mais c'est une exigence écrite et une fuite mesurée.

#### Minor

1. **JSON dans la commande** (`secrets.go:149`, `blank` contient `"`).
   - Fuites : `["curl","-d","{\"password\":\"s3cr3t\"}","http://x"]` et `sh -c "curl -d '{\"user\":\"a\",\"password\":\"s3cr3t\"}' …"`. Le `:` est hors du jeton, et le mot passe pour un mot ordinaire.
   - §13 ne documente le JSON que pour l'environnement.
   - Correction prototypée et verte : `jsonKey = ^"\s*:\s*"` sur `s[en:]` rend `bound`. `exec: "/opt/keycloak/bin/kc.sh": permission denied` reste lisible. Sinon, documenter en §13.
2. **Blanc échappé** (`secrets.go:89`, `value`).
   - `["sh","-c","PGPASSWORD=s3\\ cr3t psql"]` donne `PGPASSWORD=•••••• cr3t psql`. Même chose pour `--password s3\ cr3t`.
   - Correction : ajouter `\\.` aux alternatives de `value` et de `flagValue`.
3. **Secrets hors des mots de §7, à écrire en §13 :**
   - `curl -H 'Authorization: Bearer s3cr3t'`, et `HEALTH=…Bearer…` en environnement ;
   - `curl -u admin:s3cr3t`, `-p=s3cr3t` ;
   - en environnement : `OPTS=token:s3cr3t`, `password: s3cr3t` (forme YAML).
   - Le §13 actuel ne cite que `mysql -pX`, `password = …` et le JSON.
4. **Masquage à tort sans révélation possible :** un mot de §7 dans la **valeur** d'un drapeau neutre rend tout le jeton suspect (`isFlag`, l. 179).
   - `["kc.sh","start","--hostname=my.keycloak.org","--db-username=keycloak"]` → `["kc.sh","start","••••••","••••••"]` ;
   - `sh -c "/opt/keycloak/bin/kc.sh start --hostname=my.keycloak.org"` → `••••••` en entier ;
   - `sh -c "export PATH=/opt/keycloak/bin:$PATH && kc.sh start"` → `••••••` (le `:` du PATH empêche l'exemption des chemins) ;
   - `--db=monkeydb` → masqué ;
   - `sh -c "keytool … -storepass:file /run/secrets/p"`, la bonne pratique, → `••••••` ;
   - `State.Error` `failed to read secret key: permission denied` → `••••••` ;
   - `app 2>&1 | grep -v key:` et `echo 'token: none'` → masqués en entier.
   - Correction prudente : un mot placé après le premier `=` d'un drapeau n'est pas « drapeau », sauf si le segment commence par `-`. `sh -c "app --opt=--password s3cr3t"` doit rester masqué ; il l'est aujourd'hui.
5. **Règle de la sous-chaîne dans la commande, sans révélation (Q27) :** `--password-file /run/secrets/x`, `--api-key-file=…`, `--tls-key /etc/ssl/tls.key`, `etcd --key-file=…`, `--private false`, `--max-tokens=4096`, `--sort-key=name`, `--passthrough=true`, `-Dkeycloak.profile=prod`.
   - Dans l'environnement, `POSTGRES_PASSWORD_FILE=/run/secrets/db`, `JAVA_OPTS=… -Dkeycloak.profile=prod`, `LOG=key=value` et `SORT=--sort-key=name` sont masqués, mais révélables.
   - C'est conforme à la lettre de §7. À envisager : exempter une valeur qui est un chemin absolu après un nom en `-file` ou `_FILE`.
6. **`Line` avale la fin d'un message** (l. 89, un guillemet non fermé court jusqu'à la fin). `exec: "--token=s3cr3t": unknown` devient `exec: "--token=••••••` : la fin `": unknown` disparaît. Côté sûr, mais diagnostic perdu.
7. **Unicode :** ces entrées ne sont pas masquées.
   - Les entrées : `PаSSWORD=` (а cyrillique), `ＰＡＳＳ=` (pleine largeur), `--pаssword=s3cr3t`, `--ｐａｓｓｗｏｒｄ=s3cr3t`.
   - Pas de menace réaliste : aucune application n'analyse ces drapeaux, et l'auteur de l'image connaît déjà le secret.
   - Incohérence mineure : `SecretName` (`ToUpper`, l. 23) ignore le K Kelvin, que `word` (`(?i)`, l. 125) reconnaît.
   - À documenter, ou à ignorer.
8. **Lisibilité, l. 223 :** `case A, w == nil:` mêle deux conditions par une virgule (`A || w == nil`). Mieux vaut deux `case`, ou un commentaire.

### Assessment

**Task quality:** Needs fixes.

**Reasoning :**
- Le masquage tient sur toutes les formes courantes, et les contrôles R11 passent.
- Trois défauts restent à corriger :
  - le coût quadratique, piloté par le contenu de l'image (minutes de CPU pour une valeur de 128 Ko) ;
  - la couverture « locale » trompée par le masque d'une autre clé du même mot ;
  - l'écart `--pwd=` à Q27.
- Chacun se corrige en quelques lignes, et deux corrections sont déjà prototypées et vertes.
