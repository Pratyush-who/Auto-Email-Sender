package pdf

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromFile opens a PDF file from disk and extracts all plain text content.
func ExtractTextFromFile(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open pdf file: %w", err)
	}
	defer f.Close()

	plainText, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("failed to get plain text from pdf: %w", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, plainText); err != nil {
		return "", fmt.Errorf("failed to read pdf text stream: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}
