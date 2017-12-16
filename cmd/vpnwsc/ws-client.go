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
		// ignoring this error
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
	Url                string
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

func (this *WSClient) Open() (err error) {
	if this == nil {
		return ErrNil
	}
	if this.RWTimer == nil {
		return ErrNil
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
	this.ws, err = websocket.DialConfig(conf)
	return
}

func (this *WSClient) Close() error {
	if this == nil || this.ws == nil {
		return nil
	}
	if err := this.ws.Close(); err != nil {
		return err
	}
	this.ws = nil
	return nil
}

func (this *WSClient) Read(p []byte) (n int, err error) {
	if this == nil || this.ws == nil {
		return 0, ErrNil
	}
	// log.Println("websocket read started")
	n, err = this.ws.Read(p)
	// log.Println("websocket read finished")
	return
}

func (this *WSClient) Write(p []byte) (n int, err error) {
	if this == nil || this.ws == nil {
		return 0, ErrNil
	}
	// log.Println("websocket write started")
	n, err = this.ws.Write(p)
	// log.Println("websocket write finished")
	return
}
