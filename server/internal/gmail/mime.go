package gmail

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
)

// Attachment represents an email attachment with filename, mime type, and binary content.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     []byte `json:"content"`
}

// BuildRawMIME constructs a multipart/mixed RFC 2822 MIME message (matching gmailService.js logic)
// with text/plain body, optional attachments, In-Reply-To, and References headers.
// Returns a URL-safe Base64 encoded string ready for the Gmail REST API messages.send endpoint.
func BuildRawMIME(to, subject, body string, attachments []Attachment, inReplyTo, references string) (string, error) {
	randomBytes := make([]byte, 16)
	_, _ = rand.Read(randomBytes)
	boundary := fmt.Sprintf("boundary_%s", hex.EncodeToString(randomBytes))
	crlf := "\r\n"

	recipient := strings.TrimSpace(to)
	recipient = strings.TrimPrefix(recipient, "<")
	recipient = strings.TrimSuffix(recipient, ">")

	var headers []string
	headers = append(headers, fmt.Sprintf("To: %s", recipient))
	headers = append(headers, fmt.Sprintf("Subject: %s", subject))
	headers = append(headers, "MIME-Version: 1.0")
	headers = append(headers, fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"", boundary))

	if strings.TrimSpace(inReplyTo) != "" {
		headers = append(headers, fmt.Sprintf("In-Reply-To: %s", strings.TrimSpace(inReplyTo)))
	}
	if strings.TrimSpace(references) != "" {
		headers = append(headers, fmt.Sprintf("References: %s", strings.TrimSpace(references)))
	}

	bodyPart := strings.Join([]string{
		fmt.Sprintf("--%s", boundary),
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"Content-Transfer-Encoding: 7bit",
		"",
		body,
	}, crlf)

	var attachmentParts []string
	for _, att := range attachments {
		contentType := att.ContentType
		if contentType == "" {
			if len(att.Content) > 0 {
				contentType = http.DetectContentType(att.Content)
			}
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}

		encodedContent := base64.StdEncoding.EncodeToString(att.Content)
		part := strings.Join([]string{
			fmt.Sprintf("--%s", boundary),
			fmt.Sprintf("Content-Type: %s; name=\"%s\"", contentType, att.Filename),
			"Content-Transfer-Encoding: base64",
			fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"", att.Filename),
			"",
			encodedContent,
		}, crlf)
		attachmentParts = append(attachmentParts, part)
	}

	var parts []string
	parts = append(parts, strings.Join(headers, crlf))
	parts = append(parts, bodyPart)
	parts = append(parts, attachmentParts...)
	parts = append(parts, fmt.Sprintf("--%s--", boundary))
	parts = append(parts, "")

	fullMIME := strings.Join(parts, crlf)
	return base64.RawURLEncoding.EncodeToString([]byte(fullMIME)), nil
}
