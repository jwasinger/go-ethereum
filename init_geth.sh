#! /usr/bin/env bash

rm -rf .verkle

./build/bin/geth --verkle --datadir .verkle init genesis.json
./build/bin/geth --datadir .verkle account import vanity-sk.txt --password=vanity-sk-pw.txt
