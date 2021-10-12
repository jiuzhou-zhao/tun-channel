package proto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/ioutil"
)

const (
	// magicNumberV1 uint32 = 0x1F082A91
	magicNumberV2 uint32 = 0x1F082A92
)

var (
	errInvaliMagic = errors.New("invalidMagic")
)

type MethodID uint8

const (
	MethodPing           MethodID = 1
	MethodPong           MethodID = 2
	MethodKeyRequest     MethodID = 3
	MethodKeyResponse    MethodID = 4
	MethodData           MethodID = 5
	MethodForwardControl MethodID = 7
)

func Decode(rawData []byte) (m MethodID, d []byte, err error) {
	reader := bytes.NewReader(rawData)

	var magicNumber uint32
	err = binary.Read(reader, binary.BigEndian, &magicNumber)

	if err != nil {
		return
	}

	if magicNumber != magicNumberV2 {
		err = errInvaliMagic

		return
	}

	err = binary.Read(reader, binary.BigEndian, &m)
	if err != nil {
		return
	}

	d, err = ioutil.ReadAll(reader)

	return
}

func Encode(m MethodID, d []byte) (ed []byte, err error) {
	writer := &bytes.Buffer{}
	err = binary.Write(writer, binary.BigEndian, magicNumberV2)

	if err != nil {
		return
	}

	err = binary.Write(writer, binary.BigEndian, m)
	if err != nil {
		return
	}

	_, err = writer.Write(d)
	ed = writer.Bytes()

	return
}
