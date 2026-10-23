# Releasing

Use `Link Messenger <dev@linkmessenger.me>` for Git authors, committers and
taggers. Publish through the Link Messenger account or GitHub Actions. Review
source, history and built archives for personal names, paths and credentials.

1. Update both the package version and the runtime version; date the changelog.
2. Run the release checks:

```sh
go test -race ./...
go vet ./...
make fmt-check
```

3. Install the built package in a clean consumer environment and verify its API.
4. Commit the reviewed changes on a release branch and publish them to `main`.
   The release workflow reruns checks, creates an annotated version tag and a
   GitHub release. Existing tags are never moved.
5. Verify the version through the public Go module proxy and `go get`.
