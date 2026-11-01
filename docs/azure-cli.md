# Azure CLI

The Azure CLI works in part. `az` can sign in to dollycli as a service principal and call its Graph, but only with a setup that needs a stand-in for the Azure Resource Manager (ARM). Some commands do not work yet.

All checks were made on 2026-10-30 with Azure CLI 2.87.0, against a dollycli that ran with `--tls` (see [sign-in libraries](libraries.md) for https). They used a separate config folder (`AZURE_CONFIG_DIR`), so the normal `~/.azure` was not touched. Anything not listed here was not tested.

## What works

| Command | Result |
|---|---|
| `az login --service-principal -u <client ID> -p <any secret> --tenant <tenant ID> --allow-no-subscriptions` | Works. |
| `az account get-access-token --resource https://graph.microsoft.com/` | Works. It gives a signed token for Graph. |
| `az rest --method get --url https://127.0.0.1:8650/v1.0/users --resource https://graph.microsoft.com/` | Works. The answer is the answer of dollycli Graph. |

## What does not work yet

| Command | What happens |
|---|---|
| `az ad user list`, `az ad sp list`, and the other `az ad` commands | `az` uses the Graph audience as the base address of its calls. With `https://graph.microsoft.com/`, `az ad` calls the real Graph, and the token of dollycli is no good there. With the address of dollycli, dollycli answers `AADSTS500011`, because it knows Graph only under the names of Microsoft. |
| `az login -u <user> -p <password>` | It stops. MSAL Python asks for `https://127.0.0.1/common/userrealm/<user>` and leaves out the port, so it tries port 443. Not tried: dollycli on port 443. |
| `az login` in the browser | Not tested. |
| Any command for ARM (`az group`, `az vm`, and so on) | dollycli has no Azure Resource Manager. |

## What the setup needs

1. **https.** `az` does not send a Bearer token to an `http` address. Start dollycli with `--tls`, and give `az` the CA: `REQUESTS_CA_BUNDLE=dolly-data/tls/ca.pem`.
2. **No instance discovery.** Run `az config set core.instance_discovery=false`. Without it `az` asks `login.microsoftonline.com` about the host.
3. **A registered cloud.** `az cloud register` always fetches `<resource manager>/metadata/endpoints?api-version=2022-09-01`. dollycli has no such path, so the command fails with a 404 (also with `--cloud-config`). During the check a small separate server answered it, over https (also `/subscriptions?api-version=2022-12-01`, which `az login` asks for; the answer was an empty list). The document had `name`, `portal`, `authentication` (`loginEndpoint`, `audiences`, `tenant`, `identityProvider`), `graphAudience`, `graph`, `microsoftGraphResourceId`, `resourceManager`, `batch`, `gallery`, `sqlManagement`, and `suffixes`. The check did not show which of these `az` needs.
4. **An audience that dollycli knows.** `az login` asks for a token for the audience `activeDirectoryResourceId` (the ARM audience, `https://management.core.windows.net/` as it comes). dollycli answers `AADSTS500011`: it has no such service principal. During the check this audience was set to `https://graph.microsoft.com/`.

The cloud was registered like this (the resource manager is the stand-in server):

    az cloud register -n dollycli \
      --endpoint-resource-manager https://127.0.0.1:8653 \
      --endpoint-active-directory https://127.0.0.1:8650 \
      --endpoint-active-directory-resource-id https://graph.microsoft.com/ \
      --endpoint-active-directory-graph-resource-id https://graph.windows.net/ \
      --endpoint-microsoft-graph-resource-id https://graph.microsoft.com/
    az cloud set -n dollycli
    az config set core.instance_discovery=false

## What would make it work

These are not done:

- dollycli answers the ARM metadata, `/subscriptions`, and `/tenants` over https, and has a service principal for the ARM audience. Then `az cloud register` and `az login` need no stand-in and no borrowed audience.
- dollycli takes its own address as a name for the Graph audience. Then `az ad` commands call dollycli, and not the real Graph.
- A check of `az` in `make compat` (see [library checks](compat/README.md)), so that this page stays true.
