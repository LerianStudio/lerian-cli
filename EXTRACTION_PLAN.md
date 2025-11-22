# Lerian CLI Extraction Plan

**Created:** 2025-11-22
**Source Repository:** saas-poc/applications/lerian-cli
**Target Repository:** /Users/ferr3ira/Documents/empresas/lerian-studio/projetos/midaz/applications/lerian-cli

## Phase 1: Preparation & Validation ✅ COMPLETED

### Source Code Analysis

**Source Directory:** `~/Documents/empresas/lerian-studio/projetos/midaz/applications/saas-poc/applications/lerian-cli`

**Current State:**
- Go module verified: ✅ All modules verified
- Build status: ✅ Successful (`go build ./...`)
- Module path: `github.com/lerian-studio/lerian-cli`
- Go version: 1.25.4

### Directory Structure
```
lerian-cli/
├── cmd/                      # Command implementations
│   ├── auth/                 # Authentication commands
│   ├── config/               # Configuration commands
│   └── midaz/                # Midaz-specific commands
├── internal/                 # Internal packages
│   ├── client/               # HTTP API client
│   ├── config/               # Config management
│   ├── kubectl/              # Kubernetes operations
│   └── output/               # Output formatting
├── main.go                   # Entry point
├── go.mod                    # Go module definition
├── go.sum                    # Dependency checksums
├── Makefile                  # Build automation
├── README.md                 # Documentation
└── IMPLEMENTATION_PLAN.md    # Historical implementation plan
```

### External Dependencies

**Direct Dependencies:**
- `github.com/google/uuid v1.6.0` - UUID generation
- `github.com/spf13/cobra v1.10.1` - CLI framework
- `gopkg.in/yaml.v3 v3.0.1` - YAML parsing

**Indirect Dependencies:**
- `github.com/fatih/color v1.18.0` - Terminal colors
- `github.com/inconshreveable/mousetrap v1.1.0` - Windows support
- `github.com/mattn/go-colorable v0.1.13` - Color support
- `github.com/mattn/go-isatty v0.0.20` - TTY detection
- `github.com/spf13/pflag v1.0.9` - POSIX flags
- `golang.org/x/sys v0.25.0` - System calls

**External Tool Dependencies:**
- `kubectl` - Required for Kubernetes operations (logs, port-forward, exec, events)

### Integration Points

**✅ Self-Contained Verification:**
- No imports from parent saas-poc repository
- No relative imports outside CLI boundary (`../../` not found)
- All imports use the CLI module path or external dependencies

**Integration with Lerian Ecosystem:**
1. **Lerian Control Plane API** (HTTP/REST)
   - Endpoint: Configurable via `~/.lerian/config.yaml`
   - Authentication: API key in `X-API-Key` header
   - Tenant isolation: All operations scoped by tenant ID
   - API paths: `/api/tenants/{tenant_id}/deployments`, `/api/deployments/{id}`

2. **Kubernetes Clusters** (kubectl wrapper)
   - Direct kubectl command execution
   - No programmatic K8s API usage
   - Requires kubectl installed and configured

3. **Configuration Storage**
   - Local file: `~/.lerian/config.yaml`
   - Profile-based credential management
   - No external config service

### Test Coverage

**Current Status:**
- Unit tests: ❌ None found (0 `*_test.go` files)
- Integration tests: ❌ None found
- Test fixtures: ❌ None found

**Impact:** No test migration required, but tests should be added in future phases.

### Files to Copy

**Core Application Files:**
- [x] `main.go`
- [x] `go.mod`
- [x] `go.sum`
- [x] `Makefile`
- [x] `cmd/` directory (all subdirectories)
- [x] `internal/` directory (all subdirectories)

**Documentation Files:**
- [x] `README.md` (as reference, will be rewritten)
- [x] `IMPLEMENTATION_PLAN.md` (as historical reference)

**Files to Exclude:**
- [ ] `bin/` - Compiled binaries
- [ ] `backups/` - Backup files
- [ ] `.DS_Store` - macOS metadata

### Validation Checklist

#### Source Validation
- [x] All Go files compile without errors
- [x] Go modules verified (`go mod verify`)
- [x] No build errors
- [x] No cross-repository imports
- [x] No relative parent imports
- [x] Module path is correct: `github.com/lerian-studio/lerian-cli`

#### Dependency Validation
- [x] All dependencies available in public registries
- [x] No internal/private dependencies
- [x] Compatible Go version (1.25.4)

#### Structure Validation
- [x] Standard Go project layout
- [x] Clear separation of concerns (cmd, internal)
- [x] Self-contained codebase

### Identified Risks

#### ✅ Mitigated Risks
1. **Import Path Issues** - None found, all imports are self-contained
2. **Hidden Dependencies** - None found, codebase is self-contained
3. **Module Path Conflicts** - Module path already correct

#### ⚠️ Remaining Risks
1. **Missing Tests** - No test coverage (will add in Phase 8)
2. **kubectl Dependency** - Runtime requirement (document in README)
3. **Version Information** - Currently returns hardcoded "0.1.0" (will implement in Phase 7)

### Next Steps

**Phase 2: Repository Bootstrap**
- Verify target repository structure
- Ensure .gitignore is comprehensive
- Set up branch structure

**Phase 3: CLI Extraction**
- Copy source files from saas-poc to standalone repo
- Preserve file timestamps and structure
- Create extraction commit with source reference

**Phase 4: Structure & Cleanup**
- Move `main.go` to `cmd/lerian/main.go`
- Verify module path
- Clean up any temporary files

### Notes

- Source code is in excellent condition for extraction
- No refactoring required for isolation
- Original implementation already followed best practices
- Module path matches target GitHub organization
- No breaking changes expected during extraction

### Source Repository State

**Last Commit (saas-poc):**
- Date: 2025-11-18
- Files: All CLI files present and buildable

**Build Verification:**
```bash
cd ~/Documents/empresas/lerian-studio/projetos/midaz/applications/saas-poc/applications/lerian-cli
go mod verify    # ✅ all modules verified
go build ./...   # ✅ successful
```

---

## Extraction Execution Log

### Phase 1: Preparation & Validation - COMPLETED ✅
**Date:** 2025-11-22
**Duration:** ~15 minutes
**Status:** All validation checks passed

**Completed Tasks:**
1. ✅ Verified target repository exists at correct path
2. ✅ Analyzed source code structure
3. ✅ Verified module integrity
4. ✅ Checked for cross-repository dependencies (none found)
5. ✅ Documented external dependencies
6. ✅ Identified integration points
7. ✅ Created extraction plan document

**Key Findings:**
- Source code is 100% self-contained
- No saas-poc dependencies
- All imports use correct module path
- No tests to migrate (will create new tests)
- Ready for clean extraction

**Ready for Phase 2: Repository Bootstrap** ✅
