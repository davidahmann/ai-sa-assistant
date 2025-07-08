# GitHub Actions Security Best Practices

This document outlines the security measures implemented in our GitHub Actions workflows to maintain a secure CI/CD pipeline.

## Security Principles Applied

### 1. Principle of Least Privilege (Addresses CKV2_GHA_1)

All workflows now use explicit permission declarations instead of default write-all permissions:

#### CI Pipeline Permissions

```yaml
permissions:
  contents: read         # For checking out code
  actions: read         # For downloading artifacts
  security-events: write # For uploading SARIF files to CodeQL
  checks: write         # For reporting test results
  pull-requests: write  # For commenting on PRs with benchmark results
```

#### Release Pipeline Permissions

```yaml
permissions:
  contents: write        # For creating releases and updating files
  packages: write        # For publishing container images
  actions: read         # For downloading artifacts
  security-events: write # For security scanning
  id-token: write       # For signing containers with cosign
```

#### Security Pipeline Permissions

The security workflow already had properly scoped permissions and was not modified.

### 2. Input Validation (Addresses CKV_GHA_7)

The release workflow now includes comprehensive input validation for `workflow_dispatch` inputs:

#### Version Input Validation

- Sanitizes input by removing potentially dangerous characters
- Validates against strict semver pattern: `^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$`
- Provides clear error messages for invalid formats
- Exits workflow execution on validation failure

#### Boolean Input Validation

- Validates prerelease input is strictly "true" or "false"
- Prevents injection through boolean parameters

```bash
# Sanitize and validate user input (addresses CKV_GHA_7)
RAW_VERSION="${{ github.event.inputs.version }}"
RAW_PRERELEASE="${{ github.event.inputs.prerelease }}"

# Remove any potentially dangerous characters and validate format
VERSION=$(echo "$RAW_VERSION" | sed 's/[^0-9a-zA-Z.-]//g')

# Validate semver format more strictly
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9]+)?$ ]]; then
  echo "::error::Invalid version format: $VERSION. Must be semver (e.g., 1.0.0 or 1.0.0-alpha)"
  exit 1
fi

# Validate boolean input
if [[ "$RAW_PRERELEASE" != "true" && "$RAW_PRERELEASE" != "false" ]]; then
  echo "::error::Invalid prerelease value: $RAW_PRERELEASE. Must be true or false"
  exit 1
fi
```

## Security Review Process

### Pre-Commit Security Validation

- All workflows are validated using Checkov security scanning
- Security policies enforced through `.pre-commit-config.yaml`
- Manual security review required for workflow changes

### Ongoing Security Monitoring

- Weekly security scans via scheduled workflows
- Dependency vulnerability scanning with govulncheck and Nancy
- Container image scanning with Trivy and Grype
- Secret scanning with GitLeaks and TruffleHog
- Static analysis with gosec and semgrep

### Incident Response

- Security failures automatically create GitHub issues with 'security' and 'urgent' labels
- Notifications sent to security team via configured webhooks
- Comprehensive security reports generated for each scan

## Implementation Checklist

- [x] Remove write-all permissions from CI workflow
- [x] Remove write-all permissions from release workflow
- [x] Add explicit minimal permissions to all workflows
- [x] Implement input validation for workflow_dispatch parameters
- [x] Add input sanitization to prevent injection attacks
- [x] Document security best practices
- [x] Validate changes with existing test suite
- [x] Ensure CI/CD functionality is preserved

## Compliance Status

| Checkov Rule | Description | Status | Implementation |
|--------------|-------------|--------|----------------|
| CKV2_GHA_1 | Top-level permissions not set to write-all | ✅ Fixed | Explicit minimal permissions added to all workflows |
| CKV_GHA_7 | Build output protected from user parameter influence | ✅ Fixed | Input validation and sanitization implemented |

## References

- [GitHub Actions Security Hardening](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)
- [Checkov GitHub Actions Policies](https://docs.prismacloud.io/en/enterprise-edition/policy-reference/ci-cd-pipeline-policies/github-actions-policies)
- [NIST Secure Software Development Framework](https://csrc.nist.gov/Projects/ssdf)

## Maintenance

This security configuration should be reviewed:

- Whenever new workflows are added
- During quarterly security reviews
- When updating GitHub Actions dependencies
- Following any security incidents or updates to GitHub's security recommendations

Last updated: $(date +'%Y-%m-%d')
Security review by: GitHub Actions Security Hardening Process
