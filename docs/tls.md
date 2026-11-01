# Run dollycli with TLS

Some clients refuse a plain `http` address: MSAL and `@azure/identity` stop at the authority, and the Azure CLI does not send a token to it. Browsers keep the cookie of the [sign-in page](sign-in.md) in a hidden frame only over `https`. For these, run dollycli with TLS:

    dolly serve --tls

The portal, Graph, and the sign-in API are now at `https://127.0.0.1:8650`. (`DOLLY_TLS=1` does the same as `--tls`.)

The first call from a client fails until it trusts the certificate. This page shows how to do that once.

## What dollycli makes

dollycli makes its own small certificate authority (CA), and signs the certificate of the server with it. The files are in the data folder:

| File | What it is |
|---|---|
| `dolly-data/tls/ca.pem` | The public certificate of the CA. You give this file to the clients. |
| `dolly-data/tls/ca-key.pem` | The private key of the CA. Keep it secret (the file is `0600`, and the folder is `0700`). |

- The CA is good for 10 years, and it stays in the folder. A new server certificate is made at each start, good for 397 days. So you trust the CA **once**, and a restart changes nothing.
- The certificate has these names: `localhost`, `127.0.0.1`, `::1`, the address of `--host` (if it is one address), and every name in `--allow-host`. A client that calls another name gets an error about the name. Add the name with `--allow-host` and restart.
- With no data folder (`--dir=`) the CA is new at each start, so nothing can trust it. dollycli says so in the log. Keep the data folder, or use your own certificate (see the end of this page).

## Trust the CA

Pick the narrowest way that works. A tool that has a setting for it needs no change in the system.

| Client | How |
|---|---|
| curl | `curl --cacert dolly-data/tls/ca.pem https://127.0.0.1:8650/_dolly/apis` |
| Node.js, MSAL Node, `@azure/identity` | `NODE_EXTRA_CA_CERTS=dolly-data/tls/ca.pem node app.js` |
| Python (`requests`) and the Azure CLI | `REQUESTS_CA_BUNDLE=dolly-data/tls/ca.pem` (see [Azure CLI](azure-cli.md)) |
| A browser | Add `ca.pem` as an authority in the settings of the browser (in Firefox: *Privacy & Security*, *View Certificates*, *Authorities*, *Import*) or in the system. |
| The commands of dollycli | Nothing to do. The commands that talk to a running dollycli (`dolly health` for one) read `ca.pem` from `--dir` themselves. |

The checks of the first four rows were made with a dollycli that ran with `--tls`, see [sign-in libraries](libraries.md) and [Azure CLI](azure-cli.md). The checks of the browser and of the system store were not made for this page.

To trust the CA for the whole system, use the way of your system. These are the usual commands. They are not run for this page, so check them for your system:

- **Debian and Ubuntu:** `sudo cp ca.pem /usr/local/share/ca-certificates/dollycli-ca.crt && sudo update-ca-certificates`
- **Fedora, RHEL:** `sudo cp ca.pem /etc/pki/ca-trust/source/anchors/dollycli-ca.pem && sudo update-ca-trust`
- **macOS:** `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ca.pem`
- **Windows** (an administrator shell): `certutil -addstore -f Root ca.pem`

If a machine cannot read the data folder, it can fetch the CA from the server: `https://<address>:8650/dolly-ca.pem` (the file is public). Compare it with the file on the server before you trust it:

    openssl x509 -in ca.pem -noout -fingerprint -sha256

## A container

Start it with TLS, and put the name that other containers use in the certificate. The addresses `0.0.0.0` that a container listens on are not a name, so they do not go in the certificate.

    docker run --rm -p 127.0.0.1:8650:8650 -v dolly-data:/data \
      -e DOLLY_TLS=1 -e DOLLY_ALLOW_HOSTS=dolly dolly

The CA is in the volume: `/data/tls/ca.pem`. Get it with `docker cp <container>:/data/tls/ca.pem .`, or mount the volume (read only) into the container of the client and point `NODE_EXTRA_CA_CERTS` at it. The client then calls `https://dolly:8650/v1.0/...`. More about containers is in [Run in a container](container.md). The health check of the image (`dolly health`) works with TLS, and trusts the CA in `/data/tls` itself.

## Your own certificate

To use a certificate that you already trust, give both files (PEM):

    dolly serve --tls-cert cert.pem --tls-key key.pem

dollycli then makes no CA, and the address `/dolly-ca.pem` is not served. The certificate must have every name that clients use. A tool such as [mkcert](https://github.com/FiloSottile/mkcert) makes one that the system already trusts (this was not tried for this page):

    mkcert -install
    mkcert localhost 127.0.0.1 ::1

## Keep it safe

- Anyone who has `ca-key.pem` can make a certificate that every client which trusts the CA accepts, for any name. Do not share the data folder, do not commit it, and do not put the key in an image. `dolly-data` is for one machine.
- Trust the CA in one tool (`NODE_EXTRA_CA_CERTS` and the like) when you can, and not in the whole system.
- To stop trusting it, remove it from the trust store, and delete `dolly-data/tls`. The next start makes a new CA.

## When it does not work

| What you see | The cause and the fix |
|---|---|
| `unable to get local issuer certificate`, `SELF_SIGNED_CERT_IN_CHAIN`, `certificate signed by unknown authority`, or a warning page in a browser | The client does not trust the CA. Give it `ca.pem` as above. |
| `no alternative certificate subject name matches target hostname`, or an error that says the certificate is for another name | The client calls a name that is not in the certificate. Start dollycli with `--allow-host <name>`. |
| `Client sent an HTTP request to an HTTPS server.` | The client calls `http://` on a port that serves `https`. Use `https://`. |
| `authority_uri_insecure` (MSAL), or `The authorityHost address must use the 'https' protocol.` (`@azure/identity`) | The authority is `http`. Start dollycli with `--tls`, and use `https://` in the authority. |
| `--tls-cert and --tls-key go together` | Give both files, or neither. |

The libraries that need this have their own notes: [sign-in libraries](libraries.md) (MSAL, `@azure/identity`, `oidc-client-ts`, `openid-client`) and [Azure CLI](azure-cli.md).
