package chart

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

var qualifiedImageRegex = regexp.MustCompile(`image:\s*["']?([a-zA-Z0-9._\-]+(?:\.[a-zA-Z0-9._\-]+)*(?::[0-9]+)?/[a-zA-Z0-9._\-/]+(?::[a-zA-Z0-9._\-]+)?(?:@sha256:[a-f0-9]{64})?)["']?`)
var containerImageRegex = regexp.MustCompile(`-\s+image:\s*["']?([a-zA-Z0-9._\-]+(?::[a-zA-Z0-9._\-]+)?)["']?`)

// ExtractImages finds all container image references in a Helm chart using three strategies:
// template rendering, regex scanning on rendered output, and values tree walking.
func ExtractImages(helmChart *chart.Chart, valuesOverrides map[string]interface{}) ([]string, error) {
	renderedImages := make(map[string]struct{})
	valuesImages := make(map[string]struct{})

	rendered, err := renderTemplates(helmChart, valuesOverrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: template rendering failed, falling back to values scanning: %v\n", err)
	} else {
		for _, content := range rendered {
			scanRenderedContentForImages(content, renderedImages)
		}
	}

	mergedValues := mergeChartValues(helmChart, valuesOverrides)
	extractImagesFromValues(mergedValues, valuesImages)

	return deduplicateImages(renderedImages, valuesImages), nil
}

func mergeChartValues(helmChart *chart.Chart, valuesOverrides map[string]interface{}) map[string]interface{} {
	mergedValues := make(map[string]interface{})
	if valuesOverrides != nil {
		for key, value := range valuesOverrides {
			mergedValues[key] = value
		}
	}
	chartutil.CoalesceTables(mergedValues, helmChart.Values)

	for _, dependency := range helmChart.Dependencies() {
		dependencyName := dependency.Name()
		if _, exists := mergedValues[dependencyName]; !exists {
			mergedValues[dependencyName] = dependency.Values
		} else if dependencyMap, isMap := mergedValues[dependencyName].(map[string]interface{}); isMap {
			chartutil.CoalesceTables(dependencyMap, dependency.Values)
		}
	}

	return mergedValues
}

func deduplicateImages(renderedImages, valuesImages map[string]struct{}) []string {
	byImageName := make(map[string]string)

	for rawReference := range renderedImages {
		normalized := NormalizeImageRef(rawReference)
		name := imageNameWithoutTag(normalized)
		byImageName[name] = normalized
	}

	for rawReference := range valuesImages {
		normalized := NormalizeImageRef(rawReference)
		name := imageNameWithoutTag(normalized)
		if existing, exists := byImageName[name]; exists {
			if strings.HasSuffix(existing, ":latest") && !strings.HasSuffix(normalized, ":latest") {
				byImageName[name] = normalized
			}
		} else {
			byImageName[name] = normalized
		}
	}

	result := make([]string, 0, len(byImageName))
	for _, reference := range byImageName {
		result = append(result, reference)
	}
	sort.Strings(result)
	return result
}

func renderTemplates(helmChart *chart.Chart, overrides map[string]interface{}) (map[string]string, error) {
	renderValues, err := chartutil.ToRenderValues(helmChart, overrides, chartutil.ReleaseOptions{
		Name:      "breeze-render",
		Namespace: "default",
		IsInstall: true,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing render values: %w", err)
	}

	templateEngine := engine.Engine{LintMode: true}
	rendered, err := templateEngine.Render(helmChart, renderValues)
	if err == nil {
		return rendered, nil
	}

	fmt.Fprintf(os.Stderr, "Note: full template render failed (%v), rendering templates individually...\n", err)
	return renderTemplatesIndividually(helmChart, renderValues)
}

func renderTemplatesIndividually(helmChart *chart.Chart, renderValues chartutil.Values) (map[string]string, error) {
	helperFiles, templateFiles := separateHelperAndTemplateFiles(helmChart.Templates)

	result := make(map[string]string)
	templateEngine := engine.Engine{LintMode: true}

	for _, templateFile := range templateFiles {
		singleTemplateChart := &chart.Chart{
			Metadata:  helmChart.Metadata,
			Values:    helmChart.Values,
			Templates: append([]*chart.File{templateFile}, helperFiles...),
		}
		for _, dependency := range helmChart.Dependencies() {
			singleTemplateChart.AddDependency(dependency)
		}

		rendered, err := templateEngine.Render(singleTemplateChart, renderValues)
		if err != nil {
			continue
		}
		for name, content := range rendered {
			result[name] = content
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("all templates failed to render")
	}
	return result, nil
}

func separateHelperAndTemplateFiles(allTemplates []*chart.File) (helpers []*chart.File, templates []*chart.File) {
	for _, templateFile := range allTemplates {
		fileName := templateFile.Name
		if lastSlash := strings.LastIndex(fileName, "/"); lastSlash >= 0 {
			fileName = fileName[lastSlash+1:]
		}
		if strings.HasPrefix(fileName, "_") {
			helpers = append(helpers, templateFile)
		} else {
			templates = append(templates, templateFile)
		}
	}
	return helpers, templates
}

func scanRenderedContentForImages(content string, images map[string]struct{}) {
	for _, match := range qualifiedImageRegex.FindAllStringSubmatch(content, -1) {
		imageReference := strings.TrimSpace(match[1])
		if isValidImageRef(imageReference) {
			images[imageReference] = struct{}{}
		}
	}

	for _, match := range containerImageRegex.FindAllStringSubmatch(content, -1) {
		imageReference := strings.TrimSpace(match[1])
		if isValidImageRef(imageReference) {
			images[imageReference] = struct{}{}
		}
	}
}
