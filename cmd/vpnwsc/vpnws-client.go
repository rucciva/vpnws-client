package main

import (
	"context"
	"os/exec"
	"strings"
)

type VPNWSClient struct {
	wsc     *WSClient
	tap     *TapDevice
	buf     int
	cmdPrev string
	cmdNext string
}

func NewVPNWSClient(w *WSClient, t *TapDevice, b int, cp, cn string) (vc *VPNWSClient, err error) {
	vc = &VPNWSClient{}
	vc.wsc = w
	vc.tap = t
	vc.cmdPrev = cp
	vc.cmdNext = cn
	vc.buf = b
	return vc, err
}

func (this *VPNWSClient) execCMD(s string) error {
	cmd := strings.Replace(s, "{{.dev}}", this.tap.device.Name(), -1)
	if _, err := exec.Command("/bin/bash", "-c", cmd).Output(); err != nil {
		return err
	}
	return nil
}

func (this *VPNWSClient) Open(ctx context.Context) (cCtx context.Context, err error) {
	cCtx, cCancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cCancel()
		}
	}()

	if err = this.tap.Open(); err != nil {
		return cCtx, err
	}
	if err = this.wsc.Open(); err != nil {
		return cCtx, err
	}
	if err = this.execCMD(this.cmdPrev); err != nil {
		return cCtx, err
	}

	go func() {
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.tap, this.wsc, buf); err != nil {
				cCancel()
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.wsc, this.tap, buf); err != nil {
				cCancel()
				return
			}
		}
	}()

	if err = this.execCMD(this.cmdNext); err != nil {
		return cCtx, err
	}

	return cCtx, err
}

func (this *VPNWSClient) Close() (err error) {
	if this == nil {
		return nil
	}
	err = this.tap.Close()
	this.wsc.Close()

	return err
}
