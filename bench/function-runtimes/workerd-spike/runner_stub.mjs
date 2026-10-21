import http from "node:http";
let n = 0;
http.createServer((req, res) => { n++; req.resume(); req.on("end", () => { res.writeHead(200, {"content-type":"application/json", "content-length": 2}); res.end("{}"); }); }).listen(19090, "127.0.0.1", () => console.log("stub up"));
