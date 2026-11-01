# Library checks

These scripts check that sign-in libraries work with dollycli. The page [../libraries.md](../libraries.md) is made from their results.

    make compat                        # from the root of the repo
    cd docs/compat && npm install && npm run check

[run.mjs](run.mjs) builds dollycli (or takes `DOLLY_BIN`), and starts two copies with their own data in a temporary folder: one on http (port 8661), one on https (port 8662, `--tls`). It then runs every case of [cases.mjs](cases.mjs), prints a table, and stops the servers. The exit code is 1 when a case does not do what it expects.

It needs Go, Node 22 or newer, and Chromium (`CHROMIUM=/path/to/chromium`, or one of the usual paths). Without Chromium the browser cases are skipped. Port 3017 must be free: the browser cases run a web app there.

## What keeps the checks true

- A case that must work has `expect: "ok"`. A case that must stop (a library that refuses `http`) has `expect: "fail"` and `text`, a piece of the error message. A case passes only when it does what it expects. When a new version of a library, or a change in dollycli, changes a result, the row says `FAILED`.
- Node trusts the CA of the https server through `NODE_EXTRA_CA_CERTS`. Chromium trusts the public key of that server's certificate (`--ignore-certificate-errors-spki-list`), and no other certificate. No case switches the check of certificates off.
- A person picks the user on the sign-in page. In Node the script sends the same form that the page sends (`pick` in [lib.mjs](lib.mjs)). In Chromium it clicks the button.

## Files

| File | What it is |
|---|---|
| [run.mjs](run.mjs) | Builds and starts dollycli, runs the cases in a child process, prints the table. |
| [cases.mjs](cases.mjs) | The cases. One entry in `cases` is one row of the table. |
| [lib.mjs](lib.mjs) | Helpers: `pick` (the sign-in page), `webApp` (a web app in Chromium), the addresses. |
| [package.json](package.json), [package-lock.json](package-lock.json) | The libraries, with exact versions. These are the versions that [libraries.md](../libraries.md) names. |

To change what is checked, or to add a library, read [WORKFLOW.md](WORKFLOW.md).
