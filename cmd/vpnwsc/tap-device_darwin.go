package main

import (
	"strconv"

	"github.com/liudanking/tuntap"
)

func openTapDevice(prefix string) (tap tapDevice, err error) {
	for i := 0; i < maxTapDeviceCount; i++ {
		dev := strconv.AppendInt([]byte(prefix), int64(i), 10)
		if tap, err = tuntap.Open(string(dev), tuntap.DevTap); err == nil {
			return
		}
	}
	return
}
