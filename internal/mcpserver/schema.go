package mcpserver

import "github.com/jabronicrab/go-dependable-agent-mcp/internal/preflight"

func checkDependencyInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Operator-approved logical dependency name returned by list_dependencies.",
				"pattern":     `^[a-z][a-z0-9_-]{0,63}$`,
			},
		},
		"required": []string{"name"},
	}
}

func listDependenciesOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"dependencies": map[string]any{
				"type":        "array",
				"description": "Operator-approved logical dependency names and descriptions.",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{
							"type": "string",
						},
						"description": map[string]any{
							"type": "string",
						},
					},
					"required": []string{"name", "description"},
				},
			},
		},
		"required": []string{"dependencies"},
	}
}

func checkDependencyOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"result": readinessResultSchema(),
			"error":  toolFailureOutputSchema(),
		},
		"oneOf": []any{
			map[string]any{"required": []string{"result"}},
			map[string]any{"required": []string{"error"}},
		},
	}
}

func readinessResultSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"dependency": map[string]any{
				"type": "string",
			},
			"status": map[string]any{
				"type": "string",
				"enum": []string{
					string(preflight.StatusReady),
					string(preflight.StatusNotReady),
				},
			},
			"checked_at": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"duration_ms": map[string]any{
				"type":    "integer",
				"minimum": 0,
			},
			"failed_stage": map[string]any{
				"type": "string",
			},
			"error": failureOutputSchema(),
			"stages": map[string]any{
				"type":     "array",
				"minItems": 4,
				"maxItems": 4,
				"items":    stageResultOutputSchema(),
			},
		},
		"required": []string{
			"dependency",
			"status",
			"checked_at",
			"duration_ms",
			"stages",
		},
	}
}

func failureOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"category": map[string]any{
				"type": "string",
			},
			"message": map[string]any{
				"type": "string",
			},
			"http_status": map[string]any{
				"type":    "integer",
				"minimum": 100,
				"maximum": 599,
			},
		},
		"required": []string{"category", "message"},
	}
}

func stageResultOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"status": map[string]any{
				"type": "string",
			},
			"duration_ms": map[string]any{
				"type":    "integer",
				"minimum": 0,
			},
			"reason": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"name", "status", "duration_ms"},
	}
}

func toolFailureOutputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"category": map[string]any{
				"type": "string",
			},
			"message": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"category", "message"},
	}
}
