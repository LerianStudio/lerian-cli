# Testing Strategy for Lerian CLI

## Current Status

**Test Coverage**: ~5% (only version package has tests)

**Goal**: Achieve 80%+ test coverage with comprehensive test suite

## Testing Philosophy

### Test Pyramid

```
        /\
       /  \
      / E2E\           10% - End-to-End Tests
     /------\
    /        \
   /Integration\       20% - Integration Tests
  /------------\
 /              \
/   Unit Tests   \     70% - Unit Tests
------------------
```

## Test Types

### 1. Unit Tests (Priority: HIGH)

**What**: Test individual functions and methods in isolation

**Coverage Target**: 80%+

**Files to Test**:
- `internal/config/config.go` - Configuration parsing
- `internal/output/table.go` - Output formatting
- `internal/version/version.go` - ✓ Already tested (90.9%)
- `internal/kubectl/kubectl.go` - Kubectl wrapper
- `internal/client/*.go` - API client methods
- `cmd/auth/*.go` - Auth command logic
- `cmd/midaz/ledger/*.go` - Ledger command logic

**Example**: `internal/config/config_test.go`
```go
func TestLoadConfig(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Config
        wantErr bool
    }{
        {
            name:  "valid config",
            input: "testdata/valid-config.yaml",
            want:  &Config{...},
            wantErr: false,
        },
        {
            name:  "invalid yaml",
            input: "testdata/invalid.yaml",
            want:  nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := LoadConfig(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("LoadConfig() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Best Practices**:
- Use table-driven tests
- Test happy path and error cases
- Mock external dependencies
- Use `-short` flag for fast unit tests

### 2. Integration Tests (Priority: MEDIUM)

**What**: Test multiple components working together

**Coverage Target**: Key workflows

**Files to Test**:
- `cmd/lerian/integration_test.go` - ✓ Already exists
- Command execution with real file I/O
- Config loading with auth flow
- Kubectl interactions with mocked k8s

**Example**: `cmd/auth/integration_test.go`
```go
func TestAuthFlow_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Setup temporary config directory
    tmpDir := t.TempDir()
    os.Setenv("HOME", tmpDir)
    defer os.Unsetenv("HOME")

    // Test login flow
    cmd := exec.Command("lerian", "auth", "login", "--profile", "test")
    cmd.Stdin = strings.NewReader("username\npassword\n")
    output, err := cmd.CombinedOutput()

    if err != nil {
        t.Fatalf("auth login failed: %v\n%s", err, output)
    }

    // Verify config was created
    configPath := filepath.Join(tmpDir, ".lerian", "config.yaml")
    if _, err := os.Stat(configPath); os.IsNotExist(err) {
        t.Error("config file not created")
    }
}
```

**Best Practices**:
- Use `testing.Short()` to skip in unit test runs
- Clean up resources (use `t.TempDir()`)
- Test realistic workflows
- Run with `make test-integration`

### 3. Table Output Tests (Priority: HIGH)

**What**: Test CLI output formatting

**Coverage Target**: All output formats (table, JSON, YAML)

**Example**: `internal/output/table_test.go`
```go
func TestFormatTable(t *testing.T) {
    tests := []struct {
        name    string
        data    interface{}
        format  string
        want    string
        wantErr bool
    }{
        {
            name: "simple struct to table",
            data: struct {
                Name string
                Age  int
            }{"John", 30},
            format: "table",
            want:   "NAME\tAGE\nJohn\t30\n",
            wantErr: false,
        },
        {
            name: "struct to json",
            data: struct {
                Name string `json:"name"`
            }{"John"},
            format: "json",
            want:   `{"name":"John"}`,
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Format(tt.data, tt.format)
            if (err != nil) != tt.wantErr {
                t.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Format() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 4. Command Tests (Priority: HIGH)

**What**: Test Cobra command setup and flags

**Example**: `cmd/midaz/ledger/create_test.go`
```go
func TestCreateCommand(t *testing.T) {
    // Setup
    cmd := NewCreateCommand()

    // Test flags
    tests := []struct {
        name    string
        args    []string
        wantErr bool
    }{
        {
            name:    "valid args",
            args:    []string{"--name", "test-ledger", "--namespace", "default"},
            wantErr: false,
        },
        {
            name:    "missing required flag",
            args:    []string{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd.SetArgs(tt.args)
            err := cmd.Execute()
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### 5. Mock Tests (Priority: MEDIUM)

**What**: Test with mocked external dependencies

**Use Cases**:
- Mock Kubernetes API
- Mock HTTP clients
- Mock file system

**Example**: `internal/client/client_test.go`
```go
type MockHTTPClient struct {
    DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    return m.DoFunc(req)
}

func TestGetDeployment(t *testing.T) {
    mockClient := &MockHTTPClient{
        DoFunc: func(req *http.Request) (*http.Response, error) {
            json := `{"name":"test-deployment"}`
            return &http.Response{
                StatusCode: 200,
                Body:       io.NopCloser(strings.NewReader(json)),
            }, nil
        },
    }

    client := &Client{httpClient: mockClient}
    deployment, err := client.GetDeployment("test")

    if err != nil {
        t.Fatalf("unexpected error: %v", err, )
    }
    if deployment.Name != "test-deployment" {
        t.Errorf("got %s, want test-deployment", deployment.Name)
    }
}
```

### 6. Benchmark Tests (Priority: LOW)

**What**: Performance testing

**Example**: `internal/output/table_bench_test.go`
```go
func BenchmarkFormatTable(b *testing.B) {
    data := []struct {
        Name string
        Age  int
    }{
        {"John", 30},
        {"Jane", 25},
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        Format(data, "table")
    }
}
```

**Run**: `make test-bench`

### 7. End-to-End Tests (Priority: LOW)

**What**: Full CLI workflow tests in real environment

**Use Cases**:
- Complete user workflows
- Multi-command sequences
- Real Kubernetes cluster interaction

**Example**: `e2e/ledger_lifecycle_test.go`
```go
func TestLedgerLifecycle_E2E(t *testing.T) {
    if os.Getenv("E2E") != "true" {
        t.Skip("skipping E2E test")
    }

    // Create ledger
    cmd := exec.Command("lerian", "midaz", "ledger", "create", "--name", "test")
    if err := cmd.Run(); err != nil {
        t.Fatalf("create failed: %v", err)
    }

    // List ledgers
    cmd = exec.Command("lerian", "midaz", "ledger", "list")
    output, _ := cmd.Output()
    if !strings.Contains(string(output), "test") {
        t.Error("ledger not found in list")
    }

    // Delete ledger
    cmd = exec.Command("lerian", "midaz", "ledger", "delete", "test")
    if err := cmd.Run(); err != nil {
        t.Fatalf("delete failed: %v", err)
    }
}
```

**Run**: `E2E=true go test ./e2e/...`

## Test Organization

### Directory Structure

```
lerian-cli/
├── cmd/
│   ├── auth/
│   │   ├── login.go
│   │   ├── login_test.go          # Unit tests
│   │   └── integration_test.go    # Integration tests
│   ├── midaz/
│   │   └── ledger/
│   │       ├── create.go
│   │       └── create_test.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   ├── config_test.go
│   │   └── testdata/              # Test fixtures
│   │       ├── valid-config.yaml
│   │       └── invalid-config.yaml
│   └── output/
│       ├── table.go
│       ├── table_test.go
│       └── table_bench_test.go    # Benchmarks
└── e2e/
    └── ledger_test.go             # E2E tests
```

### Test Files Naming

- Unit tests: `*_test.go`
- Integration tests: `*_integration_test.go` or `integration_test.go`
- Benchmarks: `*_bench_test.go`
- E2E tests: `e2e/*_test.go`

## Test Coverage Goals

| Package | Current | Target | Priority |
|---------|---------|--------|----------|
| `internal/version` | 90.9% | 90%+ | ✓ Done |
| `internal/config` | 0% | 80%+ | HIGH |
| `internal/output` | 0% | 80%+ | HIGH |
| `internal/kubectl` | 0% | 70%+ | MEDIUM |
| `internal/client` | 0% | 70%+ | MEDIUM |
| `cmd/auth` | 0% | 60%+ | MEDIUM |
| `cmd/midaz/ledger` | 0% | 60%+ | MEDIUM |
| **Overall** | **~5%** | **80%+** | - |

## Testing Tools & Libraries

### Required

- **testing** - Go standard library
- **testify/assert** - Assertions (optional but recommended)
- **testify/mock** - Mocking framework
- **cobra** - Already used for CLI

### Optional

- **gomega** - Matcher library
- **ginkgo** - BDD testing framework
- **httptest** - HTTP testing
- **go-cmp** - Deep equality comparison

## Running Tests

### Make Commands

```bash
# Run all tests
make test

# Run unit tests only (fast)
make test-unit

# Run integration tests
make test-integration

# Generate coverage report
make test-coverage

# Run with race detector
make test-race

# Run benchmarks
make test-bench

# Run linter
make lint

# Format code
make fmt
```

### Test Flags

```bash
# Run specific test
go test -v -run TestLoadConfig ./internal/config

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...

# Skip long tests
go test -short ./...

# Generate HTML coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## CI/CD Integration

Tests are automatically run in CI pipeline:

- **PR validation**: Unit tests (fast)
- **Merge to develop**: Unit + Integration tests
- **Release**: Full test suite + benchmarks

## Test-Driven Development (TDD)

Recommended workflow:

1. **Red**: Write failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code while keeping tests green

Example workflow:
```bash
# 1. Write test
vim internal/config/config_test.go

# 2. Run test (should fail)
go test -v ./internal/config

# 3. Implement feature
vim internal/config/config.go

# 4. Run test (should pass)
go test -v ./internal/config

# 5. Refactor and re-run
make test
```

## Next Steps

### Phase 1: Critical Coverage (Week 1-2)
- [ ] `internal/config` - 80%+ coverage
- [ ] `internal/output` - 80%+ coverage
- [ ] `internal/client` - 70%+ coverage

### Phase 2: Command Coverage (Week 3-4)
- [ ] `cmd/auth/*` - 60%+ coverage
- [ ] `cmd/midaz/ledger/*` - 60%+ coverage
- [ ] Integration tests for main workflows

### Phase 3: Advanced Testing (Week 5-6)
- [ ] `internal/kubectl` - 70%+ coverage
- [ ] E2E tests for critical paths
- [ ] Performance benchmarks

### Phase 4: Maintenance
- [ ] Keep coverage above 80%
- [ ] Add tests for all new features
- [ ] Update tests when refactoring

## Resources

- [Go Testing Docs](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Table-Driven Tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [Go Test Comments](https://github.com/golang/go/wiki/TestComments)

---

**Last Updated**: 2025-11-24
**Coverage Goal**: 80%+
**Current Coverage**: ~5%
