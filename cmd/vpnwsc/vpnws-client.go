package main

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
)

type VPNWSClient struct {
	wsc *WSClient
	tap *TapDevice
	buf int

	cmdBeforeConnect    string
	cmdAfterConnect     string
	cmdBeforeDisconnect string
	cmdAfterDisconnect  string

	wg sync.WaitGroup
}

func NewVPNWSClient(w *WSClient, t *TapDevice, buf int) (vc *VPNWSClient, err error) {
	vc = &VPNWSClient{}
	vc.wsc = w
	vc.tap = t
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
	if err = this.execCMD(this.cmdBeforeConnect); err != nil {
		return cCtx, err
	}

	this.wg.Add(2)
	go func() {
		log.Printf("websocket -> tap started")
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.wsc, this.tap, buf); err != nil {
				log.Printf("websocket -> tap error : %s", err)
				this.wg.Done()
				cCancel()
				return
			}
		}
	}()

	go func() {
		log.Printf("tap -> websocket started")
		buf := make([]byte, this.buf)
		for {
			if err = readWriteWithContext(ctx, this.tap, this.wsc, buf); err != nil {
				log.Printf("tap -> websocker error : %s", err)
				this.wg.Done()
				cCancel()
				return
			}
		}
	}()

	if err = this.execCMD(this.cmdAfterConnect); err != nil {
		return cCtx, err
	}

	return cCtx, err
}
func (this *VPNWSClient) WaitReadWrite() {
	this.wg.Wait()
}

func (this *VPNWSClient) Close() (err error) {
	if this == nil {
		return nil
	}
	if err = this.execCMD(this.cmdBeforeDisconnect); err != nil {
		log.Println("error executin cleanup-prev cmd", err)
	}
	if err = this.wsc.Close(); err != nil {
		log.Println("error stopping web socket connection", err)
	}
	if err := this.execCMD(this.cmdAfterDisconnect); err != nil {
		log.Println("error executin cleanup-next cmd", err)
	}

	return this.tap.Close()
}
