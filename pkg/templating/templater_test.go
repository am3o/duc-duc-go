package templating

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
)

func TestTemplater_Parse(t *testing.T) {
	tt := []struct {
		input    io.Reader
		expected string
		error    error
	}{
		{
			input:    strings.NewReader(""),
			expected: "<html><head></head><body></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html></html>"),
			expected: "<html><head></head><body></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html><body><div><a/></div><a/></body></html>"),
			expected: "<html><head></head><body><div><a></a></div><a></a></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html><body><fragment></fragment></body></html>"),
			expected: "<html><head></head><body><></></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html><body><div><fragment></fragment></div></body></html>"),
			expected: "<html><head></head><body><div><></></div></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html><body><div><fragment></fragment></div><fragment></fragment></body></html>"),
			expected: "<html><head></head><body><div><></></div><></></body></html>",
			error:    nil,
		},
		{
			input:    strings.NewReader("<html><body><fragment></fragment><fragment></fragment></body></html>"),
			expected: "<html><head></head><body><></><></></body></html>",
			error:    nil,
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			templater := New()
			response, err := templater.Parse(tc.input)
			assert.Equal(t, tc.error, err)
			assert.Equal(t, tc.expected, response)
		})
	}
}

func TestTemplater_ParseWithNode_FallBack(t *testing.T) {
	brokenDummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))

	t.Run("should render the fallback because direct dependent service response an unhealthy status", func(t *testing.T) {
		const expected = "<html><head></head><body><>Foo</></body></html>"
		root, _ := html.Parse(strings.NewReader(fmt.Sprintf(`<html><body><fragment src="%s">Foo</fragment></body></html>`, brokenDummy.URL)))

		templater := New()
		templater.ParseWithNode(root)

		var actual bytes.Buffer
		html.Render(&actual, root)
		assert.Equal(t, expected, actual.String())
	})

	t.Run("should render the fallback because the indirect dependent service response an unhealthy status", func(t *testing.T) {
		const expected = "<html><head></head><body><><content><p>hello</p><>from</><a>the other site</a></content></></body></html>"

		dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(fmt.Sprintf(`<content><p>hello</p><fragment src="%s">from</fragment><a>the other site</a></content>`, brokenDummy.URL)))
		}))

		root, _ := html.Parse(strings.NewReader(fmt.Sprintf(`<html><body><fragment src="%s">Foo</fragment></body></html>`, dummy.URL)))

		templater := New()
		templater.ParseWithNode(root)

		var actual bytes.Buffer
		html.Render(&actual, root)
		assert.Equal(t, expected, actual.String())
	})

	t.Run("should render the fallback because the indirect dependent service response multiple times an unhealthy status", func(t *testing.T) {
		const expected = "<html><head></head><body><><content><p>hello</p><>from</><>the other site</></content></></body></html>"

		dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(fmt.Sprintf(`<content><p>hello</p><fragment src="%s">from</fragment><fragment src="%s">the other site</fragment></content>`, brokenDummy.URL, brokenDummy.URL)))
		}))

		root, _ := html.Parse(strings.NewReader(fmt.Sprintf(`<html><body><fragment src="%s">Foo</fragment></body></html>`, dummy.URL)))

		templater := New()
		templater.ParseWithNode(root)

		var actual bytes.Buffer
		html.Render(&actual, root)
		assert.Equal(t, expected, actual.String())
	})

	t.Run("should render the fallback because the indirect dependent service response multiple times an unhealthy status", func(t *testing.T) {
		const expected = "<html><head></head><body><><content><p>hello</p><>from</><><content>the other site</content></></content></></body></html>"

		anotherDummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(`<content>the other site</content>`))
		}))

		dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(fmt.Sprintf(`<content><p>hello</p><fragment src="%s">from</fragment><fragment src="%s"></fragment></content>`, brokenDummy.URL, anotherDummy.URL)))
		}))

		root, _ := html.Parse(strings.NewReader(fmt.Sprintf(`<html><body><fragment src="%s">Foo</fragment></body></html>`, dummy.URL)))

		templater := New()
		templater.ParseWithNode(root)

		var actual bytes.Buffer
		html.Render(&actual, root)
		assert.Equal(t, expected, actual.String())
	})
}

func TestTemplater_Walk(t *testing.T) {
	tt := []struct {
		input    io.Reader
		expected string
	}{
		{
			input:    strings.NewReader(""),
			expected: "body head html ",
		},
		{
			input:    strings.NewReader("<html></html>"),
			expected: "body head html ",
		},
		{
			input:    strings.NewReader(`<html><head/><body><p><a/><a/></p><div><a/></div><div><foo/><bar/></body></html>`),
			expected: "bar foo a div a div a a p body head html ",
		},
		{
			input:    strings.NewReader(`<html><head/><body><div/><div/></body></html>`),
			expected: "div div body head html ",
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			var templater Templater
			content, _ := html.Parse(tc.input)

			var actual string
			for _, value := range templater.Walk(content) {
				actual += fmt.Sprintf("%s ", value.Data)
			}
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestTemplater_Resolve(t *testing.T) {
	dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(fmt.Sprintf("<html><head/><body><%v>Foo</%v></body></html>", contentIdentifier, contentIdentifier)))
	}))

	tt := []struct {
		fragment      html.Node
		expected      string
		expectedError bool
	}{
		{
			fragment: html.Node{
				Data: fragmentIdentifier,
				Attr: []html.Attribute{},
			},
			expected:      "",
			expectedError: true,
		},
		{
			fragment: html.Node{
				Data: fragmentIdentifier,
				Attr: []html.Attribute{{"", "src", dummy.URL}},
			},
			expected: "<><content>Foo</content></>",
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			var templater Templater
			resolved, err := templater.Resolve(tc.fragment)
			if err != nil {
				assert.Equal(t, tc.expectedError, err != nil, err.Error())
				return
			}

			var actual bytes.Buffer
			html.Render(&actual, resolved)
			assert.Equal(t, tc.expected, actual.String())
		})
	}
}

func TestTemplater_Resolve_Fallback(t *testing.T) {
	dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))

	tt := []struct {
		fragment      html.Node
		expected      string
		expectedError bool
	}{
		{
			fragment: html.Node{
				Data: fragmentIdentifier,
				Attr: []html.Attribute{{"", "src", dummy.URL}},
			},
			expectedError: true,
		},
	}

	for _, tc := range tt {
		t.Run("", func(t *testing.T) {
			var templater Templater
			resolved, err := templater.Resolve(tc.fragment)
			if err != nil {
				assert.Equal(t, tc.expectedError, err != nil, err.Error())
				return
			}

			var actual bytes.Buffer
			html.Render(&actual, resolved)
			assert.Equal(t, tc.expected, actual.String())
		})
	}
}

func TestTemplater_FindSection(t *testing.T) {
	root, _ := html.Parse(strings.NewReader("<html><head/><body><a>Foo</a></body></html>"))

	start := root.FirstChild.LastChild.FirstChild

	var templater Templater
	section, err := templater.FindSection("head", start)
	assert.NoError(t, err)
	assert.Equal(t, "head", section.Data)
}

func TestTemplater_ParseWithNode_Head(t *testing.T) {
	t.SkipNow()
	
	const expected = ""
	dummy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`<content><link id="styles" type="text/css" media="all" rel="stylesheet" href="https://example.com"></content>`))
	}))

	root, _ := html.Parse(strings.NewReader(fmt.Sprintf(`<html><body><fragment src="%s">Foo</fragment></body></html>`, dummy.URL)))

	templater := New()
	templater.ParseWithNode(root)

	var actual bytes.Buffer
	html.Render(&actual, root)
	assert.Equal(t, expected, actual.String())

}
