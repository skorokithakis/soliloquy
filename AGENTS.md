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
