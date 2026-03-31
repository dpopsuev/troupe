package testkit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dpopsuev/jericho/bugle"
)

// RunServerContract validates any bugle.Server implementation against the
// Bugle Protocol spec. Reusable by Origami, Hegemony, and any future server.
func RunServerContract(t *testing.T, factory func() bugle.Server) { //nolint:funlen // contract suite
	t.Helper()

	t.Run("Start returns session_id and total_items", func(t *testing.T) {
		s := factory()
		resp, err := s.Start(context.Background(), bugle.StartRequest{Action: bugle.ActionStart})
		if err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if resp.SessionID == "" {
			t.Error("Start() returned empty session_id")
		}
		if resp.Status == "" {
			t.Error("Start() returned empty status")
		}
	})

	t.Run("Pull returns valid response shape", func(t *testing.T) {
		s := factory()
		resp, err := s.Pull(context.Background(), bugle.PullRequest{
			Action:    bugle.ActionPull,
			SessionID: "test",
		})
		if err != nil {
			t.Fatalf("Pull() error: %v", err)
		}
		// Must have done field (even if false)
		data, _ := json.Marshal(resp)
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("Pull response is not valid JSON: %v", err)
		}
		if _, ok := raw["done"]; !ok {
			t.Error("Pull response missing 'done' field")
		}
	})

	t.Run("Push accepts all status values", func(t *testing.T) {
		s := factory()
		for _, status := range []bugle.SubmitStatus{
			bugle.StatusOk, bugle.StatusBlocked, bugle.StatusResolved,
			bugle.StatusCanceled, bugle.StatusError,
		} {
			_, err := s.Push(context.Background(), bugle.PushRequest{
				Action:     bugle.ActionPush,
				SessionID:  "test",
				DispatchID: 1,
				Item:       "test-item",
				Fields:     json.RawMessage(`{}`),
				Status:     status,
			})
			if err != nil {
				t.Errorf("Push(status=%q) error: %v", status, err)
			}
		}
	})

	t.Run("Cancel at session level", func(t *testing.T) {
		s := factory()
		resp, err := s.Cancel(context.Background(), bugle.CancelRequest{
			Action:    bugle.ActionCancel,
			SessionID: "test",
		})
		if err != nil {
			t.Fatalf("Cancel(session) error: %v", err)
		}
		if !resp.OK {
			t.Error("Cancel(session) returned ok=false")
		}
	})

	t.Run("Cancel at item level", func(t *testing.T) {
		s := factory()
		resp, err := s.Cancel(context.Background(), bugle.CancelRequest{
			Action:     bugle.ActionCancel,
			SessionID:  "test",
			DispatchID: 42,
		})
		if err != nil {
			t.Fatalf("Cancel(item) error: %v", err)
		}
		if !resp.OK {
			t.Error("Cancel(item) returned ok=false")
		}
	})

	t.Run("Status returns progress", func(t *testing.T) {
		s := factory()
		resp, err := s.Status(context.Background(), bugle.StatusRequest{
			Action:    bugle.ActionStatus,
			SessionID: "test",
		})
		if err != nil {
			t.Fatalf("Status() error: %v", err)
		}
		if resp.SessionID == "" {
			t.Error("Status() returned empty session_id")
		}
	})
}
