package git

import (
	"reflect"
	"testing"
)

func TestGrepFiles(t *testing.T) {
	repo := setupGrepRepo(t)

	t.Run("counts the matching lines of the tracked files, ignoring case", func(t *testing.T) {
		hits, err := GrepFiles(repo, "NEEDLE")
		if err != nil {
			t.Fatalf("GrepFiles() error = %v", err)
		}
		// fresh.txt is untracked, so the file browser does not list it.
		want := map[string]int{"staged.txt": 1, "unstaged.txt": 2}
		if !reflect.DeepEqual(hits, want) {
			t.Errorf("GrepFiles() = %v, want %v", hits, want)
		}
	})

	t.Run("matches unchanged lines too", func(t *testing.T) {
		hits, err := GrepFiles(repo, "untouched context")
		if err != nil {
			t.Fatalf("GrepFiles() error = %v", err)
		}
		want := map[string]int{"staged.txt": 1, "unstaged.txt": 1}
		if !reflect.DeepEqual(hits, want) {
			t.Errorf("GrepFiles() = %v, want %v", hits, want)
		}
	})

	t.Run("returns an empty map when nothing matches", func(t *testing.T) {
		hits, err := GrepFiles(repo, "no such text")
		if err != nil {
			t.Fatalf("GrepFiles() error = %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no hits, got %v", hits)
		}
	})

	t.Run("returns an empty map for an empty query", func(t *testing.T) {
		hits, err := GrepFiles(repo, "")
		if err != nil {
			t.Fatalf("GrepFiles() error = %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no hits for an empty query, got %v", hits)
		}
	})
}
