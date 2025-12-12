# Contributing to Volt

First off, thanks for taking the time to contribute! 🎉

## Development Setup

```bash
# Clone the repository
git clone https://github.com/yourorg/volt.git
cd volt

# Install dependencies
go mod download

# Run tests
go test -v ./...

# Run linter
golangci-lint run
```

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages. This enables automatic changelog generation and semantic versioning.

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Description | Changelog Section |
|------|-------------|-------------------|
| `feat` | A new feature | 🚀 Features |
| `fix` | A bug fix | 🐛 Bug Fixes |
| `docs` | Documentation changes | 📚 Documentation |
| `test` | Adding or updating tests | 🧪 Tests |
| `refactor` | Code change that neither fixes a bug nor adds a feature | ♻️ Refactoring |
| `perf` | Performance improvements | ⚡ Performance |
| `chore` | Maintenance tasks | 🔧 Maintenance |
| `ci` | CI/CD changes | 👷 CI/CD |
| `build` | Build system changes | 📦 Build |
| `style` | Code style changes (formatting, etc.) | (hidden) |

### Scopes

Common scopes for this project:

- `authz` - Authorization system
- `otel` - OpenTelemetry integration
- `registry` - Service registry
- `middleware` - HTTP middleware
- `config` - Configuration
- `errors` - Error handling
- `deps` - Dependencies

### Examples

```bash
# Feature
feat(authz): add composite policy support

# Bug fix
fix(middleware): handle nil context in auth check

# Breaking change (note the !)
feat(registry)!: change service registration API

# With body and footer
fix(otel): prevent panic on nil tracer provider

The tracer provider could be nil when OTEL is disabled,
causing a panic in the logging middleware.

Fixes #123
```

### Breaking Changes

For breaking changes, add `!` after the type/scope and include a `BREAKING CHANGE:` footer:

```
feat(api)!: redesign operation registration

BREAKING CHANGE: The Register function now requires an Operation struct
instead of individual parameters.

Migration:
- Before: volt.Register(app, "GET", "/path", handler)
- After:  volt.Register(app, volt.Operation{Method: "GET", Path: "/path"}, handler)
```

## Pull Request Process

1. **Fork** the repository
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-awesome-feature
   ```
3. **Make your changes** and commit using conventional commits
4. **Run tests and linter**:
   ```bash
   go test -v -race ./...
   golangci-lint run
   ```
5. **Push** to your fork and create a Pull Request
6. **PR title** must follow conventional commit format (it becomes the squash commit message)

### PR Checklist

- [ ] Tests added/updated for changes
- [ ] Documentation updated if needed
- [ ] PR title follows conventional commit format
- [ ] All CI checks pass

## Code Style

- Follow standard Go conventions
- Use `gofmt` and `goimports`
- Write table-driven tests where applicable
- Add doc comments for exported functions/types
- Keep functions focused and small

## Testing

```bash
# Run all tests
go test -v ./...

# Run with race detector
go test -v -race ./...

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -v -run TestAuthzRequirement ./...
```

## Release Process

Releases are automated via GitHub Actions:

1. **Merge PRs** to `main` with conventional commit messages
2. **Release Please** automatically creates/updates a release PR
3. **Merge the release PR** to trigger the release
4. A new GitHub release is created with auto-generated changelog

### Manual Release

If needed, you can trigger a manual release:

1. Go to Actions → Release → Run workflow
2. Enter the version (e.g., `v1.2.3`)
3. Check "prerelease" if applicable

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions
- Check existing issues before creating new ones

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
