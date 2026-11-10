package domain

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type TemplateID [16]byte

type TemplateSaveID [16]byte

func (id TemplateID) String() string {
	return formatUUID(id)
}

func (id TemplateSaveID) String() string {
	return formatUUID(id)
}

type BlockType string

const (
	BlockInstruction    BlockType = "instruction"
	BlockMaterial       BlockType = "material"
	BlockSingleChoice   BlockType = "single_choice"
	BlockMultipleChoice BlockType = "multiple_choice"
	BlockShortText      BlockType = "short_text"
	BlockShortNumber    BlockType = "short_number"
	BlockLongText       BlockType = "long_text"
	BlockFile           BlockType = "file"
)

var BlockTypes = []BlockType{
	BlockInstruction, BlockMaterial, BlockSingleChoice, BlockMultipleChoice, BlockShortText, BlockShortNumber, BlockLongText, BlockFile,
}

func (t BlockType) known() bool {
	for _, known := range BlockTypes {
		if t == known {
			return true
		}
	}
	return false
}

func (t BlockType) Question() bool {
	return t.known() && t != BlockInstruction && t != BlockMaterial
}

type ScoringMode string

const (
	ScoringAllOrNothing ScoringMode = "all_or_nothing"
	ScoringPartial      ScoringMode = "partial"
)

type AnswerFileKind string

const (
	AnswerFileImages AnswerFileKind = "images"
	AnswerFilePDF    AnswerFileKind = "pdf"
	AnswerFileOffice AnswerFileKind = "office"
	AnswerFileAudio  AnswerFileKind = "audio"
)

type ProblemCode string

const (
	ProblemNoBlocks            ProblemCode = "no_blocks"
	ProblemTextMissing         ProblemCode = "text_missing"
	ProblemPromptMissing       ProblemCode = "prompt_missing"
	ProblemTooFewOptions       ProblemCode = "too_few_options"
	ProblemOptionEmpty         ProblemCode = "option_empty"
	ProblemCorrectCount        ProblemCode = "correct_count"
	ProblemNoCorrectOption     ProblemCode = "no_correct_option"
	ProblemNoAcceptedAnswer    ProblemCode = "no_accepted_answer"
	ProblemAcceptedAnswerEmpty ProblemCode = "accepted_answer_empty"
	ProblemValueInvalid        ProblemCode = "value_invalid"
	ProblemToleranceInvalid    ProblemCode = "tolerance_invalid"
)

const (
	TemplateSchemaVersion        = 1
	TemplateMaxBlocks            = 100
	TemplateTextMaxRunes         = 10_000
	TemplateOptionMaxRunes       = 500
	TemplateMaxOptions           = 10
	TemplateAnswerMaxRunes       = 500
	TemplateMaxAnswers           = 20
	TemplateNumberMaxRunes       = 64
	TemplateMaxImages            = 3
	TemplateMaxPoints            = 100
	TemplateLongTextMax          = 20_000
	TemplateMaxFiles             = 5
	TemplateBlockBudgetBytes     = 64 << 20
	TemplateSavesPerSecond       = 2
	TemplateSavesPerHour         = 3_600
	TemplateSaveSecondWindow     = time.Second
	TemplateSaveHourWindow       = time.Hour
	PurposeTemplateSavesHour     = RateLimitPurpose("template_saves_hour")
	PurposeTemplateSavesSecond   = RateLimitPurpose("template_saves_second")
	templateDecimalFractionLimit = 6
)

var templateDecimal = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]{1,` + fmt.Sprint(templateDecimalFractionLimit) + `})?$`)

type TemplateOption struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Correct *bool  `json:"correct,omitempty"`
}

type TemplateBlock struct {
	ID              string            `json:"id"`
	Type            BlockType         `json:"type"`
	Text            *string           `json:"text,omitempty"`
	MaterialID      *string           `json:"material_id,omitempty"`
	Prompt          *string           `json:"prompt,omitempty"`
	ImageIDs        *[]string         `json:"image_ids,omitempty"`
	Options         *[]TemplateOption `json:"options,omitempty"`
	Scoring         *ScoringMode      `json:"scoring,omitempty"`
	AcceptedAnswers *[]string         `json:"accepted_answers,omitempty"`
	Value           *string           `json:"value,omitempty"`
	Tolerance       *string           `json:"tolerance,omitempty"`
	MaxLength       *int32            `json:"max_length,omitempty"`
	AllowedKinds    *[]AnswerFileKind `json:"allowed_kinds,omitempty"`
	MaxFiles        *int32            `json:"max_files,omitempty"`
	Points          *int32            `json:"points,omitempty"`
	Explanation     *string           `json:"explanation,omitempty"`
}

type FieldIssue struct {
	Path string
	Code FieldCode
}

type TemplateProblem struct {
	Block *string
	Code  ProblemCode
}

type BlockImages struct {
	Position int
	Images   []AttachmentID
}

type BlockMaterialRef struct {
	Position int
	Material MaterialID
}

type TemplateSummary struct {
	QuestionCount int32
	MaxPoints     int32
	Ready         bool
}

type CheckedTemplate struct {
	Blocks    []TemplateBlock
	Images    []BlockImages
	Materials []BlockMaterialRef
	Summary   TemplateSummary
	Problems  []TemplateProblem
}

type blockField struct {
	name    string
	present func(TemplateBlock) bool
	types   []BlockType
}

var questionTypes = []BlockType{BlockSingleChoice, BlockMultipleChoice, BlockShortText, BlockShortNumber, BlockLongText, BlockFile}

var blockFields = []blockField{
	{"text", func(b TemplateBlock) bool { return b.Text != nil }, []BlockType{BlockInstruction}},
	{"material_id", func(b TemplateBlock) bool { return b.MaterialID != nil }, []BlockType{BlockMaterial}},
	{"prompt", func(b TemplateBlock) bool { return b.Prompt != nil }, questionTypes},
	{"image_ids", func(b TemplateBlock) bool { return b.ImageIDs != nil }, questionTypes},
	{"options", func(b TemplateBlock) bool { return b.Options != nil }, []BlockType{BlockSingleChoice, BlockMultipleChoice}},
	{"scoring", func(b TemplateBlock) bool { return b.Scoring != nil }, []BlockType{BlockMultipleChoice}},
	{"accepted_answers", func(b TemplateBlock) bool { return b.AcceptedAnswers != nil }, []BlockType{BlockShortText}},
	{"value", func(b TemplateBlock) bool { return b.Value != nil }, []BlockType{BlockShortNumber}},
	{"tolerance", func(b TemplateBlock) bool { return b.Tolerance != nil }, []BlockType{BlockShortNumber}},
	{"max_length", func(b TemplateBlock) bool { return b.MaxLength != nil }, []BlockType{BlockLongText}},
	{"allowed_kinds", func(b TemplateBlock) bool { return b.AllowedKinds != nil }, []BlockType{BlockFile}},
	{"max_files", func(b TemplateBlock) bool { return b.MaxFiles != nil }, []BlockType{BlockFile}},
	{"points", func(b TemplateBlock) bool { return b.Points != nil }, questionTypes},
	{"explanation", func(b TemplateBlock) bool { return b.Explanation != nil }, questionTypes},
}

var requiredFields = map[BlockType][]string{
	BlockMaterial:       {"material_id"},
	BlockSingleChoice:   {"points"},
	BlockMultipleChoice: {"scoring", "points"},
	BlockShortText:      {"points"},
	BlockShortNumber:    {"points"},
	BlockLongText:       {"max_length", "points"},
	BlockFile:           {"allowed_kinds", "max_files", "points"},
}

type templateChecker struct {
	issues   []FieldIssue
	blockIDs map[TemplateID]struct{}
	options  map[TemplateID]struct{}
	checked  CheckedTemplate
}

func CheckTemplateBlocks(blocks []TemplateBlock) (CheckedTemplate, []FieldIssue) {
	if len(blocks) > TemplateMaxBlocks {
		return CheckedTemplate{}, []FieldIssue{{Path: "body.blocks", Code: FieldTooMany}}
	}
	checker := templateChecker{
		blockIDs: make(map[TemplateID]struct{}, len(blocks)),
		options:  map[TemplateID]struct{}{},
		checked:  CheckedTemplate{Blocks: make([]TemplateBlock, 0, len(blocks)), Problems: []TemplateProblem{}},
	}
	for position, block := range blocks {
		checker.block(position, block)
	}
	if len(checker.issues) > 0 {
		return CheckedTemplate{}, checker.issues
	}
	checker.checked.Problems = TemplateProblems(checker.checked.Blocks)
	checker.checked.Summary = summarise(checker.checked.Blocks, len(checker.checked.Problems) == 0)
	return checker.checked, nil
}

func (c *templateChecker) refuse(path string, code FieldCode) {
	c.issues = append(c.issues, FieldIssue{Path: path, Code: code})
}

func (c *templateChecker) block(position int, block TemplateBlock) {
	path := fmt.Sprintf("body.blocks[%d]", position)
	id, valid := c.identifier(path+".id", block.ID, c.blockIDs)
	if valid {
		block.ID = id
	}
	if !block.Type.known() {
		c.refuse(path+".type", FieldInvalid)
		return
	}
	for _, field := range blockFields {
		if field.present(block) && !containsType(field.types, block.Type) {
			c.refuse(path+"."+field.name, FieldNotAllowed)
		}
	}
	for _, name := range requiredFields[block.Type] {
		if !fieldPresent(block, name) {
			c.refuse(path+"."+name, FieldRequired)
		}
	}
	c.text(path+".text", block.Text, TemplateTextMaxRunes)
	c.text(path+".prompt", block.Prompt, TemplateTextMaxRunes)
	c.text(path+".explanation", block.Explanation, TemplateTextMaxRunes)
	c.text(path+".value", block.Value, TemplateNumberMaxRunes)
	c.text(path+".tolerance", block.Tolerance, TemplateNumberMaxRunes)
	c.material(path, position, &block)
	c.images(path, position, &block)
	c.optionList(path, &block)
	c.answers(path, block.AcceptedAnswers)
	c.settings(path, block)
	c.checked.Blocks = append(c.checked.Blocks, block)
}

func (c *templateChecker) identifier(path string, text string, seen map[TemplateID]struct{}) (string, bool) {
	id, valid := ParseUUID[TemplateID](text)
	if !valid || id[6]>>4 != 4 || id[8]&0xC0 != 0x80 {
		c.refuse(path, FieldInvalid)
		return "", false
	}
	if _, repeated := seen[id]; repeated {
		c.refuse(path, FieldInvalid)
		return "", false
	}
	seen[id] = struct{}{}
	return id.String(), true
}

func (c *templateChecker) text(path string, text *string, limit int) {
	switch {
	case text == nil:
	case strings.ContainsRune(*text, 0):
		c.refuse(path, FieldInvalid)
	case utf8.RuneCountInString(*text) > limit:
		c.refuse(path, FieldTooLong)
	}
}

func (c *templateChecker) material(path string, position int, block *TemplateBlock) {
	if block.MaterialID == nil || block.Type != BlockMaterial {
		return
	}
	material, valid := ParseUUID[MaterialID](*block.MaterialID)
	if !valid {
		c.refuse(path+".material_id", FieldInvalid)
		return
	}
	canonical := material.String()
	block.MaterialID = &canonical
	c.checked.Materials = append(c.checked.Materials, BlockMaterialRef{Position: position, Material: material})
}

func (c *templateChecker) images(path string, position int, block *TemplateBlock) {
	if block.ImageIDs == nil || !block.Type.Question() {
		return
	}
	if len(*block.ImageIDs) > TemplateMaxImages {
		c.refuse(path+".image_ids", FieldTooMany)
		return
	}
	images := make([]AttachmentID, 0, len(*block.ImageIDs))
	canonical := make([]string, 0, len(*block.ImageIDs))
	for index, text := range *block.ImageIDs {
		image, valid := ParseUUID[AttachmentID](text)
		if !valid || containsAttachment(images, image) {
			c.refuse(fmt.Sprintf("%s.image_ids[%d]", path, index), FieldInvalid)
			continue
		}
		images = append(images, image)
		canonical = append(canonical, image.String())
	}
	block.ImageIDs = &canonical
	if len(images) > 0 {
		c.checked.Images = append(c.checked.Images, BlockImages{Position: position, Images: images})
	}
}

func (c *templateChecker) optionList(path string, block *TemplateBlock) {
	if block.Options == nil || (block.Type != BlockSingleChoice && block.Type != BlockMultipleChoice) {
		return
	}
	if len(*block.Options) > TemplateMaxOptions {
		c.refuse(path+".options", FieldTooMany)
		return
	}
	options := make([]TemplateOption, 0, len(*block.Options))
	for index, option := range *block.Options {
		optionPath := fmt.Sprintf("%s.options[%d]", path, index)
		if id, valid := c.identifier(optionPath+".id", option.ID, c.options); valid {
			option.ID = id
		}
		c.text(optionPath+".text", &option.Text, TemplateOptionMaxRunes)
		if option.Correct == nil {
			c.refuse(optionPath+".correct", FieldRequired)
		}
		options = append(options, option)
	}
	block.Options = &options
}

func (c *templateChecker) answers(path string, answers *[]string) {
	if answers == nil {
		return
	}
	if len(*answers) > TemplateMaxAnswers {
		c.refuse(path+".accepted_answers", FieldTooMany)
		return
	}
	for index, answer := range *answers {
		c.text(fmt.Sprintf("%s.accepted_answers[%d]", path, index), &answer, TemplateAnswerMaxRunes)
	}
}

func (c *templateChecker) settings(path string, block TemplateBlock) {
	if block.Points != nil && (*block.Points < 0 || *block.Points > TemplateMaxPoints) {
		c.refuse(path+".points", FieldInvalid)
	}
	if block.MaxLength != nil && (*block.MaxLength < 1 || *block.MaxLength > TemplateLongTextMax) {
		c.refuse(path+".max_length", FieldInvalid)
	}
	if block.MaxFiles != nil && (*block.MaxFiles < 1 || *block.MaxFiles > TemplateMaxFiles) {
		c.refuse(path+".max_files", FieldInvalid)
	}
	if block.Scoring != nil && *block.Scoring != ScoringAllOrNothing && *block.Scoring != ScoringPartial {
		c.refuse(path+".scoring", FieldInvalid)
	}
	if block.AllowedKinds != nil && !validAllowedKinds(*block.AllowedKinds) {
		c.refuse(path+".allowed_kinds", FieldInvalid)
	}
}

func validAllowedKinds(kinds []AnswerFileKind) bool {
	if len(kinds) == 0 {
		return false
	}
	seen := map[AnswerFileKind]bool{}
	for _, kind := range kinds {
		switch kind {
		case AnswerFileImages, AnswerFilePDF, AnswerFileOffice, AnswerFileAudio:
		default:
			return false
		}
		if seen[kind] {
			return false
		}
		seen[kind] = true
	}
	return true
}

func fieldPresent(block TemplateBlock, name string) bool {
	for _, field := range blockFields {
		if field.name == name {
			return field.present(block)
		}
	}
	return false
}

func containsType(types []BlockType, wanted BlockType) bool {
	for _, candidate := range types {
		if candidate == wanted {
			return true
		}
	}
	return false
}

func containsAttachment(attachments []AttachmentID, wanted AttachmentID) bool {
	for _, candidate := range attachments {
		if candidate == wanted {
			return true
		}
	}
	return false
}

func TemplateProblems(blocks []TemplateBlock) []TemplateProblem {
	problems := []TemplateProblem{}
	if len(blocks) == 0 {
		return append(problems, TemplateProblem{Code: ProblemNoBlocks})
	}
	for _, block := range blocks {
		id := block.ID
		for _, code := range blockProblems(block) {
			problems = append(problems, TemplateProblem{Block: &id, Code: code})
		}
	}
	return problems
}

func blockProblems(block TemplateBlock) []ProblemCode {
	switch block.Type {
	case BlockInstruction:
		if blank(block.Text) {
			return []ProblemCode{ProblemTextMissing}
		}
		return nil
	case BlockMaterial:
		return nil
	case BlockSingleChoice, BlockMultipleChoice:
		return append(promptProblems(block), choiceProblems(block)...)
	case BlockShortText:
		return append(promptProblems(block), shortTextProblems(block)...)
	case BlockShortNumber:
		return append(promptProblems(block), numberProblems(block)...)
	case BlockLongText, BlockFile:
		return promptProblems(block)
	}
	return nil
}

func promptProblems(block TemplateBlock) []ProblemCode {
	if blank(block.Prompt) {
		return []ProblemCode{ProblemPromptMissing}
	}
	return nil
}

func numberProblems(block TemplateBlock) []ProblemCode {
	var problems []ProblemCode
	if !templateDecimal.MatchString(valueOf(block.Value)) {
		problems = append(problems, ProblemValueInvalid)
	}
	if !validTolerance(valueOf(block.Tolerance)) {
		problems = append(problems, ProblemToleranceInvalid)
	}
	return problems
}

func choiceProblems(block TemplateBlock) []ProblemCode {
	var problems []ProblemCode
	var options []TemplateOption
	if block.Options != nil {
		options = *block.Options
	}
	if len(options) < 2 {
		problems = append(problems, ProblemTooFewOptions)
	}
	correct := 0
	empty := false
	for _, option := range options {
		if strings.TrimSpace(option.Text) == "" {
			empty = true
		}
		if option.Correct != nil && *option.Correct {
			correct++
		}
	}
	if empty {
		problems = append(problems, ProblemOptionEmpty)
	}
	if block.Type == BlockSingleChoice && correct != 1 {
		problems = append(problems, ProblemCorrectCount)
	}
	if block.Type == BlockMultipleChoice && correct == 0 {
		problems = append(problems, ProblemNoCorrectOption)
	}
	return problems
}

func shortTextProblems(block TemplateBlock) []ProblemCode {
	if block.AcceptedAnswers == nil || len(*block.AcceptedAnswers) == 0 {
		return []ProblemCode{ProblemNoAcceptedAnswer}
	}
	for _, answer := range *block.AcceptedAnswers {
		if NormaliseTextV1(answer) == "" {
			return []ProblemCode{ProblemAcceptedAnswerEmpty}
		}
	}
	return nil
}

func validTolerance(text string) bool {
	if !templateDecimal.MatchString(text) {
		return false
	}
	tolerance, parsed := new(big.Rat).SetString(text)
	return parsed && tolerance.Sign() >= 0
}

func blank(text *string) bool {
	return text == nil || strings.TrimSpace(*text) == ""
}

func valueOf(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func summarise(blocks []TemplateBlock, ready bool) TemplateSummary {
	summary := TemplateSummary{Ready: ready}
	for _, block := range blocks {
		if !block.Type.Question() {
			continue
		}
		summary.QuestionCount++
		if block.Points != nil {
			summary.MaxPoints += *block.Points
		}
	}
	return summary
}
