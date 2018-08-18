FROM ethereum/cpp-build-env
USER root

RUN git clone --recursive https://github.com/ewasm/hera
# RUN cd hera && cd evmc && git pull origin master && cd .. && cmake -DHERA_DEBUGGING=ON -DBUILD_SHARED_LIBS=ON . && make -j8 && mv src/libhera.so /

ARG NODEID

# RUN apk add --no-cache make gcc musl-dev linux-headers bash
RUN apt update -y && apt install -y gcc make bash wget vim
RUN wget https://dl.google.com/go/go1.10.3.linux-amd64.tar.gz && tar -xvf go1.10.3.linux-amd64.tar.gz -C /usr/local

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/home/builder/go"

RUN rm /bin/sh && ln -s /bin/bash /bin/sh

ADD . /go-ethereum

RUN cd /go-ethereum && make geth

COPY genesis.json /

# Pull Geth into a second stage deploy alpine container
# FROM alpine:latest

# RUN apk add --no-cache ca-certificates
RUN mv /go-ethereum/build/bin/geth /usr/local/bin/

RUN geth init /genesis.json

# TODO create faucet config dockerfile
# RUN geth account import /faucet-priv.txt --password /pass.txt

ENV NODEID=$NODEID
# ENV EVMC_PATH=/libhera.so

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["/usr/local/bin/geth"]
