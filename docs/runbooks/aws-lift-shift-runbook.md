---
metadata:
  scenario: "migration"
  cloud: "aws"
  tags: ["runbook", "lift-and-shift", "mgn", "step-by-step", "operational"]
  difficulty: "intermediate"
  execution_time: "4-6 hours"
  cross_reference: "docs/playbooks/aws-lift-shift-guide.md"
---

# AWS Lift-and-Shift Migration Runbook

## Overview

This operational runbook provides step-by-step procedures for executing AWS lift-and-shift migrations using AWS Application Migration Service (MGN). Follow these procedures to migrate on-premises workloads to AWS with minimal downtime.

## Prerequisites

- AWS CLI installed and configured
- Administrative access to source servers
- Target AWS account with appropriate permissions
- Network connectivity between source and target environments
- MGN service activated in target AWS account

## Phase 1: Environment Preparation (60 minutes)

### Step 1: Verify AWS Account Setup

```bash
# Verify AWS credentials
aws sts get-caller-identity

# Check MGN service activation
aws mgn describe-source-servers --region us-east-1

# Verify required IAM permissions
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::ACCOUNT:user/migration-user \
  --action-names mgn:*
```

### Step 2: Create Target VPC Infrastructure

```bash
# Create VPC
VPC_ID=$(aws ec2 create-vpc \
  --cidr-block 10.0.0.0/16 \
  --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=Migration-VPC}]' \
  --query 'Vpc.VpcId' --output text)

# Create subnets
SUBNET_WEB=$(aws ec2 create-subnet \
  --vpc-id $VPC_ID \
  --cidr-block 10.0.1.0/24 \
  --availability-zone us-east-1a \
  --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=Web-Subnet}]' \
  --query 'Subnet.SubnetId' --output text)

SUBNET_APP=$(aws ec2 create-subnet \
  --vpc-id $VPC_ID \
  --cidr-block 10.0.2.0/24 \
  --availability-zone us-east-1a \
  --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=App-Subnet}]' \
  --query 'Subnet.SubnetId' --output text)

# Create Internet Gateway
IGW_ID=$(aws ec2 create-internet-gateway \
  --tag-specifications 'ResourceType=internet-gateway,Tags=[{Key=Name,Value=Migration-IGW}]' \
  --query 'InternetGateway.InternetGatewayId' --output text)

aws ec2 attach-internet-gateway --vpc-id $VPC_ID --internet-gateway-id $IGW_ID
```

### Step 3: Configure Security Groups

```bash
# Create web tier security group
SG_WEB=$(aws ec2 create-security-group \
  --group-name WebTierSG \
  --description "Security group for web tier" \
  --vpc-id $VPC_ID \
  --query 'GroupId' --output text)

# Allow HTTP/HTTPS traffic
aws ec2 authorize-security-group-ingress \
  --group-id $SG_WEB \
  --protocol tcp \
  --port 80 \
  --cidr 0.0.0.0/0

aws ec2 authorize-security-group-ingress \
  --group-id $SG_WEB \
  --protocol tcp \
  --port 443 \
  --cidr 0.0.0.0/0

# Create application tier security group
SG_APP=$(aws ec2 create-security-group \
  --group-name AppTierSG \
  --description "Security group for application tier" \
  --vpc-id $VPC_ID \
  --query 'GroupId' --output text)

# Allow traffic from web tier
aws ec2 authorize-security-group-ingress \
  --group-id $SG_APP \
  --protocol tcp \
  --port 8080 \
  --source-group $SG_WEB
```

**Validation Step**: Verify VPC and security group creation

```bash
aws ec2 describe-vpcs --vpc-ids $VPC_ID
aws ec2 describe-security-groups --group-ids $SG_WEB $SG_APP
```

## Phase 2: MGN Agent Installation (45 minutes)

### Step 4: Install MGN Agent on Source Servers

**For Linux servers:**

```bash
# Download MGN agent
wget -O ./aws-replication-installer-init.py \
  https://aws-application-migration-service-us-east-1.s3.amazonaws.com/latest/linux/aws-replication-installer-init.py

# Install agent
sudo python3 aws-replication-installer-init.py \
  --region us-east-1 \
  --aws-access-key-id YOUR_ACCESS_KEY \
  --aws-secret-access-key YOUR_SECRET_KEY \
  --no-prompt
```

**For Windows servers (PowerShell):**

```powershell
# Download MGN agent
Invoke-WebRequest -Uri "https://aws-application-migration-service-us-east-1.s3.amazonaws.com/latest/windows/AwsReplicationInstaller.exe" -OutFile "AwsReplicationInstaller.exe"

# Install agent
.\AwsReplicationInstaller.exe --region us-east-1 --aws-access-key-id YOUR_ACCESS_KEY --aws-secret-access-key YOUR_SECRET_KEY --no-prompt
```

### Step 5: Verify Agent Installation

```bash
# Check agent status on source servers
sudo systemctl status aws-replication-agent

# Verify connectivity
telnet mgn.us-east-1.amazonaws.com 443

# Check AWS console for discovered servers
aws mgn describe-source-servers --region us-east-1
```

**Validation Step**: Ensure all source servers appear in MGN console

```bash
# List all source servers
aws mgn describe-source-servers --region us-east-1 --query 'Items[*].[SourceServerID,Hostname,ReplicationInfo.ReplicationStatus]' --output table
```

## Phase 3: Replication Configuration (30 minutes)

### Step 6: Configure Launch Templates

```bash
# Create launch template for web servers
aws ec2 create-launch-template \
  --launch-template-name WebServerTemplate \
  --launch-template-data '{
    "ImageId": "ami-0abcdef1234567890",
    "InstanceType": "t3.large",
    "SecurityGroupIds": ["'$SG_WEB'"],
    "SubnetId": "'$SUBNET_WEB'",
    "UserData": "IyEvYmluL2Jhc2gKc3VkbyB5dW0gdXBkYXRlIC15"
  }'

# Create launch template for app servers
aws ec2 create-launch-template \
  --launch-template-name AppServerTemplate \
  --launch-template-data '{
    "ImageId": "ami-0abcdef1234567890",
    "InstanceType": "m5.xlarge",
    "SecurityGroupIds": ["'$SG_APP'"],
    "SubnetId": "'$SUBNET_APP'"
  }'
```

### Step 7: Configure Replication Settings

```bash
# For each source server, update launch configuration
for server_id in $(aws mgn describe-source-servers --query 'Items[*].SourceServerID' --output text); do
  aws mgn update-launch-configuration \
    --source-server-id $server_id \
    --launch-template-id $(aws ec2 describe-launch-templates --launch-template-names WebServerTemplate --query 'LaunchTemplates[0].LaunchTemplateId' --output text)
done

# Configure replication settings
aws mgn update-replication-configuration \
  --source-server-id $server_id \
  --replication-settings '{
    "ReplicationServerInstanceType": "m5.large",
    "ReplicationServerSecurityGroupsIDs": ["'$SG_APP'"],
    "ReplicationServerSubnetId": "'$SUBNET_APP'"
  }'
```

**Validation Step**: Verify replication configuration

```bash
aws mgn describe-source-servers --source-server-ids $server_id --query 'Items[0].LaunchConfiguration'
```

## Phase 4: Test Migration (90 minutes)

### Step 8: Launch Test Instances

```bash
# Start test instances for all servers
for server_id in $(aws mgn describe-source-servers --query 'Items[*].SourceServerID' --output text); do
  aws mgn start-test \
    --source-server-ids $server_id
done

# Monitor test launch progress
aws mgn describe-source-servers --query 'Items[*].[SourceServerID,Hostname,LifeCycle.State]' --output table
```

### Step 9: Validate Test Environment

```bash
# Get test instance details
TEST_INSTANCES=$(aws mgn describe-source-servers --query 'Items[?LifeCycle.State==`TESTING`].LaunchedInstance.EC2InstanceID' --output text)

# Check instance status
aws ec2 describe-instances --instance-ids $TEST_INSTANCES --query 'Reservations[*].Instances[*].[InstanceId,State.Name,PrivateIpAddress]' --output table

# Test application connectivity
for instance in $TEST_INSTANCES; do
  PRIVATE_IP=$(aws ec2 describe-instances --instance-ids $instance --query 'Reservations[0].Instances[0].PrivateIpAddress' --output text)
  echo "Testing connectivity to $instance at $PRIVATE_IP"
  # Add application-specific health checks here
done
```

### Step 10: Terminate Test Instances

```bash
# Terminate test instances after validation
for server_id in $(aws mgn describe-source-servers --query 'Items[*].SourceServerID' --output text); do
  aws mgn terminate-target-instances \
    --source-server-ids $server_id
done
```

**Validation Step**: Ensure test instances are terminated

```bash
aws mgn describe-source-servers --query 'Items[*].[SourceServerID,LifeCycle.State]' --output table
```

## Phase 5: Production Cutover (45 minutes)

### Step 11: Final Replication Sync

```bash
# Ensure replication is up to date
aws mgn describe-source-servers --query 'Items[*].[SourceServerID,ReplicationInfo.ReplicationStatus,ReplicationInfo.LastReplicationDateTime]' --output table

# Wait for replication to complete
while true; do
  STATUS=$(aws mgn describe-source-servers --query 'Items[?ReplicationInfo.ReplicationStatus!=`HEALTHY`].SourceServerID' --output text)
  if [ -z "$STATUS" ]; then
    echo "All servers healthy, proceeding with cutover"
    break
  fi
  echo "Waiting for replication to complete..."
  sleep 30
done
```

### Step 12: Execute Production Cutover

```bash
# Start cutover for all servers
for server_id in $(aws mgn describe-source-servers --query 'Items[*].SourceServerID' --output text); do
  aws mgn start-cutover \
    --source-server-ids $server_id
done

# Monitor cutover progress
aws mgn describe-source-servers --query 'Items[*].[SourceServerID,Hostname,LifeCycle.State]' --output table
```

### Step 13: Post-Cutover Validation

```bash
# Verify production instances are running
PROD_INSTANCES=$(aws mgn describe-source-servers --query 'Items[?LifeCycle.State==`CUTOVER`].LaunchedInstance.EC2InstanceID' --output text)

# Check instance health
aws ec2 describe-instances --instance-ids $PROD_INSTANCES --query 'Reservations[*].Instances[*].[InstanceId,State.Name,PrivateIpAddress,PublicIpAddress]' --output table

# Update DNS records (example)
for instance in $PROD_INSTANCES; do
  PUBLIC_IP=$(aws ec2 describe-instances --instance-ids $instance --query 'Reservations[0].Instances[0].PublicIpAddress' --output text)
  # Update DNS A record for production domain
  echo "Update DNS for $instance to $PUBLIC_IP"
done
```

## Troubleshooting Common Issues

### Issue 1: MGN Agent Installation Fails

**Symptoms**: Agent installation returns error or connectivity issues
**Solution**:

```bash
# Check network connectivity
curl -I https://mgn.us-east-1.amazonaws.com
netstat -an | grep 443

# Verify IAM permissions
aws iam get-user
aws iam list-attached-user-policies --user-name migration-user
```

### Issue 2: Replication Stalls

**Symptoms**: Replication status remains "STALLED" or "INITIATING"
**Solution**:

```bash
# Check replication agent logs
sudo tail -f /var/log/aws-replication-agent.log

# Restart replication agent
sudo systemctl restart aws-replication-agent

# Check disk space and network bandwidth
df -h
iftop -i eth0
```

### Issue 3: Test Instance Launch Fails

**Symptoms**: Test instances fail to launch or remain in "PENDING" state
**Solution**:

```bash
# Check launch template configuration
aws ec2 describe-launch-templates --launch-template-names WebServerTemplate

# Verify security group rules
aws ec2 describe-security-groups --group-ids $SG_WEB

# Check subnet availability
aws ec2 describe-subnets --subnet-ids $SUBNET_WEB
```

### Issue 4: Application Connectivity Problems

**Symptoms**: Applications not responding after migration
**Solution**:

```bash
# Check security group rules
aws ec2 describe-security-groups --group-ids $SG_WEB $SG_APP

# Verify route tables
aws ec2 describe-route-tables --filters "Name=vpc-id,Values=$VPC_ID"

# Test network connectivity
telnet $PRIVATE_IP 8080
nmap -p 80,443,8080 $PRIVATE_IP
```

## Post-Migration Checklist

### Immediate Actions (within 1 hour)

- [ ] Verify all applications are accessible
- [ ] Update DNS records to point to new instances
- [ ] Validate database connectivity
- [ ] Check application logs for errors
- [ ] Confirm backup jobs are running

### Short-term Actions (within 24 hours)

- [ ] Monitor application performance
- [ ] Verify SSL certificates are working
- [ ] Test disaster recovery procedures
- [ ] Update monitoring and alerting
- [ ] Document any configuration changes

### Long-term Actions (within 1 week)

- [ ] Optimize instance sizing based on usage
- [ ] Implement cost optimization strategies
- [ ] Decommission source servers
- [ ] Update security policies
- [ ] Conduct lessons learned session

**Execution Time**: 4-6 hours total
**Success Criteria**: All applications accessible with <15 minutes downtime
**Rollback Plan**: Revert DNS to source servers if issues arise within 24 hours
