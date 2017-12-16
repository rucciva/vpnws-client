package main

import (
	"context"
	"log"
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

func NewVPNWSClient(w *WSClient, t *TapDevice, buf int, cmdPrev, cmdNext string) (vc *VPNWSClient, err error) {
	vc = &VPNWSClient{}
	vc.wsc = w
	vc.tap = t
	vc.cmdPrev = cmdPrev
	vc.cmdNext = cmdNext
	vc.buf = buf
	return vc, err
}

func (this *VPNWSClient) execCMD(s string) error {
	if this == nil || this.wsc == nil || this.tap == nil || this.tap.device == nil {
		return ErrNil
	}
	cmd := strings.Replace(s, "{{.dev}}", this.tap.device.Name(), -1)
	log.Printf("Executing command: %s", cmd)
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

	if err = this.wsc.Open(); err != nil {
		return cCtx, err
	}
	if err = this.tap.Open(); err != nil {
		return cCtx, err
	}
	if err = this.execCMD(this.cmdPrev); err != nil {
		return cCtx, err
	}

	go func() {
		log.Printf("start read from web socket and write to tap")
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.wsc, this.tap, buf); err != nil {
				log.Printf("got error: '%s' when read from ws connection then write to tap device", err)
				cCancel()
				return
			}
		}
	}()

	go func() {
		log.Printf("start read from tap and write to web socket")
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.tap, this.wsc, buf); err != nil {
				log.Printf("got error: '%s' when read from tap device then write to ws connection", err)
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
