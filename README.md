# remarkable-cli

One binary to control your reMarkable Paper Pro. SSH + Cloud. Agent-native JSON.

## Quick start

```bash
git clone https://github.com/itsfabioroma/remarkable-cli
cd remarkable-cli
go build -o remarkable .
./remarkable connect            # USB
./remarkable connect 192.168.1.5  # or WiFi
./remarkable ls
```

That's it. Four commands from zero to listing your documents.

## One-shot setup (agents)

```bash
git clone https://github.com/itsfabioroma/remarkable-cli && cd remarkable-cli && go build -o remarkable . && ./remarkable connect
```

## Global install

After building, make `remarkable` available everywhere:

```bash
sudo ln -sf $(pwd)/remarkable /usr/local/bin/remarkable
# or
make install  # installs to $GOPATH/bin
```

## What it does

```bash
remarkable ls                          # list all documents (JSON)
remarkable get "My Notes"              # download PDF/EPUB
remarkable put report.pdf "Work"       # upload to folder
remarkable rm "Old Draft"              # delete
remarkable mv "Draft" "Final"          # rename
remarkable mv "Doc" "Folder"           # move
remarkable mkdir "Projects"            # create folder
remarkable export "Notebook" -o /tmp   # render handwriting → SVG
remarkable watch --on-change "cmd {id}" # live change monitoring
remarkable splash set art.png          # change sleep screen
remarkable splash list                 # see current splash screens
remarkable splash restore              # restore originals
remarkable read "My Notes"              # extract text from PDF/EPUB
remarkable highlights "My Notes"        # extract highlights as markdown
remarkable backup                       # full structured backup
remarkable backup --raw                 # raw device backup
remarkable search "meeting"             # search by name
remarkable search "PMF" --tag work      # search with tag filter
remarkable fetch https://url/paper.pdf  # download URL → upload to device
remarkable info "My Notes"              # detailed doc info
remarkable password "newpass"           # change SSH password
remarkable setup-key                    # install SSH key (passwordless)
remarkable auth                         # set up cloud access
remarkable disconnect                   # forget device
```

## How it works

`connect` probes SSH and cloud, saves what's available. Every command after that auto-picks the best transport:

- **SSH** (~1s) — full access: read, write, export, watch, splash, device management
- **Cloud** (~20s) — fallback: document listing when SSH is unreachable

If SSH is down (device sleeping, different network), `ls` falls back to cloud automatically. Device commands (export, splash, watch) tell you SSH is needed and how to fix it.

## Output

JSON by default when called by an agent (non-TTY stdout). Structured errors with actionable hints:

```json
{"error": "this command requires SSH\n  remarkable connect 192.168.1.5", "code": "transport_unavailable"}
```

The agent reads the error and knows its next move. No `-h` needed.

## Agent skills

This repo ships skills for three agent platforms:

| Agent | Skill path | Auto-loaded |
|-------|-----------|-------------|
| Claude Code | `.claude/skills/remarkable-cli/` | yes |
| Codex / universal | `.agents/skills/remarkable-cli/` | yes |
| OpenClaw | `skills/remarkable-cli/` | yes |

The skill teaches the agent every command, transport rules, and typical workflows.

## Requirements

- Go 1.21+
- reMarkable Paper Pro with developer mode enabled (for SSH)
- optional: reMarkable Cloud subscription (for cloud fallback)

## Install

```bash
# from source (puts in $GOPATH/bin)
go install github.com/itsfabioroma/remarkable-cli@latest

# or build locally
git clone https://github.com/itsfabioroma/remarkable-cli
cd remarkable-cli
make install
```

## WiFi SSH

Paper Pro blocks WiFi SSH by default. Enable it once via USB:

```bash
ssh root@10.11.99.1
rm-ssh-over-wlan on
```

Then connect over WiFi:

```bash
remarkable connect 192.168.1.5
```
