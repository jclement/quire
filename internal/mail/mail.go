// Package mail is quire's email layer. The provider abstraction is SMTP
// itself — every transactional provider (Mailgun, SES, Postmark, Resend)
// exposes an SMTP endpoint, so "switch providers" means "change four env
// vars". The Sender interface exists so an API-based transport can slot in
// later without touching callers; go-mail (wneessen) does the SMTP work.
package mail

import (
	"fmt"
	"os"
	"strconv"

	gomail "github.com/wneessen/go-mail"
)

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	Text    string // plain-text body (always present)
	HTML    string // optional HTML alternative
}

// Sender delivers messages. Implementations must be safe for concurrent use.
type Sender interface {
	Send(msg Message) error
}

// Config is the SMTP transport configuration, from QUIRE_SMTP_* env vars.
type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// FromEnv reads SMTP configuration; ok is false when mail is not configured
// (no host), which callers treat as "email features off".
func FromEnv() (Config, bool) {
	cfg := Config{
		Host: os.Getenv("QUIRE_SMTP_HOST"),
		Port: 587,
		User: os.Getenv("QUIRE_SMTP_USER"),
		Pass: os.Getenv("QUIRE_SMTP_PASS"),
		From: os.Getenv("QUIRE_SMTP_FROM"),
	}
	if v := os.Getenv("QUIRE_SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if cfg.From == "" {
		cfg.From = cfg.User
	}
	return cfg, cfg.Host != ""
}

// SMTPSender sends via an SMTP relay.
type SMTPSender struct {
	cfg Config
}

// NewSMTP validates the config and returns a sender.
func NewSMTP(cfg Config) (*SMTPSender, error) {
	if cfg.Host == "" || cfg.From == "" {
		return nil, fmt.Errorf("SMTP host and from address are required (QUIRE_SMTP_HOST, QUIRE_SMTP_FROM)")
	}
	return &SMTPSender{cfg: cfg}, nil
}

// Send delivers one message. Port 465 uses implicit TLS; everything else
// negotiates STARTTLS (mandatory — no plaintext fallback).
func (s *SMTPSender) Send(msg Message) error {
	m := gomail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("from address: %w", err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("to address: %w", err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(gomail.TypeTextPlain, msg.Text)
	if msg.HTML != "" {
		m.AddAlternativeString(gomail.TypeTextHTML, msg.HTML)
	}

	options := []gomail.Option{
		gomail.WithPort(s.cfg.Port),
		gomail.WithTLSPortPolicy(gomail.TLSMandatory),
	}
	if s.cfg.Port == 465 {
		options = append(options, gomail.WithSSLPort(false))
	}
	if s.cfg.User != "" {
		options = append(options,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(s.cfg.User),
			gomail.WithPassword(s.cfg.Pass))
	}
	client, err := gomail.NewClient(s.cfg.Host, options...)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	if err := client.DialAndSend(m); err != nil {
		return fmt.Errorf("sending mail via %s: %w", s.cfg.Host, err)
	}
	return nil
}
