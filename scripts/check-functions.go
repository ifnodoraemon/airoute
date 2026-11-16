// Command check-functions ratchets existing function-size debt while rejecting
// new functions over the engineering guideline's 60-line limit.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	root := flag.String("root", ".", "repository root")
	baseline := flag.String("baseline", "scripts/function-size-baseline.txt", "baseline relative to root")
	list := flag.Bool("list", false, "print current violations for baseline review")
	flag.Parse()
	if err := checkFunctions(*root, *baseline, *list); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkFunctions(root, baseline string, list bool) error {
	functions, err := businessFunctions(root)
	if err != nil {
		return err
	}
	allowed := map[string]int{}
	if !list {
		allowed, err = readBaseline(filepath.Join(root, baseline))
		if err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(functions))
	for key := range functions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	failed := false
	for _, key := range keys {
		lines := functions[key]
		if lines <= 60 {
			continue
		}
		if list {
			fmt.Printf("%s %d\n", key, lines)
		} else if lines > allowed[key] {
			fmt.Fprintf(os.Stderr, "function size violation: %s (%d lines; limit 60, baseline %d)\n", key, lines, allowed[key])
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("function-size ratchet failed; split new or growing functions")
	}
	return nil
}

func businessFunctions(root string) (map[string]int, error) {
	result := map[string]int{}
	for _, directory := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			return collectFunctions(root, path, result)
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func collectFunctions(root, path string, result map[string]int) error {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, 0)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		name := function.Name.Name
		if function.Recv != nil {
			receiver := function.Recv.List[0].Type
			if pointer, ok := receiver.(*ast.StarExpr); ok {
				receiver = pointer.X
			}
			if identifier, ok := receiver.(*ast.Ident); ok {
				name = identifier.Name + "." + name
			}
		}
		result[filepath.ToSlash(relative)+":"+name] = positions.Position(function.End()).Line - positions.Position(function.Pos()).Line + 1
	}
	return nil
}

func readBaseline(path string) (map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := map[string]int{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid function baseline: %s", line)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil || count < 61 {
			return nil, fmt.Errorf("invalid function baseline size: %s", line)
		}
		result[fields[0]] = count
	}
	return result, scanner.Err()
}
