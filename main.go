package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/liudanking/tuntap"
	"github.com/pborman/getopt"
	"golang.org/x/crypto/pkcs12"
	"golang.org/x/net/websocket"
)

const defBuffSize int = 1526
const errWaitTime time.Duration = 3000
const maxTapDeviceCount = 16

var (
	ErrExpired            = errors.New("certificate has expired or is not yet valid")
	ErrInvalidCertificate = errors.New("certificate is not yet valid")
	ErrParalelReconnect   = errors.New("parallel reconnection")
	ErrTooEarlyToConnect  = errors.New("possible parallel reconnect. Must wait before reconnection")

	tap  *TapDevice
	wsc  *WSClient
	vpnc *VPNClient
)

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func loadCertificatePair(pkcs12File, pkcs12FilePassword string) (*tls.Certificate, error) {
	content, err := ioutil.ReadFile(pkcs12File)
	if err != nil {
		return nil, err
	}

	privateKey, cert, err := pkcs12.Decode(content, pkcs12FilePassword)
	if err != nil {
		return nil, err
	}

	c := &tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  privateKey,
		Leaf:        cert,
	}
	if err := verify(cert); err != nil {
		return c, err
	}

	// wraps x509 certificate as a tls.Certificate:
	return c, nil
}

func verify(cert *x509.Certificate) error {
	_, err := cert.Verify(x509.VerifyOptions{})
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case x509.CertificateInvalidError:
		switch e.Reason {
		case x509.Expired:
			return ErrExpired
		default:
			return err
		}
	case x509.UnknownAuthorityError:
		// Apple cert isn't in the cert pool
		// ignoring this error
		return nil
	default:
		return err
	}
}

type WSClient struct {
	Url                string
	Origin             string
	Username           string
	Password           string
	PKCS12File         string
	PKCS12FilePassword string

	SkipVerifyServer bool
	SkipVerifyClient bool

	ws                  *websocket.Conn
	chanConn            chan bool
	lastSuccessConnTime time.Time
}

func NewWSClient() (c *WSClient, err error) {
	c = new(WSClient)
	c.chanConn = make(chan bool, 1)
	c.lastSuccessConnTime = time.Time{}
	return c, err
}

func (this *WSClient) Open() error {
	if this.ws != nil {
		this.ws.Close()
	}
	conf, err := websocket.NewConfig(this.Url, this.Origin)
	if err != nil {
		return err
	}
	conf.Header.Add("Authorization", "Basic "+basicAuth(this.Username, this.Password))
	if this.PKCS12File == "" {
		return ErrInvalidCertificate
	}
	cert, err := loadCertificatePair(this.PKCS12File, this.PKCS12FilePassword)
	if err != nil && cert == nil {
		return err
	}
	if err != nil && cert != nil && !this.SkipVerifyClient {
		return err
	}
	conf.TlsConfig = &tls.Config{
		InsecureSkipVerify: this.SkipVerifyServer,
		Certificates: []tls.Certificate{
			*cert,
		},
	}
	if this.ws, err = websocket.DialConfig(conf); err != nil {
		return err
	}
	return nil
}

func (this *WSClient) Close() error {
	if this == nil || this.ws == nil {
		return nil
	}
	return this.ws.Close()
}

func (this *WSClient) Read(p []byte) (n int, err error) {
	return this.ws.Read(p)
}

func (this *WSClient) Write(p []byte) (n int, err error) {
	return this.ws.Write(p)
}

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
	return this.device.Read(p)
}

func (this *TapDevice) Write(p []byte) (n int, err error) {
	return this.device.Write(p)
}

func exeCmd(cmd string, ws *WSClient, tap *TapDevice) (string, error) {
	cmd = strings.Replace(cmd, "{{.dev}}", tap.device.Name(), -1)
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		return "", err
	}
	if len(out) > 0 {
		return string(out), nil
	}
	return "", nil
}

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

type VPNClient struct {
	wsc *WSClient
	tap *TapDevice
	buf int
	cmd string
}

func NewVPNClient(w *WSClient, t *TapDevice, b int, c string) (vc *VPNClient, err error) {
	vc = &VPNClient{}
	vc.wsc = w
	vc.tap = t
	vc.cmd = c
	vc.buf = b
	return vc, err
}

func (this *VPNClient) execCMD() error {
	cmd := strings.Replace(this.cmd, "{{.dev}}", this.tap.device.Name(), -1)
	if _, err := exec.Command("bash", "-c", cmd).Output(); err != nil {
		return err
	}
	return nil
}

func (this *VPNClient) Open(ctx context.Context) (cCtx context.Context, err error) {
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
	if err = this.execCMD(); err != nil {
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

	return cCtx, err
}

func (this *VPNClient) Close() (err error) {
	if this == nil {
		return nil
	}
	defer func() { err = this.tap.Close() }()
	defer func() { err = this.wsc.Close() }()
	return err
}

func main() {
	var err error

	// prepare command line options and arguments
	origin := getopt.StringLong("origin", 'o', "http://localhost", "origin of the request. Default: http://localhost", "http[s]://<origin>")
	username := getopt.StringLong("username", 'u', "", "http basic username.", "string")
	password := getopt.StringLong("password", 'p', "", "http basic password.", "string")
	pkcs12File := getopt.StringLong("pkcs12-file", 0, "", "PKCS12 file containing private key and certificate.", "string")
	pkcs12FilePassword := getopt.StringLong("pkcs12-file-pass", 0, "", "PKCS12 password.", "string")
	skipVerifyClient := getopt.BoolLong("skip-verify-client", 0, "Skip Verify Client Certificate", "boolean")
	skipVerifyServer := getopt.BoolLong("skip-verify-server", 0, "Skip Verify Server Certificate", "boolean")
	tapPref := getopt.StringLong("interface", 'i', "tap", "tap interface prefix. Default: tap", "string")
	cmd := getopt.StringLong("exec", 'x', "echo", "command to run right after successful connection and before read write operation, e.g 'ipconfig set tap1 DHCP'", "string")
	bufSize := getopt.IntLong("buf-size", 0, defBuffSize, "read write buffer size. Default: 1526", "int")
	getopt.SetParameters("ws[s]://websocket.server.address[/some/path?some=query]")
	if err := getopt.Getopt(nil); err != nil {
		fmt.Println("error in parsing commang line argument:" + err.Error())
		getopt.Usage()
		os.Exit(1)
	}
	// check necessary arguments
	args := getopt.Args()
	if len(args) < 1 {
		getopt.Usage()
		os.Exit(1)
	}
	url := args[0]

	// open tap device
	if tap, err = NewTapDevice(*tapPref); err != nil {
		fmt.Println("cannot init tap device:" + err.Error())
		os.Exit(1)
	}

	if wsc, err = NewWSClient(); err != nil {
		fmt.Println("cannot init ws client:" + err.Error())
		os.Exit(1)
	}
	wsc.Url = url
	wsc.Origin = *origin
	wsc.Username = *username
	wsc.Password = *password
	wsc.PKCS12File = *pkcs12File
	wsc.PKCS12FilePassword = *pkcs12FilePassword
	wsc.SkipVerifyClient = *skipVerifyClient
	wsc.SkipVerifyServer = *skipVerifyServer

	ctx, cancel := context.WithCancel(context.Background())
	chSigs := make(chan os.Signal, 1)
	signal.Notify(chSigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-chSigs
		fmt.Println("Got exit command")
		fmt.Println("Please wait till clean up process done")
		cancel()
		vpnc.Close()
		os.Exit(0)
	}()

	if vpnc, err = NewVPNClient(wsc, tap, *bufSize, *cmd); err != nil {
		fmt.Println("cannot init vpn client:" + err.Error())
		os.Exit(1)
	}

	var cCtx context.Context
	if cCtx, err = vpnc.Open(ctx); err != nil {
		fmt.Println("cannot open vpn connection:" + err.Error())
	} else {
		fmt.Println("VPN Connection Established")
	}

	for {
		select {
		case <-cCtx.Done():
			fmt.Println("VPN Connection Interrupted")
			vpnc.Close()

			fmt.Println("Retrying...")
			if cCtx, err = vpnc.Open(ctx); err != nil {
				fmt.Println("Cannot Re-Establish VPN connection:" + err.Error())
				fmt.Println("Wait before retrying...")
				<-time.After(time.Minute)
			}

			fmt.Println("VPN Re-Established")

		}
	}
}