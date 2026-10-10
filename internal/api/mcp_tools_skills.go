package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// executeSkillTools executes progressive discovery and skill inspection tools.
func (h *MCPHandler) executeSkillTools(_ context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "airoute_search_skills", "nano_search_skills", "nano_discover_skills":
		if h.repo == nil {
			return "Storage repository not initialized", true, true
		}
		skills, err := h.repo.ListSkills()
		if err != nil {
			return fmt.Sprintf("List skills error: %v", err), true, true
		}
		catFilter, _ := args["category"].(string)
		queryFilter, _ := args["query"].(string)
		queryFilter = strings.ToLower(strings.TrimSpace(queryFilter))

		type SkillSummary struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Category    string   `json:"category"`
			LoadingMode string   `json:"loading_mode"`
			Tools       []string `json:"tools"`
		}
		var list []SkillSummary
		for _, s := range skills {
			if !s.Enabled {
				continue
			}
			if catFilter != "" && s.Category != catFilter {
				continue
			}
			if queryFilter != "" {
				matched := strings.Contains(strings.ToLower(s.Name), queryFilter) ||
					strings.Contains(strings.ToLower(s.Description), queryFilter) ||
					strings.Contains(strings.ToLower(s.ID), queryFilter)
				if !matched {
					for _, t := range s.Tools {
						if strings.Contains(strings.ToLower(t), queryFilter) {
							matched = true
							break
						}
					}
				}
				if !matched {
					continue
				}
			}
			list = append(list, SkillSummary{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				Category:    s.Category,
				LoadingMode: s.LoadingMode,
				Tools:       s.Tools,
			})
		}
		b, _ := json.MarshalIndent(list, "", "  ")
		return string(b), false, true

	case "airoute_inspect_skill", "nano_inspect_skill":
		if h.repo == nil {
			return "Storage repository not initialized", true, true
		}
		skillID, _ := args["skill_id"].(string)
		if skillID == "" {
			return "Error: 'skill_id' is required", true, true
		}
		skill, err := h.repo.GetSkill(skillID)
		if err != nil {
			return fmt.Sprintf("Skill [%s] not found: %v", skillID, err), true, true
		}
		type SkillInspect struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Category    string   `json:"category"`
			Author      string   `json:"author"`
			Version     string   `json:"version"`
			LoadingMode string   `json:"loading_mode"`
			Tools       []string `json:"tools"`
			Enabled     bool     `json:"enabled"`
			Stage       string   `json:"stage_guidance"`
		}
		inspect := SkillInspect{
			ID:          skill.ID,
			Name:        skill.Name,
			Description: skill.Description,
			Category:    skill.Category,
			Author:      skill.Author,
			Version:     skill.Version,
			LoadingMode: skill.LoadingMode,
			Tools:       skill.Tools,
			Enabled:     skill.Enabled,
			Stage:       "Stage 2 Confirmed. Call 'airoute_get_skill_manifest' with skill_id to fetch full prompt instructions and schemas.",
		}
		b, _ := json.MarshalIndent(inspect, "", "  ")
		return string(b), false, true

	case "airoute_get_skill_manifest", "nano_get_skill_manifest":
		if h.repo == nil {
			return "Storage repository not initialized", true, true
		}
		skillID, _ := args["skill_id"].(string)
		if skillID == "" {
			return "Error: 'skill_id' is required", true, true
		}
		skill, err := h.repo.GetSkill(skillID)
		if err != nil {
			return fmt.Sprintf("Skill [%s] not found: %v", skillID, err), true, true
		}
		if skill.Manifest != "" {
			return skill.Manifest, false, true
		}
		b, _ := json.MarshalIndent(skill, "", "  ")
		return string(b), false, true
	}

	return "", false, false
}
