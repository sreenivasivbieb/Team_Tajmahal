package golang

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/google/uuid"
	"github.com/vyuha/vyuha-ai/internal/graph"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// FunctionDataFlow is the complete data-flow analysis for a single function.
type FunctionDataFlow struct {
	FunctionID       string
	DataFlowNodes    []*storage.DataFlowRecord
	Edges            []*graph.Edge // FIELD_MAP, TRANSFORMS edges
	AggregateNodes   []*storage.DataFlowRecord
	HasAsyncOutput   bool
	HasExternalFetch bool
	RiskScore        float64
}

// ---------------------------------------------------------------------------
// Internal helper types
// ---------------------------------------------------------------------------

// dfNode is an in-flight data-flow node being constructed. It is converted
// to a storage.DataFlowRecord at the end of analysis.
type dfNode struct {
	id         string
	kind       string // input|context|error_input|fetched|computed|constructed|output|published|side_effect
	typeName   string
	source     string
	sink       string
	fieldMaps  []graph.FieldMap
	fan        int
	aggregate  bool
	varName    string // the variable name that holds this value (for tracker)
	expression string // string representation of the expression (for computed nodes)
}

// ---------------------------------------------------------------------------
// Fetch / publish / side-effect call name heuristics
// ---------------------------------------------------------------------------

var fetchPrefixes = []string{
	"Get", "Fetch", "Find", "Load", "Read", "Query", "Retrieve", "Lookup",
}

var repoSuffixes = []string{
	"repo", "Repo", "Repository", "repository",
	"store", "Store",
	"client", "Client",
	"service", "Service",
	"gateway", "Gateway",
	"db", "DB", "Db",
	"dao", "DAO", "Dao",
	"cache", "Cache",
}

var publishNames = map[string]bool{
	"Publish":       true,
	"SendMessage":   true,
	"SendMessages":  true,
	"WriteMessages": true,
	"Produce":       true,
	"Send":          true,
	"Emit":          true,
}

var sideEffectPrefixes = []string{
	"Save", "Insert", "Update", "Delete", "Remove", "Set", "Put",
	"Log", "Warn", "Error", "Info", "Debug", "Print",
	"Close", "Flush",
}

// ---------------------------------------------------------------------------
// DataFlowExtractor
// ---------------------------------------------------------------------------

// DataFlowExtractor analyses function ASTs to extract intra-function data
// flow: what types enter, get fetched, computed, constructed, published and
// returned.
type DataFlowExtractor struct {
	modulePath string
	fset       *token.FileSet
}

// NewDataFlowExtractor creates a DataFlowExtractor.
func NewDataFlowExtractor(modulePath string, fset *token.FileSet) *DataFlowExtractor {
	return &DataFlowExtractor{
		modulePath: modulePath,
		fset:       fset,
	}
}

// ---------------------------------------------------------------------------
// ExtractAll — batch entry point
// ---------------------------------------------------------------------------

// ExtractAll runs ExtractFunction for every function node in result.
// astFiles maps relative file paths (forward-slash) to their parsed AST.
// Returns a map keyed by function node ID.
func (e *DataFlowExtractor) ExtractAll(
	ctx context.Context,
	result *ParseResult,
	astFiles map[string]*ast.File,
) (map[string]*FunctionDataFlow, error) {
	// Build lookup: node ID → node.
	nodeByID := make(map[string]*graph.Node, len(result.Nodes))
	for _, n := range result.Nodes {
		nodeByID[n.ID] = n
	}

	// For each file node, collect its imports so we can pass them in.
	fileImports := make(map[string][]graph.ImportSpec)
	for _, n := range result.Nodes {
		if n.Type == graph.NodeTypeFile {
			fileImports[n.ID] = n.Metadata.Imports
		}
	}

	out := make(map[string]*FunctionDataFlow)

	for _, n := range result.Nodes {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if n.Type != graph.NodeTypeFunction {
			continue
		}

		// Locate the parent file node.
		fileNode, ok := nodeByID[n.ParentID]
		if !ok || fileNode.Type != graph.NodeTypeFile {
			continue
		}

		astFile, ok := astFiles[fileNode.FilePath]
		if !ok {
			continue
		}

		funcDecl := findFuncDeclByName(astFile, n.Name, n.Metadata.Receiver)
		if funcDecl == nil || funcDecl.Body == nil {
			continue
		}

		fdf, err := e.ExtractFunction(n, funcDecl, fileImports[fileNode.ID])
		if err != nil {
			continue // non-fatal
		}
		if fdf != nil && len(fdf.DataFlowNodes) > 0 {
			out[n.ID] = fdf
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// ExtractFunction — single function data-flow extraction
// ---------------------------------------------------------------------------

// ExtractFunction analyses a single function and returns its intra-function
// data-flow graph.
func (e *DataFlowExtractor) ExtractFunction(
	funcNode *graph.Node,
	funcDecl *ast.FuncDecl,
	fileImports []graph.ImportSpec,
) (*FunctionDataFlow, error) {
	if funcDecl.Body == nil {
		return nil, nil
	}

	fdf := &FunctionDataFlow{
		FunctionID: funcNode.ID,
	}

	// varTracker maps Go variable names to dataflow node IDs.
	varTracker := make(map[string]string)

	// All data-flow nodes built for this function.
	var dfNodes []*dfNode

	// ---- 1. INPUT NODES (function parameters) ---------------------------
	if funcDecl.Type.Params != nil {
		for _, field := range funcDecl.Type.Params.List {
			typStr := astExprToString(field.Type, e.fset)
			kind := classifyInputKind(typStr)

			for _, nameIdent := range field.Names {
				dn := &dfNode{
					id:       newDFID(),
					kind:     kind,
					typeName: typStr,
					source:   "caller",
					varName:  nameIdent.Name,
				}
				dfNodes = append(dfNodes, dn)
				varTracker[nameIdent.Name] = dn.id
			}

			// Unnamed params (e.g. interface satisfaction stubs).
			if len(field.Names) == 0 {
				dn := &dfNode{
					id:       newDFID(),
					kind:     kind,
					typeName: typStr,
					source:   "caller",
				}
				dfNodes = append(dfNodes, dn)
			}
		}
	}

	// ---- 2-7. Walk the function body ------------------------------------
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {

		// ------ Assignment / short var decl : a, b := expr ---------------
		case *ast.AssignStmt:
			e.handleAssign(stmt, &dfNodes, varTracker, fdf)

		// ------ Return statements ----------------------------------------
		case *ast.ReturnStmt:
			e.handleReturn(stmt, funcDecl, &dfNodes, varTracker, fdf)

		// ------ Expression statements (bare calls like db.Save(...)) -----
		case *ast.ExprStmt:
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				e.handleBareCall(call, &dfNodes, varTracker, fdf)
			}

		// ------ Defer statements -----------------------------------------
		case *ast.DeferStmt:
			e.handleBareCall(stmt.Call, &dfNodes, varTracker, fdf)

		// ------ Go statements --------------------------------------------
		case *ast.GoStmt:
			e.handleBareCall(stmt.Call, &dfNodes, varTracker, fdf)
		}
		return true
	})

	// ---- Build result ---------------------------------------------------
	for _, dn := range dfNodes {
		rec := e.dfnodeToRecord(funcNode.ID, dn)
		fdf.DataFlowNodes = append(fdf.DataFlowNodes, rec)

		if dn.aggregate || dn.fan >= 3 {
			fdf.AggregateNodes = append(fdf.AggregateNodes, rec)
		}
	}

	// ---- Risk score -----------------------------------------------------
	fdf.RiskScore = computeRiskScore(fdf)

	return fdf, nil
}

// ---------------------------------------------------------------------------
// handleAssign — classify assignments into fetched, computed, constructed
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleAssign(
	stmt *ast.AssignStmt,
	dfNodes *[]*dfNode,
	varTracker map[string]string,
	fdf *FunctionDataFlow,
) {
	// We process each RHS expression and pair with the corresponding LHS
	// name (if named).
	for i, rhs := range stmt.Rhs {
		varName := ""
		if i < len(stmt.Lhs) {
			if ident, ok := stmt.Lhs[i].(*ast.Ident); ok && ident.Name != "_" {
				varName = ident.Name
			}
		}

		dn := e.classifyRHS(rhs, varTracker, fdf)
		if dn == nil {
			continue
		}
		dn.varName = varName
		*dfNodes = append(*dfNodes, dn)

		if varName != "" {
			varTracker[varName] = dn.id
		}
	}
}

// classifyRHS determines what kind of data-flow node an RHS expression
// produces.
func (e *DataFlowExtractor) classifyRHS(
	expr ast.Expr,
	varTracker map[string]string,
	fdf *FunctionDataFlow,
) *dfNode {
	// Strip address-of / pointer deref.
	expr = unwrapUnary(expr)

	switch x := expr.(type) {
	case *ast.CompositeLit:
		return e.handleCompositeLit(x, varTracker)

	case *ast.CallExpr:
		return e.handleCallExpr(x, varTracker, fdf)

	case *ast.BinaryExpr:
		return e.handleBinaryExpr(x, varTracker)

	case *ast.TypeAssertExpr:
		// v := x.(Type)  —  treat like a pass-through.
		typStr := astExprToString(x.Type, e.fset)
		return &dfNode{
			id:       newDFID(),
			kind:     "computed",
			typeName: typStr,
		}

	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// handleCompositeLit → CONSTRUCTED nodes
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleCompositeLit(
	lit *ast.CompositeLit,
	varTracker map[string]string,
) *dfNode {
	typeName := extractStructTypeName(lit, e.fset)
	if typeName == "" {
		typeName = "__anon__"
	}

	dn := &dfNode{
		id:       newDFID(),
		kind:     "constructed",
		typeName: typeName,
	}

	sourceNodes := make(map[string]bool)

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		toField := astExprToString(kv.Key, e.fset)
		fromField, sourceID, isComputed := e.traceValue(kv.Value, varTracker)

		fm := graph.FieldMap{
			FromField:  fromField,
			ToField:    toField,
			IsComputed: isComputed,
			Expression: astExprToString(kv.Value, e.fset),
		}
		dn.fieldMaps = append(dn.fieldMaps, fm)

		if sourceID != "" {
			sourceNodes[sourceID] = true
		}
	}

	dn.fan = len(sourceNodes)
	if dn.fan >= 2 {
		dn.aggregate = true
	}
	if dn.fan >= 3 {
		dn.aggregate = true
	}

	return dn
}

// traceValue follows an expression back to the source data-flow node.
// Returns (fromField, sourceNodeID, isComputed).
func (e *DataFlowExtractor) traceValue(
	expr ast.Expr,
	varTracker map[string]string,
) (fromField string, sourceID string, isComputed bool) {
	expr = unwrapUnary(expr)

	switch v := expr.(type) {
	case *ast.Ident:
		// Simple variable reference.
		if id, ok := varTracker[v.Name]; ok {
			return v.Name, id, false
		}
		return v.Name, "", false

	case *ast.SelectorExpr:
		// x.Field — trace x.
		fieldName := v.Sel.Name
		if ident, ok := v.X.(*ast.Ident); ok {
			if id, ok := varTracker[ident.Name]; ok {
				return ident.Name + "." + fieldName, id, false
			}
			return ident.Name + "." + fieldName, "", false
		}
		return astExprToString(expr, e.fset), "", false

	case *ast.CallExpr:
		// Function call result used as a field value → computed.
		return astExprToString(expr, e.fset), "", true

	case *ast.BinaryExpr:
		return astExprToString(expr, e.fset), "", true

	default:
		return astExprToString(expr, e.fset), "", false
	}
}

// ---------------------------------------------------------------------------
// handleCallExpr → FETCHED, PUBLISHED, or generic call node
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleCallExpr(
	call *ast.CallExpr,
	varTracker map[string]string,
	fdf *FunctionDataFlow,
) *dfNode {
	// Determine function/method name and receiver.
	funcName, receiverName := callNames(call)

	// Check for publish calls.
	if publishNames[funcName] {
		fdf.HasAsyncOutput = true
		typStr := inferCallReturnType(call, e.fset)
		return &dfNode{
			id:       newDFID(),
			kind:     "published",
			typeName: typStr,
			sink:     receiverName + "." + funcName,
		}
	}

	// Check for fetch calls: method on a repo/store/client field.
	if isRepoOrStore(receiverName) {
		fdf.HasExternalFetch = true
		typStr := inferCallReturnType(call, e.fset)
		return &dfNode{
			id:       newDFID(),
			kind:     "fetched",
			typeName: typStr,
			source:   receiverName,
		}
	}

	// Check for fetch by function name prefix (Get, Fetch, Find, ...).
	if isFetchCall(call, e.fset) {
		fdf.HasExternalFetch = true
		typStr := inferCallReturnType(call, e.fset)
		return &dfNode{
			id:       newDFID(),
			kind:     "fetched",
			typeName: typStr,
			source:   funcName,
		}
	}

	// Generic call → treated as a computed value.
	typStr := inferCallReturnType(call, e.fset)
	return &dfNode{
		id:       newDFID(),
		kind:     "computed",
		typeName: typStr,
	}
}

// ---------------------------------------------------------------------------
// handleBinaryExpr → COMPUTED nodes
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleBinaryExpr(
	expr *ast.BinaryExpr,
	varTracker map[string]string,
) *dfNode {
	dn := &dfNode{
		id:         newDFID(),
		kind:       "computed",
		typeName:   "__inferred__",
		expression: astExprToString(expr, e.fset),
	}

	// Track which source nodes feed into this computation.
	sources := make(map[string]bool)
	collectSources(expr, varTracker, sources)

	dn.fan = len(sources)
	return dn
}

// collectSources walks an expression tree and records referenced variable
// tracker IDs.
func collectSources(expr ast.Expr, varTracker map[string]string, out map[string]bool) {
	switch v := expr.(type) {
	case *ast.Ident:
		if id, ok := varTracker[v.Name]; ok {
			out[id] = true
		}
	case *ast.BinaryExpr:
		collectSources(v.X, varTracker, out)
		collectSources(v.Y, varTracker, out)
	case *ast.ParenExpr:
		collectSources(v.X, varTracker, out)
	case *ast.CallExpr:
		for _, arg := range v.Args {
			collectSources(arg, varTracker, out)
		}
	case *ast.SelectorExpr:
		collectSources(v.X, varTracker, out)
	case *ast.UnaryExpr:
		collectSources(v.X, varTracker, out)
	}
}

// ---------------------------------------------------------------------------
// handleReturn → OUTPUT nodes
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleReturn(
	stmt *ast.ReturnStmt,
	funcDecl *ast.FuncDecl,
	dfNodes *[]*dfNode,
	varTracker map[string]string,
	fdf *FunctionDataFlow,
) {
	var retTypes []string
	if funcDecl.Type.Results != nil {
		for _, field := range funcDecl.Type.Results.List {
			typStr := astExprToString(field.Type, e.fset)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for j := 0; j < count; j++ {
				retTypes = append(retTypes, typStr)
			}
		}
	}

	for i, retExpr := range stmt.Results {
		typStr := "__unknown__"
		if i < len(retTypes) {
			typStr = retTypes[i]
		}

		// Skip nil, error-typed returns for output lineage.
		if typStr == "error" {
			continue
		}
		if ident, ok := retExpr.(*ast.Ident); ok && ident.Name == "nil" {
			continue
		}

		dn := &dfNode{
			id:       newDFID(),
			kind:     "output",
			typeName: typStr,
		}

		// Trace the returned value to its source.
		fromField, sourceID, isComputed := e.traceValue(retExpr, varTracker)
		if fromField != "" || sourceID != "" {
			dn.fieldMaps = append(dn.fieldMaps, graph.FieldMap{
				FromField:  fromField,
				ToField:    fmt.Sprintf("return_%d", i),
				IsComputed: isComputed,
				Expression: astExprToString(retExpr, e.fset),
			})
		}

		if sourceID != "" {
			// Create a TRANSFORMS edge from source → output.
			edge := graph.NewEdge(sourceID, dn.id, graph.EdgeTypeTransforms)
			edge.Metadata.FromField = fromField
			edge.Metadata.ToField = fmt.Sprintf("return_%d", i)
			fdf.Edges = append(fdf.Edges, edge)
		}

		*dfNodes = append(*dfNodes, dn)
	}
}

// ---------------------------------------------------------------------------
// handleBareCall → SIDE_EFFECT or PUBLISHED nodes from expression stmts
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) handleBareCall(
	call *ast.CallExpr,
	dfNodes *[]*dfNode,
	varTracker map[string]string,
	fdf *FunctionDataFlow,
) {
	funcName, receiverName := callNames(call)

	// Publish call.
	if publishNames[funcName] {
		fdf.HasAsyncOutput = true
		dn := &dfNode{
			id:       newDFID(),
			kind:     "published",
			typeName: inferArgType(call, e.fset),
			sink:     receiverName + "." + funcName,
		}
		*dfNodes = append(*dfNodes, dn)
		return
	}

	// Side-effect calls (Save, Delete, Log, ...).
	if isSideEffectCall(funcName) {
		dn := &dfNode{
			id:       newDFID(),
			kind:     "side_effect",
			typeName: inferArgType(call, e.fset),
			sink:     receiverName + "." + funcName,
		}
		*dfNodes = append(*dfNodes, dn)
		return
	}
}

// ---------------------------------------------------------------------------
// dfnodeToRecord — convert internal representation to storage record
// ---------------------------------------------------------------------------

func (e *DataFlowExtractor) dfnodeToRecord(functionID string, dn *dfNode) *storage.DataFlowRecord {
	fmJSON := "[]"
	if len(dn.fieldMaps) > 0 {
		if b, err := json.Marshal(dn.fieldMaps); err == nil {
			fmJSON = string(b)
		}
	}

	metaMap := make(map[string]interface{})
	if dn.expression != "" {
		metaMap["expression"] = dn.expression
	}
	if dn.varName != "" {
		metaMap["var_name"] = dn.varName
	}

	metaJSON := "{}"
	if len(metaMap) > 0 {
		if b, err := json.Marshal(metaMap); err == nil {
			metaJSON = string(b)
		}
	}

	return &storage.DataFlowRecord{
		ID:          dn.id,
		FunctionID:  functionID,
		Kind:        dn.kind,
		TypeName:    dn.typeName,
		Source:      dn.source,
		Sink:        dn.sink,
		FieldMaps:   fmJSON,
		IsAggregate: dn.aggregate,
		FanIn:       dn.fan,
		Metadata:    metaJSON,
	}
}

// ---------------------------------------------------------------------------
// Risk score computation
// ---------------------------------------------------------------------------

// computeRiskScore estimates the data-flow complexity/risk of a function.
// Higher scores indicate functions that aggregate many sources and have
// external dependencies — valuable targets for monitoring and testing.
func computeRiskScore(fdf *FunctionDataFlow) float64 {
	score := 0.0

	for _, dn := range fdf.DataFlowNodes {
		switch dn.Kind {
		case "fetched":
			score += 2.0 // external dependency
		case "constructed":
			if dn.IsAggregate {
				score += 3.0 // high-value aggregation point
			} else {
				score += 1.0
			}
			score += float64(dn.FanIn) * 0.5
		case "published":
			score += 2.5 // async output
		case "side_effect":
			score += 1.0
		case "computed":
			score += 0.5
		}
	}

	if fdf.HasAsyncOutput {
		score += 2.0
	}
	if fdf.HasExternalFetch {
		score += 1.5
	}

	return score
}

// ---------------------------------------------------------------------------
// AST helper functions
// ---------------------------------------------------------------------------

// astExprToString converts an AST expression to its Go source representation.
func astExprToString(expr ast.Expr, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}

	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return astExprToString(v.X, fset) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + astExprToString(v.X, fset)
	case *ast.UnaryExpr:
		return v.Op.String() + astExprToString(v.X, fset)
	case *ast.BinaryExpr:
		return astExprToString(v.X, fset) + " " + v.Op.String() + " " + astExprToString(v.Y, fset)
	case *ast.CallExpr:
		return astExprToString(v.Fun, fset) + "(...)"
	case *ast.IndexExpr:
		return astExprToString(v.X, fset) + "[" + astExprToString(v.Index, fset) + "]"
	case *ast.SliceExpr:
		return astExprToString(v.X, fset) + "[:]"
	case *ast.TypeAssertExpr:
		return astExprToString(v.X, fset) + ".(" + astExprToString(v.Type, fset) + ")"
	case *ast.ParenExpr:
		return "(" + astExprToString(v.X, fset) + ")"
	case *ast.BasicLit:
		return v.Value
	case *ast.CompositeLit:
		return astExprToString(v.Type, fset) + "{...}"
	case *ast.ArrayType:
		return "[]" + astExprToString(v.Elt, fset)
	case *ast.MapType:
		return "map[" + astExprToString(v.Key, fset) + "]" + astExprToString(v.Value, fset)
	case *ast.FuncLit:
		return "func(){...}"
	case *ast.Ellipsis:
		return "..." + astExprToString(v.Elt, fset)
	case *ast.ChanType:
		return "chan " + astExprToString(v.Value, fset)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.KeyValueExpr:
		return astExprToString(v.Key, fset) + ": " + astExprToString(v.Value, fset)
	default:
		return fmt.Sprintf("<%T>", expr)
	}
}

// extractStructTypeName extracts the type name from a composite literal.
func extractStructTypeName(lit *ast.CompositeLit, fset *token.FileSet) string {
	if lit.Type == nil {
		return ""
	}
	return astExprToString(lit.Type, fset)
}

// isFetchCall returns true if the call's function name starts with a known
// fetch prefix (Get, Fetch, Find, ...).
func isFetchCall(call *ast.CallExpr, fset *token.FileSet) bool {
	name, _ := callNames(call)
	for _, prefix := range fetchPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isRepoOrStore reports whether the name ends with a known repository/
// store/client suffix, suggesting an external data source.
func isRepoOrStore(name string) bool {
	if name == "" {
		return false
	}
	for _, suffix := range repoSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// isSideEffectCall reports whether a function name indicates a write-only
// or logging operation.
func isSideEffectCall(funcName string) bool {
	for _, prefix := range sideEffectPrefixes {
		if strings.HasPrefix(funcName, prefix) {
			return true
		}
	}
	return false
}

// callNames extracts the function name and optional receiver / package name
// from a call expression. For `s.repo.GetUser(...)` → ("GetUser", "repo").
// For `ProcessOrder(...)` → ("ProcessOrder", "").
func callNames(call *ast.CallExpr) (funcName, receiverName string) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name, ""
	case *ast.SelectorExpr:
		funcName = fn.Sel.Name
		// Walk the receiver chain to find the deepest meaningful ident.
		receiverName = deepestIdent(fn.X)
		return
	default:
		return "", ""
	}
}

// deepestIdent walks a selector chain and returns the last identifier name
// before the final selector. For `s.userRepo` it returns "userRepo".
// For `s.client.storage` it returns "storage".
func deepestIdent(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		// Prefer the selector (e.g. `s.userRepo` → "userRepo").
		return v.Sel.Name
	case *ast.CallExpr:
		return deepestIdent(v.Fun)
	case *ast.IndexExpr:
		return deepestIdent(v.X)
	default:
		return ""
	}
}

// inferCallReturnType attempts to produce a human-readable representation
// of what a call expression returns. Without type-checker info this is
// necessarily heuristic.
func inferCallReturnType(call *ast.CallExpr, fset *token.FileSet) string {
	return astExprToString(call.Fun, fset) + "()"
}

// inferArgType returns the type string of the first argument to a call, or
// a generic label.
func inferArgType(call *ast.CallExpr, fset *token.FileSet) string {
	if len(call.Args) > 0 {
		return astExprToString(call.Args[0], fset)
	}
	return "__void__"
}

// unwrapUnary strips &, *, and parentheses from an expression to reveal the
// underlying value expression.
func unwrapUnary(expr ast.Expr) ast.Expr {
	for {
		switch v := expr.(type) {
		case *ast.UnaryExpr:
			if v.Op == token.AND || v.Op == token.MUL {
				expr = v.X
				continue
			}
			return expr
		case *ast.ParenExpr:
			expr = v.X
			continue
		default:
			return expr
		}
	}
}

// classifyInputKind determines the data-flow kind for a parameter type.
func classifyInputKind(typeName string) string {
	if strings.Contains(typeName, "Context") {
		return "context"
	}
	if typeName == "error" {
		return "error_input"
	}
	return "input"
}

// newDFID generates a unique data-flow node identifier.
func newDFID() string {
	return "df:" + uuid.New().String()
}
