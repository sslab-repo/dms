package mlexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
)

// Slugify turns a dataset name into a zip/filesystem-safe slug:
// lowercase, alphanumerics preserved, everything else collapsed to dashes.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true // avoid a leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "dataset"
	}
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

// safeEntryName strips anything that could escape the archive root or upset
// a filesystem: path separators, traversal segments, control characters.
func safeEntryName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"|?*`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "file"
	}
	return cleaned
}

// assignZipNames gives every source file a unique, safe name under raw/.
// Collisions after sanitization get a numeric suffix before the extension.
func assignZipNames(files []SourceFile) {
	used := map[string]bool{}
	for i := range files {
		base := safeEntryName(files[i].OriginalName)
		candidate := base
		for n := 2; used[candidate]; n++ {
			ext := path.Ext(base)
			candidate = fmt.Sprintf("%s.%d%s", strings.TrimSuffix(base, ext), n, ext)
		}
		used[candidate] = true
		files[i].zipName = candidate
	}
}

// hashReader copies r to w (which may be io.Discard) while computing sha256,
// returning the hex digest and bytes copied.
func hashCopy(w io.Writer, r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), r)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return hashCopy(io.Discard, f)
}

func marshalIndented(v any) ([]byte, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}
