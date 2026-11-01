# Run in a container

    docker build -t dolly .
    docker run --rm -p 127.0.0.1:8650:8650 -v dolly-data:/data dolly

Or `docker compose up -d` (see [compose.yaml](../compose.yaml)). The image is one static program with the portal built in, and it runs as a
user that is not root. The data is in the volume `/data`.

- **Listen address.** dollycli listens on `127.0.0.1` as it comes. In the image it is `0.0.0.0` (`--host`, or `DOLLY_HOST`): inside a container,
  loopback cannot be reached from outside. **Publish the port to `127.0.0.1` only**, as above. dollycli has no sign-in of its own:
  anyone who can reach the port can read and change the data, and dollycli says so in the log when it listens on more than loopback.
- **Other containers** reach it by the name of the service, for example `http://dolly:8650/v1.0/users`. The Graph and sign-in APIs take any
  name. The admin API (and so the portal) takes `localhost` and `127.0.0.1` only, unless you allow a name: `--allow-host dolly`, or
  `DOLLY_ALLOW_HOSTS=dolly`. The addresses in the sign-in documents follow the name that the client used.
- **Stop and start.** `docker stop` is a clean stop (SIGTERM closes the data). A container that was killed leaves a lock in the volume,
  and the next one takes it over. In a container dollycli is always process 1, so it must know that a lock with its own process ID is old.
- **TLS.** For a client that needs `https`, set `DOLLY_TLS=1` and put the name of the service in the certificate (`DOLLY_ALLOW_HOSTS=dolly`). How to trust the CA is in [TLS](tls.md).
- **Health check.** `dolly health` asks the running dollycli for its list of APIs. The image runs it, as it has no shell and no curl.
- **Settings** are the same as the flags, as environment variables: `DOLLY_USERS`, `DOLLY_SEED`, `DOLLY_PAGE_SIZE`, `DOLLY_THROTTLE=1`,
  `DOLLY_PORT`, `DOLLY_HOST`, `DOLLY_ALLOW_HOSTS`, `DOLLY_USAGE_HISTORY` (for example `6h`). The first start makes the seed, so `DOLLY_USERS` counts then.
