package push

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zalando/go-keyring"
)

const keyringService = "PicPress"

// ConnectionKind distinguishes how credentials are obtained.
type ConnectionKind string

const (
	ConnConfig ConnectionKind = "config" // .picpress-config token (F1/F2)
	ConnCustom ConnectionKind = "custom" // hand-entered S3 endpoint (F3)
)

// Connection is the persisted, secret-free description of the active target.
// Secrets (token, or secret key + session token) live in the keychain under
// "conn-<ID>".
type Connection struct {
	ID             string         `json:"id"`
	Kind           ConnectionKind `json:"kind"`
	Label          string         `json:"label"`
	Host           string         `json:"host"`
	Notice         string         `json:"notice,omitempty"`
	CredentialsURL string         `json:"credentialsUrl,omitempty"`
	Limits         Limits         `json:"limits"`
	Video          VideoConfig    `json:"video"`
	// Custom S3 only (non-secret parts).
	Endpoint    string `json:"endpoint,omitempty"`
	Region      string `json:"region,omitempty"`
	Bucket      string `json:"bucket,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
	AccessKeyID string `json:"accessKeyId,omitempty"`
}

type secret struct {
	Token           string `json:"token,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	SessionToken    string `json:"sessionToken,omitempty"`
}

// CustomS3 is the F3 form.
type CustomS3 struct {
	Label           string `json:"label"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
}

// Store persists the connection, the device id and per-target journals under
// one application data directory.
type Store struct {
	Base string
}

// DefaultStore uses the OS application-config directory.
func DefaultStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(dir, "PicPress")
	return &Store{Base: base}, os.MkdirAll(base, 0o700)
}

func (s *Store) connPath() string { return filepath.Join(s.Base, "connection.json") }

// DeviceID returns the random per-installation id, creating it on first use.
func (s *Store) DeviceID() (string, error) {
	p := filepath.Join(s.Base, "device-id")
	if data, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id, nil
		}
	}
	prefix := map[string]string{"darwin": "mac", "windows": "win"}[runtime.GOOS]
	if prefix == "" {
		prefix = runtime.GOOS
	}
	id := prefix + "-" + randomHex(6)
	return id, os.WriteFile(p, []byte(id+"\n"), 0o600)
}

// Connection returns the active connection, if any.
func (s *Store) Connection() (*Connection, error) {
	data, err := os.ReadFile(s.connPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Connection
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveConfig replaces the active connection with an imported config.
func (s *Store) SaveConfig(cfg Config) (*Connection, error) {
	u, _ := url.Parse(cfg.CredentialsURL)
	c := Connection{
		ID: randomHex(8), Kind: ConnConfig, Label: cfg.Label, Host: u.Host, Notice: cfg.Notice,
		CredentialsURL: cfg.CredentialsURL, Limits: cfg.Limits, Video: cfg.Video,
	}
	return &c, s.replace(c, secret{Token: cfg.Token})
}

// SaveCustom replaces the active connection with a hand-entered endpoint.
func (s *Store) SaveCustom(in CustomS3) (*Connection, error) {
	if in.Region == "" {
		in.Region = "auto"
	}
	if in.Prefix != "" && !strings.HasSuffix(in.Prefix, "/") {
		in.Prefix += "/"
	}
	creds := Credentials{Endpoint: in.Endpoint, Region: in.Region, Bucket: in.Bucket, Prefix: in.Prefix,
		AccessKeyID: in.AccessKeyID, SecretAccessKey: in.SecretAccessKey}
	if err := creds.validate(); err != nil {
		return nil, &ConfigError{Message: "自定义 S3 地址配置有误：" + err.Error()}
	}
	u, _ := url.Parse(in.Endpoint)
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = u.Host + "/" + in.Bucket
	}
	c := Connection{
		ID: randomHex(8), Kind: ConnCustom, Label: label, Host: u.Host,
		Limits: Limits{}.Clamped(), Video: VideoConfig{Frames: DefaultVideoFrames},
		Endpoint: in.Endpoint, Region: in.Region, Bucket: in.Bucket, Prefix: in.Prefix, AccessKeyID: in.AccessKeyID,
	}
	return &c, s.replace(c, secret{SecretAccessKey: in.SecretAccessKey, SessionToken: in.SessionToken})
}

func (s *Store) replace(c Connection, sec secret) error {
	data, _ := json.Marshal(sec)
	if err := keyring.Set(keyringService, "conn-"+c.ID, string(data)); err != nil {
		return errors.New("无法写入系统钥匙串：" + err.Error())
	}
	old, _ := s.Connection()
	meta, _ := json.MarshalIndent(c, "", "  ")
	tmp := s.connPath() + ".tmp"
	if err := os.WriteFile(tmp, meta, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.connPath()); err != nil {
		return err
	}
	if old != nil && old.ID != c.ID {
		_ = keyring.Delete(keyringService, "conn-"+old.ID)
	}
	return nil
}

// Remove forgets the active connection and its keychain entry. Journals stay:
// re-importing the same target keeps dedup working.
func (s *Store) Remove() error {
	c, err := s.Connection()
	if err != nil || c == nil {
		return err
	}
	_ = keyring.Delete(keyringService, "conn-"+c.ID)
	return os.Remove(s.connPath())
}

// Source builds the credential source for a connection from the keychain.
func (s *Store) Source(c *Connection, dev DeviceInfo) (CredentialSource, error) {
	raw, err := keyring.Get(keyringService, "conn-"+c.ID)
	if err != nil {
		return nil, &FatalError{Reason: ReasonAuth, Message: "钥匙串里找不到这个连接的令牌，请重新导入配置。"}
	}
	var sec secret
	if err := json.Unmarshal([]byte(raw), &sec); err != nil {
		return nil, &FatalError{Reason: ReasonAuth, Message: "钥匙串里的令牌已损坏，请重新导入配置。"}
	}
	if c.Kind == ConnCustom {
		return StaticSource{Creds: Credentials{
			Endpoint: c.Endpoint, Region: c.Region, Bucket: c.Bucket, Prefix: c.Prefix,
			AccessKeyID: c.AccessKeyID, SecretAccessKey: sec.SecretAccessKey, SessionToken: sec.SessionToken,
		}}, nil
	}
	return TokenSource{URL: c.CredentialsURL, Token: sec.Token, Device: dev}, nil
}

// JournalDir is the connection's journal directory. It is derived from what
// identifies the destination without asking the server (config label + URL,
// or endpoint + bucket + prefix), so an open batch can be found at startup
// offline. The server-assigned target is part of every record's dedup
// identity, so a changed prefix re-uploads instead of wrongly skipping.
func (s *Store) JournalDir(c *Connection) string {
	id := c.Label + "\x00" + c.CredentialsURL
	if c.Kind == ConnCustom {
		id = strings.TrimRight(c.Endpoint, "/") + "/" + c.Bucket + "/" + c.Prefix
	}
	return TargetDir(s.Base, id)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
