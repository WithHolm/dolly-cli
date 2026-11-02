# Graph descriptions

Two extracts of Microsoft's descriptions of the Graph API (v1.0), for the resources that dollycli serves. Both come from
[msgraph-metadata](https://github.com/microsoftgraph/msgraph-metadata), which is MIT licensed (the file `LICENSE` in that repo). Microsoft's files are changed: only a part is kept.

```
graph-v1.0.json       the OpenAPI description: every operation of the resources (method, path, summary, the page of Learn, the query options, the headers), and the
                      properties of the main schemas (type, nullable, read-only, whether Graph returns it by default). Tests check dollycli against it.
metadata-v1.0.xml     the OData description (CSDL): the types that the resources need, with their base types, properties, and navigation properties, and the entity sets.
                      dollycli serves it at /v1.0/$metadata.
spec.go               the loaders (Load, Metadata)
gen/                  the tool that makes both files from Microsoft's
```

## Update

```
make graph-spec
```

This downloads `openapi/v1.0/openapi.yaml` and `clean_v10_metadata/cleanMetadata.xml` (about 3.4 MB) from the master branch, and writes both files. It needs the network.

## What the OData description keeps

The types of `microsoft.graph` that dollycli has (the list `metadataRoots` in [gen/main.go](gen/main.go)), the types that these need (base types and the types of properties, as far as they go), the navigation
properties that point at a kept type, and the entity sets and the singleton `me` that dollycli has (`metadataSets`). It leaves out the other types, the actions and functions (dollycli serves few
of them, and a description that lists them would promise calls that it does not answer), and the annotations. The types are written in full (`microsoft.graph.user`), as the real Graph writes
them: Microsoft's file uses the alias `graph.user`.

To add a type or a set, add its name to the lists in `gen/main.go` and run `make graph-spec`. A test checks that every type that the file names is in it, and that its properties are properties of the
OpenAPI schema too.

## The descriptions are not the truth

They say what Microsoft means the API to be. The real Graph differs: it refuses some routes that they list (`GET /applications/{id}/homeRealmDiscoveryPolicies` is a 404), and the description leaves out
behavior (the exact errors, the limits, when a value is null). Check a route against the real Graph (see [Add a route to Graph](../../docs/development.md)).

## Credit

The descriptions are from <https://github.com/microsoftgraph/msgraph-metadata>, MIT license, Copyright (c) 2018 Microsoft Graph (the file `LICENSE` in that repo). Give credit to Microsoft when you use the data.
