package schemas

type StudentExportFile struct {
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type StudentExportAnswer struct {
	StudentAnswer
	Files []StudentExportFile `json:"files" nullable:"false"`
}

type StudentExportReviewItem struct {
	StudentReviewItem
	CorrectedFiles []StudentExportFile `json:"corrected_files" nullable:"false"`
}

type StudentExportReview struct {
	StudentReview
	Items []StudentExportReviewItem `json:"items" nullable:"false"`
}

type StudentExportTemplateFile struct {
	Filename  string  `json:"filename"`
	MediaType string  `json:"media_type"`
	SizeBytes int64   `json:"size_bytes"`
	Width     *int32  `json:"width" nullable:"true"`
	Height    *int32  `json:"height" nullable:"true"`
	Path      *string `json:"path" nullable:"true"`
}

type StudentExportTemplateMaterial struct {
	Title     string  `json:"title"`
	Filename  string  `json:"filename"`
	MediaType string  `json:"media_type"`
	SizeBytes int64   `json:"size_bytes"`
	Width     *int32  `json:"width" nullable:"true"`
	Height    *int32  `json:"height" nullable:"true"`
	Path      *string `json:"path" nullable:"true"`
}

type StudentExportTemplateBlock struct {
	StudentTemplateBlock
	Material *StudentExportTemplateMaterial `json:"material,omitempty"`
	Images   *[]StudentExportTemplateFile   `json:"images,omitempty"`
}

type StudentExportSubmission struct {
	StudentSubmission
	Blocks  []StudentExportTemplateBlock `json:"blocks" nullable:"false"`
	Answers []StudentExportAnswer        `json:"answers" nullable:"false"`
	Review  *StudentExportReview         `json:"review"`
}

type StudentExportMessageAttachment struct {
	ID        string  `json:"id" format:"uuid"`
	Filename  string  `json:"filename"`
	MediaType string  `json:"media_type"`
	SizeBytes int64   `json:"size_bytes"`
	Width     *int32  `json:"width" nullable:"true"`
	Height    *int32  `json:"height" nullable:"true"`
	Path      *string `json:"path" nullable:"true"`
}

type StudentExportMessage struct {
	Message
	Attachments []StudentExportMessageAttachment `json:"attachments" nullable:"false"`
}

type StudentExportChannelHistory struct {
	Channel  ChannelItem            `json:"channel"`
	Messages []StudentExportMessage `json:"messages" nullable:"false"`
}

type StudentExportLessonMaterial struct {
	MaterialID   *string `json:"material_id,omitempty" format:"uuid"`
	AttachmentID string  `json:"attachment_id" format:"uuid"`
	Title        string  `json:"title"`
	Filename     string  `json:"filename"`
	MediaType    string  `json:"media_type"`
	SizeBytes    int64   `json:"size_bytes"`
	Width        *int32  `json:"width" nullable:"true"`
	Height       *int32  `json:"height" nullable:"true"`
	Path         *string `json:"path" nullable:"true"`
}

type StudentExportLesson struct {
	LessonDetail
	Materials []StudentExportLessonMaterial `json:"materials" nullable:"false"`
}

type StudentExportFileEntry struct {
	Path       string   `json:"path"`
	Filename   string   `json:"filename"`
	MediaType  string   `json:"media_type"`
	SizeBytes  int64    `json:"size_bytes"`
	SHA256     string   `json:"sha256"`
	References []string `json:"references" nullable:"false"`
}
