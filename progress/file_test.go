package progress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufferWriterAt(t *testing.T) {
	buf := NewBufferWriterAt(20)
	buf.WriteAt([]byte{5, 6, 7, 8, 9}, 4)
	buf.WriteAt([]byte{15, 16, 17}, 14)
	buf.WriteAt([]byte{1, 2, 3, 4}, 0)
	buf.WriteAt([]byte{10, 11, 12, 13, 14}, 9)
	buf.WriteAt([]byte{18, 19, 20}, 17)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, buf.Bytes())
}
