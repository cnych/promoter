package template

import (
	"bytes"
	tmplhtml "html/template"
	"io/ioutil"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	tmpltext "text/template"
)

type FuncMap map[string]interface{}

var DefaultFuncs = FuncMap{
	"toUpper": strings.ToUpper,
	"toLower": strings.ToLower,
	"title":   strings.Title,
	// join is equal to strings.Join but inverts the argument order
	// for easier pipelining in templates.
	"join": func(sep string, s []string) string {
		return strings.Join(s, sep)
	},
	"match": regexp.MatchString,
	"safeHtml": func(text string) tmplhtml.HTML {
		return tmplhtml.HTML(text)
	},
	"reReplaceAll": func(pattern, repl, text string) string {
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(text, repl)
	},
	"stringSlice": func(s ...string) []string {
		return s
	},
	"markdown": markdownEscapeString,
}

// Template bundles a text and a html template instance.
type Template struct {
	text *tmpltext.Template
	html *tmplhtml.Template

	ExternalURL *url.URL
}

func FromGlobs(paths ...string) (*Template, error) {
	t := &Template{
		text: tmpltext.New("").Option("missingkey=zero"),
		html: tmplhtml.New("").Option("missingkey=zero"),
	}

	t.text = t.text.Funcs(tmpltext.FuncMap(DefaultFuncs))
	t.html = t.html.Funcs(tmplhtml.FuncMap(DefaultFuncs))

	defaultTemplates := []string{"default.tmpl"}

	for _, file := range defaultTemplates {
		f, err := Assets.Open(path.Join("/templates", file))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		if t.text, err = t.text.Parse(string(b)); err != nil {
			return nil, err
		}
		if t.html, err = t.html.Parse(string(b)); err != nil {
			return nil, err
		}
	}

	for _, tp := range paths {
		// ParseGlob in the template packages errors if not at least one file is
		// matched. We want to allow empty matches that may be populated later on.
		p, err := filepath.Glob(tp)
		if err != nil {
			return nil, err
		}
		if len(p) > 0 {
			if t.text, err = t.text.ParseGlob(tp); err != nil {
				return nil, err
			}
			if t.html, err = t.html.ParseGlob(tp); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

// ExecuteTextString needs a meaningful doc comment (TODO(fabxc)).
func (t *Template) ExecuteTextString(text string, data interface{}) (string, error) {
	if text == "" {
		return "", nil
	}
	tmpl, err := t.text.Clone()
	if err != nil {
		return "", err
	}
	tmpl, err = tmpl.New("").Option("missingkey=zero").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}

// ExecuteHTMLString needs a meaningful doc comment (TODO(fabxc)).
func (t *Template) ExecuteHTMLString(html string, data interface{}) (string, error) {
	if html == "" {
		return "", nil
	}
	tmpl, err := t.html.Clone()
	if err != nil {
		return "", err
	}
	tmpl, err = tmpl.New("").Option("missingkey=zero").Parse(html)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}
