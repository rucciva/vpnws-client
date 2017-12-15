package main

import (
	"strconv"

	"github.com/liudanking/tuntap"
)

type TapDevice struct {
	prefix string
	device *tuntap.Interface
}

func NewTapDevice(p string) (t *TapDevice, err error) {
	t = new(TapDevice)
	t.prefix = p
	return t, err
}

func (this *TapDevice) Open() (err error) {
	if this == nil {
		return nil
	}
	for i := 0; i < maxTapDeviceCount; i++ {
		dev := strconv.AppendInt([]byte(this.prefix), int64(i), 10)
		if this.device, err = tuntap.Open(string(dev), tuntap.DevTap); err == nil {
			return nil
		}
	}
	return err
}

func (this *TapDevice) Close() error {
	if this == nil || this.device == nil {
		return nil
	}
	return this.device.Close()
}

func (this *TapDevice) Read(p []byte) (n int, err error) {
	if this == nil || this.device == nil {
		return 0, nil
	}
	return this.device.Read(p)
}

func (this *TapDevice) Write(p []byte) (n int, err error) {
	if this == nil || this.device == nil {
		return 0, nil
	}
	return this.device.Write(p)
}
