# Contributing

## Local Checks

Enable repository-local hooks once before regular work:

```sh
make hooks
```

Common local checks:

```sh
make fmt
make test
make ci
```

`make ci` runs:

- `make lint`
- `make test`
- `make deps`

## Commit And Push

- `pre-commit` checks staged Go files for `gofmt` drift and runs `make lint`
- `pre-push` runs `make ci`

## Release Flow

1. Update `CHANGELOG.md`
2. Run `./scripts/release.sh vX.Y.Z`
3. After the tag is pushed, the GitHub release workflow builds release archives and checksums

## Test Fixtures

Keep repository fixtures intentionally small.

- prefer short clips that are just long enough to reproduce the behavior under test
- avoid adding large ad hoc recordings directly to the repository
- as a working guideline, keep ordinary audio fixtures under roughly `100 KB` when possible
- if a larger fixture is unavoidable, document why it is needed in the test or PR description
- remove `.DS_Store` and other local filesystem noise before committing

Use opt-in integration tests for provider behavior that does not require a large checked-in fixture.
