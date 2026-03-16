package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func prepareStreamWriter(w http.ResponseWriter) (func(streamEvent) error, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported by server")
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeEvent := func(ev streamEvent) error {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	return writeEvent, nil
}
