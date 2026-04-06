# remarkable-cli

Give your AI agent full control of your reMarkable Paper Pro.

## Setup prompt

Paste this into Claude Code, Codex, or any agent with shell access:

> Clone https://github.com/itsfabioroma/remarkable-cli, build it with `go build -o remarkable .`, symlink the binary to /usr/local/bin/remarkable, then run `remarkable connect` to set up SSH. Once connected, read the skill file in the repo to learn every command.

That's it. The agent reads the skill, learns every command, and starts managing your tablet.

## What the agent can do

```bash
remarkable ls                          # list all documents (JSON)
remarkable get "My Notes"              # download PDF/EPUB
remarkable put report.pdf "Work"       # upload to folder
remarkable rm "Old Draft"              # delete
remarkable mv "Draft" "Final"          # rename
remarkable mv "Doc" "Folder"           # move
remarkable mkdir "Projects"            # create folder
remarkable export "Notebook" -o /tmp   # render handwriting → PNG
remarkable export "Notebook" --svg     # or SVG
remarkable export "Notebook" --page 3  # single page
remarkable write "NB" --text "Hello"   # write text as pen strokes
remarkable write "NB" --text "More" --new-page  # on a new page
remarkable pages "Notebook"            # list pages
remarkable pages add "Notebook"        # add blank page
remarkable pages rm "Notebook" --page 5 # delete page
remarkable tag "Notebook" "work"       # add tag
remarkable tags                        # list all tags
remarkable screenshot                  # capture device screen
remarkable refresh                     # reload UI after changes
remarkable watch --on-change "cmd {id}" # live change monitoring
remarkable splash set art.png          # change sleep screen
remarkable splash list                 # see current splash screens
remarkable splash restore              # restore originals
remarkable read "My Notes"             # extract text from PDF/EPUB
remarkable highlights "My Notes"       # extract highlights as markdown
remarkable backup                      # full structured backup
remarkable backup --raw                # raw device backup
remarkable search "meeting"            # search by name
remarkable search "PMF" --tag work     # search with tag filter
remarkable fetch https://url/paper.pdf # download URL → upload to device
remarkable info "My Notes"             # detailed doc info
remarkable password "newpass"          # change SSH password
remarkable setup-key                   # install SSH key (passwordless)
remarkable auth                        # set up cloud access
remarkable disconnect                  # forget device
```

## How it works

One binary, two transports. `connect` probes SSH and cloud, saves what's available. Every command auto-picks the best transport:

- **SSH** (~1s) — full access: read, write, export, watch, splash, device management
- **Cloud** (~3s) — read-only fallback when SSH is unreachable

JSON output by default when called by an agent. Structured errors with actionable hints — the agent reads the error and knows its next move.

## Agent skills

Ships skills for three agent platforms — auto-loaded, no config needed:

| Agent | Skill path |
|-------|-----------|
| Claude Code | `.claude/skills/remarkable-cli/` |
| Codex / universal | `.agents/skills/remarkable-cli/` |
| OpenClaw | `skills/remarkable-cli/` |

## Manual install

```bash
git clone https://github.com/itsfabioroma/remarkable-cli
cd remarkable-cli
go build -o remarkable .
sudo ln -sf $(pwd)/remarkable /usr/local/bin/remarkable
./remarkable connect            # USB (10.11.99.1)
./remarkable connect 192.168.1.5  # or WiFi
```

## Requirements

- Go 1.21+
- reMarkable Paper Pro with developer mode enabled
- optional: reMarkable Cloud subscription (for cloud fallback)

## WiFi SSH

Paper Pro blocks WiFi SSH by default. Enable once via USB:

```bash
ssh root@10.11.99.1
rm-ssh-over-wlan on
```
