package proto

func BuildPingMethodData(d []byte) []byte {
	ed, _ := Encode(MethodPing, d)
	return ed
}

func BuildPongMethodData(d []byte) []byte {
	ed, _ := Encode(MethodPong, d)
	return ed
}

func BuildKeyRequestData() []byte {
	ed, _ := Encode(MethodKeyRequest, nil)
	return ed
}

func BuildKeyResponseData(key string) []byte {
	ed, _ := Encode(MethodKeyResponse, []byte(key))
	return ed
}

func BuildData(d []byte) []byte {
	ed, _ := Encode(MethodData, d)
	return ed
}

func ParseKeyResponsePayloadData(d []byte) string {
	return string(d)
}
