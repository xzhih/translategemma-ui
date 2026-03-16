package models

// QuantizedModel is a local representation of a selectable packaged runtime.
type QuantizedModel struct {
	ID          string
	Kind        string
	FileName    string
	Size        string
	SizeBytes   int64
	DownloadURL string
	Recommended bool
}
