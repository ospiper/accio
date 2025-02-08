package accio

import "net/http"

type Response struct {
	*http.Response
	BodyBytes []byte
}

func (r *Response) JSON(v any) error {
	return jsonUnmarshal(r.BodyBytes, v)
}
