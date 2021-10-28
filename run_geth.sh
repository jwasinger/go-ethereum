#! /usr/bin/env bash

./build/bin/geth --datadir=.verkle --nodiscover --mine --miner.threads=1 --http --http.corsdomain="*" --http.api="eth,web3" --networkid 86 --unlock=34a600a929c439fcc9fd87bf493fea453add3d5f --password=$(pwd)/vanity-sk-pw.txt --allow-insecure-unlock --miner.etherbase=46894ba29049119a6a79bfebab03949b355d2278 console
