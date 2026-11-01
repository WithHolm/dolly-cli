# Workflow: keep the library guidance true

For an AI agent (or a person) that must read, check, or update the guidance about sign-in libraries. The guidance is [../libraries.md](../libraries.md). The checks are in this folder. Follow the steps in order.

## Rules

1. **Only what was checked.** A row, a setting, or an error text in [libraries.md](../libraries.md) must come from a run of `npm run check` or from a run that you made and describe. If you did not check it, say "not tested". Never fill a row from memory of how a library should work.
2. **The run is the source.** If [libraries.md](../libraries.md) and the run disagree, the run is right. Change the page.
3. **A failed row is information.** Do not change `expect` to make a row green. First find the cause: a new version of a library, or a change in dollycli. Then decide which of the two is right (see "A row fails").
4. **Say what you could not run.** A skipped case (no Chromium), a library that you did not install, or a port that was busy goes in your answer and, when it matters, in the page.
5. **Write like the other docs.** Short, simple sentences. The name is dollycli (the command is `dolly`). No claims about hosting or about real Entra that you did not check. A guess is marked as a guess.
6. **Use the Edit tool** for changes to existing files.

## 1. Run the checks

    make compat

Or, in this folder, `npm install && npm run check`. Read the whole table. Note the versions on the last line.

If it cannot start: Go builds dollycli (`go build ./cmd/dolly`), ports 8661, 8662, and 3017 must be free, and the browser cases need Chromium (`CHROMIUM=...`).

## 2. Compare with the page

Open [libraries.md](../libraries.md). For each library in the first table, check that the version, "Needs https", "Other settings", and "Result" still match the rows of the run. Check the errors table the same way: every error text in it must be in the `detail` of a row (or be marked "not tested").

All rows ok and the page matches: you are done. Only change the date line at the top of [libraries.md](../libraries.md) if you ran all the cases again.

## 3. A row fails

Find out which side moved.

- **A library changed** (you bumped a version, or `npm install` gave a newer one). Read what the detail says. Update the setting or the result on the page, and the `expect` and `text` of the case, so that the case describes the new behavior. Keep the old behavior in the page if people still use the old version ("up to 5.27: ...").
- **dollycli changed.** Run `go test ./login ./shared`. If a test fails, the change is probably a mistake: fix dollycli, not the case. If the tests pass and the new behavior is right, update the case and the page.
- **Neither.** The case may be flaky (a browser case that did not wait long enough). Run it again. If it still fails sometimes, make the wait in `webApp` ([lib.mjs](lib.mjs)) longer, and say so.

## 4. Test a new library or a new version

1. Add it to [package.json](package.json) with an exact version. Run `npm install` (this updates [package-lock.json](package-lock.json)).
2. Add one or more cases to [cases.mjs](cases.mjs), following the pattern of the cases that are there:
   - Node library: a function like `msalNode` or `openidClient`. It does the code flow with `pick(...)` for the user, then checks `/v1.0/me`.
   - Browser library: a page for `webApp(...)`. It writes lines in `<pre id=out>` and the case reads them. Copy `oidcClientTs` and change the library.
   - Cover the settings that matter: the plain case (no setting) and the case with the setting that makes it work. The plain case usually has `expect: "fail"` with the error text.
3. Run `npm run check`. Every new row must be as you expect, and the old rows must still pass.
4. Update [libraries.md](../libraries.md): a row in the first table, a section for the library with a working snippet (take the settings from the case), and rows in the errors table for new errors. Add the library to the `versions` list in [run.mjs](run.mjs) so that its version is printed.
5. Add the library name and version to the list in the README of the repo (the section "A web app with a library").

## 5. Report

End your answer with:

- the command that you ran and its last two lines (the libraries, and the count of cases that do what they expect);
- what you changed in [libraries.md](../libraries.md) and why;
- what you did not check.
