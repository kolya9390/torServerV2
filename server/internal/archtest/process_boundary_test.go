package archtest

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

const projectImportPrefix = "server/"

var clientBoundary = dependencyBoundary{
	name: "torrctl",
	roots: []string{
		"cmd/torrctl",
		"internal/apiclient",
		"internal/cliapp",
	},
	entryPackage: "server/cmd/torrctl",
	allowedProjectImports: []string{
		"server/internal/apiclient",
		"server/internal/cliapp",
		"server/version",
	},
	forbiddenExternalImports: []string{
		"github.com/anacrolix",
	},
}

var serverBoundary = dependencyBoundary{
	name: "torrserver",
	roots: []string{
		"cmd/torrserver",
		"internal/daemon",
	},
	entryPackage: "server/cmd/torrserver",
	forbiddenProjectImports: []string{
		"server/internal/apiclient",
		"server/internal/cliapp",
	},
	forbiddenExternalImports: []string{
		"github.com/spf13/cobra",
		"golang.org/x/term",
	},
}

var approvedSharedLeafPackages = []string{"server/version"}

type dependencyBoundary struct {
	name                     string
	roots                    []string
	entryPackage             string
	allowedProjectImports    []string
	forbiddenProjectImports  []string
	forbiddenExternalImports []string
}

type importEdge struct {
	dependency string
	file       string
}

type importGraph map[string][]importEdge

func TestProcessBoundariesRejectDirectForbiddenImports(t *testing.T) {
	root := projectRoot(t)
	for _, boundary := range []dependencyBoundary{clientBoundary, serverBoundary} {
		t.Run(boundary.name, func(t *testing.T) {
			for _, relativeDir := range boundary.roots {
				dir := filepath.Join(root, filepath.FromSlash(relativeDir))
				files := collectGoFiles(t, dir, productionGoFile)
				for _, path := range files {
					file := parseFile(t, path)
					for _, imp := range file.Imports {
						dependency := importPath(t, path, imp)
						if boundary.forbids(dependency) {
							t.Errorf(
								"%s boundary: %s imports forbidden dependency %q",
								boundary.name,
								relativePath(t, root, path),
								dependency,
							)
						}
					}
				}
			}
		})
	}
}

func TestProcessBoundariesRejectTransitiveForbiddenImports(t *testing.T) {
	graph := buildImportGraph(t, projectRoot(t))
	for _, boundary := range []dependencyBoundary{clientBoundary, serverBoundary} {
		t.Run(boundary.name, func(t *testing.T) {
			violations := findBoundaryViolations(graph, boundary)
			for _, violation := range violations {
				t.Error(violation)
			}
		})
	}
}

func TestProcessSidesShareOnlyApprovedLeafPackages(t *testing.T) {
	graph := buildImportGraph(t, projectRoot(t))
	clientPackages := reachableProjectPackages(graph, clientBoundary.entryPackage)
	serverPackages := reachableProjectPackages(graph, serverBoundary.entryPackage)

	shared := make([]string, 0)
	for pkg := range clientPackages {
		if _, ok := serverPackages[pkg]; !ok {
			continue
		}

		shared = append(shared, pkg)
	}
	sort.Strings(shared)

	for _, pkg := range shared {
		if !slices.Contains(approvedSharedLeafPackages, pkg) {
			t.Errorf("process sides share unapproved project package %q", pkg)
		}
	}

	if !slices.Equal(shared, approvedSharedLeafPackages) {
		t.Errorf("shared project packages = %v, want %v", shared, approvedSharedLeafPackages)
	}

	for _, pkg := range approvedSharedLeafPackages {
		for _, edge := range graph[pkg] {
			if isProjectImport(edge.dependency) {
				t.Errorf(
					"approved shared leaf %q imports project package %q in %s",
					pkg,
					edge.dependency,
					edge.file,
				)
			}
		}
	}
}

func TestDedicatedCommandPackagesContainOnlyMain(t *testing.T) {
	root := projectRoot(t)
	for _, relativeDir := range []string{"cmd/torrctl", "cmd/torrserver"} {
		dir := filepath.Join(root, filepath.FromSlash(relativeDir))
		files := collectGoFiles(t, dir, productionGoFile)
		if len(files) != 1 || filepath.Base(files[0]) != "main.go" {
			relativeFiles := make([]string, 0, len(files))
			for _, path := range files {
				relativeFiles = append(relativeFiles, relativePath(t, root, path))
			}

			t.Errorf("%s must contain only a thin main.go, found %v", relativeDir, relativeFiles)
		}
	}
}

func TestFindBoundaryViolationsReportsDeterministicImportChain(t *testing.T) {
	tests := []struct {
		name       string
		boundary   dependencyBoundary
		graph      importGraph
		violations []string
	}{
		{
			name:     "client project dependency",
			boundary: clientBoundary,
			graph: importGraph{
				"server/cmd/torrctl": {
					{dependency: "server/internal/cliapp", file: "cmd/torrctl/main.go"},
				},
				"server/internal/cliapp": {
					{dependency: "server/internal/bridge", file: "internal/cliapp/application.go"},
				},
			},
			violations: []string{
				`torrctl boundary: internal/cliapp/application.go imports forbidden dependency "server/internal/bridge" ` +
					`via server/cmd/torrctl -> server/internal/cliapp`,
			},
		},
		{
			name:     "server external dependency",
			boundary: serverBoundary,
			graph: importGraph{
				"server/cmd/torrserver": {
					{dependency: "server/internal/daemon", file: "cmd/torrserver/main.go"},
				},
				"server/internal/daemon": {
					{dependency: "github.com/spf13/cobra", file: "internal/daemon/args.go"},
				},
			},
			violations: []string{
				`torrserver boundary: internal/daemon/args.go imports forbidden dependency "github.com/spf13/cobra" ` +
					`via server/cmd/torrserver -> server/internal/daemon`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := findBoundaryViolations(test.graph, test.boundary)
			if !slices.Equal(violations, test.violations) {
				t.Fatalf("violations = %q, want %q", violations, test.violations)
			}
		})
	}
}

func (boundary dependencyBoundary) forbids(dependency string) bool {
	if isProjectImport(dependency) {
		if len(boundary.allowedProjectImports) > 0 {
			return !hasImportPrefix(dependency, boundary.allowedProjectImports)
		}

		return hasImportPrefix(dependency, boundary.forbiddenProjectImports)
	}

	return hasImportPrefix(dependency, boundary.forbiddenExternalImports)
}

func hasImportPrefix(dependency string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
			return true
		}
	}

	return false
}

func buildImportGraph(t *testing.T, root string) importGraph {
	t.Helper()

	graph := make(importGraph)
	for _, path := range collectGoFiles(t, root, productionGoFile) {
		pkg := projectPackagePath(t, root, path)
		file := parseFile(t, path)
		for _, imp := range file.Imports {
			graph[pkg] = append(graph[pkg], importEdge{
				dependency: importPath(t, path, imp),
				file:       relativePath(t, root, path),
			})
		}
	}

	for pkg := range graph {
		sort.Slice(graph[pkg], func(first, second int) bool {
			if graph[pkg][first].dependency == graph[pkg][second].dependency {
				return graph[pkg][first].file < graph[pkg][second].file
			}

			return graph[pkg][first].dependency < graph[pkg][second].dependency
		})
	}

	return graph
}

func findBoundaryViolations(graph importGraph, boundary dependencyBoundary) []string {
	violations := make(map[string]struct{})
	visited := make(map[string]struct{})

	var visit func(string, []string)
	visit = func(pkg string, chain []string) {
		if _, ok := visited[pkg]; ok {
			return
		}
		visited[pkg] = struct{}{}

		for _, edge := range graph[pkg] {
			if boundary.forbids(edge.dependency) {
				message := fmt.Sprintf(
					"%s boundary: %s imports forbidden dependency %q via %s",
					boundary.name,
					edge.file,
					edge.dependency,
					strings.Join(chain, " -> "),
				)
				violations[message] = struct{}{}

				continue
			}

			if isProjectImport(edge.dependency) {
				visit(edge.dependency, append(chain, edge.dependency))
			}
		}
	}

	visit(boundary.entryPackage, []string{boundary.entryPackage})

	result := make([]string, 0, len(violations))
	for violation := range violations {
		result = append(result, violation)
	}
	sort.Strings(result)

	return result
}

func reachableProjectPackages(graph importGraph, entryPackage string) map[string]struct{} {
	reachable := make(map[string]struct{})

	var visit func(string)
	visit = func(pkg string) {
		if _, ok := reachable[pkg]; ok {
			return
		}
		reachable[pkg] = struct{}{}

		for _, edge := range graph[pkg] {
			if isProjectImport(edge.dependency) {
				visit(edge.dependency)
			}
		}
	}

	visit(entryPackage)

	return reachable
}

func projectPackagePath(t *testing.T, root, path string) string {
	t.Helper()

	relativeDir, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		t.Fatalf("resolve package path for %s: %v", path, err)
	}

	if relativeDir == "." {
		return "server"
	}

	return projectImportPrefix + filepath.ToSlash(relativeDir)
}

func relativePath(t *testing.T, root, path string) string {
	t.Helper()

	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("resolve relative path for %s: %v", path, err)
	}

	return filepath.ToSlash(relative)
}

func productionGoFile(path string) bool {
	return !strings.HasSuffix(path, "_test.go")
}

func isProjectImport(dependency string) bool {
	return dependency == "server" || strings.HasPrefix(dependency, projectImportPrefix)
}
