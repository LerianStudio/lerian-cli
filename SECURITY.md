# Security Policy

## Supported Versions

We release patches for security vulnerabilities in the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please report it by emailing:

**security@lerian.studio**

You should receive a response within 48 hours. If for some reason you do not, please follow up via email to ensure we received your original message.

### What to Include in Your Report

Please include the following information in your report:

- **Type of vulnerability** (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
- **Full paths of source file(s)** related to the vulnerability
- **Location of the affected source code** (tag/branch/commit or direct URL)
- **Step-by-step instructions** to reproduce the issue
- **Proof-of-concept or exploit code** (if possible)
- **Impact of the issue**, including how an attacker might exploit it

This information will help us triage your report more quickly.

### What to Expect

After submitting a vulnerability report, you can expect:

1. **Acknowledgment** within 48 hours
2. **Initial assessment** within 7 days
3. **Regular updates** on our progress
4. **Credit** in the security advisory (if you wish)

## Security Best Practices for Users

### API Key Management

**Do:**
- Store API keys securely in `~/.lerian/config.yaml` with restricted file permissions (600)
- Use different API keys for development, staging, and production environments
- Rotate API keys regularly
- Revoke compromised API keys immediately

**Don't:**
- Commit `config.yaml` or API keys to version control
- Share API keys via email, chat, or other insecure channels
- Use production API keys in development or CI/CD environments
- Hard-code API keys in scripts or applications

### Configuration File Security

The CLI stores sensitive configuration at `~/.lerian/config.yaml`. Ensure this file has proper permissions:

```bash
# Set restrictive permissions on config file
chmod 600 ~/.lerian/config.yaml

# Verify permissions
ls -la ~/.lerian/config.yaml
# Should show: -rw------- (600)
```

### Multi-Profile Security

When using multiple profiles (e.g., development, staging, production):

```yaml
current-profile: development  # Default to least privileged

profiles:
  development:
    api-url: https://dev-api.lerian.studio
    api-key: dev_key_xxx
    tenant-id: dev-tenant-xxx

  production:
    api-url: https://api.lerian.studio
    api-key: prod_key_xxx  # Most privileged
    tenant-id: prod-tenant-xxx
```

Always:
- Default to the least privileged profile (development)
- Explicitly specify `--profile production` for production operations
- Audit production profile usage regularly
- Use separate tenants for non-production environments

### Kubernetes Operations Security

The CLI executes `kubectl` commands for certain operations. Ensure:

1. **kubeconfig Security**
   ```bash
   # Restrict kubeconfig permissions
   chmod 600 ~/.kube/config
   ```

2. **Context Awareness**
   - Always verify your current kubectl context before operations
   - Use dedicated contexts for production clusters
   - Never point development CLI at production clusters

3. **RBAC Compliance**
   - Ensure your Kubernetes credentials have appropriate RBAC permissions
   - Use service accounts with limited scope where possible
   - Audit kubectl operations regularly

### Network Security

**Private Deployments:**
- Ensure Lerian Agent has secure network connectivity to your clusters
- Use VPNs or private networks for agent communication
- Restrict agent API access with firewall rules

**API Communication:**
- Always use HTTPS endpoints (enforced by CLI)
- Verify SSL/TLS certificates
- Be cautious with custom `--api-url` flags

### CI/CD Integration Security

When using the CLI in CI/CD pipelines:

**Do:**
- Use environment variables for sensitive data
- Store API keys in secret management systems (GitHub Secrets, Vault, etc.)
- Use dedicated service accounts with minimal required permissions
- Implement secret rotation in your pipelines
- Audit CI/CD access logs regularly

**Don't:**
- Log API keys or config file contents
- Use personal API keys in shared CI/CD environments
- Store secrets in plaintext in CI/CD configuration files
- Use wildcards when granting CI/CD permissions

**Example (GitHub Actions):**
```yaml
- name: Deploy Ledger
  env:
    LERIAN_API_KEY: ${{ secrets.LERIAN_API_KEY }}
    LERIAN_TENANT_ID: ${{ secrets.LERIAN_TENANT_ID }}
  run: |
    lerian auth login \
      --api-url https://api.lerian.studio \
      --api-key "$LERIAN_API_KEY" \
      --tenant-id "$LERIAN_TENANT_ID"
    lerian midaz ledger create --name ci-ledger --region us-east-1 --env dev
```

### Binary Verification

When downloading pre-built binaries:

1. **Download from official sources only**
   - GitHub releases: https://github.com/lerian-studio/lerian-cli/releases
   - Official website: https://lerian.studio (when available)

2. **Verify checksums**
   ```bash
   # Download checksum file
   curl -LO https://github.com/lerian-studio/lerian-cli/releases/download/vX.Y.Z/checksums.txt

   # Verify checksum
   sha256sum -c checksums.txt 2>&1 | grep lerian
   ```

3. **Build from source when possible**
   ```bash
   git clone https://github.com/lerian-studio/lerian-cli.git
   cd lerian-cli
   git checkout vX.Y.Z  # Use specific version tag
   make build
   ```

## Vulnerability Disclosure Process

### For Reporters

1. **Submit Report** to security@lerian.studio
2. **Wait for Acknowledgment** (48 hours)
3. **Work with us** to understand and validate the issue
4. **Coordinated Disclosure** - we'll work with you on timing
5. **Public Disclosure** after patch is released (if applicable)

### For Maintainers

When a vulnerability is reported:

1. **Acknowledge** receipt within 48 hours
2. **Validate** the vulnerability
3. **Assess** severity using CVSS v3.1
4. **Develop** a fix in a private branch
5. **Test** the fix thoroughly
6. **Coordinate** disclosure timeline with reporter
7. **Release** patched version
8. **Publish** security advisory
9. **Credit** reporter (with permission)

## Security Advisories

Security advisories will be published at:
- **GitHub Security Advisories**: https://github.com/lerian-studio/lerian-cli/security/advisories
- **Lerian Security Page**: https://lerian.studio/security (when available)

## Responsible Disclosure

We follow a responsible disclosure process:

- **Embargo period**: Typically 90 days from initial report
- **Early disclosure**: If a vulnerability is being actively exploited
- **Coordinated disclosure**: We work with reporters on timing
- **Public credit**: We acknowledge reporters in advisories (with permission)

## Security Update Policy

- **Critical vulnerabilities**: Patches released within 7 days
- **High severity**: Patches released within 30 days
- **Medium severity**: Patches released within 90 days
- **Low severity**: Patches released in next regular version

## Scope

This security policy applies to:
- The `lerian-cli` binary and source code
- Official Docker images (if any)
- Official installation scripts
- GitHub Actions workflows in this repository

**Out of Scope:**
- Lerian Control Plane API (report to https://lerian.studio/security)
- Third-party dependencies (report to respective maintainers)
- Midaz ledger deployments (report to Midaz security team)

## Contact

**Security Team**: security@lerian.studio
**General Support**: support@lerian.studio
**Bug Reports**: https://github.com/lerian-studio/lerian-cli/issues

For urgent security matters, please use the security email. For general bugs, please use GitHub issues.

## Recognition

We appreciate the security research community's efforts in identifying and responsibly disclosing vulnerabilities. Reporters who follow our responsible disclosure process may be acknowledged in:

- Security advisories
- CHANGELOG.md
- GitHub security hall of fame (when available)

Thank you for helping keep Lerian CLI and our users safe!
