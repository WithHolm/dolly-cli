---
name: update-cost
description: Update the Graph throttling and cost data of this repo (graph/cost). Use when
  the user asks to update, refresh, or rebuild the cost data, the throttling limits, or
  cost.json, or runs /update-cost.
---

# update-cost

The cost data is in `graph/cost/`. The rules for `cost.json` and its form are in
`graph/cost/README.md`. That file is the one source for them: read it first, and do not
copy its rules here.

Steps:

1. Run `make update-cost`. It needs the network and changes only `graph/cost/raw/`.
   If GitHub limits the call, ask the user for a `GITHUB_TOKEN`. Do not look for one.
2. Read `graph/cost/raw/SOURCE.json`, `graph/cost/raw/concepts/throttling-limits.md`, and the
   files in `graph/cost/raw/includes/`.
3. Write `graph/cost/cost.json` as `graph/cost/README.md` says ("Make cost.json").
   Use Edit or Write. Do not use a script to read the tables. Copy every number as it is, and
   never guess one.
4. Run `go test ./graph/cost`. Fix `cost.json` until it passes. Never change `raw/` or the
   test to make it pass. The Go types in `graph/cost/cost.go` are the schema: the loader refuses
   a field that they do not have. If the source has something that they cannot hold, tell the
   user and propose a change to the types. Do not invent a field.
5. Compare three services other than identity by hand with their include files (pick ones
   with different table forms, for example Outlook, Cloud communication, and Files and lists).
6. Tell the user: the commit and date of the data, how many services have limits, which
   numbers changed from the old `cost.json` (use `git diff`), and anything you left out
   with a note because the source was not clear.

Do not commit. Do not use the cost data against a real tenant: this is data about the real
Graph, and Dolly is the only thing that reads it.
