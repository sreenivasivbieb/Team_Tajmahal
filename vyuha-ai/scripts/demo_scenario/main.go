// ---------------------------------------------------------------------------
// scripts/demo_scenario/main.go — Scripted 2-minute incident demo
//
// Usage:
//   go run ./scripts/demo_scenario --server http://localhost:8080
//
// Flags:
//   --server    Base URL of the VYUHA AI server  (default: http://localhost:8080)
//   --duration  Total demo duration in seconds    (default: 120)
//   --service   Service name to target            (default: auto-detect)
// ---------------------------------------------------------------------------
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// ANSI colour helpers
// ---------------------------------------------------------------------------

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
	bgRed   = "\033[41m"
	bgGreen = "\033[42m"
	bgBlue  = "\033[44m"
)

func colour(c, s string) string { return c + s + reset }
func header(phase int, msg string) {
	bar := strings.Repeat("━", 60)
	fmt.Println()
	fmt.Println(colour(dim, bar))
	fmt.Printf("  %s  %s\n", colour(bold+cyan, fmt.Sprintf("Phase %d/5", phase)), colour(bold+white, msg))
	fmt.Println(colour(dim, bar))
}

// ---------------------------------------------------------------------------
// API types (mirrors the backend JSON shapes)
// ---------------------------------------------------------------------------

type graphNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type servicesResp struct {
	Data struct {
		Services []graphNode `json:"services"`
	} `json:"data"`
}

type childrenResp struct {
	Data struct {
		Nodes []graphNode `json:"nodes"`
	} `json:"data"`
}

type logEvent struct {
	Service   string `json:"service"`
	File      string `json:"file"`
	Function  string `json:"function"`
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	Status    string `json:"status"`
	EventType string `json:"event_type"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	LatencyMs int    `json:"latency_ms"`
	Timestamp string `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func getJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func postEvent(serverURL string, ev logEvent) error {
	body, _ := json.Marshal(ev)
	resp, err := http.Post(serverURL+"/api/ingest/log", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ingest returned %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Discovery
// ---------------------------------------------------------------------------

func discoverService(serverURL, serviceFlag string) (graphNode, error) {
	var sr servicesResp
	if err := getJSON(serverURL+"/api/graph/services", &sr); err != nil {
		return graphNode{}, fmt.Errorf("discover services: %w", err)
	}
	if len(sr.Data.Services) == 0 {
		return graphNode{}, fmt.Errorf("no services found — run a scan first")
	}

	// If --service given, find by name.
	if serviceFlag != "" {
		for _, s := range sr.Data.Services {
			if strings.EqualFold(s.Name, serviceFlag) {
				return s, nil
			}
		}
		return graphNode{}, fmt.Errorf("service %q not found", serviceFlag)
	}

	// Auto-pick: prefer the first non-root service.
	for _, s := range sr.Data.Services {
		if s.Type == "service" {
			return s, nil
		}
	}
	return sr.Data.Services[0], nil
}

func discoverFunctions(serverURL string, parent graphNode) ([]graphNode, error) {
	var cr childrenResp
	url := fmt.Sprintf("%s/api/graph/children?parent_id=%s&depth=3", serverURL, parent.ID)
	if err := getJSON(url, &cr); err != nil {
		return nil, fmt.Errorf("discover children: %w", err)
	}

	var funcs []graphNode
	for _, n := range cr.Data.Nodes {
		if n.Type == "function" {
			funcs = append(funcs, n)
		}
	}
	if len(funcs) == 0 {
		return nil, fmt.Errorf("no functions found under service %q", parent.Name)
	}
	return funcs, nil
}

// ---------------------------------------------------------------------------
// Event factory
// ---------------------------------------------------------------------------

func makeEvent(service, funcName, status string, latencyMs int, errMsg string) logEvent {
	ev := logEvent{
		Service:   service,
		Function:  funcName,
		File:      "demo_scenario",
		TraceID:   fmt.Sprintf("demo-%d", rand.Int63()),
		SpanID:    fmt.Sprintf("span-%d", rand.Int63()),
		Status:    status,
		EventType: "request",
		LatencyMs: latencyMs,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if errMsg != "" {
		ev.Error = errMsg
		ev.ErrorCode = "DEMO_ERROR"
	}
	return ev
}

func randBetween(lo, hi int) int {
	if lo >= hi {
		return lo
	}
	return lo + rand.Intn(hi-lo)
}

// ---------------------------------------------------------------------------
// Phase runners
// ---------------------------------------------------------------------------

type phaseConfig struct {
	number     int
	title      string
	titleColor string
	duration   time.Duration
	scale      float64 // relative share of total duration
}

var phases = []phaseConfig{
	{1, "Baseline traffic", green, 0, 0.20},
	{2, "Latency increasing", yellow, 0, 0.15},
	{3, "INCIDENT — failures detected!", red, 0, 0.10},
	{4, "Cascading failures", bgRed + white, 0, 0.20},
	{5, "Recovery in progress", cyan, 0, 0.20},
}

func runDemo(serverURL string, totalDuration time.Duration, service graphNode, funcs []graphNode) {
	svcName := service.Name

	// Classify: first ~30% are "entry" functions, rest are "dependencies".
	entryCount := len(funcs) / 3
	if entryCount < 1 {
		entryCount = 1
	}
	entryFuncs := funcs[:entryCount]
	depFuncs := funcs[entryCount:]
	if len(depFuncs) == 0 {
		depFuncs = funcs // only a few funcs; reuse
	}

	// Pick the specific "failing" function for the incident.
	failFunc := depFuncs[0]

	// Compute phase durations.
	for i := range phases {
		phases[i].duration = time.Duration(float64(totalDuration) * phases[i].scale)
	}

	// Remaining time goes to a cooldown buffer (distribute evenly to last 2 phases).
	var used time.Duration
	for _, p := range phases {
		used += p.duration
	}
	leftover := totalDuration - used
	if leftover > 0 {
		half := leftover / 2
		phases[3].duration += half
		phases[4].duration += leftover - half
	}

	printBanner(svcName, totalDuration, funcs, failFunc)

	eventsSent := 0
	sleep := 500 * time.Millisecond

	// ---- Phase 1 — Baseline ------------------------------------------------
	p := phases[0]
	header(p.number, colour(p.titleColor, p.title))
	deadline := time.Now().Add(p.duration)
	for time.Now().Before(deadline) {
		fn := funcs[rand.Intn(len(funcs))]
		ev := makeEvent(svcName, fn.Name, "success", randBetween(20, 80), "")
		sendAndDot(serverURL, ev, green, &eventsSent)
		time.Sleep(sleep)
	}

	// ---- Phase 2 — Slowdown ------------------------------------------------
	p = phases[1]
	header(p.number, colour(p.titleColor, p.title))
	deadline = time.Now().Add(p.duration)
	for time.Now().Before(deadline) {
		fn := funcs[rand.Intn(len(funcs))]
		ev := makeEvent(svcName, fn.Name, "success", randBetween(200, 800), "")
		sendAndDot(serverURL, ev, yellow, &eventsSent)
		time.Sleep(sleep)
	}

	// ---- Phase 3 — Incident begins -----------------------------------------
	p = phases[2]
	header(p.number, colour(p.titleColor, p.title))
	deadline = time.Now().Add(p.duration)
	for time.Now().Before(deadline) {
		// Mix: 70% errors on failFunc, 30% slow successes on others.
		if rand.Float64() < 0.7 {
			ev := makeEvent(svcName, failFunc.Name, "error", randBetween(3000, 5000),
				"connection timeout: context deadline exceeded")
			sendAndDot(serverURL, ev, red, &eventsSent)
		} else {
			fn := entryFuncs[rand.Intn(len(entryFuncs))]
			ev := makeEvent(svcName, fn.Name, "success", randBetween(500, 1200), "")
			sendAndDot(serverURL, ev, yellow, &eventsSent)
		}
		time.Sleep(sleep)
	}

	// ---- Phase 4 — Cascade -------------------------------------------------
	p = phases[3]
	header(p.number, colour(p.titleColor, p.title))
	deadline = time.Now().Add(p.duration)
	cascadeTargets := depFuncs
	if len(cascadeTargets) > 5 {
		cascadeTargets = cascadeTargets[:5]
	}
	for time.Now().Before(deadline) {
		// failFunc always errors, cascade targets have ~60% error rate, entry funcs degrade.
		roll := rand.Float64()
		switch {
		case roll < 0.35:
			ev := makeEvent(svcName, failFunc.Name, "error", randBetween(5000, 10000),
				"connection timeout: context deadline exceeded")
			sendAndDot(serverURL, ev, red, &eventsSent)
		case roll < 0.70:
			fn := cascadeTargets[rand.Intn(len(cascadeTargets))]
			ev := makeEvent(svcName, fn.Name, "error", randBetween(2000, 6000),
				"upstream dependency failed: "+failFunc.Name)
			sendAndDot(serverURL, ev, red, &eventsSent)
		default:
			fn := entryFuncs[rand.Intn(len(entryFuncs))]
			ev := makeEvent(svcName, fn.Name, "error", randBetween(1000, 3000),
				"internal: cascading failure from "+failFunc.Name)
			sendAndDot(serverURL, ev, magenta, &eventsSent)
		}
		time.Sleep(sleep)
	}

	// ---- Phase 5 — Recovery ------------------------------------------------
	p = phases[4]
	header(p.number, colour(p.titleColor, p.title))
	deadline = time.Now().Add(p.duration)
	elapsed := time.Duration(0)
	for time.Now().Before(deadline) {
		// Recovery probability increases over time.
		progress := float64(elapsed) / float64(p.duration)
		successRate := 0.3 + (progress * 0.7) // 30% → 100%

		fn := funcs[rand.Intn(len(funcs))]
		if rand.Float64() < successRate {
			ev := makeEvent(svcName, fn.Name, "success", randBetween(20, 100), "")
			sendAndDot(serverURL, ev, green, &eventsSent)
		} else {
			ev := makeEvent(svcName, fn.Name, "error", randBetween(1000, 3000),
				"connection timeout: context deadline exceeded")
			sendAndDot(serverURL, ev, red, &eventsSent)
		}
		time.Sleep(sleep)
		elapsed += sleep
	}

	// Final healthy burst.
	for i := 0; i < len(funcs) && i < 10; i++ {
		ev := makeEvent(svcName, funcs[i].Name, "success", randBetween(15, 50), "")
		sendAndDot(serverURL, ev, green, &eventsSent)
		time.Sleep(100 * time.Millisecond)
	}

	printFooter(eventsSent)
}

// ---------------------------------------------------------------------------
// Printing helpers
// ---------------------------------------------------------------------------

func sendAndDot(serverURL string, ev logEvent, dotColour string, count *int) {
	if err := postEvent(serverURL, ev); err != nil {
		fmt.Print(colour(red, "✗"))
		return
	}
	*count++
	// Newline every 60 dots for readability.
	if *count%60 == 0 {
		fmt.Println(colour(dotColour, "·"))
	} else {
		fmt.Print(colour(dotColour, "·"))
	}
}

func printBanner(svcName string, dur time.Duration, funcs []graphNode, failTarget graphNode) {
	bar := strings.Repeat("═", 60)
	fmt.Println()
	fmt.Println(colour(bold+blue, bar))
	fmt.Println(colour(bold+blue, "  ╦  ╦╦ ╦╦ ╦╦ ╦╔═╗   ╔═╗╦  "))
	fmt.Println(colour(bold+blue, "  ╚╗╔╝╚╦╝║ ║╠═╣╠═╣   ╠═╣║  "))
	fmt.Println(colour(bold+blue, "   ╚╝  ╩ ╚═╝╩ ╩╩ ╩   ╩ ╩╩═╝"))
	fmt.Println(colour(bold+blue, bar))
	fmt.Println(colour(bold+white, "  Incident Demo Scenario"))
	fmt.Println()
	fmt.Printf("  %s %s\n", colour(dim, "Target service:"), colour(bold+white, svcName))
	fmt.Printf("  %s %d\n", colour(dim, "Functions:     "), len(funcs))
	fmt.Printf("  %s %s\n", colour(dim, "Fail target:   "), colour(bold+red, failTarget.Name))
	fmt.Printf("  %s %s\n", colour(dim, "Duration:      "), dur)
	fmt.Println()
	fmt.Println(colour(dim, "  Events will be sent every 500ms."))
	fmt.Println(colour(dim, "  Each · is one event. Watch the graph update in real time!"))
	fmt.Println(colour(bold+blue, bar))
}

func printFooter(eventsSent int) {
	fmt.Println()
	bar := strings.Repeat("━", 60)
	fmt.Println(colour(dim, bar))
	fmt.Printf("\n  %s\n", colour(bold+green, "✓ Demo complete. All systems healthy."))
	fmt.Printf("  %s %d events sent\n\n", colour(dim, "Total:"), eventsSent)
	fmt.Println(colour(dim, bar))
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	serverFlag := flag.String("server", "http://localhost:8080", "VYUHA AI server base URL")
	durationFlag := flag.Int("duration", 120, "Total demo duration in seconds")
	serviceFlag := flag.String("service", "", "Service name to target (default: auto-detect)")
	flag.Parse()

	serverURL := strings.TrimRight(*serverFlag, "/")
	totalDuration := time.Duration(*durationFlag) * time.Second

	fmt.Printf("\n  %s %s\n", colour(dim, "Connecting to"), colour(white, serverURL))

	// ---- Step 1: Discover services ----------------------------------------
	service, err := discoverService(serverURL, *serviceFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %s %s\n\n", colour(bold+red, "Error:"), err)
		os.Exit(1)
	}
	fmt.Printf("  %s %s (%s)\n", colour(dim, "Service:"), colour(bold+white, service.Name), service.ID)

	// ---- Step 2: Discover functions ---------------------------------------
	funcs, err := discoverFunctions(serverURL, service)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %s %s\n\n", colour(bold+red, "Error:"), err)
		os.Exit(1)
	}
	fmt.Printf("  %s %d functions discovered\n", colour(dim, "Graph:"), len(funcs))

	// ---- Step 3: Run the demo scenario ------------------------------------
	runDemo(serverURL, totalDuration, service, funcs)
}
