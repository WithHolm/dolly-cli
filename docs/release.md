# Releases

A release is the program for Linux, macOS, and Windows (amd64 and arm64), on the [releases page](https://github.com/WithHolm/Dolly-cli/releases) of GitHub. [GoReleaser](https://goreleaser.com) makes it from [.goreleaser.yaml](../.goreleaser.yaml). Today GitHub is the only place. The stores come later (see the end of this page).

## Make a release

1. Check that the portal is current. The program has `ui/dist` built in, and a release build does not run Node. After a change in `ui/src`, run `make ui` and commit `ui/dist`.
2. Run `go test ./...`.
3. Tag the commit and push the tag. [scripts/tag.sh](../scripts/tag.sh) asks for major, minor, or patch, tags the latest commit (`HEAD`, pushed or not) with the next version, and prints the push command:

```
make tag
git push origin v1.2.3
```

   By hand, it is `git tag v1.2.3` and the same push.

The tag starts the workflow [release.yml](../.github/workflows/release.yml). It runs the Go tests, builds every archive, and makes the release with the checksums and the changelog. A tag such as `v1.2.3-rc1` makes a pre-release.

## What is in a release

| File | What it is |
|---|---|
| `dolly_1.2.3_linux_amd64.tar.gz`, `..._linux_arm64.tar.gz` | Linux |
| `dolly_1.2.3_darwin_amd64.tar.gz`, `..._darwin_arm64.tar.gz` | macOS (Intel, and Apple silicon) |
| `dolly_1.2.3_windows_amd64.zip`, `..._windows_arm64.zip` | Windows |
| `checksums.txt` | SHA-256 of every archive |

Each archive has `dolly` (or `dolly.exe`), `LICENSE`, `NOTICE`, and `README.md`. The program is one static file (`CGO_ENABLED=0`) with the portal built in, and `dolly --version` says the version of the tag. A build from source says `dev`.

## Test it before a tag

Install [GoReleaser](https://goreleaser.com/install/) and run, in the root of the repository:

```
goreleaser check
goreleaser release --snapshot --clean
```

The first checks the file. The second builds every archive into `dist/` and makes no release. Unpack one and run `dolly --version`.

## What was checked

The file is new. These parts were run: the build with the same flags as the file (`-trimpath`, `-s -w`, and `-X main.version=…`) prints the version that it was given, and the program builds for all six systems of the file (and for FreeBSD and 32-bit ARM, which the file does not ship). `goreleaser check` and a snapshot were **not** run: GoReleaser was not installed where the file was written. Run them once before the first tag.

## The stores, later

GoReleaser can publish to several stores from the same file. Each needs a repository or an account that this project does not have yet:

- **Scoop**: a `scoops` section, and a repository for the bucket.
- **WinGet**: a `winget` section. It makes a pull request to `microsoft/winget-pkgs` from a fork of it. A WinGet package needs a stable download address and the checksum of each file, which the release already has.
- **Homebrew**: a `homebrew_casks` section, and a tap repository.
- **Docker image**: the [Dockerfile](../Dockerfile) builds the image today (see [Run in a container](container.md)). GoReleaser can build and push it, to the GitHub container registry, for example.
- **Flatpak**: GoReleaser does not make it. A Flatpak is made with its own manifest, and it is a poor fit for a command line program that runs a server.

Check the section names against the GoReleaser documentation when you add one: they change between versions.
