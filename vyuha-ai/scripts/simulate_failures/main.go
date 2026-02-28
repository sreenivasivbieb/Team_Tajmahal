// ===========================================================================
// scripts/simulate_failures.go — Live incident simulator for demos
//
// Sends log events to a running VYUHA server to simulate a real-time
// incident with three phases: normal → failure → recovery.
//
// Usage:
//   go run scripts/simulate_failures.go \
//       --server http://localhost:8080 \
//       --target "function:github.com/gin-gonic/gin/context.go::(*Context).JSON" \
//       --rate 2
//
// Tip: To find valid node IDs, call:
//   curl http://localhost:8080/api/graph/search?q=JSON&type=function
// ===========================================================================
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

var (
	server = flag.String("server", "http://localhost:8080", "VYUHA server URL")
	target = flag.String("target", "", "Node ID of the function that will start failing (required)")
	rate   = flag.Int("rate", 2, "Events per second")
	phases = flag.String("phases", "30,30,30", "Phase durations in seconds: normal,incident,recovery")
)

// ---------------------------------------------------------------------------
// Types (mirrors the server's ingest API)
// ---------------------------------------------------------------------------

type LogEvent struct {
	NodeID       string `json:"node_id"`
	EventType    string `json:"event_type"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	LatencyMs    int    `json:"latency_ms,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Error templates
// ---------------------------------------------------------------------------

var incidentErrors = []string{
	"context deadline exceeded",
	"context deadline exceeded",
	"context deadline exceeded",
	"dial tcp 10.0.0.1:5432: connection refused",
	"read tcp: i/o timeout",
}

func main() {
	flag.Parse()

	if *target == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --target is required")
		fmt.Fprintln(os.Stderr, "  Pass a node ID of a function that should fail.")
		fmt.Fprintln(os.Stderr, "  To discover node IDs:")
		fmt.Fprintf(os.Stderr, "    curl %s/api/graph/search?q=JSON&type=function\n", *server)
		os.Exit(1)
	}

	// Parse phase durations.
	phaseDurations := parsePhaseDurations(*phases)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Discover callers of the target (for cascade degradation).
	callers := discoverCallers(*server, *target)
	fmt.Printf("Target:  %s\n", *target)
	fmt.Printf("Callers: %d upstream functions\n", len(callers))
	fmt.Printf("Rate:    %d events/sec\n", *rate)
	fmt.Printf("Phases:  normal=%ds → incident=%ds → recovery=%ds\n",
		phaseDurations[0], phaseDurations[1], phaseDurations[2])
	fmt.Println()

	allNodes := append([]string{*target}, callers...)
	ticker := time.NewTicker(time.Second / time.Duration(*rate))
	defer ticker.Stop()

	start := time.Now()
	phase1End := start.Add(time.Duration(phaseDurations[0]) * time.Second)
	phase2End := phase1End.Add(time.Duration(phaseDurations[1]) * time.Second)
	phase3End := phase2End.Add(time.Duration(phaseDurations[2]) * time.Second)

	eventCount := 0

	for now := range ticker.C {
		if now.After(phase3End) {
			break
		}

		var event LogEvent

		switch {
		// -------- Phase 1: Normal operation --------
		case now.Before(phase1End):
			elapsed := now.Sub(start).Seconds()
			phase := "NORMAL"
			nodeID := allNodes[rng.Intn(len(allNodes))]
			event = LogEvent{
				NodeID:    nodeID,
				EventType: "http_request",
				Status:    "success",
				LatencyMs: 5 + rng.Intn(46),
			}
			printPhase(phase, elapsed, phaseDurations[0], event)

		// -------- Phase 2: Incident — target fails, callers degrade --------
		case now.Before(phase2End):
			elapsed := now.Sub(phase1End).Seconds()
			phase := "\033[31mINCIDENT\033[0m"

			if rng.Float64() < 0.5 {
				// Target fails
				errMsg := incidentErrors[rng.Intn(len(incidentErrors))]
				event = LogEvent{
					NodeID:       *target,
					EventType:    "http_request",
					Status:       "error",
					ErrorMessage: errMsg,
					ErrorCode:    "ETIMEOUT",
					LatencyMs:    5000,
				}
			} else if len(callers) > 0 {
				// Caller degrades
				caller := callers[rng.Intn(len(callers))]
				if rng.Float64() < 0.6 {
					event = LogEvent{
						NodeID:       caller,
						EventType:    "http_request",
						Status:       "error",
						ErrorMessage: "upstream dependency timeout",
						ErrorCode:    "EUPSTREAM",
						LatencyMs:    3000 + rng.Intn(2000),
					}
				} else {
					event = LogEvent{
						NodeID:    caller,
						EventType: "http_request",
						Status:    "success",
						LatencyMs: 200 + rng.Intn(800), // slow but ok
					}
				}
			} else {
				event = LogEvent{
					NodeID:       *target,
					EventType:    "http_request",
					Status:       "error",
					ErrorMessage: "context deadline exceeded",
					ErrorCode:    "ETIMEOUT",
					LatencyMs:    5000,
				}
			}
			printPhase(phase, elapsed, phaseDurations[1], event)

		// -------- Phase 3: Recovery --------
		default:
			elapsed := now.Sub(phase2End).Seconds()
			phase := "\033[32mRECOVERY\033[0m"
			nodeID := allNodes[rng.Intn(len(allNodes))]

			// Gradually improve — early in recovery still some errors.
			recoveryProgress := elapsed / float64(phaseDurations[2])
			if rng.Float64() > recoveryProgress && rng.Float64() < 0.3 {
				event = LogEvent{
					NodeID:       nodeID,
					EventType:    "http_request",
					Status:       "error",
					ErrorMessage: "connection reset by peer",
					LatencyMs:    1000 + rng.Intn(2000),
				}
			} else {
				event = LogEvent{
					NodeID:    nodeID,
					EventType: "http_request",
					Status:    "success",
					LatencyMs: 5 + rng.Intn(50),
				}
			}
			printPhase(phase, elapsed, phaseDurations[2], event)
		}

		if err := sendEvent(*server, event); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ Send failed: %v\n", err)
		}
		eventCount++
	}

	fmt.Println()
	fmt.Println("════════════════════════════════════════════")
	fmt.Printf("  Simulation complete: %d events sent\n", eventCount)
	fmt.Println("════════════════════════════════════════════")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sendEvent(serverURL string, event LogEvent) error {
	body, _ := json.Marshal(event)
	resp, err := http.Post(
		serverURL+"/api/ingest/log",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func discoverCallers(serverURL, targetID string) []string {
	// Try to get the node detail which includes callers.
	url := fmt.Sprintf("%s/api/graph/node/%s", serverURL, targetID)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Could not discover callers: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "  ⚠ Node lookup returned %d — will simulate target only\n", resp.StatusCode)
		return nil
	}

	var detail struct {
		Callers []struct {
			ID string `json:"id"`
		} `json:"callers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil
	}

	var callerIDs []string
	for _, c := range detail.Callers {
		callerIDs = append(callerIDs, c.ID)
	}
	return callerIDs
}

func printPhase(phase string, elapsed float64, total int, event LogEvent) {
	statusIcon := "✓"
	if event.Status != "success" {
		statusIcon = "✗"
	}
	// Truncate node ID for display.
	nodeShort := event.NodeID
	if len(nodeShort) > 50 {
		parts := strings.Split(nodeShort, "/")
		if len(parts) > 2 {
			nodeShort = "…/" + strings.Join(parts[len(parts)-2:], "/")
		}
	}
	fmt.Printf("  [%s %4.0fs/%ds] %s %s → %s (%dms)\n",
		phase, elapsed, total, statusIcon, nodeShort, event.Status, event.LatencyMs)
}

func parsePhaseDurations(s string) [3]int {
	durations := [3]int{30, 30, 30}
	parts := strings.Split(s, ",")
	for i := 0; i < 3 && i < len(parts); i++ {
		var d int
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[i]), "%d", &d); err == nil && d > 0 {
			durations[i] = d
		}
	}
	return durations
}
