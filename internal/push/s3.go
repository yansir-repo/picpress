package push

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Credentials are short-lived, write-only S3 credentials scoped to Prefix.
// They live only in memory.
type Credentials struct {
	Endpoint        string    `json:"endpoint"`
	Region          string    `json:"region"`
	Bucket          string    `json:"bucket"`
	Prefix          string    `json:"prefix"`
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	ExpiresAt       time.Time `json:"expiresAt"` // zero: does not expire
}

// Target identifies where objects land; it is what the journal is keyed by.
func (c Credentials) Target() string {
	return strings.TrimRight(c.Endpoint, "/") + "/" + c.Bucket + "/" + c.Prefix
}

func (c Credentials) validate() error {
	if _, err := CheckURL(c.Endpoint); err != nil {
		return err
	}
	if c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return errors.New("凭证不完整")
	}
	if c.Prefix != "" && !strings.HasSuffix(c.Prefix, "/") {
		return errors.New("前缀必须以 / 结尾")
	}
	if len(c.Prefix) > MaxPrefixBytes {
		return fmt.Errorf("前缀超过 %d 字节", MaxPrefixBytes)
	}
	return nil
}

// ObjectMeta is written with every object.
type ObjectMeta struct {
	ContentType string
	Meta        map[string]string // x-amz-meta-*
}

// S3Error is a non-2xx S3 response.
type S3Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter time.Duration
	ServerDate time.Time
}

func (e *S3Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("S3 %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("S3 %d", e.Status)
}

// S3 is a write-only S3 client. It deliberately has no GET, HEAD, LIST or
// DELETE-object operation: the credentials forbid them and nothing in the
// pipeline may depend on one. Abort is the single DELETE, on an upload id.
type S3 struct {
	HTTP *http.Client
	Now  func() time.Time
}

// NewS3 returns a client that never follows redirects.
func NewS3() *S3 {
	return &S3{HTTP: noRedirectClient(), Now: time.Now}
}

func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// objectURL builds the path-style URL; key is already the full key.
func objectURL(c Credentials, key string, query string) string {
	u := strings.TrimRight(c.Endpoint, "/") + "/" + uriEncode(c.Bucket, true) + "/" + uriEncode(key, false)
	if query != "" {
		u += "?" + query
	}
	return u
}

func (s *S3) do(ctx context.Context, c Credentials, method, key, query string, body []byte, hdr http.Header) (*http.Response, []byte, error) {
	if err := c.validate(); err != nil {
		return nil, nil, &FatalError{Reason: ReasonAuth, Message: "上传地址配置有误：" + err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, method, objectURL(c, key, query), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.ContentLength = int64(len(body))
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	signV4(req, c, hexSHA256(body), s.Now())
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode/100 != 2 {
		return resp, data, parseS3Error(resp, data)
	}
	// CompleteMultipartUpload can fail with a 200 and an <Error> body.
	if bytes.Contains(data, []byte("<Error>")) {
		return resp, data, parseS3Error(resp, data)
	}
	return resp, data, nil
}

func parseS3Error(resp *http.Response, body []byte) error {
	var x struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	_ = xml.Unmarshal(body, &x)
	e := &S3Error{Status: resp.StatusCode, Code: x.Code, Message: x.Message}
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = time.Duration(secs) * time.Second
	}
	if d, err := http.ParseTime(resp.Header.Get("Date")); err == nil {
		e.ServerDate = d
	}
	if e.Status == 200 {
		e.Status = 500 // treat an error body on 200 as a server error
	}
	return e
}

func objectHeaders(m ObjectMeta) http.Header {
	h := http.Header{}
	if m.ContentType != "" {
		h.Set("Content-Type", m.ContentType)
	}
	for k, v := range m.Meta {
		h.Set("X-Amz-Meta-"+k, encodeMetaValue(v))
	}
	return h
}

// encodeMetaValue keeps header values ASCII: S3 metadata must be US-ASCII,
// and source paths are often Chinese. RFC 2047 "=?UTF-8?B?…?=" is what S3
// SDKs and R2 both understand.
func encodeMetaValue(v string) string {
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(v)) + "?="
		}
	}
	return v
}

// Put uploads a whole object in one request.
func (s *S3) Put(ctx context.Context, c Credentials, key string, body []byte, m ObjectMeta) error {
	h := objectHeaders(m)
	sum := sha256.Sum256(body)
	// x-amz-checksum-sha256 asks the server to verify and store the checksum.
	// The signed x-amz-content-sha256 already makes the server reject a body
	// that does not match, so integrity holds even where the stored checksum
	// is ignored.
	h.Set("X-Amz-Checksum-Sha256", base64.StdEncoding.EncodeToString(sum[:]))
	_, _, err := s.do(ctx, c, http.MethodPut, key, "", body, h)
	return err
}

// CreateMultipart starts a multipart upload and returns its id.
func (s *S3) CreateMultipart(ctx context.Context, c Credentials, key string, m ObjectMeta) (string, error) {
	_, data, err := s.do(ctx, c, http.MethodPost, key, "uploads=", nil, objectHeaders(m))
	if err != nil {
		return "", err
	}
	var x struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(data, &x); err != nil || x.UploadID == "" {
		return "", &S3Error{Status: 500, Code: "BadResponse", Message: "missing UploadId"}
	}
	return x.UploadID, nil
}

// UploadPart uploads one part and returns its ETag.
func (s *S3) UploadPart(ctx context.Context, c Credentials, key, uploadID string, n int, body []byte) (string, error) {
	q := "partNumber=" + strconv.Itoa(n) + "&uploadId=" + uriEncode(uploadID, true)
	resp, _, err := s.do(ctx, c, http.MethodPut, key, q, body, nil)
	if err != nil {
		return "", err
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", &S3Error{Status: 500, Code: "BadResponse", Message: "missing ETag"}
	}
	return etag, nil
}

// Part is a completed part.
type Part struct {
	N    int    `json:"n"`
	ETag string `json:"etag"`
}

// CompleteMultipart finishes an upload.
func (s *S3) CompleteMultipart(ctx context.Context, c Credentials, key, uploadID string, parts []Part) error {
	type xmlPart struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	body := struct {
		XMLName xml.Name  `xml:"CompleteMultipartUpload"`
		Parts   []xmlPart `xml:"Part"`
	}{}
	for _, p := range parts {
		body.Parts = append(body.Parts, xmlPart{PartNumber: p.N, ETag: p.ETag})
	}
	data, err := xml.Marshal(body)
	if err != nil {
		return err
	}
	h := http.Header{}
	h.Set("Content-Type", "application/xml")
	_, _, err = s.do(ctx, c, http.MethodPost, key, "uploadId="+uriEncode(uploadID, true), data, h)
	return err
}

// AbortMultipart cancels an upload. NoSuchUpload counts as success.
func (s *S3) AbortMultipart(ctx context.Context, c Credentials, key, uploadID string) error {
	_, _, err := s.do(ctx, c, http.MethodDelete, key, "uploadId="+uriEncode(uploadID, true), nil, nil)
	var se *S3Error
	if errors.As(err, &se) && se.Code == "NoSuchUpload" {
		return nil
	}
	return err
}
