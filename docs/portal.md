# The portal

The portal is Svelte 5 with TypeScript and Vite. `ui/dist` is in the repo, so `go install` needs no Node. After a
change in `ui/src`:

    make ui        # npm ci, a type check, the tests of the logic, and the build into ui/dist. Commit the new ui/dist.
    make ui-dev    # live reload on http://localhost:5173, against a 'dolly serve' on port 8650

The portal has no state of its own. The pages Users, Groups, and Organization (and the sample user on the Sign-in page) are a client of the
Graph API: the portal signs in as its own service principal, *Dolly portal*, and calls `/v1.0` like any tool. That service principal is not
throttled, and its calls are not in the list of calls. The other pages (Calls, Service principals, Data, History, API) use the admin API of
each API (`/_dolly/<name>/`), because they show what Graph does not have: usage, behavior, the history, and the permissions of a client.

On the page *Users*, a click selects a user, and a double click (or Enter, or the button *Open details*) opens a dialog with four tabs:
*Properties* (and the job, which you can change), *Organisation* (the managers above, the user, and the reports, as a tree: a click on a name
opens that person), *Groups*, and *Enterprise applications* (the app roles that the user has, also through a group, and the consents that apply). The address has
the user and the tab (`#/graph/users/<id>/organisation`), so a link opens the dialog on a tab.

The page *Service principals* works the same way. The dialog has three tabs: *Overview*, *Sign-in* (permission mode and credential), and
*Permissions*. On the *Overview* tab, two switches set how dollycli treats the calls of that client. *Never throttle this client* lets it
pass the limits of Graph (its cost is still counted). *Keep its calls out of the list of calls* hides its calls from the page *Calls*.
The same switches are in the admin API: `PUT /_dolly/graph/service-principals/<client ID>/flags` with `exemptFromThrottle` or `hideCalls`.

The page *App registrations* lists the app registrations of the tenant, and the page *Service principals* the service principals. They are two objects
in Entra, with the same client ID and different object IDs: the [app registration](graph.md#app-registrations-and-service-principals) is the definition
of the app, and the service principal is its instance. The *Overview* of a service principal shows both IDs. A service principal with the tag
`WindowsAzureActiveDirectoryIntegratedApp` has the chip *enterprise application*. One without it has *no enterprise tag*, and the dialog says that Entra
shows only service principals with that tag under Enterprise applications.

The *Permissions* tab of a service principal starts with *Needed by its calls*. dollycli notes the calls of a client by kind of token (delegated:
the token has `scp`, application: it has `roles`), and looks up in [graph/permissions/operations](../graph/permissions/operations/README.md) which
permissions allow each call. The tab shows the *Minimum*: the permissions with the least privilege that cover all the calls, and the calls with their
least privileged permissions. When the client has permissions, one line compares them with the calls: *Looks good*, *Missing …* (*Grant the missing* adds
them with the grant API that the picker uses), or *Could be lowered: A to B* (A allows the calls, but B is enough). *Not used* lists permissions that no
call needed so far: the client may make calls that dollycli has not seen. Nothing is ever refused: the comparison is advice, and the mode of a client
stays open. *+ Permission* opens the picker. A permission that Microsoft's pages name and that is not in dollycli's list of the permissions of Graph cannot
be granted: the tab says so. The minimum is a simple search over Microsoft's tables, and these are not always complete. Check it against real Graph.
The notes are in memory, and *Forget these calls* (or the reset of the use) clears them.

The use of resource units (RU) over time is drawn in two places: a chart of all clients above the list, and a chart of one client on the *Overview*
tab of its dialog, with a table of its calls and RU by route (the rows of the cost table). A bar is the most RU that one bucket of 10 seconds had.
10 seconds is the span of the RU limit of a client in a tenant, so in the dialog the limit is a dashed line (3500, 5000, or 8000 RU by the size
of the tenant, from [graph/cost](../graph/cost/README.md)). A bar with throttled calls is red and has a mark. The tooltip has the time, the peak, the
RU and calls of the bar, and the throttled calls. The ranges are 1 minute up to what the history holds (the default view is 15 minutes), and
*Scale to the limit* makes the top of the chart the limit. A range of more than 40 minutes groups buckets into bars, and a bar then shows the
peak of its buckets.

The server keeps the history in memory, in buckets of 10 seconds, for one hour. `--usage-history 6h` (or `DOLLY_USAGE_HISTORY`) sets it, from 1
minute to 24 hours. It is gone when dollycli stops, and `DELETE /_dolly/graph/usage` clears it. The admin API gives it:
`GET /_dolly/graph/usage/history` for all clients, or `?app=<client ID>` for one, with a point `{t, ru, calls, throttled}` for every bucket
(the empty ones too) and the `limit`. At most 200 clients have a series of their own. A client ID comes from the token, so the clients after
that are counted together as `other`.

The page *Generator* (next to *Sign-in* in the bar) is a wizard for a [profile](profile.md). You start from the current data (the profile that
it was made from), the default, Fjordvik (the example in [profiles/norway.json](profiles/norway.json)), or a file. Then you go through these steps:

- *Organization*: the display name and the tenant name are enough. The initial domain follows the tenant name, and the country sets the usage
  location, the language, and the phone code. "+ Domain" adds a domain. A radio marks the default domain, and the bars say how often a domain is used
  in the addresses of the users. Company, address, and the rest are under *More*.
- *Offices*: define the offices first. The country sets the usage location and the calling code. Below the offices, a bar for each one sets the share
  of the people who work there.
- *Departments*: a bar for each department, and under it the job titles, each with its own bar. The manager word is here too: a job title with
  that word is a manager role (the page marks these titles), and it decides who the managers are in the data (`/users/{id}/manager`). The first
  manager of a department is its head. A department with no manager title has its first person as the head.
- *People*: bars for the shares, the guest domains, and the languages. Each language has its own lists of names: a person gets a language by the
  shares, and a name from the list of that language. A country fills a list (made-up names for 12 countries), and you can change it.
- *Formats*, *Attributes* (the extension attributes, with weighted values or a pattern), and *Owner and admin*.

The bars of a list (the offices, the departments, the job titles of a department, the languages, the domains, and the values of an attribute) are
percents with one decimal (34.3%), and each list adds up to 100%. When you raise one, all the others give the same amount (in steps of 0.1%; a
leftover of rounding goes to the smaller ones, so the bigger ones give a little more), and none goes below 0.1%. A tenth of a percent is finer than
a pixel of a bar, so the others do not jump while you drag. In a file the weights are whole numbers with the sum 1000. A drag is worked out from where the shares were when it began,
so a slow drag takes from all of them. A lock beside a share keeps it: the others cannot take from it, and it cannot be set. When every other share
is locked, a share cannot change, and "+" is off until one is unlocked. A lock is of the editor and not of the profile, so it is gone when the page
is loaded again. When you add an item, it gets an equal share from the free items, and they keep their ratios. A bar changes when you drag it, press the arrow keys (Shift for steps of 10), or
type in the box. A profile file with other weights is scaled to 100 when the wizard opens it, with the same ratios.

A step shows its errors when you press *Next*, and stays when it has one. The editor has no rules of its own: the server checks the profile and
the page shows the errors with the names of the fields, and marks the fields. The work stays in the tab when you load the page again, and *Start
over* clears it. The last step shows what the profile would make (the server makes it in memory), lets you download the profile as a file, and
regenerates the data from it. That replaces the users, groups, service principals, and the history, so the button needs a second click. The data
keeps the profile, so the next regeneration, a reset, and a restart use it too. The groups and the demo apps are not in the wizard. They stay as
they are in the profile you started with. The text of a profile goes in the page as text only.

The page *API* of Graph is a reference with one page for each resource (users, groups, and so on). A call is closed
until you open it, and the open call is in the address, so a link points to it. It shows what dollycli answers for the
call (the options, the forms of `$filter` and `$search`), what Graph says about it (the page of Microsoft Learn,
the properties that `$filter` works on), and the difference between the two. The forms of `$filter` are read from the
table in `graph/api/query.go` that answers them, and a test sends the example of each form to a server.

## Write the documentation of the API page

The text of the *API* page is in markdown files, `graph/docs/*.md`, and not in code. They are built into the program.
One file is one page. To add a page, add a file: the name, the order, and the text of the page are in the file, and
the portal needs no change. A `## GET /v1.0/users` title adds text to that call. Its parts (`### Filtering`,
`### Search`) show next to the text that the code makes, with the label *For this call*, so a call can say what is
special about it (a filter that Graph has and dollycli refuses, for example). [graph/docs/README.md](../graph/docs/README.md) has the format.

A test fails for a title that is not a call, and for a call with no page. A call with no text is fine. Its
one-line summary is in the code, with the route, so the two cannot differ.

While you write, use a `dev` build with the folder: `dolly serve --docs graph/docs` (or `DOLLY_DOCS`). It reads the files
at each reload of the page, and a mistake in a file keeps the last good text. A release build
(`make release VERSION=1.2.3`) has no `--docs` flag: the flag exists only when `version` is `dev`.
