# Stop testing your Graph automation in production: meet Dolly

For some time i've had a lot of work doing identity management stuff, and a huge part of that work has been management of Entra.
Entra is the identity management for Microsoft Azure and is the baseline for all identity for the entire Azure/M365 stack. at the heart of Entra is graph api. 

This is the underlying api for all Entra and partially m365 services (at least that's the idea, but full service parity for M365 services has yet to be achieved, i believe).

Because of this, it's a natural gateway for automation, as it's simple to set up, there is a connector for about every language that I use daily, and it's fast enough in most cases for daily usage.
But it does have some caveats; some endpoints support certain features that others don't, some processes your requests in a weird way, some even straight up deny your request if you don't have an extra header value (looking at you `$count` + `ConsistencyLevel: eventual`).

However when doing automation, the main thing that has annoyed me about graph is is the lack of a "dev area" or emulated tenant. Yes you can generate a canary tenant and provide your own data to test large changes, or you could order a demo from microsoft with some seeded services, but this is often tedious work as these are all "production" environments, meaning any features that is hidden behind licenses would would be hard to script against, resetting after testing changes would require a reset-script, wondering if you are breaking anything else in the process. 

in all cases you would need to have some sort of script to provide extended data to fit your automation case.

I grew tired and i didnt see any real alternatives out there..

## So i created a graph emulator, i call it Dolly

dolly cli (Dolly CLI) is made to be as close as a local graph api implementation can be, with the goal of this being able to be used to locally test any automation, and being able to help you create better solutions.

Truth be told, when i started the project I pored through OpenAPI + Odata spec, documentation from Microsoft docs and test calls against Microsoft own graph explorer instance with their test tenant and realised that the scope was WAY too big for myself to take on.
To gain any meaningful progress I realised that setting up an AI workflow with Claude would suit this project the best.
Claude handles the technical implementation, and i help guide it along, finding correct documentation and testing the outputs.

AI is so much better at interfacing between json, markdown and actually writing documentation spec as code than i am, but i want to remove as much guessing as i can, so I ended up making a "facts before implementation" design where we would:

- read what OpenAPI spec said
- read what Odata resource metadata said about resource and its relation to other resources
- read what Microsoft learn said about any caveats using this route
- test this against Microsoft Graph Explorer api with their test tenant where we are unsure about caveats
- write route tests before adding any code with baseline expected calls
- when baseline for route was created, create new Odata tests to make sure Odata was implemented correctly

All of this work goes back into trying to deliver as close as a 1:1 representation of graph api as dolly cli possibly can.
what good is an emulator if it doesn't emulate correctly?

### Quick start

if you have go installed you can easily install and run dolly:

```
go install github.com/withholm/dolly cli/cmd/dolly@latest
dolly serve --users 50
```

it will then try to spin up a server on `http://127.0.0.1:8650`. if port is already in use try to start it with --port argument.
The main address will show the site and any graph api calls will be on `/v1.0/..`

currently supports users, groups, organization, service principals, app registrations, app reg permission grants, places, $batch, delta query, resource subscription and /me, with $filter/$orderby/$search/$count checked. No SAML, SCIM or Azure data plane yet

### SSO compliant ish

You can make a local webapp that uses this for SSO.
Currently only OpenID is supported, and I want to add SAML as well but it's more advanced so it might take some time.

I have tested with `MSAL`, `@Azure/identity`, `OIDC-client-ts` and `openid-client` with varying results. these are documented in repo/docs. if you have other tests of other frameworks you have tested, please let me know the results.

### Api documentation

With all the discovery done i also document everything, so we get an easy to read list of all calls that can be done with details of all necessary odata executions you can do alongside and necessary headers needed for the calls to work.
![api-docs](image.png)

### Dataset generation

To be truly helpful, i've also designed in a "tenant profile generator" where you can go through a wizard to select what the shape of the company looks like, departments, countries in tenant, job-titles in departments and how people distribute across the organisation (e.g. 10% of the company is sales and in sales 30% are KAM). etc.

This will help you generate a company that is somewhat close to your desired target.

![data-generation](image_1.png)

### rate limit and cost analysis

Something not a lot of people know about Graph: every call actually costs something.

For the identity APIs (users, groups, apps etc.), each app gets a wallet of resource units, and it’s per tenant. How much is in that wallet depends on how big the tenant is, with the top tier at 500+ users.

Every call you make pays from that wallet, and they don’t all cost the same. A simple GET on a user is cheap, but start expanding relationships or pulling big lists and the price goes up fast. Whatever you spend gets held in a rolling window, and once that window passes you get it back, ready to spend again.

Spend more than you’ve got and Graph hits you with a 429 Too Many Requests and a Retry-After header telling you to chill for a bit.

Honestly, this is really hard to keep track of against a real tenant, since you can’t see any of it. So Dolly has an RU cost view where you can see what each call actually costs, and exactly where your automation would start getting throttled. And if you don’t care about any of this right now, you can just turn rate limiting off.

Dolly's RU cost view showing per-call cost and throttling threshold
![rate-limiting-ui](image_2.png)

### call log and "learn" permissions

Any service principal you create in dolly cli comes without permission, and permissions are not enforced. instead dolly reads every call you make and gives you a best guess as to what permission you would need.

If you have an already defined permission set, you could add it and dolly cli would tell you if you ask for too much, too little and give possible corrections.

This is great for testing and being able to deliver correct estimates of the needed permissions for your app.

The data from this is gathered from microsoft learn and microsoft's own [devx-content](https://github.com/microsoftgraph/microsoft-graph-devx-content), so it should be mostly correct.

However both the microsoft learn data and devx have some missing meta-data and weird formatting so we do have some extra steps to make sure that permissions don't overlap (e.g user read + user update is `user.readupdate.all` instead of `user.read.all` + `user.readupdate.all`). the overlap checker is a best guess, not a guarantee.

Of course, any errors can be reported and i will fix them as soon as i can.

### containerized

you can run this in a container as a stand alone service. but please remember, there isn't any password protection anywhere here, it's in no way shape or form designed as an identity solution. hell even the sign on pages just have a "select user to sign on as" without password...

### TLS

dolly cli runs on plain http by default, and most of the time that's fine. pwsh graph doesn't care (it's what [easy connect](#easy-connect) below uses). but MSAL, @azure/identity and the azure cli flat out refuse a plain http authority, so for those you start it with `dolly serve --tls`. it makes a small local CA in the data folder, signs its own cert with it, and you can trust that CA once.

i wasn't comfortable asking you to trust a CA that could sign for anything, so i locked it down. it can only sign for `localhost`, loopback (127.0.0.0/8 and ::1) and the hosts you give it with `--allow-host`, so if the key ever leaks nobody can use it to impersonate a real site. i checked that by signing certs for real domains with the CA key: go, openssl and curl all refuse them. i have not tried browsers (or macos/windows), and the spec doesn't force a client to enforce it, so check yours. don't give it a real domain as an allowed host, it will happily sign for it (dolly cli warns you). the limits are fixed when the CA is made, so adding a host later means a new CA you have to trust again.

the CA lives one year, the key stays in your data folder, and `dolly tls remove` deletes it and tells you how to untrust it. it's a local dev tool, so please don't put it on a network.

### Reset/Rollback

It supports full auditing history of all items with a button to roll back before a specific change happened. you can also regenerate your tenant within the same portal giving you easy way to go back to start.

![rollback](image_3.png)

### Easy connect

I also added easy connect options for powershell where you would only need to run `iwr 'http://127.0.0.1:8650/pwsh' | iex` to connect via powershell graph.

Of course you could also manually add the connection profile to your local graph, grab a premade token (honestly any string works, but premade jwt gives you an actual user), and then connect using token.

If you have tls enabled the actual connect-mggraph with login should work

### try it out

to try it out you can go to [the github page](https://github.com/WithHolm/dolly cli). if you are missing endpoints, or there are some odata issues you don't see in the official graph client or suggestions for better QOL, please don't be afraid to open an issue. all feedback is welcomed.


### what im currently working towards

Currently im focusing on implementing subscription api. 

### why the name dolly?

Dolly is the cloned sheep (it's supposed to be a clone of a tenant), the cart that carries heavy equipment around safely, and a nod to “dollhouse”: a tiny replica you can rearrange and reset. You might also find someone in every generated tenant who should be used to living in a doll’s house.

dolly cli is not affiliated with or endorsed by Microsoft. Entra, Azure and Microsoft Graph are trademarks of Microsoft Corporation. It's just a product i really needed, and think you might do too :)

### licences and credits

dollycli is licensed under Apache-2.0. the made-up tenants (fjordvik, acme and their people) are my own. 

it uses data from microsoft, under their own licences:

- [microsoft learn docs](https://github.com/microsoftgraph/microsoft-graph-docs-contrib) (CC BY 4.0): the permissions each call
  needs, the throttling and cost tables, and the list of microsoft client ids. i have changed and reshaped this data.
- [microsoft-graph-devx-content](https://github.com/microsoftgraph/microsoft-graph-devx-content) (MIT): the list of graph
  permissions.
- [msgraph-metadata](https://github.com/microsoftgraph/msgraph-metadata) (MIT): the openapi description of graph that i check
  routes + metadat against.
