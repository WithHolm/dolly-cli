.PHONY: build release test ui ui-dev dev graph-spec update-cost update-query update-permissions

build:
	@go build -o bin/dolly ./cmd/dolly

# A release build: make release VERSION=1.2.3. It has no --docs flag.
release:
	@go build -ldflags "-X main.version=$(VERSION)" -o bin/dolly ./cmd/dolly

serve: ui
	@go run ./cmd/dolly serve

generate:
	@go run ./cmd/dolly graph generate --force

test:
	@go test ./...

# Build the portal (the Svelte app in ui/src) into ui/dist, which the Go program embeds. It checks the
# types and runs the tests of the logic first. ui/dist is in the repo, so Node is needed only after a
# change in ui/src. Commit the new ui/dist with the change.
ui:
	@cd ui && npm ci && npm run check && npm test && npm run build

# The portal with live reload on http://localhost:5173. It needs 'dolly serve' on port 8650.
ui-dev:
	@cd ui && npm run dev

# Dolly on http://127.0.0.1:8651 with data in memory. It builds the portal and the program again at every change
# (see .air.toml). It is for development, and for the checks that need a running server.
dev:
	@air

# Make the extract of Microsoft's OpenAPI description of Graph that Dolly is
# checked against (graph/spec). It downloads 44 MB. Run it when you want newer data,
# and commit the extract. The description is MIT licensed.
graph-spec:
	@curl -sfL -o /tmp/graph-openapi-v1.0.yaml https://raw.githubusercontent.com/microsoftgraph/msgraph-metadata/master/openapi/v1.0/openapi.yaml
	@go run ./graph/spec/gen -in /tmp/graph-openapi-v1.0.yaml -out graph/spec/graph-v1.0.json

# Download Microsoft's documents about Graph throttling (the cost of each call, and the
# limits) into graph/cost/raw. It changes nothing else. Then ask Claude to make
# graph/cost/cost.json from them: the steps are in graph/cost/README.md.
update-cost:
	@go run ./graph/cost/fetch -out graph/cost/raw
	@echo "Next: ask Claude to make graph/cost/cost.json (see graph/cost/README.md), then run: go test ./graph/cost"

# Download the list of the permissions of Microsoft Graph (names, IDs, descriptions) into
# graph/permissions/raw. Dolly reads that file as it is: nothing else is to be made.
update-query:
	@go run ./graph/query/fetch -out graph/query/raw -matrix graph/query/matrix.json
	@go test ./graph/query

update-permissions:
	@go run ./graph/permissions/fetch -out graph/permissions/raw
	@go test ./graph/permissions
