package control

import (
	"strings"
	"testing"

	"github.com/lsprdev/Navego/internal/mcpserver"
)

func TestPublicMCPUsesScopedLogoutAuthorization(t *testing.T) {
	if !strings.Contains(multiBrowserInstructions, mcpserver.LogoutInstructions) {
		t.Fatal("public MCP must include the worker's scoped logout policy")
	}
	if strings.Contains(multiBrowserInstructions, "deletions, logout,") {
		t.Fatal("public MCP still requires a new confirmation for every logout")
	}
	if !strings.Contains(multiBrowserInstructions, "Purchases, payments, deletions, and similarly high-impact effects always require fresh confirmation") {
		t.Fatal("other high-impact actions must retain fresh confirmation")
	}
}
