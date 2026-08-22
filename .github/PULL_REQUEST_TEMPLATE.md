# Pull Request

Thanks for contributing to Spotter!

## Summary

<!-- One or two sentences: what does this PR do, and why? -->

## Linked issues

<!-- List any issues this PR fixes or makes progress on. -->

- Closes #
- Related: #

## Type of change

<!-- Check exactly one. The maintainer may re-categorise. -->

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation / build / tooling (no production code change)

If breaking, describe the migration path:
<!-- e.g. "Bumps /api/v1/info to v2; old schema still served at /api/v1/info until 1.0" -->

## Affected component

<!-- Pick all that apply so reviewers route correctly. -->

- [ ] spotterd agent (`cmd/agent/`, `internal/agentd/`)
- [ ] spotter-client (`main.go`, `frontend/`)
- [ ] Wire protocol (`internal/protocol/`)
- [ ] Collectors (`internal/collector/`)
- [ ] Scanner / discovery (`internal/scanner/`, `internal/registry/`)
- [ ] Scripts / build (`Makefile`, `scripts/`)
- [ ] Docs (`docs/`, `README.md`, `README.en.md`)
- [ ] CI / GitHub workflow

## How I tested

<!-- Check what you ran. CI runs `make test` and the lint workflow; expand if you did more. -->

- [ ] `make test` passes locally with `-race`
- [ ] `make agent-linux-arm64` builds clean
- [ ] `make agent-linux-x64` builds clean
- [ ] `make client` builds clean on the active GOOS
- [ ] Manual end-to-end test against a real device (please describe)
- [ ] Frontend `npm run build` produces no TS errors
- [ ] `golangci-lint run ./...` is clean

If you tested manually, what device + what commands?

```
<!-- paste -->
```

## Checklist

- [ ] I added or updated `CHANGELOG.md` under "Unreleased" if the change is user-facing.
- [ ] I added or updated unit tests for any changed behavior.
- [ ] I re-read the relevant files (`cmd/agent/main.go`, `internal/...`) to make sure my change is consistent with the rest of the package.
- [ ] I confirmed that no unrelated files were modified.
- [ ] I followed the [Committing guide](CONTRIBUTING.md#commit-messages).

## Screenshots / recordings

<!-- Optional. Especially useful for frontend work. -->
