#!/bin/sh
# Runs every measurement in the spike image for one platform and writes
# results/<mode>-linux-<arch>.txt. From this directory: ./run-linux.sh arm64
set -u
A=${1:-arm64}
I=fc-wtspike:$A
R=results
mkdir -p $R
run() { name=$1; shift; echo "== $name $*" >&2; docker run --rm --platform linux/$A $I "$@" > $R/$name-linux-$A.txt 2>&1; }
run equiv -mode equiv -engines wazero,wasmtime,wt-armed,wt-noep,wt-map
run runaway -mode runaway -engines wasmtime,wt-armed -rounds 3
run runaway-wazero -mode runaway -engines wazero -rounds 2 -cases js-loop,js-bigint,js-recursion,go-loop
run runaway-wazero-indexof -mode runaway -engines wazero -rounds 1 -cases js-indexof -watch 30s
run gcpause -mode gcpause -engines wasmtime,wt-armed -rounds 3
run gcpause-wazero -mode gcpause -engines wazero -rounds 3 -cases js-loop,js-bigint,go-loop
run gcpause-wazero-indexof -mode gcpause -engines wazero -rounds 1 -cases js-indexof
run mem -mode mem -engines wazero,wasmtime -rounds 3 -cap 67108864
run idle -mode idle -engines wazero,wasmtime,wt-noep
run idle10 -mode idle -engines wasmtime -tick 10ms
run migrate -mode migrate -engines wazero,wasmtime
run lat -mode lat -engines wazero,wasmtime,wt-noep -passes 2 -runs 3 -dur 1s
run hostcost -mode hostcost -engines wazero,wasmtime,wt-map -passes 2 -runs 3 -dur 1s
run conc-w3 -mode conc -engines wazero,wasmtime -guests qjs -cw 3 -conc 1,14,64 -passes 2 -cdur 5s
run conc-w5 -mode conc -engines wazero,wasmtime,wt-map -guests qjs,go -cw 5 -conc 1,14,64 -passes 1 -cdur 4s
echo done >&2
