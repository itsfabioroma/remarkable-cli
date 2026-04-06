---
name: remarkable-cli
description: Interact with a reMarkable Paper Pro tablet. Use this skill whenever the user mentions their reMarkable, wants to manage tablet documents (list, upload, download, delete, move), export handwritten notes, change splash screens, manage SSH access, or monitor the tablet for changes. Also use when the user says "remarkable", "tablet", "e-ink", "paper pro", "my notes on the device", or references syncing handwritten content.
---

# remarkable-cli

A Go CLI for reMarkable Paper Pro. One binary, two transports (SSH + Cloud), JSON output by default.

The binary is at the project root: `./remarkable` (or `remarkable` if installed globally).

## First-time setup

Before any command works, the agent must connect to the device:

```bash
# if device is on USB
remarkable connect

# if device is on WiFi (get IP from device Settings > About)
remarkable connect 192.168.1.5

# for cloud-only access (no SSH needed)
remarkable auth
remarkable connect
```

`connect` probes SSH and cloud, saves both. All future commands auto-pick the best transport. If a command fails with "no device configured", run `connect` first.

## Transport rules

Commands auto-select the best available transport:

| Transport | Speed | What it can do |
|-----------|-------|----------------|
| SSH | ~1s | Everything: ls, get, put, rm, mv, mkdir, export, watch, splash, password |
| Cloud | ~20s | Read-only: ls (document listing only) |

SSH is preferred when available. Cloud is the automatic fallback for `ls` when SSH is unreachable (device sleeping, different network).

These commands REQUIRE SSH (will fail on cloud-only with a helpful error):
`export`, `watch`, `splash`, `password`, `setup-key`, `get`, `put`, `rm`, `mv`, `mkdir`

## Commands

### List documents
```bash
remarkable ls
```
Returns JSON array of all documents and folders. Each doc has: id, name, type, parent, lastModified, fileType, pageCount. Documents with `parent: "trash"` are trashed.

### Download a document
```bash
remarkable get "Document Name"
```
Downloads the source PDF/EPUB to the current directory.

### Upload a document
```bash
remarkable put document.pdf
remarkable put document.pdf "Folder Name"
```
Accepts PDF and EPUB only.

### Delete a document
```bash
remarkable rm "Document Name"
```

### Move or rename
```bash
remarkable mv "Old Name" "New Name"        # rename
remarkable mv "Document" "Folder Name"     # move into folder
```

### Create a folder
```bash
remarkable mkdir "Folder Name"
remarkable mkdir "Parent/Child"             # nested
```

### Export handwritten annotations to SVG
```bash
remarkable export "Notebook Name"
remarkable export "Notebook Name" -o /tmp/export
```
Parses the v6 .rm binary format, renders pen strokes to SVG files (one per page). Output dir defaults to `{name}_export/`. Each page is a standalone SVG with correct pen styling (ballpoint, pencil, highlighter, marker, etc).

Requires SSH.

### Watch for changes
```bash
remarkable watch --on-change "echo document {id} was {type}"
```
Polls the device every 2 seconds for document changes. Fires the hook command with `{id}` (document UUID) and `{type}` (created/modified/deleted) placeholders. Runs until Ctrl+C or SIGTERM.

Requires SSH.

### Manage splash screens
```bash
remarkable splash list                    # show current screens + backups
remarkable splash set image.png           # replace sleep screen
remarkable splash set image.jpg poweroff  # replace poweroff screen
remarkable splash restore                 # restore original sleep screen
remarkable splash restore poweroff        # restore specific screen
```
Screen names: sleep (default), poweroff, starting, battery, reboot.
Auto-resizes any PNG/JPG to 1620x2160 (Paper Pro dimensions). Backs up originals before replacing. Disables carousel overlay for sleep screen.

Requires SSH.

### Change SSH password
```bash
remarkable password "newpassword"
```
Updates DeveloperPassword in xochitl.conf (survives firmware updates), restarts xochitl to apply.

Requires SSH.

### Install SSH key (passwordless access)
```bash
remarkable setup-key
```
Copies `~/.ssh/id_ed25519.pub` or `~/.ssh/id_rsa.pub` to the device. After this, no password is needed.

Requires SSH.

### Cloud authentication
```bash
remarkable auth
```
Interactive: prompts for a one-time code from https://my.remarkable.com/device/browser/connect. Stores tokens at `~/.config/remarkable-cli/tokens.json`.

### Disconnect
```bash
remarkable disconnect
```
Forgets saved device config.

## Output format

All commands output JSON by default when stdout is not a terminal (which is the case when an agent calls it). Errors are structured:

```json
{"error": "description with actionable hints", "code": "error_code"}
```

Error codes: `transport_unavailable`, `not_found`, `permission_denied`, `auth_required`, `auth_expired`, `unsupported_operation`, `corrupted_data`, `conflict`

Every error message includes hints about what command to run next to fix the problem. The agent should read the error message and follow the hint rather than retrying or calling with `-h`.

## Ambiguous document names

If multiple documents share the same name, commands return a `conflict` error. The agent should use `remarkable ls` to find the full list and then use a more specific identifier or path.

## Typical agent workflows

### Read what's on the tablet
```bash
remarkable connect 192.168.1.5
remarkable ls
remarkable export "My Notes" -o /tmp/notes
# then read the SVG files in /tmp/notes/
```

### Push a document to the tablet
```bash
remarkable put report.pdf "Work"
```

### Monitor for new handwriting and process it
```bash
remarkable watch --on-change "process_notes.sh {id}"
```

### Customize the device
```bash
remarkable splash set company-logo.png
remarkable password "securepass123"
```
