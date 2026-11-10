package library

import (
	"context"
	"time"

	"ovc/backend/internal/domain"
)

type StudentFile struct {
	Filename  string
	MediaType string
	SizeBytes int64
	Width     *int32
	Height    *int32
	URL       string
	ExpiresAt time.Time
}

type StudentMaterial struct {
	Title string
	File  StudentFile
}

type StudentOption struct {
	ID   string
	Text string
}

type StudentBlock struct {
	ID           string
	Type         domain.BlockType
	Text         *string
	Prompt       *string
	Options      []StudentOption
	MaxLength    *int32
	AllowedKinds []domain.AnswerFileKind
	MaxFiles     *int32
	Material     *StudentMaterial
	Images       []StudentFile
}

type StudentTemplate struct {
	Title  string
	Blocks []StudentBlock
}

func (s *Service) StudentView(ctx context.Context, tutor domain.UserID, id domain.TemplateID) (StudentTemplate, error) {
	template, err := s.Template(ctx, tutor, id)
	if err != nil {
		return StudentTemplate{}, err
	}
	view := StudentTemplate{Title: template.Title, Blocks: make([]StudentBlock, 0, len(template.Blocks))}
	for _, block := range template.Blocks {
		view.Blocks = append(view.Blocks, studentBlockOf(block))
	}
	return view, nil
}

func studentBlockOf(block TemplateBlock) StudentBlock {
	student := StudentBlock{ID: block.ID, Type: block.Type}
	switch block.Type {
	case domain.BlockInstruction:
		student.Text = emptyWhenMissing(block.Text)
	case domain.BlockMaterial:
		if block.Material != nil {
			student.Material = &StudentMaterial{Title: block.Material.Title, File: studentFileOf(block.Material.File)}
		}
	case domain.BlockSingleChoice, domain.BlockMultipleChoice:
		studentQuestion(&student, block)
		student.Options = []StudentOption{}
		if block.Options != nil {
			for _, option := range *block.Options {
				student.Options = append(student.Options, StudentOption{ID: option.ID, Text: option.Text})
			}
		}
	case domain.BlockShortText, domain.BlockShortNumber:
		studentQuestion(&student, block)
	case domain.BlockLongText:
		studentQuestion(&student, block)
		student.MaxLength = block.MaxLength
	case domain.BlockFile:
		studentQuestion(&student, block)
		student.MaxFiles = block.MaxFiles
		if block.AllowedKinds != nil {
			student.AllowedKinds = *block.AllowedKinds
		}
	}
	return student
}

func studentQuestion(student *StudentBlock, block TemplateBlock) {
	student.Prompt = emptyWhenMissing(block.Prompt)
	student.Images = make([]StudentFile, 0, len(block.Images))
	for _, image := range block.Images {
		student.Images = append(student.Images, studentFileOf(image))
	}
}

func studentFileOf(file TemplateFile) StudentFile {
	return StudentFile{
		Filename: file.Filename, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Width: file.Width, Height: file.Height, URL: file.URL,
		ExpiresAt: file.ExpiresAt,
	}
}

func emptyWhenMissing(text *string) *string {
	if text != nil {
		return text
	}
	empty := ""
	return &empty
}
