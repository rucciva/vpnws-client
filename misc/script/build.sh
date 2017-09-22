#!/bin/bash

package=$1
gopathInferred="$(pwd | grep -o -E '.*/go/')"

if [ "$(echo $GOPATH | grep $gopathInferred | wc -l)" -eq 0 ]; then
    export GOPATH=$GOPATH:$gopathInferred
fi
echo "GOPATH: $GOPATH"
echo "Working Directory : $(pwd)"
echo "package: $1"
go test $package && \
go install $package