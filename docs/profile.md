# Profiles

A profile is a JSON file that says what the generated tenant looks like. Without one, dollycli makes the default tenant: Acme, with Nordic offices. With one, it can make a Norwegian energy company, a small clinic, or your own company with made-up people.

The same profile and the same `--seed` make the same data every time.

## Use a profile

```sh
dolly profile default > mine.json      # the default profile, to change
dolly profile check mine.json          # check it, and see what it makes
dolly serve --profile mine.json        # make the seed from it
dolly graph generate --force --profile mine.json
```

`--profile` (or `DOLLY_PROFILE`) counts only when dollycli makes a seed. A data folder that has a `seed.json` keeps its data, and dollycli says so. Use `generate --force` to replace it.

A running dollycli can take a profile too. The page *Generator* of the portal does it with a wizard. The admin API has:

| Call | What it does |
|---|---|
| `GET /_dolly/profile/default` | The default profile. |
| `GET /_dolly/profile/current` | The profile that the data was made from, or the default one if the data has none. |
| `POST /_dolly/profile/preview` | Body `{"profile": {...}, "users": 200, "seed": 1}`. Makes the data in memory and answers with counts, languages, and six sample people. Changes nothing. |
| `POST /_dolly/generate` | The same body. Replaces the running data: a new seed, no history. A delta token from before gets 410 Gone. A rollback cannot bring the old data back. Leave `profile` out to make the data again from the profile that it has. |

## The data keeps its profile

When a profile is given, `seed.json` keeps it (the field `profile`). A restart, a reset, and a new generation with no profile use it. Data from the
default profile has no such field, so its seed is the same as before profiles. Data from before profiles has none either, and `GET /_dolly/profile/current`
then gives the default profile.

## The Norway example

[profiles/norway.json](profiles/norway.json) is a made-up Norwegian company, Fjordvik, with six offices, Norwegian names, titles, and groups, `+47` phone numbers, and admin accounts named `adm.<first name>`. Use it as a start.

## The format

| Part | What it sets |
|---|---|
| `organization` | Names, domains, address, phone, and language of the tenant. `initialDomain` is the `.onmicrosoft.com` domain. `domain` is the default verified domain, and the owner and the admin account use it. `domains` lists other verified domains with a `weight`, and `domainWeight` is the weight of the default one. With more than one domain, the addresses of the members follow the weights. |
| `offices` | At least 2. Each has a name, address, `callingCode`, and a `weight`. The first two become the buildings. |
| `departments` | Each has a `weight` and its own job `titles` with weights. |
| `names` | The lists `first` and `last`. Accents stay in display names. Addresses use plain letters (`Håkon` gives `hakon`). With `byLanguage` (a list of `first` and `last` for each code in `people.languages`), the language of a person is picked first, by the weights of the languages, and the names come from the list of that language. With Norwegian 85 and English 15, about 85% of the people have a Norwegian name. A language with no list uses `first` and `last`, and these two are needed only then. |
| `people` | Percents of guests, synced users, enabled users, and contractors. The word for a manager. The weighted `languages`. The weighted `guestDomains`, where the mail of a guest is. (`organization.guestDomain` is the older way to name one.) |
| `formats` | Patterns for the employee ID, mobile and business phone, and cost center. A list of streets. |
| `extensionAttributes` | `extensionAttribute1` to `15`. Each has `values` with weights, or a `pattern`. `fill` is the percent of users that have it. |
| `groups` | The name `prefix`, department and office groups, one dynamic group, Microsoft 365 groups, and the demo apps. |
| `owner` | The department and job title of Philip Meholm, the owner. The name stays. |
| `admin` | How the admin account of the owner is named. See below. |

A weight is a ratio inside its list: 85 and 15 is the same as 17 and 3. The portal keeps every list at a sum of 1000 and shows a weight as a percent with one decimal (343 is 34.3%). A file with other weights is scaled to 1000 when the portal opens it, with the same ratios.

Every unknown field is an error, and the error names the field. A file may be 1 MB. Lists may have 2000 items, text 200 characters, weights 1 to 1000, percents 0 to 100.

## Patterns

A pattern makes a value from a few characters. There is no code and no regex.

| Write | You get |
|---|---|
| `#` | A random digit. A run like `#####` is a number with that many digits, with leading zeros. A run has at most 9. |
| `{1000-1099}` | A number in the range. `{01-30}` is padded to the width of the first number. |
| `{cc}` | The calling code of the office (phone patterns). |
| Anything else | Stays as written. A pattern has at most 100 characters. |

Examples: `+{cc} 9## ## ###` gives `+47 912 34 567`. `K-{1000-1099}` gives `K-1042`. The employee ID pattern has exactly one run of `#` and a `start`, so the IDs count up.

## The admin account

An organization names its admin accounts in its own way. `admin.userPrincipalName` is the part before the `@`, made from the first and last name of the owner (Philip Meholm):

| Token | Gives |
|---|---|
| `{first}`, `{last}` | `philip`, `meholm` (lowercase, no accents) |
| `{f}`, `{l}` | `p`, `m` |
| `{First}`, `{Last}` | `Philip`, `Meholm` (as written, for the display name) |

| Pattern | Account |
|---|---|
| `a.{first}` (default) | `a.philip@acme.example` |
| `adm.{first}` | `adm.philip@acme.example` |
| `{first}.a.{last}` | `philip.a.meholm@acme.example` |
| `{first}.{last}.adm` | `philip.meholm.adm@acme.example` |

The pattern must have `{first}` or `{last}`, and no spaces. The quick connect on the sign-in page and `/pwsh` use this account.

## Checked and not checked

- The default profile makes the same bytes as dollycli made before profiles. A test holds the hashes for four sizes.
- The limits, the pattern language, the admin names, the Norway world, preview, and replace (in a folder and in memory) have tests.
- Not checked against the real Graph: a profile is a dollycli feature. The data it makes follows the same shapes as before.
- The wizard in the portal (page *Generator*, see [the portal](portal.md)) was opened in a browser: Norway as the start, the steps, the preview, and making the data. A test holds the helpers of the editor.
- Not done yet: a tool that reads a profile from a real tenant.
