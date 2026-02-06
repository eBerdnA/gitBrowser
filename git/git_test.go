package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFileHistoryTracksPathAcrossRename(t *testing.T) {
	repoPath, hashSwitch, hashMove, oldPath, newPath := setupRepoWithRenamedFile(t)

	entries, err := GetFileHistory(repoPath, "HEAD", newPath)
	if err != nil {
		t.Fatalf("GetFileHistory returned error: %v", err)
	}
	if len(entries) < 3 {
		t.Fatalf("expected at least 3 history entries, got %d", len(entries))
	}

	var foundSwitch, foundMove bool
	for _, entry := range entries {
		if entry.Hash == hashSwitch {
			foundSwitch = true
			if entry.Path != oldPath {
				t.Fatalf("switch commit path mismatch: got %q want %q", entry.Path, oldPath)
			}
		}
		if entry.Hash == hashMove {
			foundMove = true
			if entry.Path != newPath {
				t.Fatalf("move commit path mismatch: got %q want %q", entry.Path, newPath)
			}
		}
	}

	if !foundSwitch {
		t.Fatalf("expected to find switch commit %s in history", hashSwitch)
	}
	if !foundMove {
		t.Fatalf("expected to find move commit %s in history", hashMove)
	}
}

func TestGetCommitFileDiffRequiresPathAtThatCommit(t *testing.T) {
	repoPath, hashSwitch, _, oldPath, newPath := setupRepoWithRenamedFile(t)

	emptyDiff, err := GetCommitFileDiff(repoPath, hashSwitch, newPath)
	if err != nil {
		t.Fatalf("GetCommitFileDiff returned error for new path: %v", err)
	}
	if strings.TrimSpace(emptyDiff) != "" {
		t.Fatalf("expected empty diff for path that did not exist at commit, got %q", emptyDiff)
	}

	diff, err := GetCommitFileDiff(repoPath, hashSwitch, oldPath)
	if err != nil {
		t.Fatalf("GetCommitFileDiff returned error for historical path: %v", err)
	}
	if !strings.Contains(diff, "diff --git") {
		t.Fatalf("expected patch output to contain diff header, got %q", diff)
	}
	if !strings.Contains(diff, "toUri") {
		t.Fatalf("expected patch output to contain updated symbol, got %q", diff)
	}
}

func setupRepoWithRenamedFile(t *testing.T) (repoPath, hashSwitch, hashMove, oldPath, newPath string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found")
	}

	repoPath = t.TempDir()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	oldPath = "androidApp/src/main/java/com/example/loclogger/MainActivity.kt"
	newPath = "Android/" + oldPath

	writeFile(t, filepath.Join(repoPath, oldPath), mainActivityBefore)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "first working implementation")

	writeFile(t, filepath.Join(repoPath, oldPath), mainActivityAfter)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "switched to `toUri`")
	hashSwitch = runGit(t, repoPath, "rev-parse", "HEAD")

	if err := os.MkdirAll(filepath.Join(repoPath, "Android"), 0o755); err != nil {
		t.Fatalf("mkdir Android failed: %v", err)
	}
	runGit(t, repoPath, "mv", "androidApp", "Android/androidApp")
	runGit(t, repoPath, "commit", "-m", "moved Android app to a subfolder")
	hashMove = runGit(t, repoPath, "rev-parse", "HEAD")

	return repoPath, hashSwitch, hashMove, oldPath, newPath
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

const mainActivityBefore = `package com.example.loclogger

import android.net.Uri

fun mainActivityBefore() {
    val uri = Uri.parse("geo:0,0?q=1,2(label)")
    println(uri)
}
`

const mainActivityAfter = `package com.example.loclogger

import androidx.core.net.toUri

fun mainActivityAfter() {
    val uri = "geo:0,0?q=1,2(label)".toUri()
    println(uri)
}
`
