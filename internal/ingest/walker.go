package ingest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bmatcuk/doublestar/v4"
)

// WalkOptions controls directory walking behavior.
type WalkOptions struct {
	AllowedDirs       []string
	AllowedExtensions []string
	ExcludedDirs      []string
	MaxFileSize       int64
}

// FileEntry is an eligible file discovered during walking.
type FileEntry struct {
	Path    string
	Content []byte
}

// ValidateDirectory checks that dir is under one of the allowed base directories.
func ValidateDirectory(dir string, allowedDirs []string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}

	for _, allowed := range allowedDirs {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if absDir == absAllowed || strings.HasPrefix(absDir, absAllowed+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("directory %q is not under any allowed base directory", dir)
}

// Walk recursively walks a directory for eligible files.
func Walk(dir string, opts WalkOptions) ([]FileEntry, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dir)
	}

	ragignorePatterns, _ := LoadRagignore(dir)

	extSet := make(map[string]bool, len(opts.AllowedExtensions))
	for _, ext := range opts.AllowedExtensions {
		extSet[ext] = true
	}

	excludeSet := make(map[string]bool, len(opts.ExcludedDirs))
	for _, d := range opts.ExcludedDirs {
		excludeSet[d] = true
	}

	var entries []FileEntry

	// Use WalkDir instead of Walk to avoid following directory symlinks (CRIT-02).
	// WalkDir calls os.Lstat (not os.Stat), so symlinks are reported correctly.
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		relPath, _ := filepath.Rel(dir, path)

		// Skip symlinks at directory and file level (ING-15)
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			name := filepath.Base(path)
			if excludeSet[name] {
				return filepath.SkipDir
			}
			if path != dir && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		name := filepath.Base(path)

		// Skip hidden files (except .env.example)
		if strings.HasPrefix(name, ".") && name != ".env.example" {
			return nil
		}

		// Check extension
		ext := filepath.Ext(name)
		if !extSet[ext] {
			return nil
		}

		// Check file size
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if opts.MaxFileSize > 0 && info.Size() > opts.MaxFileSize {
			return nil
		}

		// Check ragignore
		if MatchesRagignore(relPath, ragignorePatterns) {
			return nil
		}

		// Read file with O_NOFOLLOW and path verification
		content, err := readFileNoFollow(path, dir, opts.AllowedDirs)
		if err != nil {
			return nil // skip unreadable files
		}

		entries = append(entries, FileEntry{
			Path:    path,
			Content: content,
		})

		return nil
	})

	return entries, err
}

// readFileNoFollow reads a file ensuring no symlink following and path verification.
func readFileNoFollow(path string, baseDir string, allowedDirs []string) ([]byte, error) {
	// Resolve real path
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Verify real path is under allowed directory
	if err := ValidateDirectory(filepath.Dir(realPath), allowedDirs); err != nil {
		return nil, fmt.Errorf("path traversal detected: %w", err)
	}

	// Open with O_NOFOLLOW (ING-15)
	fd, err := syscall.Open(realPath, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	f := os.NewFile(uintptr(fd), realPath)
	defer f.Close()

	content, err := readAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return content, nil
}

func readAll(f *os.File) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, info.Size())
	_, err = f.Read(buf)
	return buf, err
}

// LoadRagignore reads a .ragignore file from the given directory root.
func LoadRagignore(dir string) ([]string, error) {
	path := filepath.Join(dir, ".ragignore")
	f, err := os.Open(path)
	if err != nil {
		return nil, nil // no ragignore
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

// MatchesRagignore checks if a relative path matches any ragignore pattern.
func MatchesRagignore(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := doublestar.Match(pattern, relPath); matched {
			return true
		}
		// Also try matching just the filename
		if matched, _ := doublestar.Match(pattern, filepath.Base(relPath)); matched {
			return true
		}
	}
	return false
}
