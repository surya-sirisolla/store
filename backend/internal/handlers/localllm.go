package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// localCandidate is a well-known local LLM server we probe for. All expose an
// OpenAI-compatible /v1 API, which is also what the bot uses to talk to them.
type localCandidate struct {
	name    string
	baseURL string // OpenAI-compatible base, e.g. http://host.docker.internal:11434/v1
}

var localCandidates = []localCandidate{
	{name: "Ollama", baseURL: "http://host.docker.internal:11434/v1"},
	{name: "LM Studio", baseURL: "http://host.docker.internal:1234/v1"},
}

type localEndpoint struct {
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url"`
	Models  []string `json:"models"`
}

// DetectLocalLLM probes the host for a running local LLM server (Ollama, LM
// Studio) and returns any it finds along with their available models. Used by
// the AI Providers page to light up the "Local" option only when usable.
func (h *SettingsHandler) DetectLocalLLM(c *gin.Context) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	found := []localEndpoint{}

	for _, cand := range localCandidates {
		models := probeModels(client, cand.baseURL)
		if models == nil {
			continue
		}
		found = append(found, localEndpoint{Name: cand.name, BaseURL: cand.baseURL, Models: models})
	}

	c.JSON(http.StatusOK, gin.H{"available": len(found) > 0, "endpoints": found})
}

// probeModels GETs <base>/models and parses the OpenAI-style {data:[{id}]}
// list. Returns nil if the server isn't reachable or returns nothing.
func probeModels(client *http.Client, base string) []string {
	resp, err := client.Get(base + "/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return nil
	}
	models := []string{}
	for _, m := range parsed.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if len(models) == 0 {
		return nil
	}
	return models
}
