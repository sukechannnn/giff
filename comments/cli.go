package comments

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sukechannnn/giff/git"
)

//go:embed skill/giff/SKILL.md
var skillFS embed.FS

const usage = `usage: giff comment <command>

The comments left with 'i' in giff, of the Git repository of the current directory.

commands:
  list [--json] [--all]     the comments not answered yet (--all: and those answered)
  next [--wait MINUTES | --forever]
                            pick up the next comment to answer, as JSON, waiting
                            for one MINUTES minutes or until there is one
  answer <id> [text]        answer the comment (the text is read from stdin when not given)
  reply <id> [text]         reply to the answer, for Claude Code to answer again (stdin too)
  rm <id>                   delete the comment
  clear [--all]             delete the answered comments (--all: every comment)
  path                      print the file the comments are kept in
  install-skill             install the giff skill for Claude Code
`

// pollInterval is how often `next --wait` looks for a new comment
const pollInterval = time.Second

// Run runs `giff comment args...` and returns the exit code
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if args[0] == "install-skill" {
		return installSkill(stdout, stderr)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	repoRoot, err := git.FindGitRoot(cwd)
	if err != nil {
		fmt.Fprintln(stderr, "giff comment: not in a Git repository")
		return 1
	}

	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet("giff comment "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	switch cmd {
	case "list":
		asJSON := fs.Bool("json", false, "print the comments as JSON")
		all := fs.Bool("all", false, "list the answered comments too")
		if fs.Parse(rest) != nil {
			return 2
		}
		return list(repoRoot, *asJSON, *all, stdout, stderr)
	case "next":
		wait := fs.Int("wait", 0, "minutes to wait for a comment when there is none")
		forever := fs.Bool("forever", false, "wait for a comment until there is one")
		if fs.Parse(rest) != nil {
			return 2
		}
		if *forever {
			return next(repoRoot, waitForever, stdout, stderr)
		}
		return next(repoRoot, time.Duration(*wait)*time.Minute, stdout, stderr)
	case "answer", "reply":
		if len(rest) == 0 {
			fmt.Fprintf(stderr, "usage: giff comment %s <id> [text]\n", cmd)
			return 2
		}
		text := strings.Join(rest[1:], " ")
		if text == "" {
			data, err := io.ReadAll(stdin)
			if err != nil {
				fmt.Fprintln(stderr, "giff comment:", err)
				return 1
			}
			text = string(data)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			fmt.Fprintf(stderr, "giff comment: the %s is empty\n", cmd)
			return 2
		}
		write, done := SetAnswer, "Answered"
		if cmd == "reply" {
			write, done = Reply, "Replied to"
		}
		if err := write(repoRoot, rest[0], text); err != nil {
			fmt.Fprintf(stderr, "giff comment: %s: %v\n", rest[0], err)
			return 1
		}
		fmt.Fprintln(stdout, done, rest[0])
		return 0
	case "rm":
		if len(rest) != 1 {
			fmt.Fprintln(stderr, "usage: giff comment rm <id>")
			return 2
		}
		if err := Remove(repoRoot, rest[0]); err != nil {
			fmt.Fprintf(stderr, "giff comment: %s: %v\n", rest[0], err)
			return 1
		}
		fmt.Fprintln(stdout, "Removed", rest[0])
		return 0
	case "clear":
		all := fs.Bool("all", false, "delete every comment, not only the answered ones")
		if fs.Parse(rest) != nil {
			return 2
		}
		n, err := RemoveIf(repoRoot, func(n Comment) bool { return *all || n.Status == StatusAnswered })
		if err != nil {
			fmt.Fprintln(stderr, "giff comment:", err)
			return 1
		}
		fmt.Fprintf(stdout, "Removed %d comments\n", n)
		return 0
	case "path":
		path, err := PathFor(repoRoot)
		if err != nil {
			fmt.Fprintln(stderr, "giff comment:", err)
			return 1
		}
		fmt.Fprintln(stdout, path)
		return 0
	}
	fmt.Fprintf(stderr, "giff comment: unknown command %q\n\n%s", cmd, usage)
	return 2
}

// commentJSON is a comment as `next` and `list --json` print it, with the
// repository it belongs to
type commentJSON struct {
	Comment
	Repo string `json:"repo"`
}

func list(repoRoot string, asJSON, all bool, stdout, stderr io.Writer) int {
	f, err := Read(repoRoot)
	if err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	shown := []commentJSON{}
	for _, n := range f.Comments {
		if all || n.Status != StatusAnswered {
			shown = append(shown, commentJSON{Comment: n, Repo: f.Repo})
		}
	}
	if asJSON {
		data, _ := json.MarshalIndent(shown, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	if len(shown) == 0 {
		fmt.Fprintln(stderr, "No comments")
		return 0
	}
	for _, n := range shown {
		question, _, _ := strings.Cut(n.Question, "\n")
		fmt.Fprintf(stdout, "%s  %-11s  %s  %s\n", n.ID, n.Status, n.Location(), question)
	}
	return 0
}

// waitForever, as the time next waits, is until there is a comment
const waitForever time.Duration = -1

// next prints the next comment to answer, waiting up to wait for one
func next(repoRoot string, wait time.Duration, stdout, stderr io.Writer) int {
	deadline := time.Now().Add(wait)
	waiting := false
	for {
		n, err := Next(repoRoot)
		if err != nil {
			fmt.Fprintln(stderr, "giff comment:", err)
			return 1
		}
		if n != nil {
			data, _ := json.Marshal(commentJSON{Comment: *n, Repo: canonicalRoot(repoRoot)})
			fmt.Fprintln(stdout, string(data))
			return 0
		}
		if wait != waitForever && !time.Now().Before(deadline) {
			if wait > 0 {
				fmt.Fprintln(stderr, "Timed out waiting for comments")
			} else {
				fmt.Fprintln(stderr, "No comments")
			}
			return 1
		}
		if !waiting {
			if wait == waitForever {
				fmt.Fprintln(stderr, "Waiting for comments...")
			} else {
				fmt.Fprintf(stderr, "Waiting for comments... (timeout: %s)\n", wait)
			}
			waiting = true
		}
		time.Sleep(pollInterval)
	}
}

// installSkill writes the giff skill to ~/.claude/skills
func installSkill(stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	data, err := skillFS.ReadFile("skill/giff/SKILL.md")
	if err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	dir := filepath.Join(home, ".claude", "skills", "giff")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintln(stderr, "giff comment:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Installed the giff skill:", path)
	fmt.Fprintln(stdout, "Run /giff in Claude Code, in the repository you review with giff.")
	return 0
}
