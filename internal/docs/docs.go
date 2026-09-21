// Package docs embeds the guidance documents served as MCP prompts.
package docs

import _ "embed"

// BestPractices covers common recipes and the self-critique checklist.
//
//go:embed best_practices.md
var BestPractices string

// IterativeWorkflow covers layer planning and incremental validation.
//
//go:embed iterative_workflow.md
var IterativeWorkflow string
