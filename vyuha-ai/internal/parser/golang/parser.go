package golang

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/vyuha/vyuha-ai/internal/graph"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// ProgressFn is called after each file finishes processing. It receives the
// number of files processed so far, the total file count, and the path of
// the file that was just completed.
type ProgressFn func(filesProcessed, totalFiles int, currentFile string)

// ParseResult contains the complete output of a Parse run.
type ParseResult struct {
	Nodes       []*graph.Node
	Edges       []*graph.Edge
	Checksums   map[string]string // relative file path → SHA-256 hex digest
	FileCount   int
	ParseErrors []ParseError
	ASTFiles    map[string]*ast.File // relative file path → parsed AST (populated during parse)
}

// ParseError records a single error encountered while parsing a Go file.
type ParseError struct {
	FilePath string
	Line     int
	Message  string
}

// ---------------------------------------------------------------------------
// Internal types
// ---------------------------------------------------------------------------

// parseResult bundles the output of a single-file parse operation sent
// through the worker-pool channel.
type parseResult struct {
	nodes   []*graph.Node
	edges   []*graph.Edge
	errors  []ParseError
	astFile *ast.File
	relPath string
}

// fileJob is the unit of work dispatched to a parsing worker.
type fileJob struct {
	absPath    string // absolute path on disk
	relPath    string // forward-slash relative path from repository root
	content    []byte
	importPath string // Go import path for the enclosing package
	pkgNodeID  string // node ID of the enclosing package node
}

// callInfo describes one call site extracted from a function body.
type callInfo struct {
	name     string // "Function" or "receiver.Method"
	line     int
	isGo     bool
	isDefer  bool
	callType string // "function_call" | "method_call"
}

// ---------------------------------------------------------------------------
// Built-in function set (skipped during call extraction)
// ---------------------------------------------------------------------------

var builtinFuncs = map[string]bool{
	"make":    true,
	"len":     true,
	"cap":     true,
	"append":  true,
	"copy":    true,
	"close":   true,
	"delete":  true,
	"panic":   true,
	"recover": true,
	"new":     true,
	"print":   true,
	"println": true,
	"complex": true,
	"real":    true,
	"imag":    true,
}

// ---------------------------------------------------------------------------
// GoParser
// ---------------------------------------------------------------------------

// GoParser parses a Go repository rooted at rootPath and produces an
// AST-level code-intelligence graph of nodes and edges.
type GoParser struct {
	rootPath   string
	modulePath string
	fset       *token.FileSet
	mu         sync.Mutex
}

// ModulePath returns the Go module path for this parser.
func (p *GoParser) ModulePath() string { return p.modulePath }

// Fset returns the shared token.FileSet used for position tracking.
func (p *GoParser) Fset() *token.FileSet { return p.fset }

// New creates a GoParser for the repository at rootPath. It reads go.mod to
// determine the module path used as the canonical prefix for all node IDs.
func New(rootPath string) (*GoParser, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("goparser: resolve root path: %w", err)
	}

	modPath, err := resolveModulePath(absRoot)
	if err != nil {
		return nil, fmt.Errorf("goparser: %w", err)
	}

	return &GoParser{
		rootPath:   absRoot,
		modulePath: modPath,
		fset:       token.NewFileSet(),
	}, nil
}

// ---------------------------------------------------------------------------
// Parse — main entry point
// ---------------------------------------------------------------------------

// Parse walks the repository, extracts the full AST-level code-intelligence
// graph and returns it as a ParseResult.
//
// checksums maps relative file paths to their previously-known SHA-256 hex
// digests. Files whose content has not changed are skipped (incremental
// mode). Pass nil to force a full parse.
//
// progress (optional) is called after every file finishes processing.
func (p *GoParser) Parse(
	ctx context.Context,
	checksums map[string]string,
	progress ProgressFn,
) (*ParseResult, error) {
	parseStart := time.Now()
	if checksums == nil {
		checksums = make(map[string]string)
	}

	// ---- 1. Repository node ---------------------------------------------
	repoID := "repo:" + p.modulePath
	repoNode := graph.NewNode(repoID, graph.NodeTypeRepository, p.modulePath)
	repoNode.Language = "go"
	repoNode.Depth = 0

	allNodes := []*graph.Node{repoNode}
	var allEdges []*graph.Edge

	// ---- 2. Detect services (directories with package main) -------------
	services, err := detectServices(p.rootPath, p.modulePath)
	if err != nil {
		return nil, fmt.Errorf("goparser: detect services: %w", err)
	}
	for _, svc := range services {
		svc.ParentID = repoID
		allNodes = append(allNodes, svc)
		allEdges = append(allEdges, newContainsEdge(repoID, svc.ID, 1))
	}

	// ---- 3. Collect .go files -------------------------------------------
	goFiles, err := collectGoFiles(p.rootPath)
	if err != nil {
		return nil, fmt.Errorf("goparser: collect files: %w", err)
	}

	// Build per-directory package information.
	type dirInfo struct {
		importPath string
		pkgNodeID  string
	}
	dirMap := make(map[string]*dirInfo) // abs dir → info
	pkgNodes := make(map[string]*graph.Node)

	for _, absFile := range goFiles {
		dir := filepath.Dir(absFile)
		if _, ok := dirMap[dir]; ok {
			continue
		}

		relDir, _ := filepath.Rel(p.rootPath, dir)
		relDir = filepath.ToSlash(relDir)

		var importPath string
		if relDir == "." {
			importPath = p.modulePath
		} else {
			importPath = p.modulePath + "/" + relDir
		}

		pkgID := "pkg:" + importPath
		dirMap[dir] = &dirInfo{importPath: importPath, pkgNodeID: pkgID}

		if _, exists := pkgNodes[pkgID]; !exists {
			pkgNode := graph.NewNode(pkgID, graph.NodeTypePackage, filepath.Base(dir))
			pkgNode.Language = "go"
			pkgNode.ParentID = repoID
			pkgNode.Depth = 1
			if relDir == "." {
				pkgNode.FilePath = "."
			} else {
				pkgNode.FilePath = relDir
			}
			pkgNodes[pkgID] = pkgNode
		}
	}

	for _, pkg := range pkgNodes {
		allNodes = append(allNodes, pkg)
		allEdges = append(allEdges, newContainsEdge(repoID, pkg.ID, 1))
	}

	// ---- 4. Incremental checksum filter ---------------------------------
	newChecksums := make(map[string]string, len(goFiles))
	var jobs []fileJob
	totalFiles := len(goFiles)
	var skippedCount int32

	for _, absFile := range goFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		relPath, err := filepath.Rel(p.rootPath, absFile)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)

		content, err := os.ReadFile(absFile)
		if err != nil {
			continue // skip unreadable files
		}

		sum := computeChecksum(content)
		newChecksums[relPath] = sum

		if old, ok := checksums[relPath]; ok && old == sum {
			// File unchanged — skip parsing.
			skippedCount++
			if progress != nil {
				progress(int(skippedCount), totalFiles, relPath)
			}
			continue
		}

		dir := filepath.Dir(absFile)
		di := dirMap[dir]
		jobs = append(jobs, fileJob{
			absPath:    absFile,
			relPath:    relPath,
			content:    content,
			importPath: di.importPath,
			pkgNodeID:  di.pkgNodeID,
		})
	}

	// ---- 5. Early return when nothing to parse --------------------------
	if len(jobs) == 0 {
		return &ParseResult{
			Nodes:     allNodes,
			Edges:     allEdges,
			Checksums: newChecksums,
			FileCount: totalFiles,
			ASTFiles:  make(map[string]*ast.File),
		}, nil
	}

	// ---- 6. Parallel parse with worker pool -----------------------------
	numWorkers := runtime.NumCPU()
	if numWorkers > len(jobs) {
		numWorkers = len(jobs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobCh := make(chan fileJob, len(jobs))
	resultCh := make(chan *parseResult, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				res := p.parseFile(job)
				resultCh <- res
			}
		}()
	}

	// Enqueue jobs.
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	// Close the result channel once all workers have finished.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// ---- 7. Collect results ---------------------------------------------
	var (
		parseErrors []ParseError
		processed   int64
	)
	atomic.StoreInt64(&processed, int64(skippedCount))
	astFiles := make(map[string]*ast.File, len(jobs))

	for res := range resultCh {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if res.errors != nil {
			parseErrors = append(parseErrors, res.errors...)
		}
		allNodes = append(allNodes, res.nodes...)
		allEdges = append(allEdges, res.edges...)

		if res.astFile != nil && res.relPath != "" {
			astFiles[res.relPath] = res.astFile
		}

		count := int(atomic.AddInt64(&processed, 1))
		if progress != nil {
			file := ""
			if len(res.nodes) > 0 {
				file = res.nodes[0].FilePath
			}
			p.mu.Lock()
			progress(count, totalFiles, file)
			p.mu.Unlock()
		}
	}

	slog.Info("parse complete",
		"files", totalFiles,
		"nodes", len(allNodes),
		"edges", len(allEdges),
		"errors", len(parseErrors),
		"duration_ms", time.Since(parseStart).Milliseconds(),
	)

	return &ParseResult{
		Nodes:       allNodes,
		Edges:       allEdges,
		Checksums:   newChecksums,
		FileCount:   totalFiles,
		ParseErrors: parseErrors,
		ASTFiles:    astFiles,
	}, nil
}

// ---------------------------------------------------------------------------
// parseFile — single-file AST extraction
// ---------------------------------------------------------------------------

func (p *GoParser) parseFile(job fileJob) *parseResult {
	var (
		nodes  []*graph.Node
		edges  []*graph.Edge
		errors []ParseError
	)

	// -- Parse AST --------------------------------------------------------
	astFile, err := goparser.ParseFile(
		p.fset, job.absPath, job.content,
		goparser.ParseComments|goparser.AllErrors,
	)
	if err != nil {
		if errList, ok := err.(scanner.ErrorList); ok {
			for _, e := range errList {
				errors = append(errors, ParseError{
					FilePath: job.relPath,
					Line:     e.Pos.Line,
					Message:  e.Msg,
				})
			}
		} else {
			errors = append(errors, ParseError{
				FilePath: job.relPath,
				Message:  err.Error(),
			})
		}
		// Continue with partial AST if available.
		if astFile == nil {
			return &parseResult{errors: errors}
		}
	}

	contentStr := string(job.content)
	lines := strings.Split(contentStr, "\n")
	checksum := computeChecksum(job.content)

	// -- File node --------------------------------------------------------
	fileID := fmt.Sprintf("file:%s/%s", p.modulePath, job.relPath)
	fileNode := graph.NewNode(fileID, graph.NodeTypeFile, filepath.Base(job.relPath))
	fileNode.FilePath = job.relPath
	fileNode.Language = "go"
	fileNode.ParentID = job.pkgNodeID
	fileNode.Depth = 2
	fileNode.Metadata.Checksum = checksum
	fileNode.Metadata.SizeBytes = int64(len(job.content))

	if astFile.Name != nil {
		fileNode.Metadata.PackageName = astFile.Name.Name
	}

	// Detect generated files.
	for _, cg := range astFile.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") && strings.Contains(c.Text, "DO NOT EDIT") {
				fileNode.Metadata.IsGenerated = true
				break
			}
		}
		if fileNode.Metadata.IsGenerated {
			break
		}
	}

	// -- Imports ----------------------------------------------------------
	var imports []graph.ImportSpec
	for _, imp := range astFile.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		imports = append(imports, graph.ImportSpec{Path: impPath, Alias: alias})

		// IMPORTS edge: file → imported package.
		targetID := "pkg:" + impPath
		edge := graph.NewEdge(fileID, targetID, graph.EdgeTypeImports)
		edge.Metadata.Alias = alias
		edge.Metadata.IsStdlib = isStdlibImport(impPath)
		edge.Metadata.IsThirdParty = !isStdlibImport(impPath) && !strings.HasPrefix(impPath, p.modulePath)
		edges = append(edges, edge)
	}
	fileNode.Metadata.Imports = imports

	// Contains edge: package → file.
	edges = append(edges, newContainsEdge(job.pkgNodeID, fileID, 2))

	// -- Walk declarations ------------------------------------------------
	var funcCount, structCount, ifaceCount int

	for _, decl := range astFile.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			funcCount++
			fNode, fEdges := p.extractFunction(d, job, fileID, lines)
			if fNode != nil {
				nodes = append(nodes, fNode)
				edges = append(edges, fEdges...)
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch st := ts.Type.(type) {
				case *ast.StructType:
					structCount++
					sNode := p.extractStruct(ts, st, d, job, fileID)
					if sNode != nil {
						nodes = append(nodes, sNode)
						edges = append(edges, newContainsEdge(fileID, sNode.ID, 3))
					}

				case *ast.InterfaceType:
					ifaceCount++
					iNode := p.extractInterface(ts, st, d, job, fileID)
					if iNode != nil {
						nodes = append(nodes, iNode)
						edges = append(edges, newContainsEdge(fileID, iNode.ID, 3))
					}
				}
			}
		}
	}

	fileNode.Metadata.FunctionCount = funcCount
	_ = structCount // stored implicitly via child struct nodes
	_ = ifaceCount  // stored implicitly via child interface nodes

	// File node is always the first in the returned slice.
	nodes = append([]*graph.Node{fileNode}, nodes...)

	return &parseResult{
		nodes:   nodes,
		edges:   edges,
		errors:  errors,
		astFile: astFile,
		relPath: job.relPath,
	}
}

// ---------------------------------------------------------------------------
// extractFunction — builds a Function node + call edges from ast.FuncDecl
// ---------------------------------------------------------------------------

func (p *GoParser) extractFunction(
	decl *ast.FuncDecl,
	job fileJob,
	fileID string,
	lines []string,
) (*graph.Node, []*graph.Edge) {
	name := decl.Name.Name

	// -- Receiver ---------------------------------------------------------
	var receiver string     // full receiver including * for pointer receivers
	var receiverBase string // base type name (for node ID)
	var isMethod bool

	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		isMethod = true
		recvField := decl.Recv.List[0]
		recvType := recvField.Type

		if star, ok := recvType.(*ast.StarExpr); ok {
			receiverBase = typeToString(star.X, p.fset)
			receiver = "*" + receiverBase
		} else {
			receiverBase = typeToString(recvType, p.fset)
			receiver = receiverBase
		}
	}

	// -- Node ID ----------------------------------------------------------
	var nodeID string
	if receiverBase != "" {
		nodeID = fmt.Sprintf("func:%s:%s.%s", job.importPath, receiverBase, name)
	} else {
		nodeID = fmt.Sprintf("func:%s:%s", job.importPath, name)
	}

	funcNode := graph.NewNode(nodeID, graph.NodeTypeFunction, name)
	funcNode.FilePath = job.relPath
	funcNode.Language = "go"
	funcNode.ParentID = fileID
	funcNode.Depth = 3
	funcNode.IsExported = isExported(name)

	// -- Position ---------------------------------------------------------
	startPos := p.fset.Position(decl.Pos())
	endPos := p.fset.Position(decl.End())
	funcNode.LineStart = startPos.Line
	funcNode.LineEnd = endPos.Line

	// -- Core metadata ----------------------------------------------------
	funcNode.Metadata.IsMethod = isMethod
	funcNode.Metadata.Receiver = receiver
	funcNode.Metadata.DocComment = extractDocComment(decl.Doc)

	// -- Parameters -------------------------------------------------------
	var params []graph.Parameter
	if decl.Type.Params != nil {
		for _, field := range decl.Type.Params.List {
			typStr := typeToString(field.Type, p.fset)
			if len(field.Names) == 0 {
				// Unnamed parameter.
				params = append(params, graph.Parameter{Type: typStr})
			} else {
				for _, n := range field.Names {
					params = append(params, graph.Parameter{Name: n.Name, Type: typStr})
				}
			}
		}
	}
	funcNode.Metadata.Parameters = params

	// -- Return types -----------------------------------------------------
	var retTypes []string
	if decl.Type.Results != nil {
		for _, field := range decl.Type.Results.List {
			typStr := typeToString(field.Type, p.fset)
			if len(field.Names) == 0 {
				retTypes = append(retTypes, typStr)
			} else {
				for range field.Names {
					retTypes = append(retTypes, typStr)
				}
			}
		}
	}
	funcNode.Metadata.ReturnTypes = retTypes

	// -- Signature --------------------------------------------------------
	funcNode.Metadata.Signature = buildSignature(name, receiver, params, retTypes)

	// -- has_error_return -------------------------------------------------
	if len(retTypes) > 0 && retTypes[len(retTypes)-1] == "error" {
		funcNode.Metadata.HasErrorReturn = true
	}

	// -- has_context_param ------------------------------------------------
	if len(params) > 0 && strings.Contains(params[0].Type, "Context") {
		funcNode.Metadata.HasContextParam = true
	}

	// -- Cyclomatic complexity --------------------------------------------
	funcNode.Metadata.CyclomaticComplexity = computeCyclomaticComplexity(decl.Body)

	// -- Source snippet ---------------------------------------------------
	if funcNode.LineStart >= 1 && funcNode.LineEnd <= len(lines) {
		snippet := strings.Join(lines[funcNode.LineStart-1:funcNode.LineEnd], "\n")
		funcNode.Metadata.SourceSnippet = snippet
	}

	// -- Edges ------------------------------------------------------------
	var edges []*graph.Edge

	// Contains edge: file → function.
	edges = append(edges, newContainsEdge(fileID, nodeID, 3))

	// -- Call edges (AST-level, unresolved) --------------------------------
	if decl.Body != nil {
		calls := extractCalls(decl.Body, p.fset)
		var callNames []string

		for _, ci := range calls {
			if ci.name == "" {
				continue
			}
			callNames = append(callNames, ci.name)

			// Best-effort target ID.
			// Direct calls → same package; selector calls → unresolved.
			targetID := fmt.Sprintf("func:%s:%s", job.importPath, ci.name)

			callEdge := graph.NewEdge(nodeID, targetID, graph.EdgeTypeCalls)
			callEdge.Metadata.CallType = ci.callType
			callEdge.Metadata.IsResolved = false
			callEdge.Metadata.CallSiteLine = ci.line
			callEdge.Metadata.IsGoroutine = ci.isGo
			callEdge.Metadata.IsDeferred = ci.isDefer
			edges = append(edges, callEdge)
		}

		funcNode.Metadata.Calls = callNames
	}

	return funcNode, edges
}

// ---------------------------------------------------------------------------
// extractStruct — Struct node from ast.TypeSpec + ast.StructType
// ---------------------------------------------------------------------------

func (p *GoParser) extractStruct(
	ts *ast.TypeSpec,
	st *ast.StructType,
	decl *ast.GenDecl,
	job fileJob,
	fileID string,
) *graph.Node {
	name := ts.Name.Name
	nodeID := fmt.Sprintf("struct:%s:%s", job.importPath, name)

	node := graph.NewNode(nodeID, graph.NodeTypeStruct, name)
	node.FilePath = job.relPath
	node.Language = "go"
	node.ParentID = fileID
	node.IsExported = isExported(name)
	node.Depth = 3

	startPos := p.fset.Position(ts.Pos())
	endPos := p.fset.Position(ts.End())
	node.LineStart = startPos.Line
	node.LineEnd = endPos.Line

	// Doc comment — prefer TypeSpec doc, fall back to GenDecl doc.
	doc := ts.Doc
	if doc == nil {
		doc = decl.Doc
	}
	node.Metadata.DocComment = extractDocComment(doc)

	// -- Fields -----------------------------------------------------------
	var fields []graph.StructField
	var embeds []string

	if st.Fields != nil {
		for _, field := range st.Fields.List {
			typStr := typeToString(field.Type, p.fset)

			// Raw struct tag.
			tagStr := ""
			if field.Tag != nil {
				tagStr = strings.Trim(field.Tag.Value, "`")
			}

			if len(field.Names) == 0 {
				// Embedded field — derive a friendly name.
				embedName := typStr
				embedName = strings.TrimPrefix(embedName, "*")
				if idx := strings.LastIndex(embedName, "."); idx >= 0 {
					embedName = embedName[idx+1:]
				}

				embeds = append(embeds, typStr)
				fields = append(fields, graph.StructField{
					Name:       embedName,
					Type:       typStr,
					Tag:        tagStr,
					IsExported: isExported(embedName),
					DocComment: extractDocComment(field.Doc),
				})
			} else {
				for _, n := range field.Names {
					fields = append(fields, graph.StructField{
						Name:       n.Name,
						Type:       typStr,
						Tag:        tagStr,
						IsExported: isExported(n.Name),
						DocComment: extractDocComment(field.Doc),
					})
				}
			}
		}
	}

	node.Metadata.Fields = fields
	node.Metadata.Embeds = embeds

	return node
}

// ---------------------------------------------------------------------------
// extractInterface — Interface node from ast.TypeSpec + ast.InterfaceType
// ---------------------------------------------------------------------------

func (p *GoParser) extractInterface(
	ts *ast.TypeSpec,
	it *ast.InterfaceType,
	decl *ast.GenDecl,
	job fileJob,
	fileID string,
) *graph.Node {
	name := ts.Name.Name
	nodeID := fmt.Sprintf("interface:%s:%s", job.importPath, name)

	node := graph.NewNode(nodeID, graph.NodeTypeInterface, name)
	node.FilePath = job.relPath
	node.Language = "go"
	node.ParentID = fileID
	node.IsExported = isExported(name)
	node.Depth = 3

	startPos := p.fset.Position(ts.Pos())
	endPos := p.fset.Position(ts.End())
	node.LineStart = startPos.Line
	node.LineEnd = endPos.Line

	// Doc comment — prefer TypeSpec doc, fall back to GenDecl doc.
	doc := ts.Doc
	if doc == nil {
		doc = decl.Doc
	}
	node.Metadata.DocComment = extractDocComment(doc)

	// -- Methods & embedded interfaces ------------------------------------
	var methods []graph.InterfaceMethod
	var embeddedIfaces []string

	if it.Methods != nil {
		for _, field := range it.Methods.List {
			if len(field.Names) == 0 {
				// Embedded interface reference.
				embeddedIfaces = append(embeddedIfaces, typeToString(field.Type, p.fset))
				continue
			}

			ft, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}

			for _, n := range field.Names {
				method := graph.InterfaceMethod{
					Name:      n.Name,
					Signature: n.Name + formatFuncType(ft, p.fset),
				}

				// Parameters.
				if ft.Params != nil {
					for _, pf := range ft.Params.List {
						typStr := typeToString(pf.Type, p.fset)
						if len(pf.Names) == 0 {
							method.Parameters = append(method.Parameters, graph.Parameter{Type: typStr})
						} else {
							for _, pn := range pf.Names {
								method.Parameters = append(method.Parameters, graph.Parameter{
									Name: pn.Name,
									Type: typStr,
								})
							}
						}
					}
				}

				// Return types.
				if ft.Results != nil {
					for _, rf := range ft.Results.List {
						typStr := typeToString(rf.Type, p.fset)
						if len(rf.Names) == 0 {
							method.ReturnTypes = append(method.ReturnTypes, typStr)
						} else {
							for range rf.Names {
								method.ReturnTypes = append(method.ReturnTypes, typStr)
							}
						}
					}
				}

				methods = append(methods, method)
			}
		}
	}

	node.Metadata.InterfaceMethods = methods
	node.Metadata.EmbedsInterfaces = embeddedIfaces

	return node
}

// ---------------------------------------------------------------------------
// extractCalls — call site extraction from function bodies
// ---------------------------------------------------------------------------

// extractCalls walks an ast.BlockStmt and returns every call site found,
// correctly tagging calls inside go and defer statements.
func extractCalls(body *ast.BlockStmt, fset *token.FileSet) []callInfo {
	if body == nil {
		return nil
	}

	var calls []callInfo
	processed := make(map[*ast.CallExpr]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			processed[x.Call] = true
			if ci := extractSingleCall(x.Call, fset); ci != nil {
				ci.isGo = true
				calls = append(calls, *ci)
			}

		case *ast.DeferStmt:
			processed[x.Call] = true
			if ci := extractSingleCall(x.Call, fset); ci != nil {
				ci.isDefer = true
				calls = append(calls, *ci)
			}

		case *ast.CallExpr:
			if !processed[x] {
				if ci := extractSingleCall(x, fset); ci != nil {
					calls = append(calls, *ci)
				}
			}
		}
		return true
	})

	return calls
}

// extractSingleCall resolves the target name and kind of a single
// ast.CallExpr. Returns nil for built-in calls and unrecognisable
// expressions (e.g. function literal invocations).
func extractSingleCall(call *ast.CallExpr, fset *token.FileSet) *callInfo {
	var name, ct string

	switch fn := call.Fun.(type) {
	case *ast.Ident:
		// Direct call: Function()
		if isBuiltinCall(fn.Name) {
			return nil
		}
		name = fn.Name
		ct = "function_call"

	case *ast.SelectorExpr:
		// Selector call: x.Method() or pkg.Function()
		if ident, ok := fn.X.(*ast.Ident); ok {
			name = ident.Name + "." + fn.Sel.Name
		} else {
			// Chained expression — record only the final selector.
			name = fn.Sel.Name
		}
		ct = "method_call"

	default:
		// Function literal call, index expression, etc. — skip.
		return nil
	}

	if name == "" {
		return nil
	}

	pos := fset.Position(call.Pos())
	return &callInfo{
		name:     name,
		line:     pos.Line,
		callType: ct,
	}
}

// ---------------------------------------------------------------------------
// resolveModulePath
// ---------------------------------------------------------------------------

// resolveModulePath reads go.mod from rootPath and extracts the module
// directive (e.g. "github.com/myorg/myapp").
func resolveModulePath(rootPath string) (string, error) {
	modFile := filepath.Join(rootPath, "go.mod")
	content, err := os.ReadFile(modFile)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("module directive not found in %s", modFile)
}

// ---------------------------------------------------------------------------
// detectServices
// ---------------------------------------------------------------------------

// detectServices walks the repository tree and creates a Service node for
// every directory that contains at least one file declaring package main.
func detectServices(rootPath, modulePath string) ([]*graph.Node, error) {
	seen := make(map[string]bool) // abs dir → already recorded
	var services []*graph.Node

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		dir := filepath.Dir(path)
		if seen[dir] {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		// Fast parse — only the package clause.
		fset := token.NewFileSet()
		f, parseErr := goparser.ParseFile(fset, path, content, goparser.PackageClauseOnly)
		if parseErr != nil || f.Name == nil {
			return nil
		}

		if f.Name.Name != "main" {
			return nil
		}

		seen[dir] = true

		relDir, _ := filepath.Rel(rootPath, dir)
		relDir = filepath.ToSlash(relDir)

		var serviceID string
		if relDir == "." {
			serviceID = "service:" + modulePath
		} else {
			serviceID = "service:" + modulePath + "/" + relDir
		}

		svcNode := graph.NewNode(serviceID, graph.NodeTypeService, filepath.Base(dir))
		svcNode.Language = "go"
		svcNode.Depth = 1
		svcNode.Metadata.IsMainPackage = true

		if relDir == "." {
			svcNode.FilePath = "."
		} else {
			svcNode.FilePath = relDir
		}

		// Best-effort entry point detection.
		entryRel, _ := filepath.Rel(rootPath, path)
		if entryRel != "" && strings.Contains(string(content), "func main()") {
			svcNode.Metadata.EntryPoint = filepath.ToSlash(entryRel)
		}

		services = append(services, svcNode)
		return nil
	})

	return services, err
}

// ---------------------------------------------------------------------------
// collectGoFiles
// ---------------------------------------------------------------------------

// collectGoFiles returns the absolute paths of all .go files under rootPath,
// skipping hidden directories, vendor, node_modules, and testdata.
func collectGoFiles(rootPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// ---------------------------------------------------------------------------
// Pure helper functions
// ---------------------------------------------------------------------------

// typeToString converts an ast.Expr representing a type into its Go source
// string representation.
func typeToString(expr ast.Expr, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}
	return types.ExprString(expr)
}

// extractDocComment returns the trimmed text of a comment group, or "".
func extractDocComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Text())
}

// computeChecksum returns the hex-encoded SHA-256 digest of content.
func computeChecksum(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h)
}

// computeCyclomaticComplexity walks an AST block statement and computes
// McCabe cyclomatic complexity.
//
//	base = 1
//	+1 for each: if, for, range, case, comm clause, &&, ||, select
func computeCyclomaticComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1
	}
	complexity := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		case *ast.CommClause:
			complexity++
		case *ast.SelectStmt:
			complexity++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// isBuiltinCall reports whether name refers to a Go built-in function.
func isBuiltinCall(name string) bool {
	return builtinFuncs[name]
}

// isExported reports whether name starts with an upper-case Unicode letter.
func isExported(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

// isStdlibImport returns true when the import path belongs to the Go
// standard library (heuristic: the first path element contains no dot).
func isStdlibImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

// formatFuncType formats an *ast.FuncType as a Go-style signature string,
// e.g. "(ctx context.Context, id string) (error)".
func formatFuncType(ft *ast.FuncType, fset *token.FileSet) string {
	var b strings.Builder
	b.WriteByte('(')
	if ft.Params != nil {
		writeFieldList(&b, ft.Params, fset)
	}
	b.WriteByte(')')
	if ft.Results != nil && len(ft.Results.List) > 0 {
		b.WriteByte(' ')
		if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
			b.WriteString(typeToString(ft.Results.List[0].Type, fset))
		} else {
			b.WriteByte('(')
			writeFieldList(&b, ft.Results, fset)
			b.WriteByte(')')
		}
	}
	return b.String()
}

// writeFieldList writes a comma-separated "name Type" list to b.
func writeFieldList(b *strings.Builder, fl *ast.FieldList, fset *token.FileSet) {
	for i, field := range fl.List {
		if i > 0 {
			b.WriteString(", ")
		}
		typStr := typeToString(field.Type, fset)
		if len(field.Names) == 0 {
			b.WriteString(typStr)
		} else {
			for j, n := range field.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(n.Name)
			}
			b.WriteByte(' ')
			b.WriteString(typStr)
		}
	}
}

// buildSignature constructs a human-readable function signature string.
func buildSignature(name, receiver string, params []graph.Parameter, returns []string) string {
	var b strings.Builder
	if receiver != "" {
		fmt.Fprintf(&b, "(%s) ", receiver)
	}
	b.WriteString(name)
	b.WriteByte('(')
	for i, p := range params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Name != "" {
			b.WriteString(p.Name)
			b.WriteByte(' ')
		}
		b.WriteString(p.Type)
	}
	b.WriteByte(')')
	if len(returns) > 0 {
		b.WriteByte(' ')
		if len(returns) == 1 {
			b.WriteString(returns[0])
		} else {
			b.WriteByte('(')
			b.WriteString(strings.Join(returns, ", "))
			b.WriteByte(')')
		}
	}
	return b.String()
}

// newContainsEdge creates a CONTAINS edge with the given hierarchy depth
// stored in metadata.
func newContainsEdge(parentID, childID string, depth int) *graph.Edge {
	e := graph.NewEdge(parentID, childID, graph.EdgeTypeContains)
	e.Metadata.HierarchyDepth = depth
	return e
}
