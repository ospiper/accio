package accio

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// Request reusable structural request, read-only by default so it is thread-safe
type Request struct {
	cli *http.Client

	reuse bool

	noErrorOnFail bool

	method string
	url    string

	body   []byte
	header map[string]string

	timeout time.Duration

	auth AuthProvider
}

// initializers

func New() *Request {
	return NewWithClient(defaultCli)
}

func NewWithClient(client *http.Client) *Request {
	return &Request{cli: client}
}

// NewSession returns new request with a persistent cookie jar
func NewSession() *Request {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
	}
	return NewWithClient(client)
}

// structural info

// Reuse request will not clone itself on altering
func (r *Request) Reuse() *Request {
	r.reuse = true
	return r
}

func (r *Request) Clone() *Request {
	ret := *r
	ret.reuse = false
	ret.header = make(map[string]string, len(r.header))
	for k, v := range r.header {
		ret.header[k] = v
	}
	return &ret
}

func (r *Request) copyOnDemand() *Request {
	if r.reuse {
		return r
	}
	return r.Clone()
}

// request info

func (r *Request) Be(method, url string) *Request {
	ret := r.copyOnDemand()
	ret.method = method
	ret.url = url
	return ret
}

func (r *Request) Method(method string) *Request {
	ret := r.copyOnDemand()
	ret.method = method
	return ret
}

func (r *Request) Get(url string) *Request {
	return r.Be(http.MethodGet, url)
}

func (r *Request) Post(url string) *Request {
	return r.Be(http.MethodPost, url)
}

func (r *Request) Put(url string) *Request {
	return r.Be(http.MethodPut, url)
}

func (r *Request) Patch(url string) *Request {
	return r.Be(http.MethodPatch, url)
}

func (r *Request) Delete(url string) *Request {
	return r.Be(http.MethodDelete, url)
}

func (r *Request) Head(url string) *Request {
	return r.Be(http.MethodHead, url)
}

func (r *Request) Options(url string) *Request {
	return r.Be(http.MethodOptions, url)
}

// body

func (r *Request) Body(b []byte) *Request {
	ret := r.copyOnDemand()
	ret.body = b
	return ret
}

func (r *Request) BodyJSON(v any) *Request {
	ret := r.copyOnDemand()
	ret.body, _ = jsonMarshal(v)
	return ret
}

// header

func (r *Request) Header(kvs ...string) *Request {
	if len(kvs) == 0 {
		return r
	}
	ret := r.copyOnDemand()
	if len(kvs)%2 != 0 {
		kvs = append(kvs, "")
	}
	for i := 0; i < len(kvs); i += 2 {
		ret.header[kvs[i]] = kvs[i+1]
	}
	return ret
}

func (r *Request) Timeout(timeout time.Duration) *Request {
	ret := r.copyOnDemand()
	ret.timeout = timeout
	return ret
}

func (r *Request) WithoutTimeout() *Request {
	return r.Timeout(-1)
}

func (r *Request) Auth(auth AuthProvider) *Request {
	ret := r.copyOnDemand()
	ret.auth = auth
	return ret
}

func (r *Request) Range(start, end int64) *Request {
	ret := r.copyOnDemand()
	ret.header["Range"] = fmt.Sprintf("bytes=%d-%d", start, end)
	return ret
}

// functional info

// NoErrorOnFail response will be returned as-is on unexpected status codes (< 200 || >= 300)
func (r *Request) NoErrorOnFail() *Request {
	ret := r.copyOnDemand()
	ret.noErrorOnFail = true
	return ret
}
