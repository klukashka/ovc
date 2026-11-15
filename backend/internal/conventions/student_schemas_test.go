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

var pinnedStudentEmbeddings = map[string]string{
	"StudentCard":        "CardFields",
	"StudentCardDetail":  "StudentCard",
	"StudentCardSummary": "CardFields",
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

func studentReach(structs map[string][]schemaField) map[string]string {
	reached := map[string]string{}
	var queue []string
	for name := range structs {
		if _, exempt := pinnedRevealedFields[name]; strings.HasPrefix(name, studentPrefix) && !exempt {
			reached[name] = name
			queue = append(queue, name)
		}
	}
	slices.Sort(queue)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, field := range structs[name] {
			for _, referred := range field.refers {
				_, known := structs[referred]
				_, exempt := pinnedRevealedFields[referred]
				if _, seen := reached[referred]; seen || !known || exempt {
					continue
				}
				reached[referred] = reached[name] + "." + field.name + " -> " + referred
				queue = append(queue, referred)
			}
		}
	}
	return reached
}

func studentSchemaProblems(structs map[string][]schemaField) []string {
	var problems []string
	for name, fields := range structs {
		pinned, exempt := pinnedRevealedFields[name]
		if !strings.HasPrefix(name, studentPrefix) || !exempt {
			continue
		}
		var keys []string
		for _, field := range fields {
			keys = append(keys, field.key)
		}
		if !slices.Equal(keys, pinned) {
			problems = append(problems, name+" declares "+strings.Join(keys, ", ")+" instead of its pinned fields")
		}
	}
	for name, path := range studentReach(structs) {
		for _, field := range structs[name] {
			if field.embedded && !slices.Equal(field.refers, []string{pinnedStudentEmbeddings[name]}) {
				problems = append(problems, path+" embeds "+strings.Join(field.refers, "."))
			}
			if slices.Contains(forbiddenStudentGoNames, field.name) || slices.Contains(forbiddenStudentJSONKeys, field.key) {
				problems = append(problems, path+" declares the forbidden field "+field.name+" ("+field.key+")")
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
		"StudentEmbeds":   {{embedded: true, refers: []string{"TutorWork"}}},
		"StudentCard":     {{embedded: true, refers: []string{"TutorWork"}}},
		"TutorWork":       {{name: "AutoCorrect", key: "auto_correct"}},
		"StudentHolder":   {{name: "Detail", key: "detail", refers: []string{"Shared"}}},
		"Shared":          {{name: "Files", key: "files", refers: []string{"map", "string", "Nested"}}},
		"Nested":          {{name: "Points", key: "points"}},
	}

	problems := strings.Join(studentSchemaProblems(structs), "\n")

	for _, expected := range []string{
		"StudentLeak declares the forbidden field Points", "StudentOther.Revealed refers to the revealed struct StudentRevealed",
		"StudentRevealedQuestion declares question_id, points instead of its pinned fields", "TutorView. refers to the revealed struct StudentRevealedQuestion",
		"StudentEmbeds embeds TutorWork", "StudentCard embeds TutorWork", ". -> TutorWork declares the forbidden field AutoCorrect (auto_correct)",
		"StudentHolder.Detail -> Shared.Files -> Nested declares the forbidden field Points (points)",
	} {
		if !strings.Contains(problems, expected) {
			t.Fatalf("expected %q among:\n%s", expected, problems)
		}
	}
}
