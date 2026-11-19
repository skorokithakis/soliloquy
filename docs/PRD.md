# Soliloquy: Product Requirements Document

Status: approved.

Licence: AGPL-3.0.

## 1. Summary

Soliloquy is a small forum for work discussions and friendly chats. It is
private by default, but an admin can make it public and read-only for
visitors. It is easy to deploy: you copy one binary, run it, and put a reverse
proxy in front of it. Threads have nested comments, like Reddit and Hacker
News. New comments and notifications show up live, with no page reload.

## 2. Users and scale

- Fewer than 100 users.
- Access is by invitation only. There is no public signup.
- The forum is private by default. An admin can make it readable by visitors
  without an account.
- Two roles: **user** and **admin**.

## 3. Functional requirements

### 3.1 Accounts and invites

- Accounts use a username and a password. There is no email address and no
  email sending.
- Usernames are 3 to 20 characters from `a-z`, `0-9`, `_` and `-`. They are
  case-insensitive and unique. Users cannot change their username.
- Sessions last one year. The year starts again each time the user uses the
  site. The new expiry is written at most once a day.
- Users can log out.
- **Change password:** a user can change their own password. They must give
  their current password. Their other sessions end.
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
  - A page lists the user's unused, unexpired invite links. Users cannot
    cancel invites.
- The system records who invited each user.
- **First admin:** when the app starts with an empty database, it prints a
  one-time admin invite URL to the log.
- **Password reset:** an admin can create a one-time reset link for a user. It
  follows the same single-use and 7-day expiry rules as invites. When the link
  is used, the user's password changes and all of their sessions end.
- **Login throttling:** after each failed login for a username, the next try
  for that username must wait longer. The wait doubles each time, up to about
  one minute. A successful login resets the wait. This state is kept in memory
  only. Tries made during a wait are rejected and do not count as failures.
  An entry is removed when the username has had no failed login for a few
  minutes, so the memory use stays small.

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
  - A deleted comment is hidden when it has no visible replies under it. This
    is decided when the page renders.
- A deleted thread does not show on the front page or in search. Its page
  returns 404. Its rows stay in the database.

### 3.5 Upvotes

- Users can upvote threads and comments.
- A new thread or comment starts with a score of 1, for the author's own
  implicit vote.
- One vote per user per item. Clicking again removes the vote.
- Users cannot upvote their own posts.
- There are no downvotes and no karma totals.

### 3.6 Real-time updates

- When a user has a thread open, new comments in that thread appear in the
  correct place in the tree with no reload.
- When a user has a thread open, these changes to that thread also appear with
  no reload: edits of the thread and of its comments, deletes of its comments,
  vote counts and the comment count.
- The notification badge updates live.
- Each page opens one Server-Sent Events stream. It carries the user's badge
  updates and, on a thread page, that thread's changes.
- A new comment always has the lowest score (1) and is the newest, so it goes
  last under its parent. The browser does not need to sort.
- A live update changes only the content that changed. The viewer's own vote
  state and the viewer's own edit and delete links stay as they are.
- A comment that is deleted live shows as `[deleted]` in its place. On the
  next page load it is gone if it has no replies.
- A thread page can miss changes while its stream is not connected, for
  example during a network drop or a server restart. When the stream connects
  again, the server sends the new comments, the edits and the deletes that
  happened after the page was loaded. The server does not keep a history of
  events. It reads the current state from the database.
- Vote counts are not sent again after a reconnect. They are correct on the
  next page load.
- Live updates are for the thread page only. The front page and the other
  pages show changes on reload.
- A deleted thread does not update live. Its stream closes, and the next page
  load shows that it is gone.
- All forms work as plain POST requests with a redirect, so pages work with no
  JavaScript. JavaScript adds live updates and votes with no reload.

### 3.7 Notifications

A user gets an in-app notification when:

1. someone replies to their comment,
2. someone posts a top-level comment on their thread,
3. someone mentions them as `@username` in a thread or comment. When a post
   is edited, only users who are newly mentioned get a notification.
   Mentions are found in normal text only, not in code spans or code blocks.
   Only existing, active users get a notification. Mentions are highlighted,
   not links.
4. someone posts a comment, at any depth, on a thread they follow. Following
   is opt-in: only the follow button on the thread page turns it on, and
   posting a thread or a comment does not. A member gets one notification per
   comment: a reply, thread-comment or mention notification replaces the
   followed one for the same comment.

Users do not get notifications for their own actions. There is a
notifications page and an unread count badge in the header.

Users can also receive their notifications through a personal RSS feed,
desktop alerts in the browser (while a forum tab is open), and ntfy pushes.
Followed-thread notifications use the same channels. ntfy can also send a
push for each new thread, if the user turns it on.

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
- rename the forum and make it public or private.
- An admin cannot disable themselves. The last active admin cannot delete their
  own account.

### 3.10 Public read-only mode

- The forum is private by default. Visitors are sent to the login page.
- An admin can make the forum public on the admin settings page.
- When it is public, a visitor can read the front page, thread pages and
  search. Paging works as it does for members.
- Visitors cannot write. Every POST redirects them to the login page.
- Account, notifications, invites, the invite tree, member profiles, admin pages, the thread
  form, edit and reply pages, and live updates are for logged-in users only.
  Visitors who open them are sent to the login page.
- Visitors see the usual header with the forum name, the "threads" link and
  the search box. They see a "log in" link instead of the new-thread,
  notifications and account links.
- Visitors see scores but no vote buttons. They see no comment form, and no
  reply, edit or delete links. In place of the comment form, the thread page
  shows a "Log in to comment" link.

### 3.11 Appearance

- Server-rendered pages that work on desktop and mobile.
- Each account can choose system, light or dark on the account page. The default follows the system setting.

### 3.12 Member profiles

- Each member has a profile page at `/u/{username}`. Only logged-in users can
  see it.
- The page shows the username, the join date, who invited the member, and
  whether the member is an admin or is disabled.
- It also lists the member's 20 newest threads and 20 newest comments, newest
  first. There is no paging. Deleted threads, deleted comments and comments in
  deleted threads are not listed.
- A deleted account has no profile page.
- For logged-in users, author names on the front page, thread pages, comments
  and search link to the author's profile. Names in the invite tree link to
  the profile too. Visitors see names as plain text.

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
| CSRF | SameSite cookies plus an Origin header check on POST. The Origin must match the base URL. A POST with no Origin is rejected | No token plumbing | Per-form CSRF tokens |
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
- Public mode is read-only. The server allows only the front page, thread
  pages and search for visitors, and sends every write to the login page.
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
- Public write access, public profile pages and a public invite tree.
- Downvotes, karma, categories, tags.
- Live updates for visitors, live updates on pages other than the thread
  page, and live updates for thread deletes.
- Typing indicators and online presence.
- Image or file uploads.
- Locked threads.
- Changing usernames.
- General rate limiting (only login throttling is in scope).
- Disabling a whole invite subtree in one action.
- Vote-based ranking of the front page.
- Running more than one instance.
- Cancelling invites.
- A global dark mode toggle; themes are per account.
