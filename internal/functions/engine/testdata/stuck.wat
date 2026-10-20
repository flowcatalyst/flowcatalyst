;; A cooperative-runtime guest that ignores its deadline: fc_handle spins until
;; the byte at address 0 becomes non-zero (the test releases it). Used to test
;; the watchdog. Build: wasm-opt testdata/stuck.wat -o testdata/stuck.wasm
(module
  (memory (export "memory") 1)
  (func (export "fc_abi_v1"))
  (func (export "fc_alloc") (param i32) (result i32) (i32.const 1024))
  (func (export "fc_handle") (param i32 i32) (result i64)
    (loop $spin (br_if $spin (i32.eqz (i32.load8_u (i32.const 0)))))
    (i64.const 0))
  (func (export "fc_describe") (result i64) (i64.const 0))
  (func (export "fc_js_deadline") (param i64)))
