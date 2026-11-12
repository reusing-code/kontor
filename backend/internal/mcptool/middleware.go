package mcptool

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	methodToolsCall = "tools/call"
	methodToolsList = "tools/list"
)

func (r *Registrar) spec(name string) (Spec, bool) {
	for _, t := range r.tools {
		if t.spec.Name == name {
			return t.spec, true
		}
	}
	return Spec{}, false
}

func toolErrorResult(e *ToolError) *mcp.CallToolResult {
	res := &mcp.CallToolResult{}
	res.SetError(e)
	return res
}

// guard enforces D7 for one request: tools/list shows only tools whose scope
// the token has and whose module is enabled; tools/call of anything else is a
// tool error rather than a protocol error.
func (r *Registrar) guard(call Call) mcp.Middleware {
	enabled := map[string]bool{}
	moduleEnabled := func(ctx context.Context, moduleID string) (bool, error) {
		if moduleID == "" {
			return true, nil
		}
		if v, ok := enabled[moduleID]; ok {
			return v, nil
		}
		v, err := r.env.Enabled(ctx, call.UserID, moduleID)
		if err != nil {
			return false, err
		}
		enabled[moduleID] = v
		return v, nil
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case methodToolsCall:
				return r.guardCall(ctx, call, moduleEnabled, next, method, req)
			case methodToolsList:
				res, err := next(ctx, method, req)
				if err != nil {
					return res, err
				}
				list, ok := res.(*mcp.ListToolsResult)
				if !ok {
					return res, nil
				}
				return r.filterList(ctx, call, moduleEnabled, list)
			}
			return next(ctx, method, req)
		}
	}
}

func (r *Registrar) guardCall(ctx context.Context, call Call, moduleEnabled func(context.Context, string) (bool, error), next mcp.MethodHandler, method string, req mcp.Request) (mcp.Result, error) {
	ctr, ok := req.(*mcp.CallToolRequest)
	if !ok || ctr.Params == nil {
		return next(ctx, method, req)
	}
	spec, known := r.spec(ctr.Params.Name)
	if !known {
		return next(ctx, method, req)
	}
	if !slices.Contains(call.Scopes, spec.Scope) {
		r.record(spec.Name, call.TokenID, outcomeForbid, time.Duration(0))
		return toolErrorResult(Errorf(CodeInsufficientScope, "the token lacks the %s scope needed for %s", spec.Scope, spec.Name)), nil
	}
	on, err := moduleEnabled(ctx, spec.Module)
	if err != nil {
		r.env.Logger.Error("mcp module check failed", "tool", spec.Name, "tokenId", call.TokenID, "error", err)
		r.record(spec.Name, call.TokenID, outcomeInternal, time.Duration(0))
		return toolErrorResult(internalError()), nil
	}
	if !on {
		r.record(spec.Name, call.TokenID, outcomeForbid, time.Duration(0))
		return toolErrorResult(Errorf(CodeModuleDisabled, "the %s module is disabled", spec.Module)), nil
	}
	return next(ctx, method, req)
}

func (r *Registrar) filterList(ctx context.Context, call Call, moduleEnabled func(context.Context, string) (bool, error), list *mcp.ListToolsResult) (mcp.Result, error) {
	visible := make([]*mcp.Tool, 0, len(list.Tools))
	for _, t := range list.Tools {
		spec, known := r.spec(t.Name)
		if !known || !slices.Contains(call.Scopes, spec.Scope) {
			continue
		}
		on, err := moduleEnabled(ctx, spec.Module)
		if err != nil {
			r.env.Logger.Error("mcp module check failed", "tool", spec.Name, "tokenId", call.TokenID, "error", err)
			return nil, errors.New("internal error")
		}
		if on {
			visible = append(visible, t)
		}
	}
	list.Tools = visible
	return list, nil
}
