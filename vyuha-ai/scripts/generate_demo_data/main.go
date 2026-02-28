// ===========================================================================
// scripts/generate_demo_data.go — Generate a production-quality demo database
//
// Usage:
//   go run scripts/generate_demo_data.go \
//       --repo-path /tmp/gin \
//       --db-path ./vyuha-demo.db
//
// Prerequisites:
//   git clone https://github.com/gin-gonic/gin /tmp/gin
// ===========================================================================
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/vyuha/vyuha-ai/internal/graph"
	goparser "github.com/vyuha/vyuha-ai/internal/parser/golang"
	"github.com/vyuha/vyuha-ai/internal/storage"
)

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

var (
	repoPath = flag.String("repo-path", "", "Path to a cloned Go repository (e.g. /tmp/gin)")
	dbPath   = flag.String("db-path", "./vyuha-demo.db", "Output SQLite database path")
	seed     = flag.Int64("seed", 42, "Random seed for reproducibility")
)

// ---------------------------------------------------------------------------
// Realistic Go error messages
// ---------------------------------------------------------------------------

var errorMessages = []string{
	"context deadline exceeded",
	"dial tcp 10.0.0.1:5432: connection refused",
	"sql: no rows in result set",
	"EOF",
	"read tcp 10.0.0.1:8080->10.0.0.2:443: i/o timeout",
	"tls: handshake failure",
	"json: cannot unmarshal string into Go value of type int",
	"http: server gave HTTP response to HTTPS client",
	"net/http: request canceled (Client.Timeout exceeded)",
	"redis: connection pool timeout",
}

var errorCodes = []string{
	"ECONNREFUSED",
	"ETIMEOUT",
	"ENOROWS",
	"EEOF",
	"ETLSFAIL",
	"EJSONPARSE",
	"EHTTPFAIL",
	"ECANCELED",
}

var eventTypes = []string{
	"http_request",
	"grpc_call",
	"db_query",
}

func main() {
	flag.Parse()

	if *repoPath == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --repo-path is required")
		fmt.Fprintln(os.Stderr, "Example: go run scripts/generate_demo_data.go --repo-path /tmp/gin")
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(*seed))
	ctx := context.Background()

	// Remove any existing demo DB.
	os.Remove(*dbPath)

	log.Println("══════════════════════════════════════════")
	log.Println("  VYUHA AI — Demo Data Generator")
	log.Println("══════════════════════════════════════════")
	log.Printf("  Repo:  %s", *repoPath)
	log.Printf("  DB:    %s", *dbPath)
	log.Println()

	// =====================================================================
	// Step 1: Parse the repository
	// =====================================================================
	log.Println("[1/6] Parsing repository…")

	parser, err := goparser.New(*repoPath)
	if err != nil {
		log.Fatalf("  ✗ Failed to create parser: %v", err)
	}

	result, err := parser.Parse(ctx, nil, func(done, total int, file string) {
		if done%20 == 0 || done == total {
			log.Printf("  Parsed %d/%d files", done, total)
		}
	})
	if err != nil {
		log.Fatalf("  ✗ Parse failed: %v", err)
	}
	log.Printf("  ✓ Parsed %d files → %d nodes, %d edges",
		result.FileCount, len(result.Nodes), len(result.Edges))
	if len(result.ParseErrors) > 0 {
		log.Printf("  ⚠ %d parse warnings (non-fatal)", len(result.ParseErrors))
	}

	// =====================================================================
	// Step 2: Cloud detection
	// =====================================================================
	log.Println("[2/6] Running cloud detection…")

	cloudDet := goparser.NewCloudDetector(parser.ModulePath(), parser.Fset())
	if err := cloudDet.Detect(ctx, result, result.ASTFiles); err != nil {
		log.Printf("  ⚠ Cloud detection error (non-fatal): %v", err)
	} else {
		// Count cloud nodes.
		cloudCount := 0
		for _, n := range result.Nodes {
			if n.Type == graph.NodeTypeCloudService {
				cloudCount++
			}
		}
		log.Printf("  ✓ Detected %d cloud service nodes", cloudCount)
	}

	// =====================================================================
	// Step 3: Data flow extraction
	// =====================================================================
	log.Println("[3/6] Extracting data flows…")

	dfExtractor := goparser.NewDataFlowExtractor(parser.ModulePath(), parser.Fset())
	dataFlows, err := dfExtractor.ExtractAll(ctx, result, result.ASTFiles)
	if err != nil {
		log.Printf("  ⚠ Data flow extraction error (non-fatal): %v", err)
	} else {
		log.Printf("  ✓ Extracted data flows for %d functions", len(dataFlows))
	}

	// =====================================================================
	// Step 4: Save to database
	// =====================================================================
	log.Println("[4/6] Saving to database…")

	store, err := storage.New(*dbPath)
	if err != nil {
		log.Fatalf("  ✗ Failed to open database: %v", err)
	}
	defer store.Close()

	if err := store.SaveNodes(ctx, result.Nodes); err != nil {
		log.Fatalf("  ✗ Failed to save nodes: %v", err)
	}
	if err := store.SaveEdges(ctx, result.Edges); err != nil {
		log.Fatalf("  ✗ Failed to save edges: %v", err)
	}

	// Save data-flow records.
	dfRecordCount := 0
	for _, fdf := range dataFlows {
		for _, rec := range fdf.DataFlowNodes {
			if err := store.SaveDataFlow(ctx, rec); err != nil {
				log.Printf("  ⚠ data flow save error: %v", err)
				continue
			}
			dfRecordCount++
		}
	}
	log.Printf("  ✓ Saved %d nodes, %d edges, %d data-flow records",
		len(result.Nodes), len(result.Edges), dfRecordCount)

	// =====================================================================
	// Step 5: Inject synthetic runtime events
	// =====================================================================
	log.Println("[5/6] Injecting synthetic runtime events…")

	// Collect function nodes.
	var funcNodes []*graph.Node
	for _, n := range result.Nodes {
		if n.Type == graph.NodeTypeFunction {
			funcNodes = append(funcNodes, n)
		}
	}
	log.Printf("  Found %d function nodes", len(funcNodes))

	if len(funcNodes) == 0 {
		log.Println("  ⚠ No function nodes — skipping runtime event injection")
	} else {
		// Shuffle & partition: 80% healthy, 10% degraded, 10% failing.
		rng.Shuffle(len(funcNodes), func(i, j int) {
			funcNodes[i], funcNodes[j] = funcNodes[j], funcNodes[i]
		})

		degradedStart := int(float64(len(funcNodes)) * 0.80)
		failingStart := int(float64(len(funcNodes)) * 0.90)

		healthy := funcNodes[:degradedStart]
		degraded := funcNodes[degradedStart:failingStart]
		failing := funcNodes[failingStart:]

		// Find the service parent for each function (for service_id).
		serviceOf := buildServiceMap(result.Nodes)

		now := time.Now().UTC()
		sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
		var allEvents []*storage.RuntimeEvent

		// ---- Healthy functions: 50–100 success events each ----
		for _, fn := range healthy {
			count := 50 + rng.Intn(51)
			svcID := serviceOf[fn.ID]
			for i := 0; i < count; i++ {
				ts := randomTimeBetween(rng, sevenDaysAgo, now)
				allEvents = append(allEvents, &storage.RuntimeEvent{
					ID:         uuid.New().String(),
					NodeID:     fn.ID,
					ServiceID:  svcID,
					FunctionID: fn.ID,
					EventType:  eventTypes[rng.Intn(len(eventTypes))],
					Status:     "success",
					LatencyMs:  5 + rng.Intn(46), // 5–50ms
					Timestamp:  ts,
				})
			}
		}

		// ---- Degraded functions: mixed success/error, higher latency ----
		for _, fn := range degraded {
			count := 30 + rng.Intn(41)
			svcID := serviceOf[fn.ID]
			for i := 0; i < count; i++ {
				ts := randomTimeBetween(rng, sevenDaysAgo, now)
				status := "success"
				errMsg := ""
				errCode := ""
				latency := 100 + rng.Intn(401) // 100–500ms
				if rng.Float64() < 0.35 {       // 35% error rate
					status = "error"
					errMsg = errorMessages[rng.Intn(len(errorMessages))]
					errCode = errorCodes[rng.Intn(len(errorCodes))]
					if rng.Float64() < 0.3 {
						status = "timeout"
						errMsg = "context deadline exceeded"
						latency = 5000
					}
				}
				allEvents = append(allEvents, &storage.RuntimeEvent{
					ID:           uuid.New().String(),
					NodeID:       fn.ID,
					ServiceID:    svcID,
					FunctionID:   fn.ID,
					EventType:    eventTypes[rng.Intn(len(eventTypes))],
					Status:       status,
					ErrorMessage: errMsg,
					ErrorCode:    errCode,
					LatencyMs:    latency,
					Timestamp:    ts,
				})
			}
		}

		// ---- Failing functions: mostly errors ----
		for _, fn := range failing {
			count := 40 + rng.Intn(31)
			svcID := serviceOf[fn.ID]
			for i := 0; i < count; i++ {
				ts := randomTimeBetween(rng, sevenDaysAgo, now)
				status := "error"
				errMsg := errorMessages[rng.Intn(len(errorMessages))]
				errCode := errorCodes[rng.Intn(len(errorCodes))]
				latency := 500 + rng.Intn(4501) // 500–5000ms
				if rng.Float64() < 0.15 {        // 15% succeed
					status = "success"
					errMsg = ""
					errCode = ""
					latency = 20 + rng.Intn(80)
				}
				allEvents = append(allEvents, &storage.RuntimeEvent{
					ID:           uuid.New().String(),
					NodeID:       fn.ID,
					ServiceID:    svcID,
					FunctionID:   fn.ID,
					EventType:    eventTypes[rng.Intn(len(eventTypes))],
					Status:       status,
					ErrorMessage: errMsg,
					ErrorCode:    errCode,
					LatencyMs:    latency,
					Timestamp:    ts,
				})
			}
		}

		// Save events in batches.
		batchSize := 500
		for i := 0; i < len(allEvents); i += batchSize {
			end := i + batchSize
			if end > len(allEvents) {
				end = len(allEvents)
			}
			if err := store.SaveRuntimeEvents(ctx, allEvents[i:end]); err != nil {
				log.Fatalf("  ✗ Failed to save runtime events: %v", err)
			}
		}
		log.Printf("  ✓ Injected %d runtime events", len(allEvents))
		log.Printf("    Healthy: %d funcs | Degraded: %d funcs | Failing: %d funcs",
			len(healthy), len(degraded), len(failing))

		// Update node statuses.
		for _, fn := range healthy {
			store.UpdateNodeStatus(ctx, fn.ID, "healthy", 0)
		}
		for _, fn := range degraded {
			store.UpdateNodeStatus(ctx, fn.ID, "degraded", rng.Intn(10)+1)
		}
		for _, fn := range failing {
			store.UpdateNodeStatus(ctx, fn.ID, "error", rng.Intn(30)+10)
		}

		// ================================================================
		// Step 5b: Create trace data (5 traces, 2 with errors)
		// ================================================================
		log.Println("  Creating demo traces…")

		// Pick call chains from edges.
		callChains := buildCallChains(result.Nodes, result.Edges, rng, 5)
		traceCount := 0
		errorTraces := 0
		for i, chain := range callChains {
			traceID := fmt.Sprintf("trace-%04d-%s", i+1, uuid.New().String()[:8])
			hasError := i >= 3 // last 2 traces have errors
			parentSpan := ""

			for j, nodeID := range chain {
				spanID := fmt.Sprintf("span-%s-%02d", traceID[:10], j+1)
				ts := now.Add(-time.Duration(len(chain)-j) * 100 * time.Millisecond)

				status := "success"
				errMsg := ""
				latency := 5 + rng.Intn(50)

				if hasError && j == len(chain)-1 {
					status = "error"
					errMsg = errorMessages[rng.Intn(len(errorMessages))]
					latency = 5000
				}

				ev := &storage.RuntimeEvent{
					ID:           uuid.New().String(),
					NodeID:       nodeID,
					ServiceID:    serviceOf[nodeID],
					FunctionID:   nodeID,
					TraceID:      traceID,
					SpanID:       spanID,
					ParentSpanID: parentSpan,
					EventType:    "http_request",
					Status:       status,
					ErrorMessage: errMsg,
					LatencyMs:    latency,
					Timestamp:    ts,
				}
				allEvents = append(allEvents, ev)
				if err := store.SaveRuntimeEvent(ctx, ev); err != nil {
					log.Printf("  ⚠ trace event save error: %v", err)
				}
				parentSpan = spanID
			}
			traceCount++
			if hasError {
				errorTraces++
			}
		}
		log.Printf("  ✓ Created %d traces (%d with errors)", traceCount, errorTraces)
	}

	// =====================================================================
	// Step 6: Generate embeddings (synthetic)
	// =====================================================================
	log.Println("[6/6] Generating synthetic embeddings…")

	embCount := 0
	for _, n := range result.Nodes {
		if n.Type != graph.NodeTypeFunction {
			continue
		}
		// Create a simple text description for embedding content.
		content := fmt.Sprintf("Function %s in %s: %s",
			n.Name, n.FilePath, n.Metadata.DocComment)
		if n.Metadata.Signature != "" {
			content += "\nSignature: " + n.Metadata.Signature
		}

		// Generate a synthetic 384-dim vector (placeholder).
		// In production, the AI provider generates real embeddings.
		vector := make([]float32, 384)
		for i := range vector {
			vector[i] = rng.Float32()*2 - 1 // [-1, 1]
		}

		emb := &storage.Embedding{
			ID:         uuid.New().String(),
			NodeID:     n.ID,
			Content:    content,
			Vector:     vector,
			Model:      "demo-synthetic-384d",
			Dimensions: 384,
			CreatedAt:  time.Now().UTC(),
		}
		if err := store.SaveEmbedding(ctx, emb); err != nil {
			log.Printf("  ⚠ embedding save error: %v", err)
			continue
		}
		embCount++
	}
	log.Printf("  ✓ Generated %d synthetic embeddings", embCount)

	// =====================================================================
	// Summary
	// =====================================================================
	fmt.Println()
	fmt.Println("══════════════════════════════════════════")
	fmt.Printf("  Demo database ready: %s\n", *dbPath)
	fmt.Printf("  Nodes: %d | Edges: %d\n", len(result.Nodes), len(result.Edges))

	failCount := 0
	for _, n := range result.Nodes {
		if n.RuntimeStatus == "error" {
			failCount++
		}
	}
	fmt.Printf("  Failing nodes: %d (good for demo)\n", failCount)
	fmt.Printf("  Demo traces: 5 (2 with errors)\n")
	fmt.Println("══════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  go run cmd/server/main.go --db-path %s --port 8080\n", *dbPath)
	fmt.Println("  open http://localhost:8080")
}

// ===========================================================================
// Helpers
// ===========================================================================

// buildServiceMap returns a map from nodeID → nearest service ancestor ID.
func buildServiceMap(nodes []*graph.Node) map[string]string {
	idToNode := make(map[string]*graph.Node, len(nodes))
	for _, n := range nodes {
		idToNode[n.ID] = n
	}

	cache := make(map[string]string)
	var find func(id string) string
	find = func(id string) string {
		if v, ok := cache[id]; ok {
			return v
		}
		n, ok := idToNode[id]
		if !ok {
			return ""
		}
		if n.Type == graph.NodeTypeService {
			cache[id] = n.ID
			return n.ID
		}
		if n.ParentID == "" {
			return ""
		}
		svc := find(n.ParentID)
		cache[id] = svc
		return svc
	}
	for _, n := range nodes {
		find(n.ID)
	}
	return cache
}

// randomTimeBetween returns a random Time in [start, end).
func randomTimeBetween(rng *rand.Rand, start, end time.Time) time.Time {
	delta := end.Sub(start)
	offset := time.Duration(rng.Int63n(int64(delta)))
	return start.Add(offset)
}

// buildCallChains walks the "calls" edges to find chains of the requested
// length. Returns up to count chains, each 5–10 nodes long.
func buildCallChains(
	nodes []*graph.Node,
	edges []*graph.Edge,
	rng *rand.Rand,
	count int,
) [][]string {
	// Build adjacency list for "calls" edges.
	adj := make(map[string][]string)
	nodeSet := make(map[string]bool)
	for _, n := range nodes {
		if n.Type == graph.NodeTypeFunction {
			nodeSet[n.ID] = true
		}
	}
	for _, e := range edges {
		if e.Type == graph.EdgeTypeCalls {
			adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
		}
	}

	// Find chains by DFS from random starting points.
	var chains [][]string
	tried := make(map[string]bool)

	// Collect all functions that have outgoing calls.
	var starters []string
	for id := range adj {
		if nodeSet[id] {
			starters = append(starters, id)
		}
	}
	if len(starters) == 0 {
		// Fallback: use any function nodes.
		for id := range nodeSet {
			starters = append(starters, id)
		}
	}
	rng.Shuffle(len(starters), func(i, j int) {
		starters[i], starters[j] = starters[j], starters[i]
	})

	for _, start := range starters {
		if len(chains) >= count {
			break
		}
		if tried[start] {
			continue
		}
		tried[start] = true

		chain := []string{start}
		current := start
		visited := map[string]bool{start: true}

		targetLen := 5 + rng.Intn(6) // 5–10
		for len(chain) < targetLen {
			neighbors := adj[current]
			if len(neighbors) == 0 {
				break
			}
			next := neighbors[rng.Intn(len(neighbors))]
			if visited[next] {
				break
			}
			visited[next] = true
			chain = append(chain, next)
			current = next
		}

		if len(chain) >= 3 { // at least 3 hops
			chains = append(chains, chain)
		}
	}

	return chains
}
