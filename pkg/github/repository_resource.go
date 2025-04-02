package github

import (
	"context"
	"encoding/base64"
	"errors"
	"mime"
	"path/filepath"
	"strings"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v69/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getRepositoryResourceContent defines the resource template and handler for getting repository content.
func getRepositoryResourceContent(client *github.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_DESCRIPTION", "Repository Content"),
		),
		repositoryResourceContentsHandler(client)
}

// getRepositoryContent defines the resource template and handler for getting repository content for a branch.
func getRepositoryResourceBranchContent(client *github.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/heads/{branch}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_BRANCH_DESCRIPTION", "Repository Content for specific branch"),
		),
		repositoryResourceContentsHandler(client)
}

// getRepositoryResourceCommitContent defines the resource template and handler for getting repository content for a commit.
func getRepositoryResourceCommitContent(client *github.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/sha/{sha}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_COMMIT_DESCRIPTION", "Repository Content for specific commit"),
		),
		repositoryResourceContentsHandler(client)
}

// getRepositoryResourceTagContent defines the resource template and handler for getting repository content for a tag.
func getRepositoryResourceTagContent(client *github.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/tags/{tag}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_TAG_DESCRIPTION", "Repository Content for specific tag"),
		),
		repositoryResourceContentsHandler(client)
}

// getRepositoryResourcePrContent defines the resource template and handler for getting repository content for a pull request.
func getRepositoryResourcePrContent(client *github.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/pull/{pr_number}/head/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_PR_DESCRIPTION", "Repository Content for specific pull request"),
		),
		repositoryResourceContentsHandler(client)
}

func repositoryResourceContentsHandler(client *github.Client) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// the matcher will give []string with one elemenent
		// https://github.com/mark3labs/mcp-go/pull/54
		o, ok := request.Params.Arguments["owner"].([]string)
		if !ok || len(o) == 0 {
			return nil, errors.New("owner is required")
		}
		owner := o[0]

		r, ok := request.Params.Arguments["repo"].([]string)
		if !ok || len(r) == 0 {
			return nil, errors.New("repo is required")
		}
		repo := r[0]

		// path should be a joined list of the path parts
		path := strings.Join(request.Params.Arguments["path"].([]string), "/")

		opts := &github.RepositoryContentGetOptions{}

		sha, ok := request.Params.Arguments["sha"].([]string)
		if ok && len(sha) > 0 {
			opts.Ref = sha[0]
		}

		branch, ok := request.Params.Arguments["branch"].([]string)
		if ok && len(branch) > 0 {
			opts.Ref = "refs/heads/" + branch[0]
		}

		tag, ok := request.Params.Arguments["tag"].([]string)
		if ok && len(tag) > 0 {
			opts.Ref = "refs/tags/" + tag[0]
		}
		prNumber, ok := request.Params.Arguments["pr_number"].([]string)
		if ok && len(prNumber) > 0 {
			opts.Ref = "refs/pull/" + prNumber[0] + "/head"
		}

		fileContent, directoryContent, _, err := client.Repositories.GetContents(ctx, owner, repo, path, opts)
		if err != nil {
			return nil, err
		}

		if directoryContent != nil {
			var resources []mcp.ResourceContents
			for _, entry := range directoryContent {
				mimeType := "text/directory"
				if entry.GetType() == "file" {
					mimeType = mime.TypeByExtension(filepath.Ext(entry.GetName()))
				}
				resources = append(resources, mcp.TextResourceContents{
					URI:      entry.GetHTMLURL(),
					MIMEType: mimeType,
					Text:     entry.GetName(),
				})

			}
			return resources, nil

		}
		if fileContent != nil {
			if fileContent.Content != nil {
				decodedContent, err := fileContent.GetContent()
				if err != nil {
					return nil, err
				}

				mimeType := mime.TypeByExtension(filepath.Ext(fileContent.GetName()))

				if strings.HasPrefix(mimeType, "text") {
					return []mcp.ResourceContents{
						mcp.TextResourceContents{
							URI:      request.Params.URI,
							MIMEType: mimeType,
							Text:     decodedContent,
						},
					}, nil
				}

				return []mcp.ResourceContents{
					mcp.BlobResourceContents{
						URI:      request.Params.URI,
						MIMEType: mimeType,
						Blob:     base64.StdEncoding.EncodeToString([]byte(decodedContent)), // Encode content as Base64
					},
				}, nil
			}
		}

		return nil, nil
	}
}
