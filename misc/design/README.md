# Design reference

`../design_reference.html` is the original export. It is a self-unpacking
bundle and needs a browser with JavaScript.

This folder has the same 22 screens unpacked, for easy reading:

- `NN-name.html` is the markup and CSS of one screen. Each file has its own
  copy of the shared CSS. The `{{...}}` placeholders and the `<script>` tags
  belong to the design tool. Ignore them.
- `NN-name.png` is a screenshot of that screen. Desktop screens are 1280 px
  wide and mobile screens are 390 px wide.
- The seven comment screens (`15-` through `21-`) are one template in different
  states: how much parent context is open, which reactions are shown, and
  whether the visitor is logged in.

These files are a reference only. The app does not use them.
