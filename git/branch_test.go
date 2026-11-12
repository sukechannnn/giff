package git

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestOrderBranches(t *testing.T) {
	prs := []PullRequest{
		{Number: 5, Title: "Other", Head: "other", Base: "develop"},
		{Number: 4, Title: "Top of the stack", Head: "feature-c", Base: "feature-b"},
		{Number: 3, Title: "Middle", Head: "feature-b", Base: "feature-a"},
		{Number: 2, Title: "Bottom", Head: "feature-a", Base: "develop"},
	}
	names := func(branches []Branch) []string {
		var out []string
		for _, b := range branches {
			out = append(out, b.Name)
		}
		return out
	}

	t.Run("follows the bases of the current branch up to the default branch", func(t *testing.T) {
		got := orderBranches([]string{"mine", "feature-c", "develop", "feature-a"}, "feature-c", "develop", prs)
		want := []string{"feature-b", "feature-a", "develop", "mine", "other"}
		if !reflect.DeepEqual(names(got), want) {
			t.Fatalf("orderBranches() = %v, want %v", names(got), want)
		}
		if got[0].BaseDepth != 1 || got[1].BaseDepth != 2 || got[2].BaseDepth != 0 {
			t.Errorf("BaseDepth = %d, %d, %d, want 1, 2, 0", got[0].BaseDepth, got[1].BaseDepth, got[2].BaseDepth)
		}
		if got[0].Local || !got[1].Local {
			t.Errorf("Local = %v, %v, want false, true", got[0].Local, got[1].Local)
		}
		if got[0].PR == nil || got[0].PR.Number != 3 || got[2].PR != nil {
			t.Errorf("PR of feature-b = %v, of develop = %v", got[0].PR, got[2].PR)
		}
	})

	t.Run("puts the default branch first off the chain", func(t *testing.T) {
		got := orderBranches([]string{"mine", "develop"}, "mine", "develop", prs)
		want := []string{"develop", "other", "feature-c", "feature-b", "feature-a"}
		if !reflect.DeepEqual(names(got), want) {
			t.Errorf("orderBranches() = %v, want %v", names(got), want)
		}
	})

	t.Run("stops at a base it has listed already", func(t *testing.T) {
		loop := []PullRequest{
			{Number: 1, Head: "x", Base: "y"},
			{Number: 2, Head: "y", Base: "x"},
		}
		got := orderBranches([]string{"x"}, "x", "main", loop)
		if want := []string{"y"}; !reflect.DeepEqual(names(got), want) {
			t.Errorf("orderBranches() = %v, want %v", names(got), want)
		}
	})

	t.Run("lists the local branches without pull requests", func(t *testing.T) {
		got := orderBranches([]string{"b", "main", "a"}, "a", "main", nil)
		if want := []string{"main", "b"}; !reflect.DeepEqual(names(got), want) {
			t.Errorf("orderBranches() = %v, want %v", names(got), want)
		}
	})
}

func TestFailureReason(t *testing.T) {
	output := "error: Your local changes to the following files would be overwritten by checkout:\n\ta.txt\nPlease commit your changes or stash them before you switch branches.\nAborting\n"
	if got, want := failureReason(output), "error: Your local changes to the following files would be overwritten by checkout:"; got != want {
		t.Errorf("failureReason() = %q, want %q", got, want)
	}
	if got, want := failureReason("something went wrong\n\n"), "something went wrong"; got != want {
		t.Errorf("failureReason() = %q, want %q", got, want)
	}
}

func TestLocalBranchesAndSwitch(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("commit", "--allow-empty", "-m", "first")
	run("branch", "topic")

	names, current, err := localBranches(repo)
	if err != nil {
		t.Fatalf("localBranches() error = %v", err)
	}
	if current != "main" || len(names) != 2 {
		t.Fatalf("localBranches() = %v, %q", names, current)
	}

	if err := SwitchBranch(repo, Branch{Name: "topic", Local: true}); err != nil {
		t.Fatalf("SwitchBranch() error = %v", err)
	}
	if _, current, _ := localBranches(repo); current != "topic" {
		t.Errorf("current branch = %q after switching, want topic", current)
	}
	if err := SwitchBranch(repo, Branch{Name: "no-such-branch"}); err == nil {
		t.Error("SwitchBranch() to a missing branch succeeded")
	}
}
