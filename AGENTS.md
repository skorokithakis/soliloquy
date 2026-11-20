# Notes for agents

## Design reference

The repository has a design reference for every screen of the app. Read it
before you change templates or CSS, and compare your result with it.

- `misc/design/` has one HTML file and one PNG screenshot for each of the 14
  screens (desktop, mobile and dark). `misc/design/README.md` explains the
  files.
- `misc/design_reference.html` is the original export. It needs a browser with
  JavaScript, so use the unpacked files in `misc/design/` instead.

The app must match the design unless a deviation is deliberate. Deliberate
deviations are recorded in gnosis. Run `gn show design-reference` to see them.

## QA data

`.symphony/serve` seeds `.symphony-cache/qa.db` on every run with `cmd/qaseed`.
The seeder does nothing when the database already has threads.

- Admin account: `admin` / `admin`.
- Eight members: `packet`, `kernel`, `nullptr`, `tabs`, `segfault`,
  `rubberduck`, `coldstart`, `bytemare`. Their password is `password`.
- Twelve threads from the last ten days, including a busy thread with 80
  nested comments, long posts with a table of contents, and votes and
  reactions spread across the content.

Delete `.symphony-cache/qa.db` (and its `-wal` and `-shm` files) to reseed with
fresh relative dates.

## Finishing work

When you finish a task, commit and push the changes. Do not wait to be asked.
Skip this only when the user tells you not to commit or push.

## Commit messages

Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/).
Release Please reads them to choose the next version and to write the
changelog, so a commit without a type does not appear in the changelog.

- `feat:` is a new feature. It bumps the minor version (`0.1.0` to `0.2.0`).
- `fix:` and `perf:` bump the patch version (`0.2.0` to `0.2.1`).
- `feat!:` or a `BREAKING CHANGE:` footer also bumps only the minor version.
  We do not bump the major version automatically.
- `chore:`, `docs:`, `refactor:`, `test:`, `ci:` and `style:` do not start a
  release. They are included in the next one.

Keep the ticket reference at the end of the subject, for example
`feat: add thread pinning (STA-700)`.
