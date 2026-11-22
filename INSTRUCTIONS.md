# Cloudflare R2 on Windows

Run these steps from the repository root after installing this checkout's
`agentsview.exe` and adding its directory to your Windows PATH.

## 1. Set your R2 credentials

PowerShell:

```powershell
Copy-Item .env.example .env
```

Git Bash:

```bash
cp .env.example .env
```

Edit only **`R2_BUCKET`**, **`AWS_ACCESS_KEY_ID`**,
**`AWS_SECRET_ACCESS_KEY`**, and **`AWS_ENDPOINT_URL`** in `.env`. Use credentials
with read access to the bucket and the S3 API endpoint shown in the R2
dashboard. Keep each value unquoted on one line. Windows CRLF line endings work
in both shells.

## 2. Configure the session prefixes

Create `~/.agentsview/config.toml` under your Windows user profile. If you have
an existing config, save a copy before replacing its contents with this setup.

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

## 3. Sync or open the dashboard

Each script stops the existing daemon for `~/.agentsview` and rescans the
configured R2 prefixes. The serve script then opens the dashboard on loopback.

| What to run | PowerShell | Git Bash |
| --- | --- | --- |
| Sync once | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/r2-sync.ps1` | `bash scripts/r2-sync.sh` |
| Sync and open the dashboard | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/r2-serve.ps1` | `bash scripts/r2-serve.sh` |

If a previous run imported local sessions, append **`-Clean`** in PowerShell or
**`--clean`** in Git Bash. This moves the existing `sessions.db` and any SQLite
sidecar files into a `sqlite-backup-*` folder under `~/.agentsview` before
syncing into a fresh database. Omit the flag on later runs.

## 4. Verify the sources

While the dashboard runs, list sessions from a second terminal. Use your
bucket name so the CLI can read the same TOML configuration.

```powershell
$env:R2_BUCKET = 'your-r2-bucket'
agentsview session list --no-sync
```

```bash
R2_BUCKET=your-r2-bucket agentsview session list --no-sync
```

For a complete source check, run this with the SQLite CLI and substitute
your bucket name in the query.

```powershell
sqlite3 -readonly "$env:USERPROFILE/.agentsview/sessions.db" "SELECT count(*) AS unexpected FROM sessions WHERE coalesce(file_path, '') NOT GLOB 's3://your-r2-bucket/*';"
```

In Git Bash, use `"$HOME/.agentsview/sessions.db"` for the path. The count must
be `0`. Confirm that the session list also contains a known bucket transcript
so an empty archive does not pass the check. In the browser, open the URL
printed by the serve script, clear the session filters, and open that same
transcript. Check that projects and machines match the configured prefixes.
