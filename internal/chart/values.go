package chart

import "fmt"

// extractImagesFromValues walks a values map tree and collects image references.
func extractImagesFromValues(values map[string]interface{}, images map[string]struct{}) {
	walkValuesTree(values, images)
}

func walkValuesTree(valuesMap map[string]interface{}, images map[string]struct{}) {
	for key, value := range valuesMap {
		switch typedValue := value.(type) {
		case map[string]interface{}:
			collectImageFromMap(typedValue, images)
			walkValuesTree(typedValue, images)

		case map[interface{}]interface{}:
			convertedMap := convertToStringKeyMap(typedValue)
			collectImageFromMap(convertedMap, images)
			walkValuesTree(convertedMap, images)

		case string:
			if key == "image" && isValidImageRef(typedValue) {
				images[typedValue] = struct{}{}
			}
		}
	}
}

func collectImageFromMap(valuesMap map[string]interface{}, images map[string]struct{}) {
	repository, hasRepository := valuesMap["repository"].(string)
	if !hasRepository || repository == "" {
		return
	}
	reference := buildImageReference(valuesMap)
	if reference != "" {
		images[reference] = struct{}{}
	}
}

// buildImageReference constructs an image reference from a map containing
// "repository" and optionally "registry", "tag"/"version", and "digest" keys.
func buildImageReference(valuesMap map[string]interface{}) string {
	repository, _ := valuesMap["repository"].(string)
	if repository == "" {
		return ""
	}

	registry, _ := valuesMap["registry"].(string)
	imageTag := resolveFirstNonEmpty(valuesMap, "tag", "version")
	digest, _ := valuesMap["digest"].(string)

	var reference string
	if registry != "" {
		reference = registry + "/" + repository
	} else {
		reference = repository
	}

	if digest != "" {
		reference = reference + "@" + digest
	} else if imageTag != "" {
		reference = reference + ":" + imageTag
	}

	return reference
}

// resolveFirstNonEmpty checks multiple keys in order and returns the first non-empty value.
func resolveFirstNonEmpty(valuesMap map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, exists := valuesMap[key]; exists {
			stringValue := fmt.Sprintf("%v", value)
			if stringValue != "" && stringValue != "<nil>" {
				return stringValue
			}
		}
	}
	return ""
}

func convertToStringKeyMap(source map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(source))
	for key, value := range source {
		if stringKey, isString := key.(string); isString {
			result[stringKey] = value
		}
	}
	return result
}
