package proto

import "encoding/json"

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

type KeyResponseData struct {
	Key    string
	VpnIPs []string
	LanIPs []string
}

func BuildKeyResponseData(key string, vpnIPs, lanIPs []string) []byte {
	d, _ := json.Marshal(&KeyResponseData{
		Key:    key,
		VpnIPs: vpnIPs,
		LanIPs: lanIPs,
	})
	ed, _ := Encode(MethodKeyResponse, d)

	return ed
}

func BuildData(d []byte) []byte {
	ed, _ := Encode(MethodData, d)

	return ed
}

func ParseKeyResponsePayloadData(d []byte) (key string, vpnIPs, lanIPs []string, err error) {
	var respD KeyResponseData
	err = json.Unmarshal(d, &respD)

	if err != nil {
		return
	}

	key = respD.Key
	vpnIPs = respD.VpnIPs
	lanIPs = respD.LanIPs

	return
}

type ForwardControlData struct {
	OldIPs []string
	NewIPs []string
}

func BuildForwardControlData(oldIPs, newIPs []string) []byte {
	d, _ := json.Marshal(&ForwardControlData{
		OldIPs: oldIPs,
		NewIPs: newIPs,
	})
	ed, _ := Encode(MethodForwardControl, d)

	return ed
}

func ParseForwardControlPayloadData(d []byte) (oldIPs, newIPs []string, err error) {
	var respD ForwardControlData
	err = json.Unmarshal(d, &respD)

	if err != nil {
		return
	}

	oldIPs = respD.OldIPs
	newIPs = respD.NewIPs

	return
}
