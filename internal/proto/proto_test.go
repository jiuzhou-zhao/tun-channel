package proto

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEnDecode(t *testing.T) {
	ed, err := Encode(MethodPing, []byte("abcd"))
	assert.Nil(t, err)
	m, d, err := Decode(ed)
	assert.Nil(t, err)
	assert.Equal(t, m, MethodPing)
	assert.Equal(t, string(d), "abcd")
}

func TestEnDecod2e(t *testing.T) {
	ed, err := Encode(MethodPing, nil)
	assert.Nil(t, err)
	m, d, err := Decode(ed)
	assert.Nil(t, err)
	assert.Equal(t, m, MethodPing)
	assert.Equal(t, string(d), "")
}
