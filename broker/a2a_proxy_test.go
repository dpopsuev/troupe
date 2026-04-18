package broker_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/dpopsuev/troupe"
	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/world"
)

func TestA2AProxy_TwoServers_RoundTrip(t *testing.T) {
	// 1. Start a "remote" A2A agent.
	remoteTransport := transport.NewA2ATransport(a2a.AgentCard{
		Name:               "remote-reviewer",
		Version:            "1.0.0",
		ProtocolVersion:    "1.0",
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		URL:                "http://localhost",
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills:             []a2a.AgentSkill{{ID: "review", Name: "review"}},
	})
	defer remoteTransport.Close()

	_ = remoteTransport.Register("remote-agent", func(_ context.Context, msg transport.Message) (transport.Message, error) {
		return transport.Message{
			Content:      "reviewed: " + msg.Content,
			Performative: "inform",
		}, nil
	})

	remoteServer := httptest.NewServer(remoteTransport.Mux())
	defer remoteServer.Close()

	// 2. Create local Troupe Lobby with A2AProxyFactory.
	w := world.NewWorld()
	localTransport := transport.NewLocalTransport()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:        w,
		Transport:    localTransport,
		ProxyFactory: broker.A2AProxyFactory(),
	})

	// 3. Admit the remote agent via its callback URL.
	id, err := lobby.Admit(context.Background(), troupe.ActorConfig{
		Role:        "reviewer",
		CallbackURL: remoteServer.URL,
	})
	if err != nil {
		t.Fatalf("Admit remote: %v", err)
	}

	// 4. Send a message to the remote agent through local Transport.
	agentID := transport.AgentID(fmt.Sprintf("agent-%d", id))
	resp, err := localTransport.Ask(context.Background(), agentID, transport.Message{
		From:    "local-caller",
		Content: "please review this code",
	})
	if err != nil {
		t.Fatalf("Ask remote via proxy: %v", err)
	}

	if resp.Content != "reviewed: please review this code" {
		t.Fatalf("response = %q, want 'reviewed: please review this code'", resp.Content)
	}

	t.Logf("A2A proxy round-trip: %s", resp.Content)
}

