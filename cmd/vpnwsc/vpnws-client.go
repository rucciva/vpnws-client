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

func (vc *VPNWSClient) execCMD(s string) error {
	if vc == nil || vc.wsc == nil || vc.tap == nil || vc.tap.device == nil {
		return ErrNil
	}
	cmd := strings.Replace(s, "{{.dev}}", vc.tap.device.Name(), -1)
	log.Printf("Executing command: %s", cmd)
	if _, err := exec.Command("/bin/bash", "-c", cmd).Output(); err != nil {
		return err
	}
	return nil
}

func (vc *VPNWSClient) Open(ctx context.Context) (cCtx context.Context, err error) {
	cCtx, cCancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cCancel()
		}
	}()

	if err = vc.wsc.Open(); err != nil {
		return cCtx, err
	}
	if err = vc.tap.Open(); err != nil {
		return cCtx, err
	}
	if err = vc.execCMD(vc.cmdBeforeConnect); err != nil {
		return cCtx, err
	}

	vc.wg.Add(2)
	go func() {
		log.Printf("websocket -> tap started")
		buf := make([]byte, vc.buf)
		for {
			if err = readWriteWithContext(ctx, vc.wsc, vc.tap, buf); err != nil {
				log.Printf("websocket -> tap error : %s", err)
				vc.wg.Done()
				cCancel()
				return
			}
		}
	}()

	go func() {
		log.Printf("tap -> websocket started")
		buf := make([]byte, vc.buf)
		for {
			if err = readWriteWithContext(ctx, vc.tap, vc.wsc, buf); err != nil {
				log.Printf("tap -> websocker error : %s", err)
				vc.wg.Done()
				cCancel()
				return
			}
		}
	}()

	if err = vc.execCMD(vc.cmdAfterConnect); err != nil {
		return cCtx, err
	}

	return cCtx, err
}
func (vc *VPNWSClient) WaitReadWrite() {
	vc.wg.Wait()
}

func (vc *VPNWSClient) Close() (err error) {
	if vc == nil {
		return nil
	}
	if err = vc.execCMD(vc.cmdBeforeDisconnect); err != nil {
		log.Println("error executin cleanup-prev cmd", err)
	}
	if err = vc.wsc.Close(); err != nil {
		log.Println("error stopping web socket connection", err)
	}
	if err := vc.execCMD(vc.cmdAfterDisconnect); err != nil {
		log.Println("error executin cleanup-next cmd", err)
	}

	return vc.tap.Close()
}
