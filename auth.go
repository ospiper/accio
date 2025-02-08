package accio

import "net/http"

// AuthProvider ...
type AuthProvider interface {
	Apply(req *http.Request) error
}

// BasicAuth ...
type BasicAuth struct {
	Username string
	Password string
}

// Apply ...
func (auth *BasicAuth) Apply(req *http.Request) error {
	req.SetBasicAuth(auth.Username, auth.Password)
	return nil
}

// BearerAuth ...
type BearerAuth struct {
	Token string
}

// Apply ...
func (auth *BearerAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	return nil
}
