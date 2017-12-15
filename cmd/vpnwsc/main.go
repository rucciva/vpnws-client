package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"

	"syscall"
	"time"

	"github.com/pborman/getopt"
	ping "github.com/sparrc/go-ping"
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
	vpnc *VPNWSClient
)

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

	if vpnc, err = NewVPNWSClient(wsc, tap, *bufSize, *cmdPrev, *cmdNext); err != nil {
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
