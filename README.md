# Soliloquy

A small, private forum for work discussions and friendly chats.

- Invite-only. Any member can invite others, and everyone can see who invited whom.
- Threads with nested comments, like Hacker News and Reddit.
- New comments and notifications appear live, with no page reload.
- Markdown, upvotes, @mentions, full-text search and dark mode.
- One Go binary and one SQLite file. Put a reverse proxy in front of it and you are done.

See [docs/PRD.md](docs/PRD.md) for the full requirements.

## Running

Build the binary (Go 1.24 or later, no CGO needed):

```sh
CGO_ENABLED=0 go build -o soliloquy .
./soliloquy -base-url https://forum.example.com
```

Or use Docker:

```sh
docker build -t soliloquy .
docker run -d -p 8080:8080 -v soliloquy-data:/data \
  -e SOLILOQUY_BASE_URL=https://forum.example.com soliloquy
```

### Settings

Each setting is a flag or an environment variable. A flag wins over the
environment variable.

| Flag | Environment variable | Default | Meaning |
|---|---|---|---|
| `-listen` | `SOLILOQUY_LISTEN_ADDR` | `:8080` | Address to listen on. |
| `-db` | `SOLILOQUY_DB_PATH` | `soliloquy.db` | Path of the SQLite file. |
| `-base-url` | `SOLILOQUY_BASE_URL` | `http://localhost:8080` | The public URL of the forum. |
| `-allow-any-origin` | `SOLILOQUY_ALLOW_ANY_ORIGIN` | `false` | Turn off the `Origin` check. This turns off CSRF protection. Use it only on QA and debugging servers. |

The base URL must be the exact URL that users type into their browser. The
app uses it to build invite and reset links. It also rejects any form post
whose `Origin` header does not match it, so a wrong base URL makes every form
fail with "403 Forbidden".

### The first admin

When the app starts with an empty database, it writes a one-time admin invite
URL to the log:

```
2026/01/01 12:00:00 INFO first admin invite url=https://forum.example.com/invite/...
```

Open that URL to create the first account. Each restart before then makes a
new URL and the old one stops working. After the first account exists, members
invite other people from the "My invites" page.

## Reverse proxy

The app does not do TLS. Put a reverse proxy in front of it. With Caddy, this
`Caddyfile` is all you need:

```
forum.example.com {
	reverse_proxy localhost:8080
}
```

With nginx, live updates need the response to go through without buffering.
The app sends `X-Accel-Buffering: no`, so no extra nginx setting is needed for
that, but make sure that `proxy_read_timeout` is longer than 15 seconds (the
app sends a keep-alive every 15 seconds).

## Limits

- Run **one process only**. Live updates go through an in-memory hub, so a
  second instance would not see the first one's events.
- Login throttling is kept in memory, so a restart resets it.

## Backups

All data is in the SQLite file. The database uses WAL mode, so while the app is
running, recent writes can be in the `-wal` file next to it. To make a backup
of a running forum, use SQLite's backup command:

```sh
sqlite3 soliloquy.db ".backup soliloquy-backup.db"
```

Alternatively, stop the app and copy `soliloquy.db`.

## Development

```sh
go test ./...
```

## Licence

[AGPL-3.0](LICENSE).
