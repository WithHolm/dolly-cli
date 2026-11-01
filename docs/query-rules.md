# Query rules

dollycli follows the rules of Graph for `$filter`, `$orderby`, `$search`, and `$count`, and it answers with the error that Graph gives.
For a directory object (users, groups, service principals), Graph decides by property and operator what works, and what needs an
*advanced query*: the header `ConsistencyLevel: eventual` and `$count=true`. The rules are Microsoft's own tables
(*Advanced query capabilities on Microsoft Entra ID objects*). `make update-query` downloads them into `graph/query/raw/` and
reads them into `graph/query/matrix.json`. A test fails if the two differ, and the parser fails on a table that has changed its form.

- `$filter` is OData: `eq ne gt ge lt le in`, `and or not`, `startsWith endsWith`, `any` and `all`, `/$count`, `null`, dates, GUIDs.
- `ne` needs an advanced query (the header and `$count=true`). `not` and `endsWith` need only the header. Some properties
  need an advanced query too. Without it the answer is `Request_UnsupportedQuery`, with the text of Graph.
- `$search` needs the header. A clause is `"property:text"`, joined with `AND`, `OR`, and parentheses.
- `/$count` without the header is `Request_UnsupportedQuery`. `?$count=true` without it is left out, with no error.
- `$orderby` works on the properties in the table of sorting. With `$filter` it always needs an advanced query.
- `$top` must be a whole number from 1 to 999 on a directory object.
- `$expand` works on a user for `manager` and `directReports`.
- An answer has the headers of Graph (`request-id`, `client-request-id`, `OData-Version`, the OData `Content-Type`), `@odata.context`,
  and the default properties with `null` for a property with no value. An error has an `innerError`. `$select` returns only
  what it names (no `id` unless you ask for it).

**Where the texts come from.** Microsoft's tables say which operators work on which property, and the real Graph answers
the rest. The texts of the errors and the shape of the answers were checked against the real Graph (the sample tenant
of Graph Explorer). The tables are not complete: Graph answers some filters on properties that they do not list, so dollycli
allows a property that Graph declares and the tables do not list.
