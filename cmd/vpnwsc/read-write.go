package main

import (
	"context"
	"errors"
	"io"
	"log"
	"runtime/debug"
	"time"
)

var (
	errRWTimeOut = errors.New("read or write timeout")
)

type WriterWithTimeout interface {
	io.Writer
	WriteTimeout() <-chan time.Time
	ResetWriteTimeout()
	RecreateWriteTimeout()
}

type ReaderWithTimeout interface {
	io.Reader
	ReadTimeout() <-chan time.Time
	ResetReadTimeout()
	RecreateReadTimeout()
}

type RWTimer struct {
	rdur, wdur     time.Duration
	rtimer, wtimer *time.Timer
}

func NewRWTimer(r, w time.Duration) *RWTimer {
	return &RWTimer{
		r, w,
		time.NewTimer(r), time.NewTimer(w),
	}
}

func (rw *RWTimer) WriteTimeout() <-chan time.Time {
	if rw == nil || rw.wtimer == nil {
		return nil
	}
	return rw.wtimer.C
}

func (rw *RWTimer) ResetWriteTimeout() {
	if rw == nil || rw.wtimer == nil {
		return
	}
	if !rw.wtimer.Stop() {
		<-rw.wtimer.C
	}
	rw.wtimer.Reset(rw.wdur)
}

func (rw *RWTimer) RecreateWriteTimeout() {
	if rw == nil {
		return
	}
	rw.wtimer = time.NewTimer(rw.wdur)
}

func (rw *RWTimer) ReadTimeout() <-chan time.Time {
	if rw == nil || rw.rtimer == nil {
		return nil
	}
	return rw.rtimer.C
}

func (rw *RWTimer) ResetReadTimeout() {
	if rw == nil || rw.rtimer == nil {
		return
	}
	if !rw.rtimer.Stop() {
		<-rw.rtimer.C
	}
	rw.rtimer.Reset(rw.rdur)
}

func (rw *RWTimer) RecreateReadTimeout() {
	if rw == nil {
		return
	}
	rw.rtimer = time.NewTimer(rw.rdur)
}

func readWrite(r ReaderWithTimeout, w WriterWithTimeout, buf []byte) (err error) {
	n, rErr, wErr := 0, make(chan error, 1), make(chan error, 1)

	// read with timeout
	r.ResetReadTimeout()
	go func() {
		var err2 error
		n, err2 = r.Read(buf)
		rErr <- err2
	}()
	select {
	case err = <-rErr:
	case <-r.ReadTimeout():
		r.RecreateReadTimeout()
		err = errRWTimeOut
	}
	if err != nil {
		return
	}

	// write with timeout
	w.ResetWriteTimeout()
	go func() {
		var err2 error
		n, err2 = w.Write(buf[:n])
		wErr <- err2
	}()
	select {
	case err = <-wErr:
	case <-w.WriteTimeout():
		w.RecreateWriteTimeout()
		err = errRWTimeOut
	}
	return
}

func readWriteWithContext(ctx context.Context, r ReaderWithTimeout, w WriterWithTimeout, buf []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("recover from read write ", string(debug.Stack()[:]))
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return readWrite(r, w, buf)
	}
}
