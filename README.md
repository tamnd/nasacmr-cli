# nasacmr

A command line for the NASA Common Metadata Repository (CMR).

`nasacmr` is a single pure-Go binary. It searches and lists Earth science data
indexed by NASA — over 54,000 collections and millions of granules — over plain
HTTPS, shapes it into clean records, and prints output that pipes into the rest
of your tools. No API key required.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
nasacmr as `nasacmr://` URIs.

## Install

```bash
go install github.com/tamnd/nasacmr-cli/cmd/nasacmr@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/nasacmr-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/nasacmr:latest --help
```

## Usage

```bash
nasacmr collections --keyword=landsat              # search collections
nasacmr collections --provider=LAADS               # by data provider
nasacmr collections --keyword=climate -o json      # as JSON, ready for jq
nasacmr granules --short-name=HLSL30               # list granules
nasacmr granules --short-name=HLSL30 --temporal=2024-01-01T00:00:00Z,2024-01-31T23:59:59Z
nasacmr search --keyword=climate                   # full-text search
nasacmr --help                                     # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
nasacmr serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
nasacmr mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`nasacmr` registers a `nasacmr` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/nasacmr-cli/nasacmr"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `nasacmr://` URIs without knowing anything about nasacmr:

```bash
ant get nasacmr://collection/C2021957657-LPCLOUD   # fetch a collection record
ant cat nasacmr://collection/C2021957657-LPCLOUD   # just the abstract
ant url nasacmr://collection/C2021957657-LPCLOUD   # the live CMR URL
```

## Development

```
cmd/nasacmr/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the nasacmr domain
nasacmr/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/nasacmr
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
