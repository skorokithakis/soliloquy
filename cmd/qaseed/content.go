package main

import "time"

const day = 24 * time.Hour

// deepThreadTitle names the thread whose comment chain exercises the
// single-comment page and its parent chain. The seed tests refer to it.
const deepThreadTitle = "Postmortem: the cache stampede that took down checkout"

// userSpec describes one QA account. The admin is created without an inviter;
// members form a short tree under it. deleted accounts are created like any
// other, so their content can be seeded, and deleted at the end.
type userSpec struct {
	username  string
	password  string
	isAdmin   bool
	invitedBy string
	age       time.Duration
	deleted   bool
}

// qaUsers lists the accounts in invite-tree order, so an inviter always exists
// before the people they invited.
var qaUsers = []userSpec{
	{username: "admin", password: "admin", isAdmin: true, age: 45 * day},
	{username: "packet", password: "password", invitedBy: "admin", age: 38 * day},
	{username: "kernel", password: "password", invitedBy: "admin", age: 37 * day},
	{username: "nullptr", password: "password", invitedBy: "admin", age: 36 * day},
	{username: "tabs", password: "password", invitedBy: "packet", age: 35 * day},
	{username: "segfault", password: "password", invitedBy: "packet", age: 34 * day},
	{username: "rubberduck", password: "password", invitedBy: "kernel", age: 33 * day},
	{username: "coldstart", password: "password", invitedBy: "kernel", age: 32 * day},
	{username: "bytemare", password: "password", invitedBy: "nullptr", age: 31 * day},
	// ghost writes one comment in the deep thread, then deletes the account.
	// The comment stays and renders as "[deleted user]".
	{username: "ghost", password: "password", invitedBy: "packet", age: 30 * day, deleted: true},
}

// commentSpec is one seeded comment. parent is an index into the thread's
// comment list, or -1 for a top-level comment. edit replaces the body and
// marks the comment edited; deleted marks it deleted after its children and
// votes exist.
type commentSpec struct {
	author  string
	parent  int
	age     time.Duration
	body    string
	edit    string
	deleted bool
}

// threadSpec is one seeded thread.
type threadSpec struct {
	author   string
	title    string
	url      string
	age      time.Duration
	body     string
	comments []commentSpec
}

func qaThreads() []threadSpec {
	return []threadSpec{
		{
			author: "kernel",
			title:  "A practical guide to structured logging in Go",
			age:    9*day + 4*time.Hour,
			body: `Logging is not about printing more. It is about printing the fields you will search for when something breaks.

# Start with the question you ask at 3am

When a request fails you want to know which request, for which user, and what the service was doing. A message like "failed to load user" answers none of those.

> The best log line is the one you can find without knowing the exact wording.

## Give every request an id

Generate an id at the edge, put it in the context, and attach it to every line:

~~~go
logger := slog.With("request_id", requestID, "route", route)
logger.InfoContext(ctx, "loading profile", "user_id", userID)
~~~

A middleware is enough. You do not need a logging framework with a plugin system.

## Keep the message stable

The message is the thing you grep for. The fields are the things that change. If the message includes a user id, every search becomes a regex.

# A small field vocabulary

Pick a short list of names and stick to it:

| Field | Type | Why |
| --- | --- | --- |
| request_id | string | joins lines from one request |
| user_id | int | answers "who was affected" |
| route | string | groups errors by endpoint |
| duration_ms | int | separates slow from broken |

The exact names matter less than using them consistently.

# What we changed

- Replaced most formatted messages with a stable message and fields.
- Added the request id in one middleware instead of in every handler.
- Logged errors once, at the boundary that decides what to do with them.
- Stopped logging the same error at four levels of the stack.

# Things I would not do again

## Do not log the whole request body

It is tempting, and it is a privacy incident waiting to happen. Log a length, a hash, or nothing.

## Do not build your own levels

Use the ones in the standard library. A custom level called "important" will be used by exactly one person and forgotten by everyone else.

# A checklist

1. Does the line have the request id?
2. Is the message stable across code changes?
3. Is there a field for the user or account?
4. Would you understand the line at 3am with no other context?`,
			comments: []commentSpec{
				{author: "packet", parent: -1, age: 9*day + 2*time.Hour, body: "The request id at the edge is the part people skip. We put it in a middleware and everything downstream inherits it."},
				{author: "tabs", parent: 0, age: 9*day - time.Hour, body: "Do you also log it on errors returned to the client? We include it in the error body so support can ask for it."},
				{author: "coldstart", parent: -1, age: 8*day + 20*time.Hour, body: "The stable message advice is underrated. We changed a message once and broke three dashboards."},
			},
		},
		{
			author: "nullptr",
			title:  "SQLite in production: one year later",
			age:    8*day + 2*time.Hour,
			body: `We moved a small internal service from Postgres to SQLite a year ago. Not because SQLite is fashionable, but because the service had one writer and a lot of reads.

# The setup

The service runs in one container with a single process. The database file lives on a local volume, and every request opens a connection from a small pool.

## WAL mode and busy_timeout

These two pragmas did most of the work:

~~~sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
~~~

WAL keeps readers from blocking the writer. The timeout turns a hard failure into a short wait. Five seconds is longer than any transaction we expect, so a wait that long means something is wrong.

## Backups

We use the online backup API through a small Go program, not a file copy. Copying a database while it is being written is how you get a corrupt backup and a false sense of safety.

The backup runs every hour and keeps seven days. Restoring it is part of the runbook, not an afterthought.

# What broke

## The one writer problem

Everything was fine until a reporting query started running inside a write transaction. It held the lock for twenty seconds and the whole service queued behind it.

> SQLite does not have a concurrency problem. It has a transaction length problem.

We moved the report to a read-only connection and the queue disappeared.

## Schema changes during a deploy

Our first migration ran while old code was still serving traffic. The old code did not know about the new column, and one query started returning the wrong rows.

Now every schema change goes through the same two-step dance: add the new shape and backfill it, then switch the code in the next deploy.

# What I would change

Three rules came out of that year:

- Keep write transactions short.
- Back up with the database open, not with a file copy.
- Test the restore, not just the backup.

| Change | Why |
| --- | --- |
| Set a maximum transaction time | Makes a long lock a bug instead of an outage |
| Monitor write latency | It rises before users notice |
| Pin the SQLite version | The query planner changes between releases |

I would also write down the backup restore procedure earlier. We had a backup, but the first restore attempt took an hour because nobody knew which file was current.

# Would I do it again

Yes, for this service. It has one writer, a few gigabytes of data, and no need for a network database.

I would not use it for a service with many writers, long-running reports, or a team that expects a database server to be someone else's problem. The constraints are the point: if you can live with them, SQLite is remarkably boring.`,
			comments: []commentSpec{
				{author: "kernel", parent: -1, age: 8*day + time.Hour, body: "The busy_timeout value matters more than people expect. Five seconds is a long time to hold a write lock, but it beats SQLITE_BUSY."},
				{author: "bytemare", parent: 0, age: 7*day + 23*time.Hour, body: "We use a single writer goroutine and a channel. It made the failure mode obvious."},
				{author: "nullptr", parent: 1, age: 7*day + 20*time.Hour, body: "That is basically what we ended up with, though the queue depth is the real tuning knob."},
				{author: "rubberduck", parent: -1, age: 7*day + 18*time.Hour, body: "How do you handle backups? The copy while writing question still trips me up."},
			},
		},
		{
			author: "packet",
			title:  "The case for boring technology",
			url:    "https://example.com/boring-technology",
			age:    7*day + 6*time.Hour,
			body:   "A good reminder that the interesting choice is usually the one you can explain to the next person.",
			comments: []commentSpec{
				{author: "segfault", parent: -1, age: 7*day + 4*time.Hour, body: "The article is good, but the comments are better. Boring is a feature until you need to hire."},
				{author: "kernel", parent: -1, age: 7*day + 2*time.Hour, body: "We replaced a clever queue with Postgres and a cron job. Nobody has missed the queue."},
			},
		},
		{
			author: "tabs",
			title:  "How do you deal with flaky tests?",
			age:    7 * day,
			body: `We have a handful of integration tests that fail once every few hundred runs. Retrying them makes CI green, but it also makes the failure invisible. I would rather fix the cause than learn to live with the noise.

What has actually worked for you? I am less interested in "just use a fake clock" than in the process that helped you find the thing worth faking.`,
			comments: busyComments(),
		},
		{
			author: "coldstart",
			title:  "Everything I know about HTTP caching",
			age:    6*day + 3*time.Hour,
			body: `Caching is one of those topics where the rules are simple and the edge cases are endless. These are the parts I keep coming back to.

# Cache-Control is the contract

The header tells every cache in the chain what it may do. A private cache and a shared cache do not read it the same way.

| Directive | What it means |
| --- | --- |
| max-age=60 | fresh for sixty seconds |
| s-maxage=300 | shared caches keep it longer |
| private | only the browser may store it |
| no-store | do not write it to disk at all |

A response with no Cache-Control is not automatically uncached. Heuristics can apply, and proxies have their own defaults. If the answer matters, say it explicitly.

There is a common pattern for HTML that changes per user:

~~~http
Cache-Control: private, max-age=0, must-revalidate
ETag: "abc123"
~~~

This lets a browser reuse the body when the ETag still matches, without letting a shared cache serve one user's page to another.

> The fastest request is the one you never send. The second fastest is one that gets a 304.

# ETags save bandwidth, not requests

An ETag still costs a round trip. It saves the body, not the request. That is still a good trade for large responses, but it will not save your origin from a traffic spike.

We use strong ETags for immutable assets and weak ETags for HTML. Weak validators are enough for "did this content change", and they survive compression and proxies that rewrite the body.

A few rules that have served us well:

- Hash the response body, not the database row.
- Include the user id in the ETag when the body is personalised.
- Do not use the same ETag for different Content-Encoding values.
- Log 304s separately from 200s, or your dashboards will lie about bandwidth.

The last one is the easiest to forget. A page that returns 304 for most visitors looks like a traffic drop until you add the second line to the graph.`,
			comments: []commentSpec{
				{author: "nullptr", parent: -1, age: 6 * day, body: "The only thing I would add is Vary. Caching a response that varies by cookie without it is how you get mysterious bugs."},
				{author: "coldstart", parent: 0, age: 5*day + 22*time.Hour, body: "Yes. We also set Cache-Control: private on anything with a session."},
				{author: "packet", parent: -1, age: 5*day + 20*time.Hour, body: "ETags are great until your framework generates a different one on every render."},
			},
		},
		{
			author: "bytemare",
			title:  "Localisation notes: headings, slugs and search",
			age:    5 * day,
			body: `A few notes from making the site work in more than one language. Most of it was translation, but a surprising amount was anchors.

# Σημειώσεις για την τοπικοποίηση

Τα ελληνικά είναι μια καλή δοκιμή γιατί το αλφάβητο δεν μοιάζει με το λατινικό. Οι τίτλοι μεταφράζονται, αλλά τα anchors δεν πρέπει να αλλάζουν με κάθε μετάφραση.

# Configuration

The configuration page was the first one we translated. The heading stayed in English because it is also the name of a menu item, which is a compromise we may revisit.

# Configuration

This second heading exists on purpose. Two sections ended up with the same title after a rewrite, and the table of contents needed to tell them apart. The slug counter adds a suffix, so the links still work.

If you are building a table of contents, test this case. It is much easier to fix before the page is live than after someone has bookmarked the wrong section.`,
			comments: []commentSpec{
				{author: "tabs", parent: -1, age: 4*day + 22*time.Hour, body: "The duplicate heading case is why our TOC links all pointed at the same anchor for a week."},
				{author: "bytemare", parent: 0, age: 4*day + 20*time.Hour, body: "The slug counter fixed it. The Greek heading gets h-section-2, which is not pretty but at least unique."},
				{author: "rubberduck", parent: -1, age: 4*day + 18*time.Hour, body: "Search still works for the Greek text, so the anchor is the only ugly part."},
			},
		},
		{
			author: "coldstart",
			title:  deepThreadTitle,
			age:    4*day + 2*time.Hour,
			body: `Checkout returned errors for eleven minutes on Tuesday. The pricing cache entry expired at the same moment as a deploy, every checkout request missed, and the pricing database spent the next ten minutes answering the same query. Retries from the browser and from our gateway kept it there.

We have the graph and we have opinions. What I want is the part that comes after "add jitter": how do you survive a cold cache, and where do you put the retry budget? Concrete setups are more useful than principles.`,
			comments: deepComments(),
		},
		{
			author: "rubberduck",
			title:  "Show us your dotfiles",
			age:    4*day + 6*time.Hour,
			body:   "I will start: one giant bashrc, a vimrc I have not read in years, and a gitconfig with three aliases I actually use. What is in yours?",
			comments: []commentSpec{
				{author: "segfault", parent: -1, age: 4*day + 4*time.Hour, body: "Mine is mostly aliases I no longer remember writing."},
				{author: "coldstart", parent: -1, age: 4*day + 2*time.Hour, body: "I moved everything to a single Nix file and now I cannot debug my own shell."},
				{author: "rubberduck", parent: 1, age: 4*day + time.Hour, body: "That is the most honest Nix review I have read."},
				{author: "packet", parent: -1, age: 3*day + 22*time.Hour, body: "The only thing I insist on is a prompt that shows the git branch. Everything else is optional."},
			},
		},
		{
			author: "segfault",
			title:  "A tiny CSS reset that finally stuck",
			url:    "https://example.com/css-reset",
			age:    3*day + 8*time.Hour,
			body:   "The reset is short enough to read in one sitting. The form control section is the part I keep stealing.",
			comments: []commentSpec{
				{author: "tabs", parent: -1, age: 3*day + 6*time.Hour, body: "The reset is fine, but the part about form controls is what I actually needed."},
				{author: "bytemare", parent: -1, age: 3*day + 4*time.Hour, body: "We copied the first twenty lines and left the rest. No regrets."},
			},
		},
		{
			author: "packet",
			title:  "Postgres 18: what are you trying first?",
			url:    "https://example.com/postgres-18",
			age:    2*day + 12*time.Hour,
			body:   "The release notes are long, so this is the short version of what caught my eye.",
			comments: []commentSpec{
				{author: "kernel", parent: -1, age: 2*day + 10*time.Hour, body: "The new incremental backup stuff is the only feature I care about. Everything else is a nice bonus."},
				{author: "packet", parent: 0, age: 2*day + 8*time.Hour, body: "Same. We are waiting for the first point release before trying it."},
				{author: "nullptr", parent: -1, age: 2*day + 2*time.Hour, body: "The query planner changes are worth reading if you have any ORM-generated queries."},
			},
		},
		{
			author: "kernel",
			title:  "How do you handle feature flags?",
			age:    2 * day,
			body:   "We started with a config file, moved to a service, and are now wondering whether the service was worth it. How do you decide what belongs in a flag and what belongs in config?",
			comments: []commentSpec{
				{author: "coldstart", parent: -1, age: 1*day + 22*time.Hour, body: "Flags are great until nobody deletes them. We now put an owner and an expiry date in the flag definition."},
				{author: "kernel", parent: 0, age: 1*day + 20*time.Hour, body: "An expiry date that fails CI is the only one that works."},
				{author: "tabs", parent: -1, age: 1*day + 18*time.Hour, body: "We keep flags for kill switches and use config for everything else. The distinction helped."},
			},
		},
		{
			author: "coldstart",
			title:  "What's your deployment checklist?",
			age:    1*day + 6*time.Hour,
			body:   "Ours is a wiki page that everyone agrees is out of date. I am trying to rewrite it as a short list of things that actually catch problems. What is on yours?",
			comments: []commentSpec{
				{author: "rubberduck", parent: -1, age: 1*day + 4*time.Hour, body: "Mine starts with \"can I roll back?\" and ends with \"did I check the rollback actually works?\""},
				{author: "segfault", parent: -1, age: 1*day + 2*time.Hour, body: "We added a step to check the dashboards before and after. It caught a silent increase in 500s twice."},
				{author: "bytemare", parent: -1, age: 1 * day, body: "The checklist is only useful if someone else can follow it. We test ours by having a new hire run it."},
			},
		},
		{
			author: "tabs",
			title:  "What's the smallest useful side project you've shipped?",
			age:    9 * time.Hour,
			body:   "Not the most impressive, the smallest. The thing that took an evening and still saves you time. I need some encouragement to finish mine.",
			comments: []commentSpec{
				{author: "packet", parent: -1, age: 8 * time.Hour, body: "A CLI that renames screenshots by date. It saves me ten seconds a day and I think about it constantly."},
				{author: "kernel", parent: -1, age: 6 * time.Hour, body: "A cron job that emails me the weather. It has been running for four years."},
				{author: "nullptr", parent: -1, age: 3 * time.Hour, body: "A bookmarklet that removes tracking parameters. The whole thing is nine lines."},
				{author: "coldstart", parent: -1, age: 1 * time.Hour, body: "A tiny service that turns a directory of Markdown into a static site. It is held together with shell scripts."},
			},
		},
	}
}

// busyComments builds the 80-comment thread. Comments are added in
// chronological order, two hours apart, so a reply is always newer than its
// parent. The tree reaches six levels.
func busyComments() []commentSpec {
	comments := make([]commentSpec, 0, 80)
	add := func(author string, parent int, body string) int {
		age := 7*day - time.Duration(len(comments))*2*time.Hour
		comments = append(comments, commentSpec{author: author, parent: parent, age: age, body: body})
		return len(comments) - 1
	}

	c1 := add("packet", -1, "First question: is it actually the test, or is it the thing the test depends on?")
	c2 := add("kernel", c1, "This, @packet. We had a test that assumed time.Now() advanced between two calls.")
	c3 := add("nullptr", c2, "Same, but with time.Sleep. Replacing it with a fake clock fixed it.")
	c4 := add("tabs", c3, "Fake clocks are great until something hands the time to a real timer.")
	c5 := add("segfault", c4, "We inject a Clock interface and use the real one in one test.")
	add("rubberduck", c5, "At that point I would rather split the test than fake everything.")
	c7 := add("coldstart", c4, "We ran into that with a cache TTL. Now the cache takes a clock too.")
	add("bytemare", c7, "How do you test the cache without testing the clock?")
	add("kernel", c4, "The timer problem is usually a symptom of testing two things at once.")

	c10 := add("packet", c1, "If it only fails in CI, the environment is part of the test.")
	c11 := add("tabs", c10, "We pin the CPU count in CI now. That fixed a whole class of races.")
	c12 := add("nullptr", c11, "How do you pin it? GOMAXPROCS or a container limit?")
	add("tabs", c12, "GOMAXPROCS=2 in the test wrapper. Boring and effective.")
	c14 := add("kernel", c10, "The other CI-only failure is DNS. We point everything at a local resolver.")
	c15 := add("rubberduck", c14, "We had a test that resolved example.com. It was flaky for months.")
	add("packet", c15, "This is why our test config forbids non-local addresses. It fails fast.")

	c17 := add("segfault", -1, "Retries are not a fix, but they are useful as a signal.\n\nTrack which tests need them. The list is a to-do list, and it is much easier to prioritise than \"the suite is flaky\".")
	c18 := add("coldstart", c17, "We log every retry with the test name and the seed. The list is a to-do list.")
	c19 := add("bytemare", c18, "Do you keep it short? Ours grew until it became background noise.")
	c20 := add("coldstart", c19, "We delete an entry after a week if it has not retried. Keeping it short is the whole point.")
	comments[c20].edit = comments[c20].body + "\n\nEdit: I meant a week without a retry, not a week since it was added."
	c21 := add("tabs", c17, "The danger is that a retry hides a real data race for another month.")
	add("segfault", c21, "We run the race detector on the retried test immediately. It usually finds it.")

	c23 := add("nullptr", -1, "Use deterministic randomness. Seed from a fixed value and log the seed when the test fails.\n\n## The two things that helped us\n\n**Deterministic seeds.** We print the seed in the failure message, so a failing run can be reproduced locally.\n\n**A failure budget.** A test that needs more than one retry a week gets quarantined with an owner.")
	add("packet", c23, "We have a helper that prints the seed in the failure message. People actually use it. @nullptr I can share the snippet if you want.")
	add("kernel", c23, "For property tests this is essential. Without it, a failure is a ghost story.")

	c26 := add("rubberduck", -1, "Delete the test for a week and see if anyone notices. Sometimes the coverage was imaginary.")
	add("tabs", c26, "I have done this. It is a surprisingly good prioritisation exercise.")
	c28 := add("coldstart", c26, "We did it for a whole package. Two tests came back; the rest stayed deleted.")
	c29 := add("bytemare", c28, "What did the two tests cover? The interesting part is always what survived.")
	c30 := add("coldstart", c29, "One was a permissions check, one was a parser edge case. Both were real.")
	c31 := add("segfault", c30, "The permissions one is the sort of thing that never looks important until it is.")
	add("rubberduck", c31, "That is why I keep security tests out of the deletion experiment.")
	comments[c28].deleted = true

	c33 := add("packet", -1, "Split integration tests into their own package. Then flakiness is visible instead of mixed in.")
	add("nullptr", c33, "We also give them a different timeout. A slow integration test is not a flaky unit test.")
	c35 := add("kernel", c33, "The package split helped our CI, because we can run the flaky set less often.")
	c36 := add("tabs", c35, "Running them less often is how they become stale and start failing for new reasons.")
	c37 := add("kernel", c36, "Fair. We run them nightly and on the merge queue, not on every push.")
	comments[c37].edit = comments[c37].body + "\n\nEdit: to be clear, the unit tests still run on every push."

	c38 := add("segfault", -1, "Check for leaked goroutines. A test that passes alone can fail when the previous test leaves work behind.")
	c39 := add("coldstart", c38, "goleak is worth the dependency. It found three leaks on the first run.")
	add("bytemare", c39, "We use it only in the package test main. Running it everywhere was too slow.")
	c41 := add("packet", c38, "The other leak is a background goroutine writing to a closed channel. It panics in the next test.")
	add("segfault", c41, "That one is nasty because the stack trace points at the wrong test.")

	c43 := add("rubberduck", -1, "Make failure output useful. A flaky test that prints nothing is a support ticket.\n\nWe attach the last hundred log lines to the failure and upload the test database. It doubled the log size and halved the debugging time.")
	c44 := add("nullptr", c43, "We attach the last hundred log lines to the failure. It is the single best change we made. @rubberduck the upload part is the useful bit.")
	add("rubberduck", c44, "Worth it. We also upload the test database on failure.")
	add("tabs", c43, "Screenshots for browser tests. The difference between \"it failed\" and \"the button moved\" is huge.")

	c47 := add("kernel", -1, "Use t.Parallel carefully. It turns ordering bugs into flakes, which is good, but only if you can debug them.")
	c48 := add("bytemare", c47, "We found a shared map that way. The race detector was not catching it because the test was not parallel.")
	c49 := add("kernel", c48, "Exactly. Parallelism makes the bug reproducible; the race detector makes it visible.")
	add("coldstart", c49, "Unless the race is in a dependency. Then you get to read someone else's code at midnight.")

	c51 := add("packet", -1, "Check the test's assumptions about file system ordering. Directory listings are not stable.")
	add("tabs", c51, "We sort everything before comparing now. It removed a whole category of flakes.")

	c53 := add("segfault", -1, "The network is not a dependency you can ignore. Stub it at the boundary or use a local server.")
	c54 := add("nullptr", c53, "We use httptest.NewServer and a real client. It catches serialization bugs that mocks miss.")
	c55 := add("segfault", c54, "The tradeoff is port exhaustion when the suite grows. We reuse one server per package.")
	add("nullptr", c55, "Same, with a cleanup that waits for outstanding requests. Otherwise the next test sees the previous one.")

	c57 := add("rubberduck", -1, "If a test fails once a week, the cheapest fix is to make it faster and run it more. You will see the pattern.")
	c58 := add("coldstart", c57, "This sounds wrong but it works. A test that runs once a day hides the frequency.")
	add("rubberduck", c58, "We moved a flaky test to the pre-commit hook for a week. It failed three times and we fixed it.")
	c60 := add("bytemare", c57, "The opposite also works: quarantine it, but put an owner and a date on it.")
	c61 := add("rubberduck", c60, "A quarantine with no owner is a deletion with extra steps.")
	comments[c60].edit = comments[c60].body + "\n\nEdit: a date that is actually enforced by CI, not one in a spreadsheet."
	comments[c61].deleted = true

	c62 := add("kernel", -1, "Look for shared state between tests. A package-level cache is the usual suspect.")
	c63 := add("tabs", c62, "We reset our cache in TestMain. It is ugly, but it is one place instead of every test.")
	add("kernel", c63, "TestMain is underused. It also gives you a place for the leak check.")
	c65 := add("packet", c62, "The database is shared state too. A transaction per test is worth the setup cost.")
	c66 := add("nullptr", c65, "We use a template database and clone it. It is faster than migrations per test.")
	c67 := add("packet", c66, "How do you handle schema changes if the template is cached?")
	add("nullptr", c67, "The template key includes the migration hash. A schema change invalidates it automatically.")

	c69 := add("coldstart", -1, "When you find the cause, write a regression test that fails without the fix. Otherwise it comes back.")
	c70 := add("bytemare", c69, "And keep the old flaky test, renamed. The name should say what went wrong.")
	add("coldstart", c70, "We prefix them with regression: and link the incident. It makes the history readable.")

	c72 := add("segfault", -1, "Do not use sleep to wait for something. Poll with a timeout and fail with the current state.")
	c73 := add("tabs", c72, "We have a helper called eventually. It prints the last error and the elapsed time.")
	add("segfault", c73, "The elapsed time is the useful part. It tells you whether the system is slow or stuck.")
	add("rubberduck", c72, "Sleeps also make the suite slow enough that people stop running it locally. That is its own failure mode.")

	c76 := add("packet", -1, "One more: run the flaky test in a loop before you call it fixed. A hundred runs is a cheap experiment.")
	c77 := add("kernel", c76, "We have a make target for that. It takes the test name and runs it with count=100.")
	add("packet", c77, "Ours also sets -failfast. The first failure is the interesting one.")
	comments[c77].edit = comments[c77].body + "\n\nEdit: the target also uses a fixed seed, so a rerun is comparable."

	c79 := add("nullptr", -1, "At some point the answer is to delete the feature. A test that cannot be made reliable is often testing something nobody needs.")
	add("tabs", c79, "The hard part is admitting that. We spent a month on a test for an admin screen that two people used.")

	return comments
}

// deepComments builds the thread used to test the single-comment page and its
// parent chain. Comments are added in chronological order, 45 minutes apart,
// so a reply is always newer than its parent. The main chain reaches depth 16:
// a top-level comment at depth 1, a long reply at depth 2, a long comment at
// depth 8, and a long comment at depth 15. Two branches off the deleted comment
// at depth 11 sit at depth 12: one is a long leaf, the other a long comment
// with a small reply tree. The author of the comment at depth 10, ghost, loses
// their account after seeding.
func deepComments() []commentSpec {
	comments := make([]commentSpec, 0, 21)
	add := func(author string, parent int, body string) int {
		age := 4*day - time.Duration(len(comments))*45*time.Minute
		comments = append(comments, commentSpec{author: author, parent: parent, age: age, body: body})
		return len(comments) - 1
	}

	c0 := add("packet", -1, `We hit this exact wall two winters ago, on a catalogue page rather than checkout. Jitter helped with the normal expiry case and did nothing for the cold start, because there is no expiry to spread out when the cache is empty.

The distinction we ended up drawing was between a stale cache and a cold cache. A stale entry can be served while one request refreshes it. A cold cache has nothing to serve, so every request is a real miss unless you invent an answer.

Our first useful change was to keep the last good response longer than its freshness window. The cache entry got two deadlines: `+"`fresh_until`"+` and `+"`serve_until`"+`. Between them, the first request to arrive took a short lock and refreshed, and everyone else got the old body with a warning header. After `+"`serve_until`"+` we served an error page rather than a broken price. That one change removed most of the pressure, because the herd only forms when the alternative is a timeout.

The second change was admission control at the origin. We put a small token bucket in front of the expensive query, sized to what the database could actually sustain, not to traffic. Requests that did not get a token waited on the refresh that was already running instead of starting their own. The important detail is that the waiters shared one result: without that, we had replaced a stampede of queries with a stampede of connections holding a lock.

The third change was to make retries boring:

- Retry only idempotent reads, and only after a full response has failed.
- Use a fixed short budget, not exponential backoff with no ceiling. Three attempts in 800ms is plenty for a page a person is waiting on.
- Add jitter at the client, not only at the server. A hundred browsers that all start at the same second will still line up.
- Treat the retry budget as a shared resource. If the origin is shedding, the client should stop, not try harder.

The thing I would do differently is measure the cold path on purpose. We had load tests for the warm path and nothing for the state we actually shipped: an empty cache at 9am. Now we run one test per release that flushes the cache and measures the first hundred requests. It is ugly and it found two more problems in the first month.

One caution about locks: a refresh lock is a single point of failure. Ours has a short TTL and the holder renews it; if the holder dies, the next request takes over. We also log every lock takeover. When the number of takeovers jumped, it turned out the refresh was slower than the lock TTL, which is its own outage waiting to happen.

So my answer is not "add jitter". It is "decide what you will serve when you have nothing, and put a small, shared gate in front of the expensive thing". The cache is a latency optimisation; the gate is what keeps the database alive.`)

	c1 := add("nullptr", c0, `We did the two-deadline trick as well, and it worked until we added a second layer of caching at the CDN. Then the origin lock stopped helping, because the CDN was holding a stale copy and the browser was holding another one. The herd moved to the edge, where we could not see it.

What fixed that was making the freshness explicit in one place and letting every layer derive from it. The origin returns `+"`Cache-Control: max-age=30, stale-while-revalidate=120, stale-if-error=600`"+`, and the CDN honours the same numbers. We no longer have a CDN TTL that disagrees with the application TTL, which used to happen after every deploy.

The table we use for deciding the numbers looks like this:

| Path | Fresh | Stale-while-revalidate | Stale-if-error |
| --- | --- | --- | --- |
| Pricing | 30s | 120s | 600s |
| Stock | 5s | 30s | 60s |
| Recommendations | 5m | 1h | 1h |
| Account menu | private, 0 | 0 | 0 |

The account menu row is the useful one. Not everything should be cached, and pretending otherwise creates a different class of bug.

On retries, we went further than a budget: we made the retry decision at the edge, in a small worker, rather than in the page. A browser retry is invisible to us and hard to bound. A worker retry is a line in a metric. The page now gets either a response or a cached error, and only the worker decides to try again. The first week of that change was noisy because the worker retried requests that had already been answered by a user hitting refresh. Idempotency keys fixed it, but only after we started storing them per request rather than per session.

Something else worth saying: the database pool was not really full of work. It was full of waiting. We had a pool of forty connections and a query that took 300ms under load, so the throughput ceiling was about 130 queries a second, and we were sending 900. The pool was doing its job; the query was not. We spent a day on the query plan and got it to 40ms, which changed the arithmetic more than any of the caching work.

> A cache stampede is usually a capacity problem wearing a caching costume.

That is not an argument against caching. It is an argument for knowing which one you have. If the origin can serve the cold load, let it. If it cannot, the gate is not optional, and neither is the test that flushes the cache and measures what happens next.

The other thing I would add is a kill switch for the cache. During the incident we wanted to bypass it to see whether the cached body was wrong, and we had no way to do that without a deploy. Now there is a query parameter that only works for admins on the internal network. It has been used twice, both times during an incident, and both times it shortened the conversation.`)

	c2 := add("coldstart", c1, "The two deadlines are doing more work than the lock in my experience. We also made the refresh return the same promise to all waiters, so a burst of requests creates one query. The lock is only there to stop two refreshes, not to serialize readers.")
	c3 := add("packet", c2, "That matches what we saw. The lock without single-flight just moves the queue from the database to the application. We ended up with a map keyed by cache key and a channel per entry, which is ten lines and a lot of tests.")
	c4 := add("nullptr", c3, "Ten lines and a lot of tests is the correct ratio for this. The tests are all about the holder dying halfway through.")
	c5 := add("coldstart", c4, "We test that by cancelling the refresh context and asserting the next caller takes over. It is the only way I trust the timeout.")
	c6 := add("packet", c5, "Cancelling the context is a good trick. We use a fake clock for the TTL and a real context for the cancellation, which keeps the two failure modes separate.")

	c7 := add("nullptr", c6, `Here is the smallest version of the single-flight code we ended up with. It is deliberately boring, and it returns the same error to every waiter rather than pretending each caller has its own problem.

~~~go
type flight struct {
    done chan struct{}
    body []byte
    err  error
}

func (c *cache) get(ctx context.Context, key string) ([]byte, error) {
    c.mu.Lock()
    if f, ok := c.inflight[key]; ok {
        c.mu.Unlock()
        select {
        case <-f.done:
            return f.body, f.err
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    f := &flight{done: make(chan struct{})}
    c.inflight[key] = f
    c.mu.Unlock()

    f.body, f.err = c.fetch(ctx, key)
    close(f.done)
    c.mu.Lock()
    delete(c.inflight, key)
    c.mu.Unlock()
    return f.body, f.err
}
~~~

The parts that are not obvious:

- The fetch uses the first caller's context. If that caller disconnects, everyone waiting gets an error. We accepted that for reads, because a cancelled read is cheap to repeat, but it is a real tradeoff.
- The map entry is deleted after the close, not before. Otherwise a new caller can start a second fetch while the waiters are still reading the first result.
- The waiters do not hold the mutex while waiting. That sounds obvious until you see a version that does.
- We do not cache errors in the single-flight map. The flight is only for concurrent callers; the next call starts a new one.

We also put a maximum number of waiters on each flight. Beyond that, callers get a fast 503 and the retry budget at the edge decides what to do. Without the cap, a slow origin turns the waiting goroutines into the outage: memory grows, the scheduler gets busy, and the thing that was supposed to protect the database becomes the thing that falls over.

Two details cost us a bug each. The flight must use the remaining TTL of the entry, not the configured one, or a refresh can loop forever. And the flight key must include the tenant when the cache is shared, or one tenant's slow query blocks another tenant's fast one.

Two ownership rules are easy to miss. The body handed to every waiter is the same slice, so a caller that mutates it corrupts the value for everyone else and for the next cache reader. We return a copy at the public boundary and document that the flight owns the original. A copy of a small JSON body is cheap; if it is not cheap, the entry is too big to be moving through a hot path. The error is shared too, and callers must not wrap it in place.

The first version used one sync.Once per key. It is shorter, and it cannot retry: once the fetch fails the Once is spent, and every later caller gets the same failure until the entry is removed. It also gives waiters no way to stop waiting. A channel plus a context is a few more lines and it lets a disconnecting caller leave.

We also tried the singleflight package from x/sync. It is good code and it was a useful reference, but it does not take a context, it has no waiter cap, and Forget affects future calls rather than the one in flight. Those are reasonable choices for a library; they were the wrong ones for a request path with a deadline. Wrapping it to add the missing pieces took about as much code as writing the map ourselves, and our map gave us a place to put the metrics.

The map is per process, which is the limitation people underestimate. During a rolling deploy we run four processes, so a cold key produces four origin fetches, not one. That is usually fine: four is much better than four hundred. When it is not fine, the answer is a lease at the cache layer, not a bigger in-process map. We set a short-lived key such as `+"`refresh:pricing:v3`"+` with a five second TTL. The process that sets it refreshes; the others serve stale. The TTL means a crashed holder cannot block the key forever, and a random suffix in the value lets us tell two takeovers apart in the logs. We do not use the lease as a correctness mechanism. It only reduces duplicate work, so a stolen lease costs one extra query rather than a corrupt write.

Panics are the failure that is easy to miss. If the fetch goroutine unwinds without closing done, every waiter blocks until its own context expires and the map entry stays forever. We run the fetch inside a function that defers the close and the delete, and recovers the panic into an error that is logged once and returned to the waiters. The recovery is not there to hide the bug; it is there so one bad response cannot take out every request for that key.

The TTL interaction is the other subtle part. The flight does not decide freshness. The fetch asks the cache whether the entry is still usable, and writes the new entry before it closes done. If the write fails, the waiters still get the body, because they have already paid for it, and the next request tries again. The flight must use the remaining TTL of the entry, not the configured one, or a refresh can be scheduled before the previous write is visible and the same key refreshes in a loop. We store the deadline with the entry and pass it through.

The key must also include the tenant when the cache is shared. We learned that one in the most annoying way: a slow report for one customer blocked the pricing read for another, because both had the same key and the first caller's context won. A tenant id in the key is not a security boundary; it is a blast radius boundary.

The table we use when this goes wrong:

| Failure | What you see | Guard |
| --- | --- | --- |
| Fetch panics | waiters time out, map grows | defer close and delete, recover once |
| Waiter cap hit | fast 503s, origin is fine | cap waiters per key |
| Distinct-key flood | many flights, memory rises | cap total inflight fetches |
| Leader disconnects | everyone gets a context error | fetch on a bounded background context |
| Multi-process deploy | origin load times instances | short lease at the cache layer |
| Stale entry refresh loop | misses never end | use remaining TTL, not configured TTL |

The tests I would write first are not the happy path. Start two callers, cancel the first, and assert that the second gets either the result or a clean context error, not a deadlock. Start a fetch that panics, and assert that the waiters get an error and the map is empty afterwards. Put a hundred waiters behind a cap of ten, and assert that ninety get 503 without allocating a goroutine each. Run the whole thing under the race detector, then run it with the fetch slowed to a second and the client timeout set to a hundred milliseconds. That last test is the one that catches a waiter that ignores its own deadline.

On sizing: we allow 128 waiters per key and 64 inflight fetches per process. Those numbers come from the origin's concurrency limit and the memory we are willing to spend on parked goroutines, not from traffic. If the cap is reached, the correct answer is a fast 503 and a `+"`Retry-After`"+` header, not a longer queue. A queue in front of a saturated origin is just a slower way to fail.

One more thing: instrument the number of inflight fetches and the number of waiters. When those two numbers diverge you have a slow query. When they are both high you have a hot key. Those are different bugs and they need different fixes. Per-key counters are worth the cardinality; a single global gauge cannot tell you which key to fix.`)

	c8 := add("coldstart", c7, "The context tradeoff is the one I always get wrong. We switched to a background context with a deadline for the fetch, so one disconnecting browser does not poison the result for everyone else.")
	c9 := add("ghost", c8, "A background context with a deadline is also easier to test, because the deadline is a value you can control rather than whatever the client happened to send.")

	c10 := add("segfault", c9, "We fixed this by turning off retries entirely during the incident and letting the error page handle it. I do not think the cache should retry at all; that belongs to the client, where you can actually see it.")
	comments[c10].deleted = true

	c11 := add("coldstart", c10, "Turning them off is the right default, but only if the error page is honest about it. Ours used to say something went wrong, so people pressed refresh, which is a retry with extra steps.")
	c12 := add("packet", c11, "The button is the part everyone forgets to count. We added a fifteen second cooldown to ours and the load during an incident dropped noticeably.")
	c13 := add("coldstart", c12, "Did the cooldown annoy anyone? We were worried about people who genuinely need to retry after fixing a typo.")

	c14 := add("packet", c13, `The cooldown is fine if it is short and the message explains it. Ours is ten seconds, and the page says "you can try again in a moment" instead of disabling the button silently. We also exempt one retry when the form has changed since the error, because that is usually someone fixing a typo.

The cookie fallback is where ours fell apart, for a reason worth writing down. A cookie is sent on every request to the origin, so a global budget stored in one is both visible to the server and impossible to update atomically. Two tabs read the same value, both decide they have budget, and both spend it.

We replaced it with two numbers in a single cookie: a window start and a count. Each tab increments before it fires and re-reads after. That is not atomic either, but it fails in the safe direction: the count can be too high, never too low. We would rather drop a retry than add one.

~~~go
// spend reports whether the tab may retry, and records the attempt.
func spend(c *http.Cookie, now time.Time, limit int) bool {
    window, count := decode(c.Value)
    if now.Sub(window) > time.Minute {
        window, count = now, 0
    }
    if count >= limit {
        return false
    }
    c.Value = encode(window, count+1)
    return true
}
~~~

There is a race between the read and the write, and we accepted it. The consequence of losing the race is one extra retry, which is cheaper than a lock in a cookie. If the consequence were a duplicate payment, we would have done it on the server.

The cookie itself is deliberately unglamorous. It is scoped to the origin, marked Secure and SameSite=Lax, and readable by JavaScript because the page has to update it. It is not HttpOnly for that reason, and it contains no secret: the worst a user can do by editing it is retry more often. We still keep a server-side limit, because a client-side budget is a politeness mechanism, not a security control. Anyone who wants to bypass it can open a private window.

The clock is the part that breaks in the field. We write an absolute window start, not an elapsed time, and clamp anything more than an hour in the future. A clock that jumps forward expires the window early, which fails safe: the next retry starts a new window. A clock that jumps backward would make the window look fresh for too long, so we ignore window starts older than the cookie's own Max-Age and reset. The sleep case you hit is the same bug from the other direction, and it is why we never compare against a timestamp we did not write.

~~~go
func windowStart(c *http.Cookie, now time.Time) time.Time {
    start, _ := decode(c.Value)
    if start.After(now.Add(5 * time.Minute)) || start.Before(now.Add(-time.Hour)) {
        return now
    }
    return start
}
~~~

The service worker path is better where it exists, but it is not a replacement for the cookie. The Cache API gives every tab of the same origin one store, so the counter is shared, and the worker survives a page reload. It does not help in a private window, it does not exist in every browser we support, and it is cleared more aggressively than a cookie. Ours writes both and uses the higher count when they disagree. The two paths are small and they fail independently, which is the point.

The budgets we ended up with:

| Budget | Shared by | Survives reload | Notes |
| --- | --- | --- | --- |
| Per tab, in memory | one tab | no | trivial, useless alone |
| Cookie | tabs and private windows | yes | sent on every request, racy |
| Service worker cache | tabs of one origin | yes | not everywhere, cleared sometimes |
| Server token bucket | everyone | yes | authoritative, needs identity |

The server-side bucket is the one that actually protects the origin. The client budgets only stop the easy stampede: a hundred tabs that all retry once. A determined or buggy client gets past them, so the origin still needs a per-IP and per-session limit and a `+"`Retry-After`"+` it believes. We use 503 for shed load and 429 for a client that is over its own budget; the first says "not now", the second says "you, specifically, not now". The CDN treats them differently: stale-if-error covers both, but only the 503 is retried by our worker. Getting those two backwards caused a small outage of its own, because every 429 turned into a worker retry that hit the same limiter again.

The other half of the problem is telling the user what happened. An error page that says "try again" invites the refresh loop. Ours now says "we are busy, this usually clears in under a minute", shows the request id, and disables the button for fifteen seconds. Support tickets dropped after that change, not because the outage was shorter but because the page stopped sounding broken.

We also stopped disabling the button silently. A disabled control with no explanation reads as a broken page, so the button keeps its label and a countdown sits next to it, in a live region so a screen reader announces the change. The form keeps its values, because asking someone to re-enter an address during an outage is how you lose the order twice. And the retry is a fresh navigation with an idempotency key, not a second submit, so a retry that overtakes the first response cannot charge twice.

A few things we check before calling a retry policy done:

- Kill the origin for five minutes in staging. The page must fail in under two seconds and must not keep retrying.
- Double the client count. The number of requests reaching the origin must stay flat.
- Block the retry endpoint. The user must still see a readable page.
- Set the system clock forward by an hour. The budget must reset once, not lock the user out.
- Set the clock backward by an hour. The budget must not stay spent forever.
- Open four tabs and retry in all of them. The origin must see one retry, not four.
- Remove the service worker. The cookie path must still enforce the budget.
- Edit the cookie to a huge window. The server must still shed the load.

The clock case is not hypothetical. A laptop waking from sleep had a clock jump, and our window arithmetic treated the cookie as eternally spent. The user could not retry until they cleared cookies, and of course they did not know that.

Once the mechanics are in place, measure them. We record retries attempted, retries allowed, and the delay between the error and the successful retry, labelled by route. The most useful graph is not the retry count; it is retry success. If retries almost never succeed, the client is wasting its budget on a failure that needs a person, and the page should say so instead of offering the button. That graph is also how we found a route whose retries always landed on the same unhealthy backend, because the load balancer had not removed it yet.

None of this is as satisfying as a clever algorithm. It is mostly deciding what failure looks like and making sure the page can show it. That is also why I like the 503 plus `+"`Retry-After`"+` approach: it gives every layer a shared vocabulary for "not now".`)

	add("nullptr", c14, "The clock jump is a great catch. We now store an absolute timestamp and treat anything more than an hour in the future as invalid.")

	cA := add("coldstart", c10, `I agree with the direction, but "let the client decide" hides a problem: the browser cannot tell the difference between an origin that is shedding load and one key that is slow. It sees a timeout and retries, and if every tab does that you have recreated the stampede one layer out.

The split that worked for us was to separate the retry decision from the retry execution:

- The page does not retry. It renders an error with a request id.
- A service worker queues one retry after a jittered delay if the response was a 503 with `+"`Retry-After`"+`.
- The worker has a per-tab budget and a global budget stored in the cache API.
- If the origin says `+"`Retry-After: 30`"+`, the worker does not retry sooner, even if the user presses the button.

That last one is the part people skip. A user pressing refresh is a retry. If your budget does not include the human, it is not a budget.

We also stopped returning 500 for shed load. A 503 with a short `+"`Retry-After`"+` is a message the whole chain can act on. Our CDN can serve stale on 503, the worker knows what to do, and the graph separates "we said no" from "we broke". Before that, every shed request looked like a bug and the on-call engineer spent twenty minutes looking for one.

There is a case for retrying inside the cache layer, though, and it is not the one in the comment above. If the cache can serve a stale body while it refreshes in the background, the refresh can retry as much as it likes without any user waiting. That is a background job with a deadline, not a request path. We run it with a two-minute budget and a circuit breaker per upstream. The breaker opens after five consecutive failures and stays open for thirty seconds. When it is open, the cache serves stale until the serve deadline and then fails closed. That is a deliberate choice: a wrong price is worse than no price.

The thing I would not do is a retry loop with exponential backoff inside the request handler. It holds a connection, it hides the failure from the latency graph, and it makes the timeout budget impossible to reason about. We had one of those for a year. It looked like the p99 was fine because the retried requests were recorded under a different route.

If you want a rule of thumb: retries belong where the work can be abandoned without anyone noticing. For a page, that is almost never the request handler.

The one exception is idempotent reads with a tiny, fixed retry count and a shared budget. Three attempts in under a second, all inside the same deadline. Anything more elaborate belongs in a queue.

> If a retry cannot be abandoned, it is not a retry. It is a second request with extra steps.

We learned that the hard way during the incident that started this thread.`)

	cA1 := add("packet", cA, "The 503 split is the change I would push for first. We used to return 500 for everything and the dashboards were useless: we could not tell load shedding from a real bug until someone read the logs.")
	add("nullptr", cA, "How do you store the global budget? localStorage is per origin but not shared across tabs in every browser, and the cache API is not available everywhere we still support.")
	add("coldstart", cA1, "We put a small counter in the service worker and fall back to the cookie when the worker is not available. The cookie path is rarer than the support matrix suggests.")

	add("nullptr", c10, `There is a second failure mode hiding in this thread: the hot key that never expires and the cold key that expires too often look the same in the cache metrics, and they need opposite fixes.

For a hot key, the problem is throughput. One key is requested thousands of times a second and every miss is expensive. For a cold key, the problem is that the first request after a deploy is always a miss. Our dashboards used to report one "miss rate" and we spent a week tuning the wrong one.

The split we use now:

| Signal | Hot key | Cold key |
| --- | --- | --- |
| Misses per key | high, concentrated | low, spread out |
| Origin latency | high for one query | normal |
| Cache age at miss | near zero | zero or missing |
| Fix | single-flight and a longer soft TTL | warm the cache before traffic |

For the cold key we added a pre-warm step to the deploy. It reads the twenty most expensive keys in the background and writes them to the cache before the load balancer sends real traffic. It is not clever and it does not cover every key, but it removed the nine o'clock spike on the pricing page.

For the hot key, the fix is the opposite of a cache change. If one key is that popular, it should be a materialised value, refreshed on a schedule, not a cache entry that a request happens to fill. We moved pricing to a small table that is written every ten seconds by one worker. Reads are a single indexed lookup with no cache at all. The cache is now the table, and its failure mode is a stale value, not a stampede.

That is not always possible. When it is not, the next best thing is a per-key lock with a short TTL and a documented takeover behaviour. What is not acceptable is a lock with no TTL, because the first process to die while holding it turns a slow page into an outage that needs a restart to clear.

We also started recording the key in the slow-query log. It is a small change and it made the investigation in this thread possible: without it we could see that the database was busy but not that it was busy with the same five keys.

The uncomfortable part is that both fixes are capacity work. Caching bought us time to do it, and we spent some of that time adding more caching. I would not recommend that trade.`)

	return comments
}
