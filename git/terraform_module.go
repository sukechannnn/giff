package git

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// terraformSource matches the source argument of a module block
var terraformSource = regexp.MustCompile(`\bsource\s*=\s*"([^"]*)"`)

// IsTerraformFile reports whether path is a Terraform file
func IsTerraformFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".tf" || ext == ".tofu"
}

// TerraformModuleSource returns the source on line, the argument of a module
// block, and the directory it points to relative to the repository root, read
// from the file at path. The directory is "" when the source is not a local
// path in the repository, such as one of the registry or of a Git URL.
func TerraformModuleSource(path, line string) (source, dir string, ok bool) {
	m := terraformSource.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	source = m[1]
	// Terraform takes only ./ and ../ for local paths
	if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
		return source, "", true
	}
	dir = filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(path), source)))
	if dir == ".." || strings.HasPrefix(dir, "../") {
		return source, "", true
	}
	return source, dir, true
}

// TerraformModuleFiles returns the Terraform files of the module in dir,
// relative to repoRoot, main.tf first and the others by name
func TerraformModuleFiles(repoRoot, dir string) ([]Location, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		return nil, err
	}
	var files []Location
	for _, e := range entries {
		if !e.IsDir() && IsTerraformFile(e.Name()) {
			files = append(files, Location{Path: filepath.ToSlash(filepath.Join(dir, e.Name())), Line: 1})
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		mi, mj := filepath.Base(files[i].Path) == "main.tf", filepath.Base(files[j].Path) == "main.tf"
		if mi != mj {
			return mi
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}
