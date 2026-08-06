# xdm

`xdm` is an independent, terminal-first Direct Messages client for X. It is not
affiliated with, endorsed by, or supported by X Corp.

## Status

The project is under active development and has no stable release yet. It
supports a local demo, the official X API backend, and an experimental web
backend for text Direct Messages in existing conversations.

## Requirements

- Go 1.26.5 or newer
- Git
- Chrome, Edge, or Chromium when using web authentication
- An X Developer application when using the official backend

## Try the demo

```sh
go run . demo
```

## Official X API

Register `http://127.0.0.1:8743/callback` for your X OAuth 2.0 client, then run:

```sh
go run . auth --client-id <id>
go run .
```

The official backend is the default.

## Experimental web backend

> [!WARNING]
> The web backend uses undocumented X web endpoints. It may stop working at any
> time, may violate [X's Terms of Service](https://x.com/en/tos), and may cause X
> to restrict or suspend the authenticated account. Use it only if you
> understand and accept that risk. The project does not bypass account
> challenges, rate limits, or other X security controls.

Authenticate using a dedicated browser profile:

```sh
go run . auth web --browser chrome
```

Sign in manually, wait for the X home timeline to load, and close that browser
window. `xdm` reopens the same profile briefly to capture the authenticated
session. Confirm that it was saved:

```sh
go run . auth status
```

Then start the TUI with the experimental backend:

```sh
go run . --backend web
```

The current web backend can synchronize text messages from the initial inbox
state and send text messages to existing conversations. Creating conversations,
attachments, reactions, typing indicators, read receipts, and complete history
pagination are not supported yet.

Saved web sessions are encrypted with a key held by the operating-system
keyring. Each web account uses a separate local message cache. Browser profile
data remains in the operating-system cache directory so subsequent logins do
not appear as a brand-new browser every time.

To select or remove saved web accounts:

```sh
go run . auth use <account>
go run . logout web --account <account>
go run . logout web
```

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, validation, commit, and pull
request conventions. Security issues must be reported according to
[SECURITY.md](SECURITY.md).
