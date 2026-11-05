package sources

import "fmt"

// Delivery attempts support their originating event. They are attached after
// attribution/inference so destination copies do not vote as extra listeners.
func attachForwardingEvidence(plan *matchPlan) {
	byEvent := map[string]string{}
	for _, r := range plan.Records {
		if r.Values.EventID != "" && r.Values.SupportID == "" && plan.Keys[r.ID] != "" {
			byEvent[fmt.Sprintf("%d:%s", r.SourceID, r.Values.EventID)] = plan.Keys[r.ID]
		}
	}
	plays := map[string]int{}
	for i, p := range plan.Plays {
		plays[p.Key] = i
	}
	for _, r := range plan.Records {
		key := byEvent[fmt.Sprintf("%d:%s", r.SourceID, r.Values.SupportID)]
		index, ok := plays[key]
		if r.Values.SupportID == "" || !ok {
			continue
		}
		plan.Keys[r.ID] = key
		plan.Reasons[r.ID] = "forwarding evidence"
		plan.Plays[index].Records = append(plan.Plays[index].Records, r.ID)
	}
}
