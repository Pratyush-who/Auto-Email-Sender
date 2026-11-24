package gmail

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildRawMIME_Basic(t *testing.T) {
	to := "test@example.com"
	subject := "Job Application"
	body := "Hello, please find my application attached."

	rawBase64, err := BuildRawMIME(to, subject, body, nil, "", "")
	if err != nil {
		t.Fatalf("BuildRawMIME failed: %v", err)
	}

	decodedBytes, err := base64.RawURLEncoding.DecodeString(rawBase64)
	if err != nil {
		t.Fatalf("failed to decode base64url: %v", err)
	}

	decoded := string(decodedBytes)

	if !strings.Contains(decoded, "To: test@example.com") {
		t.Errorf("missing To header")
	}
	if !strings.Contains(decoded, "Subject: Job Application") {
		t.Errorf("missing Subject header")
	}
	if !strings.Contains(decoded, "Content-Type: multipart/mixed; boundary=") {
		t.Errorf("missing Content-Type header")
	}
	if !strings.Contains(decoded, body) {
		t.Errorf("missing body text")
	}
}

func TestBuildRawMIME_InReplyToAndReferences(t *testing.T) {
	to := "recruiter@example.com"
	subject := "Re: Follow up"
	body := "Thanks for the response."
	inReplyTo := "<msg123@mail.gmail.com>"
	references := "<msg001@mail.gmail.com> <msg123@mail.gmail.com>"

	rawBase64, err := BuildRawMIME(to, subject, body, nil, inReplyTo, references)
	if err != nil {
		t.Fatalf("BuildRawMIME failed: %v", err)
	}

	decodedBytes, _ := base64.RawURLEncoding.DecodeString(rawBase64)
	decoded := string(decodedBytes)

	if !strings.Contains(decoded, "In-Reply-To: <msg123@mail.gmail.com>") {
		t.Errorf("missing In-Reply-To header")
	}
	if !strings.Contains(decoded, "References: <msg001@mail.gmail.com> <msg123@mail.gmail.com>") {
		t.Errorf("missing References header")
	}
}

func TestBuildRawMIME_WithAttachments(t *testing.T) {
	to := "client@example.com"
	subject := "Resume Submission"
	body := "Here is my resume."
	attachments := []Attachment{
		{
			Filename:    "resume.pdf",
			ContentType: "application/pdf",
			Content:     []byte("%PDF-1.4 sample pdf content"),
		},
	}

	rawBase64, err := BuildRawMIME(to, subject, body, attachments, "", "")
	if err != nil {
		t.Fatalf("BuildRawMIME failed: %v", err)
	}

	decodedBytes, _ := base64.RawURLEncoding.DecodeString(rawBase64)
	decoded := string(decodedBytes)

	if !strings.Contains(decoded, `Content-Disposition: attachment; filename="resume.pdf"`) {
		t.Errorf("missing attachment disposition header")
	}
	if !strings.Contains(decoded, `Content-Type: application/pdf; name="resume.pdf"`) {
		t.Errorf("missing attachment content type header")
	}
	encodedSample := base64.StdEncoding.EncodeToString(attachments[0].Content)
	if !strings.Contains(decoded, encodedSample) {
		t.Errorf("missing base64 encoded attachment content")
	}
}
