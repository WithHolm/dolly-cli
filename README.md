# dollycli
dollycli spins up a fake tenant on localhost. It speaks the OAuth/OIDC of Entra ID and the endpoints of Microsoft Graph, with a stateful directory behind them. Add a user to a group, and memberOf, members and token claims all update.

Make a tenant (`dolly serve --users 50`), test against it, then go back to the seed with `dolly graph reset`. Entra first, Azure data plane next.

Warning: dolly-cli is a development tool. It issues tokens no real service will trust, binds to localhost by default, and must never be used as an identity provider.

dollycli is not affiliated with or endorsed by Microsoft. Entra, Azure and Microsoft Graph are trademarks of Microsoft Corporation. Its just a product i really needed, and might think you do too :)

## Quick start

    go install github.com/withholm/dolly-cli/cmd/dolly@latest
    dolly serve --users 50

- The Graph address is `http://127.0.0.1:8650/v1.0`. Any token works.
- The portal is at `http://127.0.0.1:8650/`. It shows every call, the data, the sign-ins, and a reference of the API.
- A fake Entra sign-in runs on the same port.

Try it:

    curl -s 'http://127.0.0.1:8650/v1.0/users?$top=2' -H 'Authorization: Bearer any'

## Use it with your tools

- **PowerShell.** `iwr 'http://127.0.0.1:8650/pwsh' | iex` connects Microsoft Graph PowerShell as an admin user.
- **A web app.** MSAL, `@azure/identity`, `oidc-client-ts`, and `openid-client` work. See [sign-in libraries](docs/libraries.md).
- **Azure CLI.** It works in part. See [Azure CLI](docs/azure-cli.md).
- **Go tests.** Import `github.com/withholm/dolly-cli/graph`, and call `graph.New(graph.NewMemory(graph.Options{Users: 50, Seed: 1}), graph.ServerOptions{})`.
- **A container.** Run `docker compose up -d`. See [run in a container](docs/container.md).

## More

The [documentation](docs/README.md) has the rest: Graph and its data, query rules, sign-in, the portal, and development.

## Licence and credits

dollycli is licensed under Apache-2.0 ([LICENSE](LICENSE)). The made-up tenants are original. It uses data from Microsoft, changed and reshaped, under their licences:

- [Microsoft Learn docs](https://github.com/microsoftgraph/microsoft-graph-docs-contrib) (CC BY 4.0): which permissions each call needs, the throttling and cost tables, which properties support which query options, and the Microsoft client IDs. See [permissions of the calls](graph/permissions/operations/README.md) and [cost data](graph/cost/README.md).
- [microsoft-graph-devx-content](https://github.com/microsoftgraph/microsoft-graph-devx-content) (MIT): the list of Graph permissions. See [permissions](graph/permissions/README.md).
- [msgraph-metadata](https://github.com/microsoftgraph/msgraph-metadata) (MIT): the OpenAPI and OData descriptions of Graph. See [Graph descriptions](graph/spec/README.md).

The full notices, with the licence texts, the commits that the data is pinned to, and the files that use it, are in [NOTICE](NOTICE).
