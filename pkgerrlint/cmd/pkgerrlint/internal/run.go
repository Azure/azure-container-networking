package internal

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	pkgerrs "github.com/pkg/errors"
)

func Run(out io.Writer, args ...string) error {
	if len(args) != 1 {
		return errors.New("usage: golintwrap <module_path>")
	}

	modulePath := args[0]
	err := inspectFiles(out, modulePath)
	if err != nil {
		pkgerrs.Wrap(err, "inspecting files")
	}
	return nil
}

func inspectFiles(out io.Writer, root string) error {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return pkgerrs.Wrapf(err, "walking %q", path)
		}

		if !info.IsDir() && filepath.Ext(path) == ".go" {
			err := inspectFile(out, path)
			if err != nil {
				return pkgerrs.Wrapf(err, "inspecting file %q", path)
			}
		}
		return nil
	})
	if err != nil {
		return pkgerrs.Wrap(err, "walking filepath")
	}
	return nil
}

func inspectFile(out io.Writer, file string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return pkgerrs.Wrapf(err, "parsing file %q", file)
	}

	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if selExpr.Sel.Name == "Errorf" {
			pkgIdent, ok := selExpr.X.(*ast.Ident)
			if ok && pkgIdent.Name == "fmt" {
				for _, arg := range callExpr.Args {
					basicLit, ok := arg.(*ast.BasicLit)
					if ok && basicLit.Kind == token.STRING && strings.Contains(basicLit.Value, "%w") {
						position := fset.Position(callExpr.Pos())
						fmt.Fprintf(out, "%s:%d:%d: use `github.com/pkg/errors.Wrap` to wrap errors instead of `fmt.Errorf`\n", position.Filename, position.Line, position.Column)
						break
					}
				}
			}
		}

		return true
	})

	return nil
}
