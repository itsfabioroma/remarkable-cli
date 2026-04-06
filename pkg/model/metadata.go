package model

// Metadata maps to the .metadata JSON file on device
type Metadata struct {
	VisibleName      string `json:"visibleName"`
	Type             string `json:"type"`
	Parent           string `json:"parent"`
	LastModified     string `json:"lastModified"`
	LastOpened       string `json:"lastOpened,omitempty"`
	LastOpenedPage   int    `json:"lastOpenedPage,omitempty"`
	MetadataModified bool   `json:"metadatamodified"`
	Modified         bool   `json:"modified"`
	Pinned           bool   `json:"pinned"`
	Synced           bool   `json:"synced"`
	Deleted          bool   `json:"deleted"`
	Version          int    `json:"version"`
}

// Content maps to the .content JSON file on device
// supports both old format (flat pages array) and new format (cPages)
type Content struct {
	FileType       string            `json:"fileType"`
	PageCount      int               `json:"pageCount"`
	Pages          []string          `json:"pages,omitempty"`
	CPages         *CPages           `json:"cPages,omitempty"`
	LastOpenedPage int               `json:"lastOpenedPage"`
	LineHeight     int               `json:"lineHeight"`
	Margins        int               `json:"margins"`
	Orientation    string            `json:"orientation"`
	TextScale      int               `json:"textScale"`
	ExtraMetadata  map[string]string `json:"extraMetadata,omitempty"`
}

// CPages is the newer page list format (firmware 3.x+)
type CPages struct {
	Pages []CPage `json:"pages"`
}

// CPage is a single page entry in cPages format
type CPage struct {
	ID string `json:"id"`
}

// PageIDs returns all page UUIDs, handling both old and new content format
func (c *Content) PageIDs() []string {
	// new format: cPages.pages[].id
	if c.CPages != nil && len(c.CPages.Pages) > 0 {
		ids := make([]string, len(c.CPages.Pages))
		for i, p := range c.CPages.Pages {
			ids[i] = p.ID
		}
		return ids
	}

	// old format: flat pages array
	return c.Pages
}
