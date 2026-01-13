package birdactyl

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	pb "github.com/Birdactyl/Birdactyl-Go-SDK/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Plugin struct {
	id          string
	name        string
	version     string
	events      map[string]EventHandler
	routes      map[string]*RouteConfig
	schedule    map[string]ScheduleHandler
	mixins      []MixinRegistration
	addonTypes  map[string]AddonTypeHandler
	panel       pb.PanelServiceClient
	conn        *grpc.ClientConn
	api         *API
	asyncApi    *AsyncAPI
	dataDir     string
	useDataDir  bool
	onStart     func()
	pending     map[string]chan *pb.PanelMessage
	pendingMu   sync.RWMutex
	ui          *UIBuilder
}

type EventHandler func(Event) EventResult
type RouteHandler func(Request) Response
type ScheduleHandler func()
type AddonTypeHandler func(AddonTypeRequest) AddonTypeResponse

type RouteConfig struct {
	Method           string
	Path             string
	Handler          RouteHandler
	RateLimitPreset  string
	RateLimitRPM     int
	RateLimitBurst   int
}

const (
	PresetRead   = "read"
	PresetWrite  = "write"
	PresetStrict = "strict"
)

func New(id, version string) *Plugin {
	return &Plugin{
		id:         id,
		name:       id,
		version:    version,
		events:     make(map[string]EventHandler),
		routes:     make(map[string]*RouteConfig),
		schedule:   make(map[string]ScheduleHandler),
		mixins:     make([]MixinRegistration, 0),
		addonTypes: make(map[string]AddonTypeHandler),
		pending:    make(map[string]chan *pb.PanelMessage),
		ui:         newUIBuilder(),
	}
}

func (p *Plugin) SetName(name string) *Plugin {
	p.name = name
	return p
}

func (p *Plugin) UseDataDir() *Plugin {
	p.useDataDir = true
	return p
}

func (p *Plugin) OnStart(fn func()) *Plugin {
	p.onStart = fn
	return p
}

func (p *Plugin) OnEvent(eventType string, handler EventHandler) *Plugin {
	p.events[eventType] = handler
	return p
}

func (p *Plugin) Route(method, path string, handler RouteHandler) *RouteBuilder {
	cfg := &RouteConfig{
		Method:  method,
		Path:    path,
		Handler: handler,
	}
	p.routes[method+":"+path] = cfg
	return &RouteBuilder{config: cfg}
}

type RouteBuilder struct {
	config *RouteConfig
}

func (rb *RouteBuilder) RateLimit(requestsPerMinute, burstLimit int) *RouteBuilder {
	rb.config.RateLimitRPM = requestsPerMinute
	rb.config.RateLimitBurst = burstLimit
	return rb
}

func (rb *RouteBuilder) RateLimitPreset(preset string) *RouteBuilder {
	rb.config.RateLimitPreset = preset
	return rb
}

func (p *Plugin) Schedule(id, cron string, handler ScheduleHandler) *Plugin {
	p.schedule[id+":"+cron] = handler
	return p
}

func (p *Plugin) Mixin(target string, handler MixinHandler) *Plugin {
	return p.MixinWithPriority(target, 0, handler)
}

func (p *Plugin) MixinWithPriority(target string, priority int, handler MixinHandler) *Plugin {
	p.mixins = append(p.mixins, MixinRegistration{
		Target:   target,
		Priority: priority,
		Handler:  handler,
	})
	return p
}

func (p *Plugin) AddonType(typeID, name, description string, handler AddonTypeHandler) *Plugin {
	p.addonTypes[typeID] = handler
	return p
}

func (p *Plugin) UI() *UIBuilder {
	return p.ui
}

func (p *Plugin) API() *API {
	return p.api
}

func (p *Plugin) Async() *AsyncAPI {
	return p.asyncApi
}

func (p *Plugin) Log(msg string) {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-plugin-id", p.id)
	p.panel.Log(ctx, &pb.LogRequest{Level: "info", Message: msg})
}

func (p *Plugin) DataDir() string {
	return p.dataDir
}

func (p *Plugin) DataPath(filename string) string {
	return filepath.Join(p.dataDir, filename)
}

func (p *Plugin) SaveConfig(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.DataPath("config.json"), data, 0644)
}

func (p *Plugin) LoadConfig(v interface{}) error {
	data, err := os.ReadFile(p.DataPath("config.json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (p *Plugin) Start(panelAddr string) error {
	if len(os.Args) > 1 {
		panelAddr = os.Args[1]
	}
	if len(os.Args) > 2 {
		p.dataDir = filepath.Join(os.Args[2], p.id+"_data")
	} else {
		p.dataDir = p.id + "_data"
	}

	if p.useDataDir {
		if err := os.MkdirAll(p.dataDir, 0755); err != nil {
			log.Printf("[%s] failed to create data dir %s: %v", p.id, p.dataDir, err)
		}
	}

	conn, err := grpc.NewClient(panelAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx = metadata.AppendToOutgoingContext(ctx, "x-plugin-id", p.id)
			return invoker(ctx, method, req, reply, cc, opts...)
		}),
	)
	if err != nil {
		return err
	}
	p.conn = conn
	p.panel = pb.NewPanelServiceClient(conn)
	p.api = &API{panel: p.panel, pluginID: p.id}
	p.asyncApi = &AsyncAPI{panel: p.panel, pluginID: p.id}

	stream, err := p.panel.Connect(context.Background())
	if err != nil {
		return err
	}

	info := p.buildInfo()
	if err := stream.Send(&pb.PluginMessage{Payload: &pb.PluginMessage_Register{Register: info}}); err != nil {
		return err
	}

	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	if msg.GetRegistered() == nil {
		return err
	}

	log.Printf("[%s] v%s connected to panel", p.id, p.version)

	if p.onStart != nil {
		p.onStart()
	}
	p.Log(p.name + " v" + p.version + " started")

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[%s] stream closed", p.id)
			return nil
		}
		if err != nil {
			log.Printf("[%s] stream error: %v", p.id, err)
			return err
		}
		p.handleMessage(stream, msg)
	}
}

func (p *Plugin) buildInfo() *pb.PluginInfo {
	events := make([]string, 0, len(p.events))
	for e := range p.events {
		events = append(events, e)
	}

	routes := make([]*pb.RouteInfo, 0, len(p.routes))
	for _, cfg := range p.routes {
		route := &pb.RouteInfo{Method: cfg.Method, Path: cfg.Path}
		if cfg.RateLimitPreset != "" || cfg.RateLimitRPM > 0 {
			route.RateLimit = &pb.RateLimitConfig{
				Preset:            cfg.RateLimitPreset,
				RequestsPerMinute: int32(cfg.RateLimitRPM),
				BurstLimit:        int32(cfg.RateLimitBurst),
			}
		}
		routes = append(routes, route)
	}

	schedules := make([]*pb.ScheduleInfo, 0, len(p.schedule))
	for key := range p.schedule {
		id, cron := splitKey(key)
		schedules = append(schedules, &pb.ScheduleInfo{Id: id, Cron: cron})
	}

	mixins := make([]*pb.MixinInfo, 0, len(p.mixins))
	for _, m := range p.mixins {
		mixins = append(mixins, &pb.MixinInfo{Target: m.Target, Priority: int32(m.Priority)})
	}

	addonTypes := make([]*pb.AddonTypeInfo, 0, len(p.addonTypes))
	for typeID := range p.addonTypes {
		addonTypes = append(addonTypes, &pb.AddonTypeInfo{TypeId: typeID})
	}

	return &pb.PluginInfo{
		Id:         p.id,
		Name:       p.name,
		Version:    p.version,
		Events:     events,
		Routes:     routes,
		Schedules:  schedules,
		Mixins:     mixins,
		AddonTypes: addonTypes,
		Ui:         p.ui.build(),
	}
}

func (p *Plugin) handleMessage(stream pb.PanelService_ConnectClient, msg *pb.PanelMessage) {
	var resp *pb.PluginMessage

	switch payload := msg.Payload.(type) {
	case *pb.PanelMessage_Event:
		resp = p.handleEvent(payload.Event)
	case *pb.PanelMessage_Http:
		resp = p.handleHTTP(payload.Http)
	case *pb.PanelMessage_Schedule:
		resp = p.handleSchedule(payload.Schedule)
	case *pb.PanelMessage_Mixin:
		resp = p.handleMixin(payload.Mixin)
	case *pb.PanelMessage_AddonType:
		resp = p.handleAddonType(payload.AddonType)
	case *pb.PanelMessage_Shutdown:
		log.Printf("[%s] shutdown requested", p.id)
		os.Exit(0)
	default:
		return
	}

	if resp != nil {
		resp.RequestId = msg.RequestId
		stream.Send(resp)
	}
}

func (p *Plugin) handleEvent(ev *pb.Event) *pb.PluginMessage {
	handler, ok := p.events[ev.Type]
	if !ok {
		return &pb.PluginMessage{Payload: &pb.PluginMessage_EventResponse{EventResponse: &pb.EventResponse{Allow: true}}}
	}
	result := handler(Event{Type: ev.Type, Data: ev.Data, Sync: ev.Sync})
	return &pb.PluginMessage{Payload: &pb.PluginMessage_EventResponse{EventResponse: &pb.EventResponse{Allow: result.allow, Message: result.message}}}
}

func (p *Plugin) handleHTTP(req *pb.HTTPRequest) *pb.PluginMessage {
	cfg, ok := p.routes[req.Method+":"+req.Path]
	if !ok {
		for _, c := range p.routes {
			if (c.Method == "*" || c.Method == req.Method) && matchPath(c.Path, req.Path) {
				cfg = c
				break
			}
		}
	}
	if cfg == nil {
		return &pb.PluginMessage{Payload: &pb.PluginMessage_HttpResponse{HttpResponse: errorResponse(404, "not found")}}
	}

	var body map[string]interface{}
	json.Unmarshal(req.Body, &body)

	resp := cfg.Handler(Request{
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Query:   req.Query,
		Body:    body,
		RawBody: req.Body,
		UserID:  req.UserId,
	})

	return &pb.PluginMessage{Payload: &pb.PluginMessage_HttpResponse{HttpResponse: &pb.HTTPResponse{
		Status:  int32(resp.Status),
		Headers: resp.Headers,
		Body:    resp.body,
	}}}
}

func (p *Plugin) handleSchedule(req *pb.ScheduleRequest) *pb.PluginMessage {
	for key, handler := range p.schedule {
		id, _ := splitKey(key)
		if id == req.ScheduleId {
			handler()
			break
		}
	}
	return &pb.PluginMessage{Payload: &pb.PluginMessage_ScheduleResponse{ScheduleResponse: &pb.Empty{}}}
}

func (p *Plugin) handleMixin(req *pb.MixinRequest) *pb.PluginMessage {
	var handler MixinHandler
	for _, m := range p.mixins {
		if m.Target == req.Target {
			handler = m.Handler
			break
		}
	}

	if handler == nil {
		return &pb.PluginMessage{Payload: &pb.PluginMessage_MixinResponse{MixinResponse: &pb.MixinResponse{Action: pb.MixinResponse_NEXT}}}
	}

	var input map[string]interface{}
	json.Unmarshal(req.Input, &input)

	var chainData map[string]interface{}
	if len(req.ChainData) > 0 {
		json.Unmarshal(req.ChainData, &chainData)
	}

	mctx := &MixinContext{
		Target:    req.Target,
		RequestID: req.RequestId,
		input:     input,
		chainData: chainData,
	}

	result := handler(mctx)

	resp := &pb.MixinResponse{
		Action: pb.MixinResponse_Action(result.action),
	}

	if result.output != nil {
		resp.Output, _ = json.Marshal(result.output)
	}
	if result.err != "" {
		resp.Error = result.err
	}
	if result.modifiedInput != nil {
		resp.ModifiedInput, _ = json.Marshal(result.modifiedInput)
	}
	for _, n := range result.notifications {
		resp.Notifications = append(resp.Notifications, &pb.Notification{
			Title:   n.Title,
			Message: n.Message,
			Type:    n.Type,
		})
	}

	return &pb.PluginMessage{Payload: &pb.PluginMessage_MixinResponse{MixinResponse: resp}}
}

func (p *Plugin) handleAddonType(req *pb.AddonTypeRequest) *pb.PluginMessage {
	handler, ok := p.addonTypes[req.TypeId]
	if !ok {
		return &pb.PluginMessage{Payload: &pb.PluginMessage_AddonTypeResponse{AddonTypeResponse: &pb.AddonTypeResponse{
			Success: false,
			Error:   "addon type handler not found",
		}}}
	}

	addonReq := AddonTypeRequest{
		TypeID:          req.TypeId,
		ServerID:        req.ServerId,
		NodeID:          req.NodeId,
		DownloadURL:     req.DownloadUrl,
		FileName:        req.FileName,
		InstallPath:     req.InstallPath,
		SourceInfo:      req.SourceInfo,
		ServerVariables: req.ServerVariables,
	}

	result := handler(addonReq)

	resp := &pb.AddonTypeResponse{
		Success: result.Success,
		Error:   result.Error,
		Message: result.Message,
	}

	for _, action := range result.Actions {
		pbAction := &pb.AddonInstallAction{
			Type:         pb.AddonInstallAction_ActionType(action.Type),
			Url:          action.URL,
			Path:         action.Path,
			Content:      action.Content,
			Command:      action.Command,
			Headers:      action.Headers,
			NodePayload:  action.NodePayload,
			NodeEndpoint: action.NodeEndpoint,
		}
		resp.Actions = append(resp.Actions, pbAction)
	}

	return &pb.PluginMessage{Payload: &pb.PluginMessage_AddonTypeResponse{AddonTypeResponse: resp}}
}

func splitKey(key string) (string, string) {
	for i, c := range key {
		if c == ':' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

func matchPath(pattern, path string) bool {
	if pattern == path {
		return true
	}
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(path) >= len(pattern)-1 && path[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return false
}

func errorResponse(status int, msg string) *pb.HTTPResponse {
	b, _ := json.Marshal(map[string]interface{}{"success": false, "error": msg})
	return &pb.HTTPResponse{Status: int32(status), Headers: map[string]string{"Content-Type": "application/json"}, Body: b}
}
