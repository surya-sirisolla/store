// Package services — whatsappsender.go pushes proactive outbound WhatsApp
// messages through the PicoClaw gateway's internal send endpoint. Used by the
// scheduled reminder/broadcast jobs (and a console "send test" action). PicoClaw
// is the only component paired to WhatsApp, so all outbound goes through it.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WhatsAppSender delivers a single outbound WhatsApp message via PicoClaw.
type WhatsAppSender struct {
	picoclawURL string
	token       string
	client      *http.Client
}

func NewWhatsAppSender(picoclawURL, internalToken string) *WhatsAppSender {
	return &WhatsAppSender{
		picoclawURL: strings.TrimRight(picoclawURL, "/"),
		token:       internalToken,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

// normalizePhone strips a WhatsApp number down to the digits PicoClaw expects as
// a chat id (it appends the WhatsApp server itself). "+91 98765 43210" → "919876543210".
func normalizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Send delivers one message to a phone number. Returns an error if PicoClaw is
// unreachable, not paired, or rejects the send.
func (s *WhatsAppSender) Send(ctx context.Context, phone, text string) error {
	to := normalizePhone(phone)
	if to == "" {
		return fmt.Errorf("no valid phone number")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message is empty")
	}

	body, _ := json.Marshal(map[string]string{"to": to, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.picoclawURL+"/internal/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", s.token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("the WhatsApp bot is unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(readCapped(resp.Body, 512)))
		if msg == "" {
			msg = fmt.Sprintf("send failed (HTTP %d)", resp.StatusCode)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func readCapped(r io.Reader, n int64) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, n))
	return b
}
