#! /bin/bash

go tool pprof --pdf $(pwd)/build/bin/evm $(pwd)/cpu-prof.profile > asdf.pdf
