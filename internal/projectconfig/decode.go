package projectconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const maxConfigBytes = 1 << 20

// Load reads, strictly decodes, and validates a project configuration. The
// configuration root is the directory containing filename.
func Load(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", filename, err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", filename, err)
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("read %q: configuration exceeds %d bytes", filename, maxConfigBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("decode %q: configuration is not valid UTF-8", filename)
	}
	if err := rejectDuplicateNames(data); err != nil {
		return nil, fmt.Errorf("decode %q: %w", filename, err)
	}

	config, err := decodeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", filename, err)
	}
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("validate %q: %w", filename, err)
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", filename, err)
	}
	config.root = filepath.Dir(filepath.Clean(abs))
	return config, nil
}

func rejectDuplicateNames(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSONValue(decoder, "$"); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected value after top-level document: %v", token)
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, location string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("%s: object key is not a string", location)
			}
			if seen[key] {
				return fmt.Errorf("%s: duplicate field %q", location, key)
			}
			seen[key] = true
			if err := walkJSONValue(decoder, location+"."+key); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		index := 0
		for decoder.More() {
			if err := walkJSONValue(decoder, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
			index++
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("%s: unexpected delimiter %q", location, delim)
	}
}

func decodeConfig(data []byte) (*Config, error) {
	object, err := decodeObject(data, "$", []string{"version", "project", "sources"})
	if err != nil {
		return nil, err
	}
	if err := requireFields(object, "$", "version", "project", "sources"); err != nil {
		return nil, err
	}

	config := &Config{}
	if err := json.Unmarshal(object["version"], &config.Version); err != nil {
		return nil, fmt.Errorf("$.version: %w", err)
	}
	project, err := decodeProject(object["project"])
	if err != nil {
		return nil, err
	}
	config.Project = project

	var rawSources []json.RawMessage
	if isJSONNull(object["sources"]) {
		return nil, fmt.Errorf("$.sources: must be an array")
	}
	if err := json.Unmarshal(object["sources"], &rawSources); err != nil {
		return nil, fmt.Errorf("$.sources: %w", err)
	}
	config.Sources = make([]Source, 0, len(rawSources))
	for index, raw := range rawSources {
		source, err := decodeSource(raw, index)
		if err != nil {
			return nil, err
		}
		config.Sources = append(config.Sources, source)
	}
	return config, nil
}

func decodeProject(data []byte) (Project, error) {
	object, err := decodeObject(data, "$.project", []string{"kind", "minimum_zsh", "identifier"})
	if err != nil {
		return Project{}, err
	}
	if err := requireFields(object, "$.project", "kind", "minimum_zsh", "identifier"); err != nil {
		return Project{}, err
	}

	var project Project
	if err := json.Unmarshal(object["kind"], &project.Kind); err != nil {
		return Project{}, fmt.Errorf("$.project.kind: %w", err)
	}
	if err := json.Unmarshal(object["minimum_zsh"], &project.MinimumZsh); err != nil {
		return Project{}, fmt.Errorf("$.project.minimum_zsh: %w", err)
	}
	if err := json.Unmarshal(object["identifier"], &project.Identifier); err != nil {
		return Project{}, fmt.Errorf("$.project.identifier: %w", err)
	}
	return project, nil
}

func decodeSource(data []byte, index int) (Source, error) {
	location := fmt.Sprintf("$.sources[%d]", index)
	object, err := decodeObject(data, location, []string{"root", "profile", "role"})
	if err != nil {
		return Source{}, err
	}
	if err := requireFields(object, location, "root", "profile"); err != nil {
		return Source{}, err
	}

	var source Source
	if err := json.Unmarshal(object["root"], &source.Root); err != nil {
		return Source{}, fmt.Errorf("%s.root: %w", location, err)
	}
	if err := json.Unmarshal(object["profile"], &source.Profile); err != nil {
		return Source{}, fmt.Errorf("%s.profile: %w", location, err)
	}
	if raw, ok := object["role"]; ok {
		if isJSONNull(raw) {
			return Source{}, fmt.Errorf("%s.role: must be a string", location)
		}
		if err := json.Unmarshal(raw, &source.Role); err != nil {
			return Source{}, fmt.Errorf("%s.role: %w", location, err)
		}
	}
	return source, nil
}

func decodeObject(data []byte, location string, allowed []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("%s: %w", location, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s: must be an object", location)
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = true
	}
	for field := range object {
		if !allowedSet[field] {
			return nil, fmt.Errorf("%s: unknown field %q", location, field)
		}
	}
	return object, nil
}

func requireFields(object map[string]json.RawMessage, location string, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("%s: missing required field %q", location, field)
		}
	}
	return nil
}

func isJSONNull(data []byte) bool { return bytes.Equal(bytes.TrimSpace(data), []byte("null")) }
