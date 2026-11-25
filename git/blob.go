package git

import (
	"os/exec"
	"sync"
)

// blobCacheSize is how many blobs ReadBlob keeps before it starts over
const blobCacheSize = 64

type blobEntry struct {
	content string
	ok      bool
}

// The blobs read, by repository and hash. A blob never changes, so neither
// does what is kept of it, a blob missing from the repository included.
var blobCache = struct {
	sync.Mutex
	entries map[string]blobEntry
}{entries: map[string]blobEntry{}}

// ReadBlob returns the content of the blob hash names, an abbreviated one
// too, or false when the repository does not have it
func ReadBlob(repoRoot, hash string) (string, bool) {
	key := repoRoot + "\x00" + hash
	blobCache.Lock()
	entry, cached := blobCache.entries[key]
	blobCache.Unlock()
	if cached {
		return entry.content, entry.ok
	}

	cmd := exec.Command("git", "cat-file", "blob", hash)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	entry = blobEntry{content: string(output), ok: err == nil}

	blobCache.Lock()
	if len(blobCache.entries) >= blobCacheSize {
		blobCache.entries = map[string]blobEntry{}
	}
	blobCache.entries[key] = entry
	blobCache.Unlock()
	return entry.content, entry.ok
}
