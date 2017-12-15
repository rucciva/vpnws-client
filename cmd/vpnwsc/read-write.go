package main

import (
	"context"
	"io"
)

func readWrite(r io.Reader, w io.Writer, buf []byte) (err error) {
	n, err := r.Read(buf)
	if err != nil {
		return err
	}
	_, err = w.Write(buf[:n])
	if err != nil {
		return err
	}
	return nil
}

func readWriteWithContext(ctx context.Context, r io.Reader, w io.Writer, buf []byte) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return readWrite(r, w, buf)
	}
}
