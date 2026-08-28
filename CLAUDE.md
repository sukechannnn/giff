# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build
```bash
go build -o giff
```

### Run Tests
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests in a specific package
go test ./ui/commands/...
```

### Code Quality
```bash
# Format code
go fmt ./...

# Run static analysis
go vet ./...
```

### Running the Application
```bash
# Build and run
go build -o giff && ./giff

# Run with file watching (auto-refresh)
./giff --watch
```

## Architecture Overview

`giff` is a terminal UI tool for interactively staging Git hunks and lines, and committing changes, built with Go and the tview library. The architecture follows a clear separation of concerns:

### Core Components

1. **Main Entry Point** (`main.go`)
   - Handles CLI flags (`--watch`)
   - Initializes the tview application
   - Sets up signal handlers for graceful shutdown
   - Manages the application lifecycle

2. **Git Operations** (`git/` package)
   - `changed_file_list.go`: Retrieves modified, staged, and untracked files
   - `diff.go`: Generates file diffs for unstaged changes
   - `staged_diff.go`: Generates diffs for staged changes
   - `stage.go`: Applies patches to stage selected lines
   - `commit.go`: Handles git commit operations
   - `calculate_diff_header.go`: Processes diff headers for patch generation
   - `diff_grep.go`: Greps the changed lines of every pending diff (used by `/` in the file list)

3. **UI Layer** (`ui/` package)
   - `root_editor.go`: Main UI controller that manages the file list and diff views
   - `diff_view.go`: Handles diff display and line selection
   - `diff_view_updater.go`: Updates diff view based on user selections
   - `file_tree.go`: Renders the file list with status indicators
   - `split_view_content.go`: Generates side-by-side diff view
   - `unified_view_content.go`: Generates unified diff view
   - `commands/`: Contains command implementations (e.g., `a.go` for staging)

4. **Configuration** (`config/` package)
   - Manages application configuration
   - Handles patch file paths and other settings

5. **Utilities** (`util/` package)
   - `color.go`: Color scheme definitions for the UI
   - `sprit_lines.go`: Line splitting utilities

### Key Design Patterns

- **Event-Driven UI**: Uses tview's event system for keyboard navigation and actions
- **Stateful Selection**: Maintains selection state across view updates
- **Patch-Based Staging**: Generates minimal patches for selected lines using `git apply --cached`
- **Auto-Refresh Mode**: Optional file watching for real-time updates

### Important Implementation Details

- Keyboard shortcuts follow Vim-like conventions (j/k for navigation, V for visual selection)
- Split view and unified view modes are toggle-able
- The UI maintains cursor position when refreshing file lists
- Color scheme uses a custom palette defined in `util/color.go`
