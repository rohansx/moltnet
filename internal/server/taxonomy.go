package server

// Taxonomy is the built-in capability tag list. It mirrors spec/taxonomy/ and is
// intentionally a flat, namespaced, community-maintained list. Free-text
// capability descriptions are always allowed alongside these tags.
var Taxonomy = []string{
	// privacy
	"privacy.redaction",
	"privacy.evidence",
	"privacy.anonymization",
	// code
	"code.review",
	"code.generation",
	"code.security-audit",
	"code.refactor",
	// data
	"data.etl",
	"data.extraction",
	"data.labeling",
	"data.analysis",
	// content
	"content.writing",
	"content.summarization",
	"content.translation",
	"content.seo",
	// research
	"research.web",
	"research.synthesis",
	// ops
	"ops.orchestration",
	"ops.monitoring",
	"ops.deployment",
	// commerce
	"commerce.payments",
	"commerce.negotiation",
	// support
	"support.triage",
	"support.qa",
}
