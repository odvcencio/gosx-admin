(function () {
  "use strict";
  var protocol = 1, required = [], maxBytes = 100000, ready = false, loading = false;
  function send(message) { self.postMessage(Object.assign({ protocol: protocol }, message)); }
  function fail(code, message, id) { send({ type: "error", code: code, message: message, id: id || undefined }); }
  function utf8Length(value) { return new TextEncoder().encode(value).length; }
  self.onmessage = function (event) {
    var request = event.data || {};
    if (request.protocol !== protocol) { fail("protocol", "Worker protocol mismatch.", request.id); return; }
    if (request.type === "init") {
      if (loading || ready) return;
      loading = true; required = Array.isArray(request.required) ? request.required : [];
      maxBytes = Number(request.maxBytes) > 0 ? Number(request.maxBytes) : 100000;
      try {
        importScripts(request.execURL);
        var go = new Go();
        fetch(request.wasmURL).then(function (response) {
          if (!response.ok) throw new Error("WASM request failed");
          return WebAssembly.instantiateStreaming(response, go.importObject);
        }).then(function (result) { return go.run(result.instance); }).catch(function (error) { fail("wasm-load", error && error.message || "WASM failed to load."); });
      } catch (error) { fail("wasm-load", error && error.message || "WASM failed to load."); }
      return;
    }
    if (request.type !== "analyze") { fail("protocol", "Unknown worker message.", request.id); return; }
    if (!ready && typeof self.mdppAnalyze === "function") ready = true;
    if (utf8Length(request.source || "") > maxBytes) { fail("too-large", "This text is too large to check.", request.id); return; }
    if (!ready) { fail("wasm-load", "The lint worker is not ready.", request.id); return; }
    try {
      var started = Date.now();
      var json = self.mdppAnalyze(request.source || "", JSON.stringify({ required: required, maxBytes: maxBytes }));
      send({ type: "result", id: request.id, millis: Date.now() - started, analysis: JSON.parse(json) });
    } catch (error) { fail("analyze", error && error.message || "Text could not be checked.", request.id); }
  };
})();
