package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig is the JSON structure stored in notification_channels.config
// for an email channel.
type EmailConfig struct {
	SMTPHost     string   `json:"smtp_host"`
	SMTPPort     int      `json:"smtp_port"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	FromAddress  string   `json:"from_address"`
	ToAddresses  []string `json:"to_addresses"`
	UseTLS       bool     `json:"use_tls"`       // SMTPS (implicit TLS on port 465)
	UseSTARTTLS  bool     `json:"use_starttls"`  // STARTTLS upgrade (port 587)
}

// EmailSender implements Sender for SMTP email.
type EmailSender struct{}

func NewEmailSender() *EmailSender { return &EmailSender{} }

func (s *EmailSender) Send(ctx context.Context, config json.RawMessage, msg Message) error {
	cfg, err := parseEmailConfig(config)
	if err != nil {
		return err
	}

	subject := msg.Subject
	if subject == "" {
		subject = fmt.Sprintf("[%s] Platform Alert", msg.Severity)
	}
	return s.sendMail(ctx, cfg, subject, msg.Body)
}

func (s *EmailSender) Test(ctx context.Context, config json.RawMessage) error {
	cfg, err := parseEmailConfig(config)
	if err != nil {
		return err
	}
	return s.sendMail(ctx, cfg, "Test notification — Domain Platform", "This is a test message to verify your email channel configuration.")
}

func (s *EmailSender) sendMail(_ context.Context, cfg *EmailConfig, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	to := strings.Join(cfg.ToAddresses, ", ")

	// Build RFC 2822 message.
	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", cfg.FromAddress),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	}

	if cfg.UseTLS {
		return s.sendTLS(addr, cfg, auth, msg)
	}

	// Plain SMTP or STARTTLS.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", addr, err)
	}

	c, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("email: smtp client: %w", err)
	}
	defer c.Quit() //nolint:errcheck

	if cfg.UseSTARTTLS {
		tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("email: starttls: %w", err)
		}
	}

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}

	return s.doSend(c, cfg, msg)
}

func (s *EmailSender) sendTLS(addr string, cfg *EmailConfig, auth smtp.Auth, msg string) error {
	tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("email: tls dial %s: %w", addr, err)
	}

	c, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("email: smtp client (tls): %w", err)
	}
	defer c.Quit() //nolint:errcheck

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("email: auth (tls): %w", err)
		}
	}
	return s.doSend(c, cfg, msg)
}

func (s *EmailSender) doSend(c *smtp.Client, cfg *EmailConfig, msg string) error {
	if err := c.Mail(cfg.FromAddress); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	for _, to := range cfg.ToAddresses {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("email: RCPT TO %s: %w", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	defer w.Close() //nolint:errcheck

	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	return nil
}

func parseEmailConfig(raw json.RawMessage) (*EmailConfig, error) {
	var cfg EmailConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("email: parse config: %w", err)
	}
	if cfg.SMTPHost == "" {
		return nil, fmt.Errorf("email: smtp_host is required")
	}
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587 // default STARTTLS port
	}
	if cfg.FromAddress == "" {
		return nil, fmt.Errorf("email: from_address is required")
	}
	if len(cfg.ToAddresses) == 0 {
		return nil, fmt.Errorf("email: at least one to_address is required")
	}
	return &cfg, nil
}
