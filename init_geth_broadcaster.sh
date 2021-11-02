#! /usr/bin/env bash

rm -rf .data-broadcaster

./build/bin/geth --datadir .data-broadcaster init genesis.json
./build/bin/geth --datadir .data-broadcaster account import vanity-sk.txt --password=vanity-sk-pw.txt
./build/bin/geth --datadir .data-broadcaster account import vanity2-sk.txt --password=vanity2-sk-pw.txt
