package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"yxz/internal/core"
	"yxz/internal/jisuan"
	"yxz/internal/llm"
	"yxz/internal/shuchu"
	"yxz/internal/shuru"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// loadDotEnv reads KEY=VALUE lines (for running the binary directly without Docker).
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok && os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`))
		}
	}
}

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	content := fl.String("content", "", "content dir")
	envFile := fl.String("env", ".env", "env file")
	all := fl.Bool("all", false, "purge: also delete replay recordings")
	fl.Parse(args)
	loadDotEnv(*envFile)
	if *content == "" {
		*content = env("YXZ_CONTENT", "content")
	}
	data := env("YXZ_DATA", "data")
	switch cmd {
	case "purge":
		for _, d := range []string{"state", "uploads"} {
			os.RemoveAll(filepath.Join(data, d))
		}
		if *all {
			os.RemoveAll(filepath.Join(data, "replay"))
		}
		fmt.Println("已清除课堂记录")
		return
	}
	app, err := core.NewApp(*content, data, os.Getenv("YXZ_TEACHER_PIN"), env("YXZ_MODE", "live"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败：", err)
		os.Exit(1)
	}
	cfg := app.Cfg()
	gw := llm.New(os.Getenv("YXZ_TEXT_BASE"), os.Getenv("YXZ_TEXT_KEY"), os.Getenv("YXZ_TEXT_MODEL"), cfg.EffortStyle, cfg.MaxConcurrency)
	rp := llm.NewReplay(filepath.Join(data, "replay"))
	js, err := jisuan.New(app, gw, rp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "计算环节内容有误：", err)
		os.Exit(1)
	}
	sc, err := shuchu.New(app, gw, rp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "输出环节内容有误：", err)
		os.Exit(1)
	}
	mods := []core.Module{js, sc, shuru.New(app)}
	bad := 0
	for _, m := range mods {
		for _, e := range m.Check(os.DirFS(*content)) {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", m.Name(), e)
			bad++
		}
	}
	if cmd == "check" {
		if bad > 0 {
			os.Exit(1)
		}
		fmt.Println("内容校验通过")
		return
	}
	if bad > 0 {
		fmt.Fprintln(os.Stderr, "内容校验没通过，拒绝启动")
		os.Exit(1)
	}
	if cmd == "warmup" {
		js.Warmup()
		return
	}
	mux := http.NewServeMux()
	for _, m := range mods {
		app.Register(m)
		m.Routes(mux)
	}
	app.CommonRoutes(mux, func() any {
		return map[string]any{"gateway": gw.Status(), "jisuan": js.Status(), "shuchu": sc.Status(), "replay": rp.Count(), "content_ok": bad == 0}
	})
	mux.Handle("/", gz(app.Static(core.Dist())))
	go app.SnapshotLoop()
	go js.Warmup()
	port := env("YXZ_PORT", "8080")
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	fmt.Printf("元小智课堂系统已启动：http://localhost:%s  局域网：%v  模式：%s\n", port, core.LanIPs(), app.ModeName())
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// gz compresses static text responses (25 tablets share one Wi-Fi).
var gzPool = sync.Pool{New: func() any { w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed); return w }}

type gzw struct {
	http.ResponseWriter
	w io.Writer
}

func (g gzw) Write(b []byte) (int, error) { return g.w.Write(b) }
func (g gzw) WriteHeader(c int) { g.ResponseWriter.Header().Del("Content-Length"); g.ResponseWriter.WriteHeader(c) }

func gz(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		text := !strings.Contains(p, ".") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".svg") || strings.HasSuffix(p, ".json")
		if !text || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || strings.HasPrefix(p, "/uploads/") || r.Header.Get("Range") != "" {
			h.ServeHTTP(w, r)
			return
		}
		z := gzPool.Get().(*gzip.Writer)
		z.Reset(w)
		defer func() { z.Close(); gzPool.Put(z) }()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		h.ServeHTTP(gzw{w, z}, r)
	})
}
