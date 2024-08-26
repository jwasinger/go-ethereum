#! /usr/bin/env bash

for i in {0..122}
do
	pairs=$(cat testdata/precompiles/blsG1MultiExp.json| jq -r ".[$i].Input" | python3 -c "import sys; print((len(sys.stdin.read())-1) /160)")
	name=$(cat testdata/precompiles/blsG1MultiExp.json| jq -r ".[$i].Name")
	echo $name, $pairs
done
