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
			pkg := importPath(t, path, imp)
			if isForbiddenTransportInfraImport(pkg) {
				t.Errorf("forbidden import %q in transport file %s", pkg, path)
			}
		}
	}
}

func TestAppContractsDoNotImportTransportPackages(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "internal", "app", "contracts"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg := importPath(t, path, imp)
			if pkg == "server/web" || strings.HasPrefix(pkg, "server/web/") {
				t.Errorf("forbidden transport import %q in app contract file %s", pkg, path)
			}
		}
	}
}

func TestTorrentsHandlerDoesNotImportParsingAdapters(t *testing.T) {
	path := filepath.Join(projectRoot(t), "web", "api", "torrents.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		switch pkg {
		case "github.com/anacrolix/torrent", "server/torrshash", "server/web/api/utils":
			t.Errorf("forbidden parsing adapter import %q in torrents handler %s", pkg, path)
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

			// Allow os.Exit in cmd/main.go (server mode) and cmd/cli/ (CLI mode)
			mainPath := filepath.Join(projectRoot(t), "cmd", "main.go")
			cliDir := filepath.Join(projectRoot(t), "cmd", "cli")
			isCLI := strings.HasPrefix(filepath.Clean(path), filepath.Clean(cliDir))
			if filepath.Clean(path) != filepath.Clean(mainPath) && !isCLI {
				t.Errorf("os.Exit is only allowed in cmd/main.go or cmd/cli/, found in %s", path)
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
			pkg := importPath(t, path, imp)

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
			pkg := importPath(t, path, imp)

			if pkg == "server" {
				t.Errorf("forbidden internal/app import %q in %s", pkg, path)
			}
		}
	}
}

func importPath(t *testing.T, sourcePath string, imp *ast.ImportSpec) string {
	t.Helper()

	pkg, err := strconv.Unquote(imp.Path.Value)
	if err != nil {
		t.Fatalf("unquote import in %s: %v", sourcePath, err)
	}

	return pkg
}

func isForbiddenTransportInfraImport(pkg string) bool {
	if pkg == "github.com/anacrolix/torrent" || strings.HasPrefix(pkg, "github.com/anacrolix/torrent/") {
		return true
	}

	if pkg == "server/torr" || strings.HasPrefix(pkg, "server/torr/") {
		return true
	}

	return pkg == "server/modules" || pkg == "server/ffprobe"
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
