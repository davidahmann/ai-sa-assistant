# Build Documentation for AI SA Assistant

## Overview

This document describes the build requirements and processes for the AI SA Assistant project, with special attention to CGO compilation for SQLite support.

## CGO Build Requirements

### Services Requiring CGO

The following services require CGO compilation for SQLite support:

- **Retrieve Service** (`cmd/retrieve/`): Uses SQLite for metadata filtering
- **Ingest Service** (`cmd/ingest/`): Uses SQLite for document indexing

### Docker Build Dependencies

For services that use SQLite (CGO compilation):

```dockerfile
FROM golang:1.23.5-alpine AS builder

# Install C compiler and development libraries
RUN apk --no-cache add gcc musl-dev sqlite-dev

# Build with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /app/service .

# Runtime image needs SQLite
FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
```

### Required Alpine Packages

- **gcc**: C compiler for CGO compilation
- **musl-dev**: C library development headers
- **sqlite-dev**: SQLite development libraries
- **sqlite**: SQLite runtime library (for runtime image)

## Build Process

### Local Development

```bash
# Format code
go fmt ./...

# Run linter (requires golangci-lint)
golangci-lint run --config=.golangci.yml

# Run tests
go test -v ./...

# Build individual service
docker build -t ai-sa-retrieve -f cmd/retrieve/Dockerfile .

# Build all services
docker-compose build
```

### CI/CD Pipeline

The GitHub Actions CI pipeline includes:

1. **Code Quality**: Go formatting and linting
2. **Unit Tests**: Full test suite with coverage validation
3. **Docker Build**: All services built and validated
4. **Integration Tests**: End-to-end service communication
5. **Performance Tests**: Load testing and scaling validation
6. **Demo Validation**: All 4 demo scenarios tested

### Build Validation

The CI pipeline validates Docker builds for all services:

```yaml
strategy:
  matrix:
    service: [ingest, retrieve, websearch, synthesize, teamsbot]
steps:
  - name: Build Docker image
    uses: docker/build-push-action@v5
    with:
      context: .
      file: cmd/${{ matrix.service }}/Dockerfile
      push: false
      tags: ai-sa-assistant-${{ matrix.service }}:${{ github.sha }}
```

## Troubleshooting

### Common Build Issues

#### 1. CGO Compilation Errors

**Error**: `cgo: C compiler 'gcc' not found`

**Solution**: Ensure Dockerfile includes build dependencies:
```dockerfile
RUN apk --no-cache add gcc musl-dev sqlite-dev
```

#### 2. SQLite Runtime Errors

**Error**: `sqlite3: no such file or directory`

**Solution**: Ensure runtime image includes SQLite:
```dockerfile
FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
```

#### 3. Build Cache Issues

**Error**: Stale dependencies or build artifacts

**Solution**: Clean build without cache:
```bash
docker-compose build --no-cache
```

### Service-Specific Notes

#### Services with CGO (ingest, retrieve)
- Require build dependencies: `gcc musl-dev sqlite-dev`
- Must use `CGO_ENABLED=1`
- Runtime needs `sqlite` package
- Build time is longer due to C compilation

#### Services without CGO (websearch, synthesize, teamsbot)
- Use `CGO_ENABLED=0` for static binaries
- No build dependencies required
- Faster build times
- Smaller final images

## Dependencies

### Go Modules

Key dependencies that require CGO:
- `github.com/mattn/go-sqlite3 v1.14.22`

### Build Tools

- Go 1.23.5
- Docker & Docker Compose v2.x
- golangci-lint v1.62.2 (for CI)

## Performance Considerations

### Build Optimization

1. **Multi-stage builds**: Separate build and runtime environments
2. **Build caching**: Leverage Docker layer caching
3. **Dependency caching**: Cache Go modules between builds
4. **Parallel builds**: Use Docker Compose for concurrent builds

### Runtime Optimization

1. **Static linking**: Use `-installsuffix cgo` for CGO builds
2. **Minimal base images**: Alpine Linux for small image size
3. **Security**: Non-root user execution (future enhancement)

## Validation Scripts

### Quick Build Test

```bash
#!/bin/bash
set -e

echo "Testing Docker builds..."
docker build -t test-retrieve -f cmd/retrieve/Dockerfile .
docker build -t test-ingest -f cmd/ingest/Dockerfile .
docker-compose build --no-cache

echo "Testing CGO compilation..."
docker run --rm test-retrieve ./retrieve --version
docker run --rm test-ingest ./ingest --version

echo "✅ All builds successful"
```

### Full Validation

```bash
#!/bin/bash
set -e

echo "Running Go formatter..."
go fmt ./...

echo "Running Go linter..."
golangci-lint run ./...

echo "Running unit tests..."
go test -v ./...

echo "Building Docker images..."
docker-compose build

echo "Testing basic functionality..."
docker-compose up -d chromadb
sleep 10
docker-compose run --rm ingest --help
docker-compose run --rm retrieve --help
docker-compose down

echo "✅ All validations passed"
```

## Future Enhancements

1. **Build optimization**: Consider using scratch or distroless base images
2. **Security scanning**: Add container vulnerability scanning
3. **Multi-architecture**: Support ARM64 and AMD64 builds
4. **Build monitoring**: Add build time and size metrics
5. **Automated documentation**: Generate API docs from code

## References

- [Go CGO Documentation](https://pkg.go.dev/cmd/cgo)
- [mattn/go-sqlite3 Requirements](https://github.com/mattn/go-sqlite3#installation)
- [Docker Multi-stage Builds](https://docs.docker.com/develop/dev-best-practices/dockerfile_best-practices/#use-multi-stage-builds)
- [Alpine Linux Package Database](https://pkgs.alpinelinux.org/packages)