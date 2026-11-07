// Package shuru is the 输入 stage placeholder: routes and state are reserved until the design is decided.
package shuru

import (
	"io/fs"
	"net/http"

	"yxz/internal/core"
)

type Module struct{ app *core.App }

func New(app *core.App) *Module { app.Hub("shuru"); return &Module{app: app} }

func (m *Module) Name() string                 { return "shuru" }
func (m *Module) Snapshot() ([]byte, error)    { return []byte("{}"), nil }
func (m *Module) Restore([]byte) error         { return nil }
func (m *Module) Reset()                       {}
func (m *Module) Check(fs.FS) []error          { return nil }
func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/shuru/state", func(w http.ResponseWriter, r *http.Request) {
		core.JSON(w, map[string]any{"status": "pending", "text": "输入环节正在设计中"})
	})
}
