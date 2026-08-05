# Contributing to xdm

Thank you for considering a contribution to xdm.

xdm is a focused terminal Direct Message client. Contributions should preserve
its keyboard-first interface, modular Go architecture, and explicit separation
between API, authentication, cache, service, and TUI responsibilities.

## Before starting

For bug fixes, describe the problem and its expected behavior in an issue.

For new features or architectural changes, open an issue before implementation
so the scope and product direction can be discussed. Avoid combining unrelated
features, fixes, refactors, tests, CI, and documentation in one pull request.

Security vulnerabilities must not be reported in public issues. Follow
[SECURITY.md](SECURITY.md) instead.

## Development setup

Requirements:

- Go 1.26.5 or newer
- Git
- An X Developer account only when testing the official backend

Clone the repository:

```sh
git clone https://github.com/willzys/xdm.git
cd xdm
go mod download
```

The demo backend can be used without credentials or network access:

```sh
go run . demo
```

## Branches and commits

Create each branch from the latest `main`:

```sh
git switch main
git pull --ff-only
git switch -c type/short-kebab-description
```

Use lowercase Conventional Commit types with a required scope:

```text
feat(scope): add a capability
fix(scope): correct a defect
docs(scope): update documentation
refactor(scope): reorganize internals
test(scope): cover behavior
perf(scope): improve performance
build(scope): change dependencies or build tooling
ci(scope): change automation
chore(scope): perform repository maintenance
```

Keep commit subjects concise, imperative, and free of a trailing period.

Do not include a pull request number in a branch name, local commit, or pull
request title. GitHub adds the number to the final squash commit.

Do not add automated co-author trailers.

## Code guidelines

- Read affected code before changing it.
- Make the smallest complete change that solves the problem.
- Preserve existing package boundaries and code style.
- Prefer the standard library and existing project utilities.
- Avoid new dependencies unless they provide clear value.
- Keep network and disk operations outside Bubble Tea rendering.
- Write code comments and project documentation in English.
- Never commit OAuth tokens, cookies, credentials, message databases, or other
  user data.
- Do not commit build outputs or local test artifacts.

Run `gofmt` on every changed Go file.

## Validation

Run the checks relevant to your change:

```sh
go test ./...
go vet ./...
go build ./...
git diff --check
```

For Go changes, also verify formatting:

```sh
gofmt -d <changed-go-files>
```

Do not claim a check passed unless it was executed successfully.

## Pull requests

Open pull requests against `main`.

Use the same scoped Conventional Commit format for the pull request title:

```text
type(scope): concise imperative description
```

Keep pull requests in draft state until the change and its validation are ready
for review.

A pull request description should explain:

- the outcome of the change;
- the concrete implementation changes;
- user or developer impact;
- the exact validation commands that passed;
- known limitations or intentionally omitted work.

Keep each pull request focused and reviewable. If another pull request must
merge first, state the dependency and rebase plan explicitly.

The repository uses squash merging. The expected final commit subject is:

```text
<pull request title> (#PULL_REQUEST_NUMBER)
```

## Reporting bugs

A useful bug report includes:

- the observed behavior;
- the expected behavior;
- reproducible steps;
- operating system and terminal;
- Go or xdm version;
- backend in use;
- sanitized error output.

Remove OAuth tokens, cookies, Direct Message content, account identifiers, and
other private information before posting logs or screenshots.

## License

By contributing, you agree that your contributions will be licensed under the
project's [MIT License](LICENSE).
