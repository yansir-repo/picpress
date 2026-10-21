// Command push-devserver is a local stand-in for the push backend
// (docs/push-upload.md §6): a write-only S3 that refuses every read, plus the
// §5 credentials endpoint. Objects are written to -dir so you can look at
// what arrived. It also writes a .picpress-config to import into PicPress.
//
//	go run ./cmd/push-devserver -dir /tmp/picpress-inbox
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"picpress/internal/push"
)

const (
	bucket    = "inbox"
	prefix    = "DEV-001/inbox/"
	accessKey = "DEVKEY"
	secret    = "DEVSECRET"
	token     = "pp_dev_token"
)

type server struct {
	dir     string
	ttl     time.Duration
	mu      sync.Mutex
	uploads map[string]map[int][]byte
	nextID  int
	issued  int
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9900", "listen address (loopback, so plain HTTP is allowed)")
	dir := flag.String("dir", filepath.Join(os.TempDir(), "picpress-inbox"), "where received objects are written")
	ttl := flag.Duration("ttl", 15*time.Minute, "credential lifetime (try 30s to exercise refresh)")
	flag.Parse()

	s := &server{dir: *dir, ttl: *ttl, uploads: map[string]map[int][]byte{}}
	base := "http://" + *addr
	cfg := map[string]any{
		"version": 1, "label": "本地联调", "credentialsUrl": base + "/credentials", "token": token,
		"notice": "这是 push-devserver 生成的测试配置。",
	}
	cfgPath := filepath.Join(*dir, "dev.picpress-config")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/credentials", func(w http.ResponseWriter, r *http.Request) { s.credentials(w, r, base) })
	mux.HandleFunc("/", s.s3)
	log.Printf("objects  → %s", *dir)
	log.Printf("config   → %s (double-click or drop it on PicPress)", cfgPath)
	log.Printf("listening on %s", base)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func (s *server) credentials(w http.ResponseWriter, r *http.Request, base string) {
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	s.issued++
	n := s.issued
	s.mu.Unlock()
	var dev push.DeviceInfo
	_ = json.NewDecoder(r.Body).Decode(&dev)
	log.Printf("credentials #%d for %s (%s %s)", n, dev.DeviceID, dev.OS, dev.AppVersion)
	_ = json.NewEncoder(w).Encode(push.Credentials{
		Endpoint: base, Region: "auto", Bucket: bucket, Prefix: prefix,
		AccessKeyID: accessKey, SecretAccessKey: secret, SessionToken: "session-" + strconv.Itoa(n),
		ExpiresAt: time.Now().Add(s.ttl).UTC(),
	})
}

func fail(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code></Error>", code)
}

func (s *server) s3(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	q := r.URL.Query()
	key := strings.TrimPrefix(r.URL.Path, "/"+bucket+"/")
	var op string
	switch {
	case r.Method == http.MethodPut && q.Has("partNumber") && q.Has("uploadId"):
		op = "UploadPart"
	case r.Method == http.MethodPut && len(q) == 0:
		op = "PutObject"
	case r.Method == http.MethodPost && q.Has("uploads"):
		op = "CreateMultipartUpload"
	case r.Method == http.MethodPost && q.Has("uploadId"):
		op = "CompleteMultipartUpload"
	case r.Method == http.MethodDelete && q.Has("uploadId"):
		op = "AbortMultipartUpload"
	}
	if op == "" {
		log.Printf("VIOLATION: read-type request %s %s", r.Method, r.URL)
		fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	if !push.VerifySigV4(r, secret) {
		log.Printf("bad signature: %s %s", r.Method, r.URL)
		fail(w, http.StatusForbidden, "SignatureDoesNotMatch")
		return
	}
	if !strings.HasPrefix(key, prefix) {
		fail(w, http.StatusForbidden, "AccessDenied")
		return
	}
	log.Printf("%-24s %s (%d B)", op, key, len(body))

	s.mu.Lock()
	defer s.mu.Unlock()
	switch op {
	case "PutObject":
		s.write(key, body)
	case "CreateMultipartUpload":
		s.nextID++
		id := "dev-" + strconv.Itoa(s.nextID)
		s.uploads[id] = map[int][]byte{}
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", id)
	case "UploadPart":
		parts, ok := s.uploads[q.Get("uploadId")]
		if !ok {
			fail(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		n, _ := strconv.Atoi(q.Get("partNumber"))
		parts[n] = body
		w.Header().Set("ETag", fmt.Sprintf(`"part-%d"`, n))
	case "CompleteMultipartUpload":
		parts, ok := s.uploads[q.Get("uploadId")]
		if !ok {
			fail(w, http.StatusNotFound, "NoSuchUpload")
			return
		}
		var req struct {
			Parts []struct{ PartNumber int } `xml:"Part"`
		}
		_ = xml.Unmarshal(body, &req)
		nums := make([]int, 0, len(req.Parts))
		for _, p := range req.Parts {
			nums = append(nums, p.PartNumber)
		}
		sort.Ints(nums)
		var all []byte
		for _, n := range nums {
			all = append(all, parts[n]...)
		}
		s.write(key, all)
		delete(s.uploads, q.Get("uploadId"))
		fmt.Fprint(w, "<CompleteMultipartUploadResult/>")
	case "AbortMultipartUpload":
		delete(s.uploads, q.Get("uploadId"))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) write(key string, data []byte) {
	p := filepath.Join(s.dir, "objects", filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		log.Print(err)
		return
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		log.Print(err)
	}
}
