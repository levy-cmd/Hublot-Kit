# A tour of the interface

This page describes the interface of Hublot-Kit from v0.2 on. To install it, see [Installation](install.md); for what the agent can do and what protects it, see [the security model](security.md).

## The top bar

- On the left, the porthole ring, the name and the real version of the agent (`dev` for a development build).
- In the middle, the field "Search a container, an image, a file…" with the hint `Ctrl K` (`⌘ K` on macOS). It opens the search (below) and searches nothing by itself.
- On the right:
  - Docker's state: "Connected" or "Docker unreachable", followed by the daemon's address (for example `unix:///var/run/docker.sock`). An address that would carry credentials (`tcp://user:password@host`) is masked by the agent;
  - the shield: filled with "Read-only" when the agent started with `--read-only`, outlined with "Writes allowed" otherwise;
  - the theme toggle;
  - the server menu: the agent's address, "Servers", "Settings" and "Log out". Hublot-Kit has no accounts, only a token: the menu is the server's, not a person's.

## The side bar

It leads to the pages that exist: Containers (with their count), Images (with their count), Diff with the image, Search, Logs, Secrets, Snapshots and Settings. "Search" opens the global search. "Diff with the image" and "Logs" open the matching panel of the chosen container; with no container chosen, the list asks you to choose one.

At the bottom, a notice says the agent's real state:

- "Read-only: no writes into the containers" when the agent started with `--read-only`;
- "Writes allowed, always confirmed" otherwise. This is the default: every write goes through a confirmation that names the container and the path.

Under 80 rem wide, the bar shrinks to its icons; under 48 rem, it becomes a drawer opened by the menu button of the top bar.

## The search (Ctrl+K)

`Ctrl K` (`⌘ K` on macOS), the field of the top bar or the "Search" entry open a palette, on every page but the login page.

- It searches the containers (name, short ID, image, project, service), the Compose projects and the pages of the interface, in the list the browser already has: nothing is asked of the agent for that. An exact name comes first, then the start of a name, then a name that contains the search; a container comes before a project or a page.
- It also searches **files**, but only those of the open container (`?c=` or the container's page), and only in the index already built when the explorer opened it. The search never starts an export or a walk: for a running container, or a stopped one the explorer has not opened yet, the palette offers "Open the explorer". It matches the full path, case ignored; names that contain the search come before paths that contain it through a parent directory. 200 files at most, then the palette asks you to narrow the search.
- It returns only names the explorer already shows for that container, never a content, never a masked value (environment, command, labels).
- Keyboard: the arrows choose, Enter opens, Escape closes and gives the focus back where it was; Tab stays in the palette.
- Nothing is kept: no history, no browser storage, no query in the page's address. The file query goes in the body of a POST call to the agent (`/api/containers/{id}/fs/search`), never in an address; the agent does not log it (see [the limitations](limitations.md)).

On the page of an image, the palette searches the files of that image the same way: only in its index already built by the Files tab, never starting an export (`POST /api/images/{id}/fs/search`).

## The Containers page

- The All, Running and Stopped tabs filter the list; a paused or restarting container counts as "Running". The "Status" list picks one exact Docker state.
- The filter field searches the name and the image. It is not kept in the page's address, since a name can be sensitive; the tab, the status and the project are.
- The Project column and the "Project" list come from the Compose labels (`com.docker.compose.project` and `.service`). The list offers the projects present, plus "No project". A project name the secret masking would refuse is not sent: the container then counts as "no project".
- The Uptime column translates Docker's status: `Up 3 hours (healthy)` becomes "up 3 hours, healthy", `Exited (137) 2 days ago` becomes "exited 2 days ago, code 137". A text the interface does not recognise is shown as is.
- Colour always doubles a text: emerald for a running container, amber for an alert (pause, restart, exit with a non-zero code), grey for a clean stop.
- With the keyboard: `/` moves to the filter, the arrows, Home and End move between rows, Enter chooses the container.

## The detail panel

Choosing a container opens its detail on the right: the image (with a copy button), the creation date, the last exit ("never" for a container never stopped), three key figures, then two tabs.

- **Overview**: the command, in the form the agent masked (the copy button copies this form, never a revealed value), the ports, the labels, then, under "Details", the identity, the state, the networks and the mounts.
- **Secrets**: the environment variables. A secret value is masked; "Reveal" asks the agent for it, and the agent journals the reveal. A revealed value is forgotten when you leave the tab or the container.

The "Explore the files", "Compare with the image" and "See the logs" buttons open the bottom panels.

### Key figures

They come after the rest of the detail, without holding it back; "—" means loading, unknown or not available.

- **Files**: the number of entries in the index of a stopped container, if it is already built (by opening its files); else that of the container's last snapshot, with its date; else that of a recent count. The agent never starts any work for this figure by itself: without any of the three, a "Count" button offers a bounded count, after a confirmation that says its cost (a walk of the files without reading their content for a running container; the export, which locks the container for up to 5 min, for a stopped one). The result is kept 5 min.
- **Image size**: the size Docker gives for the container's image, kept in memory per image (an image does not change).
- **Container size**: the writable layer (what the container wrote since its creation), then the total with the image. To compute it, Docker walks the writable layer: the agent asks for it only when the detail opens, never for the list, and keeps it one minute. While a stopped container is being exported, Docker locks it: the figure waits, 30 s at most.

### Labels

Labels often hold secrets the masking does not recognise, for example `traefik.http.middlewares.auth.basicauth.users=admin:$apr1$…`. Only the values of `com.docker.compose.*` and `org.opencontainers.image.*` labels are shown, and only if the masking finds nothing in them; those that give paths of the host machine (`com.docker.compose.project.config_files`, `.working_dir` and `.environment_file`) are masked too. The values shown are chips; the masked labels fit on one folded line, "N other masked labels", which unfolds their names only. No label can be revealed: read its value on the host with `docker inspect`.

## The bottom panels

From 110 rem wide (1760 px), four panels side by side; below, tabs, with the content beside the file system or diff panel above 64 rem (two columns, half the width each).

1. **File system**: the explorer. "Explore" shows the whole folder, "Diff" only what changed since the image; in both, a letter marks an added (A) or modified (M) file. For a stopped container, the first opening builds an index, with its progress.
2. **Diff with the image**: the counts of additions, changes and deletions, then the list; a path opens its content diff in the next panel.
3. **File content**: the preview with its line numbers, the download and, when writes are allowed, editing. Or the content diff of the path chosen in the Diff panel; the last one chosen wins.

### The content diff

For a **modified** file, the unified diff between its version in the container's image and its version in the container, read only. Two columns of numbers: left the line in the image, right in the container; `+` marks an added line (no number in the image), `−` a removed line (no number in the container). An **added** file shows the container version only, a **deleted** file the image version only. The folder icon opens the file in the explorer.

- The image is read by its ID, without a container: the diff also works when the tag that started the container was removed or moved. The first read of a file of the image reads its export again up to the layer that holds it; a progress shows meanwhile, as in the image explorer. For a stopped container, the container's index is built the first time too.
- No diff, and a message that says why, when one of the two versions is binary (a NUL byte in the first 8 KiB, or a PNG, JPEG, GIF or WebP picture), is over 2 MiB, is not a regular file (directory, link, device), or does not exist on that side.
- When the text is the same, the panel says so: only the metadata changed, or only the line endings (CRLF and LF).
- The diff is bounded whatever the files hold (lines × edits at most 10 million, and 1,000 edits). Past that, everything between the common start and end is shown as one replaced block, and the panel says so.
- The diff shows nothing more than the two previews: what the container explorer and the image explorer would show for that path. A secrets file (`.env`, a key) reads in clear, as in the preview. It is computed in the browser.
4. **Live logs**: the timestamp, the level and the message. The level is guessed from the text (`ERROR`, `WARN`, `INFO`, `DEBUG`, or a `level` field); it is a heuristic, and otherwise the stream it came from (`stdout` or `stderr`) is shown. "Auto-scroll" follows the end of the stream; turning it off does not pause the stream. "Pause" stops receiving, "Resume" catches up with the lines that arrived meanwhile.

## The Images page

The Images page lists the images of the daemon, untagged ones included (shown by their short ID), with their size, their date and the containers that use them. It sorts by date, size or name; the field filters by tag, by start of ID or by container name. The intermediate images of the classic builder are not listed.

An image opens on its page, `/images/<ID>`. It is always named by its **full ID** (64 hex characters), never by a name such as `nginx:latest`: a name may change target between two requests. An address with a name or a short ID answers "Not found". An untagged image (`docker rmi -f` of an image in use) thus stays readable as long as it exists.

The page shows:

- the full ID (with copy), the tags, the size, the date and the platform (`linux/arm64/v8`);
- the configuration: the command, the working directory, the user, the exposed ports, the variables and the labels, masked by the same rules as on a container's page. A masked value is not revealed from the image: the reveal, journaled, is done from a container that uses it;
- the layers, that is the build history, newest first. The entries without files (`ENV`, `CMD`, `LABEL`…) are folded. The build arguments (`|2 GH_PAT=… /bin/sh -c …`, `ARG GH_PAT=…`) are masked whatever their name, and a `LABEL` line keeps none of its text: the labels are read in the configuration, filtered;
- the containers of the image, which lead to their detail.

The list costs two requests to Docker, whatever the number of images (the list of images and that of containers); two tabs open at the same time share the same read. The detail costs three. If Docker does not answer within 10 s, the page says so and "Refresh" tries again.

### The files of an image

The **Files** tab of an image's page explores its tree, read only, **without creating a container**: the agent reads the export of the image (`docker save`, a read for Docker) and builds an index of its layers, with the same rules as the explorer of a stopped container. Files deleted by a higher layer do not show; a symbolic link resolves inside the image, never on the host.

- **First opening**: a progress bar while the layers are read (0.2 s for `alpine:3`, about 2 s for `postgres:16-alpine`, 4 s for `golang:1.27`, measured on Docker Desktop). The index stays in memory, in the three slots shared with stopped containers.
- **Preview of a file**: the agent reads the export again up to the end of the layer that holds the file, checks its digest, then shows the content (text or image, 2 MiB at most). Meanwhile the preview shows its progress and asks again every 2 s; choosing another file stops asking, and the agent gives up a read nobody has asked for in 15 s. The content stays in memory for a minute.
- No write, upload or download button: the files of an image are read only.
- An image whose layers the agent cannot read (zstd, several platforms present, altered export) is refused with a message; see [the limitations](limitations.md).

## The Secrets page

The Secrets page takes the inventory of what the agent masks, across every container at once. It shows **names** only, never a value:

- the environment variables a container's Secrets tab shows masked;
- the arguments of the command and the entrypoint where the agent found a secret, by their place and what was detected ("Command, argument 3: value of --requirepass"), never by their text: a partial mask could leave a rest of the value visible;
- the names of the masked labels. Every label outside `com.docker.compose.*` and `org.opencontainers.image.*` is masked as a precaution, whether it holds a secret or not: they are counted apart.

Containers are sorted by number of detected secrets (variables and arguments), then by number of masked labels; the "Project" list filters by Compose project, as on the Containers page. A row unfolds on its variables; "Reveal" goes through the same request as the Secrets tab of the detail, journaled by the agent, and allowed in read-only mode. The command and the labels cannot be revealed.

A revealed value is kept by the page only: it is forgotten when you fold the row, refresh, leave the page or close the tab. Nothing is written to the browser's storage.

The inventory costs one `docker inspect` per container (a few milliseconds), four at a time at most for the whole agent. A container that does not answer within 3 s, for instance because Docker locks it during its export, is marked "unavailable" without holding the others; "Refresh" tries again. Nothing is cached: the page reads Docker again each time it opens.

## The Snapshots page

A snapshot is a **photo of the files** of a container at one moment: for each entry, its path, type, size, permissions, owner, date, the target of a link and, for a regular file, a keyed fingerprint of its content (HMAC-SHA256). **The content is never copied**: the secrets of a file are not duplicated in the agent's directory. The fingerprint never leaves the agent; it only tells whether a file changed.

A snapshot covers the container's own filesystem. Volumes, binds, tmpfs, `/dev`, `/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf` and every mount point are noted without being walked; `/proc` and `/sys` are noted "inaccessible".

Choose a container, then take the photo. A confirmation dialog first says what it costs:

- **running container**: a walk of its files, one at a time, up to 1 GiB read from the disk, 10 min at most. The photo is not atomic: a file changed during the walk may be seen before or after the change;
- **stopped container**: the full export by Docker, volumes included (up to 4 GiB read), during which Docker locks the container, 5 min at most: start, `docker inspect`, logs and writes wait;
- **photo of the metadata**: when the index of a stopped container is already built (its files were opened), it is offered first. It is taken from the index, without export nor lock, but without fingerprints: a comparison will only see sizes, dates, permissions, links, additions and deletions.

One photo (or one count) at a time for the whole agent; the page follows its progress. Taking and deleting a photo are allowed even with `--read-only`: they write in the agent's directory, never in Docker, and are recorded in the audit log (`snapshot.create`, `snapshot.delete`).

Photos are grouped by container, the most recent first. A "partial" photo reached the fingerprint budget (1 GiB) or a directory of more than 10,000 entries: some files are noted without a fingerprint. The agent keeps 20 photos per container and 1 GiB in all; past that, a new photo is refused, and nothing is ever deleted by the agent itself: delete a photo from the page. The photos of a removed container stay, marked "container removed".

### Comparing

"Compare", on a photo, sets it against another photo of the same container (or of a container of the same name, recreated by Compose for instance), or against the **current container**. Each difference has a kind: **added**, **deleted**, **modified** (the fingerprint of the content changed), **metadata** (size, permissions, owner, group, date or link target only), **type changed** (a file turned into a directory, for instance) or **unknown** (same size and same date, but a fingerprint is missing or not of the same key: nothing can be said of the content). Identical entries do not appear. Boxes filter by kind; the directories whose date alone changed are hidden by default. Past 10,000 differences, only the first 10,000 are shown. The "unknown" files come apart: a folded line counts them ("12,345 files that cannot be compared") and lists the first 1,000, and they never take the place of a real change. When the comparison stopped at 10,000 differences, that count is a floor: the line says "At least 12,345".

What the comparison **does not show**: the old content of a file, since it is never copied.

- **Between two photos**: "modified" only says that the fingerprint changed; no content is shown.
- **Against the current container**: the agent reads the container again as for a photo (same cost, same confirmation, one at a time), but writes nothing; the result is kept 5 min. For a file, the page shows its **current content**, with the rules of the preview (secrets, binary, 2 MiB). For a modified file, it shows a **line diff** in one case only: when the fingerprint of the old version equals that of the same file in the container's image. The old version was then exactly the image's, which the agent reads again (the layer's digest checked). Otherwise the page says "old version not kept".
- A photo of the metadata, or a photo taken before a key change (`hublot rotate-snapshot-key`), is compared by its metadata only: a file of the same size and date is "unknown" there.

## A container's page and the phone

Each container has its page, `/c/<id>`, with the Overview, Logs, Diff, Files and Secrets tabs. This is also what a phone opens: the list, then the detail as a full page. The old links of v0.1 (`?tab=changes`, `?tab=xray`) still lead to the right tab. Editing files remains designed for a computer.

## The theme

Dark by default, even when the system asks for light. The toggle of the top bar, or the Settings page, switches to the light theme; the choice is kept in this browser only (`localStorage`, key `hublot-theme`) and applied before the first paint.

## The settings

The Settings page shows the language (the browser's) and the theme, then, read only, the agent's version, read-only mode and Docker: address, state, daemon version and API version. These values are start options of the agent; they cannot be changed from the interface.
