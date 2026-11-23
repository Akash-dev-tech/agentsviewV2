# Set up AgentsView with Cloudflare R2

## 1. Clone the repository and download the binary

Copy the HTTPS clone URL from this repository's **Code** menu. Run
`git clone <repository-url>` and open the cloned folder in your terminal.

Download the matching file from **Releases** in this repository. Place the
binary in the root of the clone with `.env.example` and `scripts/`.

| System | Download | Check it from the clone |
| --- | --- | --- |
| Windows x64 | `agentsview.exe` | PowerShell: `./agentsview.exe version` |
| macOS Apple silicon | `agentsview-r2-darwin-arm64.tar.gz` | `tar -xzf agentsview-r2-darwin-arm64.tar.gz && ./agentsview version` |
| macOS Intel | `agentsview-r2-darwin-amd64.tar.gz` | `tar -xzf agentsview-r2-darwin-amd64.tar.gz && ./agentsview version` |

Run `uname -m` on macOS if you are unsure which download to choose. Use
`arm64` for Apple silicon and `x86_64` for Intel. The launchers use the
binary from the clone when present, so no separate installation is needed.

## 2. Set your R2 credentials

PowerShell:

```powershell
Copy-Item .env.example .env
```

Git Bash and macOS:

```bash
cp .env.example .env
```

Fill in **`R2_BUCKET`**, **`AWS_ACCESS_KEY_ID`**,
**`AWS_SECRET_ACCESS_KEY`**, and **`AWS_ENDPOINT_URL`** in `.env`. Use credentials
with read access to the bucket and the S3 API endpoint shown in the R2
dashboard. Keep each value unquoted on one line. Windows CRLF line endings work
in both shells.

For teammate lookup, also uncomment and fill in `AV_WORKER_URL` with the
uploader Worker's HTTPS base URL and `AV_ADMIN_SECRET` with its admin secret.

## 3. Configure the session prefixes

Create `~/.agentsview/config.toml` in your home directory. On Windows, use
`$env:USERPROFILE\.agentsview\config.toml` in PowerShell. If you have
an existing config, save a copy before replacing its contents with this setup.

Create the directory if it does not exist:

| PowerShell | Git Bash and macOS |
| --- | --- |
| `New-Item -ItemType Directory -Force "$env:USERPROFILE\.agentsview"` | `mkdir -p ~/.agentsview` |

```toml
s3_only = true

[agents.claude]
dirs = ["s3://${R2_BUCKET}/your-machine/raw/claude"] # EDIT: object prefix

[agents.codex]
dirs = ["s3://${R2_BUCKET}/your-machine/raw/codex"] # EDIT: object prefix

[agents.cursor]
dirs = ["s3://${R2_BUCKET}/your-machine/raw/cursor"] # EDIT: object prefix

[agents.icodemate]
dirs = ["s3://${R2_BUCKET}/your-machine/raw/icodemate"] # EDIT: object prefix
```

Edit only the **`dirs` lines** to match the prefixes in the bucket. Leave
`${R2_BUCKET}` as written. Remove the table for each provider you do not store
and keep at least one. For multiple machines, add one URI per matching prefix
to the array.

For example, if your Codex rollout object starts with
`backups/workstation/codex/sessions/2026/11/22/`, use
`s3://${R2_BUCKET}/backups/workstation/codex/sessions` as the Codex root.
Copy the object prefix from R2 and omit the date folders and filename.

## 4. Sync or open the dashboard

Each script stops the existing daemon for `~/.agentsview` and rescans the
configured R2 prefixes. The serve script then opens the dashboard on loopback.

| What to run | PowerShell | Git Bash |
| --- | --- | --- |
| Sync once | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/r2-sync.ps1` | `bash scripts/r2-sync.sh` |
| Sync and open the dashboard | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/r2-serve.ps1` | `bash scripts/r2-serve.sh` |

On macOS, run `bash scripts/r2-macos.sh` to sync and open the dashboard.

If a previous run imported local sessions, append **`-Clean`** in PowerShell or
**`--clean`** in Git Bash. This moves the existing `sessions.db` and any SQLite
sidecar files into a `sqlite-backup-*` folder under `~/.agentsview` before
syncing into a fresh database. Omit the flag on later runs.

## 5. Verify the sources

While the dashboard runs, list sessions from a second terminal. Replace the
example address with the URL printed by the launcher.

```powershell
./agentsview.exe --server http://127.0.0.1:8080 session list --include-one-shot
```

```bash
./agentsview --server http://127.0.0.1:8080 session list --include-one-shot
```

In Git Bash on Windows, use `./agentsview.exe` in the Bash command.

For a complete source check, run this with the SQLite CLI and substitute
your bucket name in the query.

```powershell
sqlite3 -readonly "$env:USERPROFILE/.agentsview/sessions.db" "SELECT count(*) AS total, count(CASE WHEN coalesce(file_path, '') NOT GLOB 's3://your-r2-bucket/*' THEN 1 END) AS unexpected FROM sessions;"
```

In Git Bash or macOS, use `"$HOME/.agentsview/sessions.db"` for the path.
`total` should be positive and `unexpected` should be `0`. In the browser,
open the URL printed by the serve script. In the sidebar, open **Filters**
and turn off **Hide single-turn** to see sessions with a single prompt.
Clear any project or machine filters that exclude your R2 sessions.

To filter by owner, store transcripts under `users/<email>/...` in the bucket
and configure the provider roots beneath that prefix. In the session sidebar,
open **Filters**, enter an **Owner name** or **Owner email**, and press Enter.
The name filter reads the part of the email before `@` and treats dots,
underscores, and hyphens as spaces. Both filters accept partial matches.

## Package a release

Once the desired commit is on GitHub, open **Actions → Desktop Artifacts →
Run workflow**, choose the branch, and enable `r2_cli`. Download each artifact
from the completed run and attach its contained Windows executable or macOS
tarball to a release for the same commit. The workflow builds for each
architecture and checks that the executable runs before uploading it.
