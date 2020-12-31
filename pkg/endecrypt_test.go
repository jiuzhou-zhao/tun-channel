package pkg

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAES(t *testing.T) {
	crypter := NewAESEnDecrypt("1234")
	d := []byte("123")
	ed := crypter.Encrypt(d)
	dd, err := crypter.Decrypt(ed)
	assert.Nil(t, err)
	assert.True(t, bytes.Equal(d, dd))
	t.Logf("%v -> %v", d, ed)
}

func TestAES2(t *testing.T) {
	crypter := NewAESEnDecrypt("1234")
	d := []byte("123abc")
	ed := crypter.Encrypt(d)
	t.Logf("%v -> %v", d, ed)

	d = []byte("123efg")
	ed = crypter.Encrypt(d)
	t.Logf("%v -> %v", d, ed)
}
