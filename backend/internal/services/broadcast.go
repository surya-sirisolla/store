// Package services — broadcast.go runs reminder/offer broadcasts: it builds a
// message from the owner's featured items, resolves opted-in recipients from the
// bot's contacts, and sends to each through PicoClaw with deliberate throttling
// and a daily cap. Bulk WhatsApp messaging via the unofficial client is
// ban-sensitive, so the defaults here are intentionally gentle.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"store/backend/internal/models"
	"store/backend/internal/store"
)

// Broadcast tuning defaults (used when the job config leaves them unset).
const (
	defaultBroadcastMaxItems = 5
	defaultBroadcastThrottle = 8   // seconds between sends
	defaultBroadcastCap      = 200 // max recipients per run
	broadcastMinThrottle     = 3   // never send faster than this, whatever the config
)

// BroadcastConfig is the owner-settable shape stored in ScheduledJob.Config.
type BroadcastConfig struct {
	Header          string `json:"header"`           // optional intro line above the items
	RecipientDays   int    `json:"recipient_days"`   // 0 = all opted-in contacts; else active within N days
	IncludeStaff    bool   `json:"include_staff"`    // include staff numbers (default no)
	MaxItems        int    `json:"max_items"`        // featured items to include
	ThrottleSeconds int    `json:"throttle_seconds"` // gap between sends
	DailyCap        int    `json:"daily_cap"`        // max recipients per run
}

func parseBroadcastConfig(raw models.JSONB) BroadcastConfig {
	var cfg BroadcastConfig
	if len(raw) > 0 {
		if b, err := json.Marshal(raw); err == nil {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	if cfg.MaxItems <= 0 {
		cfg.MaxItems = defaultBroadcastMaxItems
	}
	if cfg.ThrottleSeconds <= 0 {
		cfg.ThrottleSeconds = defaultBroadcastThrottle
	}
	if cfg.ThrottleSeconds < broadcastMinThrottle {
		cfg.ThrottleSeconds = broadcastMinThrottle
	}
	if cfg.DailyCap <= 0 {
		cfg.DailyCap = defaultBroadcastCap
	}
	return cfg
}

// BroadcastService builds and sends reminder broadcasts.
type BroadcastService struct {
	st     *store.Store
	sender *WhatsAppSender
}

func NewBroadcastService(st *store.Store, sender *WhatsAppSender) *BroadcastService {
	return &BroadcastService{st: st, sender: sender}
}

// Preview builds the message body (with a sample name) and reports the recipient
// count, without sending anything — powers the console's create/edit preview.
func (b *BroadcastService) Preview(ctx context.Context, cfg BroadcastConfig) (string, int, error) {
	body, err := b.buildBody(ctx, cfg)
	if err != nil {
		return "", 0, err
	}
	recipients, err := b.st.BroadcastRecipients(ctx, cfg.RecipientDays, cfg.IncludeStaff)
	if err != nil {
		return "", 0, err
	}
	count := len(recipients)
	if count > cfg.DailyCap {
		count = cfg.DailyCap
	}
	return personalize(body, "there"), count, nil
}

// buildBody composes the shared message body from the business name + featured
// items. The greeting keeps a "{name}" placeholder for per-recipient personalization.
func (b *BroadcastService) buildBody(ctx context.Context, cfg BroadcastConfig) (string, error) {
	items, err := b.st.FeaturedListings(ctx, cfg.MaxItems)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no featured items to send — mark some listings as featured/offers first")
	}

	bizName := "our shop"
	if p, err := b.st.GetBusinessProfile(ctx); err == nil && p != nil && strings.TrimSpace(p.Name) != "" {
		bizName = strings.TrimSpace(p.Name)
	}

	var sb strings.Builder
	sb.WriteString("Hi {name}! ")
	if h := strings.TrimSpace(cfg.Header); h != "" {
		sb.WriteString(h)
	} else {
		sb.WriteString(fmt.Sprintf("Here are today's picks from %s:", bizName))
	}
	sb.WriteString("\n")
	for _, it := range items {
		sb.WriteString("\n• ")
		sb.WriteString(strings.TrimSpace(it.Name))
		if it.Price != nil {
			sb.WriteString(fmt.Sprintf(" — ₹%g", *it.Price))
		}
		if o := strings.TrimSpace(it.OfferText); o != "" {
			sb.WriteString(" (" + o + ")")
		}
	}
	sb.WriteString("\n\nReply here to order or ask anything. Reply STOP to opt out.")
	return sb.String(), nil
}

// personalize substitutes the {name} placeholder, using a friendly fallback.
func personalize(body, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "there"
	}
	return strings.ReplaceAll(body, "{name}", name)
}

// Run sends the broadcast: it builds the body once, then messages each opted-in
// recipient (up to the daily cap) with a throttle between sends. Returns a
// per-run summary. Honors ctx cancellation/timeout between recipients.
func (b *BroadcastService) Run(ctx context.Context, cfg BroadcastConfig) (models.JSONB, string, error) {
	body, err := b.buildBody(ctx, cfg)
	if err != nil {
		return nil, "", err
	}
	recipients, err := b.st.BroadcastRecipients(ctx, cfg.RecipientDays, cfg.IncludeStaff)
	if err != nil {
		return nil, "", err
	}
	if len(recipients) == 0 {
		return models.JSONB{"sent": 0, "failed": 0, "recipients": 0}, "No opted-in recipients to send to.", nil
	}
	if len(recipients) > cfg.DailyCap {
		recipients = recipients[:cfg.DailyCap]
	}

	throttle := time.Duration(cfg.ThrottleSeconds) * time.Second
	sent, failed := 0, 0
	for i, ct := range recipients {
		if i > 0 {
			if err := sleepCtx(ctx, throttle); err != nil {
				// Ran out of time / cancelled — report what we managed so far.
				msg := fmt.Sprintf("Stopped early after %d sent, %d failed (%v).", sent, failed, err)
				return models.JSONB{"sent": sent, "failed": failed, "recipients": len(recipients)}, msg, nil
			}
		}
		msg := personalize(body, ct.DisplayName)
		if err := b.sender.Send(ctx, ct.Phone, msg); err != nil {
			failed++
			continue
		}
		sent++
	}

	summary := fmt.Sprintf("Sent %d reminder(s)", sent)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	summary += "."
	return models.JSONB{"sent": sent, "failed": failed, "recipients": len(recipients)}, summary, nil
}
