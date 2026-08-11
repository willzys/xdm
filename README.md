# xdm

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26.5%2B-00ADD8?logo=go)
![Status](https://img.shields.io/badge/status-alpha-orange)

**A keyboard-first Direct Messages client for X, built for the terminal.**

`xdm` combines a Bubble Tea interface, local searchable history, multiple
accounts, and two interchangeable access modes: the official X API and an
experimental X web/XChat backend.

> [!IMPORTANT]
> `xdm` is an independent open-source project. It is not affiliated with,
> endorsed by, or supported by X Corp.

## Why xdm?

- Stay in the terminal while reading, searching, and sending Direct Messages.
- Navigate the entire interface from the keyboard.
- Keep a local SQLite cache for fast startup and offline search.
- Choose the documented official API or explicitly opt into experimental web
  access.
- Keep backend, authentication, service, storage, and interface concerns
  separated for contributors.

## Project status

`xdm` is alpha software under active development. There is no stable release
or compatibility guarantee yet.

| Capability | Demo | Official API | Experimental web/XChat |
| --- | :---: | :---: | :---: |
| Browse cached conversations | Yes | Yes | Yes |
| Read and search text messages | Yes | Yes | Yes |
| Send text messages | Local | Yes | Yes, encrypted |
| Multiple web accounts | — | — | Yes |
| End-to-end encrypted XChat | — | — | Yes |
| Full history pagination | — | Not yet | Not yet |
| Create conversations | — | Not yet | Not yet |
| Media and attachments | — | Not yet | Not yet |

## Requirements

### Common

- Go 1.26.5 or newer
- Git
- A supported terminal

### Official API backend

- An application in the X Developer Console
- An OAuth 2.0 client ID

### Experimental web/XChat backend

- Chrome, Edge, or Chromium
- An XChat PIN for accounts using encrypted Chat

Packaged builds include the platform-native XChat crypto helper. Node.js and
npm are only needed by contributors who run the web backend directly from a
source checkout or assemble a release artifact.

## Installation

There are no published binaries yet. Build the current development version
from source:

```sh
git clone https://github.com/willzys/xdm.git
cd xdm
go mod download
go build -o xdm .
```

On Windows, the output is `xdm.exe`.

## Quick start: local demo

The demo requires no X account, credentials, or network access:

```sh
go run . demo
```

It is the safest way to explore the interface or work on TUI changes.

## Official X API backend

The official backend is the default and uses OAuth 2.0 with PKCE.

1. Register the following callback in your X Developer application:

   ```text
   http://127.0.0.1:8743/callback
   ```

2. Authorize `xdm`:

   ```sh
   go run . auth --client-id <your-client-id>
   ```

3. Start the TUI:

   ```sh
   go run .
   ```

Use `--no-browser` when the authorization URL must be opened manually.

## Experimental web/XChat backend

> [!WARNING]
> This backend uses undocumented X web endpoints. It may stop working without
> notice, may violate [X's Terms of Service](https://x.com/en/tos), and may
> cause X to restrict or suspend the authenticated account. Use it only after
> understanding and accepting that risk. `xdm` does not bypass challenges,
> rate limits, access controls, or other X security mechanisms.

### 1. Prepare the crypto runtime

Packaged builds already contain `xdm-xchat-helper` and its pinned runtime; no
Node.js or npm installation is needed.

When running the web backend directly from a source checkout, install the
development copy of the runtime:

```sh
npm ci --prefix ./internal/xchat/runtime
```

PowerShell users with script execution disabled can run:

```powershell
npm.cmd ci --prefix .\internal\xchat\runtime
```

The runtime uses the official, MIT-licensed
[`@xdevplatform/chat-xdk`](https://github.com/xdevplatform/chat-xdk) and
Juicebox SDK packages. Exact versions and integrity hashes are recorded in
`package-lock.json`.

### 2. Capture a web session

```sh
go run . auth web --browser chrome
```

`xdm` opens a dedicated persistent browser profile without remote debugging.
Sign in manually, confirm that the X home timeline loads, and close the browser
window. The same profile is then reopened briefly to capture the authenticated
session.

Supported browser values are `auto`, `chrome`, `edge`, and `chromium`.

### 3. Confirm authentication

```sh
go run . auth status
go run . auth web diagnose
```

The diagnostic command reports counts and response structure without printing
cookies, keys, message text, or account secrets.

### 4. Verify XChat recovery

```sh
go run . auth web unlock
```

Enter the existing XChat PIN when prompted. Nothing is echoed while typing.
The PIN is passed to the local crypto process through standard input, cleared
from the Go buffer after use, and never saved by `xdm`.

Do not guess the PIN. XChat applies a strict failed-attempt limit to key
recovery. If it was forgotten, reset it from a device that can already read the
encrypted conversations.

### 5. Start the TUI

```sh
go run . --backend web
```

The PIN is requested once per run. The unlocked private key and conversation
keys remain only in the crypto process memory while the TUI is open.

The web backend currently reads and sends encrypted text messages in existing
conversations. Conversation creation, complete history pagination, media,
reactions, typing indicators, and server-side read receipts are not supported
yet.

## Keyboard controls

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Move through conversations or results |
| `g` / `G` | Jump to the first or last conversation |
| `Tab` | Switch between inbox and conversation panes |
| `Enter` or `i` | Open the message composer |
| `Enter` in composer | Send the message |
| `Ctrl+Enter` / `Alt+Enter` | Insert a newline while composing |
| `/` | Search conversations and cached messages |
| `R` | Synchronize immediately |
| `Esc` | Leave the active input or results view |
| `q` / `Ctrl+C` | Quit |

The inbox synchronizes automatically every 65 seconds while the TUI is open.

## Authentication and accounts

Inspect saved authentication without exposing credentials:

```sh
go run . auth status
```

Select a saved web account:

```sh
go run . auth use <account>
```

Remove authentication:

```sh
go run . logout official
go run . logout web --account <account>
go run . logout web
go run . logout all
```

Each web account has an isolated local message database.
`xdm` keeps a minimal cache index so a database can still be selected by its
account name after authentication is removed.

Clear cached messages without removing authentication:

```sh
go run . cache clear official
go run . cache clear web --account <account>
go run . cache clear all
```

To remove authentication and the associated local data together, add
`--delete-data` to a `logout` command:

```sh
go run . logout official --delete-data
go run . logout web --account <account> --delete-data
go run . logout web --delete-data
go run . logout all --delete-data
```

Deleting data for any web account also removes the dedicated browser profiles,
because a profile can contain cookies for more than one saved account. Other
encrypted web sessions remain saved when `--account` selects one account.

## Data and security model

- Official OAuth tokens and the web-session encryption key are held by the
  operating-system keyring.
- Saved browser cookies are stored in an AES-GCM encrypted vault under the
  user's configuration directory.
- Web authentication uses a dedicated browser profile instead of attaching a
  debugger to the user's normal browser session.
- XChat PINs and recovered private keys are not persisted by `xdm`.
- Decrypted message history is cached locally in SQLite for browsing and
  search. The message database itself is **not encrypted**; protect the user
  account and disk accordingly.
- The web cache index contains account identifiers and cache keys, but no
  tokens, cookies, PINs, keys, or message content. It is removed with the
  corresponding caches.
- `cache clear` removes unencrypted message databases while preserving saved
  authentication and dedicated browser profiles.
- `logout --delete-data` removes the selected message database, SQLite sidecar
  files and cache-index entries before removing authentication. For web
  authentication it also removes all dedicated browser profiles.
- File deletion cannot guarantee physical media sanitization on SSDs,
  copy-on-write filesystems, backups, or snapshots. Use operating-system disk
  encryption and retention controls when that threat is relevant.
- Sensitive logs, cookies, tokens, message databases, and real message content
  must never be attached to public issues.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.

## Roadmap

- Add complete inbox and conversation history pagination.
- Create new one-to-one and group conversations.
- Support encrypted media and attachments.
- Add replies, reactions, edits, deletes, and read receipts.
- Publish reproducible cross-platform binaries, bundled helpers, and checksums.

Roadmap items are directional, not release commitments. Open an issue before
starting a large feature so its design and scope can be discussed.

## Development

Run the standard validation suite:

```sh
gofmt -d <changed-go-files>
go test ./...
go vet ./...
go build ./...
git diff --check
```

To assemble an XChat-capable artifact, use an extracted official Node.js
distribution for the host platform. Its `LICENSE` file is included in the
bundle along with the pinned SDK packages and both WebAssembly modules:

```sh
npm ci --prefix ./internal/xchat/runtime
go run ./cmd/package-xchat-helper --node-dir <node-distribution> --output ./dist/xdm
go build -trimpath -o ./dist/xdm/xdm .
```

On Windows, name the final application `xdm.exe`. The packaging command only
accepts a new output directory, records the Node version and package-lock hash,
and never overwrites an existing artifact.

Use the demo backend for interface work and sanitized test fixtures for network
or cryptographic changes. Never commit credentials, cookies, PINs, private
keys, message databases, or real Direct Message content.

See [CONTRIBUTING.md](CONTRIBUTING.md) for branch naming, Conventional Commits,
code guidelines, validation, and pull request expectations.

## Contributing

Bug reports, focused pull requests, documentation improvements, and design
discussion are welcome.

- Use the repository issue templates for bugs and feature proposals.
- Keep pull requests focused and open them against `main`.
- Report vulnerabilities privately according to [SECURITY.md](SECURITY.md).
- By contributing, you agree that your work is licensed under the MIT License.

## License

Copyright © 2026 Willian.

Released under the [MIT License](LICENSE).
