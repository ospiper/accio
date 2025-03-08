package accio

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

var data []byte

func TestServer(t *testing.T) {
	f, err := os.Open("/Users/xiao.liang/Downloads/19 WE ARE SOUTH.wav")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	data, _ = io.ReadAll(f)
	fmt.Println("original file md5:", getMD5(data))
	app := gin.Default()
	app.StaticFile("test-file", "/Users/xiao.liang/Downloads/19 WE ARE SOUTH.wav")

	app.Run(":19527")
}
