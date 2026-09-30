# Contributing

## Development setup

1. Copy `.env.example` to `.env` and use local, unique values.
2. Never disable authentication on a public interface.
3. Keep music, databases, caches, logs and generated binaries outside Git.

Run the relevant checks before submitting a pull request:

```bash
make test-python
make test-go
make sim
make bench
```

For Go changes, run `gofmt`. For Flutter changes, run `dart format`,
`flutter analyze` and `flutter test`.

## Architecture and roadmap

- [docs/ROADMAP.md](docs/ROADMAP.md) is the authoritative implementation plan.
  Do not restore old numbered “ТЗ 3.0” milestones in comments or docs.
- Python migrations are the only owner of SQLite schema changes. Add a numbered
  migration, test fresh and legacy databases, and update the exact supported
  schema version in Go.
- Go production startup must remain read-only with respect to schema.
- Do not document roadmap features as shipped until their API and tests exist.
- Recommendation changes must keep `make sim` deterministic and update the
  2k/50k benchmark when their performance characteristics change.

## Pull requests

- Keep changes focused and explain user-visible behavior.
- Add tests for bug fixes and API contract changes.
- Update `.env.example` and documentation for new configuration.
- Do not commit generated APKs, model caches or personal library examples.
- Use synthetic metadata in fixtures and screenshots.

## Secret check

Before staging:

```bash
git status --short --ignored
gitleaks detect --no-git --source .
```

If a secret is accidentally staged or committed, stop and rotate it before
continuing. Rewriting history does not revoke an exposed credential.
