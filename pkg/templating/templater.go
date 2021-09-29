package templating

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	fragmentIdentifier = "fragment"
	contentIdentifier  = "content"
)

var (
	ErrorNoValidInput = errors.New("no valid input")
)

type Templater struct {
	client http.Client
}

func New() Templater {
	return Templater{client: *http.DefaultClient}
}

func (t *Templater) Parse(reader io.Reader) (string, error) {
	root, err := html.Parse(reader)
	if err != nil {
		return "", ErrorNoValidInput
	}

	t.ParseWithNode(root)

	var writer bytes.Buffer
	if err := html.Render(&writer, root); err != nil {
		return "", err
	}

	return writer.String(), nil
}

func (t *Templater) ParseWithNode(node *html.Node) {
	for _, element := range t.Walk(node) {
		switch element.Data {
		case fragmentIdentifier:
			fragment, err := t.Resolve(*element)
			if err != nil {
				fragment = &html.Node{
					Type:       html.ElementNode,
					FirstChild: element.FirstChild,
					LastChild:  element.LastChild,
				}
			}

			for _, value := range t.Walk(fragment) {
				switch value.Data {
				case fragmentIdentifier:
					// fixme not the best way to use recursion
					t.ParseWithNode(&html.Node{FirstChild: value})
				case "link":
					// fixme clean up this peace of sh*t
					t.AddHeader(element, value)
					value.Parent.RemoveChild(value)
				}
			}

			parent := element.Parent
			parent.InsertBefore(fragment, element)
			parent.RemoveChild(element)
		}
	}
}

func (t *Templater) Resolve(node html.Node) (*html.Node, error) {
	var attributeSource string
	for _, value := range node.Attr {
		if value.Key == "src" {
			attributeSource = value.Val
		}
	}

	if attributeSource == "" {
		return nil, errors.New("no valid url found")
	}

	resp, err := t.client.Get(attributeSource)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("could not resolve the fragment")
	}

	content, err := html.ParseFragment(resp.Body, &html.Node{Type: html.ElementNode, DataAtom: atom.Lookup([]byte(contentIdentifier)), Data: contentIdentifier})
	if err != nil {
		return nil, err
	}

	result := &html.Node{
		Type: html.ElementNode,
	}
	for _, value := range content {
		result.AppendChild(value)
	}

	return result, nil
}

func (t *Templater) FindSection(data string, node *html.Node) (*html.Node, error) {
	root := node
	if node.Parent != nil {
		for root.Parent != nil {
			root = root.Parent
		}
	}

	for _, section := range t.Walk(root) {
		if section.Data == data {
			return section, nil
		}
	}
	return nil, errors.New("could not find section")
}

// todo clean up the ugly way to add new header entries
func (t *Templater) AddHeader(root, element *html.Node) error {
	head, err := t.FindSection("head", root)
	if err != nil {
		return fmt.Errorf("could not find head section: %w", err)
	}

	head.AppendChild(&html.Node{
		FirstChild: element.FirstChild,
		LastChild:  element.LastChild,
		Type:       element.Type,
		DataAtom:   element.DataAtom,
		Data:       element.Data,
		Attr:       element.Attr,
	})
	return nil
}

func (t *Templater) AddScript(node *html.Node) error {
	return nil
}

func (t *Templater) Walk(node *html.Node) (result []*html.Node) {
	var visit func(n *html.Node) []*html.Node
	visit = func(n *html.Node) []*html.Node {
		if n.FirstChild != nil {
			return append(result, append(visit(n.FirstChild), n.FirstChild)...)
		}

		if n.NextSibling != nil {
			return append(result, append(visit(n.NextSibling), n.NextSibling)...)
		}

		if n.Parent != nil && n.Parent.NextSibling != nil {
			return append(result, append(visit(n.Parent.NextSibling), n.Parent.NextSibling)...)
		}

		return nil
	}
	return visit(node)
}
