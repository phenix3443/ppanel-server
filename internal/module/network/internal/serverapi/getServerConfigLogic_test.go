package serverapi

import (
	"testing"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
)

func TestCompatibleDoesNotInventLegacyNowhereContract(t *testing.T) {
	logic := &GetServerConfigLogic{}
	if config := logic.compatible(node.Protocol{
		Type: Nowhere, Port: 443, Version: 1, Enable: true, Security: "tls",
		Network: "mix", SNI: "node.example", ALPN: []string{"now/1"}, CertMode: "self",
	}); config != nil {
		t.Fatalf("compatible() = %#v, want nil so callers direct Nowhere nodes to /v2/server/{server_id}", config)
	}
}
