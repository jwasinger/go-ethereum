#! /usr/bin/env bash

./build/bin/geth --datadir=.data-miner --nodiscover --mine --miner.threads=1 --http --http.corsdomain="*" --http.api="eth,web3" --networkid 86 --miner.etherbase=46894ba29049119a6a79bfebab03949b355d2278 --nodekeyhex 0000000000000000000000000000000000000000000000000000000000000001
