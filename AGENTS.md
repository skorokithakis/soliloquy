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
