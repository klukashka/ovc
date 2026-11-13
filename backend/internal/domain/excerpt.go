package domain

import (
	"strings"
	"unicode"
)

const (
	FeedbackExcerptMaxRunes = 140
	excerptEllipsis         = '…'
	fenceMinimum            = 3
	listMarkerIndent        = 3
	orderedMarkerDigits     = 9
)

type excerptCell struct {
	letter  rune
	literal bool
}

func FeedbackExcerpt(feedback string) *string {
	text := strings.Join(strings.Fields(strings.Join(excerptBlocks(feedback), " ")), " ")
	if text == "" {
		return nil
	}
	letters := []rune(text)
	if len(letters) <= FeedbackExcerptMaxRunes {
		return &text
	}
	limit := FeedbackExcerptMaxRunes - 1
	cut := limit
	if !unicode.IsSpace(letters[limit]) {
		for index := limit - 1; index > 0; index-- {
			if unicode.IsSpace(letters[index]) {
				cut = index
				break
			}
		}
	}
	excerpt := strings.TrimRightFunc(string(letters[:cut]), unicode.IsSpace) + string(excerptEllipsis)
	return &excerpt
}

func excerptBlocks(feedback string) []string {
	lines := strings.Split(strings.ReplaceAll(feedback, "\r\n", "\n"), "\n")
	blocks := []string{}
	var paragraph []string
	flush := func() {
		if len(paragraph) > 0 {
			blocks = append(blocks, plainInline(strings.Join(paragraph, " ")))
			paragraph = nil
		}
	}
	var fence string
	for _, line := range lines {
		if fence != "" {
			if closesFence(line, fence) {
				fence = ""
				continue
			}
			blocks = append(blocks, line)
			continue
		}
		if opened := openedFence(line); opened != "" {
			flush()
			fence = opened
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if item, isItem := listItemText(line); isItem {
			flush()
			paragraph = append(paragraph, item)
			continue
		}
		paragraph = append(paragraph, line)
	}
	flush()
	return blocks
}

func indentWithin(line string, limit int) (string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	return trimmed, len(line)-len(trimmed) <= limit
}

func openedFence(line string) string {
	trimmed, shallow := indentWithin(line, listMarkerIndent)
	if !shallow || trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	run := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	if run < fenceMinimum || (trimmed[0] == '`' && strings.Contains(trimmed[run:], "`")) {
		return ""
	}
	return trimmed[:run]
}

func closesFence(line string, fence string) bool {
	trimmed, shallow := indentWithin(line, listMarkerIndent)
	if !shallow || !strings.HasPrefix(trimmed, fence) {
		return false
	}
	rest := strings.TrimLeft(trimmed, fence[:1])
	return strings.TrimSpace(rest) == ""
}

func listItemText(line string) (string, bool) {
	trimmed, shallow := indentWithin(line, listMarkerIndent)
	if !shallow || trimmed == "" {
		return "", false
	}
	marker := 0
	switch trimmed[0] {
	case '-', '*', '+':
		marker = 1
	default:
		digits := 0
		for digits < len(trimmed) && digits < orderedMarkerDigits && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(trimmed) || (trimmed[digits] != '.' && trimmed[digits] != ')') {
			return "", false
		}
		marker = digits + 1
	}
	if marker < len(trimmed) && trimmed[marker] != ' ' && trimmed[marker] != '\t' {
		return "", false
	}
	return trimmed[marker:], true
}

func plainInline(source string) string {
	cells := escapedCells([]rune(source))
	cells = codeSpans(cells)
	return string(withoutMarkers(cells))
}

func escapedCells(letters []rune) []excerptCell {
	cells := make([]excerptCell, 0, len(letters))
	for index := 0; index < len(letters); index++ {
		if letters[index] == '\\' && index+1 < len(letters) && asciiPunctuation(letters[index+1]) {
			cells = append(cells, excerptCell{letter: letters[index+1], literal: true})
			index++
			continue
		}
		cells = append(cells, excerptCell{letter: letters[index]})
	}
	return cells
}

func asciiPunctuation(letter rune) bool {
	return letter < unicode.MaxASCII && (unicode.IsPunct(letter) || unicode.IsSymbol(letter))
}

type backtickRun struct {
	start  int
	length int
}

func codeSpans(cells []excerptCell) []excerptCell {
	var runs []backtickRun
	for index := 0; index < len(cells); {
		if cells[index].literal || cells[index].letter != '`' {
			index++
			continue
		}
		end := index
		for end < len(cells) && !cells[end].literal && cells[end].letter == '`' {
			end++
		}
		runs = append(runs, backtickRun{start: index, length: end - index})
		index = end
	}
	byLength := map[int][]int{}
	for index, run := range runs {
		byLength[run.length] = append(byLength[run.length], index)
	}
	cursors := map[int]int{}
	result := make([]excerptCell, 0, len(cells))
	copied := 0
	for index, run := range runs {
		if run.start < copied {
			continue
		}
		same := byLength[run.length]
		cursor := cursors[run.length]
		for cursor < len(same) && same[cursor] <= index {
			cursor++
		}
		cursors[run.length] = cursor
		if cursor == len(same) {
			continue
		}
		closer := runs[same[cursor]]
		result = append(result, cells[copied:run.start]...)
		for _, cell := range cells[run.start+run.length : closer.start] {
			result = append(result, excerptCell{letter: cell.letter, literal: true})
		}
		copied = closer.start + closer.length
	}
	return append(result, cells[copied:]...)
}

func withoutMarkers(cells []excerptCell) []rune {
	nextClose := nextMarker(cells, ']')
	nextParen := nextMarker(cells, ')')
	letters := make([]rune, 0, len(cells))
	for index := 0; index < len(cells); index++ {
		cell := cells[index]
		switch {
		case cell.literal:
			letters = append(letters, cell.letter)
		case cell.letter == '[':
			closing := nextClose[index]
			if closing < 0 || closing+1 >= len(cells) || cells[closing+1].literal || cells[closing+1].letter != '(' || nextParen[closing+1] < 0 {
				letters = append(letters, cell.letter)
				continue
			}
			letters = append(letters, withoutEmphasis(cells[index+1:closing])...)
			index = nextParen[closing+1]
		case cell.letter == '*' || cell.letter == '_':
			end := index
			for end < len(cells) && !cells[end].literal && cells[end].letter == cell.letter {
				end++
			}
			if keptDelimiter(cells, index, end) {
				for _, kept := range cells[index:end] {
					letters = append(letters, kept.letter)
				}
			}
			index = end - 1
		default:
			letters = append(letters, cell.letter)
		}
	}
	return letters
}

func withoutEmphasis(cells []excerptCell) []rune {
	letters := make([]rune, 0, len(cells))
	for index := 0; index < len(cells); index++ {
		cell := cells[index]
		if cell.literal || (cell.letter != '*' && cell.letter != '_') {
			letters = append(letters, cell.letter)
			continue
		}
		end := index
		for end < len(cells) && !cells[end].literal && cells[end].letter == cell.letter {
			end++
		}
		if keptDelimiter(cells, index, end) {
			for _, kept := range cells[index:end] {
				letters = append(letters, kept.letter)
			}
		}
		index = end - 1
	}
	return letters
}

func keptDelimiter(cells []excerptCell, start int, end int) bool {
	before, after := ' ', ' '
	if start > 0 {
		before = cells[start-1].letter
	}
	if end < len(cells) {
		after = cells[end].letter
	}
	if unicode.IsSpace(before) && unicode.IsSpace(after) {
		return true
	}
	wordy := func(letter rune) bool { return unicode.IsLetter(letter) || unicode.IsDigit(letter) }
	return cells[start].letter == '_' && wordy(before) && wordy(after)
}

func nextMarker(cells []excerptCell, marker rune) []int {
	next := make([]int, len(cells)+1)
	next[len(cells)] = -1
	for index := len(cells) - 1; index >= 0; index-- {
		next[index] = next[index+1]
		if !cells[index].literal && cells[index].letter == marker {
			next[index] = index
		}
	}
	return next[:len(cells)]
}
