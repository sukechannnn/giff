package git

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTerraformModuleSource(t *testing.T) {
	const path = "infra/envs/production/task.tf"
	tests := []struct {
		line    string
		source  string
		dir     string
		ok      bool
		comment string
	}{
		{`  source = "../../modules/scheduled-ecs-task"`, "../../modules/scheduled-ecs-task", "infra/modules/scheduled-ecs-task", true, "a relative path"},
		{`+  source = "./modules/app"`, "./modules/app", "infra/envs/production/modules/app", true, "a line of a diff"},
		{`  source  = "terraform-aws-modules/vpc/aws"`, "terraform-aws-modules/vpc/aws", "", true, "the registry"},
		{`  source = "../../../../../outside"`, "../../../../../outside", "", true, "out of the repository"},
		{`  name = "source"`, "", "", false, "no source argument"},
	}
	for _, tt := range tests {
		source, dir, ok := TerraformModuleSource(path, tt.line)
		if source != tt.source || dir != tt.dir || ok != tt.ok {
			t.Errorf("%s: TerraformModuleSource(%q) = %q, %q, %v, want %q, %q, %v",
				tt.comment, tt.line, source, dir, ok, tt.source, tt.dir, tt.ok)
		}
	}
}

func TestTerraformModuleFiles(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, "modules", "task")
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"variables.tf", "main.tf", "outputs.tf", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := TerraformModuleFiles(repo, "modules/task")
	if err != nil {
		t.Fatalf("TerraformModuleFiles() error = %v", err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	want := []string{"modules/task/main.tf", "modules/task/outputs.tf", "modules/task/variables.tf"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("TerraformModuleFiles() = %v, want %v", paths, want)
	}
}
