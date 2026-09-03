// Package mock implements domain.EmailProvider/SMSProvider/WhatsAppProvider
// with in-memory recording — zero network calls, safe for the automated
// test suite (same role as internal/modules/einvoice/v1/mock in Stage 8).
package mock

import (
	"context"
	"sync"
)

type Sent struct {
	Channel   string
	Recipient string
	Body      string
}

type Provider struct {
	mu       sync.Mutex
	sent     []Sent
	FailNext bool
}

func New() *Provider { return &Provider{} }

func (p *Provider) Sent() []Sent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Sent, len(p.sent))
	copy(out, p.sent)
	return out
}

func (p *Provider) record(channel, recipient, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FailNext {
		p.FailNext = false
		return errSimulated
	}
	p.sent = append(p.sent, Sent{Channel: channel, Recipient: recipient, Body: body})
	return nil
}

var errSimulated = simulatedError{}

type simulatedError struct{}

func (simulatedError) Error() string { return "mock: simulated provider failure" }

func (p *Provider) SendEmail(ctx context.Context, to, subject, bodyHTML, attachmentName string, attachment []byte) error {
	return p.record("EMAIL", to, subject)
}

func (p *Provider) SendSMS(ctx context.Context, toE164, body string) error {
	return p.record("SMS", toE164, body)
}

func (p *Provider) SendTemplateMessage(ctx context.Context, toE164, templateName string, params []string) error {
	return p.record("WHATSAPP", toE164, templateName)
}
