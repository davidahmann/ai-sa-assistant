# AI SA Assistant - Docker Setup and Validation

## Overview

This document provides step-by-step procedures for deploying and validating the AI SA Assistant Docker infrastructure, with a focus on the ChromaDB vector store foundation.

## Prerequisites

- Docker and Docker Compose installed
- Network access to Docker Hub
- Available ports: 8000, 8080, 8081, 8082, 8083
- At least 2GB available disk space for volumes

## ChromaDB Container Deployment

### Step 1: Deploy ChromaDB Container

```bash
# Start ChromaDB service only
docker-compose up chromadb -d

# Verify container is running
docker ps --filter "name=chromadb"
```

Expected output:

```
CONTAINER ID   IMAGE                    STATUS                    PORTS                    NAMES
xxxxxx         chromadb/chroma:latest   Up X seconds (healthy)    0.0.0.0:8000->8000/tcp   ai-sa-assistant-chromadb-1
```

### Step 2: Validate ChromaDB API Accessibility

```bash
# Test heartbeat endpoint
curl http://localhost:8000/api/v2/heartbeat

# Expected response:
# {"nanosecond heartbeat": 1751991429948947466}
```

### Step 3: Verify Health Check Status

```bash
# Check container health status
docker inspect ai-sa-assistant-chromadb-1 --format='{{.State.Health.Status}}'

# Expected output: healthy
```

### Step 4: Test Data Persistence

```bash
# Stop and restart container
docker-compose stop chromadb
docker-compose start chromadb

# Wait for health check
sleep 35

# Verify container is healthy after restart
docker ps --filter "name=chromadb"
```

## Full Stack Deployment

### Deploy All Services

```bash
# Start all services except ingest (which runs on-demand)
docker-compose up -d

# Check all service status
docker-compose ps
```

### Service Endpoints

- **ChromaDB**: <http://localhost:8000> (Vector store)
- **Teams Bot**: <http://localhost:8080> (Main API)
- **Retrieve**: <http://localhost:8081> (Retrieval service)
- **Synthesize**: <http://localhost:8082> (Synthesis service)
- **Web Search**: <http://localhost:8083> (Web search service)

### Run Ingestion Service

```bash
# Run document ingestion (one-time or when docs change)
docker-compose --profile ingest up ingest

# Verify ingestion completed successfully
docker-compose logs ingest
```

## Data Persistence Verification

### ChromaDB Data Volume

```bash
# Check volume exists
docker volume ls | grep chromadb_data

# Inspect volume details
docker volume inspect ai-sa-assistant_chromadb_data

# Check volume mount point
docker inspect ai-sa-assistant-chromadb-1 --format='{{range .Mounts}}{{.Source}} -> {{.Destination}}{{end}}'
```

### SQLite Metadata Database

```bash
# Verify metadata.db file exists
ls -la metadata.db

# Check database file size (should be > 0 after ingestion)
du -h metadata.db
```

## Health Check Configuration

The ChromaDB service includes automated health checks:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8000/api/v2/heartbeat"]
  interval: 30s
  timeout: 10s
  retries: 3
```

### Health Check Status

- **starting**: Container is starting, health check not yet run
- **healthy**: Health check passing consistently
- **unhealthy**: Health check failing (check logs for issues)

## Service Dependencies

Services are configured with proper dependency chains:

1. **ChromaDB** (foundation) - No dependencies
2. **Retrieve & Ingest** - Depend on ChromaDB health
3. **Teams Bot** - Depends on Retrieve, Web Search, and Synthesize services

## Validation Checklist

Before proceeding with application testing:

- [ ] ChromaDB container running with status "healthy"
- [ ] ChromaDB accessible at <http://localhost:8000>
- [ ] Heartbeat endpoint returns valid JSON response
- [ ] Volume persistence verified through container restart
- [ ] All microservices started without errors
- [ ] Service dependencies resolved correctly
- [ ] Metadata database created and accessible
- [ ] No port conflicts on host system
- [ ] Container logs show successful startup messages

## Integration Testing

### Test ChromaDB Connection

```bash
# Test ChromaDB client connection from retrieve service
docker-compose exec retrieve curl -f http://chromadb:8000/api/v2/heartbeat

# Expected: {"nanosecond heartbeat": <timestamp>}
```

### Test Service Communication

```bash
# Test retrieve service health
curl http://localhost:8081/health

# Test synthesize service health
curl http://localhost:8082/health

# Test web search service health
curl http://localhost:8083/health

# Test teams bot service health
curl http://localhost:8080/health
```

## Next Steps

Once ChromaDB deployment is validated:

1. **Document Ingestion**: Run the ingest service to populate ChromaDB
2. **Service Testing**: Validate individual microservice functionality
3. **Integration Testing**: Test end-to-end workflow through Teams Bot
4. **Performance Testing**: Validate response times and throughput
5. **Monitoring Setup**: Configure logging and metrics collection

## Troubleshooting Guide

### Common ChromaDB Container Issues

#### Issue 1: Container Starts but Shows "Unhealthy" Status

**Symptoms**: Container running but health check failing

```bash
# Check specific health check logs
docker inspect ai-sa-assistant-chromadb-1 --format='{{range .State.Health.Log}}{{.Output}}{{end}}'

# Manually test health check command
docker exec ai-sa-assistant-chromadb-1 curl -f http://localhost:8000/api/v2/heartbeat
```

**Solutions**:

- Ensure curl is available in container (already included in chromadb/chroma image)
- Verify ChromaDB is listening on port 8000 inside container
- Check if v2 API endpoint is correct (should be `/api/v2/heartbeat`)

#### Issue 2: Port 8000 Already in Use

**Symptoms**: `bind: address already in use` error

```bash
# Check what's using port 8000
sudo lsof -i :8000
# or
netstat -tulpn | grep :8000
```

**Solutions**:

- Stop conflicting service: `sudo kill <PID>`
- Change ChromaDB port in docker-compose.yml: `"8001:8000"`
- Update health check and client configurations accordingly

#### Issue 3: Container Fails to Start

**Symptoms**: Container exits immediately or won't start

```bash
# Check container logs for startup errors
docker-compose logs chromadb

# Check detailed container information
docker inspect ai-sa-assistant-chromadb-1
```

**Common Causes & Solutions**:

- **Insufficient disk space**: Free up space or change volume location
- **Permission issues**: Check volume mount permissions
- **Memory constraints**: Increase Docker memory limits
- **Corrupted volume**: Remove and recreate volume

#### Issue 4: Data Not Persisting

**Symptoms**: Data lost after container restart

```bash
# Verify volume mount
docker inspect ai-sa-assistant-chromadb-1 --format='{{range .Mounts}}{{.Type}}: {{.Source}} -> {{.Destination}}{{end}}'

# Check volume exists
docker volume ls | grep chromadb_data
```

**Solutions**:

- Ensure volume is properly defined in docker-compose.yml
- Check volume mount path: `/chroma/.chroma/`
- Verify volume permissions and ownership
- Recreate volume if corrupted

#### Issue 5: ChromaDB API Errors

**Symptoms**: API requests failing or returning errors

```bash
# Test different API endpoints
curl http://localhost:8000/api/v2/heartbeat
curl http://localhost:8000/api/v2/version
curl http://localhost:8000/api/v2/collections
```

**Solutions**:

- Use v2 API endpoints (v1 is deprecated)
- Check ChromaDB container logs for specific errors
- Verify ChromaDB version compatibility
- Restart container if API becomes unresponsive

#### Issue 6: Network Connectivity Issues

**Symptoms**: Services can't connect to ChromaDB

```bash
# Test internal Docker network connectivity
docker-compose exec retrieve curl -f http://chromadb:8000/api/v2/heartbeat

# Check Docker network configuration
docker network ls
docker network inspect ai-sa-assistant_default
```

**Solutions**:

- Ensure services are on same Docker network
- Use service name `chromadb` for internal connections
- Check firewall settings if using external connections
- Verify Docker network bridge configuration

### Container Recovery Procedures

#### Quick Recovery

```bash
# Stop and remove problematic container
docker-compose down chromadb

# Remove container and restart
docker-compose up chromadb -d

# Monitor startup
docker-compose logs -f chromadb
```

#### Full Reset (Data Loss)

```bash
# Stop all services
docker-compose down

# Remove volumes (WARNING: This deletes all data)
docker volume rm ai-sa-assistant_chromadb_data

# Restart from clean state
docker-compose up chromadb -d
```

#### Volume Backup and Restore

```bash
# Backup ChromaDB data
docker run --rm -v ai-sa-assistant_chromadb_data:/data -v $(pwd):/backup alpine tar czf /backup/chromadb-backup.tar.gz -C /data .

# Restore ChromaDB data
docker run --rm -v ai-sa-assistant_chromadb_data:/data -v $(pwd):/backup alpine tar xzf /backup/chromadb-backup.tar.gz -C /data
```

### Performance Troubleshooting

#### High Memory Usage

```bash
# Monitor container resource usage
docker stats ai-sa-assistant-chromadb-1

# Check ChromaDB memory settings
docker-compose logs chromadb | grep -i memory
```

#### Slow API Responses

```bash
# Test API response times
time curl http://localhost:8000/api/v2/heartbeat

# Check container resource constraints
docker inspect ai-sa-assistant-chromadb-1 --format='{{.HostConfig.Memory}}'
```

### Getting Help

If issues persist:

1. Check [ChromaDB GitHub Issues](https://github.com/chroma-core/chroma/issues)
2. Review [ChromaDB Documentation](https://docs.trychroma.com/)
3. Consult Docker Compose logs: `docker-compose logs`
4. Check system resources: `docker system df`
5. Verify Docker installation: `docker version`

## References

- [ChromaDB Documentation](https://docs.trychroma.com/)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [ChromaDB GitHub Repository](https://github.com/chroma-core/chroma)
- Project WORK_PLAN.md Phase 1 Epic 1 Story 1.1
