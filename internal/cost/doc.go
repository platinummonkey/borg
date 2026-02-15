// Package cost provides cost tracking and aggregation for LLM usage
// reported by agents via the COST-REPORT protocol action.
//
// The [CostStore] records individual cost entries and provides aggregation
// by agent, task, and model. The [LLMAdapter] interface abstracts provider-
// specific cost calculation and reporting.
package cost
