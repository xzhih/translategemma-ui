package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"translategemma-ui/internal/models"
	"translategemma-ui/internal/platform"
)

const (
	runtimeRepo        = "xzhih/translategemma-4b-it-llamafile"
	runtimeManifestURL = "https://huggingface.co/" + runtimeRepo + "/resolve/main/manifest-v1.json"
	cacheTTL           = 5 * time.Minute
	listTimeout        = 20 * time.Second
	downloadTimeout    = 90 * time.Minute
	runtimeSubdir      = "runtimes"
)

type manifest struct {
	Models []manifestModel `json:"models"`
}

type manifestModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Recommended bool   `json:"recommended"`
	Features    struct {
		Vision bool `json:"vision"`
	} `json:"features"`
	Runtime struct {
		Llamafile struct {
			FileName   string `json:"file_name"`
			PathInRepo string `json:"path_in_repo"`
			SizeBytes  int64  `json:"size_bytes"`
		} `json:"llamafile"`
	} `json:"runtime"`
}

type DownloadProgress struct {
	Downloaded       int64
	Total            int64
	Percent          float64
	SpeedBytesPerSec float64
	Message          string
}

var (
	cacheMu         sync.Mutex
	cachedArtifacts []models.QuantizedModel
	cacheAt         time.Time
)

// SeedCatalogForTests replaces the catalog cache for unit tests and returns a restore function.
func SeedCatalogForTests(items []models.QuantizedModel) func() {
	cacheMu.Lock()
	prevItems := append([]models.QuantizedModel(nil), cachedArtifacts...)
	prevAt := cacheAt
	cachedArtifacts = append([]models.QuantizedModel(nil), items...)
	if len(items) > 0 {
		cacheAt = time.Now()
	} else {
		cacheAt = time.Time{}
	}
	cacheMu.Unlock()

	return func() {
		cacheMu.Lock()
		cachedArtifacts = prevItems
		cacheAt = prevAt
		cacheMu.Unlock()
	}
}

// ListTranslateGemmaModels returns the supported packaged runtimes.
func ListTranslateGemmaModels() []models.QuantizedModel {
	return listArtifacts()
}

// RecommendedVisionRuntime returns the supported vision runtime SKU.
func RecommendedVisionRuntime() (models.QuantizedModel, bool) {
	for _, item := range listArtifacts() {
		if supportsVision(item) {
			return item, true
		}
	}
	return models.QuantizedModel{}, false
}

// DownloadModel downloads a runtime artifact into the local app data root.
func DownloadModel(dataRoot string, item models.QuantizedModel, onProgress func(DownloadProgress)) (string, error) {
	return DownloadModelWithContext(context.Background(), dataRoot, item, onProgress)
}

// DownloadModelWithContext downloads a runtime artifact into the local app data root.
func DownloadModelWithContext(ctx context.Context, dataRoot string, item models.QuantizedModel, onProgress func(DownloadProgress)) (string, error) {
	if item.FileName == "" {
		return "", fmt.Errorf("invalid artifact: empty file name")
	}
	downloadURL := strings.TrimSpace(item.DownloadURL)
	if downloadURL == "" {
		return "", fmt.Errorf("invalid artifact: missing download URL")
	}

	dstDir := filepath.Join(dataRoot, runtimeSubdir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	dstPath, existingPath, err := resolveRuntimeDestination(dstDir, item.FileName, item.SizeBytes)
	if err != nil {
		return "", err
	}
	if existingPath != "" {
		st, statErr := os.Stat(existingPath)
		if statErr == nil && st.Mode().IsRegular() {
			reportDownload(onProgress, st.Size(), item.SizeBytes, 0, "Artifact already exists locally")
			return existingPath, nil
		}
	}

	ctx, cancel := withDownloadTimeout(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("download failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return writeDownloadedArtifact(ctx, resp.Body, dstPath, resp.ContentLength, item.SizeBytes, onProgress)
}

func listArtifacts() []models.QuantizedModel {
	cacheMu.Lock()
	if time.Since(cacheAt) < cacheTTL && len(cachedArtifacts) > 0 {
		out := make([]models.QuantizedModel, len(cachedArtifacts))
		copy(out, cachedArtifacts)
		cacheMu.Unlock()
		return out
	}
	cacheMu.Unlock()

	artifacts, err := fetchArtifacts()
	if err != nil {
		cacheMu.Lock()
		defer cacheMu.Unlock()
		if len(cachedArtifacts) == 0 {
			return builtinCatalog()
		}
		out := make([]models.QuantizedModel, len(cachedArtifacts))
		copy(out, cachedArtifacts)
		return out
	}

	cacheMu.Lock()
	cachedArtifacts = make([]models.QuantizedModel, len(artifacts))
	copy(cachedArtifacts, artifacts)
	cacheAt = time.Now()
	cacheMu.Unlock()

	out := make([]models.QuantizedModel, len(artifacts))
	copy(out, artifacts)
	return out
}

func fetchArtifacts() ([]models.QuantizedModel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeManifestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: listTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("runtime manifest fetch failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var doc manifest
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	return manifestToCatalog(doc)
}

func manifestToCatalog(doc manifest) ([]models.QuantizedModel, error) {
	if len(doc.Models) == 0 {
		return nil, fmt.Errorf("runtime manifest does not contain models")
	}

	out := make([]models.QuantizedModel, 0, len(doc.Models))
	for _, entry := range doc.Models {
		fileName := strings.TrimSpace(entry.Runtime.Llamafile.FileName)
		pathInRepo := strings.TrimSpace(entry.Runtime.Llamafile.PathInRepo)
		if fileName == "" || pathInRepo == "" {
			continue
		}

		out = append(out, models.QuantizedModel{
			ID:          strings.TrimSpace(entry.ID),
			Kind:        "model",
			FileName:    fileName,
			Size:        humanSize(entry.Runtime.Llamafile.SizeBytes),
			SizeBytes:   entry.Runtime.Llamafile.SizeBytes,
			DownloadURL: buildRuntimeDownloadURL(pathInRepo),
			Recommended: entry.Recommended,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("runtime manifest did not produce usable runtimes")
	}

	sort.Slice(out, func(i, j int) bool {
		return catalogRank(out[i]) < catalogRank(out[j])
	})
	return out, nil
}

func builtinCatalog() []models.QuantizedModel {
	items := []models.QuantizedModel{
		{
			ID:          "q4_k_m",
			Kind:        "model",
			FileName:    "translategemma-4b-it.Q4_K_M.llamafile",
			SizeBytes:   2532719437,
			DownloadURL: buildRuntimeDownloadURL("translategemma-4b-it.Q4_K_M.llamafile"),
			Recommended: true,
		},
		{
			ID:          "q6_k",
			Kind:        "model",
			FileName:    "translategemma-4b-it.Q6_K.llamafile",
			SizeBytes:   3233561417,
			DownloadURL: buildRuntimeDownloadURL("translategemma-4b-it.Q6_K.llamafile"),
		},
		{
			ID:          "q8_0",
			Kind:        "model",
			FileName:    "translategemma-4b-it.Q8_0.llamafile",
			SizeBytes:   4173216585,
			DownloadURL: buildRuntimeDownloadURL("translategemma-4b-it.Q8_0.llamafile"),
		},
		{
			ID:          "q8_0_vision",
			Kind:        "model",
			FileName:    "translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile",
			SizeBytes:   4764613607,
			DownloadURL: buildRuntimeDownloadURL("translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile"),
			Recommended: true,
		},
	}
	for i := range items {
		items[i].Size = humanSize(items[i].SizeBytes)
	}
	return items
}

func buildRuntimeDownloadURL(pathInRepo string) string {
	parts := strings.Split(pathInRepo, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://huggingface.co/" + runtimeRepo + "/resolve/main/" + strings.Join(parts, "/")
}

func resolveRuntimeDestination(dstDir, fileName string, sizeBytes int64) (string, string, error) {
	preferredName := platform.PreferredRuntimeFileName(runtimeGOOS(), fileName)
	if preferredName == "" {
		return "", "", fmt.Errorf("invalid artifact: empty file name")
	}

	preferredPath := filepath.Join(dstDir, preferredName)
	for _, candidate := range platform.RuntimeFileCandidates(runtimeGOOS(), fileName) {
		candidatePath := filepath.Join(dstDir, candidate)
		st, err := os.Stat(candidatePath)
		if err != nil {
			continue
		}
		if !st.Mode().IsRegular() {
			continue
		}
		if sizeBytes > 0 && st.Size() < sizeBytes {
			continue
		}
		if candidatePath == preferredPath {
			return preferredPath, preferredPath, nil
		}
		if err := os.Rename(candidatePath, preferredPath); err == nil {
			return preferredPath, preferredPath, nil
		}
		return preferredPath, candidatePath, nil
	}
	return preferredPath, "", nil
}

func runtimeGOOS() string {
	current := platform.Current()
	if idx := strings.IndexByte(current, '/'); idx > 0 {
		return current[:idx]
	}
	return current
}

func supportsVision(item models.QuantizedModel) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(item.ID)), "_vision") ||
		strings.Contains(strings.ToLower(item.FileName), ".mmproj-")
}

func catalogRank(item models.QuantizedModel) string {
	switch item.ID {
	case "q4_k_m":
		return "1"
	case "q6_k":
		return "2"
	case "q8_0":
		return "3"
	case "q8_0_vision":
		return "4"
	default:
		return "9:" + item.ID
	}
}

func writeDownloadedArtifact(ctx context.Context, src io.Reader, dstPath string, contentLength, sizeBytes int64, onProgress func(DownloadProgress)) (string, error) {
	tmpPath := dstPath + ".partial"
	_ = os.Remove(tmpPath)

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}

	total := contentLength
	if total <= 0 {
		total = sizeBytes
	}

	buf := make([]byte, 1024*1024)
	var downloaded int64
	startedAt := time.Now()
	reportDownload(onProgress, 0, total, 0, "Downloading artifact")
	for {
		if err := ctx.Err(); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			wn, writeErr := out.Write(buf[:n])
			downloaded += int64(wn)
			reportDownload(onProgress, downloaded, total, downloadSpeed(startedAt, downloaded), "Downloading artifact")
			if writeErr != nil {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return "", writeErr
			}
			if wn != n {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				return "", io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			return "", readErr
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	if sizeBytes > 0 {
		st, err := os.Stat(tmpPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return "", err
		}
		if st.Size() < sizeBytes {
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("download incomplete: got %d bytes, expected %d", st.Size(), sizeBytes)
		}
	}

	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	finalSize := totalOr(sizeBytes, downloaded)
	reportDownload(onProgress, finalSize, finalSize, downloadSpeed(startedAt, finalSize), "Download completed")
	return dstPath, nil
}

func withDownloadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), downloadTimeout)
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, downloadTimeout)
}

func reportDownload(cb func(DownloadProgress), downloaded, total int64, speedBytesPerSec float64, msg string) {
	if cb == nil {
		return
	}
	percent := 0.0
	if total > 0 {
		percent = (float64(downloaded) / float64(total)) * 100
		if percent > 100 {
			percent = 100
		}
	}
	cb(DownloadProgress{
		Downloaded:       downloaded,
		Total:            total,
		Percent:          percent,
		SpeedBytesPerSec: speedBytesPerSec,
		Message:          msg,
	})
}

func downloadSpeed(startedAt time.Time, downloaded int64) float64 {
	if downloaded <= 0 || startedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(startedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(downloaded) / elapsed
}

func totalOr(v, fallback int64) int64 {
	if v > 0 {
		return v
	}
	return fallback
}

func humanSize(n int64) string {
	if n <= 0 {
		return "unknown"
	}
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	}
	return fmt.Sprintf("%.0f MB", float64(n)/float64(mb))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
