package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// remarkable tag "Notebook" "work"           add tag
// remarkable tag "Notebook" --rm "work"      remove tag
// remarkable tag "Notebook" --page 3 "todo"  tag a page
// remarkable tags                            list all tags
// remarkable ls --tag "work"                 filter by tag

var tagCmd = &cobra.Command{
	Use:   "tag <document> <tag>",
	Short: "Add or remove a tag on a document or page",
	Long: `Add or remove tags on a document, or on a specific page within a document.`,
	Example: `  remarkable tag "Notebook" work
  remarkable tag "Notebook" work --rm
  remarkable tag "Notebook" important --page 3`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		docName := args[0]
		tagName := args[1]
		rmTag, _ := cmd.Flags().GetBool("rm")
		pageNum, _ := cmd.Flags().GetInt("page")

		sshT, err := getSSH()
		if err != nil {
			return err
		}
		defer sshT.Close()

		doc, err := findDoc(sshT, docName)
		if err != nil {
			return err
		}

		_, rawContent, err := readContent(sshT, doc.ID)
		if err != nil {
			return err
		}

		var raw map[string]any
		json.Unmarshal(rawContent, &raw)

		if pageNum > 0 {
			// page-level tag
			pageTags, _ := raw["pageTags"].([]any)
			if rmTag {
				pageTags = removePageTag(pageTags, pageNum-1, tagName)
			} else {
				pageTags = addPageTag(pageTags, pageNum-1, tagName)
			}
			raw["pageTags"] = pageTags
		} else {
			// document-level tag
			tags := getStringSlice(raw, "tags")
			if rmTag {
				tags = removeString(tags, tagName)
			} else {
				tags = addString(tags, tagName)
			}
			raw["tags"] = tags
		}

		newContent, _ := json.MarshalIndent(raw, "", "    ")
		contentPath := filepath.Join("/home/root/.local/share/remarkable/xochitl", doc.ID+".content")
		if err := sshT.WriteRawFile(contentPath, newContent); err != nil {
			return err
		}

		sshT.RunCommand("systemctl restart xochitl")

		action := "added"
		if rmTag {
			action = "removed"
		}
		scope := "document"
		if pageNum > 0 {
			scope = fmt.Sprintf("page %d", pageNum)
		}

		output(map[string]any{
			"document": doc.Name,
			"tag":      tagName,
			"action":   action,
			"scope":    scope,
			"status":   "ok",
		})
		return nil
	},
}

var tagsListCmd = &cobra.Command{
	Use:   "tags",
	Short: "List all tags across the library",
	Long: `List every tag in use across the document library, with the documents they appear on. Requires SSH.`,
	Example: `  remarkable tags`,
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := getTransport()
		if err != nil {
			return err
		}
		defer t.Close()

		docs, err := t.ListDocuments()
		if err != nil {
			return err
		}

		// need SSH for reading .content files
		ssh, err := getSSH()
		if err != nil {
			return err
		}
		defer ssh.Close()

		tagMap := make(map[string][]string) // tag → doc names
		for _, doc := range docs {
			if doc.IsTrashed() {
				continue
			}

			content, _, err := readContent(ssh, doc.ID)
			if err != nil {
				continue
			}

			// get tags from raw content (Content struct doesn't have tags field)
			rc, err := ssh.ReadFile(doc.ID, "content")
			if err != nil {
				continue
			}
			var raw map[string]any
			json.NewDecoder(rc).Decode(&raw)
			rc.Close()

			tags := getStringSlice(raw, "tags")
			for _, tag := range tags {
				tagMap[tag] = append(tagMap[tag], doc.Name)
			}
			_ = content
		}

		if len(tagMap) == 0 {
			output(map[string]any{"tags": []string{}, "message": "no tags found"})
			return nil
		}

		var result []map[string]any
		for tag, docNames := range tagMap {
			result = append(result, map[string]any{
				"tag":       tag,
				"documents": docNames,
				"count":     len(docNames),
			})
		}
		output(result)
		return nil
	},
}

// --- helpers ---

func getStringSlice(raw map[string]any, key string) []string {
	arr, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func addString(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, existing := range slice {
		if existing != s {
			result = append(result, existing)
		}
	}
	return result
}

func addPageTag(pageTags []any, pageIdx int, tag string) []any {
	// pageTags is an array where index = page index
	// each entry is an array of tag strings
	for len(pageTags) <= pageIdx {
		pageTags = append(pageTags, []any{})
	}

	tags, _ := pageTags[pageIdx].([]any)
	// check if already exists
	for _, t := range tags {
		if s, ok := t.(string); ok && s == tag {
			return pageTags
		}
	}
	tags = append(tags, tag)
	pageTags[pageIdx] = tags
	return pageTags
}

func removePageTag(pageTags []any, pageIdx int, tag string) []any {
	if pageIdx >= len(pageTags) {
		return pageTags
	}

	tags, _ := pageTags[pageIdx].([]any)
	var newTags []any
	for _, t := range tags {
		if s, ok := t.(string); ok && s != tag {
			newTags = append(newTags, t)
		}
	}
	pageTags[pageIdx] = newTags
	return pageTags
}

func init() {
	tagCmd.Flags().Bool("rm", false, "remove the tag instead of adding")
	tagCmd.Flags().Int("page", 0, "tag a specific page (1-indexed)")
	rootCmd.AddCommand(tagCmd)
	rootCmd.AddCommand(tagsListCmd)
}
