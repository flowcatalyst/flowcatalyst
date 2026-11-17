#!/bin/sh
# The subset that means something under emulation (amd64 image on an arm64 host):
# equivalence, deadlines, the GC pause, memory, threads. Timings are emulated.
set -u
A=amd64; I=fc-wtspike:$A; R=results; mkdir -p $R
run() { name=$1; shift; echo "== $name $*" >&2; docker run --rm --platform linux/$A $I "$@" > $R/$name-linux-$A-emulated.txt 2>&1; }
run equiv -mode equiv -engines wazero,wasmtime,wt-armed,wt-noep,wt-map
run runaway -mode runaway -engines wasmtime,wt-armed -rounds 3
run runaway-wazero -mode runaway -engines wazero -rounds 1 -cases js-loop,js-bigint,js-recursion,go-loop
run gcpause -mode gcpause -engines wasmtime -rounds 3
run mem -mode mem -engines wazero,wasmtime -rounds 2 -cap 67108864
run migrate -mode migrate -engines wazero,wasmtime
run idle -mode idle -engines wazero,wasmtime
run lat -mode lat -engines wazero,wasmtime -passes 1 -runs 3 -dur 1s -w 1,3,5
run hostcost -mode hostcost -engines wazero,wasmtime -passes 1 -runs 3 -dur 1s
echo done >&2
