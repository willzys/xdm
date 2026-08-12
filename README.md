# xdm

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26.5%2B-00ADD8?logo=go)
![Status](https://img.shields.io/badge/status-alpha-orange)

**A keyboard-first Direct Messages client for X, built for the terminal.**

Bubble Tea interface, local searchable history, multiple accounts, two
interchangeable backends.

> [!IMPORTANT]
> `xdm` is an independent open-source project. It is not affiliated with,
> endorsed by, or supported by X Corp.

---

## Try it in 10 seconds

No account, no credentials, no network access required:

```sh
git clone https://github.com/willzys/xdm.git && cd xdm
go run . demo
```

---

## Which backend should I use?

`xdm` talks to X in two ways. Pick one; you can switch later.

| | 🌐 Web/XChat | 🔑 Official API |
| --- | --- | --- |
| **Cost** | Free, uses your normal X login | X's Developer Platform is pay-per-use; there's no free tier for new developer apps ([current pricing](https://docs.x.com/x-api/getting-started/pricing)) |
| **Setup** | Log in through a browser window | Requires an X Developer app + OAuth client ID |
| **Status** | Experimental, undocumented endpoints | Official, documented |
| **Risk** | May break without notice; using it may violate [X's ToS](https://x.com/en/tos) and could get an account restricted | Stable, sanctioned access |
| **Encryption** | Supports end-to-end encrypted XChat | Text only |
| **Best for** | Most people who just want DMs in the terminal | Developers who already have X API access, or who need guaranteed stability |

If you're unsure, start with **Web/XChat** below. It's the fastest way to get real DMs working. If it stops working for you at some point, that's the trade-off of it being unofficial; the Official API is the fallback.

---

## Install

Grab a build from [Releases](https://github.com/willzys/xdm/releases) (Windows x64, Linux x64, macOS Intel/Apple Silicon):

1. Download the archive for your OS.
2. Verify it against `checksums.txt` (`sha256sum -c`).
3. Extract the **whole folder**, keeping `xdm` next to its helper and the `xchat-runtime` directory.
4. Run it.

> Builds aren't code-signed yet, so your OS will show an "unknown publisher" warning while the project is in alpha. Verifying the checksum is your integrity check in the meantime.

**Building from source instead:**

```sh
git clone https://github.com/willzys/xdm.git
cd xdm
go mod download
go build -o xdm .        # xdm.exe on Windows
```

---

## Set up: Web/XChat backend

Free, recommended for most people. Uses your regular X login through a real browser, no developer account needed.

**Requirements:** Chrome, Edge, or Chromium, and your XChat PIN (only if you use encrypted Chat).

```sh
# 1. Log in (opens a dedicated browser profile, sign in manually, then close it)
go run . auth web --browser chrome   # or: edge / chromium / auto

# 2. Confirm it worked
go run . auth status

# 3. Unlock XChat (only needed if you use encrypted conversations)
go run . auth web unlock             # enter your XChat PIN when prompted

# 4. Launch
go run . --backend web
```

Contributors running from a source checkout (not a packaged release) also need the crypto runtime once:

```sh
npm ci --prefix ./internal/xchat/runtime
```

**Good to know:**
- This uses undocumented X web endpoints. It can stop working without notice, may violate [X's ToS](https://x.com/en/tos), and carries a real risk of your account being restricted. `xdm` doesn't bypass any of X's rate limits, challenges, or access controls; use it knowingly.
- Currently supports reading and sending encrypted text in *existing* conversations. Creating new conversations, full history pagination, media, and reactions aren't supported yet.
- Never guess your XChat PIN. There's a strict failed-attempt lockout. Reset it from a device that can already read your chats if you forget it.

<details>
<summary>Multiple accounts / logout / clearing cache</summary>

```sh
go run . auth use <account>              # switch saved web account
go run . logout web --account <account>  # remove one account's auth
go run . logout web                      # remove all web auth
go run . cache clear web --account <account>
```

Add `--delete-data` to any `logout` command to also wipe that account's local message database and dedicated browser profile.

</details>

---

## Set up: Official API backend

Paid. Use this if you already have X Developer Platform access, or need officially sanctioned, stable access. As of 2026, X's developer platform is pay-per-use with no free tier for new apps; check [current pricing](https://docs.x.com/x-api/getting-started/pricing) before committing.

```sh
# 1. In your X Developer app, register this callback:
#    http://127.0.0.1:8743/callback

# 2. Authorize
go run . auth --client-id <your-client-id>   # add --no-browser if needed

# 3. Launch (this is the default backend)
go run .
```

---

## Keyboard controls

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move through conversations or results |
| `g` / `G` | Jump to first / last conversation |
| `Tab` | Switch between inbox and conversation panes |
| `Enter` / `i` | Open composer |
| `Enter` (in composer) | Send |
| `Ctrl+Enter` / `Alt+Enter` | Newline while composing |
| `/` | Search conversations and cached messages |
| `R` | Sync now |
| `Esc` | Leave current input/view |
| `q` / `Ctrl+C` | Quit |

Inbox auto-syncs every 65 seconds.

---

## What's supported right now

| Capability | Demo | Official API | Web/XChat |
| --- | :---: | :---: | :---: |
| Browse cached conversations | ✅ | ✅ | ✅ |
| Read and search messages | ✅ | ✅ | ✅ |
| Send text messages | Local only | ✅ | ✅ (encrypted) |
| Multiple accounts | - | - | ✅ |
| End-to-end encryption | - | - | ✅ |
| Full history pagination | - | 🔜 | 🔜 |
| Create new conversations | - | 🔜 | 🔜 |
| Media / attachments | - | 🔜 | 🔜 |

`xdm` is alpha software. No stable release or compatibility guarantee yet.

---

## Security essentials

- OAuth tokens and web-session keys live in your OS keyring; browser cookies are stored in an AES-GCM encrypted vault.
- **Your local message database is not encrypted.** It's plain SQLite, used for offline browsing/search. Protect it the way you'd protect any other local file with private data (disk encryption, account access control).
- XChat PINs and decrypted private keys are never written to disk.
- `logout --delete-data` wipes the message database, cache index, and (for web accounts) the dedicated browser profile.
- Deleting files doesn't guarantee physical erasure on SSDs, snapshots, or backups. Use OS-level disk encryption if that threat matters to you.

Full details in the README history / [SECURITY.md](SECURITY.md). Report vulnerabilities there privately, never in a public issue.

---

## Roadmap

- Full inbox/conversation history pagination
- Creating new conversations
- Encrypted media and attachments
- Replies, reactions, edits, deletes, read receipts
- Reproducible builds + code signing

Open an issue before starting a large feature so scope can be discussed first.

---

## Contributing

```sh
gofmt -d <changed-go-files>
go test ./...
go vet ./...
go build ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for branch naming, commit style, and PR expectations. By contributing you agree your work is MIT-licensed.

## License

MIT © 2026 Willian. See [LICENSE](LICENSE).
