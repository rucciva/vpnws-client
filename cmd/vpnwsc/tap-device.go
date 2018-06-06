package main

import (
	"log"
)

type tapDevice interface {
	Name() string
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

type TapDevice struct {
	prefix string
	device tapDevice

	*RWTimer
}

func NewTapDevice(p string, rw *RWTimer) (t *TapDevice, err error) {
	t = new(TapDevice)
	t.prefix = p
	t.RWTimer = rw
	return t, err
}

func (tap *TapDevice) Name() string {
	return tap.device.Name()
}

func (tap *TapDevice) Open() (err error) {
	if tap == nil {
		return ErrNil
	}
	if tap.RWTimer == nil {
		return ErrNil
	}
	tap.device, err = openTapDevice(tap.prefix)
	return err
}

func (tap *TapDevice) Close() error {
	if tap == nil || tap.device == nil {
		return nil
	}
	log.Print("closing tap device ", tap.device.Name())
	if err := tap.device.Close(); err != nil {
		return err
	}
	tap.device = nil
	return nil
}

func (tap *TapDevice) Read(p []byte) (n int, err error) {
	if tap == nil || tap.device == nil {
		return 0, ErrNil
	}
	// log.Println("tap read started")
	n, err = tap.device.Read(p)
	// log.Println("tap read finished")
	return
}

func (tap *TapDevice) Write(p []byte) (n int, err error) {
	if tap == nil || tap.device == nil {
		return 0, ErrNil
	}
	// log.Println("tap write started")
	n, err = tap.device.Write(p)
	// log.Println("tap write finished")
	return
}
