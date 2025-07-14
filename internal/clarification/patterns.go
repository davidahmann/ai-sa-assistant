package clarification

import (
	"regexp"
	"strings"

	"github.com/your-org/ai-sa-assistant/internal/session"
)

// buildAmbiguityPatterns builds patterns for detecting ambiguous queries
func buildAmbiguityPatterns() []*regexp.Regexp {
	patterns := []string{
		`(?i)^(help|assist|support)(\s+me)?$`,
		`(?i)^(how|what|which|when|where|why)\s+(do|should|can|is|are).*\?$`,
		`(?i)\b(best|good|better|simple|easy|quick|basic|general|overview)\b`,
		`(?i)^(migrate|move|transfer|deploy|implement|setup|configure)(\s+to|\s+from)?$`,
		`(?i)^(security|compliance|audit|assessment)(\s+plan|\s+strategy)?$`,
		`(?i)^(architecture|design|solution|approach|strategy)(\s+for)?$`,
		`(?i)^(cloud|aws|azure|gcp|google\s+cloud)(\s+migration|\s+deployment)?$`,
		`(?i)^(disaster\s+recovery|backup|dr)(\s+plan|\s+strategy)?$`,
		`(?i)^(cost|pricing|budget|optimization)(\s+analysis|\s+planning)?$`,
		`(?i)\b(something|anything|everything|somewhere|anywhere)\b`,
		`(?i)\b(stuff|things|issues|problems|challenges)\b`,
		`(?i)\b(typically|usually|generally|commonly|normally)\b`,
	}

	var compiledPatterns []*regexp.Regexp
	for _, pattern := range patterns {
		if compiled, err := regexp.Compile(pattern); err == nil {
			compiledPatterns = append(compiledPatterns, compiled)
		}
	}

	return compiledPatterns
}

// buildIncompletenessRules builds rules for detecting incomplete queries
func buildIncompletenessRules() []incompletenessRule {
	rules := []incompletenessRule{
		{
			Pattern:     regexp.MustCompile(`(?i)^(help|assist|support)(\s+me)?$`),
			Weight:      0.8,
			Description: "Query is too general - just asking for help",
			Category:    "general",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)^(how|what|which|when|where|why)\s+\w+\?$`),
			Weight:      0.6,
			Description: "Query lacks specific context",
			Category:    "context",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(cloud|aws|azure|gcp)\b.*\?$`),
			Weight:      0.4,
			Description: "Cloud query without specific requirements",
			Category:    "requirements",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(migrate|move|transfer)\b.*\b(cloud|aws|azure|gcp)\b`),
			Weight:      0.3,
			Description: "Migration query lacks specific source environment details",
			Category:    "migration",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(security|compliance)(\s+plan|\s+assessment)?\b`),
			Weight:      0.5,
			Description: "Security query without compliance or threat context",
			Category:    "security",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)^(architecture|design)(\s+for)?$`),
			Weight:      0.6,
			Description: "Architecture query without requirements",
			Category:    "architecture",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)^(cost|pricing|budget)(\s+analysis)?$`),
			Weight:      0.5,
			Description: "Cost query without workload specification",
			Category:    "cost",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(best|good|better|simple|easy)\b`),
			Weight:      0.3,
			Description: "Query uses subjective terms without criteria",
			Category:    "subjective",
		},
		{
			Pattern:     regexp.MustCompile(`(?i)^.{1,15}\b(help|assist|support)\b.{0,15}$`),
			Weight:      0.6,
			Description: "Very short help request without context",
			Category:    "general",
		},
	}

	return rules
}

// buildIntentClassifiers builds classifiers for detecting query intent
func buildIntentClassifiers() []intentClassifier {
	classifiers := []intentClassifier{
		{
			Intent: "migration",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(migrat|move|transfer|lift\s+and\s+shift|rehost|replatform|refactor)\b`),
				regexp.MustCompile(`(?i)\b(on-prem|on-premises|datacenter|data\s+center)\b.*\b(cloud|aws|azure|gcp)\b`),
				regexp.MustCompile(`(?i)\b(vmware|hyper-v|physical\s+servers?)\b.*\b(cloud|aws|azure|gcp)\b`),
			},
			Keywords: []string{"migration", "migrate", "move", "transfer", "lift and shift", "rehost", "replatform"},
			Weight:   0.9,
		},
		{
			Intent: "security",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(security|secure|compliance|audit|assessment|penetration\s+test)\b`),
				regexp.MustCompile(`(?i)\b(hipaa|gdpr|sox|pci|dss|iso\s+27001|nist)\b`),
				regexp.MustCompile(`(?i)\b(threat|risk|vulnerability|attack|breach|incident)\b`),
				regexp.MustCompile(`(?i)\b(encryption|identity|access|authentication|authorization)\b`),
			},
			Keywords: []string{"security", "compliance", "audit", "assessment", "threat", "risk", "vulnerability"},
			Weight:   0.9,
		},
		{
			Intent: "architecture",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(architecture|design|solution|approach|pattern|framework)\b`),
				regexp.MustCompile(`(?i)\b(microservices|serverless|containers|kubernetes|docker)\b`),
				regexp.MustCompile(`(?i)\b(load\s+balancer|database|storage|networking|vpc|subnet)\b`),
				regexp.MustCompile(`(?i)\b(scalability|availability|reliability|performance|latency)\b`),
			},
			Keywords: []string{"architecture", "design", "solution", "approach", "pattern", "framework"},
			Weight:   0.8,
		},
		{
			Intent: "disaster_recovery",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(disaster\s+recovery|dr|backup|restore|failover|business\s+continuity)\b`),
				regexp.MustCompile(`(?i)\b(rto|recovery\s+time|rpo|recovery\s+point|downtime|outage)\b`),
				regexp.MustCompile(`(?i)\b(replication|snapshot|backup|archive|vault)\b`),
			},
			Keywords: []string{"disaster recovery", "dr", "backup", "restore", "failover", "business continuity"},
			Weight:   0.9,
		},
		{
			Intent: "cost_optimization",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(cost|price|pricing|budget|spend|expense|bill|invoice)\b`),
				regexp.MustCompile(`(?i)\b(optimiz|reduc|save|cut|lower|cheap|econom)\b`),
				regexp.MustCompile(`(?i)\b(reserved\s+instance|spot\s+instance|savings\s+plan|commitment)\b`),
			},
			Keywords: []string{"cost", "pricing", "budget", "optimization", "savings", "reduce", "save"},
			Weight:   0.8,
		},
		{
			Intent: "monitoring",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(monitor|observ|log|metric|alert|dashboard|trace)\b`),
				regexp.MustCompile(`(?i)\b(cloudwatch|azure\s+monitor|stackdriver|prometheus|grafana)\b`),
				regexp.MustCompile(`(?i)\b(performance|health|uptime|availability|sla|slo)\b`),
			},
			Keywords: []string{"monitoring", "observability", "logging", "metrics", "alerts", "dashboard"},
			Weight:   0.8,
		},
		{
			Intent: "networking",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(network|vpc|subnet|routing|firewall|security\s+group)\b`),
				regexp.MustCompile(`(?i)\b(load\s+balancer|alb|elb|gateway|proxy|cdn)\b`),
				regexp.MustCompile(`(?i)\b(dns|domain|certificate|ssl|tls|https)\b`),
				regexp.MustCompile(`(?i)\b(vpn|expressroute|direct\s+connect|peering|interconnect)\b`),
			},
			Keywords: []string{"network", "vpc", "subnet", "routing", "firewall", "load balancer"},
			Weight:   0.8,
		},
		{
			Intent: "storage",
			Patterns: []*regexp.Regexp{
				regexp.MustCompile(`(?i)\b(storage|s3|blob|file|object|block|database|db)\b`),
				regexp.MustCompile(`(?i)\b(backup|archive|snapshot|replication|sync|copy)\b`),
				regexp.MustCompile(`(?i)\b(mysql|postgresql|mongodb|redis|elasticsearch|dynamodb)\b`),
			},
			Keywords: []string{"storage", "database", "backup", "archive", "replication", "sync"},
			Weight:   0.8,
		},
	}

	return classifiers
}

// buildClarificationRules builds rules for generating clarifications
func buildClarificationRules() []clarificationRule {
	rules := []clarificationRule{
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "migration") && contains(analysis.MissingContext, "source_environment")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "source_environment",
						Questions: []string{
							"What is your current environment?",
							"Where are your applications currently hosted?",
						},
						Suggestions: []string{
							"Specify if you're using on-premises, VMware, Hyper-V, or other virtualization",
							"Include details about your current infrastructure",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "Migration requires source environment specification",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "migration") && contains(analysis.MissingContext, "target_cloud")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "target_cloud",
						Questions: []string{
							"Which cloud provider are you targeting?",
							"Do you have a preference for AWS, Azure, or Google Cloud?",
						},
						Suggestions: []string{
							"Specify AWS, Azure, Google Cloud, or multi-cloud",
							"Consider any existing cloud relationships or preferences",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "Migration requires target cloud specification",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "security") && contains(analysis.MissingContext, "compliance_standards")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "compliance",
						Questions: []string{
							"What compliance standards do you need to meet?",
							"Are there specific regulatory requirements?",
						},
						Suggestions: []string{
							"Specify HIPAA, GDPR, SOX, PCI-DSS, or other standards",
							"Include industry-specific compliance requirements",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "Security planning requires compliance context",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "architecture") && contains(analysis.MissingContext, "scale")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "scale",
						Questions: []string{
							"What scale are you planning for?",
							"How many users or requests per second?",
						},
						Suggestions: []string{
							"Specify expected load, concurrent users, or data volume",
							"Include growth projections and peak usage patterns",
						},
						Priority: "medium",
					},
				}
			},
			Priority:    "medium",
			Description: "Architecture design requires scale understanding",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "disaster_recovery") && contains(analysis.MissingContext, "rto")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "recovery_objectives",
						Questions: []string{
							"What is your target Recovery Time Objective (RTO)?",
							"How much downtime can you tolerate?",
						},
						Suggestions: []string{
							"Specify RTO in minutes, hours, or days",
							"Consider business impact of different downtime scenarios",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "DR planning requires recovery objectives",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return contains(analysis.DetectedIntents, "cost_optimization") && contains(analysis.MissingContext, "current_spend")
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "current_costs",
						Questions: []string{
							"What is your current cloud spending?",
							"Which services are consuming the most budget?",
						},
						Suggestions: []string{
							"Provide monthly/annual cloud bills or estimates",
							"Identify top cost drivers and optimization priorities",
						},
						Priority: "medium",
					},
				}
			},
			Priority:    "medium",
			Description: "Cost optimization requires current spend understanding",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				return analysis.IsAmbiguous && len(analysis.DetectedIntents) == 0
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "intent",
						Questions: []string{
							"What specific outcome are you trying to achieve?",
							"Are you looking for migration, security, architecture, or cost guidance?",
						},
						Suggestions: []string{
							"Specify your primary goal or objective",
							"Choose from migration, security, architecture, DR, or cost optimization",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "Ambiguous queries need intent clarification",
		},
		{
			Trigger: func(analysis QueryAnalysis) bool {
				wordCount := len(strings.Fields(analysis.Query))
				queryLower := strings.ToLower(analysis.Query)
				isGeneralHelp := strings.Contains(queryLower, "can you help") ||
					strings.Contains(queryLower, "help me") ||
					strings.Contains(queryLower, "assist me")
				return (wordCount <= 3 && len(analysis.DetectedIntents) == 0) ||
					(isGeneralHelp && len(analysis.DetectedIntents) == 0)
			},
			Generator: func(_ QueryAnalysis) []Area {
				return []Area{
					{
						Area: "context",
						Questions: []string{
							"What specific topic do you need help with?",
							"Can you provide more context about your situation?",
						},
						Suggestions: []string{
							"Describe your current environment or challenge",
							"Specify the type of solution you're looking for",
						},
						Priority: "high",
					},
				}
			},
			Priority:    "high",
			Description: "Very short queries need more context",
		},
	}

	return rules
}

// buildFollowupDetectors builds detectors for follow-up questions
func buildFollowupDetectors() []followupDetector {
	detectors := []followupDetector{
		{
			Pattern:     regexp.MustCompile(`(?i)\b(that|this|it|the above|previous|earlier|mentioned)\b`),
			Type:        "reference",
			Description: "Detects references to previous content",
			Resolver: func(query string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    extractReferences(query),
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "reference_resolution",
				}
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(more|additional|further|expand|elaborate|detail)\b`),
			Type:        "expansion",
			Description: "Detects requests for more information",
			Resolver: func(_ string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    []string{"expansion_request"},
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "expansion",
				}
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(different|alternative|another|instead|other)\b`),
			Type:        "alternative",
			Description: "Detects requests for alternatives",
			Resolver: func(_ string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    []string{"alternative_request"},
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "alternative_generation",
				}
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(cost|price|pricing|budget|expensive|cheap)\b`),
			Type:        "cost_inquiry",
			Description: "Detects cost-related follow-up questions",
			Resolver: func(_ string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    []string{"cost_inquiry"},
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "cost_analysis",
				}
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(security|secure|compliance|audit|risk)\b`),
			Type:        "security_inquiry",
			Description: "Detects security-related follow-up questions",
			Resolver: func(_ string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    []string{"security_inquiry"},
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "security_analysis",
				}
			},
		},
		{
			Pattern:     regexp.MustCompile(`(?i)\b(that|this|the)\s+(diagram|chart|visual|architecture|design)\b`),
			Type:        "diagram_inquiry",
			Description: "Detects diagram-related follow-up questions",
			Resolver: func(_ string, history []session.Message) *FollowupContext {
				return &FollowupContext{
					ReferencesFound:    []string{"diagram_inquiry"},
					ContextElements:    extractContextElements(history),
					ResolutionStrategy: "diagram_enhancement",
				}
			},
		},
	}

	return detectors
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func extractReferences(query string) []string {
	var references []string
	queryLower := strings.ToLower(query)

	referencePatterns := []string{
		"that diagram", "this plan", "the architecture", "previous response",
		"that solution", "this approach", "the design", "mentioned earlier",
		"above recommendation", "that section", "this part", "the cost",
		"that security", "this migration", "the backup", "that storage",
	}

	for _, pattern := range referencePatterns {
		if strings.Contains(queryLower, pattern) {
			references = append(references, pattern)
		}
	}

	return references
}

func extractContextElements(history []session.Message) []string {
	var elements []string

	// Extract key elements from the most recent assistant response
	if len(history) > 0 {
		lastMessage := history[len(history)-1]
		if lastMessage.Role == session.AssistantRole {
			content := strings.ToLower(lastMessage.Content)

			// Look for key elements that might be referenced
			keyElements := []string{
				"diagram", "architecture", "plan", "solution", "approach",
				"design", "cost", "security", "migration", "backup",
				"database", "storage", "network", "compute", "monitoring",
			}

			for _, element := range keyElements {
				if strings.Contains(content, element) {
					elements = append(elements, element)
				}
			}
		}
	}

	return elements
}
