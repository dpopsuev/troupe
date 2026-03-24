//go:build e2e

package testkit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dpopsuev/bugle/palette"
	"github.com/dpopsuev/bugle/signal"
	"github.com/dpopsuev/bugle/transport"
	"github.com/dpopsuev/bugle/world"
)

func TestStress_10Agents_50Messages(t *testing.T) {
	w, agents := QuickWorld(10, "Stress")
	tr := QuickTransport(w, agents)
	defer tr.Close()

	ctx := context.Background()
	const messagesPerAgent = 50

	var completed atomic.Int64
	var wg sync.WaitGroup

	for i := range agents {
		wg.Add(1)
		go func(sender int) {
			defer wg.Done()
			senderColor := world.Get[palette.ColorIdentity](w, agents[sender])
			for j := range messagesPerAgent {
				target := (sender + j + 1) % len(agents)
				targetColor := world.Get[palette.ColorIdentity](w, agents[target])
				task, err := tr.SendMessage(ctx, targetColor.Short(), transport.Message{
					From:         senderColor.Short(),
					To:           targetColor.Short(),
					Performative: signal.Request,
					Content:      fmt.Sprintf("msg-%d-%d", sender, j),
				})
				if err != nil {
					t.Errorf("SendMessage from %s to %s: %v", senderColor.Short(), targetColor.Short(), err)
					return
				}

				ch, subErr := tr.Subscribe(ctx, task.ID)
				if subErr != nil {
					t.Errorf("Subscribe %s: %v", task.ID, subErr)
					return
				}

				for ev := range ch {
					if ev.State == transport.TaskCompleted {
						completed.Add(1)
					}
				}
			}
		}(i)
	}
	wg.Wait()

	want := int64(len(agents) * messagesPerAgent)
	got := completed.Load()
	if got != want {
		t.Errorf("completed = %d, want %d", got, want)
	}
}
