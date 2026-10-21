package push

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Summary is the end-of-batch report (F11). Text can be pasted into a chat.
type Summary struct {
	Total     int    `json:"total"`
	Uploaded  int    `json:"uploaded"`
	Duplicate int    `json:"duplicate"`
	Skipped   int    `json:"skipped"`
	Failed    int    `json:"failed"`
	Videos    int    `json:"videos"`
	Text      string `json:"text"`
}

var reasonLabels = map[Reason]string{
	ReasonUnsupported: "暂不支持",
	ReasonTooLarge:    "超过大小上限",
	ReasonEmpty:       "空文件或空文件夹",
	ReasonHidden:      "隐藏或系统文件",
	ReasonSymlink:     "快捷方式",
	ReasonDecode:      "无法解码",
	ReasonRead:        "读不到",
	ReasonNetwork:     "网络",
	ReasonAuth:        "没有权限",
}

// Summarize derives the report from the plan and the latest state of every
// file, so it stays right across pause / resume / retry runs. Renames are not
// skips: the renamed file was uploaded.
func Summarize(plan Plan, files []FileUpdate) Summary {
	var skipped, failed []Skip
	for _, s := range plan.Skipped {
		if s.Reason != ReasonRenamed {
			skipped = append(skipped, s)
		}
	}
	s := Summary{Skipped: len(skipped)}
	for _, f := range files {
		switch f.Status {
		case StatusUploaded:
			s.Uploaded++
		case StatusDuplicate:
			s.Duplicate++
		case StatusFailed:
			failed = append(failed, Skip{Path: f.Source, Reason: f.Reason, Detail: f.Detail})
			continue
		default:
			continue
		}
		if f.Kind == KindVideo {
			s.Videos++
		}
	}
	s.Failed = len(failed)
	s.Total = s.Uploaded + s.Duplicate + s.Skipped + s.Failed

	var b strings.Builder
	fmt.Fprintf(&b, "已送达 %d 个（共 %d 个）；重复跳过 %d；跳过 %d", s.Uploaded, s.Total, s.Duplicate, s.Skipped)
	if d := describe(skipped); d != "" {
		b.WriteString("（" + d + "）")
	}
	fmt.Fprintf(&b, "；失败 %d", s.Failed)
	if d := describe(failed); d != "" {
		b.WriteString("（" + d + "）")
	}
	b.WriteString("。")
	if s.Videos > 0 {
		fmt.Fprintf(&b, "视频 %d 个只传了抽帧。", s.Videos)
	}
	s.Text = b.String()
	return s
}

// describe groups skips by reason, most common first; unsupported formats
// name their extensions, e.g. ".psd 暂不支持".
func describe(skips []Skip) string {
	if len(skips) == 0 {
		return ""
	}
	counts := map[Reason]int{}
	exts := map[Reason][]string{}
	seenExt := map[string]bool{}
	for _, s := range skips {
		counts[s.Reason]++
		if s.Reason == ReasonUnsupported {
			if e := strings.ToLower(path.Ext(s.Path)); e != "" && !seenExt[e] {
				seenExt[e] = true
				exts[s.Reason] = append(exts[s.Reason], e)
			}
		}
	}
	reasons := make([]Reason, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if counts[reasons[i]] != counts[reasons[j]] {
			return counts[reasons[i]] > counts[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		label := reasonLabels[r]
		if e := exts[r]; len(e) > 0 {
			if len(e) > 3 {
				e = append(e[:3:3], "…")
			}
			label = strings.Join(e, " ") + " " + label
		}
		if len(reasons) > 1 {
			label += fmt.Sprintf(" %d", counts[r])
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "，")
}
