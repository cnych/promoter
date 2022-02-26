package template

import (
	"bytes"
	"io"
	"path/filepath"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

type Template struct {
	tmpl *template.Template
}

func FromGlobs(loadBuiltinTemplate bool, paths ...string) (*Template, error) {
	tmpl := template.New("").Option("missingkey=zero").Funcs(defaultFuncs).Funcs(sprig.TxtFuncMap())

	if loadBuiltinTemplate {
		f, err := Assets.Open("/templates/default.tmpl")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}

		if _, err := tmpl.Parse(string(b)); err != nil {
			return nil, err
		}
	}

	for _, tpl := range paths {
		p, err := filepath.Glob(tpl)
		if err != nil {
			return nil, err
		}
		if len(p) > 0 {
			if _, err := tmpl.ParseGlob(tpl); err != nil {
				return nil, err
			}
		}
	}

	return &Template{tmpl: tmpl}, nil
}

func (t *Template) ExecuteTextString(text string, data interface{}) (string, error) {
	if text == "" {
		return "", nil
	}
	tmpl, err := t.tmpl.Clone()
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
