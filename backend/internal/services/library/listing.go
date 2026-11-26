package library

import (
	"bytes"
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"ovc/backend/internal/domain"
	"ovc/backend/internal/repositories"
)

const cursorSeparator = "\n"

type ListingQuery struct {
	Folder *domain.FolderID
	Query  *string
	Limit  int32
	Cursor string
}

type LibraryItem struct {
	Kind     domain.LibraryItemKind
	Material *Material
	Template *TemplateListItem
}

type orderedItem struct {
	after repositories.MaterialListAfter
	item  LibraryItem
	path  *domain.FolderID
}

type Listing struct {
	Folder        *Folder
	Path          []PathEntry
	Folders       []Folder
	Items         []LibraryItem
	NextCursor    *string
	TemplateCount int64
}

var likeEscapes = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Service) Listing(ctx context.Context, tutor domain.UserID, query ListingQuery) (Listing, error) {
	page, err := listingPage(tutor, query)
	if err != nil {
		return Listing{}, err
	}
	now, err := s.deps.Clock.Instant(ctx)
	if err != nil {
		return Listing{}, err
	}
	var listing Listing
	err = s.deps.Store.WithReadTx(ctx, func(repos repositories.Repositories) error {
		listing, err = s.listing(ctx, repos, tutor, query, page, now)
		return err
	})
	return listing, err
}

func (s *Service) ListingIn(ctx context.Context, repos repositories.Repositories, tutor domain.UserID, query ListingQuery, now time.Time) (Listing, error) {
	page, err := listingPage(tutor, query)
	if err != nil {
		return Listing{}, err
	}
	return s.withoutAddresses().listing(ctx, repos, tutor, query, page, now)
}

func listingPage(tutor domain.UserID, query ListingQuery) (repositories.MaterialPage, error) {
	if query.Query != nil && query.Folder != nil {
		return repositories.MaterialPage{}, invalid(fieldFolder, domain.FieldNotAllowed)
	}
	page := repositories.MaterialPage{Tutor: tutor, Folder: query.Folder, PageSize: query.Limit + 1}
	if query.Query != nil {
		if !utf8.ValidString(*query.Query) || strings.ContainsRune(*query.Query, 0) {
			return repositories.MaterialPage{}, invalid(fieldQuery, domain.FieldInvalid)
		}
		normalised := domain.NormaliseTextV1(*query.Query)
		if normalised == "" {
			return repositories.MaterialPage{}, invalid(fieldQuery, domain.FieldRequired)
		}
		pattern := likeEscapes.Replace(string(normalised))
		page.Pattern = &pattern
	}
	if query.Cursor != "" {
		after, valid := parseCursor(query.Cursor)
		if !valid {
			return repositories.MaterialPage{}, invalid(fieldCursor, domain.FieldInvalid)
		}
		page.After = after
	}
	return page, nil
}

func (s *Service) listing(
	ctx context.Context, repos repositories.Repositories, tutor domain.UserID, query ListingQuery, page repositories.MaterialPage, now time.Time,
) (Listing, error) {
	rows, tree, err := s.folderTree(ctx, repos, tutor)
	if err != nil {
		return Listing{}, err
	}
	if err := requireFolder(tree, query.Folder); err != nil {
		return Listing{}, err
	}
	names := make(map[domain.FolderID]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	listing := Listing{Path: []PathEntry{}, Folders: []Folder{}}
	if query.Query == nil {
		listing.Folder, listing.Path, listing.Folders = folderView(rows, tree, names, query.Folder)
	}
	materials, err := repos.Library.Materials(ctx, page)
	if err != nil {
		return Listing{}, err
	}
	templates, err := repos.Library.Templates(ctx, page)
	if err != nil {
		return Listing{}, err
	}
	if listing.TemplateCount, err = repos.Library.LiveTemplateCount(ctx, tutor); err != nil {
		return Listing{}, err
	}
	ordered := make([]orderedItem, 0, len(materials)+len(templates))
	for _, row := range materials {
		material := s.materialOf(row, now)
		ordered = append(ordered, orderedItem{
			after: repositories.MaterialListAfter{Title: row.NormalisedTitle, Kind: domain.LibraryItemMaterial, ID: domain.EntityID(row.ID)},
			item:  LibraryItem{Kind: domain.LibraryItemMaterial, Material: &material}, path: row.FolderID,
		})
	}
	for _, row := range templates {
		template := templateItemOf(row)
		ordered = append(ordered, orderedItem{
			after: repositories.MaterialListAfter{Title: row.NormalisedTitle, Kind: domain.LibraryItemTemplate, ID: domain.EntityID(row.ID)},
			item:  LibraryItem{Kind: domain.LibraryItemTemplate, Template: &template}, path: row.FolderID,
		})
	}
	slices.SortFunc(ordered, func(left, right orderedItem) int { return compareAfter(left.after, right.after) })
	if len(ordered) > int(query.Limit) {
		ordered = ordered[:query.Limit]
		next := formatCursor(ordered[len(ordered)-1].after)
		listing.NextCursor = &next
	}
	listing.Items = make([]LibraryItem, 0, len(ordered))
	for _, entry := range ordered {
		if query.Query != nil {
			path := pathOf(tree, names, entry.path)
			if entry.item.Material != nil {
				entry.item.Material.Path = path
			} else {
				entry.item.Template.Path = path
			}
		}
		listing.Items = append(listing.Items, entry.item)
	}
	return listing, nil
}

func compareAfter(left, right repositories.MaterialListAfter) int {
	if order := strings.Compare(left.Title, right.Title); order != 0 {
		return order
	}
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	return bytes.Compare(left.ID[:], right.ID[:])
}

func folderView(
	rows []repositories.FolderRow, tree domain.FolderTree, names map[domain.FolderID]string, folder *domain.FolderID,
) (*Folder, []PathEntry, []Folder) {
	children := []Folder{}
	var current *Folder
	for _, row := range rows {
		if sameFolder(row.ParentID, folder) {
			children = append(children, folderOf(row))
		}
		if folder != nil && row.ID == *folder {
			found := folderOf(row)
			current = &found
		}
	}
	return current, pathOf(tree, names, folder), children
}

func pathOf(tree domain.FolderTree, names map[domain.FolderID]string, folder *domain.FolderID) []PathEntry {
	path := []PathEntry{}
	for current := folder; current != nil && len(path) <= len(tree); current = tree[*current] {
		path = append(path, PathEntry{ID: *current, Name: names[*current]})
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func formatCursor(after repositories.MaterialListAfter) string {
	text := string(after.Kind) + cursorSeparator + after.ID.String() + cursorSeparator + after.Title
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func parseCursor(text string) (repositories.MaterialListAfter, bool) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(text)
	if err != nil {
		return repositories.MaterialListAfter{}, false
	}
	parts := strings.SplitN(string(decoded), cursorSeparator, 3)
	if len(parts) != 3 {
		return repositories.MaterialListAfter{}, false
	}
	kind := domain.LibraryItemKind(parts[0])
	if kind != domain.LibraryItemMaterial && kind != domain.LibraryItemTemplate {
		return repositories.MaterialListAfter{}, false
	}
	id, valid := domain.ParseUUID[domain.EntityID](parts[1])
	title := parts[2]
	if !valid || title == "" || len(title) > domain.LibraryTitleNormalisedBytes || !utf8.ValidString(title) || strings.ContainsRune(title, 0) {
		return repositories.MaterialListAfter{}, false
	}
	return repositories.MaterialListAfter{Title: title, Kind: kind, ID: id}, true
}
