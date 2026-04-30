package install

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// mergeJSON reads a JSON file (or starts from {}), deep-merges overlay into it, and writes back atomically.
func mergeJSON(path string, overlay map[string]any) error {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &root)
	}
	deepMerge(root, overlay)
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

// deepMerge merges src into dst recursively for map values; otherwise dst[k] = src[k].
func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			if dMap, ok := dst[k].(map[string]any); ok {
				deepMerge(dMap, vMap)
				continue
			}
		}
		dst[k] = v
	}
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
