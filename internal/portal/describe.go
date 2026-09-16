package portal

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// infoPageMarker identifies the metadata pages of geodata layers, whose URLs
// vary in casing. The statistics datasets link to a DCAT page instead, whose
// description is a placeholder, so only these pages are worth reading.
const infoPageMarker = "service=ikx"

// Describe fills in the description and origin from the dataset's information
// page. Datasets without such a page are left untouched.
func (c *Client) Describe(ctx context.Context, dataset *Dataset) error {
	var pageURL string
	for _, r := range dataset.Resources {
		if r.Format == "Information" && strings.Contains(strings.ToLower(r.URL), infoPageMarker) {
			pageURL = r.URL
		}
	}
	if pageURL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return err
	}
	body, err := c.do(req)
	if err != nil {
		return err
	}
	fields, err := parseInfoPage(body)
	if err != nil {
		return err
	}
	dataset.Description = fields["Beschreibung"]
	dataset.Origin = fields["Herkunft"]
	return nil
}

// parseInfoPage reads the caption/value table rows of a metadata page
func parseInfoPage(page []byte) (map[string]string, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, err
	}
	fields := make(map[string]string)
	var caption string
	for node := range doc.Descendants() {
		if node.Type != html.ElementNode || node.Data != "td" {
			continue
		}
		switch class(node) {
		case "caption0":
			caption = text(node)
		case "value0":
			if caption != "" {
				fields[caption] = text(node)
			}
		}
	}
	return fields, nil
}

func class(node *html.Node) string {
	for _, attr := range node.Attr {
		if attr.Key == "class" {
			return attr.Val
		}
	}
	return ""
}

// text joins the node's text with line breaks for <br>, collapsing the
// page's indentation whitespace
func text(node *html.Node) string {
	var b strings.Builder
	for n := range node.Descendants() {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(strings.Join(strings.Fields(n.Data), " "))
			b.WriteString(" ")
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteString("\n")
		}
	}
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
