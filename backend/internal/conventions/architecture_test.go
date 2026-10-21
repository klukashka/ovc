package conventions_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	modulePath   = "ovc/backend"
	layersSource = "../../../knowledge/architecture/detailed.md"
)

var (
	tableRow     = regexp.MustCompile("^\\| `([^`]+)` \\| (.+) \\|$")
	codeSpan     = regexp.MustCompile("`([^`]+)`")
	everyPackage = "every package"
)

type layerRule struct {
	allowed    []string
	everything bool
}

func loadLayerTable(t *testing.T) map[string]layerRule {
	t.Helper()
	text, err := os.ReadFile(layersSource)
	if err != nil {
		t.Fatal(err)
	}
	section := string(text)
	start := strings.Index(section, "## Package layers")
	if start < 0 {
		t.Fatal("no package layers section")
	}
	section = section[start:]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}
	rules := map[string]layerRule{}
	for _, line := range strings.Split(section, "\n") {
		match := tableRow.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		rule := layerRule{everything: strings.Contains(match[2], everyPackage)}
		for _, allowed := range codeSpan.FindAllStringSubmatch(match[2], -1) {
			rule.allowed = append(rule.allowed, allowed[1])
		}
		rules[match[1]] = rule
	}
	if len(rules) == 0 {
		t.Fatal("the package layers table is empty")
	}
	return rules
}

func ruleFor(rules map[string]layerRule, packagePath string) (string, bool) {
	best := ""
	for key := range rules {
		pattern := strings.TrimSuffix(key, "/*")
		if packagePath == pattern || strings.HasPrefix(packagePath, pattern+"/") {
			if len(pattern) > len(best) {
				best = key
			}
		}
	}
	return best, best != ""
}

func importAllowed(rule layerRule, own string, imported string) bool {
	if rule.everything || imported == own || strings.HasPrefix(imported, own+"/") {
		return true
	}
	for _, allowed := range rule.allowed {
		if imported == allowed || strings.HasPrefix(imported, allowed+"/") {
			return true
		}
	}
	return false
}

func checkLayers(rules map[string]layerRule, imports map[string][]string) []string {
	var problems []string
	packages := make([]string, 0, len(imports))
	for packagePath := range imports {
		packages = append(packages, packagePath)
	}
	sort.Strings(packages)
	for _, packagePath := range packages {
		key, found := ruleFor(rules, packagePath)
		if !found {
			problems = append(problems, packagePath+": not in the package layers table")
			continue
		}
		rule := rules[key]
		own := strings.TrimSuffix(key, "/*")
		for _, imported := range imports[packagePath] {
			if !importAllowed(rule, own, imported) {
				problems = append(problems, packagePath+" imports "+imported+", which its row ("+key+") does not allow")
			}
		}
	}
	return problems
}

func moduleImports(t *testing.T) map[string][]string {
	t.Helper()
	imports := map[string][]string{}
	for _, path := range sourceFiles(t, ".go") {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(backendRoot, filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		packagePath := filepath.ToSlash(relative)
		imports[packagePath] = append(imports[packagePath], []string{}...)
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(imported, modulePath+"/") {
				imports[packagePath] = append(imports[packagePath], strings.TrimPrefix(imported, modulePath+"/"))
			}
		}
	}
	return imports
}

func TestPackagesFollowTheLayerTable(t *testing.T) {
	rules := loadLayerTable(t)
	imports := moduleImports(t)

	problems := checkLayers(rules, imports)

	if len(problems) > 0 {
		t.Fatalf("package layer violations:\n%s", strings.Join(problems, "\n"))
	}
}

func TestLayerCheckRejectsForbiddenImports(t *testing.T) {
	rules := loadLayerTable(t)
	imports := map[string][]string{
		"internal/domain":        {"internal/core"},
		"internal/api":           {"internal/repositories"},
		"internal/unlisted":      {},
		"internal/services/chat": {"internal/services/accounts", "internal/repositories"},
	}

	problems := checkLayers(rules, imports)

	joined := strings.Join(problems, "\n")
	for _, expected := range []string{"internal/domain imports internal/core", "internal/api imports internal/repositories", "internal/unlisted: not in the package layers table"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected %q among:\n%s", expected, joined)
		}
	}
	if strings.Contains(joined, "internal/services/chat") {
		t.Fatalf("services may import repositories and each other:\n%s", joined)
	}
}

func TestProductionCodeNeverImportsTheTestDatabaseFixture(t *testing.T) {
	imports := moduleImports(t)

	var importers []string
	for packagePath, imported := range imports {
		for _, dependency := range imported {
			fixture := dependency == "internal/testdb" || strings.HasPrefix(dependency, "internal/testdb/")
			if fixture && packagePath != "internal/testdb" && !strings.HasPrefix(packagePath, "internal/testdb/") {
				importers = append(importers, packagePath)
			}
		}
	}

	if len(importers) > 0 {
		sort.Strings(importers)
		t.Fatalf("production packages import internal/testdb: %s", strings.Join(importers, ", "))
	}
}
