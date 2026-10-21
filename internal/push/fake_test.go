package push

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3 is a write-only S3: it implements exactly the five operations the
// credentials allow and records anything else as a violation (docs §6).
type fakeS3 struct {
	t      *testing.T
	srv    *httptest.Server
	bucket string

	mu         sync.Mutex
	objects    map[string][]byte
	meta       map[string]http.Header
	uploads    map[string]map[int][]byte
	uploadKey  map[string]string
	requests   []string // "METHOD key?query"
	violations []string
	validToken map[string]bool // accepted session tokens
	nextID     int

	// hook may answer a request instead of the fake; return 0 to pass.
	hook func(r *http.Request, op string) (status int, code string)
}

func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{
		t: t, bucket: "inbox",
		objects: map[string][]byte{}, meta: map[string]http.Header{},
		uploads: map[string]map[int][]byte{}, uploadKey: map[string]string{},
		validToken: map[string]bool{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) op(r *http.Request) string {
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodPut && q.Has("partNumber") && q.Has("uploadId"):
		return "UploadPart"
	case r.Method == http.MethodPut && len(q) == 0:
		return "PutObject"
	case r.Method == http.MethodPost && q.Has("uploads"):
		return "CreateMultipartUpload"
	case r.Method == http.MethodPost && q.Has("uploadId"):
		return "CompleteMultipartUpload"
	case r.Method == http.MethodDelete && q.Has("uploadId"):
		return "AbortMultipartUpload"
	}
	return ""
}

func (f *fakeS3) fail(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>%s</Message></Error>", code, code)
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	prefix := "/" + f.bucket + "/"
	key, _ := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), prefix))
	op := f.op(r)

	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+key+"?"+r.URL.RawQuery)
	if op == "" {
		f.violations = append(f.violations, r.Method+" "+r.URL.String())
		f.mu.Unlock()
		f.fail(w, 403, "AccessDenied")
		return
	}
	f.mu.Unlock()

	if !VerifySigV4(r, "SECRET") {
		f.mu.Lock()
		f.violations = append(f.violations, "bad signature: "+r.Method+" "+r.URL.String())
		f.mu.Unlock()
		f.fail(w, 403, "SignatureDoesNotMatch")
		return
	}
	if r.Header.Get("X-Amz-Content-Sha256") != hexSHA256(body) {
		f.fail(w, 400, "XAmzContentSHA256Mismatch")
		return
	}
	if f.hook != nil {
		if st, code := f.hook(r, op); st != 0 {
			if st < 0 { // simulate a dropped connection
				hj, _ := w.(http.Hijacker)
				c, _, _ := hj.Hijack()
				c.Close()
				return
			}
			f.fail(w, st, code)
			return
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.validToken) > 0 && !f.validToken[r.Header.Get("X-Amz-Security-Token")] {
		w.WriteHeader(403)
		fmt.Fprint(w, "<Error><Code>ExpiredToken</Code></Error>")
		return
	}

	q := r.URL.Query()
	switch op {
	case "PutObject":
		if strings.Contains(key, manifestDir) {
			f.checkManifestLocked(body)
		}
		f.objects[key] = body
		f.meta[key] = r.Header.Clone()
		w.Header().Set("ETag", `"`+hexSHA256(body)[:16]+`"`)
	case "CreateMultipartUpload":
		f.nextID++
		id := "up-" + strconv.Itoa(f.nextID)
		f.uploads[id] = map[int][]byte{}
		f.uploadKey[id] = key
		f.meta[key] = r.Header.Clone()
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", id)
	case "UploadPart":
		parts, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			f.fail(w, 404, "NoSuchUpload")
			return
		}
		n, _ := strconv.Atoi(q.Get("partNumber"))
		parts[n] = body
		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d-%s"`, n, hexSHA256(body)[:8]))
	case "CompleteMultipartUpload":
		parts, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			f.fail(w, 404, "NoSuchUpload")
			return
		}
		var req struct {
			Parts []struct {
				PartNumber int
				ETag       string
			} `xml:"Part"`
		}
		_ = xml.Unmarshal(body, &req)
		var all []byte
		for i, p := range req.Parts {
			if p.PartNumber != i+1 {
				f.fail(w, 400, "InvalidPartOrder")
				return
			}
			all = append(all, parts[p.PartNumber]...)
		}
		f.objects[key] = all
		delete(f.uploads, q.Get("uploadId"))
		fmt.Fprint(w, "<CompleteMultipartUploadResult></CompleteMultipartUploadResult>")
	case "AbortMultipartUpload":
		delete(f.uploads, q.Get("uploadId"))
		w.WriteHeader(204)
	}
}

// checkManifestLocked enforces F9 ordering: every file listed as uploaded
// must already exist when the manifest arrives.
func (f *fakeS3) checkManifestLocked(body []byte) {
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		f.violations = append(f.violations, "manifest is not JSON")
		return
	}
	for _, file := range m.Files {
		data, ok := f.objects[testPrefix+file.Key]
		if !ok {
			f.violations = append(f.violations, "manifest lists missing object "+file.Key)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != file.SHA256 {
			f.violations = append(f.violations, "manifest sha mismatch "+file.Key)
		}
	}
}

func (f *fakeS3) count(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.Contains(r, substr) {
			n++
		}
	}
	return n
}

func (f *fakeS3) userObjects() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for k := range f.objects {
		if !strings.Contains(k, manifestDir) {
			keys = append(keys, strings.TrimPrefix(k, testPrefix))
		}
	}
	sort.Strings(keys)
	return keys
}

func (f *fakeS3) manifests() []Manifest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for k := range f.objects {
		if strings.Contains(k, manifestDir) {
			keys = append(keys, k)
		}
	}
	var out []Manifest
	for _, k := range keys {
		var m Manifest
		_ = json.Unmarshal(f.objects[k], &m)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BatchID != out[j].BatchID {
			return out[i].BatchID < out[j].BatchID
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

func (f *fakeS3) assertNoReads(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.violations) > 0 {
		t.Fatalf("violations: %v", f.violations)
	}
}

const testPrefix = "MZ-007/inbox/"

func (f *fakeS3) creds(token string, ttl time.Duration) Credentials {
	c := Credentials{
		Endpoint: f.srv.URL, Region: "auto", Bucket: f.bucket, Prefix: testPrefix,
		AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: token,
	}
	if ttl > 0 {
		c.ExpiresAt = time.Now().Add(ttl)
	}
	return c
}

// fakeCreds is the §5 credentials endpoint.
type fakeCreds struct {
	srv     *httptest.Server
	mu      sync.Mutex
	calls   int
	status  int // non-zero: answer with this status
	onIssue func(n int) Credentials
}

func newFakeCreds(t *testing.T, issue func(n int) Credentials) *fakeCreds {
	fc := &fakeCreds{onIssue: issue}
	fc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fc.mu.Lock()
		fc.calls++
		n, st := fc.calls, fc.status
		fc.mu.Unlock()
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer pp_test" {
			w.WriteHeader(401)
			return
		}
		if st != 0 {
			w.WriteHeader(st)
			return
		}
		_ = json.NewEncoder(w).Encode(fc.onIssue(n))
	}))
	t.Cleanup(fc.srv.Close)
	return fc
}
