package main

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var adminGuideTemplate = template.Must(template.ParseFS(assets, "static/admin-guide.html"))

type guideTopic struct{ ID, Title string }

// The trusted local Markdown is rendered with raw HTML disabled. The page's
// topic links come from the headings, so edits cannot leave an outdated menu.
func renderAdminGuide(source []byte) ([]byte, error) {
	markdown := goldmark.New(goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	document := markdown.Parser().Parse(text.NewReader(source))
	topics := []guideTopic{}
	if err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if heading, ok := node.(*ast.Heading); ok && entering && heading.Level == 2 {
			if id, ok := heading.AttributeString("id"); ok {
				topics = append(topics, guideTopic{ID: string(id.([]byte)), Title: string(heading.Text(source))})
			}
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}
	var content bytes.Buffer
	if err := markdown.Renderer().Render(&content, source, document); err != nil {
		return nil, err
	}
	var page bytes.Buffer
	data := struct {
		Topics  []guideTopic
		Content template.HTML
	}{Topics: topics, Content: template.HTML(content.String())}
	if err := adminGuideTemplate.Execute(&page, data); err != nil {
		return nil, err
	}
	return page.Bytes(), nil
}
