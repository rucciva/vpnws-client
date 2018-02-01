package main

import (
	"log"
	"strconv"

	"github.com/liudanking/tuntap"
)

type TapDevice struct {
	prefix string
	device *tuntap.Interface

	*RWTimer
}

func NewTapDevice(p string, rw *RWTimer) (t *TapDevice, err error) {
	t = new(TapDevice)
	t.prefix = p
	t.RWTimer = rw
	return t, err
}

func (this *TapDevice) Open() (err error) {
	if this == nil {
		return ErrNil
	}
	if this.RWTimer == nil {
		return ErrNil
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
	log.Print("closing tap device ", this.device.Name())
	if err := this.device.Close(); err != nil {
		return err
	}
	this.device = nil
	return nil
}

func (this *TapDevice) Read(p []byte) (n int, err error) {
	if this == nil || this.device == nil {
		return 0, ErrNil
	}
	// log.Println("tap read started")
	n, err = this.device.Read(p)
	// log.Println("tap read finished")
	return
}

func (this *TapDevice) Write(p []byte) (n int, err error) {
	if this == nil || this.device == nil {
		return 0, ErrNil
	}
	// log.Println("tap write started")
	n, err = this.device.Write(p)
	// log.Println("tap write finished")
	return
}
