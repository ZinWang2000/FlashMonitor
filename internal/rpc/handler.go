package rpc

import (
	"encoding/json"
	"fmt"
)

const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
	ErrCodeHTMLNeedsTopN  = -32004
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

func NewErrorResponse(id interface{}, code int, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: msg},
		ID:      id,
	}
}

func NewResultResponse(id interface{}, result interface{}) Response {
	return Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

type Handler struct {
	methods *Methods
}

func NewHandler(m *Methods) *Handler {
	return &Handler{methods: m}
}

func (h *Handler) Handle(data []byte) Response {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return NewErrorResponse(nil, ErrCodeParse, "parse error: "+err.Error())
	}
	if req.JSONRPC != "2.0" {
		return NewErrorResponse(req.ID, ErrCodeInvalidRequest, "jsonrpc must be '2.0'")
	}

	switch req.Method {
	case "initialize":
		return h.methods.Initialize(req)
	case "tools/list":
		return h.methods.ToolsList(req)
	case "tools/call":
		return h.methods.ToolsCall(req)

	case "flash.import":
		return h.methods.Import(req)
	case "flash.listTables":
		return h.methods.ListTables(req)
	case "flash.listAllPackages":
		return h.methods.ListAllPackages(req)
	case "flash.compareAll":
		return h.methods.CompareAll(req)
	case "flash.compareTopN":
		return h.methods.CompareTopN(req)
	case "flash.timeSeries":
		return h.methods.TimeSeries(req)
	case "flash.compareExternal":
		return h.methods.CompareExternal(req)
	case "flash.exportTable":
		return h.methods.ExportTable(req)
	case "flash.deleteTable":
		return h.methods.DeleteTable(req)
	case "flash.getLogs":
		return h.methods.GetLogs(req)

	default:
		return NewErrorResponse(req.ID, ErrCodeMethodNotFound, "method not found: "+req.Method)
	}
}
