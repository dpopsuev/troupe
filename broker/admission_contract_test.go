package broker_test

import (
	"testing"

	"github.com/dpopsuev/troupe/broker"
	"github.com/dpopsuev/troupe/internal/transport"
	"github.com/dpopsuev/troupe/testkit"
	"github.com/dpopsuev/troupe/world"
)

func TestLobby_AdmissionContract(t *testing.T) {
	w := world.NewWorld()
	tr := transport.NewLocalTransport()
	log := testkit.NewStubEventLog()

	lobby := broker.NewLobby(broker.LobbyConfig{
		World:      w,
		Transport:  tr,
		ControlLog: log,
	})

	testkit.RunAdmissionContract(t, testkit.AdmissionTestDeps{
		Admission:  lobby,
		ControlLog: log,
		WorldCount: w.Count,
	})
}
