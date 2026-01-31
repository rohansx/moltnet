package git

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Service struct {
	basePath string
}

func New(basePath string) *Service {
	os.MkdirAll(basePath, 0755)
	return &Service{basePath: basePath}
}

func (s *Service) repoPath(slug string) string {
	return filepath.Join(s.basePath, slug+".git")
}

// InitRepo initializes a new bare git repository
func (s *Service) InitRepo(slug string) (string, error) {
	path := s.repoPath(slug)
	
	// Create a working directory (not bare for simplicity)
	_, err := git.PlainInit(path, false)
	if err != nil {
		return "", fmt.Errorf("failed to init repo: %w", err)
	}

	// Create initial README
	readmePath := filepath.Join(path, "README.md")
	content := fmt.Sprintf("# %s\n\nCreated on Moltnet.ai\n", slug)
	if err := os.WriteFile(readmePath, []byte(content), 0644); err != nil {
		return "", err
	}

	// Initial commit
	repo, _ := git.PlainOpen(path)
	w, _ := repo.Worktree()
	w.Add("README.md")
	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Moltnet",
			Email: "system@moltnet.ai",
			When:  time.Now(),
		},
	})

	return path, err
}

// CloneRepo clones a repo for forking
func (s *Service) CloneRepo(sourceSlug, targetSlug string) (string, error) {
	sourcePath := s.repoPath(sourceSlug)
	targetPath := s.repoPath(targetSlug)

	// Simple directory copy (go-git clone is for remote)
	if err := copyDir(sourcePath, targetPath); err != nil {
		return "", fmt.Errorf("failed to clone repo: %w", err)
	}

	return targetPath, nil
}

// GetFiles returns the file tree of a repo
func (s *Service) GetFiles(slug, branch string) ([]FileInfo, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	tree.Files().ForEach(func(f *object.File) error {
		files = append(files, FileInfo{
			Path: f.Name,
			Mode: f.Mode.String(),
			Size: f.Size,
		})
		return nil
	})

	return files, nil
}

// ReadFile reads a file from the repo
func (s *Service) ReadFile(slug, filePath string) (string, error) {
	path := s.repoPath(slug)
	fullPath := filepath.Join(path, filePath)
	
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// WriteFile writes a file and commits it
func (s *Service) WriteFile(slug, filePath, content, message, authorName, authorEmail string) (*CommitInfo, error) {
	path := s.repoPath(slug)
	fullPath := filepath.Join(path, filePath)

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(fullPath), 0755)

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	// Open repo and commit
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	_, err = w.Add(filePath)
	if err != nil {
		return nil, err
	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, err
	}

	obj, _ := repo.CommitObject(commit)
	return &CommitInfo{
		Hash:    commit.String(),
		Message: message,
		Author:  authorName,
		Time:    obj.Author.When,
	}, nil
}

// DeleteFile deletes a file and commits
func (s *Service) DeleteFile(slug, filePath, message, authorName, authorEmail string) (*CommitInfo, error) {
	path := s.repoPath(slug)
	fullPath := filepath.Join(path, filePath)

	if err := os.Remove(fullPath); err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	_, err = w.Remove(filePath)
	if err != nil {
		return nil, err
	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, err
	}

	obj, _ := repo.CommitObject(commit)
	return &CommitInfo{
		Hash:    commit.String(),
		Message: message,
		Author:  authorName,
		Time:    obj.Author.When,
	}, nil
}

// GetCommits returns commit history
func (s *Service) GetCommits(slug string, limit int) ([]CommitInfo, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	count := 0
	iter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return io.EOF
		}
		commits = append(commits, CommitInfo{
			Hash:    c.Hash.String(),
			Message: c.Message,
			Author:  c.Author.Name,
			Time:    c.Author.When,
		})
		count++
		return nil
	})

	return commits, nil
}

// CreateBranch creates a new branch
func (s *Service) CreateBranch(slug, branchName string) error {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	ref, err := repo.Head()
	if err != nil {
		return err
	}

	branchRef := plumbing.NewBranchReferenceName(branchName)
	newRef := plumbing.NewHashReference(branchRef, ref.Hash())
	return repo.Storer.SetReference(newRef)
}

// ListBranches lists all branches
func (s *Service) ListBranches(slug string) ([]string, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	iter, err := repo.Branches()
	if err != nil {
		return nil, err
	}

	var branches []string
	iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})

	return branches, nil
}

// GetCommitDiff returns the diff for a specific commit
func (s *Service) GetCommitDiff(slug, commitHash string) (*DiffResult, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %w", err)
	}

	// Get commit tree
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	// Get parent tree (if exists)
	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	// Calculate diff
	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return nil, err
	}

	var files []FileDiff
	for _, change := range changes {
		fd := FileDiff{
			Path:   change.To.Name,
			Status: getChangeStatus(change),
		}

		if change.From.Name != "" && change.From.Name != change.To.Name {
			fd.OldPath = change.From.Name
		}

		// Get patch
		patch, err := change.Patch()
		if err == nil && patch != nil {
			fd.Patch = patch.String()
			fd.Additions, fd.Deletions = countPatchLines(patch.String())
		}

		files = append(files, fd)
	}

	return &DiffResult{
		CommitHash: commitHash,
		Message:    commit.Message,
		Author:     commit.Author.Name,
		Time:       commit.Author.When,
		Files:      files,
		Stats: DiffStats{
			FilesChanged: len(files),
			Additions:    sumAdditions(files),
			Deletions:    sumDeletions(files),
		},
	}, nil
}

// GetBranchDiff returns diff between two branches (for PRs)
func (s *Service) GetBranchDiff(slug, sourceBranch, targetBranch string) (*DiffResult, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	// Get source branch head
	sourceRef, err := repo.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return nil, fmt.Errorf("source branch not found: %w", err)
	}

	// Get target branch head
	targetRef, err := repo.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		// Fallback to HEAD if target not found
		targetRef, err = repo.Head()
		if err != nil {
			return nil, fmt.Errorf("target branch not found: %w", err)
		}
	}

	sourceCommit, err := repo.CommitObject(sourceRef.Hash())
	if err != nil {
		return nil, err
	}

	targetCommit, err := repo.CommitObject(targetRef.Hash())
	if err != nil {
		return nil, err
	}

	sourceTree, err := sourceCommit.Tree()
	if err != nil {
		return nil, err
	}

	targetTree, err := targetCommit.Tree()
	if err != nil {
		return nil, err
	}

	// Diff from target to source (what source adds/changes)
	changes, err := object.DiffTree(targetTree, sourceTree)
	if err != nil {
		return nil, err
	}

	var files []FileDiff
	for _, change := range changes {
		fd := FileDiff{
			Path:   change.To.Name,
			Status: getChangeStatus(change),
		}

		if change.From.Name != "" && change.From.Name != change.To.Name {
			fd.OldPath = change.From.Name
		}

		patch, err := change.Patch()
		if err == nil && patch != nil {
			fd.Patch = patch.String()
			fd.Additions, fd.Deletions = countPatchLines(patch.String())
		}

		files = append(files, fd)
	}

	return &DiffResult{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Files:        files,
		Stats: DiffStats{
			FilesChanged: len(files),
			Additions:    sumAdditions(files),
			Deletions:    sumDeletions(files),
		},
	}, nil
}

// MergeBranches merges source into target branch
func (s *Service) MergeBranches(slug, sourceBranch, targetBranch, authorName, authorEmail, message string) (*CommitInfo, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	// Checkout target branch
	targetRef := plumbing.NewBranchReferenceName(targetBranch)
	err = w.Checkout(&git.CheckoutOptions{
		Branch: targetRef,
		Create: false,
	})
	if err != nil {
		// If target doesn't exist, use current HEAD
		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.HEAD,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to checkout target: %w", err)
		}
	}

	// Get source branch ref
	sourceRef, err := repo.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return nil, fmt.Errorf("source branch not found: %w", err)
	}

	// Perform merge (fast-forward style for simplicity)
	// In a real implementation, you'd do a proper 3-way merge
	sourceCommit, err := repo.CommitObject(sourceRef.Hash())
	if err != nil {
		return nil, err
	}

	sourceTree, err := sourceCommit.Tree()
	if err != nil {
		return nil, err
	}

	// Apply all files from source
	sourceTree.Files().ForEach(func(f *object.File) error {
		content, err := f.Contents()
		if err != nil {
			return nil
		}
		fullPath := filepath.Join(path, f.Name)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
		w.Add(f.Name)
		return nil
	})

	// Create merge commit
	if message == "" {
		message = fmt.Sprintf("Merge branch '%s' into %s", sourceBranch, targetBranch)
	}

	commit, err := w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, err
	}

	obj, _ := repo.CommitObject(commit)
	return &CommitInfo{
		Hash:    commit.String(),
		Message: message,
		Author:  authorName,
		Time:    obj.Author.When,
	}, nil
}

// GetCommit returns a single commit's info
func (s *Service) GetCommit(slug, commitHash string) (*CommitInfo, error) {
	path := s.repoPath(slug)
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("commit not found: %w", err)
	}

	return &CommitInfo{
		Hash:    commit.Hash.String(),
		Message: commit.Message,
		Author:  commit.Author.Name,
		Email:   commit.Author.Email,
		Time:    commit.Author.When,
	}, nil
}

// Helper functions
func getChangeStatus(change *object.Change) string {
	if change.From.Name == "" {
		return "added"
	}
	if change.To.Name == "" {
		return "deleted"
	}
	if change.From.Name != change.To.Name {
		return "renamed"
	}
	return "modified"
}

func countPatchLines(patch string) (additions, deletions int) {
	for _, line := range splitLines(patch) {
		if len(line) > 0 {
			if line[0] == '+' && (len(line) < 2 || line[1] != '+') {
				additions++
			} else if line[0] == '-' && (len(line) < 2 || line[1] != '-') {
				deletions++
			}
		}
	}
	return
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func sumAdditions(files []FileDiff) int {
	total := 0
	for _, f := range files {
		total += f.Additions
	}
	return total
}

func sumDeletions(files []FileDiff) int {
	total := 0
	for _, f := range files {
		total += f.Deletions
	}
	return total
}

// Types

type FileInfo struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Size int64  `json:"size"`
}

type CommitInfo struct {
	Hash    string    `json:"hash"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Email   string    `json:"email,omitempty"`
	Time    time.Time `json:"time"`
}

type FileDiff struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"` // added, modified, deleted, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}

type DiffStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

type DiffResult struct {
	CommitHash   string     `json:"commit_hash,omitempty"`
	SourceBranch string     `json:"source_branch,omitempty"`
	TargetBranch string     `json:"target_branch,omitempty"`
	Message      string     `json:"message,omitempty"`
	Author       string     `json:"author,omitempty"`
	Time         time.Time  `json:"time,omitempty"`
	Files        []FileDiff `json:"files"`
	Stats        DiffStats  `json:"stats"`
}

// Helper to copy directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
