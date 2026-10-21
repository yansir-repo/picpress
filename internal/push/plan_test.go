package push

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

func TestParseConfig(t *testing.T) {
	ok := `{"version":1,"label":"MZ-007 阳光先生","credentialsUrl":"https://x.example/api","token":"pp_live_x",
		"limits":{"maxFiles":99999,"maxFileBytes":1024}}`
	cfg, err := ParseConfig([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limits.MaxFiles != HardMaxFiles || cfg.Limits.MaxFileBytes != 1024 || cfg.Limits.MaxTotalBytes != HardMaxTotalBytes {
		t.Fatalf("limits not clamped: %+v", cfg.Limits)
	}
	if cfg.Video.Frames != DefaultVideoFrames {
		t.Fatalf("frames default: %d", cfg.Video.Frames)
	}

	cases := map[string]string{
		`not json`:      "格式不对",
		`{"label":"a"}`: "缺少版本号",
		`{"version":2,"label":"a","credentialsUrl":"https://x","token":"t"}`:           "请更新 PicPress",
		`{"version":1,"credentialsUrl":"https://x","token":"t"}`:                       "缺少名称",
		`{"version":1,"label":"a","credentialsUrl":"http://evil.example","token":"t"}`: "HTTPS",
	}
	for in, want := range cases {
		_, err := ParseConfig([]byte(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", in, err, want)
		}
	}
	if _, err := CheckURL("http://127.0.0.1:9000"); err != nil {
		t.Errorf("loopback http must be allowed: %v", err)
	}
}

func TestNormalizeSegment(t *testing.T) {
	nfd := norm.NFD.String("证书 é")
	cases := map[string]string{
		nfd:          "证书 é",
		"a.txt. . ":  "a.txt",
		"CON.txt":    "CON_.txt",
		"nul":        "nul_",
		"x\x01y.jpg": "xy.jpg",
		"...":        "_",
	}
	for in, want := range cases {
		if got := normalizeSegment(in); got != want {
			t.Errorf("normalizeSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFitKeyLimits(t *testing.T) {
	long := strings.Repeat("长", 120) + ".pdf" // 364 bytes
	key, ok := fitKey([]string{"root", long}, KindDocument)
	if !ok {
		t.Fatal("must fit")
	}
	last := key[strings.LastIndex(key, "/")+1:]
	if len(last) > maxSegmentBytes || !strings.HasSuffix(last, ".pdf") || !strings.Contains(last, "~") {
		t.Fatalf("segment not shortened correctly: %q (%d)", last, len(last))
	}
	var deep []string
	for range 10 {
		deep = append(deep, strings.Repeat("d", 200))
	}
	deep = append(deep, "a.jpg")
	key, ok = fitKey(deep, KindImage)
	_, keyRes := reserve(KindImage, ".jpg")
	if !ok || MaxPrefixBytes+len(key)+keyRes+collisionReserve > maxKeyBytes {
		t.Fatalf("key too long: %d", len(key))
	}
	again, _ := fitKey(deep, KindImage)
	if again != key {
		t.Fatal("fitKey must be deterministic")
	}
}

func write(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

var allKinds = map[Kind]bool{KindImage: true, KindDocument: true, KindVideo: true}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "证书扫描")
	write(t, filepath.Join(root, "a", "b.jpg"), []byte("x"))
	write(t, filepath.Join(root, "a", "b.png"), []byte("x")) // same stem as b.jpg
	write(t, filepath.Join(root, "Doc.pdf"), []byte("x"))
	write(t, filepath.Join(root, ".DS_Store"), []byte("x"))
	write(t, filepath.Join(root, "Thumbs.db"), []byte("x"))
	write(t, filepath.Join(root, ".hidden", "c.jpg"), []byte("x"))
	write(t, filepath.Join(root, "封面.psd"), []byte("x"))
	write(t, filepath.Join(root, "zero.pdf"), nil)
	write(t, filepath.Join(root, "big.pdf"), make([]byte, 2048))
	write(t, filepath.Join(root, "v.mp4"), []byte("x"))
	write(t, filepath.Join(root, "v.mp4.frames", "frame-01.jpg"), []byte("x")) // real folder
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Doc.pdf"), filepath.Join(root, "link.pdf")); err != nil {
		t.Fatal(err)
	}
	single := filepath.Join(dir, "单个.txt")
	write(t, single, []byte("x"))

	plan := Scan([]string{root, single, root}, PlanOptions{Limits: Limits{MaxFileBytes: 1024}, Supported: allKinds})

	keys := map[string]Kind{}
	for _, e := range plan.Entries {
		keys[e.Key] = e.Kind
	}
	for _, want := range []string{
		"证书扫描/a/b.jpg", "证书扫描/a/b (2).png", "证书扫描/Doc.pdf",
		"证书扫描/v.mp4.frames/frame-01.jpg", "证书扫描/v (2).mp4", "单个.txt",
	} {
		if _, ok := keys[want]; !ok {
			t.Errorf("missing key %q in %v", want, keys)
		}
	}
	reasons := map[string]Reason{}
	for _, s := range plan.Skipped {
		reasons[s.Path] = s.Reason
	}
	for path, want := range map[string]Reason{
		"证书扫描/.DS_Store": ReasonHidden, "证书扫描/Thumbs.db": ReasonHidden, "证书扫描/.hidden": ReasonHidden,
		"证书扫描/封面.psd": ReasonUnsupported, "证书扫描/zero.pdf": ReasonEmpty, "证书扫描/big.pdf": ReasonTooLarge,
		"证书扫描/empty": ReasonEmpty, "证书扫描/link.pdf": ReasonSymlink,
		"证书扫描/v.mp4": ReasonRenamed,
	} {
		if reasons[path] != want {
			t.Errorf("%s: reason %q, want %q", path, reasons[path], want)
		}
	}
	if len(plan.Roots) != 2 {
		t.Errorf("duplicate root not dropped: %v", plan.Roots)
	}
}

// The test file system may be case- and normalisation-insensitive (APFS),
// so name clashes that need two files in one directory are tested here.
func TestKeyspaceFoldsCaseAndNormalisation(t *testing.T) {
	reqs := []keyRequest{
		{Kind: KindImage, Key: "r/x.mov.frames/frame-01.jpg"}, // real folder: wins over the video's frames
		{Kind: KindVideo, Key: "r/x.mov"},
		{Kind: KindDocument, Key: "r/Doc.pdf"},
		{Kind: KindDocument, Key: "r/doc.PDF"},
		{Kind: KindDocument, Key: "r/" + norm.NFD.String("é.pdf")},
		{Kind: KindDocument, Key: "r/É.pdf"},
		{Kind: KindImage, Key: "r/a.jpg"},
		{Kind: KindImage, Key: "r/A.webp"},
		{Kind: KindDocument, Key: "r/a.pdf"}, // documents never share image extensions
	}
	for i := range reqs {
		reqs[i].Sort = itoa(i)
	}
	got := assignKeys(reqs)
	want := []string{"r/x.mov.frames/frame-01.jpg", "r/x (2).mov", "r/Doc.pdf", "r/doc (2).PDF",
		"r/" + norm.NFD.String("é.pdf"), "r/É (2).pdf", "r/a.jpg", "r/A (2).webp", "r/a.pdf"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("place #%d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScanNFDAndRootClash(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "x", "案例")
	b := filepath.Join(dir, "y", "案例")
	write(t, filepath.Join(a, norm.NFD.String("é.pdf")), []byte("1"))
	write(t, filepath.Join(b, "é.pdf"), []byte("2"))
	plan := Scan([]string{a, b}, PlanOptions{Supported: allKinds})
	got := map[string]bool{}
	for _, e := range plan.Entries {
		got[e.Key] = true
	}
	if !got["案例/é.pdf"] || !got["案例 (2)/é.pdf"] {
		t.Fatalf("keys: %v", got)
	}
}

func TestScanLimitsAndConfirm(t *testing.T) {
	dir := t.TempDir()
	for i := range ConfirmFiles + 1 {
		write(t, filepath.Join(dir, "r", strings.Repeat("a", 3)+itoa(i)+".txt"), []byte("x"))
	}
	plan := Scan([]string{filepath.Join(dir, "r")}, PlanOptions{Supported: allKinds})
	if len(plan.ConfirmReasons) != 1 || len(plan.Blocked) != 0 {
		t.Fatalf("confirm %v blocked %v", plan.ConfirmReasons, plan.Blocked)
	}
	plan = Scan([]string{filepath.Join(dir, "r")}, PlanOptions{Supported: allKinds, Limits: Limits{MaxFiles: 10}})
	if len(plan.Blocked) != 1 {
		t.Fatalf("blocked %v", plan.Blocked)
	}
}

func TestVideoUnsupportedWithoutProducer(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "r", "v.mov"), []byte("x"))
	plan := Scan([]string{filepath.Join(dir, "r")}, PlanOptions{Supported: map[Kind]bool{KindImage: true}})
	if len(plan.Entries) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != ReasonUnsupported {
		t.Fatalf("%+v", plan)
	}
}

// AWS SigV4 documentation example: GET /test.txt with a Range header.
func TestSignV4Vector(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	c := Credentials{Region: "us-east-1", AccessKeyID: "AKIAIOSFODNN7EXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	signV4(req, c, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, want) {
		t.Fatalf("got %s", got)
	}
}

func TestJournalReplayIgnoresTornLine(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Put(Record{Key: "a", Done: true}); err != nil {
		t.Fatal(err)
	}
	if err := j.PutBatch(BatchState{ID: "b1", Seq: 2, Status: BatchPaused}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	f, _ := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"t":"rec","rec":{"key":"b","do`)
	f.Close()

	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if r, ok := j.Get("a"); !ok || !r.Done {
		t.Fatal("record a lost")
	}
	if _, ok := j.Get("b"); ok {
		t.Fatal("torn record must be ignored")
	}
	if b, ok := j.Batch(); !ok || b.Seq != 2 || !b.Status.Open() {
		t.Fatalf("batch: %+v", b)
	}
}

func TestSummaryText(t *testing.T) {
	plan := Plan{Skipped: []Skip{
		{Path: "产品/封面.psd", Reason: ReasonUnsupported},
		{Path: "产品/b.psd", Reason: ReasonUnsupported},
		{Path: "a (2).pdf", Reason: ReasonRenamed},
	}}
	var files []FileUpdate
	for i := range 238 {
		kind := KindImage
		if i < 3 {
			kind = KindVideo
		}
		files = append(files, FileUpdate{Source: itoa(i), Kind: kind, Status: StatusUploaded})
	}
	for i := range 7 {
		files = append(files, FileUpdate{Source: "d" + itoa(i), Kind: KindImage, Status: StatusDuplicate})
	}
	s := Summarize(plan, files)
	want := "已送达 238 个（共 247 个）；重复跳过 7；跳过 2（.psd 暂不支持）；失败 0。视频 3 个只传了抽帧。"
	if s.Text != want {
		t.Fatalf("\n got %s\nwant %s", s.Text, want)
	}
}
