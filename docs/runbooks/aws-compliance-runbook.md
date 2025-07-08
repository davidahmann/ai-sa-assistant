---
metadata:
  scenario: "security-compliance"
  cloud: "aws"
  tags: ["runbook", "compliance", "hipaa", "gdpr", "security", "operational"]
  difficulty: "advanced"
  execution_time: "4-5 hours"
  cross_reference: "docs/playbooks/security-compliance.md"
---

# AWS Security Compliance Implementation Runbook

## Overview

This operational runbook provides step-by-step procedures for implementing HIPAA, GDPR, and SOC 2 compliance controls in AWS environments. Follow these procedures to establish comprehensive security and compliance monitoring with automated remediation capabilities.

## Prerequisites

- AWS account with appropriate permissions
- AWS CLI configured with administrative access
- CloudFormation and AWS Config enabled
- S3 bucket for compliance logs
- SNS topic for security notifications
- IAM roles for compliance services

## Phase 1: Compliance Infrastructure Setup (60 minutes)

### Step 1: Create Compliance Logging Infrastructure

```bash
# Create S3 bucket for compliance logs
BUCKET_NAME="compliance-logs-$(date +%s)"
aws s3 mb s3://$BUCKET_NAME --region us-east-1

# Enable bucket versioning
aws s3api put-bucket-versioning \
  --bucket $BUCKET_NAME \
  --versioning-configuration Status=Enabled

# Configure bucket encryption
aws s3api put-bucket-encryption \
  --bucket $BUCKET_NAME \
  --server-side-encryption-configuration '{
    "Rules": [
      {
        "ApplyServerSideEncryptionByDefault": {
          "SSEAlgorithm": "AES256"
        }
      }
    ]
  }'

# Set lifecycle policy for log retention
aws s3api put-bucket-lifecycle-configuration \
  --bucket $BUCKET_NAME \
  --lifecycle-configuration '{
    "Rules": [
      {
        "ID": "ComplianceLogRetention",
        "Status": "Enabled",
        "Transitions": [
          {
            "Days": 30,
            "StorageClass": "STANDARD_IA"
          },
          {
            "Days": 90,
            "StorageClass": "GLACIER"
          },
          {
            "Days": 365,
            "StorageClass": "DEEP_ARCHIVE"
          }
        ],
        "Expiration": {
          "Days": 2555
        }
      }
    ]
  }'
```

### Step 2: Enable AWS Config for Compliance Monitoring

```bash
# Create IAM role for AWS Config
aws iam create-role \
  --role-name aws-config-role \
  --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Principal": {
          "Service": "config.amazonaws.com"
        },
        "Action": "sts:AssumeRole"
      }
    ]
  }'

# Attach managed policy to Config role
aws iam attach-role-policy \
  --role-name aws-config-role \
  --policy-arn arn:aws:iam::aws:policy/service-role/ConfigRole

# Create Config delivery channel
aws configservice put-delivery-channel \
  --delivery-channel name=default,s3BucketName=$BUCKET_NAME,s3KeyPrefix=config/

# Create Config configuration recorder
aws configservice put-configuration-recorder \
  --configuration-recorder name=default,roleARN=arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):role/aws-config-role,recordingGroup='{"allSupported":true,"includeGlobalResourceTypes":true}'

# Start Config recorder
aws configservice start-configuration-recorder --configuration-recorder-name default
```

### Step 3: Enable CloudTrail for Audit Logging

```bash
# Create CloudTrail for comprehensive audit logging
aws cloudtrail create-trail \
  --name compliance-audit-trail \
  --s3-bucket-name $BUCKET_NAME \
  --s3-key-prefix cloudtrail/ \
  --include-global-service-events \
  --is-multi-region-trail \
  --enable-log-file-validation

# Start CloudTrail logging
aws cloudtrail start-logging --name compliance-audit-trail

# Enable data events for S3 buckets
aws cloudtrail put-event-selectors \
  --trail-name compliance-audit-trail \
  --event-selectors '[
    {
      "ReadWriteType": "All",
      "IncludeManagementEvents": true,
      "DataResources": [
        {
          "Type": "AWS::S3::Object",
          "Values": ["arn:aws:s3:::*/*"]
        }
      ]
    }
  ]'
```

**Validation Step**: Verify compliance infrastructure

```bash
# Check S3 bucket configuration
aws s3api get-bucket-encryption --bucket $BUCKET_NAME
aws s3api get-bucket-versioning --bucket $BUCKET_NAME

# Verify Config is running
aws configservice describe-configuration-recorders
aws configservice describe-delivery-channels

# Check CloudTrail status
aws cloudtrail describe-trails --trail-name-list compliance-audit-trail
```

## Phase 2: HIPAA Compliance Implementation (75 minutes)

### Step 4: Configure HIPAA-Required Encryption

```bash
# Enable EBS encryption by default
aws ec2 enable-ebs-encryption-by-default

# Create customer-managed KMS key for HIPAA encryption
KMS_KEY_ID=$(aws kms create-key \
  --description "HIPAA Compliance Key" \
  --key-usage ENCRYPT_DECRYPT \
  --key-spec SYMMETRIC_DEFAULT \
  --query 'KeyMetadata.KeyId' --output text)

# Create KMS key alias
aws kms create-alias \
  --alias-name alias/hipaa-compliance-key \
  --target-key-id $KMS_KEY_ID

# Enable key rotation
aws kms enable-key-rotation --key-id $KMS_KEY_ID

# Create KMS key policy for HIPAA compliance
aws kms put-key-policy \
  --key-id $KMS_KEY_ID \
  --policy-name default \
  --policy '{
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Principal": {
          "AWS": "arn:aws:iam::'$(aws sts get-caller-identity --query Account --output text)':root"
        },
        "Action": "kms:*",
        "Resource": "*"
      },
      {
        "Effect": "Allow",
        "Principal": {
          "Service": ["s3.amazonaws.com", "ec2.amazonaws.com", "rds.amazonaws.com"]
        },
        "Action": [
          "kms:Decrypt",
          "kms:GenerateDataKey"
        ],
        "Resource": "*"
      }
    ]
  }'
```

### Step 5: Implement HIPAA Access Controls

```bash
# Create IAM policy for HIPAA minimum necessary access
aws iam create-policy \
  --policy-name HIPAA-MinimumNecessary \
  --policy-document '{
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Action": [
          "s3:GetObject",
          "s3:PutObject"
        ],
        "Resource": "arn:aws:s3:::hipaa-data-bucket/*",
        "Condition": {
          "StringEquals": {
            "s3:x-amz-server-side-encryption": "aws:kms"
          }
        }
      },
      {
        "Effect": "Deny",
        "Action": "*",
        "Resource": "*",
        "Condition": {
          "Bool": {
            "aws:SecureTransport": "false"
          }
        }
      }
    ]
  }'

# Create HIPAA-compliant security group
HIPAA_SG=$(aws ec2 create-security-group \
  --group-name HIPAA-Compliant-SG \
  --description "HIPAA-compliant security group with restricted access" \
  --vpc-id $(aws ec2 describe-vpcs --query 'Vpcs[0].VpcId' --output text) \
  --query 'GroupId' --output text)

# Allow only HTTPS traffic
aws ec2 authorize-security-group-ingress \
  --group-id $HIPAA_SG \
  --protocol tcp \
  --port 443 \
  --cidr 10.0.0.0/8

# Allow SSH from specific IP range only
aws ec2 authorize-security-group-ingress \
  --group-id $HIPAA_SG \
  --protocol tcp \
  --port 22 \
  --cidr 192.168.1.0/24
```

### Step 6: Configure HIPAA Audit Controls

```bash
# Create Config rules for HIPAA compliance
aws configservice put-config-rule \
  --config-rule '{
    "ConfigRuleName": "encrypted-volumes",
    "Description": "Checks whether EBS volumes are encrypted",
    "Source": {
      "Owner": "AWS",
      "SourceIdentifier": "ENCRYPTED_VOLUMES"
    },
    "Scope": {
      "ComplianceResourceTypes": ["AWS::EC2::Volume"]
    }
  }'

aws configservice put-config-rule \
  --config-rule '{
    "ConfigRuleName": "s3-bucket-server-side-encryption-enabled",
    "Description": "Checks that S3 buckets have server-side encryption enabled",
    "Source": {
      "Owner": "AWS",
      "SourceIdentifier": "S3_BUCKET_SERVER_SIDE_ENCRYPTION_ENABLED"
    },
    "Scope": {
      "ComplianceResourceTypes": ["AWS::S3::Bucket"]
    }
  }'

aws configservice put-config-rule \
  --config-rule '{
    "ConfigRuleName": "rds-storage-encrypted",
    "Description": "Checks whether RDS storage is encrypted",
    "Source": {
      "Owner": "AWS",
      "SourceIdentifier": "RDS_STORAGE_ENCRYPTED"
    },
    "Scope": {
      "ComplianceResourceTypes": ["AWS::RDS::DBInstance"]
    }
  }'
```

**Validation Step**: Verify HIPAA controls

```bash
# Check EBS encryption status
aws ec2 get-ebs-encryption-by-default

# Verify KMS key configuration
aws kms describe-key --key-id $KMS_KEY_ID

# Check Config rules compliance
aws configservice get-compliance-details-by-config-rule --config-rule-name encrypted-volumes
```

## Phase 3: GDPR Compliance Implementation (60 minutes)

### Step 7: Implement GDPR Data Protection Controls

```bash
# Create data retention policy for GDPR
aws s3api put-bucket-lifecycle-configuration \
  --bucket gdpr-data-bucket \
  --lifecycle-configuration '{
    "Rules": [
      {
        "ID": "GDPRDataRetention",
        "Status": "Enabled",
        "Filter": {
          "Prefix": "personal-data/"
        },
        "Expiration": {
          "Days": 2555
        }
      }
    ]
  }'

# Create Lambda function for data subject rights automation
cat > data-subject-rights.py << 'EOF'
import boto3
import json
from datetime import datetime

def lambda_handler(event, context):
    """Handle GDPR data subject rights requests"""

    request_type = event['requestType']
    subject_id = event['subjectId']

    s3_client = boto3.client('s3')

    if request_type == 'ACCESS':
        # Handle right to access
        return handle_access_request(s3_client, subject_id)
    elif request_type == 'ERASURE':
        # Handle right to erasure
        return handle_erasure_request(s3_client, subject_id)
    elif request_type == 'PORTABILITY':
        # Handle right to data portability
        return handle_portability_request(s3_client, subject_id)

    return {
        'statusCode': 400,
        'body': json.dumps('Invalid request type')
    }

def handle_access_request(s3_client, subject_id):
    """Generate data export for subject access request"""
    bucket_name = 'gdpr-data-bucket'

    # Search for subject data
    response = s3_client.list_objects_v2(
        Bucket=bucket_name,
        Prefix=f'personal-data/{subject_id}/'
    )

    data_objects = []
    if 'Contents' in response:
        for obj in response['Contents']:
            data_objects.append({
                'key': obj['Key'],
                'last_modified': obj['LastModified'].isoformat(),
                'size': obj['Size']
            })

    return {
        'statusCode': 200,
        'body': json.dumps({
            'subjectId': subject_id,
            'dataObjects': data_objects,
            'exportDate': datetime.now().isoformat()
        })
    }

def handle_erasure_request(s3_client, subject_id):
    """Handle right to erasure (right to be forgotten)"""
    bucket_name = 'gdpr-data-bucket'

    # List objects to delete
    response = s3_client.list_objects_v2(
        Bucket=bucket_name,
        Prefix=f'personal-data/{subject_id}/'
    )

    deleted_objects = []
    if 'Contents' in response:
        for obj in response['Contents']:
            s3_client.delete_object(Bucket=bucket_name, Key=obj['Key'])
            deleted_objects.append(obj['Key'])

    return {
        'statusCode': 200,
        'body': json.dumps({
            'subjectId': subject_id,
            'deletedObjects': deleted_objects,
            'deletionDate': datetime.now().isoformat()
        })
    }
EOF

# Create Lambda function
aws lambda create-function \
  --function-name gdpr-data-subject-rights \
  --runtime python3.9 \
  --role arn:aws:iam::$(aws sts get-caller-identity --query Account --output text):role/lambda-execution-role \
  --handler data-subject-rights.lambda_handler \
  --zip-file fileb://data-subject-rights.zip \
  --timeout 300
```

### Step 8: Configure GDPR Breach Notification

```bash
# Create SNS topic for GDPR breach notifications
GDPR_TOPIC=$(aws sns create-topic \
  --name gdpr-breach-notifications \
  --query 'TopicArn' --output text)

# Subscribe compliance team to breach notifications
aws sns subscribe \
  --topic-arn $GDPR_TOPIC \
  --protocol email \
  --notification-endpoint compliance@company.com

# Create CloudWatch alarm for potential data breaches
aws cloudwatch put-metric-alarm \
  --alarm-name GDPR-Potential-Breach \
  --alarm-description "Alarm for potential GDPR data breach" \
  --metric-name UnauthorizedAPICallsCount \
  --namespace AWS/CloudTrail \
  --statistic Sum \
  --period 300 \
  --evaluation-periods 2 \
  --threshold 10 \
  --comparison-operator GreaterThanThreshold \
  --alarm-actions $GDPR_TOPIC
```

**Validation Step**: Verify GDPR controls

```bash
# Test data subject rights function
aws lambda invoke \
  --function-name gdpr-data-subject-rights \
  --payload '{"requestType": "ACCESS", "subjectId": "test-subject-123"}' \
  response.json

# Check SNS topic configuration
aws sns get-topic-attributes --topic-arn $GDPR_TOPIC
```

## Phase 4: Automated Compliance Monitoring (45 minutes)

### Step 9: Deploy Security Hub and GuardDuty

```bash
# Enable Security Hub
aws securityhub enable-security-hub

# Enable AWS Foundational Security Standard
aws securityhub batch-enable-standards \
  --standards-subscription-requests StandardsArn=arn:aws:securityhub:::ruleset/finding-format/aws-foundational-security-standard/v/1.0.0

# Enable CIS AWS Foundations Benchmark
aws securityhub batch-enable-standards \
  --standards-subscription-requests StandardsArn=arn:aws:securityhub:::ruleset/finding-format/cis-aws-foundations-benchmark/v/1.2.0

# Enable GuardDuty
DETECTOR_ID=$(aws guardduty create-detector \
  --enable \
  --query 'DetectorId' --output text)

# Enable GuardDuty threat intelligence
aws guardduty create-threat-intel-set \
  --detector-id $DETECTOR_ID \
  --name "Custom-Threat-Intelligence" \
  --format TXT \
  --location s3://threat-intelligence-bucket/threat-list.txt \
  --activate
```

### Step 10: Configure Automated Remediation

```bash
# Create Lambda function for automated remediation
cat > compliance-remediation.py << 'EOF'
import boto3
import json

def lambda_handler(event, context):
    """Automated compliance remediation"""

    # Parse Security Hub finding
    finding = event['detail']['findings'][0]
    resource_id = finding['Resources'][0]['Id']
    finding_type = finding['Types'][0]

    if 'S3' in finding_type and 'encryption' in finding_type.lower():
        return remediate_s3_encryption(resource_id)
    elif 'EC2' in finding_type and 'security-group' in finding_type.lower():
        return remediate_security_group(resource_id)

    return {
        'statusCode': 200,
        'body': json.dumps('No remediation action available')
    }

def remediate_s3_encryption(bucket_arn):
    """Enable S3 bucket encryption"""
    s3_client = boto3.client('s3')
    bucket_name = bucket_arn.split(':')[-1]

    try:
        s3_client.put_bucket_encryption(
            Bucket=bucket_name,
            ServerSideEncryptionConfiguration={
                'Rules': [
                    {
                        'ApplyServerSideEncryptionByDefault': {
                            'SSEAlgorithm': 'AES256'
                        }
                    }
                ]
            }
        )
        return {'statusCode': 200, 'body': f'Enabled encryption for {bucket_name}'}
    except Exception as e:
        return {'statusCode': 500, 'body': f'Failed to enable encryption: {str(e)}'}

def remediate_security_group(sg_id):
    """Remediate overly permissive security group"""
    ec2_client = boto3.client('ec2')

    try:
        # Remove 0.0.0.0/0 rules
        ec2_client.revoke_security_group_ingress(
            GroupId=sg_id,
            IpPermissions=[
                {
                    'IpProtocol': '-1',
                    'IpRanges': [{'CidrIp': '0.0.0.0/0'}]
                }
            ]
        )
        return {'statusCode': 200, 'body': f'Remediated security group {sg_id}'}
    except Exception as e:
        return {'statusCode': 500, 'body': f'Failed to remediate: {str(e)}'}
EOF

# Create EventBridge rule for Security Hub findings
aws events put-rule \
  --name SecurityHubComplianceRule \
  --event-pattern '{
    "source": ["aws.securityhub"],
    "detail-type": ["Security Hub Findings - Imported"],
    "detail": {
      "findings": {
        "Compliance": {
          "Status": ["FAILED"]
        }
      }
    }
  }' \
  --state ENABLED
```

### Step 11: Generate Compliance Report

```bash
# Create compliance dashboard
aws cloudwatch put-dashboard \
  --dashboard-name ComplianceDashboard \
  --dashboard-body '{
    "widgets": [
      {
        "type": "metric",
        "properties": {
          "metrics": [
            ["AWS/Config", "ComplianceByConfigRule"],
            ["AWS/GuardDuty", "FindingCount"],
            ["AWS/SecurityHub", "Findings"]
          ],
          "period": 300,
          "stat": "Sum",
          "region": "us-east-1",
          "title": "Compliance Status"
        }
      }
    ]
  }'

# Generate compliance report
aws configservice get-compliance-summary-by-config-rule > compliance-report.json
echo "Compliance report generated: compliance-report.json"
```

**Validation Step**: Verify compliance monitoring

```bash
# Check Security Hub status
aws securityhub get-enabled-standards

# Verify GuardDuty is active
aws guardduty get-detector --detector-id $DETECTOR_ID

# Check Config compliance
aws configservice get-compliance-summary-by-config-rule
```

## Troubleshooting Common Issues

### Issue 1: Config Rules Not Evaluating

**Symptoms**: Config rules show "NOT_APPLICABLE" status
**Solution**:

```bash
# Check Config recorder status
aws configservice describe-configuration-recorder-status

# Restart Config recorder if needed
aws configservice stop-configuration-recorder --configuration-recorder-name default
aws configservice start-configuration-recorder --configuration-recorder-name default
```

### Issue 2: CloudTrail Logging Failures

**Symptoms**: CloudTrail shows "Logging: No" status
**Solution**:

```bash
# Check CloudTrail status
aws cloudtrail describe-trails --trail-name-list compliance-audit-trail

# Verify S3 bucket permissions
aws s3api get-bucket-policy --bucket $BUCKET_NAME

# Restart logging
aws cloudtrail start-logging --name compliance-audit-trail
```

### Issue 3: KMS Key Access Denied

**Symptoms**: Services cannot access KMS key for encryption
**Solution**:

```bash
# Check KMS key policy
aws kms get-key-policy --key-id $KMS_KEY_ID --policy-name default

# Add service permissions to key policy
aws kms put-key-policy --key-id $KMS_KEY_ID --policy-name default --policy '{...}'
```

## Post-Implementation Checklist

### Immediate Actions (within 1 hour)

- [ ] Verify all Config rules are evaluating correctly
- [ ] Test CloudTrail logging functionality
- [ ] Validate KMS key permissions and rotation
- [ ] Check Security Hub and GuardDuty are active
- [ ] Test automated remediation functions

### Short-term Actions (within 24 hours)

- [ ] Monitor compliance dashboard for alerts
- [ ] Review and validate all security controls
- [ ] Test GDPR data subject rights procedures
- [ ] Verify audit log retention and access
- [ ] Update incident response procedures

### Long-term Actions (within 1 week)

- [ ] Schedule regular compliance assessments
- [ ] Implement compliance training programs
- [ ] Document all compliance procedures
- [ ] Plan for external compliance audits
- [ ] Review and update compliance policies

**Execution Time**: 4-5 hours total
**Success Criteria**: All compliance controls active with <90% compliance score
**Compliance Standards**: HIPAA, GDPR, SOC 2 requirements met
