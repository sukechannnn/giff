package util

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/rivo/tview"
)

// Diff colors
const (
	AddedLineBg       = "#002500"
	AddedLineFg       = "#00AC37"
	DeletedLineBg     = "#3A0000"
	DeletedLineFg     = "#E7454E"
	InlineAddedBg     = "#1A4D1A"
	InlineDeletedBg   = "#5C1A1A"
	SearchHighlightBg = "#665500"
	ExpandedFoldBg    = "#3a3a3a"
)

// syntaxStyle is the chroma style used for syntax highlighting
var syntaxStyle = styles.Get("dracula")

// defaultTextColor is the fallback text color
const defaultTextColor = "#f8f8f2"

// tokenCache caches tokenized results using a hash key for fast comparison.
// A single render tokenizes several sources (old file, new file, diff body),
// so a few entries are kept. The map is reset when it grows past its cap.
const tokenCacheMaxEntries = 8

var tokenCache struct {
	sync.Mutex
	entries map[uint64][][]chroma.Token
}

// hashCacheKey computes a fast hash for the cache key
func hashCacheKey(filePath string, codeLines []string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(filePath))
	h.Write([]byte{0})
	for _, line := range codeLines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return h.Sum64()
}

// TokenizeCode tokenizes code lines using chroma.
// It caches the result for repeated calls with the same filePath and code.
func TokenizeCode(filePath string, codeLines []string) [][]chroma.Token {
	cacheKey := hashCacheKey(filePath, codeLines)

	tokenCache.Lock()
	if result, ok := tokenCache.entries[cacheKey]; ok {
		tokenCache.Unlock()
		return result
	}
	tokenCache.Unlock()

	codeText := strings.Join(codeLines, "\n")

	lexer := lexers.Match(filePath)
	if lexer == nil {
		lexer = lexers.Analyse(codeText)
	}
	if lexer == nil {
		return nil
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, codeText)
	if err != nil {
		return nil
	}

	// Build per-line token slices
	result := make([][]chroma.Token, len(codeLines))
	lineIdx := 0
	for _, tok := range iterator.Tokens() {
		if lineIdx >= len(codeLines) {
			break
		}
		// Split tokens that span multiple lines
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				lineIdx++
				if lineIdx >= len(codeLines) {
					break
				}
			}
			if part != "" {
				result[lineIdx] = append(result[lineIdx], chroma.Token{
					Type:  tok.Type,
					Value: part,
				})
			}
		}
	}

	tokenCache.Lock()
	if tokenCache.entries == nil || len(tokenCache.entries) >= tokenCacheMaxEntries {
		tokenCache.entries = make(map[uint64][][]chroma.Token, tokenCacheMaxEntries)
	}
	tokenCache.entries[cacheKey] = result
	tokenCache.Unlock()

	return result
}

// resolveTokenColor returns the hex color for a token type using the chroma style.
func resolveTokenColor(tt chroma.TokenType) string {
	if syntaxStyle == nil {
		return defaultTextColor
	}
	entry := syntaxStyle.Get(tt)
	if entry.Colour.IsSet() {
		return fmt.Sprintf("#%06x", int(entry.Colour)&0xFFFFFF)
	}
	return defaultTextColor
}

// RenderHighlightedLine renders tokens into a tview color-tagged string with the given background.
func RenderHighlightedLine(tokens []chroma.Token, bgColor string) string {
	if len(tokens) == 0 {
		if bgColor != "" {
			return "[:#" + bgColor[1:] + "] [-:-]"
		}
		return ""
	}

	var sb strings.Builder
	for _, tok := range tokens {
		fg := resolveTokenColor(tok.Type)
		escaped := tview.Escape(tok.Value)
		if bgColor != "" {
			sb.WriteString("[" + fg + ":" + bgColor + "]" + escaped + "[-:-]")
		} else {
			sb.WriteString("[" + fg + "]" + escaped + "[-]")
		}
	}
	return sb.String()
}

// RenderHighlightedLineWithMask renders tokens with an inline diff mask.
// Characters where mask[charIdx] is true use maskBg instead of baseBg.
func RenderHighlightedLineWithMask(tokens []chroma.Token, baseBg string, mask []bool, maskBg string) string {
	if len(tokens) == 0 {
		if baseBg != "" {
			return "[:#" + baseBg[1:] + "] [-:-]"
		}
		return ""
	}

	var sb strings.Builder
	charIdx := 0
	for _, tok := range tokens {
		fg := resolveTokenColor(tok.Type)
		runes := []rune(tok.Value)

		segStart := 0
		for segStart < len(runes) {
			masked := charIdx+segStart < len(mask) && mask[charIdx+segStart]
			segEnd := segStart + 1
			for segEnd < len(runes) {
				nextMasked := charIdx+segEnd < len(mask) && mask[charIdx+segEnd]
				if nextMasked != masked {
					break
				}
				segEnd++
			}

			segText := tview.Escape(string(runes[segStart:segEnd]))
			bg := baseBg
			if masked && maskBg != "" {
				bg = maskBg
			}
			if bg != "" {
				sb.WriteString("[" + fg + ":" + bg + "]" + segText + "[-:-]")
			} else {
				sb.WriteString("[" + fg + "]" + segText + "[-]")
			}
			segStart = segEnd
		}
		charIdx += len(runes)
	}
	return sb.String()
}

// ReplaceBackgroundPreserving replaces background colors but preserves specified background colors.
func ReplaceBackgroundPreserving(line string, newBg string, preserve []string) string {
	preserveSet := make(map[string]bool, len(preserve))
	for _, p := range preserve {
		preserveSet[p] = true
	}
	return replaceTagBackground(line, newBg, preserveSet)
}

// replaceTagBackground rewrites the background part of every tview style tag
// in line to newBg, leaving region tags, escaped tags, and literal brackets in
// code untouched. Tags whose current background is in preserve are kept as-is.
func replaceTagBackground(line string, newBg string, preserve map[string]bool) string {
	var sb strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '[' {
			rest := line[i:]
			if m := MatchTviewEscapedTag(rest); m != "" {
				sb.WriteString(m)
				i += len(m)
				continue
			}
			if m := MatchTviewRegionTag(rest); m != "" {
				sb.WriteString(m)
				i += len(m)
				continue
			}
			if m := MatchTviewStyleTag(rest); m != "" {
				// Split "fg:bg:attrs:url" keeping attrs/url in the tail
				parts := strings.SplitN(m[1:len(m)-1], ":", 3)
				bg := ""
				if len(parts) >= 2 {
					bg = parts[1]
				}
				if preserve[bg] {
					sb.WriteString(m)
				} else {
					tail := ""
					if len(parts) == 3 {
						tail = ":" + parts[2]
					}
					sb.WriteString("[" + parts[0] + ":" + newBg + tail + "]")
				}
				i += len(m)
				continue
			}
		}
		sb.WriteByte(line[i])
		i++
	}
	return sb.String()
}

// ReplaceBackground replaces background color portions in a tview color-tagged string.
// It scans for [fg:bg] patterns and replaces bg with newBg.
// For tags like [fg] (no bg), it inserts the background.
func ReplaceBackground(line string, newBg string) string {
	return replaceTagBackground(line, newBg, nil)
}
