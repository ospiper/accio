package accio

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

var data []byte

func TestServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	path := dir + "/test-file.bin"
	data = []byte("hello, accio server test")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	data, _ = io.ReadAll(f)
	fmt.Println("original file md5:", getMD5(data))
	app := gin.Default()
	app.StaticFile("/test-file", path)

	s := httptest.NewServer(app)
	defer s.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(s.URL + "/test-file")
	if err != nil {
		t.Fatalf("get test file: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !bytes.Equal(body, data) {
		t.Fatal("response data mismatch")
	}
}
