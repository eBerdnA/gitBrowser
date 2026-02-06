package main

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestNormalizeRepoRelativePathHandlesAbsolutePathWithinRepo(t *testing.T) {
	repoPath := "/Users/andre.bering/dev/locLogger"
	absoluteRequest := "/Users/andre.bering/dev/locLogger/Android/androidApp/src/main/java/com/example/loclogger/MainActivity.kt"

	normalized, ok := normalizeRepoRelativePath(repoPath, absoluteRequest)
	if !ok {
		t.Fatalf("expected path to normalize successfully")
	}
	want := "Android/androidApp/src/main/java/com/example/loclogger/MainActivity.kt"
	if normalized != want {
		t.Fatalf("normalized path mismatch: got %q want %q", normalized, want)
	}
}

func TestFileDiffHandlerFallsBackToHistoricalPath(t *testing.T) {
	repoPath, hashSwitch, _, _, newPath := setupRepoWithRenamedFileForMainTests(t)

	a := &app{
		repos:       map[string]string{"testrepo": repoPath},
		repoNames:   []string{"testrepo"},
		defaultRepo: "testrepo",
	}

	req := httptest.NewRequest("GET", "/repo/testrepo/file-diff/"+hashSwitch+"/"+newPath, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("repo", "testrepo")
	rctx.URLParams.Add("hash", hashSwitch)
	rctx.URLParams.Add("*", newPath)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	a.fileDiffHandler(rr, req)

	if rr.Code != 200 {
		t.Fatalf("unexpected status code: got %d want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "toUri") {
		t.Fatalf("expected rendered diff to include file changes, got body %q", body)
	}
	if !strings.Contains(body, "androidApp/src/main/java/com/example/loclogger/MainActivity.kt") {
		t.Fatalf("expected rendered diff to include historical file path, got body %q", body)
	}
}

func setupRepoWithRenamedFileForMainTests(t *testing.T) (repoPath, hashSwitch, hashMove, oldPath, newPath string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found")
	}

	repoPath = t.TempDir()
	runGitMainTest(t, repoPath, "init")
	runGitMainTest(t, repoPath, "config", "user.name", "Test User")
	runGitMainTest(t, repoPath, "config", "user.email", "test@example.com")

	oldPath = "androidApp/src/main/java/com/example/loclogger/MainActivity.kt"
	newPath = "Android/" + oldPath

	writeFileMainTest(t, filepath.Join(repoPath, oldPath), mainActivityBeforeMainTest)
	runGitMainTest(t, repoPath, "add", ".")
	runGitMainTest(t, repoPath, "commit", "-m", "first working implementation")

	writeFileMainTest(t, filepath.Join(repoPath, oldPath), mainActivityAfterMainTest)
	runGitMainTest(t, repoPath, "add", ".")
	runGitMainTest(t, repoPath, "commit", "-m", "switched to `toUri`")
	hashSwitch = runGitMainTest(t, repoPath, "rev-parse", "HEAD")

	if err := os.MkdirAll(filepath.Join(repoPath, "Android"), 0o755); err != nil {
		t.Fatalf("mkdir Android failed: %v", err)
	}
	runGitMainTest(t, repoPath, "mv", "androidApp", "Android/androidApp")
	runGitMainTest(t, repoPath, "commit", "-m", "moved Android app to a subfolder")
	hashMove = runGitMainTest(t, repoPath, "rev-parse", "HEAD")

	return repoPath, hashSwitch, hashMove, oldPath, newPath
}

func runGitMainTest(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeFileMainTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

const mainActivityBeforeMainTest = `package com.example.loclogger

import android.net.Uri

fun mainActivityBefore() {
    val uri = Uri.parse("geo:0,0?q=1,2(label)")
    println(uri)
}
`

const mainActivityAfterMainTest = `package com.example.loclogger

import androidx.core.net.toUri

fun mainActivityAfter() {
    val uri = "geo:0,0?q=1,2(label)".toUri()
    println(uri)
}
`
