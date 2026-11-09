# Soliloquy: Product Requirements Document

Status: draft, waiting for approval.

Licence: AGPL-3.0.

## 1. Summary

Soliloquy is a small, private forum for work discussions and friendly chats. It is easy to
deploy: you copy one binary, run it, and put a reverse proxy in front of it.
Threads have nested comments, like Reddit and Hacker News. New comments and
notifications show up live, with no page reload.

## 2. Users and scale

- Fewer than 100 users.
- Access is by invitation only. There is no public signup.
- Two roles: **user** and **admin**.

## 3. Functional requirements

### 3.1 Accounts and invites

- Accounts use a username and a password. There is no email address and no
  email sending.
- Usernames are 3 to 20 characters from `a-z`, `0-9`, `_` and `-`. They are
  case-insensitive and unique. Users cannot change their username.
- Sessions last 30 days.
- **Delete own account:** a user can delete their own account.
  - Their threads and comments stay. The author shows as "[deleted user]".
  - Their votes stay.
  - Their sessions, notifications and unused invites are removed.
  - The username stays reserved. Nobody can register it again.
  - In the invite tree, the account shows as "[deleted user]". The people it
    invited keep their place.
- Any user can create an invite link.
  - Each link works once.
  - Each link expires 7 days after it is created.
  - There is no limit on how many invites a user can create.
- The system records who invited each user.
- **First admin:** when the app starts with an empty database, it prints a
  one-time admin invite URL to the log.
- **Password reset:** an admin can create a one-time reset link for a user. It
  follows the same single-use and 7-day expiry rules as invites. When the link
  is used, the user's password changes and all of their sessions end.
- **Login throttling:** after each failed login for a username, the next try
  for that username must wait longer. The wait doubles each time, up to about
  one minute. A successful login resets the wait. This state is kept in memory
  only.

### 3.2 Invite tree

- A page, visible to all logged-in users, shows every user nested under the
  person who invited them, with their join date.
- The first admin is the root of the tree.
- Disabled users stay in the tree and are marked as disabled.

### 3.3 Threads

- A thread has a title (up to 200 characters), an optional URL and an
  optional Markdown body. It must have at least one of URL or body.
- If a thread has a URL, its title on the front page links to the URL, and the
  domain name shows next to the title.
- The front page is a single flat list of threads. There are no categories and
  no tags.
- The front page sorts threads by most recent activity: the newest comment, or
  the thread creation time if the thread has no comments. Votes do not change
  the order. They only show as a number.
- Each row shows the title, author, score, comment count and last activity time.
- The front page shows 30 threads per page.

### 3.4 Comments

- Comments nest with no depth limit.
- Users write comments in Markdown, up to 20,000 characters. Thread bodies
  have the same limit.
- Under each parent, replies sort by score (highest first), then by age (oldest
  first).
- Authors can edit their own comments and threads at any time. Edited posts
  show "edited".
- Authors can delete their own comments and threads.
  - A deleted comment that has replies shows as "[deleted]", so the tree keeps
    its shape.
  - A deleted comment with no replies is hidden.

### 3.5 Upvotes

- Users can upvote threads and comments.
- One vote per user per item. Clicking again removes the vote.
- Users cannot upvote their own posts.
- There are no downvotes and no karma totals.

### 3.6 Real-time updates

- When a user has a thread open, new comments in that thread appear in the
  correct place in the tree with no reload.
- The notification badge updates live.
- Edits, deletes and vote counts do **not** update live. They show on reload.

### 3.7 Notifications

In-app only. A user gets a notification when:

1. someone replies to their comment,
2. someone posts a top-level comment on their thread,
3. someone mentions them as `@username` in a thread or comment. When a post
   is edited, only users who are newly mentioned get a notification.

Users do not get notifications for their own actions. There is a
notifications page and an unread count badge in the header.

### 3.8 Search

- Full-text search over thread titles, thread bodies and comment bodies.
- Results link to the thread or the comment.
- Deleted content does not show in results.

### 3.9 Admin

An admin can:

- delete any thread or comment,
- disable a user. This ends the user's sessions, blocks login, and cancels the
  user's unused invites. People that user invited keep their accounts.
- create a password reset link for a user,
- make another user an admin, or remove admin rights from another user. An
  admin cannot remove their own admin rights, so there is always at least one
  admin.

### 3.10 Appearance

- Server-rendered pages that work on desktop and mobile.
- Dark mode follows the system setting. This is CSS only, with no toggle.

## 4. Technical design

| Area | Decision | Reason | Rejected alternative |
|---|---|---|---|
| Language | Go, one binary with embedded templates and static files | Deployment is one file | Python or Node: needs a runtime or Docker |
| Database | SQLite through `modernc.org/sqlite`, WAL mode | Pure Go, no CGO, one data file, easy backup | `mattn/go-sqlite3`: needs CGO |
| Pages | Go `html/template`, one small plain JavaScript file, no build step | Simple, no frontend tooling | Single-page app framework: build chain with no benefit here |
| Live updates | Server-Sent Events (SSE) and an in-process publish/subscribe hub | One-way server-to-browser is all we need, works through proxies, reconnects by itself | WebSockets: two-way, not needed |
| Comment tree | Load all comments of a thread in one query, build the tree in memory | Threads are small at this scale | Stored tree paths or recursive SQL: more complex |
| Markdown | `goldmark` to render, `bluemonday` to sanitize, rendered once on save, source and HTML both stored | Fast reads, safe output | Render on every read |
| Search | SQLite FTS5, kept up to date on save | Built in, no extra service | External search engine |
| Sessions | Random token in an HttpOnly, SameSite=Lax cookie, stored in the database | Simple, can be revoked | JWT: hard to revoke |
| Passwords | bcrypt | Standard | |
| CSRF | SameSite cookies plus an Origin header check on POST | No token plumbing | Per-form CSRF tokens |
| Login throttling | Per username, in memory | No need to trust proxy headers | Per IP: needs a correct `X-Forwarded-For` setup |
| TLS | Reverse proxy (Caddy, nginx, Traefik) | Less code in the app | Built-in Let's Encrypt |

### Configuration

Flags or environment variables:

- listen address,
- database file path,
- base URL, used to build invite and reset links.

### Deployment

- A multi-stage Dockerfile that makes a static binary.
- README with run instructions and an example Caddyfile.
- Backup: copy the SQLite file.

### Constraints

- The app runs as **one process only**. The live-update hub is in memory, so a
  second instance would break live updates.
- The reverse proxy must not buffer SSE responses. The app sends
  `X-Accel-Buffering: no` for nginx. Caddy needs no extra settings.

## 5. Security

- Markdown output is sanitized. User content cannot run script in other users'
  browsers.
- Permission checks are on the server for every write: edit and delete own
  posts only, admin actions for admins only.
- Invite and reset tokens are random, single-use and expire.
- Known trade-off: per-username throttling lets an attacker lock a known user
  out for about a minute at a time. This is acceptable for a private forum.
- Known trade-off: any user can invite anyone. The answer is the public invite
  tree and the admin's power to disable users.

## 6. Success criteria

- A fresh server goes from nothing to a working forum with: build, copy the
  binary, run it, open the admin invite link.
- Two browsers in the same thread see each other's new comments within about a
  second.
- Markdown from users cannot run script in other users' browsers.
- Go tests cover the parts that are easy to get wrong: tree building and
  sorting, invite and reset rules, permission checks, Markdown sanitizing,
  mention parsing.

## 7. Non-goals (version 1)

- Public signup, email of any kind, SSO.
- Downvotes, karma, categories, tags.
- Live updates for edits, deletes and vote counts.
- Typing indicators and online presence.
- Image or file uploads.
- Locked threads.
- Changing usernames.
- General rate limiting (only login throttling is in scope).
- Disabling a whole invite subtree in one action.
- Vote-based ranking of the front page.
- Running more than one instance.
- A dark mode toggle.
