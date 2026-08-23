package utils

// FieldMapping represents a processing mapping for a field
type FieldMapping struct {
	TypeName string `json:"type"`
	Analyzer string `json:"analyzer"`
}

// GetFieldMapping extracts field mapping from type mapping object
func GetFieldMapping(mapping map[string]any, fieldName string) (*FieldMapping, bool) {
	propertiesRaw, ok := mapping["properties"]
	if !ok {
		return nil, false
	}
	properties, ok := propertiesRaw.(map[string]any)
	if !ok {
		return nil, false
	}
	config, ok := properties[fieldName]
	if !ok {
		return nil, false
	}
	configMap, ok := config.(map[string]any)
	if !ok {
		return nil, false
	}
	var fieldMapping FieldMapping
	fieldMapping.TypeName, _ = configMap["type"].(string)
	fieldMapping.Analyzer, _ = configMap["analyzer"].(string)
	return &fieldMapping, true
}
