package mdppstudio

// WorkerProtocol is the message protocol version. The editor and worker
// refuse a peer with another version and fall back to server lint.
const WorkerProtocol = 1

// WorkerBudgetBytes is the largest allowed gzip (level 9) size of the worker
// .wasm file: 1.5 MiB.
const WorkerBudgetBytes = 1_572_864

// WorkerRequest is a message from the editor to the worker.
//
//	{"type":"init","protocol":1,"wasmURL":"…","execURL":"…","required":["Dates"],"maxBytes":100000}
//	{"type":"analyze","protocol":1,"id":7,"source":"…"}
type WorkerRequest struct {
	// Type is "init" or "analyze".
	Type     string   `json:"type"`
	Protocol int      `json:"protocol"`
	ID       int      `json:"id,omitempty"`
	Source   string   `json:"source,omitempty"`
	WASMURL  string   `json:"wasmURL,omitempty"`
	ExecURL  string   `json:"execURL,omitempty"`
	Required []string `json:"required,omitempty"`
	MaxBytes int      `json:"maxBytes,omitempty"`
}

// WorkerResponse is a message from the worker to the editor.
//
//	{"type":"ready","protocol":1,"engine":"0.4.8"}
//	{"type":"result","protocol":1,"id":7,"millis":12,"analysis":{…}}
//	{"type":"error","protocol":1,"id":7,"code":"wasm-load","message":"…"}
type WorkerResponse struct {
	Type     string    `json:"type"`
	Protocol int       `json:"protocol"`
	ID       int       `json:"id,omitempty"`
	Engine   string    `json:"engine,omitempty"`
	Millis   int       `json:"millis,omitempty"`
	Analysis *Analysis `json:"analysis,omitempty"`
	Code     string    `json:"code,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// WorkerAssets are the URLs the editor needs to start the lint worker. A nil
// *WorkerAssets in EditorConfig means the page makes no worker request.
type WorkerAssets struct {
	ScriptURL string `json:"scriptURL"`
	WASMURL   string `json:"wasmURL"`
	ExecURL   string `json:"execURL"`
}
