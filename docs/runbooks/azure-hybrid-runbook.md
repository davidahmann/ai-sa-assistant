---
metadata:
  scenario: "hybrid"
  cloud: "azure"
  tags: ["runbook", "hybrid", "expressroute", "vmware-hcx", "step-by-step", "operational"]
  difficulty: "advanced"
  execution_time: "6-8 hours"
  cross_reference: "docs/playbooks/azure-hybrid-architecture.md"
---

# Azure Hybrid Architecture Deployment Runbook

## Overview

This operational runbook provides step-by-step procedures for deploying Azure hybrid architecture connecting on-premises VMware environments to Azure using ExpressRoute and VMware HCX. Follow these procedures to establish secure, high-performance hybrid connectivity.

## Prerequisites

- Azure subscription with Global Administrator permissions
- On-premises VMware vCenter 6.7 or later
- ExpressRoute circuit provisioned by connectivity provider
- Azure CLI and PowerShell modules installed
- Network connectivity between on-premises and Azure
- VMware HCX licenses

## Phase 1: Azure Infrastructure Setup (90 minutes)

### Step 1: Create Resource Groups and Virtual Networks

```powershell
# Login to Azure
Connect-AzAccount

# Create primary resource group
New-AzResourceGroup -Name "RG-Hybrid-Primary" -Location "East US 2"

# Create hub virtual network
$hubVnet = New-AzVirtualNetwork `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -Name "Hub-VNet" `
  -AddressPrefix "10.1.0.0/16"

# Create gateway subnet
$gatewaySubnet = Add-AzVirtualNetworkSubnetConfig `
  -Name "GatewaySubnet" `
  -AddressPrefix "10.1.0.0/27" `
  -VirtualNetwork $hubVnet

# Create management subnet
$mgmtSubnet = Add-AzVirtualNetworkSubnetConfig `
  -Name "ManagementSubnet" `
  -AddressPrefix "10.1.1.0/24" `
  -VirtualNetwork $hubVnet

# Create shared services subnet
$sharedSubnet = Add-AzVirtualNetworkSubnetConfig `
  -Name "SharedServicesSubnet" `
  -AddressPrefix "10.1.2.0/24" `
  -VirtualNetwork $hubVnet

# Apply subnet configuration
$hubVnet = Set-AzVirtualNetwork -VirtualNetwork $hubVnet
```

### Step 2: Create ExpressRoute Gateway

```powershell
# Create public IP for ExpressRoute gateway
$gwpip = New-AzPublicIpAddress `
  -Name "ExpressRoute-GW-PIP" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -AllocationMethod Static `
  -Sku Standard

# Create ExpressRoute gateway configuration
$gwipconfig = New-AzVirtualNetworkGatewayIpConfig `
  -Name "ExpressRoute-GW-IPConfig" `
  -Subnet $gatewaySubnet `
  -PublicIpAddress $gwpip

# Create ExpressRoute gateway (this takes 20-30 minutes)
New-AzVirtualNetworkGateway `
  -Name "ExpressRoute-Gateway" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -IpConfigurations $gwipconfig `
  -GatewayType ExpressRoute `
  -GatewaySku Standard `
  -AsJob
```

### Step 3: Configure ExpressRoute Circuit Connection

```powershell
# Get ExpressRoute circuit details (provided by connectivity provider)
$circuit = Get-AzExpressRouteCircuit `
  -ResourceGroupName "RG-ExpressRoute" `
  -Name "CompanyExpressRoute"

# Wait for gateway creation to complete
do {
  $gateway = Get-AzVirtualNetworkGateway -ResourceGroupName "RG-Hybrid-Primary" -Name "ExpressRoute-Gateway"
  Start-Sleep -Seconds 60
  Write-Host "Waiting for gateway creation..."
} while ($gateway.ProvisioningState -ne "Succeeded")

# Create connection to ExpressRoute circuit
New-AzVirtualNetworkGatewayConnection `
  -Name "ExpressRoute-Connection" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -VirtualNetworkGateway1 $gateway `
  -PeerId $circuit.Id `
  -ConnectionType ExpressRoute
```

**Validation Step**: Verify ExpressRoute connection

```powershell
# Check connection status
Get-AzVirtualNetworkGatewayConnection -ResourceGroupName "RG-Hybrid-Primary" -Name "ExpressRoute-Connection"

# Test connectivity from on-premises
# From on-premises: ping 10.1.1.1
```

## Phase 2: VMware HCX Deployment (120 minutes)

### Step 4: Deploy HCX Cloud Manager in Azure

```powershell
# Create HCX Cloud Manager VM
$vmConfig = New-AzVMConfig -VMName "HCX-CloudManager" -VMSize "Standard_D4s_v3"

# Configure network interface
$nic = New-AzNetworkInterface `
  -Name "HCX-CM-NIC" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -SubnetId $mgmtSubnet.Id `
  -PrivateIpAddress "10.1.1.10"

# Set VM network interface
$vmConfig = Add-AzVMNetworkInterface -VM $vmConfig -NetworkInterface $nic

# Configure OS disk
$vmConfig = Set-AzVMOSDisk -VM $vmConfig -Name "HCX-CM-OSDisk" -CreateOption FromImage -StorageAccountType Premium_LRS

# Set source image (VMware HCX OVA converted to VHD)
$vmConfig = Set-AzVMSourceImage -VM $vmConfig -Id "/subscriptions/SUBSCRIPTION_ID/resourceGroups/RG-Images/providers/Microsoft.Compute/images/HCX-CloudManager-Image"

# Deploy VM
New-AzVM -ResourceGroupName "RG-Hybrid-Primary" -Location "East US 2" -VM $vmConfig
```

### Step 5: Configure HCX Cloud Manager

```bash
# SSH to HCX Cloud Manager
ssh admin@10.1.1.10

# Configure network settings
configure
set network interface eth0 ip 10.1.1.10/24
set network interface eth0 gateway 10.1.1.1
set network dns primary 168.63.129.16
commit
exit

# Access HCX Cloud Manager web interface
# Navigate to: https://10.1.1.10:9443
# Complete initial setup wizard
```

### Step 6: Install HCX Connector On-Premises

```bash
# Download HCX Connector OVA from VMware
# Deploy OVA to on-premises vCenter
# Configure HCX Connector with following settings:
# - Management IP: 10.0.0.10/24
# - Gateway: 10.0.0.1
# - DNS: 10.0.0.2
# - vCenter: vcenter.company.local
```

**Validation Step**: Verify HCX pairing

```bash
# From HCX Cloud Manager, verify site pairing
# Navigate to: Administration > Site Pairing
# Status should show "Connected"
```

## Phase 3: Network Extension Configuration (60 minutes)

### Step 7: Create Network Extension Appliance

```powershell
# Create network extension appliance
$neConfig = New-AzVMConfig -VMName "HCX-NetworkExtension" -VMSize "Standard_D2s_v3"

# Configure multiple network interfaces for network extension
$nic1 = New-AzNetworkInterface `
  -Name "HCX-NE-NIC1" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -SubnetId $mgmtSubnet.Id `
  -PrivateIpAddress "10.1.1.11"

$nic2 = New-AzNetworkInterface `
  -Name "HCX-NE-NIC2" `
  -ResourceGroupName "RG-Hybrid-Primary" `
  -Location "East US 2" `
  -SubnetId $sharedSubnet.Id `
  -PrivateIpAddress "10.1.2.11"

$neConfig = Add-AzVMNetworkInterface -VM $neConfig -NetworkInterface $nic1 -Primary
$neConfig = Add-AzVMNetworkInterface -VM $neConfig -NetworkInterface $nic2
```

### Step 8: Configure Network Extension

```bash
# From HCX Cloud Manager web interface:
# 1. Navigate to Infrastructure > Network Extension
# 2. Create new network extension
# 3. Select on-premises network: "Production-Network (10.0.1.0/24)"
# 4. Select Azure destination network: "SharedServicesSubnet"
# 5. Configure extension settings:
#    - Gateway IP: 10.0.1.1
#    - Enable reverse lookup: Yes
#    - Enable ARP suppression: Yes
```

**Validation Step**: Test network extension

```bash
# From on-premises VM, test connectivity to Azure
ping 10.1.2.10

# From Azure VM, test connectivity to on-premises
ping 10.0.1.10
```

## Phase 4: VM Migration Setup (45 minutes)

### Step 9: Configure Migration Appliance

```powershell
# Create migration appliance VM
$migConfig = New-AzVMConfig -VMName "HCX-Migration" -VMSize "Standard_D8s_v3"

# Configure storage for migration cache
$migConfig = Set-AzVMOSDisk -VM $migConfig -Name "HCX-Migration-OSDisk" -CreateOption FromImage -StorageAccountType Premium_LRS

# Add data disk for migration cache
$migConfig = Add-AzVMDataDisk -VM $migConfig -Name "HCX-Migration-Cache" -CreateOption Empty -DiskSizeInGB 500 -StorageAccountType Premium_LRS -Lun 0

# Deploy migration appliance
New-AzVM -ResourceGroupName "RG-Hybrid-Primary" -Location "East US 2" -VM $migConfig
```

### Step 10: Execute Test Migration

```bash
# From HCX Cloud Manager:
# 1. Navigate to Migration > Migrate VMs
# 2. Select test VM from on-premises
# 3. Configure migration settings:
#    - Migration type: Bulk Migration
#    - Destination: Azure Resource Group
#    - Compute profile: Standard_D2s_v3
#    - Storage profile: Premium_LRS
#    - Network: Extended network
# 4. Schedule migration for maintenance window
```

**Validation Step**: Verify test migration

```bash
# Check VM status in Azure
az vm show --resource-group "RG-Hybrid-Primary" --name "Test-VM-Migrated" --query "provisioningState"

# Test application connectivity
curl -I http://10.1.2.50:8080/health
```

## Phase 5: Production Migration (120 minutes)

### Step 11: Execute Production Migration Batch

```bash
# From HCX Cloud Manager:
# 1. Create migration batch for production VMs
# 2. Configure migration schedule:
#    - Batch 1: Web servers (2 VMs) - 6 PM
#    - Batch 2: Application servers (4 VMs) - 8 PM
#    - Batch 3: Database servers (2 VMs) - 10 PM
# 3. Enable pre-migration replication
# 4. Monitor migration progress
```

### Step 12: Post-Migration Validation

```powershell
# Verify all migrated VMs are running
Get-AzVM -ResourceGroupName "RG-Hybrid-Primary" -Status | Where-Object {$_.PowerState -eq "VM running"}

# Check application health endpoints
$webServers = @("10.1.2.20", "10.1.2.21")
$appServers = @("10.1.2.30", "10.1.2.31", "10.1.2.32", "10.1.2.33")

foreach ($server in $webServers) {
  try {
    $response = Invoke-WebRequest -Uri "http://$server/health" -UseBasicParsing
    Write-Host "Web server $server: $($response.StatusCode)"
  } catch {
    Write-Error "Web server $server: Failed"
  }
}

foreach ($server in $appServers) {
  $result = Test-NetConnection -ComputerName $server -Port 8080
  Write-Host "App server $server port 8080: $($result.TcpTestSucceeded)"
}
```

## Troubleshooting Common Issues

### Issue 1: ExpressRoute Connection Failed

**Symptoms**: ExpressRoute connection shows "Failed" status
**Solution**:

```powershell
# Check circuit provisioning state
Get-AzExpressRouteCircuit -ResourceGroupName "RG-ExpressRoute" -Name "CompanyExpressRoute"

# Verify BGP peering status
Get-AzExpressRouteCircuitPeeringConfig -ExpressRouteCircuit $circuit

# Check gateway configuration
Get-AzVirtualNetworkGateway -ResourceGroupName "RG-Hybrid-Primary" -Name "ExpressRoute-Gateway"
```

### Issue 2: HCX Site Pairing Issues

**Symptoms**: HCX sites cannot pair or show "Disconnected" status
**Solution**:

```bash
# Check HCX Cloud Manager logs
tail -f /var/log/vmware/hcx/hcx-cloud-manager.log

# Verify network connectivity
ping hcx-connector.company.local
telnet hcx-connector.company.local 443

# Check firewall rules for HCX ports
netstat -tuln | grep -E "(443|8043|9443)"
```

### Issue 3: Network Extension Connectivity Problems

**Symptoms**: Extended networks cannot communicate between sites
**Solution**:

```bash
# Check network extension appliance status
# From HCX Cloud Manager: Infrastructure > Network Extension
# Verify appliance status shows "Connected"

# Test Layer 2 connectivity
ping -c 4 10.0.1.10
arping -c 4 10.0.1.10

# Check routing table
ip route show | grep 10.0.1.0
```

### Issue 4: VM Migration Failures

**Symptoms**: VM migration fails or stalls during transfer
**Solution**:

```bash
# Check migration appliance resources
# From HCX Cloud Manager: Administration > System Health
# Verify CPU, memory, and disk usage

# Check migration logs
tail -f /var/log/vmware/hcx/migration.log

# Verify storage performance
iostat -x 1 10
```

## Post-Migration Checklist

### Immediate Actions (within 1 hour)

- [ ] Verify all migrated VMs are running
- [ ] Test application connectivity and functionality
- [ ] Validate network extension is working
- [ ] Check DNS resolution for migrated workloads
- [ ] Confirm backup jobs are configured

### Short-term Actions (within 24 hours)

- [ ] Monitor application performance metrics
- [ ] Update load balancer configurations
- [ ] Test disaster recovery procedures
- [ ] Configure Azure monitoring and alerting
- [ ] Update network security groups

### Long-term Actions (within 1 week)

- [ ] Optimize VM sizing based on Azure metrics
- [ ] Implement Azure-native services where applicable
- [ ] Configure cost management and budgets
- [ ] Update security and compliance policies
- [ ] Decommission successfully migrated on-premises resources

**Execution Time**: 6-8 hours total
**Success Criteria**: All applications accessible with network extension functional
**Rollback Plan**: Use HCX reverse migration within 72 hours if issues arise
