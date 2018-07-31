FROM golang:1.9 as builder

RUN \
    go get -u github.com/golang/dep/cmd/dep &&\
    go get -u github.com/derekparker/delve/cmd/dlv 

WORKDIR /go/src/git.rucciva.one/rucciva/vpnws-client

# prepare project dependency
ARG MY_PACKAGE 
ENV MY_PACKAGE ${MY_PACKAGE:-change-me}
ENV	MY_WORKDIR $GOPATH/src/$MY_PACKAGE
WORKDIR $MY_WORKDIR
COPY ./Gopkg.toml ./Gopkg.lock ./vendor $GOPATH/src/$MY_PACKAGE/
RUN dep ensure --vendor-only &&\
	mkdir -p /tmp/$GOPATH/src/$MY_PACKAGE/ &&\
	mv $GOPATH/src/$MY_PACKAGE/vendor /tmp/$GOPATH/src/$MY_PACKAGE

# build project
ARG	MY_EXECUTABLE
ENV	MY_EXECUTABLE ${MY_EXECUTABLE:-change-me}
ENV	MY_PACKAGE_EXECUTABLE $MY_PACKAGE/cmd/$MY_EXECUTABLE
ARG MY_ENVIRONMENT
ENV MY_ENVIRONMENT ${MY_ENVIRONMENT:-development}
COPY . $GOPATH/src/$MY_PACKAGE
RUN	rm -rf /go/src/$MY_PACKAGE/vendor &&\
	mv /tmp/$GOPATH/src/$MY_PACKAGE/vendor $GOPATH/src/$MY_PACKAGE &&\
	mv ./misc/docker/$MY_ENVIRONMENT/$MY_EXECUTABLE/assets/build.sh /build.sh &&\
	mv ./misc/docker/$MY_ENVIRONMENT/$MY_EXECUTABLE/assets/entrypoint.sh /entrypoint.sh &&\
    chmod +x /build.sh /entrypoint.sh &&\
	/build.sh

FROM debian:stretch-slim

RUN \
    apt-get update &&\
    apt-get install -y\
		net-tools \
		isc-dhcp-client \
		ca-certificates &&\
	rm -rf /var/lib/apt/lists/*

# prepare executable
ARG MY_PACKAGE 
ENV MY_PACKAGE ${MY_PACKAGE:-change-me}
ARG	MY_EXECUTABLE
ENV	MY_EXECUTABLE ${MY_EXECUTABLE:-change-me}
ENV	MY_PACKAGE_EXECUTABLE $MY_PACKAGE/cmd/$MY_EXECUTABLE
ARG MY_ENVIRONMENT
ENV MY_ENVIRONMENT ${MY_ENVIRONMENT:-change-me}
ENV MY_USER app
ENV MY_GROUP app
COPY --from=builder /go/bin/$MY_EXECUTABLE /usr/local/bin/$MY_EXECUTABLE
COPY ./misc/docker/$MY_ENVIRONMENT/$MY_EXECUTABLE/assets/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
CMD "/bin/bash"