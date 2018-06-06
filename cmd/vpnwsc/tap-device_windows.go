package main

import "github.com/songgao/water"

func openTapDevice(prefix string) (tap tapDevice, err error) {
	return water.New(water.Config{
		DeviceType: water.TAP,
	})

}
