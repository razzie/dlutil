package dlutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/iunary/fakeuseragent"
	"github.com/razzie/razcache"
)

var DefaultDownloadOptions = DownloadOptions{
	Ctx:    context.Background(),
	Client: http.DefaultClient,
	Method: "GET",
}

type DownloadOptions struct {
	Ctx               context.Context
	Client            *http.Client
	Cache             razcache.Cache
	CacheKey          string
	CacheTTL          time.Duration
	GenError          func(r io.Reader, code int) error
	Method            string
	Body              io.Reader
	BodyContentType   string
	Header            http.Header
	AcceptContentType string
	IgnoreStatusCode  bool
}

type DownloadOption func(*DownloadOptions)

func WithContext(ctx context.Context) DownloadOption {
	return func(do *DownloadOptions) {
		if ctx == nil {
			do.Ctx = context.Background()
		} else {
			do.Ctx = ctx
		}
	}
}

func WithClient(client *http.Client) DownloadOption {
	return func(do *DownloadOptions) {
		if client == nil {
			do.Client = http.DefaultClient
		} else {
			do.Client = client
		}
	}
}

func WithCache(cache razcache.Cache, key string, ttl time.Duration) DownloadOption {
	return func(do *DownloadOptions) {
		do.Cache = cache
		do.CacheKey = key
		do.CacheTTL = ttl
	}
}

func WithErrorType[T error]() DownloadOption {
	return func(do *DownloadOptions) {
		do.GenError = func(r io.Reader, code int) error {
			var result T
			decoder := json.NewDecoder(r)
			if err := decoder.Decode(&result); err != nil {
				return BadStatus(code)
			}
			return result
		}
	}
}

func WithMethod(method string) DownloadOption {
	return func(do *DownloadOptions) {
		do.Method = method
	}
}

func WithBody(body io.Reader, contentType string) DownloadOption {
	return func(do *DownloadOptions) {
		do.Body = body
		do.BodyContentType = contentType
	}
}

func WithHeader(key, value0 string, values ...string) DownloadOption {
	return func(do *DownloadOptions) {
		if do.Header == nil {
			do.Header = make(http.Header)
		}
		do.Header.Set(key, value0)
		for _, value := range values {
			do.Header.Add(key, value)
		}
	}
}

func WithFakeUserAgent() DownloadOption {
	return WithHeader("User-Agent", fakeuseragent.RandomUserAgent())
}

func WithAcceptContentType(contentType string) DownloadOption {
	return func(do *DownloadOptions) {
		do.AcceptContentType = contentType
	}
}

func WithIgnoreStatusCode() DownloadOption {
	return func(do *DownloadOptions) {
		do.IgnoreStatusCode = true
	}
}

func Download(url string, o ...DownloadOption) (io.ReadCloser, error) {
	opts := DefaultDownloadOptions
	for _, o := range o {
		o(&opts)
	}

	if opts.Cache != nil {
		content, err := opts.Cache.Get(opts.CacheKey)
		if err == nil {
			return io.NopCloser(strings.NewReader(content)), nil
		}
	}

	req, err := http.NewRequestWithContext(opts.Ctx, opts.Method, url, opts.Body)
	if err != nil {
		return nil, err
	}
	for key, values := range opts.Header {
		req.Header[key] = values
	}
	if len(opts.BodyContentType) > 0 {
		req.Header.Set("Content-Type", opts.BodyContentType)
	}
	resp, err := opts.Client.Do(req)
	if err != nil {
		return nil, err
	}
	body := resp.Body

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		if opts.GenError != nil && matchContentType(resp, "application/json") {
			defer body.Close()
			return nil, opts.GenError(body, resp.StatusCode)
		}
		if !opts.IgnoreStatusCode {
			body.Close()
			return nil, BadStatus(resp.StatusCode)
		}
	}

	if len(opts.AcceptContentType) > 0 && !matchContentType(resp, opts.AcceptContentType) {
		body.Close()
		return nil, errors.New("bad content-type: " + resp.Header.Get("Content-Type"))
	}

	if opts.Cache != nil {
		content, err := io.ReadAll(body)
		if err != nil {
			body.Close()
			return nil, err
		}
		opts.Cache.Set(opts.CacheKey, string(content), opts.CacheTTL)
		body = io.NopCloser(bytes.NewReader(content))
	}

	return body, nil
}

func DownloadBytes(url string, o ...DownloadOption) ([]byte, error) {
	body, err := Download(url, o...)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func DownloadJSON[T any](url string, o ...DownloadOption) (*T, error) {
	body, err := Download(url, append(o, WithAcceptContentType("application/json"))...)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	decoder := json.NewDecoder(body)
	result := new(T)
	if err := decoder.Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

func matchContentType(resp *http.Response, contentType string) bool {
	parsedType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return contentType == parsedType
}
