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
	"net/http"
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

// ownerCaller is the sentinel "_caller" the PicoClaw console channel injects for
// the owner web chat. It unlocks the staff-only tools without requiring the
// owner to register a phone. MUST stay in sync with the sentinel in
// picoclaw/pkg/channels/console/console.go. Safe because the MCP server is only
// reachable on the docker network and "_caller" is set by the trusted channel,
// not the model.
const ownerCaller = "console-owner"

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
	// One deployment serves one business, so the bot reads the whole catalog —
	// there is no tenant to resolve and therefore no way to bind to the wrong one.
	st := store.New(database)

	s := server.NewMCPServer("store-directory", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithInstructions(
			"Tools to answer customer questions for this business from its directory. "+
				"Use get_business_info for address/timings/contact; search_listings to find "+
				"items/services/listings by keyword or category; list_categories to help the "+
				"customer browse. When something isn't found or is out of stock, offer to notify "+
				"the customer, and call request_alert once they agree — their WhatsApp number is "+
				"filled in automatically, so you don't need to ask for it."),
	)

	registerTools(s, st)

	// Serve the MCP endpoint alongside a plain health probe. PicoClaw connects
	// once at startup and does NOT retry if the MCP server isn't listening yet —
	// it logs "MCP tools will not be available" and runs without the catalog for
	// the rest of its life. /healthz lets compose gate PicoClaw on this server
	// actually being ready instead of merely started.
	mux := http.NewServeMux()
	mux.Handle("/mcp", server.NewStreamableHTTPServer(s))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	addr := ":" + getEnv("MCP_PORT", "9090")
	log.Printf("store-mcp (streamable HTTP) listening on %s/mcp", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func registerTools(s *server.MCPServer, st *store.Store) {
	// get_business_info ─────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("get_business_info",
		mcp.WithDescription("Get the business's contact profile (name, email, phone numbers, opening hours) and its branch locations. Each location has its own name and full address — use these to tell a customer which branch/shop stocks an item and where it is."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logActivity(st, "get_business_info", "", "profile requested", callerPhone(req))
		p, err := st.GetBusinessProfile(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if p == nil {
			return mcp.NewToolResultError("business profile not set yet"), nil
		}
		// Curated, minimal profile — identity + contact only. Street addresses
		// live on the branch locations below, not on the profile.
		profile := map[string]any{"name": p.Name}
		if strings.TrimSpace(p.Email) != "" {
			profile["email"] = p.Email
		}
		if len(p.Phones) > 0 {
			profile["phones"] = p.Phones
		}
		if strings.TrimSpace(p.Hours) != "" {
			profile["hours"] = p.Hours
		}
		out := map[string]any{"profile": profile}

		// Surface branch locations with a usable address so the bot can tell
		// customers which shop stocks an item and where it is.
		if locs, err := st.ListBusinessLocations(ctx); err == nil {
			branches := make([]map[string]any, 0, len(locs))
			for _, l := range locs {
				if !l.Active {
					continue
				}
				addr := locationAddress(l)
				if addr == "" {
					continue // no address entered yet — nothing useful to share
				}
				b := map[string]any{"name": l.Name, "address": addr}
				if strings.TrimSpace(l.Phone) != "" {
					b["phone"] = l.Phone
				}
				branches = append(branches, b)
			}
			if len(branches) > 0 {
				out["locations"] = branches
			}
		}
		return jsonResult(out)
	})

	// search_listings ────────────────────────────────────────────────────────────
	s.AddTool(mcp.NewTool("search_listings",
		mcp.WithDescription("Search the business's directory. Match listings by free-text keyword and/or an exact category. Returns matching listings; price and stock quantity are included only for staff/owner callers. For customers the result is name/category/description plus an `in_stock` flag (true/false — never the raw number) and a contact number for price & availability."),
		mcp.WithString("query", mcp.Description("Free-text keyword, e.g. 'ceiling fan', 'cardiologist', '2.5 sq mm wire'.")),
		mcp.WithString("category", mcp.Description("Exact category filter. Use list_categories for valid values.")),
		mcp.WithString("limit", mcp.Description("Max results as a string, e.g. \"20\" (default 20).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		caller := callerPhone(req)
		isStaff := caller == ownerCaller || st.IsStaffPhone(ctx, caller)

		items, err := st.SearchListings(ctx, store.SearchParams{
			Query:    query,
			Category: req.GetString("category", ""),
			Limit:    int(parseIntArg(req.GetString("limit", ""), 20)),
			// Customers now see all matches (including out-of-stock); the bot logs
			// demand for out-of-stock ones instead of hiding them.
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "search_listings", query, fmt.Sprintf("%d results", len(items)), caller)

		// Staff and the owner see everything (price, stock quantity, raw data).
		if isStaff {
			return jsonResult(map[string]any{"count": len(items), "listings": items, "viewer": "staff"})
		}

		// Customers must never be shown price or stock quantity. Return a redacted
		// view (name/category/description only) plus the business contact so the
		// bot can direct them there for price and availability.
		safe := make([]map[string]any, 0, len(items))
		for _, l := range items {
			m := map[string]any{"name": l.Name}
			if l.Category.Name != "" {
				m["category"] = l.Category.Name
			}
			if strings.TrimSpace(l.Description) != "" {
				m["description"] = l.Description
			}
			// Stock STATUS only, never the number: unknown quantity (a service) is
			// treated as available; 0 or negative is out of stock.
			m["in_stock"] = l.Quantity == nil || *l.Quantity > 0
			safe = append(safe, m)
		}
		resp := map[string]any{
			"count":    len(safe),
			"listings": safe,
			"viewer":   "customer",
			"policy":   "Do NOT state any price or stock NUMBER to this customer. Each item has an `in_stock` flag. If in_stock is false, the item isn't currently in stock: briefly tell the customer it can be arranged, and call request_alert to log their interest (their WhatsApp number is filled in automatically). For price and exact availability, tell them to contact the business.",
		}
		if p, _ := st.GetBusinessProfile(ctx); p != nil {
			if c := businessContact(p); c != "" {
				resp["contact_for_details"] = c
			}
		}
		return jsonResult(resp)
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
		mcp.WithDescription("Record a customer's interest in something the business can't currently supply, so the owner sees the demand and the customer can be notified when it's back. Call this (a) whenever a customer asks about an item whose in_stock flag is false — log it automatically, no need to ask first (use source 'bot_offered') — or (b) when the customer explicitly asks to be notified (source 'customer_asked'). The phone number defaults to this WhatsApp chat, so you usually only need item_query — you do not need to ask for their number."),
		mcp.WithString("item_query", mcp.Required(), mcp.Description("What the customer asked for, in plain words, e.g. 'Havells ceiling fan'.")),
		mcp.WithString("customer_name", mcp.Description("The customer's name, if they gave it.")),
		mcp.WithString("customer_phone", mcp.Description("Phone to alert. Leave blank to use this WhatsApp chat's number (preferred). Only set this if the customer asks to be notified on a different number.")),
		mcp.WithString("category", mcp.Description("Category, if known.")),
		mcp.WithString("availability", mcp.Description("'out_of_stock' (carried but none) or 'not_carried' (not stocked).")),
		mcp.WithString("source", mcp.Description("'customer_asked' if the customer asked to be notified, or 'bot_offered' if you offered. Defaults to customer_asked.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		item, err := req.RequireString("item_query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// Default the alert number to the WhatsApp sender so the customer doesn't
		// have to type it — the caller phone is injected by the trusted channel,
		// not the model, so it can't be spoofed.
		phone := strings.TrimSpace(req.GetString("customer_phone", ""))
		if phone == "" {
			phone = callerPhone(req)
		}
		if phone == "" || phone == ownerCaller {
			return mcp.NewToolResultError("No phone number to alert. Ask the customer which number to notify."), nil
		}
		source := strings.TrimSpace(req.GetString("source", ""))
		if source != "bot_offered" {
			source = "customer_asked"
		}
		id, err := st.CreateAlertRequest(ctx, models.AlertRequest{
			CustomerName:  strings.TrimSpace(req.GetString("customer_name", "")),
			CustomerPhone: phone,
			ItemQuery:     strings.TrimSpace(item),
			Category:      strings.TrimSpace(req.GetString("category", "")),
			Availability:  strings.TrimSpace(req.GetString("availability", "")),
			Source:        source,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		logActivity(st, "request_alert", item, "alert logged", phone)
		return jsonResult(map[string]any{"ok": true, "request_id": id, "message": "Alert request logged. The customer will be contacted when it's available."})
	})

	registerStaffTools(s, st)
}

// registerStaffTools adds tools that only staff/owner numbers may use. Each
// checks the trusted "_caller" against the staff list server-side, so a
// customer who learns the tool names still gets nothing.
func registerStaffTools(s *server.MCPServer, st *store.Store) {
	denyIfNotStaff := func(ctx context.Context, req mcp.CallToolRequest) *mcp.CallToolResult {
		caller := callerPhone(req)
		if caller == ownerCaller {
			return nil // the owner console is always trusted
		}
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
		st.DB().WithContext(ctx).Where("status = ?", "logged").
			Order("created_at DESC").Limit(50).Find(&alerts)
		logActivity(st, "staff_pending_alerts", "", fmt.Sprintf("%d pending", len(alerts)), callerPhone(req))
		return jsonResult(map[string]any{"count": len(alerts), "alerts": alerts})
	})
}

// businessContact returns the best phone number a customer can call for price
// and availability — the first listed business phone, else the WhatsApp number.
func businessContact(p *models.BusinessProfile) string {
	if p == nil {
		return ""
	}
	for _, ph := range p.Phones {
		if s := strings.TrimSpace(ph); s != "" {
			return s
		}
	}
	return strings.TrimSpace(p.WhatsApp)
}

// locationAddress joins a branch's structured address parts into one readable
// line the bot can read out, e.g. "12 MG Road, Kothapet, Guntur, Andhra Pradesh 522001".
// Returns "" when the owner hasn't entered any address yet.
func locationAddress(l models.BusinessLocation) string {
	parts := []string{}
	for _, v := range []string{l.Address, l.Area, l.City, l.State, l.Pincode} {
		if s := strings.TrimSpace(v); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
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
