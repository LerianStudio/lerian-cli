# Contributing to Lerian CLI

Thank you for your interest in contributing to Lerian CLI! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

This project adheres to a Code of Conduct that all contributors are expected to follow. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before contributing.

## How to Contribute

### Reporting Bugs

Before creating bug reports, please check the existing issues to avoid duplicates. When creating a bug report, include:

- **Clear title and description**
- **Steps to reproduce** the issue
- **Expected behavior** vs **actual behavior**
- **Environment details** (OS, Go version, CLI version)
- **Relevant logs or error messages**
- **Screenshots** if applicable

**Example Bug Report:**
```markdown
**Description:** `lerian midaz ledger create` fails with timeout error

**Steps to Reproduce:**
1. Run `lerian auth login` with valid credentials
2. Run `lerian midaz ledger create --name test --region us-east-1 --env dev`
3. Wait for timeout

**Expected:** Ledger created successfully
**Actual:** Timeout after 5 minutes

**Environment:**
- OS: macOS 13.5
- Go Version: 1.21.3
- CLI Version: v0.1.0

**Logs:**
```
[paste relevant logs]
```
```

### Suggesting Enhancements

Enhancement suggestions are welcome! Please provide:

- **Clear use case** - Why is this enhancement needed?
- **Proposed solution** - How should it work?
- **Alternatives considered** - What other approaches did you think about?
- **Additional context** - Any other relevant information

### Pull Requests

1. **Fork** the repository
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   # or
   git checkout -b fix/issue-123
   ```
3. **Make your changes**
4. **Add tests** if applicable
5. **Run tests** and ensure they pass
6. **Commit** your changes (see commit conventions below)
7. **Push** to your fork
8. **Create a Pull Request**

## Development Setup

### Prerequisites

- **Go 1.21 or higher**
- **Git**
- **Make**
- **kubectl** (for Kubernetes operations testing)

### Setting Up Your Development Environment

1. **Clone your fork:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/lerian-cli.git
   cd lerian-cli
   ```

2. **Add upstream remote:**
   ```bash
   git remote add upstream https://github.com/lerian-studio/lerian-cli.git
   ```

3. **Install dependencies:**
   ```bash
   make deps
   ```

4. **Build the project:**
   ```bash
   make build
   ```

5. **Run the CLI:**
   ```bash
   ./build/bin/lerian --help
   ```

### Development Workflow

1. **Keep your fork synced:**
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

2. **Create a feature branch:**
   ```bash
   git checkout -b feat/my-feature
   ```

3. **Make changes and test:**
   ```bash
   # Build
   make build

   # Test your changes
   ./build/bin/lerian [command]

   # Run tests (when available)
   make test
   ```

4. **Commit your changes:**
   ```bash
   git add .
   git commit -m "feat(ledger): add support for custom timeouts"
   ```

5. **Push to your fork:**
   ```bash
   git push origin feat/my-feature
   ```

6. **Create Pull Request** on GitHub

## Coding Standards

### Go Style Guide

- Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (automatically applied by most editors)
- Use `go vet` to check for common errors
- Follow the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### Code Organization

```
lerian-cli/
├── cmd/
│   ├── lerian/          # Main entry point
│   ├── auth/            # Auth commands
│   └── midaz/           # Midaz commands
│       └── ledger/      # Ledger subcommands
├── internal/            # Private packages
│   ├── client/          # HTTP API client
│   ├── config/          # Config management
│   ├── kubectl/         # Kubernetes wrapper
│   └── output/          # Output formatting
```

### Naming Conventions

- **Packages:** Short, lowercase, single-word names (e.g., `client`, `config`)
- **Files:** Lowercase with underscores if needed (e.g., `ledger.go`, `port_forward.go`)
- **Functions/Methods:** CamelCase (e.g., `CreateLedger`, `GetConfig`)
- **Variables:** CamelCase, short but descriptive (e.g., `apiClient`, `ledgerID`)
- **Constants:** CamelCase or UPPER_CASE for exported constants

### Error Handling

- Always handle errors explicitly
- Use `fmt.Errorf` with `%w` for error wrapping
- Provide context in error messages

```go
// Good
if err != nil {
    return fmt.Errorf("failed to create ledger: %w", err)
}

// Bad
if err != nil {
    return err
}
```

### Comments

- **Package comments:** Describe the package purpose
- **Exported functions:** Must have a comment starting with the function name
- **Complex logic:** Explain the "why", not the "what"

```go
// CreateLedger creates a new ledger deployment in the specified region.
// It validates the input parameters and polls for deployment status until
// the ledger is available or the operation times out.
func CreateLedger(req *CreateLedgerRequest) (*Ledger, error) {
    // Implementation...
}
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
go test -cover ./...

# Run tests verbosely
go test -v ./...

# Run specific package tests
go test ./internal/client/...
```

### Writing Tests

- Place test files next to the code they test (e.g., `client.go` → `client_test.go`)
- Use table-driven tests for multiple test cases
- Name test functions descriptively

```go
func TestCreateLedger(t *testing.T) {
    tests := []struct {
        name    string
        input   *CreateLedgerRequest
        want    *Ledger
        wantErr bool
    }{
        {
            name: "valid saas ledger",
            input: &CreateLedgerRequest{
                Name:   "test-ledger",
                Region: "us-east-1",
                Env:    "dev",
            },
            wantErr: false,
        },
        {
            name: "invalid name",
            input: &CreateLedgerRequest{
                Name:   "ab", // too short
                Region: "us-east-1",
                Env:    "dev",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CreateLedger(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateLedger() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            // Additional assertions...
        })
    }
}
```

## Commit Message Convention

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification.

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Type

- **feat**: New feature
- **fix**: Bug fix
- **docs**: Documentation only changes
- **style**: Code style changes (formatting, missing semi-colons, etc.)
- **refactor**: Code change that neither fixes a bug nor adds a feature
- **perf**: Performance improvement
- **test**: Adding or updating tests
- **chore**: Changes to build process, tooling, dependencies

### Scope

The scope should specify the area of the codebase:
- `ledger` - Ledger commands
- `auth` - Authentication
- `config` - Configuration
- `client` - API client
- `output` - Output formatting
- `cli` - General CLI changes

### Examples

```
feat(ledger): add support for custom deployment timeouts

Add --timeout flag to ledger create command allowing users to
specify custom timeout values for deployment provisioning.

Closes #123
```

```
fix(auth): handle expired API keys gracefully

Previously, expired API keys caused cryptic error messages.
Now we detect expired keys and provide clear instructions
for re-authentication.

Fixes #456
```

```
docs(readme): update installation instructions

Add instructions for installing via Homebrew and update
the quick start guide with current command syntax.
```

## Pull Request Process

### Before Submitting

1. **Update documentation** if you're changing functionality
2. **Add tests** for new features
3. **Ensure tests pass** locally
4. **Run formatting tools**: `gofmt -w .`
5. **Update CHANGELOG.md** if applicable

### PR Title

Use the same format as commit messages:
```
feat(ledger): add multi-region deployment support
```

### PR Description Template

```markdown
## Description
Brief description of the changes

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## How Has This Been Tested?
Describe the tests you ran and their results

## Checklist
- [ ] My code follows the style guidelines of this project
- [ ] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes

## Screenshots (if applicable)

## Related Issues
Closes #issue_number
```

### Review Process

1. **Automated checks** must pass (CI/CD when configured)
2. **At least one maintainer** must approve
3. **All conversations** must be resolved
4. **Branch must be up to date** with main

### After Approval

- Maintainers will merge your PR
- Your contribution will be included in the next release
- You'll be added to the contributors list

## Adding New Commands

When adding a new command:

1. **Create the command file** in the appropriate directory
2. **Follow the Cobra pattern**:
   ```go
   var myCmd = &cobra.Command{
       Use:   "mycommand",
       Short: "Short description",
       Long:  `Long description`,
       RunE:  runMyCommand,
   }

   func init() {
       // Add flags
       myCmd.Flags().StringVar(&myFlag, "flag", "", "Description")
   }

   func runMyCommand(cmd *cobra.Command, args []string) error {
       // Implementation
       return nil
   }
   ```
3. **Register the command** in the parent command's init()
4. **Add tests** for the command
5. **Update documentation**

## Documentation

### Updating Documentation

- **README.md**: User-facing documentation, installation, quick start
- **docs/**: Detailed guides and references
- **Code comments**: Implementation details, gotchas
- **CHANGELOG.md**: User-visible changes

### Documentation Style

- Use **clear, concise language**
- Provide **code examples** for common use cases
- Include **screenshots** for visual features
- Keep **line length** under 100 characters
- Use **Markdown** formatting consistently

## Release Process

(Maintained by project maintainers)

1. Update CHANGELOG.md
2. Create release branch: `release/v0.2.0`
3. Update version in code
4. Create and push tag: `v0.2.0`
5. GitHub Actions creates release artifacts
6. Publish release notes

## Getting Help

- **Questions?** Open a [Discussion](https://github.com/lerian-studio/lerian-cli/discussions)
- **Bug?** Open an [Issue](https://github.com/lerian-studio/lerian-cli/issues)
- **Chat:** Join our community (link TBD)
- **Email:** dev@lerian.studio

## Recognition

Contributors are recognized in:
- GitHub Contributors page
- CHANGELOG.md (for significant contributions)
- Release notes

Thank you for contributing to Lerian CLI! 🎉
