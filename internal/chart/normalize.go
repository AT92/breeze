package chart

import "strings"

// NormalizeImageRef ensures an image reference is fully qualified
// with a registry and tag (e.g., "nginx" -> "docker.io/library/nginx:latest").
func NormalizeImageRef(reference string) string {
	reference = strings.TrimSpace(reference)

	if strings.Contains(reference, "@sha256:") {
		nameBeforeDigest := strings.Split(reference, "@")[0]
		if !strings.Contains(nameBeforeDigest, "/") {
			reference = "docker.io/library/" + reference
		}
		return reference
	}

	imageName, imageTag := splitImageNameTag(reference)

	if imageTag == "" {
		imageTag = "latest"
	}

	imageName = addDefaultRegistry(imageName)

	return imageName + ":" + imageTag
}

func addDefaultRegistry(imageName string) string {
	if !strings.Contains(imageName, "/") {
		return "docker.io/library/" + imageName
	}

	firstSegment := strings.Split(imageName, "/")[0]
	hasRegistryDot := strings.Contains(firstSegment, ".")
	hasRegistryPort := strings.Contains(firstSegment, ":")
	if !hasRegistryDot && !hasRegistryPort {
		return "docker.io/" + imageName
	}

	return imageName
}

// splitImageNameTag splits an image reference into name and tag,
// correctly handling registry ports (e.g., "registry:5000/repo/image:v1").
// The tag is the part after the last ":" that appears after the last "/".
func splitImageNameTag(reference string) (string, string) {
	lastSlashIndex := strings.LastIndex(reference, "/")
	if lastSlashIndex == -1 {
		parts := strings.SplitN(reference, ":", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
		return reference, ""
	}

	afterLastSlash := reference[lastSlashIndex+1:]
	colonIndex := strings.Index(afterLastSlash, ":")
	if colonIndex == -1 {
		return reference, ""
	}
	return reference[:lastSlashIndex+1+colonIndex], afterLastSlash[colonIndex+1:]
}

// imageNameWithoutTag returns the image name without tag or digest.
func imageNameWithoutTag(reference string) string {
	if digestIndex := strings.Index(reference, "@"); digestIndex != -1 {
		return reference[:digestIndex]
	}
	if colonIndex := strings.LastIndex(reference, ":"); colonIndex != -1 {
		afterColon := reference[colonIndex+1:]
		if !strings.Contains(afterColon, "/") {
			return reference[:colonIndex]
		}
	}
	return reference
}

// isValidImageRef checks whether a string looks like a container image reference.
func isValidImageRef(reference string) bool {
	reference = strings.TrimSpace(reference)
	if reference == "" || reference == "null" || reference == "true" || reference == "false" {
		return false
	}
	if strings.Contains(reference, " ") || strings.Contains(reference, "{{") {
		return false
	}
	if !strings.Contains(reference, "/") && looksLikeHostname(reference) {
		return false
	}
	return containsLetter(reference)
}

func looksLikeHostname(value string) bool {
	nameWithoutTag := strings.SplitN(value, ":", 2)[0]
	return strings.Contains(nameWithoutTag, ".")
}

func containsLetter(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') {
			return true
		}
	}
	return false
}
