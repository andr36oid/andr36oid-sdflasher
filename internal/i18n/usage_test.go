package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// The keys the desktop and Android apps show must exist in en.json, and
// every key in en.json must still be used somewhere.

const repoRoot = "../.."

func sourceFiles(t *testing.T, ext string) []string {
	t.Helper()
	var out []string
	filepath.Walk(repoRoot, func(p string, fi os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if fi.IsDir() {
			switch fi.Name() {
			case ".git", "build", "dist", "node_modules", "test", "androidTestDebug", "androidTest":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ext) && !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// goKeys returns the keys passed to s.t / t / i18n.Text / Translate, the
// Phase and Name of progress events, and (in the ui package) the
// capitalised strings of string lists, which the UI translates in loops.
// all holds every string literal, for the unused-key check.
func goKeys(t *testing.T) (used map[string]string, all map[string]bool) {
	used, all = map[string]string{}, map[string]bool{}
	fset := token.NewFileSet()
	files := map[string][]*ast.File{}
	for _, p := range sourceFiles(t, ".go") {
		f, e := parser.ParseFile(fset, p, nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		files[filepath.Dir(p)] = append(files[filepath.Dir(p)], f)
	}
	for dir, fs := range files {
		consts := map[string]string{}
		for _, f := range fs {
			ast.Inspect(f, func(n ast.Node) bool {
				if v, ok := n.(*ast.ValueSpec); ok {
					for i, name := range v.Names {
						if i < len(v.Values) {
							if s, ok := strLit(v.Values[i]); ok {
								consts[name.Name] = s
							}
						}
					}
				}
				if s, ok := strLit(n); ok {
					all[s] = true
				}
				return true
			})
		}
		value := func(x ast.Expr) (string, bool) {
			if s, ok := strLit(x); ok {
				return s, true
			}
			if id, ok := x.(*ast.Ident); ok {
				s, ok := consts[id.Name]
				return s, ok
			}
			return "", false
		}
		for _, f := range fs {
			where := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					name, arg := "", -1
					switch fn := x.Fun.(type) {
					case *ast.Ident:
						name = fn.Name
					case *ast.SelectorExpr:
						name = fn.Sel.Name
					}
					switch name {
					case "t":
						arg = 0
					case "Text", "Translate":
						arg = 1
					}
					if arg >= 0 && arg < len(x.Args) {
						if s, ok := value(x.Args[arg]); ok {
							used[s] = where(x)
						}
					}
				case *ast.KeyValueExpr:
					if k, ok := x.Key.(*ast.Ident); ok && (k.Name == "Phase" || k.Name == "Name") {
						if s, ok := strLit(x.Value); ok && s != "" {
							used[s] = where(x)
						}
					}
				case *ast.CompositeLit:
					if filepath.Base(dir) != "ui" {
						return true
					}
					for _, el := range x.Elts {
						if s, ok := strLit(el); ok && s != "" && unicode.IsUpper([]rune(s)[0]) && !strings.ContainsAny(s, "/\\") {
							used[s] = where(el)
						}
					}
				}
				return true
			})
		}
	}
	return used, all
}

func strLit(n ast.Node) (string, bool) {
	b, ok := n.(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, e := strconv.Unquote(b.Value)
	return s, e == nil
}

var (
	javaConcat = regexp.MustCompile(`"\s*\+\s*"`)
	javaString = `"((?:[^"\\\n]|\\.)*)"`
	javaCalls  = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:t|button|label)\(\s*` + javaString),
		regexp.MustCompile(`\binfo\(\s*` + javaString + `,\s*` + javaString),
		regexp.MustCompile(`\bmessage\s*=\s*` + javaString),
		regexp.MustCompile(`\bmessage\s*=\s*\w+\s*\?\s*` + javaString + `\s*:\s*` + javaString),
		regexp.MustCompile(`STEPS\s*=\s*\{([^}]*)\}`),
	}
	javaAny = regexp.MustCompile(javaString)
)

func javaKeys(t *testing.T) (used map[string]string, all map[string]bool) {
	used, all = map[string]string{}, map[string]bool{}
	for _, p := range sourceFiles(t, ".java") {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		src := javaConcat.ReplaceAllString(string(b), "")
		for _, m := range javaAny.FindAllStringSubmatch(src, -1) {
			all[m[1]] = true
		}
		for _, re := range javaCalls {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				for _, g := range m[1:] {
					if strings.Contains(g, `"`) { // a list of strings
						for _, s := range javaAny.FindAllStringSubmatch(g, -1) {
							used[s[1]] = filepath.Base(p)
						}
					} else if g != "" {
						used[g] = filepath.Base(p)
					}
				}
			}
		}
	}
	return used, all
}

func TestUsedKeysExist(t *testing.T) {
	en := catalogs["en"]
	gu, _ := goKeys(t)
	ju, _ := javaKeys(t)
	for _, u := range []map[string]string{gu, ju} {
		for k, where := range u {
			if _, ok := en[k]; !ok && !technical(k) {
				t.Errorf("%s: %q is not in en.json", where, k)
			}
		}
	}
}

// technical strings that are shown as they are on purpose
func technical(k string) bool {
	switch k {
	case "andr36oid SD Flasher", "BOOT", "system", "vendor", "userdata", "EASYROMS", "cache", "GPT":
		return true
	}
	return strings.HasPrefix(k, "%") || !strings.ContainsFunc(k, unicode.IsLetter)
}

func TestNoUnusedKeys(t *testing.T) {
	_, ga := goKeys(t)
	_, ja := javaKeys(t)
	for k := range catalogs["en"] {
		if !ga[k] && !ja[k] {
			t.Errorf("en.json key %q is not used anywhere", k)
		}
	}
}
