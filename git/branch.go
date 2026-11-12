package git

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PullRequest is an open pull request, as the GitHub CLI lists it
type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Head    string `json:"headRefName"`
	Base    string `json:"baseRefName"`
	IsDraft bool   `json:"isDraft"`
}

// Branch is a branch that can be switched to: a local branch, or the head
// branch of an open pull request
type Branch struct {
	Name  string
	Local bool         // there is a local branch of the name
	PR    *PullRequest // the open pull request of the branch, if any
	// BaseDepth is how far up the chain of pull request bases from the
	// current branch the branch is: 1 for the base of the current branch's
	// pull request, 2 for the base of that one's, and so on; 0 off the chain.
	// The chain stops at the default branch.
	BaseDepth int
}

// Branches lists the branches to switch to, leaving out the current one,
// which it returns as well. The bases of the current branch's pull request
// come first, followed by the default branch, the local branches by their
// last commit, and the branches of the other open pull requests.
//
// Without the GitHub CLI, or outside GitHub, only the local branches are
// listed.
func Branches(repoRoot string) (branches []Branch, current string, err error) {
	locals, current, err := localBranches(repoRoot)
	if err != nil {
		return nil, "", err
	}
	return orderBranches(locals, current, defaultBranch(repoRoot), openPullRequests(repoRoot)), current, nil
}

// orderBranches merges the local branches, newest first, with the head
// branches of the open pull requests, also newest first, in the order
// Branches describes
func orderBranches(locals []string, current, defaultName string, prs []PullRequest) []Branch {
	prByHead := map[string]*PullRequest{}
	for i := range prs {
		// Pull requests from forks can share a head name; the newest wins
		if _, ok := prByHead[prs[i].Head]; !ok {
			prByHead[prs[i].Head] = &prs[i]
		}
	}
	isLocal := map[string]bool{}
	for _, name := range locals {
		isLocal[name] = true
	}

	var ordered []Branch
	listed := map[string]bool{current: true}
	add := func(name string, depth int) {
		if name == "" || listed[name] {
			return
		}
		listed[name] = true
		ordered = append(ordered, Branch{Name: name, Local: isLocal[name], PR: prByHead[name], BaseDepth: depth})
	}

	depth := 0
	for pr := prByHead[current]; pr != nil && pr.Base != defaultName && !listed[pr.Base]; pr = prByHead[pr.Base] {
		depth++
		add(pr.Base, depth)
	}
	if isLocal[defaultName] || prByHead[defaultName] != nil {
		add(defaultName, 0)
	}
	for _, name := range locals {
		add(name, 0)
	}
	for _, pr := range prs {
		add(pr.Head, 0)
	}
	return ordered
}

// CurrentBranch returns the name of the branch checked out, or on a
// detached HEAD its abbreviated commit, or "" outside a repository
func CurrentBranch(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD")
	cmd.Dir = repoRoot
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = repoRoot
	if output, err := cmd.Output(); err == nil {
		return "detached at " + strings.TrimSpace(string(output))
	}
	return ""
}

// localBranches returns the local branches, the one with the latest commit
// first, and the current branch ("" on a detached HEAD)
func localBranches(repoRoot string) ([]string, string, error) {
	cmd := exec.Command("git", "for-each-ref", "--sort=-committerdate", "--format=%(HEAD)%(refname:short)", "refs/heads")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("failed to list the branches: %w", err)
	}
	var names []string
	current := ""
	for _, line := range strings.Split(strings.TrimRight(string(output), "\n"), "\n") {
		if len(line) < 2 {
			continue
		}
		name := line[1:]
		if line[0] == '*' {
			current = name
		}
		names = append(names, name)
	}
	return names, current, nil
}

// defaultBranch returns the name of origin's default branch, or "" when it
// is not known
func defaultBranch(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "origin/")
}

// openPullRequests returns the open pull requests of the repository, the
// newest first, or nothing when the GitHub CLI is missing or fails
func openPullRequests(repoRoot string) []PullRequest {
	cmd := exec.Command("gh", "pr", "list", "--state", "open", "--limit", "200",
		"--json", "number,title,headRefName,baseRefName,isDraft")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var prs []PullRequest
	if json.Unmarshal(output, &prs) != nil {
		return nil
	}
	return prs
}

// SwitchBranch switches to b. The branch of a pull request is checked out
// with the GitHub CLI, which fetches it and brings the local branch up to
// date, so what is shown is what the pull request has; any other branch is
// switched to with git switch. On failure the error carries what the command
// printed.
func SwitchBranch(repoRoot string, b Branch) error {
	var cmd *exec.Cmd
	if b.PR != nil {
		cmd = exec.Command("gh", "pr", "checkout", fmt.Sprint(b.PR.Number))
	} else {
		cmd = exec.Command("git", "switch", b.Name)
	}
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		if msg := failureReason(string(output)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

// failureReason picks the line saying why git or gh failed: the first
// "error:" or "fatal:" line, or else the last line that is not blank
func failureReason(output string) string {
	reason := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "fatal:") {
			return line
		}
		if line != "" {
			reason = line
		}
	}
	return reason
}
