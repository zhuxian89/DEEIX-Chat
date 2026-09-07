package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	domainmcp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/mcp"
)

var ErrCompanionWebUnavailable = errors.New("companion web search is unavailable")

type companionToolLister interface {
	ListTools(context.Context, uint, bool) ([]domainmcp.Tool, error)
}

// CompanionWebToolIDs uses only active Exa search/fetch tools from one server.
// Normal chat execution still resolves credentials, validates URLs and bills calls.
func (s *Service) CompanionWebToolIDs(ctx context.Context) ([]uint, error) {
	if s == nil || s.cfg == nil || !s.cfg.Snapshot().MCPEnable || s.mcpClient == nil || s.mcpRepo == nil {
		return nil, nil
	}
	lister, ok := s.mcpRepo.(companionToolLister)
	if !ok {
		return nil, nil
	}
	servers, err := s.mcpRepo.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if server.Status != "active" {
			continue
		}
		tools, err := lister.ListTools(ctx, server.ID, true)
		if err != nil {
			return nil, err
		}
		var search, fetch uint
		for _, tool := range tools {
			if tool.Status != "active" || tool.ServerID != server.ID {
				continue
			}
			switch strings.TrimSpace(tool.Name) {
			case "web_search_exa":
				search = tool.ID
			case "web_fetch_exa":
				fetch = tool.ID
			}
		}
		if search != 0 {
			ids := []uint{search}
			if fetch != 0 {
				ids = append(ids, fetch)
			}
			return ids, nil
		}
	}
	return nil, nil
}

// SearchCompanionWeb is used for the platform-funded shared topic cache only.
// Callers pass a fixed public category query, never a user's private context.
func (s *Service) SearchCompanionWeb(ctx context.Context, userID uint, query string) (string, error) {
	ids, err := s.CompanionWebToolIDs(ctx)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", ErrCompanionWebUnavailable
	}
	runtime, err := s.resolveSelectedToolRuntime(ctx, ids[:1])
	if err != nil {
		return "", err
	}
	for name, binding := range runtime.mcpBindings {
		if binding.ToolName != "web_search_exa" {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(runtime.schemas[name], &schema) != nil || schema.Properties["query"] == nil {
			return "", ErrCompanionWebUnavailable
		}
		args := map[string]interface{}{"query": query}
		if schema.Properties["numResults"] != nil {
			args["numResults"] = 6
		} else if schema.Properties["num_results"] != nil {
			args["num_results"] = 6
		}
		encoded, _ := json.Marshal(args)
		cfg := binding.Config
		if cfg.TimeoutMS <= 0 || cfg.TimeoutMS > 10_000 {
			cfg.TimeoutMS = 10_000
		}
		output, err := s.mcpClient.CallTool(ctx, cfg, mcp.CallInput{
			ToolName: binding.ToolName, ArgumentsJSON: string(encoded), UserID: userID,
		})
		if err != nil {
			return "", errors.New("companion topic search failed")
		}
		if len(output) > 64*1024 {
			return "", errors.New("companion topic search response is too large")
		}
		return output, nil
	}
	return "", ErrCompanionWebUnavailable
}
