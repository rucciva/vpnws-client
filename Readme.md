# Golang Websocket VPN Client

## Linux With Docker

### Build

```bash
cd $path_to_where_you_clone_this_repository &&\
docker run \
    -it \
    --name vpnwsc-builder \
    -v "$PWD":/go/src/git.rucciva.one/rucciva/vpnws-client \
    -v "$PWD"/bin:/go/bin \
    -w /go/src/git.rucciva.one/rucciva/vpnws-client  \
    golang:1.9 \
    sh -c 'go get -u github.com/golang/dep/cmd/dep && dep ensure  && go install -v git.rucciva.one/rucciva/vpnws-client/cmd/vpnwsc'
```

### Run Example

#### Two-Way SSL + HTTP-Basic-Auth Websocket Server

Make sure you are in the same directory when you run the build command above, then run:

```bash
sudo ./bin/vpnwsc\
    -i tap\
    --buf-size=1526 \
    -u ${http_basic_username} \
    -p ${http_basic_password} \
    --pkcs12-file=$path_to_ssl_client_certificate \
    --pkcs12-file-pass=$ssl_client_certificate_password \
    --cmd-after-connect 'ifconfig {{.dev}} up && dhclient {{.dev}} && ifconfig {{.dev}} mtu 1500' \
    wss://${server_endpoint}${server_path}
```

**nb**: the appropriate buffer size (--buf-size) depends on your network

## OSX

### Build

1. install golang <https://golang.org/dl/>
1. set your *$GOPATH* <https://github.com/golang/go/wiki/SettingGOPATH#unix-systems>
1. install dep

    ```bash
    go get -u github.com/golang/dep/cmd/dep
    ```

1. move this repository into *$GOPATH*/git.rucciva.one/rucciva/vpnws-client
1. run:

    ```bash
    cd $GOPATH/git.rucciva.one/rucciva/vpnws-client &&\
    dep ensure  &&\
    go install -v git.rucciva.one/rucciva/vpnws-client/cmd/vpnwsc
    ```

### Run Example

#### Two-Way SSL + HTTP-Basic-Auth Websocket Server

```bash
sudo $GOPATH/bin/vpnwsc \
    -i tap \
    --buf-size=1526 \
    -u ${http_basic_username} \
    -p ${http_basic_password} \
    --pkcs12-file=$path_to_ssl_client_certificate \
    --pkcs12-file-pass=$ssl_client_certificate_password \
    --cmd-before-connect 'ipconfig set {{.dev}} DHCP && ifconfig {{.dev}} mtu 1500' \
    --cmd-after-disconnect 'ipconfig set {{.dev}} BOOTP' \
    wss://${server_endpoint}${server_path}
```

**nb**: the appropriate buffer size (--buf-size) depends on your network