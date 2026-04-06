# remarkable-cli

Give your AI agent full control of your reMarkable Paper Pro.

## Setup prompt

Paste this into Claude Code, Codex, or any agent with shell access:

```
Clone https://github.com/itsfabioroma/remarkable-cli
Build it with go build -o remarkable .
Symlink the binary to /usr/local/bin/remarkable
Install the skill into your agent's skills folder:
  Claude Code:  cp -r skills/remarkable-cli/ .claude/skills/remarkable-cli/
  OpenClaw:     cp -r skills/remarkable-cli/ .openclaw/workspace/skills/remarkable-cli/
  Codex:        cp -r skills/remarkable-cli/ .agents/skills/remarkable-cli/
Run remarkable connect to set up SSH
```

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

`remarkable connect` runs an interactive wizard: cloud first, then optional SSH.

- **Cloud** (~3s) — default transport, works for everyone: list, download, export, search
- **SSH** (~1s) — optional, requires developer mode: write, upload, delete, pages, tags, splash

JSON output by default when called by an agent. Structured errors with actionable hints.

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
./remarkable connect            # interactive wizard
```

## Requirements

- Go 1.21+
- reMarkable Paper Pro
- optional: developer mode + SSH for write access

## SSH setup (optional, for write access)

SSH unlocks uploads, deletes, writes, page management, and more. Requires developer mode.

### 1. Enable developer mode

On your reMarkable: Settings → General → Developer mode → Enable.

### 2. Find your device IP

- **USB**: always `10.11.99.1`
- **WiFi**: on your reMarkable go to Settings → General → About → Copyrights and licenses → General information. The IP is listed under the password.

### 3. Enable WiFi SSH (optional)

Paper Pro blocks WiFi SSH by default. Enable once via USB:

```bash
ssh root@10.11.99.1
rm-ssh-over-wlan on
```

### 4. Add SSH to your config

```bash
remarkable connect <your-device-ip> --ssh-only --password <your-password>
```
