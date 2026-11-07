package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type smtpMailer struct {
	cfg SMTPConfig
}

func NewSMTPMailer(cfg SMTPConfig) Mailer {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.Port) == "" || strings.TrimSpace(cfg.From) == "" {
		return nil
	}
	return &smtpMailer{cfg: cfg}
}

func (m *smtpMailer) Send(ctx context.Context, to string, subject string, textBody string, htmlBody string) error {
	return m.SendWithAttachments(ctx, to, subject, textBody, htmlBody, nil)
}

func (m *smtpMailer) SendWithAttachments(ctx context.Context, to string, subject string, textBody string, htmlBody string, attachments []Attachment) error {
	addr := net.JoinHostPort(m.cfg.Host, m.cfg.Port)
	host := m.cfg.Host

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", m.cfg.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if len(attachments) > 0 {
		relatedBoundary := fmt.Sprintf("boundary_rel_%d", time.Now().UnixNano())
		altBoundary := fmt.Sprintf("boundary_alt_%d", time.Now().UnixNano())

		msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=\"%s\"\r\n\r\n", relatedBoundary))

		// Start multipart/alternative part
		msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", altBoundary))

		if textBody != "" {
			msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
			msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
			msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
			msg.WriteString(textBody)
			msg.WriteString("\r\n\r\n")
		}

		if htmlBody != "" {
			msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
			msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
			msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
			msg.WriteString(htmlBody)
			msg.WriteString("\r\n\r\n")
		}

		msg.WriteString(fmt.Sprintf("--%s--\r\n\r\n", altBoundary))

		// Inline attachments
		for _, att := range attachments {
			msg.WriteString(fmt.Sprintf("--%s\r\n", relatedBoundary))
			msg.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, att.Filename))
			msg.WriteString("Content-Transfer-Encoding: base64\r\n")
			if att.Inline && att.ContentID != "" {
				msg.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", att.ContentID))
				msg.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=\"%s\"\r\n\r\n", att.Filename))
			} else {
				msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", att.Filename))
			}

			encoded := base64.StdEncoding.EncodeToString(att.Data)
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				msg.WriteString(encoded[i:end])
				msg.WriteString("\r\n")
			}
			msg.WriteString("\r\n")
		}

		msg.WriteString(fmt.Sprintf("--%s--\r\n", relatedBoundary))
	} else {
		boundary := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

		if textBody != "" {
			msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
			msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
			msg.WriteString(textBody)
			msg.WriteString("\r\n\r\n")
		}

		if htmlBody != "" {
			msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
			msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
			msg.WriteString(htmlBody)
			msg.WriteString("\r\n\r\n")
		}

		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error

	if m.cfg.Port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("failed to dial smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create smtp client: %w", err)
	}
	defer client.Close()

	if m.cfg.Port != "465" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{ServerName: host}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("failed to start tls: %w", err)
			}
		}
	}

	if m.cfg.Username != "" {
		auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("smtp MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA failed: %w", err)
	}

	if _, err := w.Write([]byte(msg.String())); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write body failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close writer failed: %w", err)
	}

	return client.Quit()
}
