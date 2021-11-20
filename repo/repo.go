package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// The directory where episode files are stored.
	EpisodeDir = "_episodes"

	// The file where guest metadata are stored.
	GuestFile = "_data/guests.yaml"
)

// Root returns the root directory of the repository.
func Root() (string, error) {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ChdirRoot changes the current working directory to the repository root.
func ChdirRoot() error {
	root, err := Root()
	if err != nil {
		return err
	}
	return os.Chdir(root)
}

// RemoteRepo returns the repository name corresponding to the given git
// remote.
func RemoteRepo(remote string) (string, error) {
	data, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		return "", err
	}
	repo := filepath.Base(strings.TrimSpace(string(data)))
	return strings.TrimSuffix(repo, ".git"), nil
}

// FileExists reports whether the specified file path exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
