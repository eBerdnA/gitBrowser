package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/andrebering/gitBrowser/git"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var templates *template.Template

type appConfig struct {
	Repos []repoConfig `json:"repos"`
}

type repoConfig struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type app struct {
	repos       map[string]string
	repoNames   []string
	defaultRepo string
}

type baseViewData struct {
	Repo     string
	Repos    []string
	Rev      string
	Branches []string
}

func init() {
	funcMap := template.FuncMap{
		"split":     strings.Split,
		"join":      strings.Join,
		"hasPrefix": strings.HasPrefix,
		"add": func(a, b int) int {
			return a + b
		},
		"slice": func(s []string, start, end int) []string {
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
	}

	var err error
	templates = template.New("").Funcs(funcMap)
	templates, err = templates.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}
}

func main() {
	application, err := loadApp()
	if err != nil {
		log.Fatal(err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	workDir := "."
	filesDir := http.Dir(filepath.Join(workDir, "static"))
	FileServer(r, "/static", filesDir)

	r.Get("/", application.rootHandler)
	r.Get("/repo/{repo}", application.repoIndexHandler)
	r.Get("/repo/{repo}/", application.repoIndexHandler)
	r.Get("/repo/{repo}/tree/{rev}", application.treeHandler)
	r.Get("/repo/{repo}/tree/{rev}/*", application.treeHandler)
	r.Get("/repo/{repo}/blob/{rev}/*", application.blobHandler)
	r.Get("/repo/{repo}/file-history/{rev}/*", application.fileHistoryHandler)
	r.Get("/repo/{repo}/file-diff/{hash}/*", application.fileDiffHandler)
	r.Get("/repo/{repo}/commits", application.commitsHandler)
	r.Get("/repo/{repo}/commits/{rev}", application.commitsHandler)
	r.Get("/repo/{repo}/commit/{hash}", application.commitHandler)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("Server starting on :8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func loadApp() (*app, error) {
	cfgPath := os.Getenv("GITBROWSER_CONFIG")
	if cfgPath == "" {
		cfgPath = "repos.json"
	}

	config, err := loadConfig(cfgPath)
	if err != nil {
		return nil, err
	}

	repos := make(map[string]string, len(config.Repos))
	repoNames := make([]string, 0, len(config.Repos))
	for _, repo := range config.Repos {
		name := strings.TrimSpace(repo.Name)
		if name == "" {
			return nil, errors.New("all repos must have a non-empty name")
		}
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("repo name %q cannot contain '/'", name)
		}
		if _, exists := repos[name]; exists {
			return nil, fmt.Errorf("repo name %q is duplicated", name)
		}

		repoPath := strings.TrimSpace(repo.Path)
		if repoPath == "" {
			return nil, fmt.Errorf("repo %q must have a non-empty path", name)
		}
		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			return nil, fmt.Errorf("resolve path for repo %q: %w", name, err)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("repo %q path %q is invalid: %w", name, absPath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("repo %q path %q is not a directory", name, absPath)
		}
		if err := git.ValidateRepository(absPath); err != nil {
			return nil, fmt.Errorf("repo %q path %q is not a valid git working tree: %w", name, absPath, err)
		}

		repos[name] = absPath
		repoNames = append(repoNames, name)
	}

	if len(repoNames) == 0 {
		return nil, errors.New("config must include at least one repo")
	}

	return &app{
		repos:       repos,
		repoNames:   repoNames,
		defaultRepo: repoNames[0],
	}, nil
}

func loadConfig(path string) (appConfig, error) {
	var cfg appConfig

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %q: %w", path, err)
	}

	return cfg, nil
}

func (a *app) repoPathFromRequest(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	repoName := chi.URLParam(r, "repo")
	repoPath, ok := a.repos[repoName]
	if !ok {
		http.NotFound(w, r)
		return "", "", false
	}
	return repoName, repoPath, true
}

func (a *app) baseData(repoName, repoPath, rev string) baseViewData {
	branches, _ := git.GetBranches(repoPath)
	return baseViewData{
		Repo:     repoName,
		Repos:    a.repoNames,
		Rev:      rev,
		Branches: branches,
	}
}

func (a *app) rootHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/repo/"+a.defaultRepo+"/", http.StatusFound)
}

func (a *app) repoIndexHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	currentBranch, err := git.GetCurrentBranch(repoPath)
	if err != nil || currentBranch == "" {
		currentBranch = "HEAD"
	}
	http.Redirect(w, r, "/repo/"+repoName+"/tree/"+currentBranch+"/", http.StatusFound)
}

func (a *app) treeHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	rev := chi.URLParam(r, "rev")
	path := chi.URLParam(r, "*")

	entries, err := git.ListTree(repoPath, rev, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		baseViewData
		Path    string
		Entries []git.TreeEntry
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Path:         path,
		Entries:      entries,
	}

	render(w, "tree.html", data)
}

func (a *app) blobHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	rev := chi.URLParam(r, "rev")
	path := chi.URLParam(r, "*")
	normalizedPath, ok := normalizeRepoRelativePath(repoPath, path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	content, err := git.GetFileContent(repoPath, rev, normalizedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lines := strings.Split(content, "\n")

	data := struct {
		baseViewData
		Path  string
		Lines []string
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Path:         normalizedPath,
		Lines:        lines,
	}

	render(w, "blob.html", data)
}

func (a *app) commitsHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	rev := chi.URLParam(r, "rev")
	if rev == "" {
		rev, _ = git.GetCurrentBranch(repoPath)
	}

	commits, err := git.GetLog(repoPath, rev)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		baseViewData
		Commits []git.LogEntry
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Commits:      commits,
	}

	render(w, "commits.html", data)
}

func (a *app) commitHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	hash := chi.URLParam(r, "hash")

	diff, err := git.GetCommitDiff(repoPath, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rev, _ := git.GetCurrentBranch(repoPath)

	data := struct {
		baseViewData
		Hash string
		Diff string
		Path string
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Hash:         hash,
		Diff:         diff,
		Path:         "",
	}

	render(w, "commit.html", data)
}

func (a *app) fileHistoryHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	rev := chi.URLParam(r, "rev")
	path := chi.URLParam(r, "*")
	normalizedPath, pathOK := normalizeRepoRelativePath(repoPath, path)
	if !pathOK {
		http.NotFound(w, r)
		return
	}

	commits, err := git.GetFileHistory(repoPath, rev, normalizedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		baseViewData
		Path    string
		Commits []git.FileHistoryEntry
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Path:         normalizedPath,
		Commits:      commits,
	}

	render(w, "file_history.html", data)
}

func (a *app) fileDiffHandler(w http.ResponseWriter, r *http.Request) {
	repoName, repoPath, ok := a.repoPathFromRequest(w, r)
	if !ok {
		return
	}

	hash := chi.URLParam(r, "hash")
	path := chi.URLParam(r, "*")
	normalizedPath, pathOK := normalizeRepoRelativePath(repoPath, path)
	if !pathOK {
		http.NotFound(w, r)
		return
	}

	diff, err := git.GetCommitFileDiff(repoPath, hash, normalizedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(diff) == "" {
		// If file path changed over time, resolve the path at this commit and retry.
		history, historyErr := git.GetFileHistory(repoPath, "HEAD", normalizedPath)
		if historyErr == nil {
			for _, entry := range history {
				if entry.Hash == hash && entry.Path != normalizedPath {
					if retryDiff, retryErr := git.GetCommitFileDiff(repoPath, hash, entry.Path); retryErr == nil {
						diff = retryDiff
						normalizedPath = entry.Path
					}
					break
				}
			}
		}
	}

	rev, _ := git.GetCurrentBranch(repoPath)
	data := struct {
		baseViewData
		Hash string
		Diff string
		Path string
	}{
		baseViewData: a.baseData(repoName, repoPath, rev),
		Hash:         hash,
		Diff:         diff,
		Path:         normalizedPath,
	}

	render(w, "commit.html", data)
}

func render(w http.ResponseWriter, name string, data interface{}) {
	err := templates.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func normalizeRepoRelativePath(repoPath, requestedPath string) (string, bool) {
	path := strings.TrimSpace(requestedPath)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", false
	}

	repoPath = filepath.ToSlash(filepath.Clean(repoPath))
	withLeadingSlash := "/" + path
	if strings.HasPrefix(withLeadingSlash, repoPath+"/") {
		path = strings.TrimPrefix(withLeadingSlash, repoPath+"/")
	} else {
		repoPathNoLeadingSlash := strings.TrimPrefix(repoPath, "/")
		if strings.HasPrefix(path, repoPathNoLeadingSlash+"/") {
			path = strings.TrimPrefix(path, repoPathNoLeadingSlash+"/")
		}
	}

	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "" || strings.HasPrefix(path, "../") {
		return "", false
	}
	return path, true
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
