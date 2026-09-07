package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	domainmcp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/mcp"
)

type companionMCPRepo struct {
	servers []domainmcp.Server
	tools   []domainmcp.Tool
}

func (r *companionMCPRepo) ListServers(context.Context) ([]domainmcp.Server, error) {
	return r.servers, nil
}
func (r *companionMCPRepo) GetServer(_ context.Context, id uint) (*domainmcp.Server, error) {
	for i := range r.servers {
		if r.servers[i].ID == id {
			return &r.servers[i], nil
		}
	}
	return nil, errors.New("missing server")
}
func (r *companionMCPRepo) ListTools(_ context.Context, serverID uint, _ bool) ([]domainmcp.Tool, error) {
	var result []domainmcp.Tool
	for _, tool := range r.tools {
		if tool.ServerID == serverID {
			result = append(result, tool)
		}
	}
	return result, nil
}
func (r *companionMCPRepo) ListToolsByIDs(_ context.Context, ids []uint) ([]domainmcp.Tool, error) {
	var result []domainmcp.Tool
	for _, id := range ids {
		for _, tool := range r.tools {
			if tool.ID == id {
				result = append(result, tool)
			}
		}
	}
	return result, nil
}

type companionMCPCaller struct {
	calls  []mcp.CallInput
	config mcp.CallConfig
}

func (c *companionMCPCaller) CallTool(_ context.Context, cfg mcp.CallConfig, input mcp.CallInput) (string, error) {
	c.calls = append(c.calls, input)
	c.config = cfg
	return "Title: a public source", nil
}

func TestCompanionWebSelectsActiveExaOnlyAndUsesDeclaredSchema(t *testing.T) {
	repo := &companionMCPRepo{
		servers: []domainmcp.Server{{ID: 1, Status: "inactive"}, {ID: 2, Status: "active", BaseURL: "https://mcp.exa.ai"}},
		tools: []domainmcp.Tool{
			{ID: 1, ServerID: 1, Name: "web_search_exa", Status: "active"},
			{ID: 2, ServerID: 2, Name: "web_search_exa", Status: "active", InputSchemaJSON: `{"type":"object","properties":{"query":{"type":"string"},"numResults":{"type":"number"}}}`},
			{ID: 3, ServerID: 2, Name: "web_fetch_exa", Status: "active"},
			{ID: 4, ServerID: 2, Name: "delete_records", Status: "active"},
		},
	}
	caller := &companionMCPCaller{}
	service := &Service{cfg: config.NewRuntime(config.Config{MCPEnable: true}), mcpRepo: repo, mcpClient: caller}
	ids, err := service.CompanionWebToolIDs(t.Context())
	if err != nil || !reflect.DeepEqual(ids, []uint{2, 3}) {
		t.Fatalf("unexpected tools: %v %v", ids, err)
	}
	if _, err := service.SearchCompanionWeb(t.Context(), 7, "公开科技新闻"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].ToolName != "web_search_exa" || caller.config.TimeoutMS != 10_000 {
		t.Fatal("wrong search call")
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(caller.calls[0].ArgumentsJSON), &args); err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args["query"] != "公开科技新闻" || args["numResults"] != float64(6) {
		t.Fatalf("query leaked extra context: %v", args)
	}
	repo.tools[1].InputSchemaJSON = `{"type":"object","properties":{"unexpected":{"type":"string"}}}`
	if _, err := service.SearchCompanionWeb(t.Context(), 7, "公开新闻"); !errors.Is(err, ErrCompanionWebUnavailable) || len(caller.calls) != 1 {
		t.Fatal("unsupported tool schema was called")
	}
	service.cfg = config.NewRuntime(config.Config{MCPEnable: false})
	if ids, err := service.CompanionWebToolIDs(t.Context()); err != nil || len(ids) != 0 {
		t.Fatal("disabled MCP remained selected")
	}
}
