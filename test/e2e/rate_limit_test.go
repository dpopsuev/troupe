package e2e_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go/providers"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/arsenal"
	"github.com/dpopsuev/troupe/billing"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/providers"
	"github.com/dpopsuev/troupe/referee"
	"github.com/dpopsuev/troupe/resilience"
)

func TestE2E_RealLLM_ConcurrentCalls_WithRetry(t *testing.T) {
	if os.Getenv("TROUPE_TEST_LIVE_LLM") == "" {
		t.Skip("TROUPE_TEST_LIVE_LLM not set — skipping billable API test")
	}

	provider, err := providers.NewProviderFromEnv("")
	if err != nil {
		t.Fatalf("NewProviderFromEnv: %v", err)
	}

	a, err := arsenal.NewArsenal("")
	if err != nil {
		t.Fatalf("NewArsenal: %v", err)
	}
	source := os.Getenv("TROUPE_PROVIDER")

	tracker := billing.NewTracker()
	sc := referee.Scorecard{
		Name:      "rate_limit_e2e",
		Threshold: 10,
		Rules: []referee.ScorecardRule{
			{On: "dispatch_routed", Weight: 1},
		},
	}
	ref := referee.New(sc)

	b, err := broker.Default(
		broker.WithDriver(noopDriver{}),
		broker.WithProviderResolver(func(_ string) (anyllm.Provider, error) {
			return provider, nil
		}),
		broker.WithTracker(tracker),
		broker.WithReferee(ref),
		broker.WithRetry(resilience.RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   500 * time.Millisecond,
			MaxDelay:    10 * time.Second,
			Jitter:      true,
		}),
	)
	if err != nil {
		t.Fatalf("broker.Default: %v", err)
	}

	picked, err := b.Pick(context.Background(), troupe.Preferences{
		Role: "load-test",
	})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}

	if source != "" {
		filtered, ferr := a.Select("", &arsenal.Preferences{
			Sources: arsenal.Filter{Allow: []string{source}},
		})
		if ferr == nil {
			picked[0].Model = filtered.Model
			picked[0].Provider = filtered.Provider
		}
	}
	t.Logf("Model: %s, Provider: %s", picked[0].Model, picked[0].Provider)

	const concurrency = 10
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		failures  atomic.Int32
	)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			actor, serr := b.Spawn(ctx, picked[0])
			if serr != nil {
				t.Logf("agent-%d spawn failed: %v", idx, serr)
				failures.Add(1)
				return
			}

			maxTokens := 32
			_ = maxTokens
			resp, perr := actor.Perform(ctx, "Reply with exactly one word: hello")
			if perr != nil {
				t.Logf("agent-%d failed: %v", idx, perr)
				failures.Add(1)
				return
			}

			t.Logf("agent-%d: %s", idx, resp)
			successes.Add(1)
		}(i)
	}

	wg.Wait()

	summary := tracker.Summary()
	result := ref.Result()

	t.Logf("Results: %d/%d succeeded, %d failed", successes.Load(), concurrency, failures.Load())
	t.Logf("Billing: %d tokens ($%.4f)", summary.TotalTokens, summary.TotalCostUSD)
	t.Logf("Referee: score=%d pass=%t events=%d", result.Score, result.Pass, len(result.Events))

	if successes.Load() == 0 {
		t.Fatal("all calls failed — no successful responses")
	}
	if failures.Load() > 0 {
		t.Errorf("%d/%d calls failed — retry should have recovered transient errors", failures.Load(), concurrency)
	}
}
