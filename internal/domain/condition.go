package domain

// Condition summarizes a named status fact for a deployment or account.
type Condition struct {
	Type   string
	Status bool
	Reason string
}
