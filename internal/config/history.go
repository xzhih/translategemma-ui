package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const historyFileName = "history.json"

type HistoryEntry struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	Target    string    `json:"target"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	CreatedAt time.Time `json:"created_at"`
}

type historyDocument struct {
	Version int            `json:"version"`
	NextID  int64          `json:"next_id"`
	Items   []HistoryEntry `json:"items"`
}

func LoadHistory(root string) ([]HistoryEntry, int64, error) {
	path := filepath.Join(root, historyFileName)
	var doc historyDocument
	if err := readJSON(path, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	nextID := doc.NextID
	for _, item := range doc.Items {
		if item.ID > nextID {
			nextID = item.ID
		}
	}
	return doc.Items, nextID, nil
}

func SaveHistory(root string, items []HistoryEntry, nextID int64) error {
	path := filepath.Join(root, historyFileName)
	doc := historyDocument{
		Version: 1,
		NextID:  nextID,
		Items:   items,
	}
	return writeJSONAtomic(path, doc)
}
