package golang

import (
	"context"
	"go/ast"
	"go/token"
	"strings"

	"github.com/vyuha/vyuha-ai/internal/graph"
)

// ---------------------------------------------------------------------------
// Import → service mapping tables
// ---------------------------------------------------------------------------

// awsImportMap maps import path prefixes/suffixes to canonical AWS service
// identifiers.
var awsImportMap = map[string]string{
	// aws-sdk-go v1
	"github.com/aws/aws-sdk-go/service/s3":              "s3",
	"github.com/aws/aws-sdk-go/service/dynamodb":         "dynamodb",
	"github.com/aws/aws-sdk-go/service/sqs":              "sqs",
	"github.com/aws/aws-sdk-go/service/sns":              "sns",
	"github.com/aws/aws-sdk-go/service/lambda":           "lambda",
	"github.com/aws/aws-sdk-go/service/secretsmanager":   "secretsmanager",
	"github.com/aws/aws-sdk-go/service/ses":              "ses",
	"github.com/aws/aws-sdk-go/service/kinesis":          "kinesis",
	// aws-sdk-go-v2
	"github.com/aws/aws-sdk-go-v2/service/s3":             "s3",
	"github.com/aws/aws-sdk-go-v2/service/dynamodb":       "dynamodb",
	"github.com/aws/aws-sdk-go-v2/service/sqs":            "sqs",
	"github.com/aws/aws-sdk-go-v2/service/sns":            "sns",
	"github.com/aws/aws-sdk-go-v2/service/lambda":         "lambda",
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime": "bedrock",
	// lambda runtime
	"github.com/aws/aws-lambda-go/lambda": "lambda",
}

// queueImportMap maps import paths to queue type identifiers.
var queueImportMap = map[string]string{
	"github.com/segmentio/kafka-go":                    "kafka",
	"github.com/IBM/sarama":                            "kafka",
	"github.com/Shopify/sarama":                        "kafka",
	"github.com/confluentinc/confluent-kafka-go/kafka": "kafka",
	"github.com/rabbitmq/amqp091-go":                   "rabbitmq",
	"github.com/nats-io/nats.go":                       "nats",
	"cloud.google.com/go/pubsub":                       "pubsub",
}

// dbImportMap maps import paths to database type identifiers.
var dbImportMap = map[string]string{
	"database/sql":                        "sql",
	"github.com/jmoiron/sqlx":             "sqlx",
	"gorm.io/gorm":                        "gorm",
	"go.mongodb.org/mongo-driver/mongo":   "mongodb",
	"github.com/go-redis/redis":           "redis",
	"github.com/redis/go-redis/v9":        "redis",
	"go.etcd.io/etcd/client":              "etcd",
}

// httpImportMap maps import paths to HTTP/RPC type identifiers.
var httpImportMap = map[string]string{
	"net/http":                       "client",
	"github.com/go-resty/resty/v2":  "resty",
	"google.golang.org/grpc":        "grpc",
}

// ---------------------------------------------------------------------------
// SDK operation patterns — used to detect specific cloud operations inside
// function bodies.
// ---------------------------------------------------------------------------

// sdkOps maps a cloud/queue/db service ID to the set of selector method
// names that constitute known operations.
var sdkOps = map[string]map[string]bool{
	// AWS S3
	"aws:s3": {
		"New": true, "NewFromConfig": true,
		"PutObject": true, "GetObject": true, "DeleteObject": true,
		"ListObjects": true, "ListObjectsV2": true,
		"CreateBucket": true, "HeadObject": true,
	},
	// AWS DynamoDB
	"aws:dynamodb": {
		"New": true, "NewFromConfig": true,
		"PutItem": true, "GetItem": true, "DeleteItem": true,
		"Query": true, "Scan": true, "UpdateItem": true,
		"BatchWriteItem": true, "BatchGetItem": true,
	},
	// AWS SQS
	"aws:sqs": {
		"New": true, "NewFromConfig": true,
		"SendMessage": true, "ReceiveMessage": true,
		"DeleteMessage": true, "GetQueueUrl": true,
	},
	// AWS SNS
	"aws:sns": {
		"New": true, "NewFromConfig": true,
		"Publish": true, "Subscribe": true,
	},
	// AWS Lambda
	"aws:lambda": {
		"New": true, "NewFromConfig": true,
		"Invoke": true, "Start": true,
	},
	// AWS Secrets Manager
	"aws:secretsmanager": {
		"New": true, "NewFromConfig": true,
		"GetSecretValue": true, "CreateSecret": true,
	},
	// AWS SES
	"aws:ses": {
		"New": true, "NewFromConfig": true,
		"SendEmail": true, "SendRawEmail": true,
	},
	// AWS Kinesis
	"aws:kinesis": {
		"New": true, "NewFromConfig": true,
		"PutRecord": true, "PutRecords": true,
		"GetRecords": true, "GetShardIterator": true,
	},
	// AWS Bedrock
	"aws:bedrock": {
		"NewFromConfig": true,
		"InvokeModel": true, "InvokeModelWithResponseStream": true,
	},
}

// kafkaProducerOps are method names on Kafka/Sarama producers that indicate
// a PRODUCES_TO relationship.
var kafkaProducerOps = map[string]bool{
	"WriteMessages": true,
	"SendMessage":   true,
	"SendMessages":  true,
}

// kafkaConsumerOps are method names that indicate a CONSUMED_BY relationship.
var kafkaConsumerOps = map[string]bool{
	"ReadMessage":      true,
	"FetchMessage":     true,
	"ConsumePartition": true,
}

// httpCallOps are selector method names on net/http that mark an outbound
// HTTP call.
var httpCallOps = map[string]bool{
	"NewRequest":    true,
	"Get":           true,
	"Post":          true,
	"Do":            true,
	"PostForm":      true,
	"Head":          true,
	"NewRequestWithContext": true,
}

// ---------------------------------------------------------------------------
// CloudDetector
// ---------------------------------------------------------------------------

// CloudDetector enriches a ParseResult by scanning import paths and function
// bodies for cloud SDK, queue, database and HTTP client usage.
type CloudDetector struct {
	modulePath string
	fileSet    *token.FileSet

	// importMap is built per-detection-run. It maps the local import alias
	// (or last path segment) to canonical service IDs like "aws:s3",
	// "queue:kafka", "db:redis", "http:client".
	importMap map[string]string
}

// NewCloudDetector creates a CloudDetector bound to the given module path
// and file set.
func NewCloudDetector(modulePath string, fset *token.FileSet) *CloudDetector {
	return &CloudDetector{
		modulePath: modulePath,
		fileSet:    fset,
		importMap:  make(map[string]string),
	}
}

// ---------------------------------------------------------------------------
// Detect — main entry point
// ---------------------------------------------------------------------------

// Detect scans every file and function node in result for cloud/queue/db
// usage and mutates result in place, appending new nodes and edges.
//
// astFiles maps relative file paths (forward-slash) to their parsed AST
// trees. It is used to walk function bodies for SDK method calls.
func (d *CloudDetector) Detect(
	ctx context.Context,
	result *ParseResult,
	astFiles map[string]*ast.File,
) error {
	// -- Phase 0: build indexes -------------------------------------------
	nodeByID := make(map[string]*graph.Node, len(result.Nodes))
	for _, n := range result.Nodes {
		nodeByID[n.ID] = n
	}

	// file node ID → service node IDs (transitive parents).
	fileToServices := d.buildFileToServiceMap(result.Nodes, nodeByID)

	// Track existing cloud/queue/db nodes across the run.
	cloudNodes := make(map[string]*graph.Node)  // canonical ID → node
	edgeSet := make(map[string]bool)            // "srcID|targetID|type" dedup

	// Pre-index existing edges.
	for _, e := range result.Edges {
		edgeSet[edgeKey(e.SourceID, e.TargetID, string(e.Type))] = true
	}

	// -- Phase 1: scan file nodes for cloud imports -----------------------
	// fileCloudIDs maps file node ID → set of cloud service IDs detected
	// through imports.
	fileCloudIDs := make(map[string]map[string]bool)
	// fileAlias maps (file relPath, alias) → cloud service ID.
	fileAlias := make(map[string]map[string]string)

	for _, n := range result.Nodes {
		if n.Type != graph.NodeTypeFile {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		detected := make(map[string]bool)
		aliases := make(map[string]string)

		for _, imp := range n.Metadata.Imports {
			cloudID, alias := d.classifyImport(imp.Path, imp.Alias)
			if cloudID == "" {
				continue
			}

			detected[cloudID] = true
			aliases[alias] = cloudID

			// Ensure cloud node exists.
			cNode := d.findOrCreateCloudNode(cloudNodes, cloudID, imp.Path)

			// Record detected-in-files.
			if !containsStr(cNode.Metadata.DetectedInFiles, n.FilePath) {
				cNode.Metadata.DetectedInFiles = append(cNode.Metadata.DetectedInFiles, n.FilePath)
			}

			// DEPENDS_ON edge: file → cloud node.
			d.addEdgeIfNew(result, edgeSet, n.ID, cNode.ID, graph.EdgeTypeDependsOn, nil)

			// DEPENDS_ON edge: service → cloud node.
			for _, svcID := range fileToServices[n.ID] {
				d.addEdgeIfNew(result, edgeSet, svcID, cNode.ID, graph.EdgeTypeDependsOn, nil)
			}
		}

		if len(detected) > 0 {
			fileCloudIDs[n.ID] = detected
		}
		if len(aliases) > 0 {
			fileAlias[n.FilePath] = aliases
		}
	}

	// -- Phase 2: scan function bodies for SDK method calls ---------------
	// funcCloudCount tracks how many unique cloud services each function
	// calls, used for high_cloud_coupling scoring later.
	funcCloudCount := make(map[string]map[string]bool)

	for _, n := range result.Nodes {
		if n.Type != graph.NodeTypeFunction {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fileID := n.ParentID
		if _, ok := fileCloudIDs[fileID]; !ok {
			continue // parent file has no cloud imports
		}

		fileNode, ok := nodeByID[fileID]
		if !ok {
			continue
		}

		astFile, ok := astFiles[fileNode.FilePath]
		if !ok {
			continue
		}

		aliases := fileAlias[fileNode.FilePath]
		if aliases == nil {
			continue
		}

		funcDecl := findFuncDeclByName(astFile, n.Name, n.Metadata.Receiver)
		if funcDecl == nil || funcDecl.Body == nil {
			continue
		}

		cloudCalls := d.scanFunctionBody(funcDecl.Body, aliases, n.ID, result, edgeSet, cloudNodes)
		if len(cloudCalls) > 0 {
			funcCloudCount[n.ID] = cloudCalls
		}
	}

	// -- Phase 3: append accumulated cloud nodes to result ----------------
	for _, cn := range cloudNodes {
		result.Nodes = append(result.Nodes, cn)
	}

	// -- Phase 4: importance scoring --------------------------------------
	d.computeImportanceScoring(result.Nodes, nodeByID, fileCloudIDs, fileToServices, funcCloudCount)

	return nil
}

// ---------------------------------------------------------------------------
// scanFunctionBody — walk a function AST for SDK method calls
// ---------------------------------------------------------------------------

func (d *CloudDetector) scanFunctionBody(
	body *ast.BlockStmt,
	aliases map[string]string,
	funcNodeID string,
	result *ParseResult,
	edgeSet map[string]bool,
	cloudNodes map[string]*graph.Node,
) map[string]bool {
	calledServices := make(map[string]bool)

	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		method := sel.Sel.Name

		// Determine the receiver/package alias.
		alias := exprAlias(sel.X)
		if alias == "" {
			return true
		}

		// Try to resolve alias → cloud service ID.
		cloudID, matched := d.resolveAlias(alias, method, aliases)
		if !matched {
			return true
		}

		// Mark as detected.
		calledServices[cloudID] = true

		line := 0
		if d.fileSet != nil {
			line = d.fileSet.Position(call.Pos()).Line
		}

		// Ensure cloud node exists.
		cNode := d.findOrCreateCloudNode(cloudNodes, cloudID, "")
		if !containsStr(cNode.Metadata.DetectedOperations, method) {
			cNode.Metadata.DetectedOperations = append(cNode.Metadata.DetectedOperations, method)
		}

		// CALLS edge: function → cloud node.
		meta := &graph.EdgeMetadata{
			IsCloudCall:    true,
			CloudService:   cloudID,
			CloudOperation: method,
			CallSiteLine:   line,
			CallType:       "method_call",
		}
		d.addEdgeIfNew(result, edgeSet, funcNodeID, cNode.ID, graph.EdgeTypeCalls, meta)

		// -- Queue-specific: PRODUCES_TO / CONSUMED_BY --------------------
		if strings.HasPrefix(cloudID, "queue:") {
			queueType := strings.TrimPrefix(cloudID, "queue:")

			if kafkaProducerOps[method] {
				topic := extractTopicFromArgs(call.Args, d.fileSet)
				topicNodeID := buildTopicNodeID(queueType, topic)
				topicNode := d.findOrCreateTopicNode(cloudNodes, topicNodeID, queueType, topic)
				_ = topicNode

				prodEdge := graph.NewEdge(funcNodeID, topicNodeID, graph.EdgeTypeProducesTo)
				prodEdge.Metadata.TopicName = topic
				prodEdge.Metadata.QueueType = queueType
				prodEdge.Metadata.CallSiteLine = line
				result.Edges = append(result.Edges, prodEdge)
			}

			if kafkaConsumerOps[method] {
				topic := extractTopicFromArgs(call.Args, d.fileSet)
				topicNodeID := buildTopicNodeID(queueType, topic)
				topicNode := d.findOrCreateTopicNode(cloudNodes, topicNodeID, queueType, topic)
				_ = topicNode

				consEdge := graph.NewEdge(topicNodeID, funcNodeID, graph.EdgeTypeConsumedBy)
				consEdge.Metadata.TopicName = topic
				consEdge.Metadata.QueueType = queueType
				consEdge.Metadata.CallSiteLine = line
				result.Edges = append(result.Edges, consEdge)
			}
		}

		// -- HTTP call detection ------------------------------------------
		if cloudID == "http:client" && httpCallOps[method] {
			meta.CloudOperation = method
		}

		return true
	})

	return calledServices
}

// ---------------------------------------------------------------------------
// resolveAlias — try multiple strategies to match alias + method to a cloud
// service ID.
// ---------------------------------------------------------------------------

func (d *CloudDetector) resolveAlias(alias, method string, aliases map[string]string) (string, bool) {
	// Strategy 1: direct alias look-up.
	if cloudID, ok := aliases[alias]; ok {
		return cloudID, true
	}

	// Strategy 2: for net/http calls, check if "http" alias is present and
	// the method is a known HTTP operation.
	if alias == "http" {
		if cloudID, ok := aliases["http"]; ok && httpCallOps[method] {
			return cloudID, true
		}
	}

	// Strategy 3: the alias matches a known short name like "s3", "sqs",
	// "dynamodb", "kafka", "sarama", etc. Search all aliases' values.
	for a, cid := range aliases {
		// Check if alias appears as part of the canonical alias.
		if strings.EqualFold(alias, a) {
			return cid, true
		}
	}

	return "", false
}

// ---------------------------------------------------------------------------
// Import classification
// ---------------------------------------------------------------------------

// classifyImport determines what cloud/queue/db/http category an import
// path belongs to. Returns ("", "") if unrecognised.
//
// The returned alias is the short name that will appear in code (either the
// explicit alias or the last segment of the import path).
func (d *CloudDetector) classifyImport(importPath, explicitAlias string) (cloudID, alias string) {
	alias = explicitAlias
	if alias == "" || alias == "." || alias == "_" {
		// Derive default alias from import path.
		alias = defaultAlias(importPath)
	}

	// AWS
	if svc, ok := isAWSImport(importPath); ok {
		return "aws:" + svc, alias
	}

	// Queue
	if qt, ok := isQueueImport(importPath); ok {
		return "queue:" + qt, alias
	}

	// Database
	if dt, ok := isDBImport(importPath); ok {
		return "db:" + dt, alias
	}

	// HTTP / RPC
	if ht, ok := isHTTPImport(importPath); ok {
		return ht, alias
	}

	return "", ""
}

// ---------------------------------------------------------------------------
// Import matcher helpers
// ---------------------------------------------------------------------------

func isAWSImport(path string) (service string, ok bool) {
	for prefix, svc := range awsImportMap {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return svc, true
		}
	}
	return "", false
}

func isQueueImport(path string) (queueType string, ok bool) {
	for prefix, qt := range queueImportMap {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return qt, true
		}
	}
	return "", false
}

func isDBImport(path string) (dbType string, ok bool) {
	for prefix, dt := range dbImportMap {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return dt, true
		}
	}
	return "", false
}

func isHTTPImport(path string) (httpID string, ok bool) {
	for prefix, ht := range httpImportMap {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			// net/http → "http:client", resty → "http:resty", grpc → "rpc:grpc"
			switch ht {
			case "grpc":
				return "rpc:grpc", true
			default:
				return "http:" + ht, true
			}
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Node creation helpers
// ---------------------------------------------------------------------------

// findOrCreateCloudNode returns an existing cloud node or creates a new one.
func (d *CloudDetector) findOrCreateCloudNode(
	cloudNodes map[string]*graph.Node,
	cloudID string,
	sdkPackage string,
) *graph.Node {
	if n, ok := cloudNodes[cloudID]; ok {
		return n
	}

	provider, service := splitCloudID(cloudID)
	nodeType := graph.NodeTypeCloudService

	node := graph.NewNode(buildCloudNodeID(provider, service), nodeType, cloudID)
	node.Metadata.Provider = provider
	node.Metadata.Service = service
	node.Metadata.SDKPackage = sdkPackage

	cloudNodes[cloudID] = node
	return node
}

// findOrCreateTopicNode returns an existing topic node or creates a new one.
func (d *CloudDetector) findOrCreateTopicNode(
	cloudNodes map[string]*graph.Node,
	topicNodeID string,
	queueType string,
	topic string,
) *graph.Node {
	if n, ok := cloudNodes[topicNodeID]; ok {
		return n
	}

	node := graph.NewNode(topicNodeID, graph.NodeTypeDataFlow, topic)
	node.Metadata.FlowKind = "topic"
	node.Metadata.TypeName = queueType
	node.Metadata.Provider = "queue"
	node.Metadata.Service = queueType

	cloudNodes[topicNodeID] = node
	return node
}

// buildCloudNodeID constructs the canonical node ID for a cloud/queue/db
// service.
func buildCloudNodeID(provider, service string) string {
	return provider + ":" + service
}

// buildTopicNodeID constructs a topic-level node ID.
func buildTopicNodeID(queueType, topic string) string {
	return "queue:" + queueType + ":" + topic
}

// splitCloudID splits "aws:s3" → ("aws", "s3"), "queue:kafka" →
// ("queue", "kafka"), etc.
func splitCloudID(cloudID string) (provider, service string) {
	parts := strings.SplitN(cloudID, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return cloudID, ""
}

// ---------------------------------------------------------------------------
// AST navigation helpers
// ---------------------------------------------------------------------------

// findFuncDeclByName locates the *ast.FuncDecl in an AST file that matches
// the given function name and optional receiver type.
func findFuncDeclByName(file *ast.File, name, receiver string) *ast.FuncDecl {
	// Strip pointer marker from receiver for matching.
	receiver = strings.TrimPrefix(receiver, "*")

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != name {
			continue
		}

		if receiver == "" {
			// Looking for a plain function.
			if fd.Recv == nil || len(fd.Recv.List) == 0 {
				return fd
			}
			continue
		}

		// Looking for a method with a specific receiver.
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recvType := exprBaseName(fd.Recv.List[0].Type)
		if recvType == receiver {
			return fd
		}
	}
	return nil
}

// exprAlias extracts the leftmost identifier name from an expression,
// returning "" if it cannot be determined. For instance, for `s3client.PutObject`
// it returns "s3client".
func exprAlias(x ast.Expr) string {
	switch v := x.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprAlias(v.X)
	case *ast.CallExpr:
		return exprAlias(v.Fun)
	default:
		return ""
	}
}

// exprBaseName extracts the base type name from a receiver expression,
// stripping pointer indirection.
func exprBaseName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return exprBaseName(v.X)
	case *ast.SelectorExpr:
		return v.Sel.Name
	default:
		return ""
	}
}

// extractTopicFromArgs attempts to extract a string literal topic name from
// call arguments. Returns "__dynamic__" if no string literal is found.
func extractTopicFromArgs(args []ast.Expr, fset *token.FileSet) string {
	for _, arg := range args {
		topic := findStringLiteral(arg)
		if topic != "" {
			return topic
		}
	}
	return "__dynamic__"
}

// findStringLiteral recursively searches an expression for a basic string
// literal and returns its unquoted value.
func findStringLiteral(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"` + "`")
		}
	case *ast.CompositeLit:
		// Walk struct literal fields for topic strings.
		for _, elt := range v.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if ident, ok := kv.Key.(*ast.Ident); ok {
					nameLower := strings.ToLower(ident.Name)
					if nameLower == "topic" || nameLower == "topicname" || nameLower == "queue" {
						if s := findStringLiteral(kv.Value); s != "" {
							return s
						}
					}
				}
			}
		}
	case *ast.UnaryExpr:
		return findStringLiteral(v.X)
	}
	return ""
}

// defaultAlias derives the default import alias from an import path.
// For "github.com/aws/aws-sdk-go/service/s3" it returns "s3".
// For "github.com/nats-io/nats.go" it returns "nats".
func defaultAlias(importPath string) string {
	// Last path segment.
	idx := strings.LastIndex(importPath, "/")
	seg := importPath
	if idx >= 0 {
		seg = importPath[idx+1:]
	}
	// Strip ".go" suffix (e.g. nats.go → nats).
	seg = strings.TrimSuffix(seg, ".go")
	// Strip version suffix (e.g. v2, v9).
	if len(seg) >= 2 && seg[0] == 'v' && seg[1] >= '0' && seg[1] <= '9' {
		// This is a version directory — use the parent.
		parentIdx := strings.LastIndex(importPath[:idx], "/")
		if parentIdx >= 0 {
			seg = importPath[parentIdx+1 : idx]
		}
	}
	return seg
}

// ---------------------------------------------------------------------------
// Edge deduplication
// ---------------------------------------------------------------------------

// addEdgeIfNew adds an edge to result only if the (source, target, type)
// triple has not been seen before.
func (d *CloudDetector) addEdgeIfNew(
	result *ParseResult,
	edgeSet map[string]bool,
	srcID, targetID string,
	edgeType graph.EdgeType,
	meta *graph.EdgeMetadata,
) {
	key := edgeKey(srcID, targetID, string(edgeType))
	if edgeSet[key] {
		return
	}
	edgeSet[key] = true

	e := graph.NewEdge(srcID, targetID, edgeType)
	if meta != nil {
		e.Metadata = *meta
	}
	result.Edges = append(result.Edges, e)
}

func edgeKey(src, target, typ string) string {
	return src + "|" + target + "|" + typ
}

// ---------------------------------------------------------------------------
// buildFileToServiceMap
// ---------------------------------------------------------------------------

// buildFileToServiceMap traces backwards from file nodes through CONTAINS
// edges to find the enclosing service nodes.
func (d *CloudDetector) buildFileToServiceMap(
	nodes []*graph.Node,
	nodeByID map[string]*graph.Node,
) map[string][]string {
	// service node ID → file path prefixes (from service.FilePath).
	type svcInfo struct {
		id       string
		filePath string
	}
	var services []svcInfo
	for _, n := range nodes {
		if n.Type == graph.NodeTypeService {
			services = append(services, svcInfo{id: n.ID, filePath: n.FilePath})
		}
	}

	fileToSvc := make(map[string][]string)
	for _, n := range nodes {
		if n.Type != graph.NodeTypeFile {
			continue
		}
		for _, svc := range services {
			// A file belongs to a service if its path starts with the
			// service directory or they share the same root.
			if svc.filePath == "." || strings.HasPrefix(n.FilePath, svc.filePath) {
				fileToSvc[n.ID] = append(fileToSvc[n.ID], svc.id)
			}
		}
	}
	return fileToSvc
}

// ---------------------------------------------------------------------------
// Importance scoring
// ---------------------------------------------------------------------------

func (d *CloudDetector) computeImportanceScoring(
	nodes []*graph.Node,
	nodeByID map[string]*graph.Node,
	fileCloudIDs map[string]map[string]bool,
	fileToServices map[string][]string,
	funcCloudCount map[string]map[string]bool,
) {
	// -- Service-level: cloud_dependency_count ----------------------------
	serviceCloudDeps := make(map[string]map[string]bool)
	for fileID, cloudIDs := range fileCloudIDs {
		for _, svcID := range fileToServices[fileID] {
			if serviceCloudDeps[svcID] == nil {
				serviceCloudDeps[svcID] = make(map[string]bool)
			}
			for cid := range cloudIDs {
				serviceCloudDeps[svcID][cid] = true
			}
		}
	}

	for _, n := range nodes {
		if n.Type != graph.NodeTypeService {
			continue
		}
		if deps, ok := serviceCloudDeps[n.ID]; ok {
			cloudDepNames := make([]string, 0, len(deps))
			for cid := range deps {
				cloudDepNames = append(cloudDepNames, cid)
			}
			n.Metadata.CloudDependencies = cloudDepNames
		}
	}

	// -- Function-level: mark high_cloud_coupling -------------------------
	for funcID, svcSet := range funcCloudCount {
		if len(svcSet) >= 3 {
			if fn, ok := nodeByID[funcID]; ok {
				// Store in AISummary as a structured annotation since we
				// don't have a dedicated bool field.  The downstream AI
				// layer will pick this up.
				if fn.Metadata.AISummary == "" {
					fn.Metadata.AISummary = "high_cloud_coupling:true"
				} else {
					fn.Metadata.AISummary += "; high_cloud_coupling:true"
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
