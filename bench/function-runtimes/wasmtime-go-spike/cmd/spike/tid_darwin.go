//go:build wasmtime && darwin

package main

// #include <pthread.h>
// #include <stdint.h>
// static uint64_t fc_tid(void) { uint64_t t = 0; pthread_threadid_np(NULL, &t); return t; }
import "C"

func threadID() int { return int(C.fc_tid()) }
