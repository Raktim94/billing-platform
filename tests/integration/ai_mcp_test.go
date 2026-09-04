//go:build integration

// Stage 9's highest-priority test per the project brief: an MCP client is
// treated as an untrusted external actor (brief §40) exactly like any
// other API-key-authenticated caller. These tests exercise the REAL wire
// path — a genuine mcp.Server (with internal/modules/ai's Toolset
// registered) connected to a genuine mcp.Client over an in-memory
// transport — not a bypass of the transport layer, so a bug in how
// ai.Toolset wires scopedCtx would actually be caught here.
package integration

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rechvix/internal/modules/ai"
	catalogueapp "rechvix/internal/modules/catalogue/app"
	contactsapp "rechvix/internal/modules/contacts/app"
	contactspg "rechvix/internal/modules/contacts/pg"
	identityapp "rechvix/internal/modules/identity/app"
	identitydomain "rechvix/internal/modules/identity/domain"
	inventoryapp "rechvix/internal/modules/inventory/app"
	reportingapp "rechvix/internal/modules/reporting/app"
	salesapp "rechvix/internal/modules/sales/app"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/permissions"
)

// connectMCP wires an in-process mcp.Server (with toolset registered) to
// a real mcp.Client over mcp.NewInMemoryTransports, and returns a
// connected ClientSession ready for CallTool.
func connectMCP(t *testing.T, ctx context.Context, toolset *ai.Toolset) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	toolset.Register(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// mcpTestServices builds one instance of every service internal/modules/ai
// needs, exactly as apps/mcp/main.go composes them.
type mcpTestServices struct {
	identity  *identityapp.Service
	catalogue *catalogueapp.Service
	contacts  *contactsapp.Service
	inventory *inventoryapp.Service
	sales     *salesapp.Service
	reporting *reportingapp.Service
}

func newMCPTestServices(t *testing.T, salesSvc *salesapp.Service, inventorySvc *inventoryapp.Service, reportingSvc *reportingapp.Service) mcpTestServices {
	t.Helper()
	identitySvc, _ := newTestIdentityService(t)
	checker := permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool)
	recorder := audit.NewPGRecorder(sharedPool)
	contactsSvc := contactsapp.NewService(sharedPool, contactspg.NewPartyRepo(sharedPool),
		contactspg.NewAddressRepo(sharedPool), contactspg.NewTaxRegistrationRepo(sharedPool), checker, recorder)
	return mcpTestServices{
		identity: identitySvc, catalogue: newTestCatalogueService(t), contacts: contactsSvc,
		inventory: inventorySvc, sales: salesSvc, reporting: reportingSvc,
	}
}

// toolsetForKey resolves rawKey (as apps/mcp/main.go does at startup) and
// builds the same Toolset an MCP server process would run for it.
func (s mcpTestServices) toolsetForKey(t *testing.T, ctx context.Context, rawKey string) *ai.Toolset {
	t.Helper()
	principal, scopes, err := s.identity.ValidateAPIKey(ctx, rawKey, "")
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	scopeStrings := make([]string, len(scopes))
	for i, sc := range scopes {
		scopeStrings[i] = string(sc)
	}
	return ai.NewToolset(principal, permissions.PermissionsForScopes(scopeStrings),
		s.catalogue, s.contacts, s.inventory, s.sales, s.reporting, audit.NewPGRecorder(sharedPool))
}

func TestMCP_ScopedAPIKey_AllowsInScopeToolsOnly(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, accountingSvc, inventorySvc := newTestAccountingServices(t)
	reportingSvc := newTestReportingService(t, accountingSvc)
	svcs := newMCPTestServices(t, salesSvc, inventorySvc, reportingSvc)

	fx := setupAccountingFixture(t, ctx, accountingSvc)
	doc := finalizeSimpleTaxInvoice(t, ctx, salesSvc, fx, "5", "200")

	// A key scoped ONLY to inventory:read — brief Scenario M's exact
	// setup ("AI MCP user has inventory:read only").
	created, err := svcs.identity.CreateAPIKey(ctx, fx.Principal, identityapp.CreateAPIKeyParams{
		Name: "mcp-inventory-only", Scopes: []identitydomain.APIScope{identitydomain.ScopeInventoryRead},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	toolset := svcs.toolsetForKey(t, ctx, created.RawKey)
	session := connectMCP(t, ctx, toolset)

	// CAN read stock (brief: "it can read stock").
	invRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_inventory",
		Arguments: map[string]any{"warehouse_id": fx.WarehouseID.String(), "product_variant_id": fx.VariantID.String()},
	})
	if err != nil {
		t.Fatalf("CallTool(get_inventory) transport error: %v", err)
	}
	if invRes.IsError {
		t.Fatalf("get_inventory: expected success for inventory:read scope, got tool error: %+v", invRes.Content)
	}

	// CANNOT read invoices ("it cannot: create invoice... access another
	// organisation, retrieve credentials" — this is the read-side
	// equivalent: no invoices:read scope means no invoice access at all,
	// not a silently-empty result).
	docRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_invoice",
		Arguments: map[string]any{"document_id": doc.ID.String()},
	})
	if err != nil {
		t.Fatalf("CallTool(get_invoice) transport error: %v", err)
	}
	if !docRes.IsError {
		t.Fatal("get_invoice: expected a tool error for a key with no invoices:read scope, got success")
	}

	listRes, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_invoices", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(list_invoices) transport error: %v", err)
	}
	if !listRes.IsError {
		t.Fatal("list_invoices: expected a tool error for a key with no invoices:read scope, got success")
	}
}

func TestMCP_InvoicesScopedKey_CannotReadAnotherOrganisationsInvoice(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, accountingSvc, inventorySvc := newTestAccountingServices(t)
	reportingSvc := newTestReportingService(t, accountingSvc)
	svcs := newMCPTestServices(t, salesSvc, inventorySvc, reportingSvc)

	fxA := setupAccountingFixture(t, ctx, accountingSvc)
	docA := finalizeSimpleTaxInvoice(t, ctx, salesSvc, fxA, "1", "50")

	fxB := setupAccountingFixture(t, ctx, accountingSvc) // a second, unrelated organisation
	docB := finalizeSimpleTaxInvoice(t, ctx, salesSvc, fxB, "1", "999")

	// Org A issues itself an invoices:read key.
	created, err := svcs.identity.CreateAPIKey(ctx, fxA.Principal, identityapp.CreateAPIKeyParams{
		Name: "mcp-invoices-A", Scopes: []identitydomain.APIScope{identitydomain.ScopeInvoicesRead},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	toolset := svcs.toolsetForKey(t, ctx, created.RawKey)
	session := connectMCP(t, ctx, toolset)

	// Org A's key can read org A's own invoice.
	okRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_invoice", Arguments: map[string]any{"document_id": docA.ID.String()},
	})
	if err != nil {
		t.Fatalf("CallTool(get_invoice, own doc) transport error: %v", err)
	}
	if okRes.IsError {
		t.Fatalf("expected org A's key to read its own invoice, got tool error: %+v", okRes.Content)
	}

	// The get_invoice tool schema has NO organisation_id argument at all
	// (internal/modules/ai/tools.go) — this proves there is no field a
	// malicious/buggy client could even attempt to pass to widen scope.
	// What IS passable is another organisation's document_id, which must
	// fail closed (RLS + GetDocument's own org-scoped query), never
	// silently return org B's real ₹999 invoice to org A's key.
	crossRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_invoice", Arguments: map[string]any{"document_id": docB.ID.String()},
	})
	if err != nil {
		t.Fatalf("CallTool(get_invoice, cross-org doc) transport error: %v", err)
	}
	if !crossRes.IsError {
		t.Fatalf("SECURITY FAILURE: org A's MCP key read org B's invoice (id=%s) by passing its document_id as an argument", docB.ID)
	}
}
