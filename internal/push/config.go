package push

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ConfigVersion is the highest .picpress-config version this build understands.
const ConfigVersion = 1

// ConfigExt is the file extension of push configuration files.
const ConfigExt = ".picpress-config"

// Built-in hard caps. A config can only tighten these, never loosen them.
const (
	HardMaxFileBytes  int64 = 200 << 20
	HardMaxFiles            = 5000
	HardMaxTotalBytes int64 = 20 << 30
)

// Second-confirmation thresholds shown in the prepare panel.
const (
	ConfirmFiles            = 500
	ConfirmTotalBytes int64 = 1 << 30
)

// DefaultVideoFrames is used when the config does not specify video.frames.
const DefaultVideoFrames = 12

// Limits caps what a single batch may contain.
type Limits struct {
	MaxFiles      int   `json:"maxFiles"`
	MaxTotalBytes int64 `json:"maxTotalBytes"`
	MaxFileBytes  int64 `json:"maxFileBytes"`
}

// Clamped fills defaults and never lets a value exceed the hard caps.
func (l Limits) Clamped() Limits {
	clamp := func(v, hard int64) int64 {
		if v <= 0 || v > hard {
			return hard
		}
		return v
	}
	return Limits{
		MaxFiles:      int(clamp(int64(l.MaxFiles), HardMaxFiles)),
		MaxTotalBytes: clamp(l.MaxTotalBytes, HardMaxTotalBytes),
		MaxFileBytes:  clamp(l.MaxFileBytes, HardMaxFileBytes),
	}
}

// VideoConfig controls frame extraction.
type VideoConfig struct {
	Frames int `json:"frames"`
}

// Config is the parsed .picpress-config file. The token is held here only
// while importing; persisted connections keep it in the system keychain.
type Config struct {
	Version        int         `json:"version"`
	Label          string      `json:"label"`
	CredentialsURL string      `json:"credentialsUrl"`
	Token          string      `json:"token"`
	Notice         string      `json:"notice,omitempty"`
	Limits         Limits      `json:"limits"`
	Video          VideoConfig `json:"video"`
}

// ConfigError carries a user-facing Chinese message that says what to do.
type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return e.Message }

// ParseConfig validates a config file's bytes.
func ParseConfig(data []byte) (Config, error) {
	var probe struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Config{}, &ConfigError{"这不是有效的 PicPress 配置文件（格式不对）。请向给你配置的人重新要一份。"}
	}
	if probe.Version == nil {
		return Config{}, &ConfigError{"配置文件缺少版本号，请向给你配置的人重新要一份。"}
	}
	if *probe.Version > ConfigVersion {
		return Config{}, &ConfigError{"这份配置的版本比你的 PicPress 新，请更新 PicPress 后再导入。"}
	}
	if *probe.Version < 1 {
		return Config{}, &ConfigError{"配置文件的版本号不对，请向给你配置的人重新要一份。"}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, &ConfigError{"配置文件内容不完整，请向给你配置的人重新要一份。"}
	}
	cfg.Label = strings.TrimSpace(cfg.Label)
	cfg.Token = strings.TrimSpace(cfg.Token)
	switch {
	case cfg.Label == "":
		return Config{}, &ConfigError{"配置文件缺少名称（label），请向给你配置的人重新要一份。"}
	case cfg.Token == "":
		return Config{}, &ConfigError{"配置文件缺少令牌（token），请向给你配置的人重新要一份。"}
	}
	if _, err := CheckURL(cfg.CredentialsURL); err != nil {
		return Config{}, &ConfigError{"配置文件里的地址不可用：" + err.Error()}
	}
	cfg.Limits = cfg.Limits.Clamped()
	if cfg.Video.Frames <= 0 {
		cfg.Video.Frames = DefaultVideoFrames
	}
	if cfg.Video.Frames > 48 {
		cfg.Video.Frames = 48
	}
	return cfg, nil
}

// CheckURL enforces the transport rule shared by the credentials endpoint and
// S3 endpoints: HTTPS, except plain HTTP to a loopback host (local MinIO and
// tests). It returns the parsed URL.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, errors.New("地址格式不对")
	}
	switch u.Scheme {
	case "https":
		return u, nil
	case "http":
		if isLoopback(u.Hostname()) {
			return u, nil
		}
		return nil, fmt.Errorf("只允许 HTTPS 地址（%s）", u.Host)
	default:
		return nil, fmt.Errorf("只允许 HTTPS 地址（%s）", u.Host)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
