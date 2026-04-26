// Package gitutil provides shared utilities for cloning Git repositories
// and copying their contents to a target directory.
package gitutil

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type gitProvider string

const (
	gitProviderGitHub    gitProvider = "github"
	gitProviderGitLab    gitProvider = "gitlab"
	gitProviderBitbucket gitProvider = "bitbucket"
)

// ParseGitHubURL parses a GitHub URL into its clone URL, branch, and subdirectory path.
// Supported formats:
//   - https://github.com/owner/repo/tree/branch/path/to/dir
//   - https://github.com/owner/repo
//
// Branch names containing slashes (e.g. feature/my-branch) are supported when
// encoded as %2F in the URL. The raw (escaped) path is used for splitting so
// the encoded branch segment is preserved, then unescaped for the return value.
func ParseGitHubURL(rawURL string) (cloneURL, branch, subPath string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Host != "github.com" {
		return "", "", "", fmt.Errorf("unsupported host %q, only github.com is supported", u.Host)
	}

	// Use EscapedPath so that percent-encoded segments (e.g. %2F in branch
	// names) are not decoded before splitting on "/".
	rawPath := u.EscapedPath()

	// Path is like /owner/repo or /owner/repo/tree/branch/sub/path
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid GitHub URL: expected at least owner/repo in path")
	}

	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	cloneURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	// If URL contains /tree/<branch>/..., extract branch and subpath.
	// The branch segment is unescaped so encoded slashes (%2F) become real
	// slashes in the returned branch name.
	if len(parts) >= 4 && parts[2] == "tree" {
		branch, _ = url.PathUnescape(parts[3])
		if len(parts) > 4 {
			raw := strings.Join(parts[4:], "/")
			subPath, _ = url.PathUnescape(raw)
		}
	}

	return cloneURL, branch, subPath, nil
}

// ParseGitURL parses provider-specific git repository URLs into a clone URL,
// ref, and subdirectory path. It currently supports GitHub, GitLab, and
// Bitbucket tree-style URLs.
func ParseGitURL(rawURL string) (cloneURL, ref, subPath string, err error) {
	return ParseGitURLWithProvider(rawURL, "")
}

// ParseGitURLWithProvider parses a git repository URL using an optional
// provider hint for ambiguous self-hosted hosts.
func ParseGitURLWithProvider(rawURL, providerHint string) (cloneURL, ref, subPath string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid URL: %w", err)
	}

	rawPath := u.EscapedPath()
	parts := splitPath(rawPath)
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid git URL: expected at least owner/repo in path")
	}
	provider, err := providerFromURLParts(providerHint, u.Hostname(), parts)
	if err != nil {
		return "", "", "", err
	}

	var repoParts []string
	switch provider {
	case gitProviderGitHub:
		repoParts = parts[:2]
		if len(parts) >= 4 && parts[2] == "tree" {
			ref, _ = url.PathUnescape(parts[3])
			if len(parts) > 4 {
				raw := strings.Join(parts[4:], "/")
				subPath, _ = url.PathUnescape(raw)
			}
		}
	case gitProviderBitbucket:
		repoParts = parts[:2]
		if len(parts) >= 4 && parts[2] == "src" {
			ref, _ = url.PathUnescape(parts[3])
			if len(parts) > 4 {
				raw := strings.Join(parts[4:], "/")
				subPath, _ = url.PathUnescape(raw)
			}
		}
	case gitProviderGitLab:
		treeIndex := indexOf(parts, "-")
		switch {
		case treeIndex >= 1 && treeIndex+2 < len(parts) && parts[treeIndex+1] == "tree":
			repoParts = parts[:treeIndex]
			ref, _ = url.PathUnescape(parts[treeIndex+2])
			if len(parts) > treeIndex+3 {
				raw := strings.Join(parts[treeIndex+3:], "/")
				subPath, _ = url.PathUnescape(raw)
			}
		default:
			repoParts = parts
		}
	}

	if len(repoParts) < 2 {
		return "", "", "", fmt.Errorf("invalid git URL: expected at least owner/repo in path")
	}

	cloneURL = fmt.Sprintf("https://%s/%s.git", u.Host, strings.TrimSuffix(strings.Join(repoParts, "/"), ".git"))
	return cloneURL, ref, subPath, nil
}

// CloneAndCopy clones a supported git repository URL and copies its contents to targetDir.
// It handles parsing the URL, shallow cloning, navigating to subpaths, and cleanup.
func CloneAndCopy(repoURL, targetDir string, verbose bool) error {
	cloneURL, branch, subPath, err := ParseGitURL(repoURL)
	if err != nil {
		return fmt.Errorf("parse git URL: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "arctl-git-clone-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// git clone --branch works for branches and tags but not commit SHAs.
	// For SHAs, clone the default branch then checkout the specific commit.
	isSHA := isCommitSHA(branch)

	cloneArgs := []string{"clone", "--depth", "1"}
	if branch != "" && !isSHA {
		cloneArgs = append(cloneArgs, "--branch", branch)
	}
	cloneArgs = append(cloneArgs, cloneURL, tempDir)

	gitCmd := exec.Command("git", cloneArgs...)
	if verbose {
		gitCmd.Stdout = os.Stdout
		gitCmd.Stderr = os.Stderr
	}
	if err := gitCmd.Run(); err != nil {
		return fmt.Errorf("clone repository: %w", err)
	}

	// For commit SHAs, fetch the specific commit and check it out.
	if isSHA {
		fetchCmd := exec.Command("git", "-C", tempDir, "fetch", "--depth", "1", "origin", branch)
		if verbose {
			fetchCmd.Stdout = os.Stdout
			fetchCmd.Stderr = os.Stderr
		}
		if err := fetchCmd.Run(); err != nil {
			return fmt.Errorf("fetch commit %s: %w", branch, err)
		}

		checkoutCmd := exec.Command("git", "-C", tempDir, "checkout", "FETCH_HEAD")
		if verbose {
			checkoutCmd.Stdout = os.Stdout
			checkoutCmd.Stderr = os.Stderr
		}
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("checkout commit %s: %w", branch, err)
		}
	}

	return CopyRepoContents(tempDir, subPath, targetDir)
}

// InferRepositoryTreeURL returns a provider-specific tree URL for the Git
// repository containing dir, using the origin remote and current ref.
func InferRepositoryTreeURL(dir string) (string, error) {
	return InferRepositoryTreeURLWithProvider(dir, "")
}

// InferRepositoryTreeURLWithProvider returns a provider-specific tree URL for
// the Git repository containing dir, using an optional provider hint for
// ambiguous self-hosted remotes.
func InferRepositoryTreeURLWithProvider(dir, providerHint string) (string, error) {
	repoRoot, err := gitOutput(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("resolve git repository root: %w", err)
	}
	remoteURL, err := gitOutput(repoRoot, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("resolve git origin remote: %w", err)
	}

	baseURL, provider, err := normalizeRemoteWebURLWithProvider(remoteURL, providerHint)
	if err != nil {
		return "", err
	}

	ref, err := gitOutput(repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		ref, err = gitOutput(repoRoot, "rev-parse", "HEAD")
		if err != nil {
			return "", fmt.Errorf("resolve current git ref: %w", err)
		}
	}

	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absRepoRoot); err == nil {
		absRepoRoot = resolved
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	relPath, err := filepath.Rel(absRepoRoot, absDir)
	if err != nil {
		return "", fmt.Errorf("resolve repository subpath: %w", err)
	}
	if relPath == "." {
		relPath = ""
	}

	return buildTreeURL(baseURL, provider, ref, filepath.ToSlash(relPath)), nil
}

// resolveSubPath validates and resolves a subPath within repoDir, returning
// the resolved source directory. It rejects absolute paths and paths that
// escape the repository root via directory traversal.
func resolveSubPath(repoDir, subPath string) (string, error) {
	if filepath.IsAbs(subPath) {
		return "", fmt.Errorf("subpath %q must be relative", subPath)
	}

	srcDir := filepath.Join(repoDir, filepath.Clean(subPath))

	absRepo, err := filepath.Abs(repoDir)
	if err != nil {
		return "", fmt.Errorf("resolve repo directory: %w", err)
	}
	absSrc, err := filepath.Abs(srcDir)
	if err != nil {
		return "", fmt.Errorf("resolve subpath directory: %w", err)
	}
	if !strings.HasPrefix(absSrc, absRepo+string(filepath.Separator)) && absSrc != absRepo {
		return "", fmt.Errorf("subpath %q escapes repository directory", subPath)
	}

	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return "", fmt.Errorf("subdirectory %q not found in repository", subPath)
	}

	return srcDir, nil
}

// CopyRepoContents copies files from a cloned repository to the output directory.
// It navigates to the subPath if specified and skips the .git directory.
// Symlinks are skipped to prevent symlink traversal attacks from untrusted repos.
func CopyRepoContents(repoDir, subPath, targetDir string) error {
	srcDir := repoDir
	if subPath != "" {
		resolved, err := resolveSubPath(repoDir, subPath)
		if err != nil {
			return err
		}
		srcDir = resolved
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read source directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}

		// Skip symlinks to prevent traversal attacks from untrusted repos
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(targetDir, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("copy directory %s: %w", entry.Name(), err)
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copy file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// CopyDir recursively copies a directory tree, skipping symlinks.
func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		// Skip symlinks to prevent traversal attacks
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFile copies a single regular file. The caller must ensure src is not a symlink.
func CopyFile(src, dst string) error {
	// Verify the source is a regular file via Lstat (does not follow symlinks)
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to copy symlink: %s", src)
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode().Perm())
}

// RepoNameFromCloneURL extracts the repository name from a clone URL
// (e.g., "https://github.com/org/my-repo.git" -> "my-repo").
func RepoNameFromCloneURL(cloneURL string) string {
	idx := strings.LastIndex(cloneURL, "/")
	if idx < 0 {
		return ""
	}
	return strings.TrimSuffix(cloneURL[idx+1:], ".git")
}

func providerFromHost(host string) (gitProvider, error) {
	switch {
	case strings.EqualFold(host, "github.com") || strings.Contains(strings.ToLower(host), "github"):
		return gitProviderGitHub, nil
	case strings.EqualFold(host, "bitbucket.org") || strings.Contains(strings.ToLower(host), "bitbucket"):
		return gitProviderBitbucket, nil
	case strings.EqualFold(host, "gitlab.com") || strings.Contains(strings.ToLower(host), "gitlab"):
		return gitProviderGitLab, nil
	default:
		return "", fmt.Errorf("unsupported host %q, only github, gitlab, and bitbucket URLs are supported", host)
	}
}

func providerFromURLParts(providerHint, host string, parts []string) (gitProvider, error) {
	if providerHint != "" {
		return parseProviderHint(providerHint)
	}
	if provider, err := providerFromHost(host); err == nil {
		return provider, nil
	}
	if provider, ok := providerFromPath(parts); ok {
		return provider, nil
	}
	return "", fmt.Errorf("unsupported host %q, only github, gitlab, and bitbucket URLs are supported", host)
}

func parseProviderHint(value string) (gitProvider, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(gitProviderGitHub):
		return gitProviderGitHub, nil
	case string(gitProviderGitLab):
		return gitProviderGitLab, nil
	case string(gitProviderBitbucket):
		return gitProviderBitbucket, nil
	default:
		return "", fmt.Errorf("unsupported git provider %q, expected github, gitlab, or bitbucket", value)
	}
}

func providerFromPath(parts []string) (gitProvider, bool) {
	switch {
	case len(parts) >= 4 && parts[2] == "tree":
		return gitProviderGitHub, true
	case len(parts) >= 4 && parts[2] == "src":
		return gitProviderBitbucket, true
	case isGitLabTreePath(parts):
		return gitProviderGitLab, true
	case len(parts) > 2:
		// Nested repository namespaces are a GitLab-specific pattern among the
		// providers we support, so treat unknown hosts with 3+ path segments as
		// GitLab-compatible repository roots.
		return gitProviderGitLab, true
	default:
		return "", false
	}
}

func normalizeRemoteWebURLWithProvider(raw, providerHint string) (string, gitProvider, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", fmt.Errorf("git origin remote is empty")
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("parse git remote URL: %w", err)
		}
		path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		if path == "" {
			return "", "", fmt.Errorf("git remote URL is missing repository path: %s", raw)
		}
		provider, err := providerFromURLParts(providerHint, u.Hostname(), splitPath(u.EscapedPath()))
		if err != nil {
			return "", "", err
		}
		webHost := u.Host
		if strings.EqualFold(u.Scheme, "ssh") {
			webHost = u.Hostname()
		}
		return fmt.Sprintf("https://%s/%s", webHost, path), provider, nil
	}

	matches := regexp.MustCompile(`^(?:ssh://)?git@([^:/]+)[:/]([^/].+?)(?:\.git)?$`).FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 3 {
		return "", "", fmt.Errorf("unsupported git remote URL format: %s", raw)
	}
	host := matches[1]
	repoPath := strings.Trim(matches[2], "/")
	provider, err := providerFromURLParts(providerHint, host, splitPath(repoPath))
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("https://%s/%s", host, repoPath), provider, nil
}

func buildTreeURL(baseURL string, provider gitProvider, ref, subPath string) string {
	ref = escapePathSegment(ref)
	subPath = escapePathSegments(subPath)
	switch provider {
	case gitProviderGitHub:
		if subPath == "" {
			return fmt.Sprintf("%s/tree/%s", baseURL, ref)
		}
		return fmt.Sprintf("%s/tree/%s/%s", baseURL, ref, subPath)
	case gitProviderGitLab:
		if subPath == "" {
			return fmt.Sprintf("%s/-/tree/%s", baseURL, ref)
		}
		return fmt.Sprintf("%s/-/tree/%s/%s", baseURL, ref, subPath)
	case gitProviderBitbucket:
		if subPath == "" {
			return fmt.Sprintf("%s/src/%s", baseURL, ref)
		}
		return fmt.Sprintf("%s/src/%s/%s", baseURL, ref, subPath)
	default:
		return baseURL
	}
}

func escapePathSegments(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	parts := strings.Split(value, "/")
	for index, part := range parts {
		parts[index] = escapePathSegment(part)
	}
	return strings.Join(parts, "/")
}

func escapePathSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func splitPath(rawPath string) []string {
	trimmed := strings.Trim(rawPath, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func isGitLabTreePath(parts []string) bool {
	treeIndex := indexOf(parts, "-")
	return treeIndex >= 1 && treeIndex+2 < len(parts) && parts[treeIndex+1] == "tree"
}

func gitOutput(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func indexOf(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

// commitSHAPattern matches full (40-char) and abbreviated (7-40 char)
// hexadecimal commit SHAs.
var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// isCommitSHA returns true if ref looks like a Git commit SHA rather than a
// branch or tag name. This is a heuristic: a 7-40 character hex string.
func isCommitSHA(ref string) bool {
	return commitSHAPattern.MatchString(ref)
}
