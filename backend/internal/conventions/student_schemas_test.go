package conventions_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	studentPrefix            = "Student"
	revealedContainer        = "StudentRevealed"
	revealedItem             = "StudentRevealedQuestion"
	studentSubmission        = "StudentSubmission"
	studentSubmissionReveals = "Revealed"
)

var forbiddenStudentGoNames = []string{
	"Points", "MaxPoints", "AutoPoints", "FinalPoints", "ManualPoints", "OverridePoints", "OverrideReason", "Overrides", "Total", "Totals",
	"Score", "Scores", "Percentage", "Correct", "AutoCorrect", "Correctness", "CorrectOptionIDs", "AcceptedAnswers", "CorrectValue",
	"Tolerance", "Explanation", "Scoring", "ScoringMode", "HasManualQuestions", "Normalised", "NormaliserVersion",
}

var forbiddenStudentJSONKeys = []string{
	"points", "max_points", "auto_points", "final_points", "manual_points", "override_points", "override_reason", "overrides", "total",
	"totals", "score", "scores", "percentage", "correct", "auto_correct", "correctness", "correct_option_ids", "accepted_answers",
	"correct_value", "tolerance", "explanation", "scoring", "scoring_mode", "has_manual_questions", "normalised", "normaliser_version",
}

var pinnedRevealedFields = map[string][]string{
	revealedContainer: {"questions"},
	revealedItem:      {"question_id", "correct_option_ids", "accepted_answers", "correct_value", "tolerance", "explanation"},
}

type schemaField struct {
	name     string
	key      string
	embedded bool
	pointer  bool
	omits    bool
	refers   []string
}

func schemaStructs(t *testing.T) map[string][]schemaField {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(backendRoot, "internal", "schemas", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	structs := map[string][]schemaField{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, isType := node.(*ast.TypeSpec)
			if !isType {
				return true
			}
			declared, isStruct := spec.Type.(*ast.StructType)
			if !isStruct {
				return true
			}
			structs[spec.Name.Name] = fieldsOf(t, declared)
			return false
		})
	}
	return structs
}

func fieldsOf(t *testing.T, declared *ast.StructType) []schemaField {
	t.Helper()
	var fields []schemaField
	for _, field := range declared.Fields.List {
		key, omits := jsonKey(t, field.Tag)
		_, pointer := field.Type.(*ast.StarExpr)
		described := schemaField{key: key, omits: omits, pointer: pointer, refers: referencedNames(field.Type)}
		if len(field.Names) == 0 {
			described.embedded = true
			fields = append(fields, described)
			continue
		}
		for _, name := range field.Names {
			named := described
			named.name = name.Name
			fields = append(fields, named)
		}
	}
	return fields
}

func jsonKey(t *testing.T, tag *ast.BasicLit) (string, bool) {
	t.Helper()
	if tag == nil {
		return "", false
	}
	unquoted, err := strconv.Unquote(tag.Value)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range strings.Fields(unquoted) {
		value, found := strings.CutPrefix(part, `json:"`)
		if !found {
			continue
		}
		options := strings.Split(strings.TrimSuffix(value, `"`), ",")
		return options[0], slices.Contains(options[1:], "omitempty")
	}
	return "", false
}

func referencedNames(expression ast.Expr) []string {
	var names []string
	ast.Inspect(expression, func(node ast.Node) bool {
		if identifier, isIdentifier := node.(*ast.Ident); isIdentifier {
			names = append(names, identifier.Name)
		}
		return true
	})
	return names
}

func studentSchemaProblems(structs map[string][]schemaField) []string {
	var problems []string
	for name, fields := range structs {
		if !strings.HasPrefix(name, studentPrefix) {
			continue
		}
		if pinned, exempt := pinnedRevealedFields[name]; exempt {
			var keys []string
			for _, field := range fields {
				keys = append(keys, field.key)
			}
			if !slices.Equal(keys, pinned) {
				problems = append(problems, name+" declares "+strings.Join(keys, ", ")+" instead of its pinned fields")
			}
			continue
		}
		for _, field := range fields {
			if slices.Contains(forbiddenStudentGoNames, field.name) || slices.Contains(forbiddenStudentJSONKeys, field.key) {
				problems = append(problems, name+" declares the forbidden field "+field.name+" ("+field.key+")")
			}
		}
	}
	for name, fields := range structs {
		for _, field := range fields {
			for exempt := range pinnedRevealedFields {
				if !slices.Contains(field.refers, exempt) {
					continue
				}
				allowedContainer := name == studentSubmission && field.name == studentSubmissionReveals && exempt == revealedContainer &&
					field.pointer && field.omits && !field.embedded
				allowedItem := name == revealedContainer && exempt == revealedItem && !field.embedded
				if !allowedContainer && !allowedItem {
					problems = append(problems, name+"."+field.name+" refers to the revealed struct "+exempt)
				}
			}
		}
	}
	slices.Sort(problems)
	return problems
}

func TestStudentSchemasHaveNoKeyOrPointFields(t *testing.T) {
	structs := schemaStructs(t)

	problems := studentSchemaProblems(structs)

	if len(problems) > 0 {
		t.Fatalf("student-facing schemas carry key or point data:\n%s", strings.Join(problems, "\n"))
	}
	if _, found := structs[studentSubmission]; !found {
		t.Fatal("the student submission schema is missing")
	}
	for exempt := range pinnedRevealedFields {
		if _, found := structs[exempt]; !found {
			t.Fatalf("the revealed struct %s is missing", exempt)
		}
	}
}

func TestStudentSchemaCheckRejectsKeyFieldsAndStrayReferences(t *testing.T) {
	structs := map[string][]schemaField{
		"StudentLeak":     {{name: "Points", key: "points"}},
		"StudentOther":    {{name: "Revealed", key: "revealed", pointer: true, omits: true, refers: []string{revealedContainer}}},
		revealedContainer: {{name: "Questions", key: "questions", refers: []string{revealedItem}}},
		revealedItem:      {{name: "QuestionID", key: "question_id"}, {name: "Points", key: "points"}},
		"TutorView":       {{embedded: true, refers: []string{revealedItem}}},
	}

	problems := strings.Join(studentSchemaProblems(structs), "\n")

	for _, expected := range []string{
		"StudentLeak declares the forbidden field Points", "StudentOther.Revealed refers to the revealed struct StudentRevealed",
		"StudentRevealedQuestion declares question_id, points instead of its pinned fields", "TutorView. refers to the revealed struct StudentRevealedQuestion",
	} {
		if !strings.Contains(problems, expected) {
			t.Fatalf("expected %q among:\n%s", expected, problems)
		}
	}
}
