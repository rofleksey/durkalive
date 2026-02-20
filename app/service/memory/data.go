package memory

type Entity struct {
	Name  string   `json:"name"`
	Facts []string `json:"facts"`
}

type KnowledgeGraph struct {
	Entities []*Entity `json:"entities"`
}

type jsonLineItem struct {
	Name  string   `json:"name,omitempty"`
	Facts []string `json:"facts,omitempty"`
}

type AddFactsRequest struct {
	EntityName string   `json:"entityName"`
	Facts      []string `json:"facts"`
}

type DeleteFactsRequest struct {
	EntityName string   `json:"entityName"`
	Facts      []string `json:"facts"`
}
