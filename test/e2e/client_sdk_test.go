package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/client"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/world"
)

func TestClientSDK_RegisterAndMessage(t *testing.T) {
	// 1. Start the external agent's A2A server.
	agentTransport := transport.NewA2ATransport(a2a.AgentCard{
		Name: "external-agent",
		URL:  "http://localhost",
	})
	defer agentTransport.Close()

	_ = agentTransport.Register("default", func(_ context.Context, msg transport.Message) (transport.Message, error) {
		return transport.Message{
			Content: "external says: " + msg.Content,
			Role:    transport.RoleAgent,
		}, nil
	})

	agentServer := httptest.NewServer(agentTransport.Mux())
	defer agentServer.Close()

	// 2. Start Troupe server with Lobby + admission endpoint.
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	lobby := broker.NewLobby(broker.LobbyConfig{
		World:        w,
		Transport:    tr,
		ProxyFactory: broker.A2AProxyFactory(),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /admission", broker.AdmissionHandler(lobby))
	troupeServer := httptest.NewServer(mux)
	defer troupeServer.Close()

	// 3. External agent registers via SDK.
	sdk := client.New(troupeServer.URL)
	entityID, err := sdk.Register(context.Background(), "reviewer", agentServer.URL)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if entityID == 0 {
		t.Fatal("expected non-zero entity ID")
	}
	t.Logf("Registered: entity_id=%d", entityID)

	// 4. Verify agent is in the World.
	if !w.Alive(world.EntityID(entityID)) {
		t.Fatal("agent should be alive in World after registration")
	}
	if lobby.Count() != 1 {
		t.Fatalf("lobby count = %d, want 1", lobby.Count())
	}

	// 5. Send message to external agent via transport.
	agentID := transport.AgentID("agent-1")
	task, err := tr.SendMessage(context.Background(), agentID, transport.Message{
		From:    "test",
		Content: "hello external",
		Role:    transport.RoleUser,
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	ch, err := tr.Subscribe(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var response string
	for ev := range ch {
		if ev.State == transport.TaskCompleted && ev.Data != nil {
			response = ev.Data.Content
		}
	}

	if response != "external says: hello external" {
		t.Errorf("response = %q, want 'external says: hello external'", response)
	}
	t.Logf("Round-trip: %q", response)
}
