#! /usr/bin/env bash

rm -rf .data-miner
./build/bin/geth --datadir .data-miner init genesis.json
