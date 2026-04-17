package planpreview

import (
	"fmt"
	"sort"
)

func list(records []Record) []Record {
	listed := append([]Record(nil), records...)
	sort.Slice(listed, func(i, j int) bool {
		return listed[i].ID < listed[j].ID
	})
	return listed
}

func get(records []Record, id string) (Record, error) {
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}
