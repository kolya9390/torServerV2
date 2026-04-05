package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNoDirectSettingsArgsUsage(t *testing.T) {
	goFiles := collectGoFiles(t, projectRoot(t), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		ast.Inspect(f, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Name == "settings" && sel.Sel.Name == "Args" {
				t.Errorf("forbidden direct settings.Args usage in %s", path)
			}

			return true
		})
	}
}

func TestAPILayerDoesNotImportInfraDirectly(t *testing.T) {
	forbidden := map[string]struct{}{
		"server/torr":    {},
		"server/modules": {},
		"server/ffprobe": {},
	}

	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "web", "api"), func(path string) bool {
		if strings.HasSuffix(path, "_test.go") {
			return false
		}

		if strings.HasSuffix(path, "services.go") {
			return false
		}

		if strings.Contains(path, string(filepath.Separator)+"utils"+string(filepath.Separator)) {
			return false
		}

		return true
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}

			if _, exists := forbidden[pkg]; exists {
				t.Errorf("forbidden import %q in transport file %s", pkg, path)
			}
		}
	}
}

func TestOsExitOnlyInMain(t *testing.T) {
	goFiles := collectGoFiles(t, projectRoot(t), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		ast.Inspect(f, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Name != "os" || sel.Sel.Name != "Exit" {
				return true
			}

			mainPath := filepath.Join(projectRoot(t), "cmd", "main.go")
			if filepath.Clean(path) != filepath.Clean(mainPath) {
				t.Errorf("os.Exit is only allowed in cmd/main.go, found in %s", path)
			}

			return true
		})
	}
}

func TestSettingsLayerDoesNotImportWebPackages(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "settings"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}

			if strings.HasPrefix(pkg, "server/web/") {
				t.Errorf("forbidden settings import %q in %s", pkg, path)
			}
		}
	}
}

func TestInternalAppDoesNotImportRootServerPackage(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "internal", "app"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}

			if pkg == "server" {
				t.Errorf("forbidden internal/app import %q in %s", pkg, path)
			}
		}
	}
}

func collectGoFiles(t *testing.T, root string, include func(string) bool) []string {
	t.Helper()

	files := make([]string, 0, 256)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		if include(path) {
			files = append(files, filepath.Clean(path))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk go files in %s: %v", root, err)
	}

	return files
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return file
}

func projectRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	root := filepath.Clean(filepath.Join(wd, "..", ".."))

	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil || info.IsDir() {
		t.Fatalf("cannot resolve project root from wd=%s", wd)
	}

	return root
}
