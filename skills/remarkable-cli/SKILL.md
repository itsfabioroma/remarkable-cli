---
name: remarkable-cli
description: Interact with a reMarkable Paper Pro tablet. Use this skill whenever the user mentions their reMarkable, wants to manage tablet documents (list, upload, download, delete, move), export handwritten notes, write text onto notebook pages, manage pages and tags, change splash screens, manage SSH access, or monitor the tablet for changes. Also use when the user says "remarkable", "tablet", "e-ink", "paper pro", "my notes on the device", "write on the tablet", or references syncing handwritten content.
---

# remarkable-cli

Go CLI for reMarkable Paper Pro. One binary, two transports (SSH + Cloud), JSON output by default.

Install: `go install github.com/itsfabioroma/remarkable-cli@latest` or `remarkable` if already built.

## First-time setup

```bash
remarkable connect              # USB (10.11.99.1)
remarkable connect 192.168.1.5  # WiFi
remarkable auth                 # cloud access (optional)
```

`connect` probes SSH and cloud, saves both. All future commands auto-pick the best transport. SSH is fast (~1s), cloud is fallback (~3s).

## Commands

### Documents
```bash
remarkable ls                          # list docs (JSON, trash filtered)
remarkable ls --all                    # include trashed docs
remarkable ls --tag "work"             # filter by tag
remarkable get "Document Name"         # download PDF/EPUB
remarkable put report.pdf "Folder"     # upload
remarkable rm "Old Draft"              # delete
remarkable mv "Draft" "Final"          # rename
remarkable mv "Doc" "Folder"           # move into folder
remarkable mkdir "Projects"            # create folder
```

### Write text onto pages
```bash
remarkable write "Notebook" --text "Hello from the agent" --new-page
remarkable write "Notebook" --text "Appended below existing content" --page 2
remarkable write "Notebook" --text "Long text auto-wraps and scales to fit"
```
Text appears as pen strokes (Fineliner). Auto-scales based on length — short text is large, long text wraps. When writing to an existing page, appends below existing content. Requires SSH.

### Export and read handwritten pages
```bash
remarkable export "Notebook"                   # all pages → PNG
remarkable export "Notebook" --page 19         # single page → PNG
remarkable export "Notebook" --page 19 --svg   # SVG instead
remarkable export "Notebook" -o /tmp/notes     # custom output dir
```
Default output is PNG (readable by AI agents). Falls back to cloud when SSH unavailable. To read a page: export it, then read the PNG file.

**Agent workflow to read handwriting:**
```bash
remarkable export "Main" --page 19 -o /tmp/notes
# then read /tmp/notes/page_019.png with your vision capability
```

### Page management
```bash
remarkable pages "Notebook"                                     # list pages
remarkable pages add "Notebook"                                 # add blank page at end
remarkable pages add "Notebook" --template "P Grid medium"      # with template
remarkable pages add "Notebook" --after 3                       # insert after page 3
remarkable pages rm "Notebook" --page 5                         # delete page
remarkable pages move "Notebook" --page 5 --to 2                # reorder within notebook
remarkable pages copy "Source NB" --page 3 --to "Dest NB"       # copy to another notebook
remarkable pages move-to "Source NB" --page 3 --to "Dest NB"    # move to another notebook
```
Requires SSH.

### Tag management
```bash
remarkable tag "Notebook" "work"                     # add doc tag
remarkable tag "Notebook" "work" --rm                # remove tag
remarkable tag "Notebook" "important" --page 3       # tag a page
remarkable tags                                      # list all tags
```
Requires SSH.

### Splash screens
```bash
remarkable splash list                    # show current screens
remarkable splash set image.png           # replace sleep screen (auto-resize)
remarkable splash set image.jpg poweroff  # replace specific screen
remarkable splash restore                 # restore original
```
Screens: sleep, poweroff, starting, battery, reboot. Auto-resizes to 1620x2160. Requires SSH.

### Read document text
```bash
remarkable read "Document Name"              # full text from PDF/EPUB
remarkable read "Document Name" --page 5     # single page
```
Extracts text from PDFs (via pdftotext) and EPUBs (built-in). Returns searchable text. Works over SSH and cloud.

### Extract highlights
```bash
remarkable highlights "Document Name"           # all highlights as JSON
remarkable highlights "Document Name" --page 5  # single page
remarkable highlights "Document Name" --markdown # as markdown blockquotes
```
Parses .highlights/ folder, merges adjacent highlights (3-char gap tolerance). Requires SSH.

### Backup
```bash
remarkable backup                   # structured backup to ./remarkable-backup/
remarkable backup /path/to/dir      # custom destination
remarkable backup --raw             # raw xochitl tar.gz
```
Downloads all documents preserving folder structure. Renders notebook pages as PNG. Requires SSH.

### Search
```bash
remarkable search "meeting"          # fuzzy name search
remarkable search "PMF" --tag work   # filter by tag
```
Case-insensitive substring search across document names. Works over SSH and cloud.

### Fetch URL
```bash
remarkable fetch https://example.com/paper.pdf
remarkable fetch https://arxiv.org/pdf/2401.12345.pdf "Papers"
```
Downloads a PDF from a URL and uploads to the device in one command. Direct PDF URLs only.

### Document info
```bash
remarkable info "Document Name"
```
Shows: name, path, type, pages, tags, last modified, ID. Quick lookup for agents.

### Refresh (reload device UI)
```bash
remarkable refresh                        # restart xochitl so changes appear
```
Call after put, mkdir, rm, mv, or fetch — especially after bulk operations. Without refresh, uploaded docs won't be visible on the device screen until the next manual reboot.

### Device management
```bash
remarkable password "newpass"             # change SSH password
remarkable setup-key                      # install SSH key (passwordless)
remarkable watch --on-change "cmd {id}"   # monitor for changes
remarkable auth                           # cloud authentication
remarkable disconnect                     # forget device
```

## Transport rules

| Transport | Speed | Capabilities |
|-----------|-------|-------------|
| SSH | ~1s | Everything |
| Cloud | ~3s | Read-only: ls, get, export, read, search |

SSH-only: write, watch, splash, password, setup-key, pages, tag, put, rm, mv, mkdir, highlights, backup, fetch

SSH preferred, cloud fallback: ls, export, get, read, search

All commands auto-detect the best transport. If SSH is unavailable (device sleeping), ls/export/get fall back to cloud automatically. No manual transport selection needed.

## Output format

JSON by default when stdout is not a terminal. Errors are structured:
```json
{"error": "description with actionable hints", "code": "transport_unavailable"}
```

Error codes: `transport_unavailable`, `not_found`, `permission_denied`, `auth_required`, `auth_expired`, `unsupported_operation`, `corrupted_data`, `conflict`

Every error includes hints about what to run next. The agent reads the error and follows the hint.

## Typical agent workflows

### Read and process notes
```bash
remarkable connect 192.168.1.5
remarkable ls
remarkable export "My Notes" -o /tmp/notes
# read SVG files in /tmp/notes/
```

### Write agent output to tablet
```bash
remarkable write "Daily Notes" --text "Summary: discussed Q2 targets, action items below" --new-page
remarkable write "Daily Notes" --text "1. Follow up with team\n2. Review budget" 
```

### Organize notebooks
```bash
remarkable pages add "Work" --template "P Grid medium"
remarkable pages move "Work" --page 5 --to 1
remarkable tag "Work" "priority"
```

### Monitor and react
```bash
remarkable watch --on-change "process_notes.sh {id}"
```
