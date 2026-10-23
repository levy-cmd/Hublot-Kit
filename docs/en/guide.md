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

It leads to the pages that exist: Containers (with their count), Diff with the image, Search, Logs, Secrets and Settings. "Search" opens the global search. "Diff with the image" and "Logs" open the matching panel of the chosen container; with no container chosen, the list asks you to choose one.

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

Images will join the palette with the Images page.

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

- **Files**: the number of entries in the index of a stopped container, if it is already built (by opening its files). The agent never starts a build for this figure: without an index, and for a running container, it stays "—".
- **Image size**: the size Docker gives for the container's image, kept in memory per image (an image does not change).
- **Container size**: the writable layer (what the container wrote since its creation), then the total with the image. To compute it, Docker walks the writable layer: the agent asks for it only when the detail opens, never for the list, and keeps it one minute. While a stopped container is being exported, Docker locks it: the figure waits, 30 s at most.

### Labels

Labels often hold secrets the masking does not recognise, for example `traefik.http.middlewares.auth.basicauth.users=admin:$apr1$…`. Only the values of `com.docker.compose.*` and `org.opencontainers.image.*` labels are shown, and only if the masking finds nothing in them; those that give paths of the host machine (`com.docker.compose.project.config_files`, `.working_dir` and `.environment_file`) are masked too. The values shown are chips; the masked labels fit on one folded line, "N other masked labels", which unfolds their names only. No label can be revealed: read its value on the host with `docker inspect`.

## The bottom panels

From 90 rem wide, four panels side by side; below, tabs.

1. **File system**: the explorer. "Explore" shows the whole folder, "Diff" only what changed since the image; in both, a letter marks an added (A) or modified (M) file. For a stopped container, the first opening builds an index, with its progress.
2. **Diff with the image**: the counts of additions, changes and deletions, then the list; a path opens the file in the explorer.
3. **File content**: the preview with its line numbers, the download and, when writes are allowed, editing.
4. **Live logs**: the timestamp, the level and the message. The level is guessed from the text (`ERROR`, `WARN`, `INFO`, `DEBUG`, or a `level` field); it is a heuristic, and otherwise the stream it came from (`stdout` or `stderr`) is shown. "Auto-scroll" follows the end of the stream; turning it off does not pause the stream. "Pause" stops receiving, "Resume" catches up with the lines that arrived meanwhile.

## The Secrets page

The Secrets page takes the inventory of what the agent masks, across every container at once. It shows **names** only, never a value:

- the environment variables the X-ray masks;
- the arguments of the command and the entrypoint where the agent found a secret, by their place and what was detected ("Command, argument 3: value of --requirepass"), never by their text: a partial mask could leave a rest of the value visible;
- the names of the masked labels. Every label outside `com.docker.compose.*` and `org.opencontainers.image.*` is masked as a precaution, whether it holds a secret or not: they are counted apart.

Containers are sorted by number of detected secrets (variables and arguments), then by number of masked labels; the "Project" list filters by Compose project, as on the Containers page. A row unfolds on its variables; "Reveal" goes through the same request as the Secrets tab of the detail, journaled by the agent, and allowed in read-only mode. The command and the labels cannot be revealed.

A revealed value is kept by the page only: it is forgotten when you fold the row, refresh, leave the page or close the tab. Nothing is written to the browser's storage.

The inventory costs one `docker inspect` per container (a few milliseconds), four at a time at most for the whole agent. A container that does not answer within 3 s, for instance because Docker locks it during its export, is marked "unavailable" without holding the others; "Refresh" tries again. Nothing is cached: the page reads Docker again each time it opens.

## A container's page and the phone

Each container has its page, `/c/<id>`, with the Overview, Logs, Diff, Files and Secrets tabs. This is also what a phone opens: the list, then the detail as a full page. The old links of v0.1 (`?tab=changes`, `?tab=xray`) still lead to the right tab. Editing files remains designed for a computer.

## The theme

Dark by default, even when the system asks for light. The toggle of the top bar, or the Settings page, switches to the light theme; the choice is kept in this browser only (`localStorage`, key `hublot-theme`) and applied before the first paint.

## The settings

The Settings page shows the language (the browser's) and the theme, then, read only, the agent's version, read-only mode and Docker: address, state, daemon version and API version. These values are start options of the agent; they cannot be changed from the interface.
