#!/bin/bash

package=$1
gopathInferred="$(pwd | grep -o -E '.*/go/')"

if [ "$(echo $GOPATH | grep $gopathInferred | wc -l)" -eq 0 ]; then
    export GOPATH=$gopathInferred:$GOPATH
fi
echo "GOPATH=$GOPATH"

echo "Working Directory : $(pwd)"

go test $package && \
go install $package