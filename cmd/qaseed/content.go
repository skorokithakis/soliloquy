package main

import "time"

const day = 24 * time.Hour

// userSpec describes one QA account. The admin is created without an inviter;
// members form a short tree under it.
type userSpec struct {
	username  string
	password  string
	isAdmin   bool
	invitedBy string
	age       time.Duration
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
