package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileInfo struct {
	Name      string
	Types     []TypeInfo
	Methods   map[string][]string
	Functions []string
}

type TypeInfo struct {
	Name        string
	Decl        string
	IsInterface bool
	Interface   []string
}

/*
 * Run:
 * go build -o gostruct
 * ./gostruct /home/andrii/go/src/evmtc
 */

func main() {

	//if len(os.Args) < 2 {
	//	fmt.Println("usage: gostruct <path>")
	//	os.Exit(1)
	//}

	//root := os.Args[1]
	root := "/home/andrii/go/src/evmtc"

	fmt.Println(filepath.Base(root))

	err := filepath.WalkDir(root, walk(root))
	if err != nil {
		fmt.Println(err)
	}
}

func walk(root string) fs.WalkDirFunc {

	return func(path string, d fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		if d.IsDir() {

			name := d.Name()

			if name == "vendor" || name == ".git" {
				return filepath.SkipDir
			}

			if path != root {

				printIndent(depth(root, path))
				fmt.Println(name)

				parsePackage(path, depth(root, path)+1)

				return filepath.SkipDir
			}

			// root
			parsePackage(path, 1)
		}

		return nil
	}
}

func parsePackage(dir string, indent int) {

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {

		if strings.HasSuffix(fi.Name(), "_test.go") {
			return false
		}

		return strings.HasSuffix(fi.Name(), ".go")

	}, 0)

	if err != nil {
		return
	}

	for _, pkg := range pkgs {

		var files []*FileInfo

		for fname, f := range pkg.Files {

			file := parseFile(fset, f)

			file.Name = filepath.Base(fname)

			files = append(files, file)
		}

		sort.Slice(files, func(i, j int) bool {
			return files[i].Name < files[j].Name
		})

		for _, f := range files {

			printIndent(indent)
			fmt.Println(f.Name)

			printFile(f, indent+1)
		}
	}
}

func parseFile(fset *token.FileSet, file *ast.File) *FileInfo {

	info := &FileInfo{
		Methods: map[string][]string{},
	}

	for _, decl := range file.Decls {

		switch d := decl.(type) {

		case *ast.GenDecl:

			for _, spec := range d.Specs {

				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}

				// -------------------------
				// Зміна тут: для struct друкуємо {...}
				// -------------------------
				var declStr string

				switch tsType := ts.Type.(type) {
				case *ast.StructType:
					declStr = "{...}"
				default:
					var buf bytes.Buffer
					printer.Fprint(&buf, fset, tsType)
					declStr = buf.String()
				}

				t := TypeInfo{
					Name: ts.Name.Name,
					Decl: declStr,
				}

				if iface, ok := ts.Type.(*ast.InterfaceType); ok {

					t.IsInterface = true

					for _, m := range iface.Methods.List {

						if len(m.Names) == 0 {
							continue
						}

						if !ast.IsExported(m.Names[0].Name) {
							continue
						}

						var mbuf bytes.Buffer
						printer.Fprint(&mbuf, fset, m.Type)

						t.Interface = append(t.Interface,
							fmt.Sprintf("%s%s",
								m.Names[0].Name,
								mbuf.String()))
					}

					sort.Strings(t.Interface)
				}

				info.Types = append(info.Types, t)
			}

		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}

			sig := signature(fset, d)

			if d.Recv == nil {
				info.Functions = append(info.Functions, fmt.Sprintf("func %s%s", d.Name.Name, sig))
				continue
			}

			recvType := receiverType(d)
			recv := receiverString(fset, d)

			info.Methods[recvType] = append(info.Methods[recvType],
				fmt.Sprintf("func (%s) %s%s", recv, d.Name.Name, sig))
		}
	}

	sort.Slice(info.Types, func(i, j int) bool {
		return info.Types[i].Name < info.Types[j].Name
	})

	sort.Strings(info.Functions)

	for k := range info.Methods {
		sort.Strings(info.Methods[k])
	}

	return info
}

func printFile(f *FileInfo, indent int) {

	for _, t := range f.Types {

		printIndent(indent)
		fmt.Printf("type %s %s\n", t.Name, t.Decl)

		if t.IsInterface {

			for _, m := range t.Interface {
				printIndent(indent + 1)
				fmt.Println(m)
			}

			continue
		}

		for _, m := range f.Methods[t.Name] {
			printIndent(indent + 1)
			fmt.Println(m)
		}
	}

	for typeName, methods := range f.Methods {

		found := false

		for _, t := range f.Types {
			if t.Name == typeName {
				found = true
			}
		}

		if found {
			continue
		}

		printIndent(indent)
		fmt.Printf("type %s\n", typeName)

		for _, m := range methods {
			printIndent(indent + 1)
			fmt.Println(m)
		}
	}

	for _, fn := range f.Functions {
		printIndent(indent)
		fmt.Println(fn)
	}
}

func signature(fset *token.FileSet, fn *ast.FuncDecl) string {
	var args []string
	for _, param := range fn.Type.Params.List {
		var typeBuf bytes.Buffer
		printer.Fprint(&typeBuf, fset, param.Type) // перетворюємо param.Type на string

		if len(param.Names) == 0 {
			args = append(args, typeBuf.String()) // анонімний параметр
		} else {
			for _, n := range param.Names {
				args = append(args, n.Name+" "+typeBuf.String())
			}
		}
	}

	argStr := "(" + strings.Join(args, ", ") + ")"

	// результат
	var resStr string
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		var results []string
		for _, r := range fn.Type.Results.List {
			var rbuf bytes.Buffer
			printer.Fprint(&rbuf, fset, r.Type)

			if len(r.Names) == 0 {
				results = append(results, rbuf.String())
			} else {
				for _, rn := range r.Names {
					results = append(results, rn.Name+" "+rbuf.String())
				}
			}
		}

		if len(results) == 1 {
			resStr = " " + results[0]
		} else {
			resStr = " (" + strings.Join(results, ", ") + ")"
		}
	}

	return argStr + resStr
}

func receiverString(fset *token.FileSet, fn *ast.FuncDecl) string {

	var buf bytes.Buffer
	printer.Fprint(&buf, fset, fn.Recv.List[0].Type)

	return buf.String()
}

func receiverType(fn *ast.FuncDecl) string {

	switch t := fn.Recv.List[0].Type.(type) {

	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}

	case *ast.Ident:
		return t.Name
	}

	return ""
}

func depth(root, path string) int {

	rel, _ := filepath.Rel(root, path)

	if rel == "." {
		return 0
	}

	return strings.Count(rel, string(os.PathSeparator)) + 1
}

func printIndent(n int) {

	for i := 0; i < n; i++ {
		fmt.Print("\t")
	}
}
