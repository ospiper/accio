package accio

import (
	"context"
	"errors"
	"testing"
)

func TestBodyJSONError(t *testing.T) {
	orig := jsonMarshal
	t.Cleanup(func() {
		jsonMarshal = orig
	})
	expected := errors.New("marshal failed")
	jsonMarshal = func(any) ([]byte, error) {
		return nil, expected
	}

	req := New().Post("http://example.invalid").BodyJSON(map[string]any{"x": 1})
	_, err := req.Do(context.Background())
	if err == nil {
		t.Fatal("expected error from BodyJSON marshal failure")
	}
	if !errors.Is(err, expected) {
		t.Fatalf("unexpected error: %v", err)
	}
}
