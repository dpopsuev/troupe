package testkit

import (
	"testing"

	"github.com/dpopsuev/jericho/bugle"
)

func TestServerContract_MockServer(t *testing.T) {
	RunServerContract(t, func() bugle.Server {
		return NewMockServer()
	})
}
