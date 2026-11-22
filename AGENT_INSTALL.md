# Ask an agent to set up AgentsView for Cloudflare R2

Open your clone in a coding agent with terminal access and give it this prompt.

> Follow AGENT_INSTALL.md and set up AgentsView on this computer. Create
> `.env` from the template if needed, then tell me when to fill it in. Once I
> confirm it is ready, finish the setup and give me the working dashboard URL.

Your part is to fill in `R2_BUCKET`, `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, and `AWS_ENDPOINT_URL` in the clone's `.env`. Use
read access to the bucket. Enter the account's R2 S3 API endpoint, without a
bucket path, and keep the existing defaults in the template.

## Instructions for the agent

Work from the repository root. Read [INSTRUCTIONS.md](INSTRUCTIONS.md) for
platform commands and [AGENTS.md](AGENTS.md) for repository rules.

1. **Prepare `.env`.** Confirm Git ignores `.env` with
   `git check-ignore .env`. If the file is missing, copy `.env.example` to
   `.env`. Preserve existing values. If any of the four required values are
   missing or still placeholders, ask the user to fill them in locally and
   confirm when ready. Continue installing the binary while they do that.
   Keep credentials out of chat, terminal output, command arguments, and Git.
   Parse the file as literal key-value pairs and pass credentials through the
   environment of the process that needs them.
2. **Install and check the fork's binary.** Use the matching download from
   this repository's GitHub Releases if available. On Windows x64, place
   `agentsview.exe` in the clone root. On macOS, extract the archive matching
   `uname -m` into the clone root. If the download is unavailable, build this
   checkout with `make build-release`. Use the repository's Go, C compiler,
   Node, npm, and Make requirements. On Windows, run the build in MSYS2 with
   `make SHELL=/usr/bin/bash build-release` and copy the output named
   `agentsview` to `agentsview.exe`. Check the version by invoking the binary
   by its path in the clone. The fork's R2 support is required for the config
   and launchers below.
3. **Discover the bucket's real session prefixes.** Use a local S3 client to
   list object keys read-only from the bucket in `.env`. With AWS CLI, run
   `aws s3api list-objects-v2 --bucket <bucket> --endpoint-url <endpoint>
   --region auto --query "Contents[].Key" --output json` with the credentials
   set in its process environment. Use an equivalent authenticated S3 client
   if AWS CLI is unavailable. Match transcript keys to supported providers
   and retain each distinct provider root. For Codex, if a rollout key is
   `machine/codex/sessions/2026/11/22/rollout-….jsonl`, use
   `machine/codex/sessions` as the root. A harvested archive can instead have
   a root like `machine/raw/codex`. Skip unrelated files. If the listing
   fails, report the error without exposing credentials. If there are no
   supported transcripts, report that before treating setup as successful.
4. **Configure AgentsView.** Back up any existing
   `~/.agentsview/config.toml` before editing. Set `s3_only = true` at the
   top level and configure `[agents.<provider>]` with a `dirs` array of
   discovered `s3://${R2_BUCKET}/<prefix>` URIs. Keep the literal
   `${R2_BUCKET}` token for the launchers to expand. Include only providers
   with matching objects and preserve unrelated settings. If existing
   settings conflict with R2-only mode, explain the conflict and keep the
   backup. Preserve any existing session database. If it contains other
   sessions, back it up before switching to a fresh R2-only database as
   described in [INSTRUCTIONS.md](INSTRUCTIONS.md).
5. **Sync and start the dashboard.** From the clone root, run
   `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/r2-serve.ps1`
   on Windows or `bash scripts/r2-macos.sh` on macOS. Keep the launcher running
   and use the loopback URL it prints. If sync imports zero sessions, check
   the actual object keys and the configured prefixes before handing off.
6. **Verify the result.** Query the running dashboard with the clone's binary.
   Replace the example URL with the one printed by the launcher. On macOS,
   use `./agentsview` in the command below.

   ```powershell
   ./agentsview.exe --server http://127.0.0.1:8080 session list --include-one-shot --include-automated --include-source --json
   ```

   Confirm that the result contains sessions and that their source paths
   point to the configured R2 bucket. Use the SQLite check in
   [INSTRUCTIONS.md](INSTRUCTIONS.md) to check the entire archive when needed.
   In the browser sidebar, turn off **Hide single-turn** if the default
   session list is empty. Enable **Include automated sessions** if applicable.
   Report the dashboard URL, the verified session count, and any blockers.
