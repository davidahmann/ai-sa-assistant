# CI/CD Pipeline Documentation

This document describes the CI/CD pipeline setup for the AI SA Assistant project.

## Pipeline Overview

The CI/CD pipeline consists of three main workflows:

1. **CI Pipeline** (`.github/workflows/ci.yml`) - Runs on every pull request and main branch push
2. **Security Scanning** (`.github/workflows/security.yml`) - Comprehensive security analysis
3. **Release Automation** (`.github/workflows/release.yml`) - Automated releases with semantic versioning

## Pipeline Structure

### CI Pipeline (`ci.yml`)

**Triggers**: Pull requests and pushes to main branch

**Jobs**:

- `lint` - Code quality checks (gofmt, golangci-lint)
- `test` - Unit tests with coverage (≥80% required)
- `build` - Docker image building for all services
- `integration` - Integration tests with ChromaDB
- `benchmark` - Performance benchmarking (response time < 30s)
- `demo` - Demo scenario validation
- `notify` - Failure notifications

**Services Built**:

- `ingest` - Document ingestion service
- `retrieve` - RAG retrieval service
- `websearch` - Web search service
- `synthesize` - LLM synthesis service
- `teamsbot` - Teams bot adapter

### Security Pipeline (`security.yml`)

**Triggers**: Pull requests, main branch pushes, and weekly schedule

**Jobs**:

- `govulncheck` - Go vulnerability scanning
- `dependency-check` - Dependency vulnerability analysis
- `secret-scan` - Secret detection (GitLeaks, TruffleHog)
- `sast` - Static application security testing
- `container-scan` - Container vulnerability scanning
- `license-check` - License compliance validation
- `supply-chain` - Supply chain security analysis
- `policy-check` - Security policy compliance
- `security-alert` - Security incident creation
- `security-report` - Comprehensive security reporting

### Release Pipeline (`release.yml`)

**Triggers**: Version tags (`v*`) and manual workflow dispatch

**Jobs**:

- `validate` - Release validation and version checking
- `build` - Multi-platform binary building
- `docker` - Docker image building and signing
- `release-notes` - Automated release notes generation
- `github-release` - GitHub release creation
- `update-compose` - Docker Compose version updates
- `deploy-demo` - Demo environment deployment
- `notify` - Release notifications

## Branch Protection Rules

To configure branch protection rules for the `main` branch:

1. Go to repository **Settings** → **Branches**
2. Click **Add rule** or edit existing rule for `main`
3. Configure the following settings:

### Required Status Checks

Enable **Require status checks to pass before merging** and require these checks:

**CI Pipeline**:

- `Code Quality`
- `Unit Tests`
- `Build Docker Images`
- `Integration Tests`

**Security Pipeline**:

- `Go Vulnerability Check`
- `Secret Scanning`
- `Static Code Analysis`
- `Container Security Scan`

### Branch Protection Settings

✅ **Require pull request reviews before merging**

- Required reviewers: 1
- Dismiss stale reviews when new commits are pushed

✅ **Require status checks to pass before merging**

- Require branches to be up to date before merging

✅ **Require conversation resolution before merging**

✅ **Require linear history**

✅ **Do not allow bypassing the above settings**

❌ **Allow force pushes** (disabled)

❌ **Allow deletions** (disabled)

### Additional Settings

**Restrict pushes that create files**:

- Require signed commits (recommended)
- Require deployments to succeed (if using deployment protection)

## Required Secrets

Configure these secrets in repository **Settings** → **Secrets**:

### GitHub Actions Secrets

- `GITHUB_TOKEN` - Automatically provided by GitHub
- `CODECOV_TOKEN` - For code coverage reporting (optional)
- `GITLEAKS_LICENSE` - GitLeaks license key (optional)

### Demo Environment (optional)

If deploying to demo environment:

- `DEMO_DEPLOY_KEY` - SSH key for demo deployment
- `DEMO_SERVER_HOST` - Demo server hostname
- `DEMO_SERVER_USER` - Demo server username

## Local Development Setup

### Prerequisites

1. Install Go 1.23.5
2. Install Docker and Docker Compose
3. Install pre-commit: `pip install pre-commit`
4. Install golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2`

### Setup Pre-commit Hooks

```bash
# Install pre-commit hooks
pre-commit install

# Install commit message hook
pre-commit install --hook-type commit-msg

# Run all hooks manually
pre-commit run --all-files
```

### Running Tests Locally

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker)
docker-compose up -d
go test -tags=integration ./tests/integration/...

# Demo tests
go test -tags=demo ./tests/demo/...

# Performance benchmarks
go test -bench=. ./tests/performance/...
```

### Code Quality Checks

```bash
# Format code
go fmt ./...

# Run linter
golangci-lint run

# Check test coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Pipeline Debugging

### Common Issues

**1. Test Coverage Below 80%**

```bash
# Check coverage by package
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | sort -k 3 -nr
```

**2. Linting Failures**

```bash
# Run linter with verbose output
golangci-lint run --verbose

# Fix auto-fixable issues
golangci-lint run --fix
```

**3. Docker Build Failures**

```bash
# Build specific service locally
docker build -f cmd/retrieve/Dockerfile -t retrieve:test .

# Check Docker logs
docker logs <container_id>
```

**4. Integration Test Failures**

```bash
# Check service health
curl http://localhost:8081/health
curl http://localhost:8000/api/v1/heartbeat

# Check service logs
docker-compose logs retrieve
```

### Workflow Debugging

**View workflow runs**:

- Go to repository **Actions** tab
- Click on failed workflow run
- Expand failed job steps
- Check logs and artifacts

**Re-run workflows**:

- Click **Re-run jobs** on failed workflow
- Or **Re-run failed jobs** to run only failed jobs

## Performance Targets

### Response Time Targets

- **Health checks**: < 1 second
- **Demo scenarios**: < 30 seconds
- **Unit tests**: < 5 minutes
- **Integration tests**: < 10 minutes
- **Security scans**: < 15 minutes

### Resource Limits

- **Docker images**: < 500MB per service
- **Memory usage**: < 2GB per service
- **Test coverage**: ≥ 80%
- **Build time**: < 20 minutes

## Monitoring and Alerting

### Failure Notifications

Failed pipelines trigger notifications to:

- GitHub Issues (automatic creation)
- Console logs (structured JSON)
- Workflow run annotations

### Security Alerts

Security issues trigger:

- Immediate GitHub issue creation
- Security report generation
- Artifact uploads with detailed findings

## Maintenance

### Regular Tasks

**Weekly**:

- Review security scan results
- Update dependency versions
- Check pipeline performance metrics

**Monthly**:

- Update pre-commit hook versions
- Review and update branch protection rules
- Analyze pipeline efficiency metrics

**Quarterly**:

- Review and update CI/CD pipeline
- Security tooling updates
- Performance optimization review

### Updating Dependencies

```bash
# Update Go dependencies
go get -u ./...
go mod tidy

# Update pre-commit hooks
pre-commit autoupdate

# Update GitHub Actions
# Edit .github/workflows/*.yml files and update action versions
```

## Support

For CI/CD pipeline issues:

1. Check this documentation
2. Review workflow logs in GitHub Actions
3. Test locally using the commands above
4. Create an issue with relevant logs and error messages

For security concerns:

1. Review security scan results in GitHub Actions
2. Check `.github/workflows/security.yml` for scan configuration
3. Review security report artifacts
4. Follow responsible disclosure practices
