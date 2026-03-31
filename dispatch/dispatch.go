// Package dispatch provides transport mechanisms for delivering prompts to
// external agents and collecting their artifact responses. It implements the
// Strategy pattern: different dispatchers handle different communication
// channels (stdin, file polling, batch).
package dispatch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

// discardLogger returns a logger that discards all output.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Dispatcher abstracts how a prompt is delivered to an external agent
// and how the resulting artifact is collected back.
type Dispatcher interface {
	// Dispatch delivers the prompt at PromptPath to the external agent and
	// blocks until the artifact appears at ArtifactPath.
	// The context controls cancellation and deadlines for this dispatch call.
	// Returns the raw artifact bytes or an error (e.g. timeout).
	Dispatch(ctx context.Context, dc Context) ([]byte, error)
}

// Context carries all the metadata a dispatcher needs to deliver
// a prompt and collect an artifact.
type Context struct {
	DispatchID    int64         // unique ID assigned by the dispatcher for artifact routing
	CaseID        string        // ground-truth case ID, e.g. "C1"
	Step          string        // circuit step name, e.g. "F0_RECALL"
	PromptPath    string        // absolute path to the filled prompt file
	PromptContent string        // inline prompt text (preferred over PromptPath when set)
	ArtifactPath  string        // absolute path where artifact JSON should appear
	Provider      string        // optional LLM provider name for routing (e.g. "cursor", "codex")
	Timeout       time.Duration // per-dispatch deadline; 0 = no limit (backward compatible)
}

// Finalizer is an optional interface for dispatchers that need post-dispatch
// cleanup (e.g. updating signal files). Components check for this interface
// instead of type-asserting specific dispatcher implementations.
type Finalizer interface {
	MarkDone(artifactPath string)
}

// Unwrapper is implemented by decorator dispatchers (e.g. TokenTrackingDispatcher)
// to expose the inner dispatcher for interface checks.
type Unwrapper interface {
	Inner() Dispatcher
}

// UnwrapFinalizer walks the dispatcher decorator chain and returns the first
// Finalizer found, or nil if none implements it.
func UnwrapFinalizer(d Dispatcher) Finalizer {
	for d != nil {
		if f, ok := d.(Finalizer); ok {
			return f
		}
		if u, ok := d.(Unwrapper); ok {
			d = u.Inner()
			continue
		}
		return nil
	}
	return nil
}

// --- StdinDispatcher (interactive, terminal-based) ---

// StdinTemplate defines the instructional text shown when a prompt is ready.
// Tools provide their own domain-specific instructions while the dispatch
// mechanism remains generic.
type StdinTemplate struct {
	Instructions []string // lines shown between the paths and the prompt
}

// DefaultStdinTemplate returns generic instructions suitable for any tool.
func DefaultStdinTemplate() StdinTemplate {
	return StdinTemplate{
		Instructions: []string{
			"1. Open the prompt file and process it",
			"2. Save the JSON response to the artifact path above",
			"3. Press Enter to continue",
		},
	}
}

// StdinDispatcher delivers prompts by printing a banner to stdout and
// blocking on stdin until the user presses Enter. The banner text is
// controlled by StdinTemplate so domain tools can customize instructions.
type StdinDispatcher struct {
	reader   *bufio.Reader
	template StdinTemplate
}

// NewStdinDispatcher creates a dispatcher that reads from os.Stdin with
// the default template.
func NewStdinDispatcher() *StdinDispatcher {
	return &StdinDispatcher{
		reader:   bufio.NewReader(os.Stdin),
		template: DefaultStdinTemplate(),
	}
}

// NewStdinDispatcherWithTemplate creates a dispatcher with custom instructions.
func NewStdinDispatcherWithTemplate(t StdinTemplate) *StdinDispatcher {
	return &StdinDispatcher{
		reader:   bufio.NewReader(os.Stdin),
		template: t,
	}
}

// Dispatch prints a banner with case/step/paths, blocks on stdin, then reads
// and validates the artifact file.
func (d *StdinDispatcher) Dispatch(_ context.Context, ctx Context) ([]byte, error) { //nolint:gocritic // value receiver for API compat
	fmt.Println()
	fmt.Println("================================================================")
	fmt.Printf("  Case: %-6s  Step: %s\n", ctx.CaseID, ctx.Step)
	fmt.Println("================================================================")
	fmt.Printf("  Prompt:   %s\n", ctx.PromptPath)
	fmt.Printf("  Artifact: %s\n", ctx.ArtifactPath)
	fmt.Println("----------------------------------------------------------------")
	for _, line := range d.template.Instructions {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println("================================================================")
	fmt.Print("  > ")
	_, _ = d.reader.ReadString('\n')

	data, err := os.ReadFile(ctx.ArtifactPath)
	if err != nil {
		return nil, fmt.Errorf("artifact not found at %s: %w", ctx.ArtifactPath, err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", ctx.ArtifactPath, err)
	}

	fmt.Printf("  Read artifact (%d bytes)\n", len(data))
	return raw, nil
}
