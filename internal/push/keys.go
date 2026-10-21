package push

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Kind is what the pipeline does with a source file.
type Kind string

const (
	KindImage    Kind = "image"    // compressed with the current settings
	KindDocument Kind = "document" // uploaded byte for byte
	KindVideo    Kind = "video"    // only frames, a contact sheet and meta.json
)

var kindByExt = func() map[string]Kind {
	m := map[string]Kind{}
	for _, e := range []string{"jpg", "jpeg", "png", "webp", "avif", "heic", "heif", "gif"} {
		m["."+e] = KindImage
	}
	for _, e := range []string{"pdf", "docx", "doc", "xlsx", "xls", "pptx", "ppt", "csv", "txt", "md", "json", "html"} {
		m["."+e] = KindDocument
	}
	for _, e := range []string{"mp4", "mov", "m4v", "avi", "mkv", "webm", "mts"} {
		m["."+e] = KindVideo
	}
	return m
}()

// KindOf classifies a file name by extension.
func KindOf(name string) (Kind, bool) {
	k, ok := kindByExt[strings.ToLower(path.Ext(name))]
	return k, ok
}

// Key length limits (UTF-8 bytes), see F10. Keys are fitted against the
// largest prefix we accept instead of the actual one, so a plan never depends
// on the server's answer and the same drop always yields the same keys.
const (
	maxKeyBytes     = 1024
	maxSegmentBytes = 255
	MaxPrefixBytes  = 128
)

// Bytes an artifact may add to an entry's key. Images can change extension
// (".jpg" -> ".webp"); videos turn "a.mov" into "a.mov.frames/frame-01.jpg".
const (
	imageExtReserve   = len(".avif")
	videoSegReserve   = len(framesSuffix)
	videoKeyReserve   = len(framesSuffix) + len("/frame-01.jpg")
	framesSuffix      = ".frames"
	shortHashLen      = 8
	collisionReserve  = len(" (999)")
	minShortenedBytes = 16
)

var folder = cases.Fold()

// fold is the identity under which two keys count as "the same name": NFC and
// full case folding, so the result is safe on case-insensitive file systems.
func fold(s string) string { return folder.String(norm.NFC.String(s)) }

var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// normalizeSegment applies F10 to one path segment: NFC, no control
// characters, no trailing spaces or dots, no Windows reserved device names.
func normalizeSegment(s string) string {
	s = norm.NFC.String(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(s, " .")
	if s == "" {
		return "_"
	}
	stem, ext := splitExt(s)
	if windowsReserved[strings.ToUpper(stem)] {
		s = stem + "_" + ext
	}
	return s
}

// splitExt splits "a.b.jpg" into ("a.b", ".jpg"). A leading dot is not an
// extension separator.
func splitExt(name string) (string, string) {
	ext := path.Ext(name)
	if ext == name {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// shorten cuts a segment to at most max bytes, keeping its extension and
// appending a short hash of the original so distinct names stay distinct.
func shorten(seg string, max int) string {
	if len(seg) <= max {
		return seg
	}
	sum := sha256.Sum256([]byte(seg))
	tag := "~" + hex.EncodeToString(sum[:])[:shortHashLen]
	stem, ext := splitExt(seg)
	if len(ext) > 16 {
		stem, ext = seg, ""
	}
	room := max - len(ext) - len(tag)
	if room < 1 {
		room = 1
	}
	return truncateUTF8(stem, room) + tag + ext
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// reserve returns how many bytes the kind's artifacts may add to the last
// segment and to the whole key.
func reserve(k Kind, ext string) (seg, key int) {
	switch k {
	case KindImage:
		if d := imageExtReserve - len(ext); d > 0 {
			return d, d
		}
		return 0, 0
	case KindVideo:
		return videoSegReserve, videoKeyReserve
	default:
		return 0, 0
	}
}

// fitKey normalises every segment and shortens segments until the key, with
// the prefix and the kind's reserve, fits the S3 limits. ok is false only when
// even maximally shortened segments cannot fit.
func fitKey(segs []string, k Kind) (string, bool) {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = normalizeSegment(s)
	}
	last := len(out) - 1
	_, ext := splitExt(out[last])
	segRes, keyRes := reserve(k, ext)
	segRes += collisionReserve
	keyRes += collisionReserve
	for i := range out {
		max := maxSegmentBytes
		if i == last {
			max -= segRes
		}
		out[i] = shorten(out[i], max)
	}
	budget := maxKeyBytes - MaxPrefixBytes - keyRes
	for total(out) > budget {
		// Shorten the longest segment; deterministic, and touches the
		// fewest names.
		longest := 0
		for i := range out {
			if len(out[i]) > len(out[longest]) {
				longest = i
			}
		}
		target := len(out[longest]) - (total(out) - budget)
		if target < minShortenedBytes {
			target = minShortenedBytes
		}
		if target >= len(out[longest]) {
			return "", false
		}
		out[longest] = shorten(out[longest], target)
	}
	return strings.Join(out, "/"), true
}

func total(segs []string) int {
	n := len(segs) - 1
	for _, s := range segs {
		n += len(s)
	}
	return n
}

// withSuffix inserts " (n)" before the extension of the last segment.
func withSuffix(key string, n int) string {
	dir, name := path.Split(key)
	stem, ext := splitExt(name)
	return dir + stem + " (" + itoa(n) + ")" + ext
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// claim is the region of the key space an entry's artifacts will occupy.
//   - document: exactly its key.
//   - image: "dir/stem" with any image extension, since the output format
//     is only known after compression.
//   - video: the whole subtree "dir/name.frames/".
type claim struct {
	id      string // folded identity
	subtree bool
}

func claimOf(k Kind, key string) claim {
	switch k {
	case KindImage:
		stem, _ := splitExt(key)
		return claim{id: fold(stem) + "\x00image"}
	case KindVideo:
		return claim{id: fold(key+framesSuffix) + "/", subtree: true}
	default:
		return claim{id: fold(key)}
	}
}

// keyspace resolves collisions deterministically. Real files (leaves) are
// placed before synthetic subtrees, so a user's own folder never gets
// renamed because of a video's frames directory.
type keyspace struct {
	leaves   map[string]bool
	dirs     map[string]bool // folded directory prefixes of placed leaves, with trailing "/"
	subtrees map[string]bool
}

func newKeyspace() *keyspace {
	return &keyspace{leaves: map[string]bool{}, dirs: map[string]bool{}, subtrees: map[string]bool{}}
}

func (ks *keyspace) free(c claim) bool {
	if c.subtree {
		return !ks.subtrees[c.id] && !ks.dirs[c.id] && !ks.underSubtree(c.id)
	}
	return !ks.leaves[c.id] && !ks.underSubtree(c.id)
}

func (ks *keyspace) underSubtree(id string) bool {
	for i := 0; i < len(id); i++ {
		if id[i] == '/' && ks.subtrees[id[:i+1]] {
			return true
		}
	}
	return false
}

func (ks *keyspace) take(c claim) {
	if c.subtree {
		ks.subtrees[c.id] = true
		return
	}
	ks.leaves[c.id] = true
	for i := 0; i < len(c.id); i++ {
		if c.id[i] == '/' {
			ks.dirs[c.id[:i+1]] = true
		}
	}
}

// keyRequest asks for a key for one source file.
type keyRequest struct {
	Kind Kind
	Key  string // fitted, normalised key
	Sort string // ordering key (the display path)
}

// assignKeys resolves all collisions at once and returns keys in request
// order. Real files (leaves) are placed before synthetic subtrees, sorted by
// path within each group. That order is what guarantees termination: while
// leaves are placed no subtree exists yet, so suffixing a leaf always frees
// it; a subtree is named after its video, so suffixing it moves it too.
func assignKeys(reqs []keyRequest) []string {
	idx := make([]int, len(reqs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ra, rb := reqs[idx[a]], reqs[idx[b]]
		sa, sb := claimOf(ra.Kind, ra.Key).subtree, claimOf(rb.Kind, rb.Key).subtree
		if sa != sb {
			return !sa
		}
		return ra.Sort < rb.Sort
	})
	ks := newKeyspace()
	out := make([]string, len(reqs))
	for _, i := range idx {
		r := reqs[i]
		candidate := r.Key
		for n := 2; !ks.free(claimOf(r.Kind, candidate)); n++ {
			candidate = withSuffix(r.Key, n)
		}
		ks.take(claimOf(r.Kind, candidate))
		out[i] = candidate
	}
	return out
}
