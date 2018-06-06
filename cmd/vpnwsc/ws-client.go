package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io/ioutil"

	"golang.org/x/crypto/pkcs12"
	"golang.org/x/net/websocket"
)

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
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
		// ignoring wsc error
		return nil
	default:
		return err
	}
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

type WSClient struct {
	URL                string
	Origin             string
	Username           string
	Password           string
	PKCS12File         string
	PKCS12FilePassword string

	SkipVerifyServer bool
	SkipVerifyClient bool

	ws *websocket.Conn

	*RWTimer
}

func NewWSClient(rw *RWTimer) (c *WSClient, err error) {
	c = new(WSClient)
	c.RWTimer = rw
	return c, err
}

func (wsc *WSClient) Open() (err error) {
	if wsc == nil {
		return ErrNil
	}
	if wsc.RWTimer == nil {
		return ErrNil
	}
	conf, err := websocket.NewConfig(wsc.URL, wsc.Origin)
	if err != nil {
		return err
	}
	conf.Header.Add("Authorization", "Basic "+basicAuth(wsc.Username, wsc.Password))
	if wsc.PKCS12File == "" {
		return ErrInvalidCertificate
	}
	cert, err := loadCertificatePair(wsc.PKCS12File, wsc.PKCS12FilePassword)
	if err != nil && cert == nil {
		return err
	}
	if err != nil && cert != nil && !wsc.SkipVerifyClient {
		return err
	}
	conf.TlsConfig = &tls.Config{
		InsecureSkipVerify: wsc.SkipVerifyServer,
		Certificates: []tls.Certificate{
			*cert,
		},
	}
	wsc.ws, err = websocket.DialConfig(conf)
	return
}

func (wsc *WSClient) Close() error {
	if wsc == nil || wsc.ws == nil {
		return nil
	}
	if err := wsc.ws.Close(); err != nil {
		return err
	}
	wsc.ws = nil
	return nil
}

func (wsc *WSClient) Read(p []byte) (n int, err error) {
	if wsc == nil || wsc.ws == nil {
		return 0, ErrNil
	}
	// log.Println("websocket read started")
	n, err = wsc.ws.Read(p)
	// log.Println("websocket read finished")
	return
}

func (wsc *WSClient) Write(p []byte) (n int, err error) {
	if wsc == nil || wsc.ws == nil {
		return 0, ErrNil
	}
	// log.Println("websocket write started")
	n, err = wsc.ws.Write(p)
	// log.Println("websocket write finished")
	return
}
