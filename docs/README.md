# dollycli documentation

Start with the [README](../README.md) for what dollycli is and how to run it. These pages have the details.

| Page | What is in it |
|---|---|
| [dollycli Graph](graph.md) | The server and the package, permissions, the fake data, and where the data of Microsoft comes from. |
| [Profiles](profile.md) | Make the tenant look like yours: organization, offices, departments, names, formats, and the admin account name. |
| [Query rules](query-rules.md) | How `$filter`, `$orderby`, `$search`, and `$count` follow Graph. |
| [Delta query](delta.md) | `users/delta`, `groups/delta`, and `servicePrincipals/delta`: how they work, what a token is, and what was checked. |
| [Sign-in](sign-in.md) | The fake Entra: grants, the browser flow, sessions, sign-out, and PowerShell. |
| [Permissions of your tenant](permissions.md) | Add the permissions of your own tenant to the picker: download them with `az` or PowerShell, and start with them. |
| [TLS](tls.md) | Run with `--tls`: what dollycli makes, how to trust the CA, a container, your own certificate, and the errors. |
| [Sign-in libraries](libraries.md) | What MSAL, `@azure/identity`, `oidc-client-ts`, and `openid-client` need, and what was checked. |
| [Azure CLI](azure-cli.md) | What works with `az`, what does not, and what the setup needs. |
| [MCP server](mcp.md) | `dolly mcp`: let a coding tool look up a call of Graph and run a throwaway dollycli to test against. |
| [Run in a container](container.md) | Docker, compose, other containers, and settings. |
| [The portal](portal.md) | The pages of the portal, and how the text of the API page is written. |
| [Releases](release.md) | How a release is made (a tag), what is in it, how to test it, and the stores that come later. |
| [Layout and development](development.md) | The packages, how to add an API, and the make targets. |
| [Library checks](compat/README.md) | The scripts that check the libraries, and the [workflow](compat/WORKFLOW.md) to keep the guidance true. |

The data that dollycli reads has its own notes: [throttling cost](../graph/cost/README.md) and [permissions](../graph/permissions/README.md). The text of the API page of the portal is in [graph/docs](../graph/docs/README.md).
