package push

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"
)

// FatalError stops the whole batch: retrying cannot help and every other file
// would fail the same way (revoked config, wrong clock, blocked device).
type FatalError struct {
	Reason  Reason
	Message string // Chinese, says what to do next
}

func (e *FatalError) Error() string { return e.Message }

// Messages shared by the credentials exchange and S3 responses.
const (
	msgRevoked    = "这份配置已失效，请联系给你配置的人。"
	msgDevice     = "这台电脑不被允许上传，请联系给你配置的人。"
	msgClockSkew  = "你电脑的时间不准，请校对系统时间后再继续。"
	msgAuthFailed = "上传被拒绝（没有权限），请联系给你配置的人。"
)

type class int

const (
	classFail      class = iota // this file failed; others continue
	classRetry                  // transient: back off and try again
	classTransport              // no response at all: maybe offline
	classRefresh                // credentials rejected: refresh once
	classFatal                  // stop the batch
	classCanceled               // pause / cancel
)

// classify maps an error to what the engine does next.
func classify(err error) class {
	var fe *FatalError
	var se *S3Error
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded) && !isTransport(err):
		return classCanceled
	case errors.As(err, &fe):
		return classFatal
	case errors.As(err, &se):
		switch {
		case se.Code == "RequestTimeTooSkewed":
			return classFatal
		case se.Code == "ExpiredToken" || se.Code == "InvalidToken" || se.Status == 403:
			return classRefresh
		case se.Status == 429 || se.Status >= 500 || se.Code == "SlowDown" || se.Code == "RequestTimeout":
			return classRetry
		default:
			return classFail
		}
	case isTransport(err):
		return classTransport
	default:
		return classFail
	}
}

func isTransport(err error) bool {
	var ue *url.Error
	var ne net.Error
	return errors.As(err, &ue) || errors.As(err, &ne)
}

// asFatal turns errors classified fatal into a FatalError with a message.
func asFatal(err error) *FatalError {
	var fe *FatalError
	if errors.As(err, &fe) {
		return fe
	}
	var se *S3Error
	if errors.As(err, &se) && se.Code == "RequestTimeTooSkewed" {
		return &FatalError{Reason: ReasonAuth, Message: msgClockSkew}
	}
	return &FatalError{Reason: ReasonNetwork, Message: err.Error()}
}

// retryAfter extracts a server-requested delay.
func retryAfter(err error) time.Duration {
	var se *S3Error
	if errors.As(err, &se) {
		return se.RetryAfter
	}
	return 0
}

// failReason is the reason code recorded for a file that failed on err.
func failReason(err error) Reason {
	var se *S3Error
	if errors.As(err, &se) && (se.Status == 401 || se.Status == 403) {
		return ReasonAuth
	}
	return ReasonNetwork
}
