package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/KevinLeeNJ/ask/internal/application/ports"
	"github.com/KevinLeeNJ/ask/internal/domain/failure"
	"github.com/KevinLeeNJ/ask/internal/domain/provider"
	"github.com/KevinLeeNJ/ask/internal/version"
)

const maxResponseBytes = 4 << 20

func NewJSONRequest(
	ctx context.Context,
	method string,
	baseURL string,
	path string,
	payload any,
	headers map[string]string,
) (*http.Request, error) {
	endpoint, err := JoinURL(baseURL, path)
	if err != nil {
		return nil, failure.Wrap(
			failure.KindConfig,
			"config.base_url_invalid",
			map[string]string{"url": baseURL},
			err,
		)
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, failure.Wrap(failure.KindInternal, "request.encode_failed", nil, err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, failure.Wrap(failure.KindInternal, "request.create_failed", nil, err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if payload != nil && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "application/json")
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", version.UserAgent())
	}
	return request, nil
}

func JoinURL(baseURL, suffix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid base URL")
	}
	suffixURL, err := url.Parse(suffix)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimSuffix(parsed.Path, "/")
	suffixPath := "/" + strings.TrimPrefix(suffixURL.Path, "/")
	parsed.Path = basePath + suffixPath
	parsed.RawPath = ""
	parsed.RawQuery = suffixURL.RawQuery
	return parsed.String(), nil
}

func Do(ctx context.Context, client ports.HTTPDoer, request *http.Request, providerID provider.ID) ([]byte, error) {
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return nil, failure.Wrap(failure.KindCanceled, "request.canceled", nil, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return nil, failure.Wrap(
				failure.KindTransport,
				"transport.timeout",
				map[string]string{"provider": string(providerID)},
				err,
			)
		}
		return nil, failure.Wrap(
			failure.KindTransport,
			"transport.network",
			map[string]string{"provider": string(providerID), "reason": err.Error()},
			err,
		)
	}
	defer response.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, failure.Wrap(
			failure.KindTransport,
			"transport.read_failed",
			map[string]string{"provider": string(providerID)},
			readErr,
		)
	}
	if len(body) > maxResponseBytes {
		return nil, failure.New(
			failure.KindProtocol,
			"protocol.response_too_large",
			map[string]string{"provider": string(providerID)},
		)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, statusError(response, body, providerID)
	}
	return body, nil
}

func OpenStream(ctx context.Context, client ports.HTTPDoer, request *http.Request, providerID provider.ID) (io.ReadCloser, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, normalizeRequestError(ctx, err, providerID)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response.Body, nil
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, failure.Wrap(
			failure.KindTransport,
			"transport.read_failed",
			map[string]string{"provider": string(providerID)},
			readErr,
		)
	}
	return nil, statusError(response, body, providerID)
}

func normalizeRequestError(ctx context.Context, err error, providerID provider.ID) error {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return failure.Wrap(failure.KindCanceled, "request.canceled", nil, err)
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		return failure.Wrap(
			failure.KindTransport,
			"transport.timeout",
			map[string]string{"provider": string(providerID)},
			err,
		)
	}
	return failure.Wrap(
		failure.KindTransport,
		"transport.network",
		map[string]string{"provider": string(providerID), "reason": err.Error()},
		err,
	)
}

func DecodeJSON(body []byte, target any, providerID provider.ID) error {
	if err := json.Unmarshal(body, target); err != nil {
		return failure.Wrap(
			failure.KindProtocol,
			"protocol.response_decode_failed",
			map[string]string{
				"provider": string(providerID),
				"reason":   err.Error(),
			},
			err,
		)
	}
	return nil
}

func statusError(response *http.Response, body []byte, providerID provider.ID) error {
	status := response.StatusCode
	args := map[string]string{
		"provider": string(providerID),
		"status":   fmt.Sprintf("%d", status),
		"reason":   summarize(body),
	}
	switch {
	case status == http.StatusUnauthorized:
		return failure.New(failure.KindUpstream, "upstream.auth_failed", args)
	case status == http.StatusForbidden:
		return failure.New(failure.KindUpstream, "upstream.permission_denied", args)
	case status == http.StatusNotFound:
		return failure.New(failure.KindUpstream, "upstream.not_found", args)
	case status == http.StatusTooManyRequests:
		if retryAfter := strings.TrimSpace(response.Header.Get("Retry-After")); retryAfter != "" {
			args["retry_after"] = retryAfter
		}
		return failure.New(failure.KindUpstream, "upstream.rate_limited", args)
	case status >= 500:
		return failure.New(failure.KindUpstream, "upstream.server_error", args)
	default:
		return failure.New(failure.KindUpstream, "upstream.request_failed", args)
	}
}

func summarize(input []byte) string {
	text := strings.TrimSpace(string(input))
	if text == "" {
		return ""
	}
	const limit = 512
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return text
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
