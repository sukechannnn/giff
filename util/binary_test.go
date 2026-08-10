package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()

	binPath := filepath.Join(dir, "image.png")
	if err := os.WriteFile(binPath, []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(dir, "text.txt")
	if err := os.WriteFile(textPath, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	isBin, err := IsBinaryFile("image.png", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !isBin {
		t.Error("expected image.png to be detected as binary")
	}

	isBin, err = IsBinaryFile("text.txt", dir)
	if err != nil {
		t.Fatal(err)
	}
	if isBin {
		t.Error("expected text.txt to be detected as text")
	}

	if _, err := IsBinaryFile("missing.txt", dir); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestIsBinaryDiff(t *testing.T) {
	binaryDiff := `diff --git a/demo.mov b/demo.mov
new file mode 100644
index 0000000..abc1234
Binary files /dev/null and b/demo.mov differ
`
	if !IsBinaryDiff(binaryDiff) {
		t.Error("expected binary diff to be detected")
	}

	textDiff := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
-old line
+Binary files a and b differ
`
	if IsBinaryDiff(textDiff) {
		t.Error("text diff mentioning binary in a body line should not be detected")
	}
}
