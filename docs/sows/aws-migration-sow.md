---
title: "AWS Lift-and-Shift Migration: Statement of Work"
client: "Enterprise Client"
project_type: "cloud_migration"
scenario: "migration"
cloud: "aws"
tags: ["migration", "lift-and-shift", "aws", "infrastructure", "ec2", "mgn"]
complexity: "advanced"
audience: "c-suite"
engagement_type: "sow"
estimated_duration: "12_weeks"
estimated_cost: "$150000-$200000"
last_updated: "2025-01-08"
---

# AWS Lift-and-Shift Migration: Statement of Work

## Executive Summary

This Statement of Work outlines a comprehensive lift-and-shift migration project to migrate 120 on-premises virtual machines (Windows and Linux) to Amazon Web Services (AWS) using AWS Application Migration Service (MGN). The project will modernize your infrastructure while minimizing application changes, reducing operational overhead, and enabling future cloud-native transformations.

**Project Duration:** 12 weeks
**Estimated Investment:** $150,000 - $200,000
**Expected ROI:** 35% reduction in infrastructure costs within 18 months

## Project Scope and Objectives

### Primary Objectives

- Migrate 120 virtual machines from on-premises VMware environment to AWS EC2
- Implement secure, scalable cloud infrastructure with modern networking
- Establish comprehensive monitoring and backup solutions
- Ensure minimal downtime during migration process
- Provide team training and knowledge transfer

### Deliverables

1. **Migration Assessment Report** - Detailed analysis of current infrastructure, dependencies, and migration readiness
2. **AWS Architecture Design** - Comprehensive cloud architecture with VPC design, security groups, and instance sizing
3. **Migration Plan** - Phase-by-phase migration strategy with timelines and rollback procedures
4. **Migrated Infrastructure** - Fully operational AWS environment with all workloads migrated
5. **Documentation Package** - Operational runbooks, disaster recovery procedures, and maintenance guides
6. **Training Materials** - AWS best practices training for internal teams

### Out of Scope

- Application code modifications or refactoring
- Database schema changes or optimization
- Third-party integrations requiring significant modifications
- Custom application development or enhancements

## Project Timeline and Phases

### Phase 1: Discovery and Planning (Weeks 1-3)

**Duration:** 3 weeks
**Team:** 2 Cloud Architects, 1 Migration Specialist

- Infrastructure assessment and dependency mapping
- AWS account setup and landing zone configuration
- Network architecture design and VPC planning
- Security framework implementation
- Migration tool configuration (AWS MGN)

### Phase 2: Proof of Concept (Weeks 4-5)

**Duration:** 2 weeks
**Team:** 2 Cloud Engineers, 1 Migration Specialist

- Pilot migration of 10 non-critical virtual machines
- Testing and validation of migration processes
- Performance benchmarking and optimization
- Refinement of migration procedures

### Phase 3: Production Migration (Weeks 6-10)

**Duration:** 5 weeks
**Team:** 3 Cloud Engineers, 1 Migration Specialist, 1 Project Manager

- Phased migration of remaining 110 virtual machines
- Real-time monitoring and issue resolution
- Application validation and testing
- Performance optimization and right-sizing

### Phase 4: Optimization and Handover (Weeks 11-12)

**Duration:** 2 weeks
**Team:** 2 Cloud Engineers, 1 Training Specialist

- Final optimization and cost analysis
- Team training and knowledge transfer
- Documentation completion
- Project closure and transition to operations

## Resource Requirements

### Technical Team

- **Project Manager:** 1 FTE (full project duration)
- **Senior Cloud Architects:** 2 FTEs (Weeks 1-8)
- **Cloud Engineers:** 3 FTEs (Weeks 4-12)
- **Migration Specialists:** 1 FTE (Weeks 1-10)
- **Training Specialist:** 1 FTE (Weeks 11-12)

### Client Resources Required

- **Technical Lead:** 25% allocation for project oversight
- **System Administrators:** 50% allocation during migration phases
- **Application Owners:** 25% allocation for testing and validation
- **Network Team:** 25% allocation for connectivity and security

### Technology and Tools

- AWS Application Migration Service (MGN) licenses
- AWS CloudFormation templates and automation tools
- Monitoring and alerting platforms (CloudWatch, third-party tools)
- Backup and disaster recovery solutions
- Security and compliance scanning tools

## Cost Breakdown

### Professional Services: $120,000 - $150,000

- Project management and coordination: $25,000
- Architecture design and planning: $30,000
- Migration execution and testing: $45,000
- Training and knowledge transfer: $15,000
- Documentation and handover: $10,000

### AWS Infrastructure Costs: $20,000 - $30,000

- EC2 instances (right-sized for workloads): $15,000
- Storage (EBS volumes and S3): $8,000
- Network (VPC, NAT gateways, data transfer): $5,000
- Security and monitoring tools: $3,000

### Third-Party Tools and Licenses: $10,000 - $20,000

- Migration tools and temporary licenses: $8,000
- Monitoring and backup solutions: $7,000
- Security and compliance tools: $5,000

## Risk Assessment and Mitigation

### High-Risk Areas

1. **Application Dependencies** - Undocumented interdependencies between applications
   - *Mitigation:* Comprehensive discovery phase with automated dependency mapping

2. **Network Connectivity** - Potential issues with hybrid connectivity during migration
   - *Mitigation:* Establish redundant connectivity options and thorough testing

3. **Data Loss or Corruption** - Risk during migration process
   - *Mitigation:* Comprehensive backup strategy and incremental migration approach

### Medium-Risk Areas

1. **Performance Degradation** - Applications may perform differently in cloud environment
   - *Mitigation:* Performance testing and right-sizing of instances

2. **Security Compliance** - Ensuring cloud environment meets security requirements
   - *Mitigation:* Security assessment and compliance validation at each phase

## Success Criteria

### Technical Success Metrics

- 100% of in-scope virtual machines successfully migrated
- Application availability maintained at 99.5% during migration
- Performance metrics meet or exceed current baselines
- All security and compliance requirements satisfied

### Business Success Metrics

- Project completed within agreed timeline and budget
- Team successfully trained on AWS operations
- Operational documentation delivered and approved
- Client satisfaction score of 4.5/5 or higher

## Assumptions and Dependencies

### Key Assumptions

- Current infrastructure documentation is accurate and complete
- Client team will be available for testing and validation activities
- Network connectivity requirements can be met with AWS Direct Connect or VPN
- No major application changes will be required during migration

### Critical Dependencies

- Timely access to source systems and applications
- Approval of AWS architecture and security frameworks
- Availability of skilled client resources for collaboration
- Stable network connectivity throughout migration period

## Next Steps

1. **Contract Execution** - Finalize SOW and begin project initiation
2. **Team Assembly** - Assign dedicated resources from both organizations
3. **Discovery Phase** - Commence infrastructure assessment and planning
4. **Stakeholder Alignment** - Establish regular communication and reporting cadence

This migration project represents a significant step toward modernizing your infrastructure and positioning your organization for future cloud-native transformations. Our experienced team is committed to delivering a seamless migration experience with minimal business disruption.

---

*This SOW references the [AWS Lift-and-Shift Migration Guide](../playbooks/aws-lift-shift-guide.md) for detailed technical implementation procedures.*
