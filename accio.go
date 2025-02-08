package accio

import (
	"encoding/json"
	"net/http"
	"time"
)

var (
	defaultTimeout = time.Second * 5
)

var (
	defaultCli    *http.Client
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
)

func init() {
	defaultCli = http.DefaultClient
}

// SetDefaultClient set default http client when using New()
func SetDefaultClient(c *http.Client) {
	defaultCli = c
}

// SetJSONProcessor set encoder and decoder functions for default JSON handling
func SetJSONProcessor(marshaller func(any) ([]byte, error), unmarshaler func([]byte, any) error) {
	jsonMarshal, jsonUnmarshal = marshaller, unmarshaler
}

// SetDefaultTimeout set default timeout for requests
func SetDefaultTimeout(timeout time.Duration) {
	defaultTimeout = timeout
}

// accio.New().Get("baidu.com").Header("x-foo", "x-bar").Do(ctx)
