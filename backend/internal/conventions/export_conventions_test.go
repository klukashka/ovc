package conventions_test

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ovc/backend/internal/core"
	"ovc/backend/internal/domain"
)

const (
	repositoriesPackage = modulePath + "/internal/repositories"
	repositoriesType    = "Repositories"
	exportSchemaPrefix  = "export_"
)

var exportRepositoryAllowlist = []string{
	"Exports", "UploadIntents", "Notifications", "Signals", "Outbox", "RateLimits",
	"Users.ByID", "Users.EarliestPurgeDue", "Users.LockCredentials", "TutorProfiles.Lock",
	"AfterCommit", "TakeExportInvalidations", "PurgeExportOf", "StartExportWithdrawal",
}

var customMarshalers = []string{"MarshalJSON", "MarshalText"}

type checkedPackage struct {
	fileSet *token.FileSet
	files   []*ast.File
	typed   *types.Package
}

func exportDataImporter(t *testing.T, fileSet *token.FileSet, pattern string) types.Importer {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}", pattern)
	command.Dir = backendRoot
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list the export data of %s: %v", pattern, err)
	}
	exports := map[string]string{}
	for line := range strings.Lines(string(output)) {
		path, file, found := strings.Cut(strings.TrimSpace(line), "\t")
		if found && file != "" {
			exports[path] = file
		}
	}
	return importer.ForCompiler(fileSet, "gc", func(path string) (io.ReadCloser, error) {
		file, found := exports[path]
		if !found {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file)
	})
}

func checkPackage(t *testing.T, directory string, info *types.Info) checkedPackage {
	t.Helper()
	fileSet := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(backendRoot, directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatalf("no Go files in %s", directory)
	}
	config := types.Config{Importer: exportDataImporter(t, fileSet, "./"+directory)}
	checked, err := config.Check(modulePath+"/"+directory, fileSet, files, info)
	if err != nil {
		t.Fatal(err)
	}
	return checkedPackage{fileSet: fileSet, files: files, typed: checked}
}

func onRepositories(selection *types.Selection) bool {
	receiver := selection.Recv()
	if pointer, isPointer := receiver.(*types.Pointer); isPointer {
		receiver = pointer.Elem()
	}
	named, isNamed := receiver.(*types.Named)
	return isNamed && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == repositoriesPackage && named.Obj().Name() == repositoriesType
}

func repositoryField(info *types.Info, expression ast.Expr) (*ast.SelectorExpr, bool) {
	selector, isSelector := expression.(*ast.SelectorExpr)
	if !isSelector {
		return nil, false
	}
	selection, found := info.Selections[selector]
	return selector, found && selection.Kind() == types.FieldVal && onRepositories(selection)
}

func repositoryUses(files []*ast.File, info *types.Info) []string {
	followed := map[*ast.SelectorExpr]bool{}
	var uses []string
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			if field, isField := repositoryField(info, selector.X); isField {
				followed[field] = true
				uses = append(uses, field.Sel.Name+"."+selector.Sel.Name)
				return true
			}
			selection, found := info.Selections[selector]
			switch {
			case !found || !onRepositories(selection):
			case selection.Kind() == types.MethodVal:
				uses = append(uses, selector.Sel.Name)
			case !followed[selector]:
				uses = append(uses, selector.Sel.Name)
			}
			return true
		})
	}
	return uses
}

func allowedRepositoryUse(use string) bool {
	field, _, _ := strings.Cut(use, ".")
	return slices.Contains(exportRepositoryAllowlist, use) || slices.Contains(exportRepositoryAllowlist, field)
}

func TestExportServiceCallsOnlyItsOwnRepositories(t *testing.T) {
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}}
	checked := checkPackage(t, "internal/services/export", info)

	uses := repositoryUses(checked.files, info)

	var problems []string
	for _, use := range uses {
		if !allowedRepositoryUse(use) {
			problems = append(problems, use)
		}
	}
	if len(uses) == 0 {
		t.Fatal("the check found no use of the repositories in the export service")
	}
	if len(problems) > 0 {
		t.Fatalf("the export service reads business data outside the owning services:\n%s", strings.Join(problems, "\n"))
	}
}

func marshalsItself(checked *types.Package, name string) bool {
	object := checked.Scope().Lookup(name)
	if object == nil {
		return false
	}
	methods := types.NewMethodSet(types.NewPointer(object.Type()))
	return slices.ContainsFunc(customMarshalers, func(method string) bool { return methods.Lookup(checked, method) != nil })
}

func exportSchemaStructs(checked checkedPackage) []string {
	var names []string
	for _, file := range checked.files {
		if !strings.HasPrefix(filepath.Base(checked.fileSet.Position(file.Package).Filename), exportSchemaPrefix) {
			continue
		}
		for _, declaration := range file.Decls {
			general, isGeneral := declaration.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				if typeSpec, isType := spec.(*ast.TypeSpec); isType {
					if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
						names = append(names, typeSpec.Name.Name)
					}
				}
			}
		}
	}
	return names
}

func TestExportSchemasEmbedNoCustomMarshaler(t *testing.T) {
	checked := checkPackage(t, "internal/schemas", nil)
	knownMarshalers := []string{"Home", "SubmissionDetail"}

	var problems []string
	for _, name := range exportSchemaStructs(checked) {
		if marshalsItself(checked.typed, name) {
			problems = append(problems, name)
		}
	}
	undetected := slices.DeleteFunc(slices.Clone(knownMarshalers), func(name string) bool { return marshalsItself(checked.typed, name) })

	if len(undetected) > 0 {
		t.Fatalf("the check does not see the custom marshalers of %s", strings.Join(undetected, ", "))
	}
	if len(problems) > 0 {
		t.Fatalf("export structs with a custom marshaler, their own or promoted from an embedded type:\n%s", strings.Join(problems, "\n"))
	}
}

var studentSheetRoots = map[core.ExportSheet]string{
	core.ExportSheetTutors: "MyTutor", core.ExportSheetLessons: "StudentExportLesson", core.ExportSheetHomework: "StudentAssignmentItem",
	core.ExportSheetAnswers: "StudentExportSubmission", core.ExportSheetFeedback: "StudentExportSubmission",
	core.ExportSheetMessages: "StudentExportChannelHistory", core.ExportSheetFiles: "StudentExportFileEntry",
}

func structOf(typed types.Type) (*types.Struct, bool) {
	for {
		switch current := typed.(type) {
		case *types.Alias:
			typed = types.Unalias(current)
		case *types.Pointer:
			typed = current.Elem()
		case *types.Slice:
			typed = current.Elem()
		case *types.Named:
			typed = current.Underlying()
		case *types.Struct:
			return current, true
		default:
			return nil, false
		}
	}
}

func jsonName(tag string) string {
	for _, part := range strings.Fields(tag) {
		if value, found := strings.CutPrefix(part, `json:"`); found {
			name, _, _ := strings.Cut(strings.TrimSuffix(value, `"`), ",")
			return name
		}
	}
	return ""
}

func jsonField(structure *types.Struct, key string) (types.Type, bool) {
	level := []*types.Struct{structure}
	for len(level) > 0 {
		var next []*types.Struct
		for _, current := range level {
			for index := range current.NumFields() {
				field := current.Field(index)
				if jsonName(current.Tag(index)) == key {
					return field.Type(), true
				}
				if embedded, isStruct := structOf(field.Type()); field.Embedded() && isStruct {
					next = append(next, embedded)
				}
			}
		}
		level = next
	}
	return nil, false
}

func projects(checked *types.Package, root string, key string) bool {
	object := checked.Scope().Lookup(root)
	if object == nil {
		return false
	}
	current := object.Type()
	for _, segment := range strings.Split(key, ".") {
		structure, isStruct := structOf(current)
		if !isStruct {
			return false
		}
		next, found := jsonField(structure, segment)
		if !found {
			return false
		}
		current = next
	}
	return true
}

func TestStudentSheetColumnsProjectStudentFields(t *testing.T) {
	checked := checkPackage(t, "internal/schemas", nil)
	languages := []domain.Language{domain.LanguageEnglish, domain.LanguageRussian}

	var problems []string
	for _, sheet := range core.StudentExportSheets() {
		root, found := studentSheetRoots[sheet]
		columns := core.StudentExportColumns(sheet)
		if !found || len(columns) == 0 {
			problems = append(problems, string(sheet)+" has no data-copy struct or no columns")
			continue
		}
		for _, column := range columns {
			if !projects(checked.typed, root, column.Key) {
				problems = append(problems, string(sheet)+"."+column.Key+" is not a field of "+root)
			}
		}
		for _, language := range languages {
			text, known := core.ExportTextFor(language)
			if !known || len(text.Sheets[sheet].Headers) != len(columns) || text.Sheets[sheet].Title == "" {
				problems = append(problems, string(sheet)+" has no matching headers in "+string(language))
			}
		}
	}
	unprojected := projects(checked.typed, "StudentExportSubmission", "points")

	if unprojected {
		t.Fatal("the projection check accepts a key the struct does not have")
	}
	if len(problems) > 0 {
		t.Fatalf("student sheet columns that are not projections of the data copy:\n%s", strings.Join(problems, "\n"))
	}
}
