(function () {
  "use strict";

  var NAME = "gosx-admin/mdpp-editor";
  var PROTOCOL = 1;

  function byId(root, suffix) { return root.querySelector("#" + CSS.escape(root.id + suffix)); }
  function makeKey() {
    var bytes = new Uint8Array(16);
    if (window.crypto && window.crypto.getRandomValues) window.crypto.getRandomValues(bytes);
    else for (var i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
    return Array.from(bytes, function (v) { return v.toString(16).padStart(2, "0"); }).join("");
  }
  function formData(values) {
    var body = new URLSearchParams();
    Object.keys(values).forEach(function (key) { body.set(key, values[key] == null ? "" : String(values[key])); });
    return body;
  }
  function text(node, value) { if (node) node.textContent = value; }
  function clamp(n, length) { return Math.max(0, Math.min(length, Number(n) || 0)); }

  function register(context) {
    function mount(root, ctx) {
      if (!root || root.getAttribute("data-gosx-runtime-surface") !== NAME) return { dispose: function () {} };
      var configNode = document.getElementById(root.getAttribute("data-gxa-config"));
      var cfg = {};
      try { cfg = JSON.parse(configNode ? configNode.textContent : "{}"); } catch (_) {}
      var form = root.querySelector(".gxa-editor__form");
      var source = root.querySelector("textarea[name='gxa_source']");
      var statusNode = root.querySelector(".gxa-editor__saved");
      var live = root.querySelector(".gxa-editor__live");
      var problemList = root.querySelector(".gxa-editor__problem-list");
      var cueLayer = root.querySelector(".gxa-editor__cues");
      var frame = root.querySelector(".gxa-editor__frame");
      var saveButton = root.querySelector("button[type='submit']:not([name])");
      var csrf = form && form.querySelector("[name='csrf_token']");
      var revisionInput = form && form.querySelector("[name='gxa_revision']");
      var keyInput = form && form.querySelector("[name='gxa_key']");
      if (!form || !source) return { dispose: function () {} };
      var labels = cfg.labels || {};
      var revision = Number(cfg.revision) || 0;
      var lastSavedSource = source.value;
      var saveTimer = null, previewTimer = null, backupTimer = null, retryTimer = null, workerTimer = null;
      var saving = false, queued = false, conflict = !!root.querySelector("[data-gxa-conflict]");
      var dirty = false, pending = null, retryIndex = 0, disposed = false;
      var target = cfg.targets && cfg.targets[0] || "site";
      var worker = null, workerReady = false, workerFailed = false, workerID = 0, latestWorkerID = 0;
      var latestAnalysis = null;
      var backupKey = "gxa-editor:" + cfg.draftID;

      function cancel(key, handle) {
        if (handle && typeof handle === "function") handle();
        if (ctx && ctx.cancelSchedule && key) ctx.cancelSchedule(key);
      }
      function schedule(key, callback, delay) {
        if (ctx && typeof ctx.schedule === "function") return ctx.schedule(key, callback, delay);
        return window.setTimeout(callback, delay);
      }
      function listen(targetNode, type, handler, options) {
        if (ctx && typeof ctx.listen === "function") return ctx.listen(targetNode, type, handler, options);
        targetNode.addEventListener(type, handler, options);
        return function () { targetNode.removeEventListener(type, handler, options); };
      }
      function requestLatest(key, url, options) {
        if (ctx && typeof ctx.requestLatest === "function") return ctx.requestLatest(key, url, options);
        return fetch(url, options);
      }
      function say(message) { text(live, message); }
      function status(message, speak) { text(statusNode, message); if (speak) say(message); }
      function updateFormRevision() { if (revisionInput) revisionInput.value = String(revision); }
      function updateKey() { if (keyInput) keyInput.value = makeKey(); }
      function csrfHeaders() { return { "Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8", "X-CSRF-Token": csrf ? csrf.value : "" }; }
      function errorMessage() { return "We could not save this yet. Please try again."; }
      function localBackup() {
        if (!cfg.localBackup) return;
        cancel("backup", backupTimer);
        backupTimer = schedule("backup", function () {
          try { localStorage.setItem(backupKey, JSON.stringify({ revision: revision, source: source.value, at: Date.now() })); } catch (_) {}
        }, 500);
      }
      function removeBackupIfSaved(savedSource) {
        if (!cfg.localBackup) return;
        try {
          var raw = localStorage.getItem(backupKey);
          if (raw && JSON.parse(raw).source === savedSource) localStorage.removeItem(backupKey);
        } catch (_) {}
      }
      function restoreText(value) {
        source.focus(); source.select();
        var inserted = false;
        try { inserted = typeof document.execCommand === "function" && document.execCommand("insertText", false, value); } catch (_) {}
        if (!inserted) source.setRangeText(value, 0, source.value.length, "end");
        source.dispatchEvent(new Event("input", { bubbles: true }));
      }
      function setDirty() {
        dirty = source.value !== lastSavedSource;
        if (dirty) status(labels.unsaved || "Not saved yet", false);
      }

      function showConflict(saved, conflictVersion) {
        conflict = true; pending = null; queued = false;
        var panel = root.querySelector("[data-gxa-conflict]");
        if (!panel) {
          panel = document.createElement("section");
          panel.className = "gxa-editor__conflict"; panel.setAttribute("role", "alert");
          panel.setAttribute("aria-labelledby", root.id + "-conflict-title"); panel.setAttribute("data-gxa-conflict", "true");
          var h = document.createElement("h3"); h.id = root.id + "-conflict-title"; h.textContent = "This draft changed somewhere else";
          var p = document.createElement("p");
          var label = document.createElement("label"); label.htmlFor = root.id + "-theirs"; label.textContent = "Saved text";
          var theirs = document.createElement("textarea"); theirs.id = root.id + "-theirs"; theirs.readOnly = true; theirs.rows = 8;
          var native = document.createElement("p"); native.className = "gxa-editor__conflict-native"; native.textContent = "Saving now makes your text the new version.";
          var actions = document.createElement("div"); actions.className = "gxa-editor__conflict-actions"; actions.hidden = true;
          var mine = document.createElement("button"); mine.type = "button"; mine.dataset.gxaResolve = "mine"; mine.textContent = labels.keepMine || "Save my text as the new version";
          var use = document.createElement("button"); use.type = "button"; use.dataset.gxaResolve = "theirs"; use.textContent = labels.useTheirs || "Replace my text with the saved version";
          actions.append(mine, use); panel.append(h, p, label, theirs, native, actions);
          var liveAt = root.querySelector(".gxa-editor__live");
          form.insertBefore(panel, liveAt ? liveAt.nextSibling : form.firstChild);
        }
        var savedText = saved && saved.source || "";
        var theirText = panel.querySelector("textarea");
        if (theirText) { theirText.value = savedText; theirText.dataset.revision = String(saved && saved.revision || revision); }
        panel.dataset.revision = String(saved && saved.revision || revision);
        var pnode = panel.querySelector("p");
        if (pnode) pnode.textContent = "Your text is still in the editor and is not saved. The saved version is revision " + (saved && saved.revision || revision) + (saved && saved.updatedBy ? ", by " + saved.updatedBy : "") + ".";
        var nativeCopy = panel.querySelector(".gxa-editor__conflict-native");
        var actions = panel.querySelector(".gxa-editor__conflict-actions");
        if (nativeCopy) nativeCopy.hidden = true;
        if (actions) actions.hidden = false;
        revision = Number(saved && saved.revision) || revision; updateFormRevision(); updateKey();
        status(labels.conflict || "Not saved: this draft changed somewhere else. Compare the two versions.", false);
        say(labels.conflict || "Not saved: this draft changed somewhere else. Compare the two versions.");
        bindConflict(panel);
        if (conflictVersion) panel.dataset.conflictVersion = conflictVersion;
      }
      function clearConflict() {
        var panel = root.querySelector("[data-gxa-conflict]");
        if (panel) panel.remove(); conflict = false;
      }
      function resolveConflict(name, panel) {
        if (name === "mine") {
          clearConflict();
          revision = Number(panel && panel.querySelector("textarea") && panel.querySelector("textarea").dataset.revision) || revision;
          updateFormRevision(); dirty = true; pending = null; save(false, true);
        } else if (name === "theirs" && window.confirm("Replace your text with the saved version?")) {
          var theirs = panel && panel.querySelector("textarea");
          revision = Number((panel && panel.dataset.revision) || revision); updateFormRevision(); clearConflict();
          restoreText(theirs ? theirs.value : ""); lastSavedSource = source.value; dirty = false;
          status(labels.saved ? labels.saved.replace("%s", new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })) : "Saved.", true);
        }
      }
      function bindConflict(panel) {
        panel.querySelectorAll("[data-gxa-resolve]").forEach(function (button) {
          if (button.dataset.gxaBound) return;
          button.dataset.gxaBound = "true";
          listen(button, "click", function (event) {
            event.preventDefault(); event.stopPropagation(); resolveConflict(button.dataset.gxaResolve, panel);
          });
        });
      }
      function handleSaveData(data, manual, savedSource) {
        data = data || {};
        if (data.conflict) { showConflict(data.conflict, data.conflictVersion); return false; }
        if (data.revision) revision = Number(data.revision);
        updateFormRevision(); lastSavedSource = savedSource; dirty = source.value !== savedSource;
        pending = null; retryIndex = 0;
        var stamp = data.savedAt ? new Date(data.savedAt) : new Date();
        var savedText = (labels.saved || "Saved at %s").replace("%s", stamp.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
        status(savedText, manual);
        removeBackupIfSaved(savedSource);
        if (Array.isArray(data.diagnostics)) setAnalysis({ diagnostics: data.diagnostics, spans: latestAnalysis && latestAnalysis.spans || [], headings: [], links: [], words: 0 });
        return true;
      }
      function parseResponse(response) {
        return response.json().catch(function () { return {}; }).then(function (body) { return { response: response, body: body || {}, data: body && body.data || {} }; });
      }
      function retryLater() {
        var waits = [5000, 10000, 20000, 40000, 60000];
        var wait = waits[Math.min(retryIndex, waits.length - 1)]; retryIndex++;
        cancel("retry", retryTimer);
        retryTimer = schedule("retry", function () { if (pending && !conflict) sendSave(pending, false); }, wait);
      }
      function save(automatic, manual) {
        if (conflict || !dirty || disposed) return Promise.resolve(false);
        if (saving) { queued = true; return Promise.resolve(false); }
        if (pending && pending.source === source.value) return sendSave(pending, manual);
        pending = { draft: cfg.draftID, source: source.value, revision: revision, key: keyInput && keyInput.value || makeKey(), autosave: automatic ? "1" : "" };
        updateKey();
        pending.key = keyInput ? keyInput.value : pending.key;
        return sendSave(pending, manual);
      }
      function sendSave(payload, manual) {
        if (saving || conflict || !payload) return Promise.resolve(false);
        saving = true; status(labels.saving || "Saving…", false);
        var values = { csrf_token: csrf && csrf.value, gxa_draft: payload.draft, gxa_source: payload.source, gxa_revision: payload.revision, gxa_key: payload.key, gxa_autosave: payload.autosave, __gosx_return_to: form.querySelector("[name='__gosx_return_to']") && form.querySelector("[name='__gosx_return_to']").value };
        var request = requestLatest("save", cfg.saveURL || form.action, { method: "POST", headers: csrfHeaders(), body: formData(values), credentials: "same-origin" });
        return Promise.resolve(request).then(parseResponse).then(function (result) {
          var response = result.response, body = result.body, data = result.data;
          if (response.status === 409 || data.conflict) { showConflict(data.conflict || {}, data.conflictVersion); return false; }
          if (!response.ok || body.ok === false) {
            if (response.status >= 500) { status(errorMessage(), false); retryLater(); return false; }
            status(body.message || errorMessage(), manual); return false;
          }
          var saved = handleSaveData(data, manual, payload.source);
          if (saved) updateKey();
          return saved;
        }).catch(function () {
          status(labels.offline || "Not saved: you are offline. Your text is kept on this device.", false);
          if (!navigator.onLine) say(labels.offline || "Not saved: you are offline. Your text is kept on this device.");
          retryLater(); return false;
        }).finally(function () {
          saving = false;
          if (queued && !conflict) { queued = false; pending = null; save(true, false); }
          else if (source.value !== lastSavedSource) dirty = true;
        });
      }

      function updateProblems(analysis) {
        if (!problemList) return;
        problemList.replaceChildren();
        var diagnostics = analysis && analysis.diagnostics || [];
        diagnostics.forEach(function (d) {
          var li = document.createElement("li"); li.className = "gxa-editor__problem gxa-editor__problem--" + (d.severity || "info");
          var button = document.createElement("button"); button.type = "button"; button.dataset.from = d.from || 0; button.dataset.to = d.to || 0;
          button.textContent = "Line " + (d.line || 1) + ": " + (d.message || "Check this text."); li.appendChild(button); problemList.appendChild(li);
        });
        var count = root.querySelector("#" + CSS.escape(root.id + "-problems-count"));
        if (count) count.textContent = "(" + diagnostics.length + ")";
      }
      function renderCues(analysis) {
        if (!cueLayer) return;
        var spans = (analysis && analysis.spans || []).slice(0, 2000);
        if (source.value.length > (cfg.maxBytes || 100000) || (analysis && analysis.spans && analysis.spans.length > 2000)) { cueLayer.replaceChildren(); return; }
        var marks = spans.map(function (s) { return { from: clamp(s.from, source.value.length), to: clamp(s.to, source.value.length), kind: s.kind }; }).filter(function (s) { return s.to > s.from; });
        (analysis && analysis.diagnostics || []).forEach(function (d) { if (d.severity === "error") marks.push({ from: clamp(d.from, source.value.length), to: clamp(d.to, source.value.length), kind: "problem" }); });
        var cuts = [0, source.value.length]; marks.forEach(function (m) { cuts.push(m.from, m.to); }); cuts.sort(function (a,b) { return a-b; });
        var unique = cuts.filter(function (v, i) { return i === 0 || v !== cuts[i-1]; });
        var fragment = document.createDocumentFragment();
        for (var i = 0; i + 1 < unique.length; i++) {
          var from = unique[i], to = unique[i+1]; if (to <= from) continue;
          var active = marks.filter(function (m) { return m.from <= from && m.to >= to; }).sort(function (a,b) { return (b.to-b.from)-(a.to-a.from); });
          var node = document.createTextNode(source.value.slice(from, to));
          for (var j = active.length - 1; j >= 0; j--) { var mark = document.createElement("mark"); mark.className = "gxa-cue gxa-cue--" + active[j].kind; mark.appendChild(node); node = mark; }
          fragment.appendChild(node);
        }
        if (source.value.length === 0) fragment.appendChild(document.createTextNode(""));
        cueLayer.replaceChildren(fragment); cueLayer.scrollTop = source.scrollTop; cueLayer.scrollLeft = source.scrollLeft;
      }
      function setAnalysis(analysis) { latestAnalysis = analysis || {}; renderCues(latestAnalysis); updateProblems(latestAnalysis); }
      function requestPreview() {
        if (disposed) return Promise.resolve();
        var values = { csrf_token: csrf && csrf.value, gxa_draft: cfg.draftID, gxa_source: source.value, gxa_revision: revision, gxa_key: makeKey(), gxa_target: target };
        return requestLatest("preview", cfg.previewURL, { method: "POST", headers: csrfHeaders(), body: formData(values), credentials: "same-origin" }).then(parseResponse).then(function (result) {
          if (!result.response.ok || !result.data) return;
          if (frame && result.data.document) frame.srcdoc = result.data.document;
          var note = root.querySelector(".gxa-editor__changed"); if (note) note.hidden = !result.data.changed;
          if (saveButton) saveButton.textContent = result.data.blocked ? "Blocked: fix the problems first" : (labels.save || "Save");
          if (workerFailed || !worker || !workerReady) setAnalysis(result.data.analysis || {});
        }).catch(function () {});
      }
      function schedulePreview() { cancel("preview", previewTimer); previewTimer = schedule("preview", requestPreview, cfg.previewMs || 800); }
      function scheduleSave() {
        if (conflict) return;
        if (saving) { queued = true; return; }
        cancel("save", saveTimer); saveTimer = schedule("save", function () { save(true, false); }, cfg.autosaveMs || 2000);
      }
      function workerFallback() {
        if (workerFailed) return;
        workerFailed = true; workerReady = false;
        if (worker) { try { worker.terminate(); } catch (_) {} worker = null; }
        if (problemList) { var hint = root.querySelector(".gxa-editor__fallback"); if (hint) hint.hidden = false; }
        requestPreview();
      }
      function startWorker() {
        if (!cfg.worker || workerFailed) return;
        if (typeof Worker !== "function" || typeof WebAssembly !== "object") { workerFallback(); return; }
        try { worker = new Worker(cfg.worker.scriptURL); } catch (_) { workerFallback(); return; }
        var readyTimer = schedule("worker-ready", workerFallback, 10000);
        worker.onmessage = function (event) {
          var m = event.data || {};
          if (m.protocol !== PROTOCOL) { workerFallback(); return; }
          if (m.type === "ready") { cancel("worker-ready", readyTimer); workerReady = true; analyzeWorker(); return; }
          if (m.type === "error") { if (m.code === "wasm-load" || m.code === "protocol") workerFallback(); return; }
          if (m.type === "result" && m.id === latestWorkerID && m.analysis) { setAnalysis(m.analysis); }
        };
        worker.onerror = workerFallback; worker.onmessageerror = workerFallback;
        var init = function () { if (worker) worker.postMessage({ type: "init", protocol: PROTOCOL, wasmURL: cfg.worker.wasmURL, execURL: cfg.worker.execURL, required: cfg.required || [], maxBytes: cfg.maxBytes || 100000 }); };
        if (typeof requestIdleCallback === "function") requestIdleCallback(init); else workerTimer = schedule("worker-init", init, 500);
      }
      function analyzeWorker() {
        if (!worker || !workerReady || workerFailed) return;
        cancel("worker-analysis", workerTimer);
        workerTimer = schedule("worker-analysis", function () {
          var id = ++workerID; latestWorkerID = id;
          worker.postMessage({ type: "analyze", protocol: PROTOCOL, id: id, source: source.value });
        }, 150);
      }
      function switchPane(name) {
        root.dataset.gxaPane = name;
        root.querySelectorAll("[data-gxa-pane]").forEach(function (button) { button.setAttribute("aria-pressed", String(button.dataset.gxaPane === name)); });
      }
      function checkWidth() {
        var switcher = root.querySelector(".gxa-editor__switch");
        if (!switcher) return;
        var size = parseFloat(getComputedStyle(root).fontSize) || 16;
        var narrow = root.getBoundingClientRect().width < size * 48;
        switcher.hidden = !narrow;
        if (!narrow) root.removeAttribute("data-gxa-pane");
        else switchPane(root.dataset.gxaPane || "write");
      }
      function onInput() {
        setDirty(); localBackup(); scheduleSave(); schedulePreview(); analyzeWorker();
      }
      function beacon() {
        if (!dirty || conflict || !navigator.sendBeacon || !cfg.saveURL) return;
        var payload = formData({ csrf_token: csrf && csrf.value, gxa_draft: cfg.draftID, gxa_source: source.value, gxa_revision: revision, gxa_key: makeKey(), gxa_autosave: "1", __gosx_return_to: form.querySelector("[name='__gosx_return_to']") && form.querySelector("[name='__gosx_return_to']").value });
        try { navigator.sendBeacon(cfg.saveURL, payload); } catch (_) {}
      }
      function restore(versionID) {
        var values = { csrf_token: csrf && csrf.value, gxa_draft: cfg.draftID, gxa_source: source.value, gxa_revision: revision, gxa_key: makeKey(), gxa_version: versionID, __gosx_return_to: form.querySelector("[name='__gosx_return_to']") && form.querySelector("[name='__gosx_return_to']").value };
        status(labels.saving || "Saving…", false);
        return requestLatest("restore", cfg.restoreURL, { method: "POST", headers: csrfHeaders(), body: formData(values), credentials: "same-origin" }).then(parseResponse).then(function (r) {
          if (r.response.status === 409 || r.data.conflict) { showConflict(r.data.conflict || {}); return; }
          if (!r.response.ok || !r.data) { status(r.body.message || errorMessage(), true); return; }
          revision = Number(r.data.revision) || revision; updateFormRevision(); updateKey();
          clearConflict(); restoreText(r.data.source || ""); lastSavedSource = source.value; dirty = false;
          status("Version restored.", true); removeBackupIfSaved(source.value); schedulePreview();
        }).catch(function () { status(errorMessage(), true); });
      }

      listen(source, "input", onInput);
      listen(source, "scroll", function () { if (cueLayer) { cueLayer.scrollTop = source.scrollTop; cueLayer.scrollLeft = source.scrollLeft; } });
      listen(form, "submit", function (event) {
        event.preventDefault();
        var submitter = event.submitter;
        if (submitter && submitter.name === "gxa_version") restore(submitter.value); else save(false, true);
      });
      listen(root, "click", function (event) {
        var pane = event.target.closest && event.target.closest("[data-gxa-pane]");
        if (pane) { switchPane(pane.dataset.gxaPane); return; }
        var targetButton = event.target.closest && event.target.closest("[data-gxa-target]");
        if (targetButton) { target = targetButton.dataset.gxaTarget; root.querySelectorAll("[data-gxa-target]").forEach(function (b) { b.setAttribute("aria-pressed", String(b === targetButton)); }); requestPreview(); return; }
        var snippet = event.target.closest && event.target.closest("[data-gxa-snippet]");
        if (snippet) {
          var found = (cfg.snippets || []).find(function (s) { return s.id === snippet.dataset.gxaSnippet; });
          if (found) { var at = source.selectionStart, body = found.body || "", marker = body.indexOf("${cursor}"), caret = marker >= 0 ? at + marker : at + body.length; body = body.replace("${cursor}", ""); source.focus(); source.setRangeText(body, source.selectionStart, source.selectionEnd, "end"); source.setSelectionRange(caret, caret); source.dispatchEvent(new Event("input", { bubbles: true })); }
          return;
        }
        var problem = event.target.closest && event.target.closest("[data-from][data-to]");
        if (problem) { source.focus(); source.setSelectionRange(Number(problem.dataset.from), Number(problem.dataset.to)); }
      });
      listen(window, "online", function () { if (pending) { cancel("retry", retryTimer); retryTimer = null; sendSave(pending, false); } say("Back online."); });
      listen(window, "offline", function () { status(labels.offline || "Not saved: you are offline. Your text is kept on this device.", false); say(labels.offline || "Not saved: you are offline. Your text is kept on this device."); });
      listen(document, "visibilitychange", function () { if (document.visibilityState === "hidden") beacon(); });
      if (typeof ResizeObserver === "function") { var observer = new ResizeObserver(checkWidth); observer.observe(root); }
      checkWidth();
      root.querySelectorAll(".gxa-editor__snippet-text").forEach(function (pre) { pre.hidden = true; });
      root.querySelectorAll("[data-gxa-snippet]").forEach(function (button) { button.hidden = false; });
      root.querySelectorAll("[data-gxa-conflict]").forEach(function (panel) { var actions = panel.querySelector(".gxa-editor__conflict-actions"); var nativeCopy = panel.querySelector(".gxa-editor__conflict-native"); if (actions) actions.hidden = false; if (nativeCopy) nativeCopy.hidden = true; var theirs = panel.querySelector("textarea"); if (theirs) { theirs.dataset.revision = String(revision); panel.dataset.revision = String(revision); } bindConflict(panel); });
      if (cfg.localBackup) {
        try {
          Object.keys(localStorage).forEach(function (key) { if (key.indexOf("gxa-editor:") !== 0) return; var item = JSON.parse(localStorage.getItem(key) || "{}"); if (!item.at || Date.now() - item.at > 7 * 24 * 60 * 60 * 1000) localStorage.removeItem(key); });
          var backup = JSON.parse(localStorage.getItem(backupKey) || "null");
          if (backup && backup.source !== source.value) {
            var prompt = document.createElement("div"); prompt.className = "gxa-editor__backup";
            var copy = document.createElement("p"); copy.textContent = "Unsaved text from this device was found.";
            var restoreButton = document.createElement("button"); restoreButton.type = "button"; restoreButton.textContent = "Restore it";
            var discardButton = document.createElement("button"); discardButton.type = "button"; discardButton.textContent = "Discard it";
            restoreButton.addEventListener("click", function () { restoreText(backup.source); prompt.remove(); });
            discardButton.addEventListener("click", function () { localStorage.removeItem(backupKey); prompt.remove(); });
            prompt.append(copy, restoreButton, discardButton); form.insertBefore(prompt, form.firstChild);
          }
        } catch (_) {}
      }
      var fallbackHint = root.querySelector(".gxa-editor__fallback"); if (fallbackHint) fallbackHint.hidden = !!cfg.worker;
      startWorker();
      if (!cfg.worker) schedulePreview();

      function dispose() {
        if (disposed) return; disposed = true; beacon();
        ["save", "preview", "backup", "retry", "worker-ready", "worker-init", "worker-analysis"].forEach(function (key) { cancel(key); });
        if (worker) { try { worker.terminate(); } catch (_) {} worker = null; }
      }
      return { dispose: dispose };
    }
    return mount(context && context.root, context || {});
  }

  function registerFactory(api) {
    if (api && typeof api.register === "function") { api.register(NAME, register); return true; }
    return false;
  }
  if (window.__gosx && registerFactory(window.__gosx.runtimeSurfaceAPI)) return;
  if (typeof window.__gosx_register_runtime_surface === "function") { window.__gosx_register_runtime_surface(NAME, register); return; }
  function mountAll() { document.querySelectorAll('[data-gosx-runtime-surface="' + NAME + '"]').forEach(function (root) { register({ root: root, listen: function (n, t, f, o) { n.addEventListener(t, f, o); return function () { n.removeEventListener(t, f, o); }; }, schedule: function (_, f, d) { return setTimeout(f, d); }, requestLatest: function (_, u, o) { return fetch(u, o); } }); }); }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", mountAll, { once: true }); else mountAll();
})();
