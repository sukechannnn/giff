package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	dbg "runtime/debug"
	"strings"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/config"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/ui"
	"github.com/sukechannnn/giff/util"
)

var version = "dev"

// Application holds the overall application state and configuration
type Application struct {
	App    *tview.Application
	Config *config.AppConfig
}

func main() {
	var autoRefresh bool
	var showVersion bool
	flag.BoolVar(&autoRefresh, "watch", false, "Watch for file changes and auto-refresh")
	flag.BoolVar(&autoRefresh, "w", false, "Watch for file changes and auto-refresh (shorthand)")
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.Parse()

	if showVersion {
		if version == "dev" {
			if info, ok := dbg.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
				version = info.Main.Version
			}
		}
		fmt.Printf("%s\n", version)
		return
	}

	// Create shell snapshot for terminal command aliases
	util.CreateShellSnapshot()
	defer util.CleanupShellSnapshot()

	// Detect the Git repository root. Missing repo is only fatal if the user
	// did not also pass a file path to open directly — external paths can be
	// viewed without a surrounding git repo.
	repoPath, repoErr := git.FindGitRoot(".")
	if repoErr != nil && flag.NArg() == 0 {
		log.Fatalf("Git repository not found: %v", repoErr)
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	app := tview.NewApplication()

	// Create the application struct
	giffApp := &Application{
		App:    app,
		Config: cfg,
	}

	// SIGINT (Ctrl+c) signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Println("SIGINT received, cleaning up and exiting...")
		// Get PatchFilePath from config and remove it
		if _, err := os.Stat(giffApp.Config.PatchFilePath); err == nil {
			if err := os.Remove(giffApp.Config.PatchFilePath); err != nil {
				log.Printf("Failed to remove %s: %v", giffApp.Config.PatchFilePath, err)
			} else {
				log.Printf("Removed %s", giffApp.Config.PatchFilePath)
			}
		}
		util.CleanupShellSnapshot()
		giffApp.App.Stop()
		os.Exit(0)
	}()

	// Application-level key input capture (Ctrl+c workaround)
	giffApp.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			log.Println("Ctrl+c captured at application level, sending SIGINT...")
			gid, err := syscall.Getpgid(os.Getpid())
			if err == nil {
				syscall.Kill(-gid, syscall.SIGINT)
			}
			return nil // Stop propagating the event
		}
		return event // Process other events normally
	})

	// Get files with changes. Skip entirely if no repo was found — we only
	// reach this point when an external file argument was supplied.
	var stagedFiles, modifiedFiles, untrackedFiles []git.FileInfo
	if repoErr == nil {
		var err error
		stagedFiles, modifiedFiles, untrackedFiles, err = git.GetChangedFiles(repoPath)
		if err != nil {
			log.Fatalf("Failed to get modified files: %v", err)
		}
	}

	// Resolve an optional file argument. A path inside the repo with pending
	// changes opens in diff view; anything else (outside the repo, untouched
	// in-repo files, or any file when no repo is present) opens in file view.
	var initialFile, externalFile string
	if args := flag.Args(); len(args) > 0 {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "giff: %v\n", err)
			os.Exit(1)
		}
		if _, err := os.Stat(abs); err != nil {
			fmt.Fprintf(os.Stderr, "giff: %v\n", err)
			os.Exit(1)
		}

		if repoErr == nil {
			rel, relErr := filepath.Rel(repoPath, abs)
			if relErr == nil {
				rel = filepath.ToSlash(rel)
				if rel != ".." && !strings.HasPrefix(rel, "../") &&
					fileInChangedList(rel, stagedFiles, modifiedFiles, untrackedFiles) {
					initialFile = rel
				}
			}
		}
		if initialFile == "" {
			externalFile = abs
		}
	}

	// Without a real repo, fall back to the external file's parent so that
	// UI components expecting a non-empty repoRoot still get a valid value.
	if repoErr != nil {
		repoPath = filepath.Dir(externalFile)
	}

	// Define file selection handler (defined as a function for recursive use)
	var updateFileList func()

	updateFileList = func() {
		updatedStagedFiles, updatedModifiedFiles, updatedUntrackedFiles, err := git.GetChangedFiles(repoPath)
		if err != nil {
			log.Fatalf("Failed to get updated files: %v", err)
		}
		rootEditor := ui.RootEditor(
			giffApp.App,
			updatedStagedFiles,
			updatedModifiedFiles,
			updatedUntrackedFiles,
			repoPath,
			giffApp.Config.PatchFilePath,
			updateFileList,
			autoRefresh,
			"",
			"",
		)
		giffApp.App.SetRoot(rootEditor, true)
	}

	// Create the initial view (file list) and set it as root
	// The onSelect parameter is currently unused, so nil is passed
	initialView := ui.RootEditor(giffApp.App, stagedFiles, modifiedFiles, untrackedFiles, repoPath, giffApp.Config.PatchFilePath, updateFileList, autoRefresh, initialFile, externalFile)
	giffApp.App.SetRoot(initialView, true)

	// Run the application only once in main
	if err := giffApp.App.Run(); err != nil {
		log.Fatalf("Error running application: %v", err)
	}
}

func fileInChangedList(path string, lists ...[]git.FileInfo) bool {
	for _, list := range lists {
		for _, fi := range list {
			if fi.Path == path {
				return true
			}
		}
	}
	return false
}
