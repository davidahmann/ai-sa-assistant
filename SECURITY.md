# Security Policy

## Supported Versions

This project is currently in demo phase. Security updates are applied to the latest version only.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly:

### For Demo/Development Issues

- Create a GitHub issue with the `security` label
- Email the project maintainers directly if the issue is sensitive

### What to Include

- A description of the vulnerability
- Steps to reproduce the issue
- Potential impact assessment
- Suggested fixes (if any)

### Response Timeline

- Initial acknowledgment: Within 48 hours
- Security assessment: Within 1 week
- Fix deployment: Within 2 weeks for critical issues

## Security Measures

### Container Security

- All containers run as non-root users
- Minimal Alpine Linux base images with pinned versions
- Regular dependency updates
- Container image scanning in CI/CD

### Application Security

- Input validation and sanitization
- No hardcoded secrets or credentials
- Secure configuration management
- Regular security scanning with gosec and semgrep

### Infrastructure Security

- All services run in isolated containers
- Network segmentation between services
- Health checks and monitoring
- Automated security scanning in CI/CD pipeline

## Known Limitations

This is a demo project with the following security considerations:

- Not intended for production use
- Limited authentication and authorization
- Demo configurations may not follow all production security practices
- Regular security audits are not performed

## Best Practices for Contributors

1. **No Secrets in Code**: Never commit API keys, passwords, or other sensitive data
2. **Dependency Management**: Keep dependencies updated and scan for vulnerabilities
3. **Input Validation**: Always validate and sanitize user inputs
4. **Error Handling**: Don't expose sensitive information in error messages
5. **Security Testing**: Run security scans before submitting pull requests

## Security Tools Used

- **gosec**: Go security analyzer
- **semgrep**: Static analysis security testing
- **nancy**: Go dependency vulnerability scanner
- **trivy**: Container image vulnerability scanner
- **grype**: Container and filesystem vulnerability scanner

## Contact

For security-related questions or concerns, please reach out to the project maintainers.
