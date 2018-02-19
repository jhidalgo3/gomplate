package gomplate

import (
	"io"
	"text/template"

	"github.com/hairyhenderson/gomplate/data"
)

// Gomplate -
type Gomplate struct {
	funcMap    template.FuncMap
	leftDelim  string
	rightDelim string
}

// RunTemplate -
func (g *Gomplate) RunTemplate(t *tplate) error {
	context := &Context{}
	tmpl, err := t.toGoTemplate(g)
	if err != nil {
		return err
	}

	switch t.target.(type) {
	case io.Closer:
		// nolint: errcheck
		defer t.target.(io.Closer).Close()
	}
	err = tmpl.Execute(t.target, context)
	return err
}

// NewGomplate -
func NewGomplate(d *data.Data, leftDelim, rightDelim string) *Gomplate {
	return &Gomplate{
		leftDelim:  leftDelim,
		rightDelim: rightDelim,
		funcMap:    initFuncs(d),
	}
}
