package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/liudanking/tuntap"
	"github.com/pborman/getopt"
	ping "github.com/sparrc/go-ping"
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
	ErrParallelClose      = errors.New("parralel close")

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
	if this == nil || this.ws == nil {
		return 0, nil
	}
	return this.ws.Read(p)
}

func (this *WSClient) Write(p []byte) (n int, err error) {
	if this == nil || this.ws == nil {
		return 0, nil
	}
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
	if this == nil || this.device == nil {
		return 0, nil
	}
	return this.device.Read(p)
}

func (this *TapDevice) Write(p []byte) (n int, err error) {
	if this == nil || this.device == nil {
		return 0, nil
	}
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
	wsc     *WSClient
	tap     *TapDevice
	buf     int
	cmdPrev string
	cmdNext string
}

func NewVPNClient(w *WSClient, t *TapDevice, b int, cp, cn string) (vc *VPNClient, err error) {
	vc = &VPNClient{}
	vc.wsc = w
	vc.tap = t
	vc.cmdPrev = cp
	vc.cmdNext = cn
	vc.buf = b
	return vc, err
}

func (this *VPNClient) execCMD(s string) error {
	cmd := strings.Replace(s, "{{.dev}}", this.tap.device.Name(), -1)
	if _, err := exec.Command("/bin/bash", "-c", cmd).Output(); err != nil {
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

func (this *VPNClient) Close() (err error) {
	if this == nil {
		return nil
	}
	err = this.tap.Close()
	this.wsc.Close()

	return err
}

func sendPing(ctx context.Context, host string, dur time.Duration) (err error) {
	p, err := ping.NewPinger(host)
	if err != nil {
		return err
	}
	p.SetPrivileged(true)
	p.Count = 1
	p.OnRecv = func(pkt *ping.Packet) {
		log.Printf("received %d ping bytes from %s: icmp_seq=%d time=%v\n",
			pkt.Nbytes, pkt.IPAddr, pkt.Seq, pkt.Rtt)
	}

	t := time.NewTicker(dur)
	log.Printf("will try to send ping to %s for every %d seconds", host, dur/time.Second)
	for {
		select {
		case <-ctx.Done():
			return

		case <-t.C:
			log.Printf("send ping to: %s", host)
			p.Run()
		}
	}
}

func main() {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	chSigs := make(chan os.Signal, 1)
	signal.Notify(chSigs, syscall.SIGINT, syscall.SIGTERM)
	chClose := make(chan bool, 1)
	go func() {
		<-chSigs
		log.Println("Got exit command. Please wait till clean up process done")

		chClose <- true
		if err = vpnc.Close(); err != nil {
			log.Fatalln("Cannot close vpn: " + err.Error())
		}
		log.Println("Clean up success. Bye")
		os.Exit(0)
	}()

	// prepare command line options and arguments
	origin := getopt.StringLong("origin", 'o', "http://localhost", "origin of the request. Default: http://localhost", "http[s]://<origin>")
	username := getopt.StringLong("username", 'u', "", "http basic username.", "string")
	password := getopt.StringLong("password", 'p', "", "http basic password.", "string")
	pkcs12File := getopt.StringLong("pkcs12-file", 0, "", "PKCS12 file containing private key and certificate.", "string")
	pkcs12FilePassword := getopt.StringLong("pkcs12-file-pass", 0, "", "PKCS12 password.", "string")
	skipVerifyClient := getopt.BoolLong("skip-verify-client", 0, "Skip Verify Client Certificate", "boolean")
	skipVerifyServer := getopt.BoolLong("skip-verify-server", 0, "Skip Verify Server Certificate", "boolean")
	tapPref := getopt.StringLong("interface", 'i', "tap", "tap interface prefix. Default: tap", "string")
	cmdPrev := getopt.StringLong("exec-prev", 0, "echo", "command to run right after successful connection and before read write operation, e.g 'ipconfig set {{.dev}} DHCP'", "string")
	cmdNext := getopt.StringLong("exec-next", 0, "echo", "command to run right after read write operation started, e.g 'dhclient {{.dev}}'", "string")
	bufSize := getopt.IntLong("buf-size", 0, defBuffSize, "read write buffer size. Default: 1526", "int")
	keepAliveHost := getopt.StringLong("keep-alive-host", 0, "192.168.11.3", "ip address of machine that will receive ping packet", "string")
	keepAliveTick := getopt.IntLong("keep-alive-tick", 0, 15, "keep alive ticker in second", "string")
	getopt.SetParameters("ws[s]://websocket.server.address[/some/path?some=query]")
	if err := getopt.Getopt(nil); err != nil {
		log.Println("error in parsing commang line argument:" + err.Error())
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
		log.Println("cannot init tap device:" + err.Error())
		os.Exit(1)
	}

	if wsc, err = NewWSClient(); err != nil {
		log.Println("cannot init ws client:" + err.Error())
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

	if vpnc, err = NewVPNClient(wsc, tap, *bufSize, *cmdPrev, *cmdNext); err != nil {
		log.Println("cannot init vpn client:" + err.Error())
		os.Exit(1)
	}

	var cCtx context.Context
	if cCtx, err = vpnc.Open(ctx); err != nil {
		log.Println("cannot open vpn connection:" + err.Error())
	} else {
		log.Println("VPN Connection Established")
		log.Println("Tap Device: " + vpnc.tap.device.Name())
	}

	go sendPing(ctx, *keepAliveHost, time.Duration(*keepAliveTick)*time.Second)

	for {
		select {
		case <-cCtx.Done():
			chClose <- true
			log.Println("VPN Connection Interrupted")
			if err = vpnc.Close(); err != nil {
				log.Println("Cannot completely close VPN connection: " + err.Error())
				log.Println("Bye")

				cancel()
				os.Exit(1)
			}
			<-chClose

			log.Println("Retrying...")
			if cCtx, err = vpnc.Open(ctx); err != nil {
				log.Println("Cannot Re-Establish VPN connection:" + err.Error())
				log.Println("Wait before retrying...")
				<-time.After(time.Minute)
			}

			log.Println("VPN Re-Established")

		}
	}
}
