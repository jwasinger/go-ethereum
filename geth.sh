#! /bin/bash
GOPATH=~/go
PATH=$PATH:/usr/local/go/bin

geth --unlock "$(cat /faucet-addr.txt),$(cat /addr.txt)" --password /pass.txt --mine --vmodule 'clique=5,rpc=5' --nodekeyhex "dc90f8f7324f1cc7ba52c4077721c939f98a628ed17e51266d01c9cd0294033$NODEID" --bootnodes "enode://c47a2b406583a2cba9f63b92414126502a4a618014cbb5c5f4937ac1bfb74a6139d46199e4f2e6185db30db2d196e6e893c0e848c77a53cb4d8eba1e40bf6c23@10.0.1.6:30303" --rpc --rpcaddr "0.0.0.0" --syncmode "full" --networkid 66
