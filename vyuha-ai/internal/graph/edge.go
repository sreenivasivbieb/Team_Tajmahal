package graph

import (
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Edge types
// ---------------------------------------------------------------------------

// EdgeType represents the kind of relationship an edge models between two
// nodes in the code-intelligence graph.
type EdgeType string

const (
	EdgeTypeContains     EdgeType = "contains"
	EdgeTypeImports      EdgeType = "imports"
	EdgeTypeCalls        EdgeType = "calls"
	EdgeTypeImplements   EdgeType = "implements"
	EdgeTypeDependsOn    EdgeType = "depends_on"
	EdgeTypeConnectsTo   EdgeType = "connects_to"
	EdgeTypeRuntimeCalls EdgeType = "runtime_calls"
	EdgeTypeFailedAt     EdgeType = "failed_at"
	EdgeTypeProducesTo   EdgeType = "produces_to"
	EdgeTypeConsumedBy   EdgeType = "consumed_by"
	EdgeTypeTransforms   EdgeType = "transforms"
	EdgeTypeFieldMap     EdgeType = "field_map"
)

// ---------------------------------------------------------------------------
// EdgeMetadata — flat struct covering every edge type
// ---------------------------------------------------------------------------

// EdgeMetadata carries optional, type-specific details for an Edge.
// Fields that do not apply to a given EdgeType remain at their zero value.
type EdgeMetadata struct {
	// -- Calls edge metadata --
	CallType         string `json:"call_type,omitempty"`          // "method_call"|"function_call"|"interface_dispatch"
	IsResolved       bool   `json:"is_resolved,omitempty"`
	ResolutionMethod string `json:"resolution_method,omitempty"`  // "type_checker"|"ast_heuristic"
	IsGoroutine      bool   `json:"is_goroutine,omitempty"`
	IsDeferred       bool   `json:"is_deferred,omitempty"`
	CallSiteLine     int    `json:"call_site_line,omitempty"`
	IsCloudCall      bool   `json:"is_cloud_call,omitempty"`
	CloudService     string `json:"cloud_service,omitempty"`
	CloudOperation   string `json:"cloud_operation,omitempty"`

	// -- Contains edge metadata --
	HierarchyDepth int `json:"hierarchy_depth,omitempty"`

	// -- Imports edge metadata --
	Alias           string `json:"alias,omitempty"`
	IsStdlib        bool   `json:"is_stdlib,omitempty"`
	IsThirdParty    bool   `json:"is_third_party,omitempty"`
	ResolvedVersion string `json:"resolved_version,omitempty"`

	// -- ProducesTo / ConsumedBy edge metadata --
	TopicName string `json:"topic_name,omitempty"`
	QueueType string `json:"queue_type,omitempty"` // "kafka"|"sqs"|"rabbitmq"|"nats"

	// -- Transforms / FieldMap edge metadata --
	FromField  string `json:"from_field,omitempty"`
	ToField    string `json:"to_field,omitempty"`
	IsComputed bool   `json:"is_computed,omitempty"`
	Expression string `json:"expression,omitempty"`
}

// ---------------------------------------------------------------------------
// Edge
// ---------------------------------------------------------------------------

// Edge is a directed relationship between two Nodes in the
// code-intelligence graph.
type Edge struct {
	ID       string       `json:"id"`
	SourceID string       `json:"source_id"`
	TargetID string       `json:"target_id"`
	Type     EdgeType     `json:"type"`
	Metadata EdgeMetadata `json:"metadata"`
}

// NewEdge creates an Edge of the given type between sourceID and targetID.
// If id-generation is desired, pass an empty string for the first positional
// argument (there is none — a new UUID v4 is always generated for the ID).
func NewEdge(sourceID, targetID string, edgeType EdgeType) *Edge {
	return &Edge{
		ID:       uuid.New().String(),
		SourceID: sourceID,
		TargetID: targetID,
		Type:     edgeType,
		Metadata: EdgeMetadata{},
	}
}
