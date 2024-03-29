package tunchannel

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

var (
	errNotDotIP  = errors.New("notDotIP")
	errInvalidIP = errors.New("invalidIP")
)

func ToIP(s string) (ip string, err error) {
	if !strings.Contains(s, "/") {
		ip = net.ParseIP(s).To4().String()

		return
	}

	ipc, _, err := net.ParseCIDR(s)
	if err != nil {
		return
	}

	ip = ipc.To4().String()

	return
}

func ToCIDR(s string) (cidr string, err error) {
	if strings.Count(s, ".") != 3 {
		err = errNotDotIP

		return
	}

	if strings.Contains(s, "/") {
		_, _, err = net.ParseCIDR(s)
		if err == nil {
			cidr = s
		}

		return
	}

	mask := net.ParseIP(s).DefaultMask()
	if mask == nil {
		err = errInvalidIP

		return
	}

	ones, _ := mask.Size()
	cidr = s + fmt.Sprintf("/%d", ones)

	return
}
