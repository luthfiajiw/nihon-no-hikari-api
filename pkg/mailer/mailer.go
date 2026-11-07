package mailer

import "context"

type Attachment struct {
	Filename    string
	ContentType string
	ContentID   string // Content-ID for inline images (e.g., "gakkobit-logo" for cid:gakkobit-logo)
	Data        []byte
	Inline      bool
}

// Mailer defines the contract for sending emails.
type Mailer interface {
	Send(ctx context.Context, to string, subject string, textBody string, htmlBody string) error
	SendWithAttachments(ctx context.Context, to string, subject string, textBody string, htmlBody string, attachments []Attachment) error
}
