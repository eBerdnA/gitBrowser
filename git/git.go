package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Command runs a native git command in the target repository and returns the output as a string.
func Command(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// LogEntry represents a single commit log entry.
type LogEntry struct {
	Hash    string
	Author  string
	Date    string
	Subject string
}

// FileHistoryEntry represents one history item for a file, including the path at that commit.
type FileHistoryEntry struct {
	LogEntry
	Path string
}

// GetLog returns the commit history of the repository.
func GetLog(repoPath, rev string) ([]LogEntry, error) {
	if rev == "" {
		rev = "HEAD"
	}
	// Format: hash|author|date|subject
	out, err := Command(repoPath, "log", rev, "--pretty=format:%H|%an|%ad|%s", "--date=short", "-n", "20")
	if err != nil {
		return nil, err
	}

	if out == "" {
		return []LogEntry{}, nil
	}

	lines := strings.Split(out, "\n")
	var entries []LogEntry
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) == 4 {
			entries = append(entries, LogEntry{
				Hash:    parts[0],
				Author:  parts[1],
				Date:    parts[2],
				Subject: parts[3],
			})
		}
	}

	return entries, nil
}

// TreeEntry represents a file or directory in the repository.
type TreeEntry struct {
	Mode string
	Type string
	Hash string
	Name string
	Path string
}

// ListTree returns the contents of a directory at a specific revision and path.
func ListTree(repoPath, rev, path string) ([]TreeEntry, error) {
	if rev == "" {
		rev = "HEAD"
	}
	// git ls-tree <rev> <path>
	// Output format: <mode> <type> <hash> <name>
	args := []string{"ls-tree", rev}
	if path != "" {
		// Ensure path doesn't start with / if it's meant to be relative to repo root
		path = strings.TrimPrefix(path, "/")
		if path != "" && !strings.HasSuffix(path, "/") {
			path += "/"
		}
		args = append(args, path)
	} else {
		args = append(args, ".")
	}

	out, err := Command(repoPath, args...)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return []TreeEntry{}, nil
	}

	lines := strings.Split(out, "\n")
	var entries []TreeEntry
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			name := strings.Join(fields[3:], " ")
			entries = append(entries, TreeEntry{
				Mode: fields[0],
				Type: fields[1],
				Hash: fields[2],
				Name: strings.TrimPrefix(name, path),
				Path: name,
			})
		}
	}
	return entries, nil
}

// GetFileContent returns the content of a file at a specific revision and path.
func GetFileContent(repoPath, rev, path string) (string, error) {
	if rev == "" {
		rev = "HEAD"
	}
	return Command(repoPath, "show", rev+":"+path)
}

// GetBranches returns a list of all local branches.
func GetBranches(repoPath string) ([]string, error) {
	out, err := Command(repoPath, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// GetCommitDiff returns the diff of a specific commit.
func GetCommitDiff(repoPath, hash string) (string, error) {
	return Command(repoPath, "show", hash)
}

// GetFileHistory returns commit history for a single file.
func GetFileHistory(repoPath, rev, path string) ([]FileHistoryEntry, error) {
	if rev == "" {
		rev = "HEAD"
	}
	path = strings.TrimPrefix(path, "/")

	out, err := Command(
		repoPath,
		"log",
		rev,
		"--pretty=format:__GB__%H|%an|%ad|%s",
		"--date=short",
		"-n", "50",
		"--follow",
		"--name-status",
		"--",
		path,
	)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []FileHistoryEntry{}, nil
	}

	lines := strings.Split(out, "\n")
	var entries []FileHistoryEntry

	var current *FileHistoryEntry
	currentPath := path
	nextPath := ""
	flush := func() {
		if current == nil {
			return
		}
		if current.Path == "" {
			current.Path = currentPath
		}
		entries = append(entries, *current)
		if nextPath != "" {
			currentPath = nextPath
		}
		current = nil
		nextPath = ""
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "__GB__") {
			flush()
			parts := strings.Split(strings.TrimPrefix(line, "__GB__"), "|")
			if len(parts) == 4 {
				current = &FileHistoryEntry{
					LogEntry: LogEntry{
						Hash:    parts[0],
						Author:  parts[1],
						Date:    parts[2],
						Subject: parts[3],
					},
					Path: currentPath,
				}
			}
			continue
		}
		if current == nil || strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		if strings.HasPrefix(status, "R") && len(parts) >= 3 {
			oldPath := parts[1]
			newPath := parts[2]
			if newPath == currentPath {
				current.Path = newPath
				nextPath = oldPath
			}
			continue
		}
		filePath := parts[1]
		if filePath == currentPath {
			current.Path = filePath
		}
	}
	flush()

	return entries, nil
}

// GetCommitFileDiff returns diff for a single file in a specific commit.
func GetCommitFileDiff(repoPath, hash, path string) (string, error) {
	path = strings.TrimPrefix(path, "/")
	return Command(repoPath, "show", hash, "--", path)
}

// GetCurrentBranch returns the name of the currently checked out branch.
func GetCurrentBranch(repoPath string) (string, error) {
	return Command(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
}

// ValidateRepository checks whether path points to a git working tree.
func ValidateRepository(repoPath string) error {
	out, err := Command(repoPath, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if out != "true" {
		return fmt.Errorf("%s is not a git repository", repoPath)
	}
	return nil
}
