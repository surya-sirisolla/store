// Command mcp exposes the business directory as Model Context Protocol tools so
// the PicoClaw WhatsApp agent can answer customers' questions from the owner's
// data. It reuses the same store/db packages as the HTTP API and talks to the
// same Postgres. Served over streamable HTTP so PicoClaw connects over the
// docker network (configure picoclaw mcp.servers.* with type:"http").
//
// Modeled on the Signet reference backend's cmd/mcp/main.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"store/backend/internal/config"
	"store/backend/internal/db"
	"store/backend/internal/models"
	"store/backend/internal/store"
)

// callerPhone reads the trusted "_caller" argument that PicoClaw injects from
// the WhatsApp-authenticated sender. Empty if the call came from a channel that
// doesn't supply it. The model cannot set this argument.
func callerPhone(req mcp.CallToolRequest) string {
	return strings.TrimSpace(req.GetString("_caller", ""))
}

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	st := store.New(database)

	s := server.NewMCPServer("store-directory", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(
			"Tools to answer customer questions for this business from its directory. "+
				"Use get_business_info for address/timings/contact; search_listings to find "+
				"items/services/listings by keyword or category; list_categories to help the "+
				"customer browse; request_alert ONLY after a customer explicitly agrees to be "+
				"notified about something currently unavailable and gives their name and phone."),
	)

	registerTools(s, st)

	addr := ":" + getEnv("MCP_PORT", "9090")
	log.Printf("store-mcp (streamable HTTP) listening on %s/mcp", addr)
	if err := server.NewStreamableHTTPServer(s).Start(addr); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func registerTools(s *server.MCPServer, st *store.Store) {
	// get_business_info ─────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("get_business_info",
		mcp.WithDescription("Get the business profile: name, address, area, city, opening hours, phone/WhatsApp and services offered."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logActivity(st, "get_business_info", "", "profile requested", callerPhone(req))
		p, err := st.GetBusinessProfile(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if p == nil {
			return mcp.NewToolResultError("business profile not set yet"), nil
		}
		return jsonResult(p)
	})

	// search_listings ────────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("search_listings",
		mcp.WithDescription("Search the business's directory. Match listings by free-text keyword and/or an exact category. Returns matching listings with phone, address, description and any extra fields."),
		mcp.WithString("query", mcp.Description("Free-text keyword, e.g. 'ceiling fan', 'cardiologist', '2.5 sq mm wire'.")),
		mcp.WithString("category", mcp.Description("Exact category filter. Use list_categories for valid values.")),
		mcp.WithString("limit", mcp.Description("Max results as a string, e.g. \"20\" (default 20).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		items, err := st.SearchListings(ctx, store.SearchParams{
			Query:    query,
			Category: req.GetString("category", ""),
			Limit:    int(parseIntArg(req.GetString("limit", ""), 20)),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "search_listings", query, fmt.Sprintf("%d results", len(items)), callerPhone(req))
		return jsonResult(map[string]any{"count": len(items), "listings": items})
	})

	// list_categories ─────────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("list_categories",
		mcp.WithDescription("List all categories with how many listings each contains, to help the customer browse."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cats, err := st.CategoryCounts(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "list_categories", "", fmt.Sprintf("%d categories", len(cats)), callerPhone(req))
		return jsonResult(map[string]any{"count": len(cats), "categories": cats})
	})

	// request_alert ───────────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("request_alert",
		mcp.WithDescription("Record a customer's request to be notified when something the business can't currently supply becomes available. Call this ONLY after the customer explicitly agrees and gives their name and phone number."),
		mcp.WithString("item_query", mcp.Required(), mcp.Description("What the customer asked for, in plain words, e.g. 'Havells ceiling fan'.")),
		mcp.WithString("customer_name", mcp.Required(), mcp.Description("The customer's name, as they gave it.")),
		mcp.WithString("customer_phone", mcp.Required(), mcp.Description("The phone number to alert.")),
		mcp.WithString("category", mcp.Description("Category, if known.")),
		mcp.WithString("availability", mcp.Description("'out_of_stock' (carried but none) or 'not_carried' (not stocked).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		item, err := req.RequireString("item_query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := req.RequireString("customer_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		phone, err := req.RequireString("customer_phone")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		id, err := st.CreateAlertRequest(ctx, models.AlertRequest{
			CustomerName:  strings.TrimSpace(name),
			CustomerPhone: strings.TrimSpace(phone),
			ItemQuery:     strings.TrimSpace(item),
			Category:      strings.TrimSpace(req.GetString("category", "")),
			Availability:  strings.TrimSpace(req.GetString("availability", "")),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "request_alert", item, "alert logged", strings.TrimSpace(phone))
		return jsonResult(map[string]any{"ok": true, "request_id": id, "message": "Alert request logged."})
	})

	registerStaffTools(s, st)
}

// registerStaffTools adds tools that only staff/owner numbers may use. Each
// checks the trusted "_caller" against the staff list server-side, so a
// customer who learns the tool names still gets nothing.
func registerStaffTools(s *server.MCPServer, st *store.Store) {
	denyIfNotStaff := func(ctx context.Context, req mcp.CallToolRequest) *mcp.CallToolResult {
		caller := callerPhone(req)
		if caller == "" || !st.IsStaffPhone(ctx, caller) {
			return mcp.NewToolResultError("This information is only available to staff. Ask the owner to add your WhatsApp number under Staff.")
		}
		return nil
	}

	// staff_recent_contacts ─────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("staff_recent_contacts",
		mcp.WithDescription("STAFF ONLY: list the people (phone numbers) who recently messaged the bot, with how many times and what they last asked. Use when a staff member asks who has been trying to reach the business."),
		mcp.WithString("limit", mcp.Description("Max contacts as a string, e.g. \"20\" (default 20).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if denied := denyIfNotStaff(ctx, req); denied != nil {
			return denied, nil
		}
		contacts, err := st.RecentContacts(ctx, int(parseIntArg(req.GetString("limit", ""), 20)), time.Time{})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "staff_recent_contacts", "", fmt.Sprintf("%d contacts", len(contacts)), callerPhone(req))
		return jsonResult(map[string]any{"count": len(contacts), "contacts": contacts})
	})

	// staff_pending_alerts ──────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("staff_pending_alerts",
		mcp.WithDescription("STAFF ONLY: list pending customer alert requests (the waitlist) — people who wanted to be notified when something becomes available. Use when a staff member asks who is waiting or who wanted to reach them."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if denied := denyIfNotStaff(ctx, req); denied != nil {
			return denied, nil
		}
		alerts := []models.AlertRequest{}
		st.DB().WithContext(ctx).Where("status = ?", "logged").Order("created_at DESC").Limit(50).Find(&alerts)
		logActivity(st, "staff_pending_alerts", "", fmt.Sprintf("%d pending", len(alerts)), callerPhone(req))
		return jsonResult(map[string]any{"count": len(alerts), "alerts": alerts})
	})
}

// logActivity records a bot tool call for the owner's monitoring view. Failures
// are non-fatal — monitoring must never break a customer's answer.
func logActivity(st *store.Store, tool, query, summary, phone string) {
	_ = st.LogActivity(context.Background(), models.BotActivity{
		Tool:          tool,
		Query:         query,
		ResultSummary: summary,
		CustomerPhone: phone,
		CreatedAt:     time.Now(),
	})
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

func parseIntArg(s string, def int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
