// go-tokens: proves that two Go source trees differ ONLY in the text of
// string/char literals (and comments).
//
// How: every .go file is tokenized with the standard library scanner
// (go/scanner). Comments are dropped and the text of every string/char literal
// is replaced by a placeholder; the remaining token stream (keywords,
// identifiers, operators, numbers, punctuation) must be identical.
//
// FAIL (security criteria):
//   - any change in code tokens, any added or deleted .go file;
//   - a changed literal that did NOT contain Chinese text before
//     (only Chinese text may have been translated, nothing else);
//   - a URL or domain that is not present in the base tree.
//
// REVIEW (functional, listed but not fatal):
//   - changed literals that sit in a "logic position" (==, !=, switch case,
//     map key, index, strings.HasPrefix/Contains/..., SQL/regexp calls).
//     These are listed so a human can confirm they are display text.
//
// Usage: go run . <base-tree> <new-tree>
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type tok struct {
	t   token.Token
	lit string
}

var (
	// Chinese ideographs + CJK punctuation + full-width forms
	cjk    = regexp.MustCompile(`[\x{3000}-\x{303f}\x{4e00}-\x{9fff}\x{ff00}-\x{ffef}]`)
	urlRe  = regexp.MustCompile(`(?i)\b(?:https?|wss?|ftp)://[A-Za-z0-9._~:/?#\[\]@!$&*+,;=%-]+`)
	domRe  = regexp.MustCompile(`(?i)\b[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:com|net|org|io|cn|ru|xyz|top|app|dev|me|co|cc|tk|gg|sh)\b`)
	logicF = regexp.MustCompile(`^(HasPrefix|HasSuffix|TrimPrefix|TrimSuffix|Contains|ContainsAny|EqualFold|Index|Split|SplitN|Replace|ReplaceAll|MustCompile|Compile|Getenv|Setenv|LookupEnv|Open|OpenFile|Command|CommandContext|NewRequest|NewRequestWithContext|Get|Post|Query|QueryRow|QueryContext|QueryRowContext|Exec|ExecContext|Prepare|PrepareContext|Set|Add|Del)$`)
)

func codeTokens(path string) ([]tok, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f := fset.AddFile(path, fset.Base(), len(src))
	var s scanner.Scanner
	nerr := 0
	s.Init(f, src, func(token.Position, string) { nerr++ }, 0) // mode 0: comments skipped
	var out []tok
	for {
		_, t, lit := s.Scan()
		if t == token.EOF {
			break
		}
		switch t {
		case token.STRING, token.CHAR:
			out = append(out, tok{t, "<lit>"})
		case token.SEMICOLON:
			out = append(out, tok{t, ""})
		default:
			out = append(out, tok{t, lit})
		}
	}
	if nerr > 0 {
		return nil, fmt.Errorf("%d syntax errors", nerr)
	}
	return out, nil
}

type lit struct{ val, ctx string }

// literals returns every string/char literal in source order with the
// "logic context" it appears in ("" = ordinary value / display text).
func literals(path string) ([]lit, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []lit
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if bl, ok := n.(*ast.BasicLit); ok && (bl.Kind == token.STRING || bl.Kind == token.CHAR) {
			ctx := ""
			if len(stack) > 0 {
				switch p := stack[len(stack)-1].(type) {
				case *ast.BinaryExpr:
					if p.Op == token.EQL || p.Op == token.NEQ {
						ctx = "comparison"
					}
				case *ast.CaseClause:
					ctx = "switch-case"
				case *ast.KeyValueExpr:
					if p.Key == bl {
						ctx = "key"
					}
				case *ast.IndexExpr:
					if p.Index == bl {
						ctx = "index"
					}
				case *ast.ImportSpec:
					ctx = "import"
				case *ast.CallExpr:
					name := ""
					switch fn := p.Fun.(type) {
					case *ast.SelectorExpr:
						name = fn.Sel.Name
					case *ast.Ident:
						name = fn.Name
					}
					if logicF.MatchString(name) {
						ctx = "call " + name
					}
				}
			}
			out = append(out, lit{bl.Value, ctx})
		}
		stack = append(stack, n)
		return true
	})
	return out, nil
}

func goFiles(root string) map[string]bool {
	out := map[string]bool{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "verify") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") {
			rel, _ := filepath.Rel(root, p)
			out[rel] = true
		}
		return nil
	})
	return out
}

func addURLs(m map[string]bool, s string) {
	for _, u := range urlRe.FindAllString(s, -1) {
		m[strings.ToLower(strings.TrimRight(u, ".,;:!?'"))] = true
	}
	for _, d := range domRe.FindAllString(s, -1) {
		m[strings.ToLower(d)] = true
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("usage: go run . <base-tree> <new-tree>")
		os.Exit(2)
	}
	base, neu := os.Args[1], os.Args[2]
	bf, nf := goFiles(base), goFiles(neu)
	var files []string
	for f := range nf {
		files = append(files, f)
	}
	sort.Strings(files)

	fail := false
	var review []string
	identical, textOnly, changed := 0, 0, 0
	bURL, nURL := map[string]bool{}, map[string]bool{}
	for f := range bf {
		if !nf[f] {
			fmt.Printf("FAIL deleted file: %s\n", f)
			fail = true
		}
	}
	for _, f := range files {
		if !bf[f] {
			fmt.Printf("FAIL new file: %s\n", f)
			fail = true
			continue
		}
		bp, np := filepath.Join(base, f), filepath.Join(neu, f)
		bt, e1 := codeTokens(bp)
		nt, e2 := codeTokens(np)
		if e1 != nil || e2 != nil {
			fmt.Printf("FAIL %s: %v %v\n", f, e1, e2)
			fail = true
			continue
		}
		same := len(bt) == len(nt)
		for i := 0; same && i < len(bt); i++ {
			same = bt[i] == nt[i]
		}
		if !same {
			fmt.Printf("FAIL code changed: %s\n", f)
			fail = true
			continue
		}
		bl, e1 := literals(bp)
		nl, e2 := literals(np)
		if e1 != nil || e2 != nil || len(bl) != len(nl) {
			fmt.Printf("FAIL cannot pair literals: %s\n", f)
			fail = true
			continue
		}
		n := 0
		for i := range bl {
			addURLs(bURL, bl[i].val)
			addURLs(nURL, nl[i].val)
			if bl[i].val == nl[i].val {
				continue
			}
			n++
			if !cjk.MatchString(bl[i].val) {
				fmt.Printf("FAIL non-Chinese literal changed in %s: %.80s\n", f, bl[i].val)
				fail = true
			}
			if bl[i].ctx != "" {
				review = append(review, fmt.Sprintf("%s [%s]\n      before: %.110s\n      after:  %.110s", f, bl[i].ctx, bl[i].val, nl[i].val))
			}
		}
		if n == 0 {
			identical++
		} else {
			textOnly++
			changed += n
		}
	}
	var newURLs []string
	for u := range nURL {
		if !bURL[u] {
			newURLs = append(newURLs, u)
		}
	}
	sort.Strings(newURLs)
	for _, u := range newURLs {
		fmt.Printf("FAIL new URL/domain: %s\n", u)
		fail = true
	}
	if len(review) > 0 {
		fmt.Printf("\nREVIEW - %d changed literals in logic positions (confirm they are display text):\n", len(review))
		for _, r := range review {
			fmt.Println("  " + r)
		}
	}
	fmt.Printf("\nGo files: %d | code identical: %d | only text literals changed: %d (%d literals) | new URLs: %d\n",
		len(files), identical, textOnly, changed, len(newURLs))
	if fail {
		fmt.Println("RESULT: FAIL")
		os.Exit(1)
	}
	fmt.Println("RESULT: PASS - the code is token-identical; only Chinese text literals were translated; no new URLs.")
}
