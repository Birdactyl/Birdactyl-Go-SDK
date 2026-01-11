package birdactyl

import "encoding/json"

type Event struct {
	Type string
	Data map[string]string
	Sync bool
}

type EventResult struct {
	allow   bool
	message string
}

func Allow() EventResult {
	return EventResult{allow: true}
}

func Block(message string) EventResult {
	return EventResult{allow: false, message: message}
}

type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Query   map[string]string
	Body    map[string]interface{}
	RawBody []byte
	UserID  string
}

type Response struct {
	Status  int
	Headers map[string]string
	body    []byte
}

func JSON(data interface{}) Response {
	b, _ := json.Marshal(map[string]interface{}{"success": true, "data": data})
	return Response{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}, body: b}
}

func Error(status int, msg string) Response {
	b, _ := json.Marshal(map[string]interface{}{"success": false, "error": msg})
	return Response{Status: status, Headers: map[string]string{"Content-Type": "application/json"}, body: b}
}

func Text(text string) Response {
	return Response{Status: 200, Headers: map[string]string{"Content-Type": "text/plain"}, body: []byte(text)}
}

func (r Response) WithStatus(status int) Response {
	r.Status = status
	return r
}

func (r Response) WithHeader(key, value string) Response {
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers[key] = value
	return r
}

type AddonTypeRequest struct {
	TypeID          string
	ServerID        string
	NodeID          string
	DownloadURL     string
	FileName        string
	InstallPath     string
	SourceInfo      map[string]string
	ServerVariables map[string]string
}

type AddonTypeResponse struct {
	Success bool
	Error   string
	Message string
	Actions []AddonInstallAction
}

type AddonInstallAction struct {
	Type         AddonActionType
	URL          string
	Path         string
	Content      []byte
	Command      string
	Headers      map[string]string
	NodePayload  []byte
	NodeEndpoint string
}

type AddonActionType int32

const (
	ActionDownloadFile  AddonActionType = 0
	ActionExtractArchive AddonActionType = 1
	ActionDeleteFile    AddonActionType = 2
	ActionCreateFolder  AddonActionType = 3
	ActionWriteFile     AddonActionType = 4
	ActionRunCommand    AddonActionType = 5
	ActionProxyToNode   AddonActionType = 6
)

func AddonSuccess(message string, actions ...AddonInstallAction) AddonTypeResponse {
	return AddonTypeResponse{Success: true, Message: message, Actions: actions}
}

func AddonError(err string) AddonTypeResponse {
	return AddonTypeResponse{Success: false, Error: err}
}

func DownloadFile(url, path string, headers map[string]string) AddonInstallAction {
	return AddonInstallAction{Type: ActionDownloadFile, URL: url, Path: path, Headers: headers}
}

func ExtractArchive(path string) AddonInstallAction {
	return AddonInstallAction{Type: ActionExtractArchive, Path: path}
}

func DeleteFile(path string) AddonInstallAction {
	return AddonInstallAction{Type: ActionDeleteFile, Path: path}
}

func CreateFolder(path string) AddonInstallAction {
	return AddonInstallAction{Type: ActionCreateFolder, Path: path}
}

func WriteFile(path string, content []byte) AddonInstallAction {
	return AddonInstallAction{Type: ActionWriteFile, Path: path, Content: content}
}

func ProxyToNode(endpoint string, payload []byte) AddonInstallAction {
	return AddonInstallAction{Type: ActionProxyToNode, NodeEndpoint: endpoint, NodePayload: payload}
}
