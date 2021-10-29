#! /usr/bin/env bash

./build/bin/geth --datadir=.data-broadcaster \
	--http \
	--http.corsdomain="*" \
	--http.api="eth,web3" \
	--networkid 86 \
	--port 30304 \
	--bootnodes "enode://79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8@127.0.0.1:30303" \
	--unlock=34a600a929c439fcc9fd87bf493fea453add3d5f \
	--password=$(pwd)/vanity-sk-pw.txt \
	--allow-insecure-unlock \
	--vmodule "p2p=12" \
	console
