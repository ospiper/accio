package accio

import "fmt"

// HTTPError wrap error for http response of unexpected status code
type HTTPError struct {
	Response *Response
}

// Error ...
func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: %s", e.Response.Status, string(e.Response.BodyBytes))
}
