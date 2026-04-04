package protocol

import (
	"encoding/json"
	"testing"
)

func TestPullRequest_MarshalRoundTrip(t *testing.T) {
	req := PullRequest{
		SessionID: "sess-1",
		WorkerID:  "worker-1",
		TimeoutMs: 30000,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded PullRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SessionID != req.SessionID {
		t.Errorf("SessionID = %q, want %q", decoded.SessionID, req.SessionID)
	}
	if decoded.WorkerID != req.WorkerID {
		t.Errorf("WorkerID = %q, want %q", decoded.WorkerID, req.WorkerID)
	}
}

func TestPushRequest_MarshalRoundTrip(t *testing.T) {
	req := PushRequest{
		SessionID:  "sess-1",
		WorkerID:   "worker-1",
		DispatchID: 42,
		Item:       "triage",
		Status:     StatusOk,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded PushRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.DispatchID != 42 {
		t.Errorf("DispatchID = %d, want 42", decoded.DispatchID)
	}
	if decoded.Item != "triage" {
		t.Errorf("Item = %q, want triage", decoded.Item)
	}
}

func TestAndon_Levels(t *testing.T) {
	levels := []AndonLevel{AndonNominal, AndonDegraded, AndonFailure, AndonBlocked, AndonDead}
	for _, l := range levels {
		if l == "" {
			t.Error("empty andon level")
		}
		_, ok := PriorityOf(l)
		if !ok {
			t.Errorf("PriorityOf(%s) not found", l)
		}
	}
}

func TestAndon_Worse(t *testing.T) {
	nom, _ := PriorityOf(AndonNominal)
	deg, _ := PriorityOf(AndonDegraded)
	fail, _ := PriorityOf(AndonFailure)

	if !Worse(deg, nom) {
		t.Error("degraded should be worse than nominal")
	}
	if !Worse(fail, deg) {
		t.Error("failure should be worse than degraded")
	}
	if Worse(nom, fail) {
		t.Error("nominal should not be worse than failure")
	}
}

func TestDefaultCapabilities(t *testing.T) {
	caps := DefaultCapabilities()
	if caps.Protocol != ProtocolVersion {
		t.Errorf("Protocol = %q, want %q", caps.Protocol, ProtocolVersion)
	}
}

func TestProtocolVersion_NotBugle(t *testing.T) {
	if ProtocolVersion == "bugle/v1" {
		t.Error("ProtocolVersion still says bugle — should be jericho")
	}
}

func TestDefaultToolName_NotBugle(t *testing.T) {
	if DefaultToolName == "bugle" {
		t.Error("DefaultToolName still says bugle — should be circuit")
	}
}
