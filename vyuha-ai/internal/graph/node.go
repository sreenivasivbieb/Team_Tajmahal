package graph

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Node types
// ---------------------------------------------------------------------------

// NodeType represents the kind of entity a graph node models.
type NodeType string

const (
	NodeTypeRepository      NodeType = "repository"
	NodeTypeService         NodeType = "service"
	NodeTypePackage         NodeType = "package"
	NodeTypeFile            NodeType = "file"
	NodeTypeFunction        NodeType = "function"
	NodeTypeStruct          NodeType = "struct"
	NodeTypeInterface       NodeType = "interface"
	NodeTypeCloudService    NodeType = "cloud_service"
	NodeTypeRuntimeInstance NodeType = "runtime_instance"
	NodeTypeDataFlow        NodeType = "data_flow"
)

// ---------------------------------------------------------------------------
// Supporting types
// ---------------------------------------------------------------------------

// Parameter describes a single function parameter.
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// StructField describes a single field within a struct.
type StructField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Tag        string `json:"tag,omitempty"`
	IsExported bool   `json:"is_exported"`
	DocComment string `json:"doc_comment,omitempty"`
}

// InterfaceMethod describes a single method on an interface.
type InterfaceMethod struct {
	Name       string   `json:"name"`
	Signature  string   `json:"signature"`
	Parameters []Parameter `json:"parameters,omitempty"`
	ReturnTypes []string `json:"return_types,omitempty"`
}

// ImportSpec represents a single import statement in a file.
type ImportSpec struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

// FieldMap describes a field-level mapping in a data transformation.
type FieldMap struct {
	FromField  string `json:"from_field"`
	ToField    string `json:"to_field"`
	IsComputed bool   `json:"is_computed"`
	Expression string `json:"expression,omitempty"`
}

// ---------------------------------------------------------------------------
// NodeMetadata — flat struct covering every node type
// ---------------------------------------------------------------------------

// NodeMetadata carries optional, type-specific details for a Node.
// Fields that do not apply to a given NodeType remain at their zero value.
type NodeMetadata struct {
	// -- Function metadata --
	Receiver             string            `json:"receiver,omitempty"`
	IsMethod             bool              `json:"is_method,omitempty"`
	Signature            string            `json:"signature,omitempty"`
	Parameters           []Parameter       `json:"parameters,omitempty"`
	ReturnTypes          []string          `json:"return_types,omitempty"`
	Calls                []string          `json:"calls,omitempty"`
	CalledBy             []string          `json:"called_by,omitempty"`
	DocComment           string            `json:"doc_comment,omitempty"`
	SourceSnippet        string            `json:"source_snippet,omitempty"`
	CyclomaticComplexity int               `json:"cyclomatic_complexity,omitempty"`
	HasErrorReturn       bool              `json:"has_error_return,omitempty"`
	HasContextParam      bool              `json:"has_context_param,omitempty"`
	AISummary            string            `json:"ai_summary,omitempty"`
	AIEmbeddingID        string            `json:"ai_embedding_id,omitempty"`

	// -- Struct metadata --
	Fields               []StructField     `json:"fields,omitempty"`
	ImplementsInterfaces []string          `json:"implements_interfaces,omitempty"`
	Methods              []string          `json:"methods,omitempty"`
	Embeds               []string          `json:"embeds,omitempty"`

	// -- Interface metadata --
	InterfaceMethods     []InterfaceMethod `json:"interface_methods,omitempty"`
	Implementors         []string          `json:"implementors,omitempty"`
	EmbedsInterfaces     []string          `json:"embeds_interfaces,omitempty"`

	// -- File metadata --
	PackageName          string            `json:"package_name,omitempty"`
	Imports              []ImportSpec      `json:"imports,omitempty"`
	Checksum             string            `json:"checksum,omitempty"`
	SizeBytes            int64             `json:"size_bytes,omitempty"`
	IsGenerated          bool              `json:"is_generated,omitempty"`

	// -- Service metadata --
	EntryPoint           string            `json:"entry_point,omitempty"`
	IsMainPackage        bool              `json:"is_main_package,omitempty"`
	CloudDependencies    []string          `json:"cloud_dependencies,omitempty"`
	PackageCount         int               `json:"package_count,omitempty"`
	FunctionCount        int               `json:"function_count,omitempty"`

	// -- Cloud service metadata --
	Provider             string            `json:"provider,omitempty"`
	Service              string            `json:"service,omitempty"`
	SDKPackage           string            `json:"sdk_package,omitempty"`
	DetectedOperations   []string          `json:"detected_operations,omitempty"`
	DetectedInFiles      []string          `json:"detected_in_files,omitempty"`

	// -- Data flow metadata --
	FlowKind             string            `json:"flow_kind,omitempty"`
	TypeName             string            `json:"type_name,omitempty"`
	Source               string            `json:"source,omitempty"`
	Sink                 string            `json:"sink,omitempty"`
	FanIn                int               `json:"fan_in,omitempty"`
	IsAggregate          bool              `json:"is_aggregate,omitempty"`
	FieldMaps            []FieldMap        `json:"field_maps,omitempty"`
}

// MarshalJSON implements json.Marshaler for NodeMetadata.
func (m NodeMetadata) MarshalJSON() ([]byte, error) {
	// Alias avoids infinite recursion by stripping the method set.
	type Alias NodeMetadata
	return json.Marshal((Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for NodeMetadata.
func (m *NodeMetadata) UnmarshalJSON(data []byte) error {
	type Alias NodeMetadata
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*m = NodeMetadata(a)
	return nil
}

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// Node is a vertex in the code-intelligence graph. It represents any
// identifiable entity in the parsed codebase — from a whole repository
// down to an individual function.
type Node struct {
	ID            string       `json:"id"`
	Type          NodeType     `json:"type"`
	Name          string       `json:"name"`
	ParentID      string       `json:"parent_id,omitempty"`
	FilePath      string       `json:"file_path,omitempty"`
	LineStart     int          `json:"line_start,omitempty"`
	LineEnd       int          `json:"line_end,omitempty"`
	RuntimeStatus string       `json:"runtime_status,omitempty"` // "healthy"|"degraded"|"error"|""
	ErrorCount    int          `json:"error_count,omitempty"`
	LastSeen      time.Time    `json:"last_seen,omitempty"`
	Language      string       `json:"language,omitempty"` // "go"|"python"|"typescript"
	IsExported    bool         `json:"is_exported,omitempty"`
	Depth         int          `json:"depth,omitempty"`
	Metadata      NodeMetadata `json:"metadata"`
}

// NewNode creates a Node with the given type and name.
// If id is empty a new UUID v4 is generated.
func NewNode(id string, nodeType NodeType, name string) *Node {
	if id == "" {
		id = uuid.New().String()
	}
	return &Node{
		ID:       id,
		Type:     nodeType,
		Name:     name,
		LastSeen: time.Now().UTC(),
		Metadata: NodeMetadata{},
	}
}

// ---------------------------------------------------------------------------
// Helper methods
// ---------------------------------------------------------------------------

// IsHealthy returns true when the node's runtime status is "healthy".
func (n *Node) IsHealthy() bool {
	return n.RuntimeStatus == "healthy"
}

// IsFailing returns true when the node's runtime status is "error" or
// its error count is greater than zero.
func (n *Node) IsFailing() bool {
	return n.RuntimeStatus == "error" || n.ErrorCount > 0
}

// IsCloud returns true if the node represents a cloud service.
func (n *Node) IsCloud() bool {
	return n.Type == NodeTypeCloudService
}

// FullyQualifiedName returns a human-readable qualified name built from
// the node's file path (when available) and its name, e.g.
// "pkg/handler/handler.go::HandleRequest".
func (n *Node) FullyQualifiedName() string {
	if n.FilePath != "" {
		return fmt.Sprintf("%s::%s", n.FilePath, n.Name)
	}
	return n.Name
}
