<table border="0" cellspacing="0" cellpadding="0">
  <tr>
    <td><img src="https://github.com/LerianStudio.png" width="72" alt="Lerian" /></td>
    <td><h1>Lerian CLI</h1></td>
  </tr>
</table>

---

## Description

<!-- Summarize what this PR changes and why. Mention the command group(s)
     affected (auth, midaz/ledger, kubectl, config, output). -->

## Type of Change

- [ ] `feat`: New feature or capability
- [ ] `fix`: Bug fix
- [ ] `perf`: Performance improvement
- [ ] `refactor`: Internal restructuring with no behavior change
- [ ] `docs`: Documentation only (README, docs/, inline comments)
- [ ] `style`: Formatting, whitespace, naming (no logic change)
- [ ] `test`: Adding or updating tests
- [ ] `ci`: CI pipeline or workflow changes
- [ ] `build`: Build system, GoReleaser, Go module dependencies
- [ ] `chore`: Maintenance, config, tooling
- [ ] `revert`: Reverts a previous commit
- [ ] `BREAKING CHANGE`: Consumers must update their integration

## Breaking Changes

<!-- If applicable, describe exactly what breaks (command names, flags, output
     format, config file schema) and how users should migrate. Remove this
     section if not applicable. -->

None.

## Testing

- [ ] `make test` passes
- [ ] `make test-integration` passes if integration paths are exercised
- [ ] `make lint` passes
- [ ] `make build` produces a working binary
- [ ] `goreleaser check` passes if `.goreleaser.yml` changed

**Test evidence / Actions run:** <!-- Optional: link to a CI run or terminal output -->

## Architectural Checklist

- [ ] Commands stay thin — parsing and flag wiring in `cmd/`, behavior in `internal/`
- [ ] No `panic()` in command paths — errors returned and wrapped with `%w`
- [ ] Human output goes through `internal/output`, never bare `fmt.Println`
- [ ] Credentials and tokens never logged or echoed
- [ ] New flags documented in the command help text and in `docs/`

## Related Issues

Closes #
