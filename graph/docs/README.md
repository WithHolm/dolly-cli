# The documentation of the Graph API

These files are the text of the *API* page of the portal. They are embedded in the program, and a test checks
that they fit the routes. This file is not a page.

One file is one page of the reference. To add a page, add a file. The portal needs no change.

    ---
    name: Users            the name of the page (required)
    order: 1               where it is in the list
    resource: users        the first part of the paths of the page. The default is the name of the file.
    covers: me=Signed-in user
                           more first parts for this page, and the section that their calls are in
    ---
    The text of the page, in markdown.

    ## GET /v1.0/users/{id}
    The text of one call. The title is the method and the path, as in the list of calls.

    ### Filtering
    A part of the call with a title.

- **Add to the generated text.** The portal makes the forms of `$filter`, the query options, and what Graph lists
  from the code and the description of Graph. Do not copy them here. Write what is special about *this* call,
  for example a filter that Graph has and Dolly refuses.
- **Titles.** `### Filtering` and `### Search` show next to the generated text for `$filter` and `$search`.
  Any other title is a heading of its own.
- **Only a call that exists** can have text. A test fails for a title that does not match a route, and for a route
  with no page. A call with no text is fine: its one-line summary comes from the code.
- **Write in short, simple sentences** (ASD-STE100), and only what you checked against a running Dolly.

While you write, run a `dev` build with `--docs graph/docs`. Dolly reads the files from that folder at each
reload of the page, so you do not rebuild. A release build has no such flag.
