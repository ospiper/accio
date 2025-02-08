package accio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

func (r *Request) toRequest(ctx context.Context) (*http.Request, error) {
	if r.method == "" || r.url == "" {
		return nil, fmt.Errorf("method and url are required")
	}
	req, err := http.NewRequestWithContext(ctx, r.method, r.url, bytes.NewReader(r.body))
	if err != nil {
		return nil, err
	}

	for k, v := range r.header {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (r *Request) prepareContext(parent context.Context) (context.Context, context.CancelFunc) {
	if r.timeout < 0 {
		return parent, func() {}
	}
	var timeout = r.timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func (r *Request) DoRaw(ctx context.Context) (*http.Response, error) {
	ctx, cancel := r.prepareContext(ctx)
	defer cancel()

	req, err := r.toRequest(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := r.cli.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Do send request
func (r *Request) Do(ctx context.Context) (*Response, error) {
	resp, err := r.DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ret := &Response{
		Response:  resp,
		BodyBytes: body,
	}
	if !r.noErrorOnFail && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return nil, &HTTPError{Response: ret}
	}
	return ret, nil
}
