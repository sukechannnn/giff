// Package comments keeps the comments left on lines of code with 'i' in giff: a
// question or a request for Claude Code, which reads them with `giff comment`
// and writes its answer back. The comments of each repository live in a file of
// their own under ~/.giff/comments, so a Claude Code session working in one
// repository never picks up the comments of another.
package comments

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// The states of a comment
const (
	StatusPending    = "pending"     // left in giff, not picked up yet
	StatusInProgress = "in_progress" // picked up by Claude Code
	StatusAnswered   = "answered"    // answered by Claude Code
)

// Comment is a question or a request on lines of a file
type Comment struct {
	ID string `json:"id"`
	// Path is the file, relative to the repository root
	Path string `json:"path"`
	// StartLine and EndLine are the lines of the file the comment is on, 1-based
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
	// View is what giff showed: "unstaged diff", "staged diff", "file",
	// "PR diff", "commit <hash>"...
	View string `json:"view"`
	// Code is the lines the comment is on as giff showed them, with the '+' and
	// '-' of a diff
	Code string `json:"code"`
	// Question is what to answer now: the comment, or the latest reply to
	// the answers
	Question string `json:"question"`
	Status   string `json:"status"`
	// Answer is the latest answer, "" while the question waits for one
	Answer string `json:"answer,omitempty"`
	// Thread is the whole exchange, the comment first
	Thread     []Message  `json:"thread"`
	CreatedAt  time.Time  `json:"createdAt"`
	AnsweredAt *time.Time `json:"answeredAt,omitempty"`
}

// Who wrote a message of a thread
const (
	FromYou    = "you"
	FromClaude = "claude"
)

// Message is a message of the thread of a comment
type Message struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Location is where the comment is, as path:line or path:start-end
func (n Comment) Location() string {
	if n.EndLine > n.StartLine {
		return fmt.Sprintf("%s:%d-%d", n.Path, n.StartLine, n.EndLine)
	}
	return fmt.Sprintf("%s:%d", n.Path, n.StartLine)
}

// File is the comments of one repository
type File struct {
	Repo     string    `json:"repo"`
	Comments []Comment `json:"comments"`
}

// ErrNotFound is returned for a comment id that is not in the repository's comments
var ErrNotFound = errors.New("no such comment")

// Dir is where the comments are kept: ~/.giff/comments, or $GIFF_COMMENTS_DIR
func Dir() (string, error) {
	if dir := os.Getenv("GIFF_COMMENTS_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".giff", "comments"), nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// canonicalRoot is repoRoot as an absolute path with the symbolic links
// resolved, so that the same repository always maps to the same file
func canonicalRoot(repoRoot string) string {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		root = repoRoot
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return filepath.Clean(root)
}

// PathFor is the file of the comments of the repository at repoRoot: its name
// and a hash of its path, which tells apart the repositories, and the
// worktrees of one, of the same name
func PathFor(repoRoot string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	root := canonicalRoot(repoRoot)
	sum := sha1.Sum([]byte(root))
	name := unsafeName.ReplaceAllString(filepath.Base(root), "_")
	return filepath.Join(dir, fmt.Sprintf("%s-%s.json", name, hex.EncodeToString(sum[:])[:12])), nil
}

// Read returns the comments of the repository at repoRoot, none when it has no
// file yet
func Read(repoRoot string) (*File, error) {
	path, err := PathFor(repoRoot)
	if err != nil {
		return nil, err
	}
	return readFile(path, repoRoot)
}

func readFile(path, repoRoot string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{Repo: canonicalRoot(repoRoot)}, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Repo == "" {
		f.Repo = canonicalRoot(repoRoot)
	}
	// Comments kept before there were threads have their question and answer
	for i := range f.Comments {
		c := &f.Comments[i]
		if len(c.Thread) > 0 || c.Question == "" {
			continue
		}
		c.Thread = []Message{{From: FromYou, Text: c.Question, At: c.CreatedAt}}
		if c.Answer != "" {
			at := c.CreatedAt
			if c.AnsweredAt != nil {
				at = *c.AnsweredAt
			}
			c.Thread = append(c.Thread, Message{From: FromClaude, Text: c.Answer, At: at})
		}
	}
	return &f, nil
}

// Update reads the comments of the repository at repoRoot, lets fn change them
// and writes them back, locked against giff and `giff comment` doing the same
func Update(repoRoot string, fn func(f *File) error) error {
	path, err := PathFor(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	unlock, err := lock(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	f, err := readFile(path, repoRoot)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// Written whole to a temporary file first, so a reader never sees half
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// lockStale is how old a lock is when the process that took it is taken to
// have died holding it
const lockStale = 10 * time.Second

// lock takes the lock file at path, waiting a while for whoever holds it
func lock(path string) (func(), error) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStale {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the comments are locked: remove %s if no giff is writing them", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// newID returns a short random id for a comment
func newID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())[:8]
	}
	return hex.EncodeToString(b)
}

// Add stores n as a new comment, pending, and returns it with its id
func Add(repoRoot string, n Comment) (Comment, error) {
	n.ID = newID()
	n.Status = StatusPending
	n.CreatedAt = time.Now()
	n.Answer = ""
	n.AnsweredAt = nil
	n.Thread = []Message{{From: FromYou, Text: n.Question, At: n.CreatedAt}}
	err := Update(repoRoot, func(f *File) error {
		f.Comments = append(f.Comments, n)
		return nil
	})
	return n, err
}

// Next picks up the next comment to answer: one picked up already but not
// answered, or else the oldest pending one, which becomes in progress. It
// returns nil when there is none.
func Next(repoRoot string) (*Comment, error) {
	var picked *Comment
	err := Update(repoRoot, func(f *File) error {
		for i := range f.Comments {
			if f.Comments[i].Status == StatusInProgress {
				n := f.Comments[i]
				picked = &n
				return nil
			}
		}
		for i := range f.Comments {
			if f.Comments[i].Status == StatusPending {
				f.Comments[i].Status = StatusInProgress
				n := f.Comments[i]
				picked = &n
				return nil
			}
		}
		return nil
	})
	return picked, err
}

// SetAnswer adds answer to the thread of the comment id, as the answer to
// its question
func SetAnswer(repoRoot, id, answer string) error {
	return Update(repoRoot, func(f *File) error {
		for i := range f.Comments {
			if c := &f.Comments[i]; c.ID == id {
				now := time.Now()
				c.Answer = answer
				c.Status = StatusAnswered
				c.AnsweredAt = &now
				c.Thread = append(c.Thread, Message{From: FromClaude, Text: answer, At: now})
				return nil
			}
		}
		return ErrNotFound
	})
}

// Reply adds text to the thread of the comment id, as the next question,
// which waits for Claude Code again
func Reply(repoRoot, id, text string) error {
	return Update(repoRoot, func(f *File) error {
		for i := range f.Comments {
			if c := &f.Comments[i]; c.ID == id {
				c.Question = text
				c.Answer = ""
				c.Status = StatusPending
				c.Thread = append(c.Thread, Message{From: FromYou, Text: text, At: time.Now()})
				return nil
			}
		}
		return ErrNotFound
	})
}

// Remove deletes the comment id
func Remove(repoRoot, id string) error {
	return Update(repoRoot, func(f *File) error {
		for i := range f.Comments {
			if f.Comments[i].ID == id {
				f.Comments = append(f.Comments[:i], f.Comments[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

// RemoveIf deletes the comments drop picks, and returns how many
func RemoveIf(repoRoot string, drop func(Comment) bool) (int, error) {
	removed := 0
	err := Update(repoRoot, func(f *File) error {
		kept := f.Comments[:0]
		for _, n := range f.Comments {
			if drop(n) {
				removed++
				continue
			}
			kept = append(kept, n)
		}
		f.Comments = kept
		return nil
	})
	return removed, err
}
