package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lsprdev/Navego/internal/approval"
	"github.com/lsprdev/Navego/internal/browser"
	"github.com/lsprdev/Navego/internal/takeover"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCatalog returns the same tool descriptors used by every browser worker.
// The control plane uses them to expose one multi-browser MCP without copying
// schemas by hand. The placeholder controller is never called.
func ToolCatalog(ctx context.Context, logger *slog.Logger) ([]*mcp.Tool, error) {
	server := New(
		unavailableController{},
		takeover.New(),
		approval.NewStore(2*time.Minute),
		"",
		logger,
		WithAuthorization(Authorization{Enabled: true}),
	)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "navego-catalog", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, err
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return listed.Tools, nil
}

type unavailableController struct{}

var errCatalogController = errors.New("catalog controller cannot execute browser operations")

func (unavailableController) Status(context.Context) (browser.Status, error) {
	return browser.Status{}, errCatalogController
}
func (unavailableController) Open(context.Context, string) (browser.Snapshot, error) {
	return browser.Snapshot{}, errCatalogController
}
func (unavailableController) Snapshot(context.Context) (browser.Snapshot, error) {
	return browser.Snapshot{}, errCatalogController
}
func (unavailableController) Click(context.Context, string) (browser.Snapshot, error) {
	return browser.Snapshot{}, errCatalogController
}
func (unavailableController) Type(context.Context, string, string, bool) (browser.Snapshot, error) {
	return browser.Snapshot{}, errCatalogController
}
func (unavailableController) Screenshot(context.Context, bool) ([]byte, string, error) {
	return nil, "", errCatalogController
}
func (unavailableController) PDF(context.Context) ([]byte, string, error) {
	return nil, "", errCatalogController
}
func (unavailableController) DescribeAction(context.Context, string) (browser.ActionTarget, error) {
	return browser.ActionTarget{}, errCatalogController
}
func (unavailableController) CommitAction(context.Context, browser.ActionTarget) (browser.Snapshot, error) {
	return browser.Snapshot{}, errCatalogController
}
func (unavailableController) Close() error { return nil }
