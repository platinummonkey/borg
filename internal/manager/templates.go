package manager

import (
	"embed"
	"html/template"
	"io"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	tmplOnce sync.Once
	tmpls    map[string]*template.Template
	tmplErr  error
)

func loadTemplates() (map[string]*template.Template, error) {
	tmplOnce.Do(func() {
		tmpls = make(map[string]*template.Template)
		pages := []string{
			"dashboard.html",
			"agent_detail.html",
			"spawn.html",
			"costs.html",
			"taskboard.html",
		}
		for _, page := range pages {
			t, err := template.ParseFS(templateFS, "templates/layout.html", "templates/"+page)
			if err != nil {
				tmplErr = err
				return
			}
			tmpls[page] = t
		}
	})
	return tmpls, tmplErr
}

func executeTemplate(w io.Writer, name string, data any) error {
	ts, err := loadTemplates()
	if err != nil {
		return err
	}
	t, ok := ts[name]
	if !ok {
		return nil
	}
	return t.ExecuteTemplate(w, "layout", data)
}
