package push

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Reason is a fixed skip / failure reason code (docs/push-upload.md F9).
// Adding one requires changing the document first.
type Reason string

const (
	ReasonUnsupported Reason = "unsupported"
	ReasonTooLarge    Reason = "too-large"
	ReasonEmpty       Reason = "empty"
	ReasonHidden      Reason = "hidden"
	ReasonSymlink     Reason = "symlink"
	ReasonRenamed     Reason = "name-conflict-renamed"
	ReasonDecode      Reason = "decode-error"
	ReasonRead        Reason = "read-error"
	ReasonNetwork     Reason = "network"
	ReasonAuth        Reason = "auth"
)

// Entry is one source file that will be processed.
type Entry struct {
	Path   string `json:"path"`   // absolute path on disk
	Source string `json:"source"` // "<root>/<rel>" as the user sees it, slash separated
	Key    string `json:"key"`    // normalised key relative to the prefix, original extension
	Kind   Kind   `json:"kind"`
	Size   int64  `json:"size"`
}

// Skip is a file that will not be uploaded, or one that was renamed.
type Skip struct {
	Path      string `json:"path"`
	Reason    Reason `json:"reason"`
	Detail    string `json:"detail"`
	RenamedTo string `json:"renamedTo,omitempty"`
}

// Plan is the result of scanning dropped paths. Building it never touches
// the network.
type Plan struct {
	Roots          []string `json:"roots"`
	Entries        []Entry  `json:"entries"`
	Skipped        []Skip   `json:"skipped"` // includes renames (ReasonRenamed)
	Images         int      `json:"images"`
	Documents      int      `json:"documents"`
	Videos         int      `json:"videos"`
	TotalBytes     int64    `json:"totalBytes"`     // source bytes of uploadable files (videos excluded)
	Blocked        []string `json:"blocked"`        // non-empty: the batch must not start
	ConfirmReasons []string `json:"confirmReasons"` // non-empty: ask a second time
}

// PlanOptions parameterises Scan.
type PlanOptions struct {
	Limits    Limits
	Supported map[Kind]bool // kinds with a working producer
}

var ignoredNames = map[string]bool{"thumbs.db": true, "desktop.ini": true}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".") || ignoredNames[strings.ToLower(name)]
}

// Scan walks the dropped paths and produces a Plan.
func Scan(paths []string, opts PlanOptions) Plan {
	limits := opts.Limits.Clamped()
	plan := Plan{Roots: []string{}, Entries: []Entry{}, Skipped: []Skip{}, Blocked: []string{}, ConfirmReasons: []string{}}

	type found struct {
		abs  string
		segs []string // root name + relative segments, raw
		size int64
		kind Kind
	}
	var files []found

	skip := func(segs []string, r Reason, detail string) {
		plan.Skipped = append(plan.Skipped, Skip{Path: display(segs), Reason: r, Detail: detail})
	}

	var walk func(abs string, segs []string)
	walk = func(abs string, segs []string) {
		name := segs[len(segs)-1]
		info, err := os.Lstat(abs)
		if err != nil {
			skip(segs, ReasonRead, "读不到这个文件")
			return
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			skip(segs, ReasonSymlink, "快捷方式 / 符号链接不上传")
			return
		case isHidden(name):
			skip(segs, ReasonHidden, "隐藏文件或系统文件不上传")
			return
		case info.IsDir():
			children, err := os.ReadDir(abs)
			if err != nil {
				skip(segs, ReasonRead, "读不到这个文件夹")
				return
			}
			if len(children) == 0 {
				skip(segs, ReasonEmpty, "空文件夹")
				return
			}
			for _, c := range children {
				walk(filepath.Join(abs, c.Name()), append(append([]string{}, segs...), c.Name()))
			}
			return
		case !info.Mode().IsRegular():
			skip(segs, ReasonUnsupported, "不是普通文件")
			return
		case info.Size() == 0:
			skip(segs, ReasonEmpty, "0 字节的空文件")
			return
		}
		kind, ok := KindOf(name)
		if !ok {
			skip(segs, ReasonUnsupported, "格式暂不支持")
			return
		}
		if !opts.Supported[kind] {
			skip(segs, ReasonUnsupported, unsupportedDetail(kind))
			return
		}
		if kind != KindVideo && info.Size() > limits.MaxFileBytes {
			skip(segs, ReasonTooLarge, fmt.Sprintf("超过单个文件上限 %s", HumanBytes(limits.MaxFileBytes)))
			return
		}
		files = append(files, found{abs: abs, segs: segs, size: info.Size(), kind: kind})
	}

	for _, root := range uniqueRoots(paths) {
		plan.Roots = append(plan.Roots, root.name)
		walk(root.abs, []string{root.name})
	}

	var reqs []keyRequest
	var placed []found
	for _, f := range files {
		key, ok := fitKey(f.segs, f.kind)
		if !ok {
			skip(f.segs, ReasonUnsupported, "路径太长")
			continue
		}
		reqs = append(reqs, keyRequest{Kind: f.kind, Key: key, Sort: display(f.segs)})
		placed = append(placed, f)
	}
	for i, key := range assignKeys(reqs) {
		f := placed[i]
		src := display(f.segs)
		if key != norm.NFC.String(src) {
			plan.Skipped = append(plan.Skipped, Skip{Path: src, Reason: ReasonRenamed, Detail: "云端名称已调整", RenamedTo: key})
		}
		plan.Entries = append(plan.Entries, Entry{Path: f.abs, Source: src, Key: key, Kind: f.kind, Size: f.size})
		switch f.kind {
		case KindImage:
			plan.Images++
			plan.TotalBytes += f.size
		case KindDocument:
			plan.Documents++
			plan.TotalBytes += f.size
		case KindVideo:
			plan.Videos++
		}
	}

	if n := len(plan.Entries); n > limits.MaxFiles {
		plan.Blocked = append(plan.Blocked, fmt.Sprintf("文件数 %d 超过上限 %d，请分几次拖入", n, limits.MaxFiles))
	}
	if plan.TotalBytes > limits.MaxTotalBytes {
		plan.Blocked = append(plan.Blocked, fmt.Sprintf("总大小 %s 超过上限 %s，请分几次拖入", HumanBytes(plan.TotalBytes), HumanBytes(limits.MaxTotalBytes)))
	}
	if n := len(plan.Entries); n > ConfirmFiles {
		plan.ConfirmReasons = append(plan.ConfirmReasons, fmt.Sprintf("这次有 %d 个文件（超过 %d 个）", n, ConfirmFiles))
	}
	if plan.TotalBytes > ConfirmTotalBytes {
		plan.ConfirmReasons = append(plan.ConfirmReasons, fmt.Sprintf("这次共 %s（超过 %s）", HumanBytes(plan.TotalBytes), HumanBytes(ConfirmTotalBytes)))
	}
	return plan
}

func unsupportedDetail(k Kind) string {
	if k == KindVideo {
		return "这个版本还不能处理视频（原片不会上传）"
	}
	return "格式暂不支持"
}

type root struct{ abs, name string }

// uniqueRoots drops repeated paths and gives same-named roots " (n)" names.
func uniqueRoots(paths []string) []root {
	seen := map[string]bool{}
	names := map[string]bool{}
	var out []root
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		base := filepath.Base(abs)
		name := base
		for n := 2; names[fold(normalizeSegment(name))]; n++ {
			name = withSuffix(base, n)
		}
		names[fold(normalizeSegment(name))] = true
		out = append(out, root{abs: abs, name: name})
	}
	return out
}

func display(segs []string) string { return strings.Join(segs, "/") }

// HumanBytes formats a byte count the way the UI does.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	suffixes := []string{"KB", "MB", "GB", "TB"}
	i := -1
	for value >= unit && i < len(suffixes)-1 {
		value /= unit
		i++
	}
	if value >= 100 || value == float64(int64(value)) {
		return fmt.Sprintf("%.0f %s", value, suffixes[i])
	}
	return fmt.Sprintf("%.1f %s", value, suffixes[i])
}
