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

### Export handwriting to SVG
```bash
remarkable export "Notebook" -o /tmp/export
```
Parses v6 .rm binary, renders pen strokes to SVG (one per page). Requires SSH.

### Page management
```bash
remarkable pages "Notebook"                          # list pages
remarkable pages add "Notebook"                      # add blank page at end
remarkable pages add "Notebook" --template "P Grid medium"
remarkable pages add "Notebook" --after 3            # insert after page 3
remarkable pages rm "Notebook" --page 5              # delete page 5
remarkable pages move "Notebook" --page 5 --to 2     # reorder
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
| Cloud | ~3s | ls only (read-only fallback) |

SSH-only commands: write, export, watch, splash, password, setup-key, pages, tag, get, put, rm, mv, mkdir

Cloud works for: ls (auto-fallback when SSH unreachable)

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
