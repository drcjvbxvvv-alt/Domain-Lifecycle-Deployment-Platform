package release

import (
	"encoding/json"

	"domain-platform/pkg/agentprotocol"
	"domain-platform/store/postgres"
)

// dispatchItem represents one (domain_task → agent → envelope) assignment
// before it is persisted as an agent_task row.
type dispatchItem struct {
	DomainTaskID int64
	AgentDBID    int64
	AgentID      string  // human-readable agent_id, for logging
	HostGroupID  *int64
	ArtifactID   int64
	ArtifactURL  string
	Envelope     agentprotocol.TaskEnvelope
}

// applyReloadBatching enforces Critical Rule #7:
// "Same host, multiple conf changes from the same release shard → buffer writes,
//  single nginx -s reload."
//
// For each agent that appears more than once in items, all assignments except
// the last one have DeferReload=true (write files, skip reload).  The last
// assignment for that agent gets DeferReload=false so it triggers the reload.
//
// If hg is non-nil and its ReloadBatchSize > 0, the last task in each group of
// ReloadBatchSize also triggers a reload (intermediate batching for very large
// domain counts on a single host).
func applyReloadBatching(items []dispatchItem, hostGroups map[int64]*postgres.HostGroup) []dispatchItem {
	// Group indices by agent DB ID.
	agentIndices := make(map[int64][]int, len(items))
	for i, it := range items {
		agentIndices[it.AgentDBID] = append(agentIndices[it.AgentDBID], i)
	}

	for agentID, indices := range agentIndices {
		if len(indices) <= 1 {
			continue // single task → no batching needed
		}

		// Determine batch size for this agent's host_group.
		batchSize := 0
		if len(indices) > 0 {
			hgID := items[indices[0]].HostGroupID
			if hgID != nil {
				if hg, ok := hostGroups[*hgID]; ok && hg.ReloadBatchSize > 0 {
					batchSize = hg.ReloadBatchSize
				}
			}
		}
		_ = agentID

		for pos, idx := range indices {
			oneBased := pos + 1
			isLast := pos == len(indices)-1
			// Trigger reload on the last task, or at every batchSize boundary.
			doReload := isLast || (batchSize > 0 && oneBased%batchSize == 0)
			items[idx].Envelope.DeferReload = !doReload

			// Re-serialize the modified envelope back into the item.
			// (The caller will JSON-encode it when creating the agent_task row.)
		}
	}
	return items
}

// envelopeJSON serializes a TaskEnvelope to a JSON string for storage.
func envelopeJSON(env agentprotocol.TaskEnvelope) string {
	b, _ := json.Marshal(env)
	return string(b)
}
