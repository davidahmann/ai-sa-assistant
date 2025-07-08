---
metadata:
  scenario: "disaster-recovery"
  cloud: "azure"
  tags: ["runbook", "disaster-recovery", "site-recovery", "rto", "rpo", "operational"]
  difficulty: "advanced"
  execution_time: "3-4 hours"
  cross_reference: "docs/playbooks/azure-disaster-recovery.md"
---

# Azure Disaster Recovery Deployment Runbook

## Overview

This operational runbook provides step-by-step procedures for deploying Azure Site Recovery (ASR) for disaster recovery with RTO=2 hours and RPO=15 minutes targets. Follow these procedures to establish comprehensive DR protection for critical workloads.

## Prerequisites

- Azure subscription with Contributor permissions
- Source and target resource groups created
- Network connectivity between primary and secondary regions
- Azure PowerShell modules installed
- Recovery Services Vault deployed
- Virtual machines to be protected

## Phase 1: Recovery Services Vault Setup (30 minutes)

### Step 1: Create and Configure Recovery Services Vault

```powershell
# Login to Azure
Connect-AzAccount

# Create Recovery Services Vault
$vault = New-AzRecoveryServicesVault `
  -ResourceGroupName "RG-ASR-Primary" `
  -Name "ASR-Vault-EastUS2" `
  -Location "East US 2"

# Set vault context
Set-AzRecoveryServicesAsrVaultContext -Vault $vault

# Configure vault backup properties
Set-AzRecoveryServicesVaultProperty `
  -Vault $vault `
  -SoftDeleteFeatureState Enable

# Set backup storage redundancy
Set-AzRecoveryServicesBackupProperty `
  -Vault $vault `
  -BackupStorageRedundancy LocallyRedundant
```

### Step 2: Configure Replication Policy

```powershell
# Create replication policy for RTO=2h, RPO=15min
$policy = New-AzRecoveryServicesAsrPolicy `
  -Name "Critical-Workload-Policy" `
  -ReplicationProvider "AzureToAzure" `
  -RecoveryPointRetentionInHours 24 `
  -ApplicationConsistentSnapshotFrequencyInHours 1 `
  -MultiVmSyncStatus Enable

# Configure recovery point retention
Set-AzRecoveryServicesAsrPolicy `
  -Policy $policy `
  -RecoveryPointRetentionInHours 48 `
  -ApplicationConsistentSnapshotFrequencyInHours 2
```

### Step 3: Prepare Target Infrastructure

```powershell
# Create target resource group
New-AzResourceGroup -Name "RG-DR-WestUS2" -Location "West US 2"

# Create target virtual network
$targetVnet = New-AzVirtualNetwork `
  -ResourceGroupName "RG-DR-WestUS2" `
  -Location "West US 2" `
  -Name "DR-VNet" `
  -AddressPrefix "10.1.0.0/16"

# Create target subnets
$targetSubnet1 = Add-AzVirtualNetworkSubnetConfig `
  -Name "DR-Database-Subnet" `
  -AddressPrefix "10.1.1.0/24" `
  -VirtualNetwork $targetVnet

$targetSubnet2 = Add-AzVirtualNetworkSubnetConfig `
  -Name "DR-Application-Subnet" `
  -AddressPrefix "10.1.2.0/24" `
  -VirtualNetwork $targetVnet

$targetSubnet3 = Add-AzVirtualNetworkSubnetConfig `
  -Name "DR-Web-Subnet" `
  -AddressPrefix "10.1.3.0/24" `
  -VirtualNetwork $targetVnet

# Apply subnet configuration
$targetVnet = Set-AzVirtualNetwork -VirtualNetwork $targetVnet
```

**Validation Step**: Verify vault and target infrastructure

```powershell
# Check vault status
Get-AzRecoveryServicesVault -ResourceGroupName "RG-ASR-Primary"

# Verify target network
Get-AzVirtualNetwork -ResourceGroupName "RG-DR-WestUS2"
```

## Phase 2: VM Protection Configuration (45 minutes)

### Step 4: Enable Protection for Critical VMs

```powershell
# Get source VMs to protect
$sourceVMs = @(
  "Database-Server-01",
  "Database-Server-02",
  "App-Server-01",
  "App-Server-02",
  "Web-Server-01",
  "Web-Server-02"
)

# Configure protection for each VM
foreach ($vmName in $sourceVMs) {
  $vm = Get-AzVM -ResourceGroupName "RG-Production" -Name $vmName

  # Determine target subnet based on VM tier
  $targetSubnet = switch ($vmName) {
    {$_ -like "Database-*"} { $targetSubnet1.Id }
    {$_ -like "App-*"} { $targetSubnet2.Id }
    {$_ -like "Web-*"} { $targetSubnet3.Id }
  }

  # Enable protection
  $job = New-AzRecoveryServicesAsrReplicationProtectedItem `
    -AzureToAzure `
    -AzureVmId $vm.Id `
    -Name $vmName `
    -ProtectionContainerMapping $containerMapping `
    -RecoveryResourceGroupId "/subscriptions/SUBSCRIPTION_ID/resourceGroups/RG-DR-WestUS2" `
    -RecoveryCloudServiceId $targetSubnet `
    -RecoveryVmName "$vmName-DR" `
    -RecoveryVmSize $vm.HardwareProfile.VmSize `
    -AsJob

  Write-Host "Enabled protection for $vmName"
}
```

### Step 5: Configure Network Security Groups for DR

```powershell
# Create network security group for DR environment
$drNsg = New-AzNetworkSecurityGroup `
  -ResourceGroupName "RG-DR-WestUS2" `
  -Location "West US 2" `
  -Name "DR-NSG"

# Add security rules for web tier
$webRule = New-AzNetworkSecurityRuleConfig `
  -Name "Allow-HTTP-HTTPS" `
  -Description "Allow HTTP and HTTPS traffic" `
  -Access Allow `
  -Protocol Tcp `
  -Direction Inbound `
  -Priority 100 `
  -SourceAddressPrefix Internet `
  -SourcePortRange * `
  -DestinationAddressPrefix * `
  -DestinationPortRange 80,443

$appRule = New-AzNetworkSecurityRuleConfig `
  -Name "Allow-App-Traffic" `
  -Description "Allow application traffic from web tier" `
  -Access Allow `
  -Protocol Tcp `
  -Direction Inbound `
  -Priority 110 `
  -SourceAddressPrefix "10.1.3.0/24" `
  -SourcePortRange * `
  -DestinationAddressPrefix "10.1.2.0/24" `
  -DestinationPortRange 8080

# Apply security rules
$drNsg = Set-AzNetworkSecurityGroup -NetworkSecurityGroup $drNsg
```

### Step 6: Monitor Initial Replication

```powershell
# Monitor replication status
do {
  $replicationItems = Get-AzRecoveryServicesAsrReplicationProtectedItem
  $initializing = $replicationItems | Where-Object {$_.ReplicationHealth -eq "Normal" -and $_.ReplicationState -eq "EnableProtection"}

  if ($initializing.Count -eq 0) {
    Write-Host "Initial replication completed for all VMs"
    break
  }

  Write-Host "Waiting for initial replication to complete for $($initializing.Count) VMs..."
  Start-Sleep -Seconds 60
} while ($true)
```

**Validation Step**: Verify protection status

```powershell
# Check protection status for all VMs
Get-AzRecoveryServicesAsrReplicationProtectedItem | Select-Object Name, ReplicationHealth, ReplicationState, LastReplicationTime
```

## Phase 3: DR Testing and Validation (60 minutes)

### Step 7: Execute Test Failover

```powershell
# Get recovery plan or create one
$recoveryPlan = Get-AzRecoveryServicesAsrRecoveryPlan -Name "Production-DR-Plan"

# If recovery plan doesn't exist, create it
if (-not $recoveryPlan) {
  $recoveryPlan = New-AzRecoveryServicesAsrRecoveryPlan `
    -Name "Production-DR-Plan" `
    -PrimaryFabric $primaryFabric `
    -RecoveryFabric $recoveryFabric `
    -ReplicationProtectedItem $replicationItems
}

# Start test failover
$testFailoverJob = Start-AzRecoveryServicesAsrTestFailoverJob `
  -RecoveryPlan $recoveryPlan `
  -Direction PrimaryToRecovery `
  -VMNetwork $targetVnet

# Monitor test failover progress
do {
  $job = Get-AzRecoveryServicesAsrJob -Job $testFailoverJob
  Write-Host "Test failover status: $($job.State) - $($job.StateDescription)"

  if ($job.State -eq "Succeeded") {
    Write-Host "Test failover completed successfully"
    break
  } elseif ($job.State -eq "Failed") {
    Write-Error "Test failover failed: $($job.StateDescription)"
    break
  }

  Start-Sleep -Seconds 30
} while ($job.State -eq "InProgress")
```

### Step 8: Validate DR Environment

```powershell
# Get test failover VMs
$testVMs = Get-AzVM -ResourceGroupName "RG-DR-WestUS2" | Where-Object {$_.Name -like "*-Test"}

# Check VM status
foreach ($vm in $testVMs) {
  $vmStatus = Get-AzVM -ResourceGroupName $vm.ResourceGroupName -Name $vm.Name -Status
  Write-Host "VM $($vm.Name): $($vmStatus.Statuses[1].DisplayStatus)"
}

# Test application connectivity
$webTestVM = $testVMs | Where-Object {$_.Name -like "Web-Server-*"}
if ($webTestVM) {
  $webIP = (Get-AzNetworkInterface -ResourceGroupName "RG-DR-WestUS2" | Where-Object {$_.VirtualMachine.Id -eq $webTestVM.Id}).IpConfigurations[0].PrivateIpAddress

  try {
    $response = Invoke-WebRequest -Uri "http://$webIP" -UseBasicParsing -TimeoutSec 30
    Write-Host "Web application test: SUCCESS (Status: $($response.StatusCode))"
  } catch {
    Write-Warning "Web application test: FAILED ($($_.Exception.Message))"
  }
}
```

### Step 9: Cleanup Test Environment

```powershell
# Cleanup test failover
$cleanupJob = Start-AzRecoveryServicesAsrTestFailoverCleanupJob `
  -RecoveryPlan $recoveryPlan `
  -Comments "Test failover validation completed successfully"

# Wait for cleanup completion
do {
  $job = Get-AzRecoveryServicesAsrJob -Job $cleanupJob
  Start-Sleep -Seconds 15
} while ($job.State -eq "InProgress")

Write-Host "Test failover cleanup completed"
```

**Validation Step**: Verify test cleanup

```powershell
# Ensure test VMs are removed
$remainingTestVMs = Get-AzVM -ResourceGroupName "RG-DR-WestUS2" | Where-Object {$_.Name -like "*-Test"}
if ($remainingTestVMs.Count -eq 0) {
  Write-Host "Test environment cleaned up successfully"
}
```

## Phase 4: Production Failover Procedures (45 minutes)

### Step 10: Execute Production Failover

```powershell
# Pre-failover validation
$replicationItems = Get-AzRecoveryServicesAsrReplicationProtectedItem
$unhealthyItems = $replicationItems | Where-Object {$_.ReplicationHealth -ne "Normal"}

if ($unhealthyItems.Count -gt 0) {
  Write-Warning "The following items are not healthy for failover:"
  $unhealthyItems | Select-Object Name, ReplicationHealth, ReplicationState

  # Prompt for confirmation
  $continue = Read-Host "Continue with failover? (y/n)"
  if ($continue -ne "y") {
    Write-Host "Failover cancelled"
    return
  }
}

# Start planned failover
$failoverJob = Start-AzRecoveryServicesAsrPlannedFailoverJob `
  -RecoveryPlan $recoveryPlan `
  -Direction PrimaryToRecovery `
  -Optimize ForDowntime

# Monitor failover progress
$startTime = Get-Date
do {
  $job = Get-AzRecoveryServicesAsrJob -Job $failoverJob
  $elapsed = (Get-Date) - $startTime

  Write-Host "Failover status: $($job.State) - Elapsed: $($elapsed.ToString('mm\:ss'))"

  if ($job.State -eq "Succeeded") {
    Write-Host "Production failover completed successfully"
    break
  } elseif ($job.State -eq "Failed") {
    Write-Error "Production failover failed: $($job.StateDescription)"
    break
  }

  Start-Sleep -Seconds 30
} while ($job.State -eq "InProgress")
```

### Step 11: Update DNS and Load Balancer

```powershell
# Update DNS records to point to DR environment
$dnsRecords = @(
  @{Name = "www"; IP = "10.1.3.10"},
  @{Name = "api"; IP = "10.1.2.10"},
  @{Name = "db"; IP = "10.1.1.10"}
)

# Update Azure DNS zones
$dnsZone = Get-AzDnsZone -ResourceGroupName "RG-DNS" -Name "company.com"

foreach ($record in $dnsRecords) {
  $dnsRecord = Get-AzDnsRecordSet -ZoneName $dnsZone.Name -ResourceGroupName "RG-DNS" -Name $record.Name -RecordType A
  $dnsRecord.Records[0].IPv4Address = $record.IP
  Set-AzDnsRecordSet -RecordSet $dnsRecord
  Write-Host "Updated DNS record $($record.Name) to $($record.IP)"
}

# Update load balancer backend pool
$lb = Get-AzLoadBalancer -ResourceGroupName "RG-LoadBalancer" -Name "Production-LB"
$backendPool = Get-AzLoadBalancerBackendAddressPoolConfig -LoadBalancer $lb -Name "BackendPool"

# Remove old backend addresses and add new ones
$backendPool.BackendAddresses.Clear()
$backendPool.BackendAddresses.Add(@{Name = "DR-Web-01"; IpAddress = "10.1.3.10"})
$backendPool.BackendAddresses.Add(@{Name = "DR-Web-02"; IpAddress = "10.1.3.11"})

Set-AzLoadBalancer -LoadBalancer $lb
```

### Step 12: Post-Failover Validation

```powershell
# Verify all DR VMs are running
$drVMs = Get-AzVM -ResourceGroupName "RG-DR-WestUS2" -Status | Where-Object {$_.PowerState -eq "VM running"}
Write-Host "DR VMs running: $($drVMs.Count)"

# Test application endpoints
$endpoints = @(
  @{Name = "Web"; URL = "http://www.company.com/health"},
  @{Name = "API"; URL = "http://api.company.com/status"},
  @{Name = "Database"; Port = 1433; Server = "db.company.com"}
)

foreach ($endpoint in $endpoints) {
  if ($endpoint.URL) {
    try {
      $response = Invoke-WebRequest -Uri $endpoint.URL -UseBasicParsing -TimeoutSec 30
      Write-Host "$($endpoint.Name) endpoint: OK (Status: $($response.StatusCode))"
    } catch {
      Write-Warning "$($endpoint.Name) endpoint: FAILED"
    }
  } else {
    $connection = Test-NetConnection -ComputerName $endpoint.Server -Port $endpoint.Port
    Write-Host "$($endpoint.Name) connection: $($connection.TcpTestSucceeded)"
  }
}
```

**Validation Step**: Verify RTO achievement

```powershell
# Calculate RTO
$rtoTarget = 120 # 2 hours in minutes
$actualRTO = ($job.EndTime - $job.StartTime).TotalMinutes
Write-Host "RTO Target: $rtoTarget minutes"
Write-Host "Actual RTO: $actualRTO minutes"

if ($actualRTO -le $rtoTarget) {
  Write-Host "✓ RTO target achieved"
} else {
  Write-Warning "✗ RTO target exceeded by $($actualRTO - $rtoTarget) minutes"
}
```

## Troubleshooting Common Issues

### Issue 1: Initial Replication Stuck

**Symptoms**: VM replication status shows "Synchronizing" for extended period
**Solution**:

```powershell
# Check replication health
Get-AzRecoveryServicesAsrReplicationProtectedItem | Where-Object {$_.ReplicationState -eq "Synchronizing"}

# Restart replication
$stuckItem = Get-AzRecoveryServicesAsrReplicationProtectedItem -Name "VM-Name"
Start-AzRecoveryServicesAsrResynchronizeReplicationJob -ReplicationProtectedItem $stuckItem
```

### Issue 2: Test Failover Failures

**Symptoms**: Test failover fails with network connectivity issues
**Solution**:

```powershell
# Check network security group rules
Get-AzNetworkSecurityGroup -ResourceGroupName "RG-DR-WestUS2" | Get-AzNetworkSecurityRuleConfig

# Verify subnet configuration
Get-AzVirtualNetwork -ResourceGroupName "RG-DR-WestUS2" | Get-AzVirtualNetworkSubnetConfig
```

### Issue 3: RPO Threshold Exceeded

**Symptoms**: Last replication time exceeds 15 minutes
**Solution**:

```powershell
# Check replication lag
Get-AzRecoveryServicesAsrReplicationProtectedItem | Select-Object Name, LastReplicationTime, ReplicationHealth

# Verify network bandwidth between regions
Test-NetConnection -ComputerName "asr-prod-cache-eastus2.backup.windowsazure.com" -Port 443
```

## Post-Failover Checklist

### Immediate Actions (within 30 minutes)

- [ ] Verify all critical VMs are running in DR region
- [ ] Test application functionality and performance
- [ ] Validate database connectivity and integrity
- [ ] Confirm DNS resolution is working correctly
- [ ] Check load balancer health probes

### Short-term Actions (within 4 hours)

- [ ] Monitor application performance metrics
- [ ] Verify backup jobs are running in DR environment
- [ ] Test user authentication and authorization
- [ ] Validate external integrations and API connections
- [ ] Update monitoring and alerting configurations

### Long-term Actions (within 24 hours)

- [ ] Plan failback procedures to primary region
- [ ] Document lessons learned and improvement areas
- [ ] Review and update DR procedures
- [ ] Conduct stakeholder communication and updates
- [ ] Schedule post-incident review meeting

**Execution Time**: 3-4 hours total
**RTO Target**: 2 hours maximum
**RPO Target**: 15 minutes maximum
**Success Criteria**: All applications accessible within RTO/RPO targets
