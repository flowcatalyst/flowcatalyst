set -e
apt-get update -qq >/dev/null 2>&1; apt-get install -y -qq ca-certificates >/dev/null 2>&1
cd /work
npm i workerd >/dev/null 2>&1
node_modules/.bin/workerd --version
uname -m
cat shim_head.js function.js shim_tail2.js > worker2.mjs
node runner_stub.mjs & 
node test.mjs
