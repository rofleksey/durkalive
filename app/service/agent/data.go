package agent

type DurkaResponse struct {
	NewSummary  string   `json:"new_summary"`
	AddFacts    []string `json:"add_facts"`
	RemoveFacts []int    `json:"remove_facts"`
	Response    string   `json:"response"`
}
