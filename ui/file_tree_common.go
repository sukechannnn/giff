package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// FileEntry represents a file entry in the file list with ID, path and status
type FileEntry struct {
	ID           string
	Path         string
	StageStatus  string // "staged", "unstaged", "untracked", "commit" (for git log view)
	ChangeStatus string // "added", "modified", "deleted", "untracked", "renamed", etc.
	IsDirectory  bool
}

// DirCollapseState manages the collapse state of directories in the file tree
type DirCollapseState struct {
	collapsed map[string]bool // key: "stageStatus:dirPath"
}

// NewDirCollapseState creates a new DirCollapseState
func NewDirCollapseState() *DirCollapseState {
	return &DirCollapseState{
		collapsed: make(map[string]bool),
	}
}

// IsCollapsed returns whether the directory is collapsed
func (s *DirCollapseState) IsCollapsed(stageStatus, dirPath string) bool {
	return s.collapsed[stageStatus+":"+dirPath]
}

// ToggleCollapsed toggles the collapse state of a directory
func (s *DirCollapseState) ToggleCollapsed(stageStatus, dirPath string) {
	key := stageStatus + ":" + dirPath
	s.collapsed[key] = !s.collapsed[key]
}

// SetCollapsed sets the collapse state of a directory
func (s *DirCollapseState) SetCollapsed(stageStatus, dirPath string, collapsed bool) {
	s.collapsed[stageStatus+":"+dirPath] = collapsed
}

// TreeNode represents a node in the file tree structure
type TreeNode struct {
	Name     string
	IsFile   bool
	Children map[string]*TreeNode
	FullPath string // Path of file or directory
}

// buildFileTreeFromGitFiles converts a list of git.FileInfo into a tree structure
func buildFileTreeFromGitFiles(files []git.FileInfo) *TreeNode {
	root := &TreeNode{
		Name:     "",
		IsFile:   false,
		Children: make(map[string]*TreeNode),
	}

	for _, fileInfo := range files {
		file := strings.TrimSpace(fileInfo.Path)
		if file == "" {
			continue
		}

		parts := strings.Split(file, "/")
		currentNode := root

		for i, part := range parts {
			isLastPart := i == len(parts)-1

			if _, exists := currentNode.Children[part]; !exists {
				newNode := &TreeNode{
					Name:     part,
					IsFile:   isLastPart,
					Children: make(map[string]*TreeNode),
				}
				if isLastPart {
					newNode.FullPath = file
				} else {
					newNode.FullPath = strings.Join(parts[:i+1], "/")
				}
				currentNode.Children[part] = newNode
			}

			currentNode = currentNode.Children[part]
		}
	}

	return root
}

// buildFileTreeFromFileEntries converts a list of FileEntry into a tree structure
func buildFileTreeFromFileEntries(files []FileEntry) *TreeNode {
	root := &TreeNode{
		Name:     "",
		IsFile:   false,
		Children: make(map[string]*TreeNode),
	}

	for _, fileInfo := range files {
		file := strings.TrimSpace(fileInfo.Path)
		if file == "" {
			continue
		}

		parts := strings.Split(file, "/")
		currentNode := root

		for i, part := range parts {
			isLastPart := i == len(parts)-1

			if _, exists := currentNode.Children[part]; !exists {
				newNode := &TreeNode{
					Name:     part,
					IsFile:   isLastPart,
					Children: make(map[string]*TreeNode),
				}
				if isLastPart {
					newNode.FullPath = file
				} else {
					newNode.FullPath = strings.Join(parts[:i+1], "/")
				}
				currentNode.Children[part] = newNode
			}

			currentNode = currentNode.Children[part]
		}
	}

	return root
}

// renderFileTreeForGitFiles renders the tree structure for git.FileInfo.
// stageStatusOverride (nullable) overrides the per-file StageStatus by path;
// directories always use the section-level stageStatus.
func renderFileTreeForGitFiles(
	node *TreeNode,
	depth int,
	prefix string,
	sb *strings.Builder,
	fileList *[]FileEntry,
	stageStatus string,
	regionIndex *int,
	currentSelection int,
	focusedPane bool,
	lineNumberMap map[int]int,
	currentLine *int,
	fileInfos []git.FileInfo,
	collapseState *DirCollapseState,
	statusMap map[string]string,
	stageStatusOverride map[string]string,
) {
	// Sort children for consistent ordering
	var sortedKeys []string
	for key := range node.Children {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	// Separate directories and files
	var directories []string
	var files []string

	for _, key := range sortedKeys {
		child := node.Children[key]
		if child.IsFile {
			files = append(files, key)
		} else {
			directories = append(directories, key)
		}
	}

	// Process all items (directories + files)
	allItems := append(directories, files...)

	for i, key := range allItems {
		isLast := i == len(allItems)-1
		child := node.Children[key]

		// Connector symbol for the current item
		connector := "├─"
		if isLast {
			connector = "└─"
		}

		// Prefix for the next level
		childPrefix := prefix + "│ "
		if isLast {
			childPrefix = prefix + "  "
		}

		if child.IsFile {
			// File node
			regionID := fmt.Sprintf("file-%d", *regionIndex)
			entryStage := stageStatus
			if stageStatusOverride != nil {
				if ov, ok := stageStatusOverride[child.FullPath]; ok {
					entryStage = ov
				}
			}
			*fileList = append(*fileList, FileEntry{
				ID:          regionID,
				Path:        child.FullPath,
				StageStatus: entryStage,
			})

			// Add status decoration to filename
			displayName := child.Name
			if status, ok := statusMap[child.FullPath]; ok {
				displayName = formatFileWithStatus(child.Name, status)
			}

			// Escape tview color tags
			escapedDisplayName := escapeTviewTags(displayName)

			if focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[white:blue]["file-%d"]%s%s[""][-:-]`+"\n", prefix, *regionIndex, connector, escapedDisplayName))
			} else if !focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[black:white]["file-%d"]%s%s[""][-:-]`+"\n", prefix, *regionIndex, connector, escapedDisplayName))
			} else {
				sb.WriteString(fmt.Sprintf(`%s[white:%s]["file-%d"]%s%s[""][-:-]`+"\n", prefix, util.NotSelectedFileLineColor, *regionIndex, connector, escapedDisplayName))
			}
			lineNumberMap[*regionIndex] = *currentLine
			(*regionIndex)++
			(*currentLine)++
		} else {
			// Directory node
			collapsed := collapseState != nil && collapseState.IsCollapsed(stageStatus, child.FullPath)
			escapedDirName := escapeTviewTags(child.Name)
			dirDisplay := escapedDirName + "/"

			regionID := fmt.Sprintf("file-%d", *regionIndex)
			*fileList = append(*fileList, FileEntry{
				ID:          regionID,
				Path:        child.FullPath,
				StageStatus: stageStatus,
				IsDirectory: true,
			})

			if focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[white:blue]["file-%d"]%s%s[""][-:-]`+"\n", prefix, *regionIndex, connector, dirDisplay))
			} else if !focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[black:white]["file-%d"]%s%s[""][-:-]`+"\n", prefix, *regionIndex, connector, dirDisplay))
			} else {
				sb.WriteString(fmt.Sprintf(`%s[white:%s]["file-%d"]%s%s[""][-:-]`+"\n", prefix, util.NotSelectedFileLineColor, *regionIndex, connector, dirDisplay))
			}
			lineNumberMap[*regionIndex] = *currentLine
			(*regionIndex)++
			(*currentLine)++

			// Only render children if not collapsed
			if !collapsed {
				renderFileTreeForGitFiles(child, depth+1, childPrefix, sb, fileList,
					stageStatus, regionIndex, currentSelection, focusedPane, lineNumberMap, currentLine, fileInfos, collapseState, statusMap, stageStatusOverride)
			}
		}
	}
}

// renderFileTreeForFileEntries renders the tree structure for FileEntry (used by git log view)
func renderFileTreeForFileEntries(
	node *TreeNode,
	depth int,
	prefix string,
	sb *strings.Builder,
	fileList *[]FileEntry,
	stageStatus string,
	regionIndex *int,
	currentSelection int,
	focusedPane bool,
	lineNumberMap map[int]int,
	currentLine *int,
	fileEntries []FileEntry,
	collapseState *DirCollapseState,
	statusMap map[string]string,
) {
	// Sort children for consistent ordering
	var sortedKeys []string
	for key := range node.Children {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	// Separate directories and files
	var directories []string
	var files []string

	for _, key := range sortedKeys {
		child := node.Children[key]
		if child.IsFile {
			files = append(files, key)
		} else {
			directories = append(directories, key)
		}
	}

	// Process all items (directories + files)
	allItems := append(directories, files...)

	for i, key := range allItems {
		isLast := i == len(allItems)-1
		child := node.Children[key]

		// Connector symbol for the current item
		connector := "├─"
		if isLast {
			connector = "└─"
		}

		// Prefix for the next level
		childPrefix := prefix + "│ "
		if isLast {
			childPrefix = prefix + "  "
		}

		if child.IsFile {
			// File node
			displayName := child.Name
			var changeStatus string

			if status, ok := statusMap[child.FullPath]; ok {
				displayName = formatFileWithStatus(child.Name, status)
				changeStatus = status
			}

			// Escape tview color tags
			escapedDisplayName := escapeTviewTags(displayName)

			regionID := fmt.Sprintf("file-%d", *regionIndex)
			if fileList != nil {
				*fileList = append(*fileList, FileEntry{
					ID:           regionID,
					Path:         child.FullPath,
					StageStatus:  stageStatus,
					ChangeStatus: changeStatus,
					IsDirectory:  false,
				})
			}

			if *regionIndex == currentSelection && focusedPane {
				sb.WriteString(fmt.Sprintf(`%s[white:blue]%s%s[""][-:-]`+"\n", prefix, connector, escapedDisplayName))
			} else if *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[black:white]%s%s[""][-:-]`+"\n", prefix, connector, escapedDisplayName))
			} else {
				sb.WriteString(fmt.Sprintf(`%s[white:%s]%s%s[""][-:-]`+"\n", prefix, util.NotSelectedFileLineColor, connector, escapedDisplayName))
			}
			lineNumberMap[*regionIndex] = *currentLine
			(*regionIndex)++
			(*currentLine)++
		} else {
			// Directory node
			collapsed := collapseState != nil && collapseState.IsCollapsed(stageStatus, child.FullPath)
			escapedDirName := escapeTviewTags(child.Name)
			dirDisplay := escapedDirName + "/"

			regionID := fmt.Sprintf("file-%d", *regionIndex)
			if fileList != nil {
				*fileList = append(*fileList, FileEntry{
					ID:          regionID,
					Path:        child.FullPath,
					StageStatus: stageStatus,
					IsDirectory: true,
				})
			}

			if focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[white:blue]%s%s[""][-:-]`+"\n", prefix, connector, dirDisplay))
			} else if !focusedPane && *regionIndex == currentSelection {
				sb.WriteString(fmt.Sprintf(`%s[black:white]%s%s[""][-:-]`+"\n", prefix, connector, dirDisplay))
			} else {
				sb.WriteString(fmt.Sprintf(`%s[white:%s]%s%s[""][-:-]`+"\n", prefix, util.NotSelectedFileLineColor, connector, dirDisplay))
			}
			lineNumberMap[*regionIndex] = *currentLine
			(*regionIndex)++
			(*currentLine)++

			// Only render children if not collapsed
			if !collapsed {
				renderFileTreeForFileEntries(child, depth+1, childPrefix, sb, fileList,
					stageStatus, regionIndex, currentSelection, focusedPane, lineNumberMap, currentLine, fileEntries, collapseState, statusMap)
			}
		}
	}
}

// collectPathsInTreeOrder collects file paths in the same order as tree rendering
// (directories first, then files, both sorted alphabetically at each level)
// Collapsed directories' children are skipped.
func collectPathsInTreeOrder(node *TreeNode, collapseState *DirCollapseState, stageStatus string) []string {
	var sortedKeys []string
	for key := range node.Children {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	var directories []string
	var files []string
	for _, key := range sortedKeys {
		child := node.Children[key]
		if child.IsFile {
			files = append(files, key)
		} else {
			directories = append(directories, key)
		}
	}

	var result []string
	allItems := append(directories, files...)
	for _, key := range allItems {
		child := node.Children[key]
		if child.IsFile {
			result = append(result, child.FullPath)
		} else {
			result = append(result, child.FullPath)
			// Only add children if not collapsed
			collapsed := collapseState != nil && collapseState.IsCollapsed(stageStatus, child.FullPath)
			if !collapsed {
				result = append(result, collectPathsInTreeOrder(child, collapseState, stageStatus)...)
			}
		}
	}
	return result
}

// formatFileWithStatus adds status decoration to filename
func formatFileWithStatus(filename string, status string) string {
	switch status {
	case "added", "untracked":
		return filename + " " + "(+)"
	case "deleted":
		return filename + " " + "(-)"
	case "modified":
		return filename + " " + "(•)"
	default:
		return filename
	}
}

// escapeTviewTags escapes tview color tag characters in text
func escapeTviewTags(text string) string {
	return tview.Escape(text)
}
