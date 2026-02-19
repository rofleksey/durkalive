package memory

type Entity struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

type Relation struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
}

type KnowledgeGraph struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

type jsonLineItem struct {
	Type         string   `json:"type"`
	Name         string   `json:"name,omitempty"`
	EntityType   string   `json:"entityType,omitempty"`
	Observations []string `json:"observations,omitempty"`
	From         string   `json:"from,omitempty"`
	To           string   `json:"to,omitempty"`
	RelationType string   `json:"relationType,omitempty"`
}

type AddObservationsRequest struct {
	EntityName string   `json:"entityName"`
	Contents   []string `json:"contents"`
}

type AddObservationsResult struct {
	EntityName        string   `json:"entityName"`
	AddedObservations []string `json:"addedObservations"`
}

type DeleteObservationsRequest struct {
	EntityName   string   `json:"entityName"`
	Observations []string `json:"observations"`
}
