package core

// KnowledgeVectors is the optional persistent semantic index of a repository.
// Keys are content fingerprints scoped by tenant.
type KnowledgeVectors interface {
	HasKnowledgeVectors(tenant string, keys []string) (map[string]bool, error)
	RankKnowledgeVectors(tenant string, keys []string, additions map[string][]float64, query []float64, limit int) ([]VectorMatch, error)
}

type VectorMatch struct {
	Key   string
	Score float64
}
