package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// LogTailer watches a single log file for new JSON-encoded events and
// submits them to an Ingestor for processing.
// ---------------------------------------------------------------------------

// LogTailer tails a log file starting from EOF and forwards new lines to
// the configured Ingestor.
type LogTailer struct {
	filePath string
	ingestor *Ingestor
	done     chan struct{}
	wg       sync.WaitGroup

	// stats
	linesRead  atomic.Int64
	parseErrs  atomic.Int64
	submitErrs atomic.Int64
	startedAt  time.Time
}

// NewLogTailer creates a new tailer for the given file. Call Start to begin
// watching.
func NewLogTailer(filePath string, ingestor *Ingestor) *LogTailer {
	return &LogTailer{
		filePath: filePath,
		ingestor: ingestor,
		done:     make(chan struct{}),
	}
}

// FilePath returns the path being tailed.
func (t *LogTailer) FilePath() string { return t.filePath }

// StartedAt returns when the tailer started.
func (t *LogTailer) StartedAt() time.Time { return t.startedAt }

// LinesRead returns the number of lines read so far.
func (t *LogTailer) LinesRead() int64 { return t.linesRead.Load() }

// Start opens filePath, seeks to the end, and begins polling for new lines.
// It blocks until ctx is cancelled or Stop is called.
func (t *LogTailer) Start(ctx context.Context) error {
	f, err := os.Open(t.filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", t.filePath, err)
	}

	// Seek to end — we only want new lines.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return fmt.Errorf("seek to end of %s: %w", t.filePath, err)
	}

	t.startedAt = time.Now().UTC()
	t.wg.Add(1)

	slog.Info("log tailer started", "file", t.filePath)

	go func() {
		defer t.wg.Done()
		defer f.Close()
		t.readLoop(ctx, f)
	}()

	return nil
}

// Stop signals the tailer to stop and waits for the read loop to finish.
func (t *LogTailer) Stop() {
	select {
	case <-t.done:
		// already closed
	default:
		close(t.done)
	}
	t.wg.Wait()
	slog.Info("log tailer stopped",
		"file", t.filePath,
		"lines_read", t.linesRead.Load(),
	)
}

// readLoop polls the file every 100ms for new lines.
func (t *LogTailer) readLoop(ctx context.Context, f *os.File) {
	reader := bufio.NewReader(f)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// partial accumulates bytes when a line is only partially written
	var partial []byte

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		case <-ticker.C:
			t.drainLines(ctx, reader, &partial)
		}
	}
}

// drainLines reads all available complete lines from reader.
func (t *LogTailer) drainLines(ctx context.Context, reader *bufio.Reader, partial *[]byte) {
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			// Combine with any partial line from previous tick.
			if len(*partial) > 0 {
				line = append(*partial, line...)
				*partial = nil
			}

			// If line doesn't end with newline, it's partial — save for later.
			if line[len(line)-1] != '\n' {
				*partial = line
				return
			}

			t.linesRead.Add(1)
			t.processLine(ctx, line)
		}

		if err != nil {
			// EOF — no more data right now; partial data already saved above.
			if err == io.EOF {
				return
			}
			slog.Error("log tailer read error", "file", t.filePath, "error", err)
			return
		}
	}
}

// processLine parses a single JSON line and submits it.
func (t *LogTailer) processLine(ctx context.Context, line []byte) {
	var event LogEvent
	if err := json.Unmarshal(line, &event); err != nil {
		t.parseErrs.Add(1)
		// Log only occasionally to avoid spam on non-JSON lines.
		if t.parseErrs.Load()%100 == 1 {
			slog.Debug("log tailer: failed to parse JSON line",
				"file", t.filePath,
				"error", err,
				"sample", truncate(string(line), 120),
			)
		}
		return
	}

	// Skip lines with no useful data.
	if event.Service == "" && event.Function == "" {
		return
	}

	if err := t.ingestor.Submit(ctx, event); err != nil {
		t.submitErrs.Add(1)
		slog.Warn("log tailer: submit error",
			"file", t.filePath,
			"error", err,
		)
	}
}

// truncate returns the first n bytes of s (for log messages).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Status returns a snapshot of the tailer's current state.
func (t *LogTailer) Status() TailerStatus {
	return TailerStatus{
		FilePath:   t.filePath,
		Active:     true,
		LinesRead:  t.linesRead.Load(),
		ParseErrs:  t.parseErrs.Load(),
		SubmitErrs: t.submitErrs.Load(),
		StartedAt:  t.startedAt,
		EventCount: t.ingestor.EventCount(),
	}
}

// TailerStatus is a JSON-friendly snapshot of tailer state.
type TailerStatus struct {
	FilePath   string    `json:"file_path"`
	Active     bool      `json:"active"`
	LinesRead  int64     `json:"lines_read"`
	ParseErrs  int64     `json:"parse_errors"`
	SubmitErrs int64     `json:"submit_errors"`
	StartedAt  time.Time `json:"started_at"`
	EventCount int64     `json:"event_count"`
}
