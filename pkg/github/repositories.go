package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	ghErrors "github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetCommit(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_commit",
			mcp.WithDescription(t("TOOL_GET_COMMITS_DESCRIPTION", "Get details for a commit from a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_COMMITS_USER_TITLE", "Get commit details"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("sha",
				mcp.Required(),
				mcp.Description("Commit SHA, branch name, or tag name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sha, err := RequiredParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.ListOptions{
				Page:    pagination.page,
				PerPage: pagination.perPage,
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			commit, resp, err := client.Repositories.GetCommit(ctx, owner, repo, sha, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to get commit: %s", sha),
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get commit: %s", string(body))), nil
			}

			r, err := json.Marshal(commit)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListCommits creates a tool to get commits of a branch in a repository.
func ListCommits(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_commits",
			mcp.WithDescription(t("TOOL_LIST_COMMITS_DESCRIPTION", "Get list of commits of a branch in a GitHub repository. Returns at least 30 results per page by default, but can return more if specified using the perPage parameter (up to 100).")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_COMMITS_USER_TITLE", "List commits"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("sha",
				mcp.Description("Commit SHA, branch or tag name to list commits of. If not provided, uses the default branch of the repository. If a commit SHA is provided, will list commits up to that SHA."),
			),
			mcp.WithString("author",
				mcp.Description("Author username or email address to filter commits by"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sha, err := OptionalParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			author, err := OptionalParam[string](request, "author")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Set default perPage to 30 if not provided
			perPage := pagination.perPage
			if perPage == 0 {
				perPage = 30
			}
			opts := &github.CommitsListOptions{
				SHA:    sha,
				Author: author,
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to list commits: %s", sha),
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list commits: %s", string(body))), nil
			}

			r, err := json.Marshal(commits)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListBranches creates a tool to list branches in a GitHub repository.
func ListBranches(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_branches",
			mcp.WithDescription(t("TOOL_LIST_BRANCHES_DESCRIPTION", "List branches in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_BRANCHES_USER_TITLE", "List branches"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.BranchListOptions{
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: pagination.perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			branches, resp, err := client.Repositories.ListBranches(ctx, owner, repo, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to list branches",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list branches: %s", string(body))), nil
			}

			r, err := json.Marshal(branches)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateOrUpdateFile creates a tool to create or update a file in a GitHub repository.
func CreateOrUpdateFile(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_or_update_file",
			mcp.WithDescription(t("TOOL_CREATE_OR_UPDATE_FILE_DESCRIPTION", "Create or update a single file in a GitHub repository. If updating, you must provide the SHA of the file you want to update. Use this tool to create or update a file in a GitHub repository remotely; do not use it for local file operations.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_OR_UPDATE_FILE_USER_TITLE", "Create or update file"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path where to create/update the file"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Content of the file"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to create/update the file in"),
			),
			mcp.WithString("sha",
				mcp.Description("SHA of file being replaced (for updates)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := RequiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			content, err := RequiredParam[string](request, "content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := RequiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := RequiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// json.Marshal encodes byte arrays with base64, which is required for the API.
			contentBytes := []byte(content)

			// Create the file options
			opts := &github.RepositoryContentFileOptions{
				Message: github.Ptr(message),
				Content: contentBytes,
				Branch:  github.Ptr(branch),
			}

			// If SHA is provided, set it (for updates)
			sha, err := OptionalParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if sha != "" {
				opts.SHA = github.Ptr(sha)
			}

			// Create or update the file
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			fileContent, resp, err := client.Repositories.CreateFile(ctx, owner, repo, path, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create/update file",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 && resp.StatusCode != 201 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create/update file: %s", string(body))), nil
			}

			r, err := json.Marshal(fileContent)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateRepository creates a tool to create a new GitHub repository.
func CreateRepository(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_repository",
			mcp.WithDescription(t("TOOL_CREATE_REPOSITORY_DESCRIPTION", "Create a new GitHub repository in your account")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_REPOSITORY_USER_TITLE", "Create repository"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("description",
				mcp.Description("Repository description"),
			),
			mcp.WithBoolean("private",
				mcp.Description("Whether repo should be private"),
			),
			mcp.WithBoolean("autoInit",
				mcp.Description("Initialize with README"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := RequiredParam[string](request, "name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			description, err := OptionalParam[string](request, "description")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			private, err := OptionalParam[bool](request, "private")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			autoInit, err := OptionalParam[bool](request, "autoInit")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			repo := &github.Repository{
				Name:        github.Ptr(name),
				Description: github.Ptr(description),
				Private:     github.Ptr(private),
				AutoInit:    github.Ptr(autoInit),
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			createdRepo, resp, err := client.Repositories.Create(ctx, "", repo)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create repository",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create repository: %s", string(body))), nil
			}

			r, err := json.Marshal(createdRepo)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// GetFileContents creates a tool to get the contents of a file or directory from a GitHub repository.
func GetFileContents(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_file_contents",
			mcp.WithDescription(t("TOOL_GET_FILE_CONTENTS_DESCRIPTION", "Get the contents of a file or directory from a GitHub repository. For files, this tool always returns the file content as a resource and its SHA hash.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_FILE_CONTENTS_USER_TITLE", "Get file or directory contents"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to file/directory (directories must end with a slash '/')"),
			),
			mcp.WithString("ref",
				mcp.Description("Accepts optional git refs such as `refs/tags/{tag}`, `refs/heads/{branch}` or `refs/pull/{pr_number}/head`"),
			),
			mcp.WithString("sha",
				mcp.Description("Accepts optional commit SHA. If specified, it will be used to fetch the file at a specific version to avoid race conditions."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := RequiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			ref, err := OptionalParam[string](request, "ref")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			userSHA, err := OptionalParam[string](request, "sha")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// ディレクトリの場合の処理
			if strings.HasSuffix(path, "/") {
				opts := &github.RepositoryContentGetOptions{Ref: ref}
				if userSHA != "" {
					opts.Ref = userSHA // refよりshaを優先
				}
				_, dirContent, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, opts)
				if err != nil {
					return ghErrors.NewGitHubAPIErrorResponse(ctx, "failed to get directory contents", resp, err), nil
				}
				defer func() { _ = resp.Body.Close() }()

				r, err := json.Marshal(dirContent)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal directory content: %w", err)
				}
				return mcp.NewToolResultText(string(r)), nil
			}

			// ファイルの場合の処理
			var fileSHA string
			if userSHA != "" {
				fileSHA = userSHA
			} else {
				getOpts := &github.RepositoryContentGetOptions{Ref: ref}
				fileContent, _, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, getOpts)
				if err != nil {
					return ghErrors.NewGitHubAPIErrorResponse(ctx, fmt.Sprintf("failed to get metadata for file: %s", path), resp, err), nil
				}
				if fileContent == nil {
					return mcp.NewToolResultError(fmt.Sprintf("The path '%s' is not a file or does not exist in the repository.", path)), nil
				}
				fileSHA = fileContent.GetSHA()
			}

			rawClient, err := getRawClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub raw content client: %w", err)
			}

			rawOpts := &raw.ContentOpts{SHA: fileSHA}
			resp, err := rawClient.GetRawContent(ctx, owner, repo, path, rawOpts)
			if err != nil {
				return nil, fmt.Errorf("failed to get raw repository content: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("failed to download file content, status code %d: %s", resp.StatusCode, string(body))
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}

			resourceURI, err := url.JoinPath("repo://", owner, repo, "sha", fileSHA, "contents", path)
			if err != nil {
				return nil, fmt.Errorf("failed to create resource URI: %w", err)
			}

			contentType := http.DetectContentType(body)
			
			shaTextPayload := map[string]string{"sha": fileSHA}
			shaText, err := json.Marshal(shaTextPayload)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal sha payload: %w", err)
			}

			if strings.HasPrefix(contentType, "text/") || contentType == "application/json" {
				return mcp.NewToolResultResource(string(shaText), mcp.TextResourceContents{
					URI:      resourceURI,
					Text:     string(body),
					MIMEType: contentType,
				}), nil
			}

			return mcp.NewToolResultResource(string(shaText), mcp.BlobResourceContents{
				URI:      resourceURI,
				Blob:     base64.StdEncoding.EncodeToString(body),
				MIMEType: contentType,
			}), nil
		}
}

// ForkRepository creates a tool to fork a repository.
func ForkRepository(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("fork_repository",
			mcp.WithDescription(t("TOOL_FORK_REPOSITORY_DESCRIPTION", "Fork a GitHub repository to your account or specified organization")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_FORK_REPOSITORY_USER_TITLE", "Fork repository"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("organization",
				mcp.Description("Organization to fork to"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			org, err := OptionalParam[string](request, "organization")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.RepositoryCreateForkOptions{}
			if org != "" {
				opts.Organization = org
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			forkedRepo, resp, err := client.Repositories.CreateFork(ctx, owner, repo, opts)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusAccepted {
					return mcp.NewToolResultText("Fork is in progress"), nil
				}
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to fork repository",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusAccepted {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to fork repository: %s", string(body))), nil
			}

			r, err := json.Marshal(forkedRepo)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// DeleteFile creates a tool to delete a file in a GitHub repository.
func DeleteFile(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("delete_file",
			mcp.WithDescription(t("TOOL_DELETE_FILE_DESCRIPTION", "Delete a file from a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:           t("TOOL_DELETE_FILE_USER_TITLE", "Delete file"),
				ReadOnlyHint:    ToBoolPtr(false),
				DestructiveHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner (username or organization)"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the file to delete"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to delete the file from"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := RequiredParam[string](request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := RequiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := RequiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
			if err != nil {
				return nil, fmt.Errorf("failed to get branch reference: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			baseCommit, resp, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get base commit",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get commit: %s", string(body))), nil
			}

			treeEntries := []*github.TreeEntry{
				{
					Path: github.Ptr(path),
					Mode: github.Ptr("100644"), 
					Type: github.Ptr("blob"),
					SHA:  nil, 
				},
			}

			newTree, resp, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, treeEntries)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create tree",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create tree: %s", string(body))), nil
			}

			commit := &github.Commit{
				Message: github.Ptr(message),
				Tree:    newTree,
				Parents: []*github.Commit{{SHA: baseCommit.SHA}},
			}
			newCommit, resp, err := client.Git.CreateCommit(ctx, owner, repo, commit, nil)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create commit",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to create commit: %s", string(body))), nil
			}

			ref.Object.SHA = newCommit.SHA
			_, resp, err = client.Git.UpdateRef(ctx, owner, repo, ref, false)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to update reference",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to update reference: %s", string(body))), nil
			}

			response := map[string]interface{}{
				"commit":  newCommit,
				"content": nil,
			}

			r, err := json.Marshal(response)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// CreateBranch creates a tool to create a new branch.
func CreateBranch(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("create_branch",
			mcp.WithDescription(t("TOOL_CREATE_BRANCH_DESCRIPTION", "Create a new branch in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_CREATE_BRANCH_USER_TITLE", "Create branch"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Name for new branch"),
			),
			mcp.WithString("from_branch",
				mcp.Description("Source branch (defaults to repo default)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := RequiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			fromBranch, err := OptionalParam[string](request, "from_branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			var ref *github.Reference

			if fromBranch == "" {
				repository, resp, err := client.Repositories.Get(ctx, owner, repo)
				if err != nil {
					return ghErrors.NewGitHubAPIErrorResponse(ctx,
						"failed to get repository",
						resp,
						err,
					), nil
				}
				defer func() { _ = resp.Body.Close() }()

				fromBranch = *repository.DefaultBranch
			}

			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+fromBranch)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get reference",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			newRef := &github.Reference{
				Ref:    github.Ptr("refs/heads/" + branch),
				Object: &github.GitObject{SHA: ref.Object.SHA},
			}

			createdRef, resp, err := client.Git.CreateRef(ctx, owner, repo, newRef)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create branch",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			r, err := json.Marshal(createdRef)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// PushFiles creates a tool to push multiple files in a single commit to a GitHub repository.
func PushFiles(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("push_files",
			mcp.WithDescription(t("TOOL_PUSH_FILES_DESCRIPTION", "Push multiple files to a GitHub repository in a single commit")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_PUSH_FILES_USER_TITLE", "Push files to repository"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to push to"),
			),
			mcp.WithArray("files",
				mcp.Required(),
				mcp.Items(
					map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"path", "content"},
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "path to the file",
							},
							"content": map[string]interface{}{
								"type":        "string",
								"description": "file content",
							},
						},
					}),
				mcp.Description("Array of file objects to push, each object with path (string) and content (string)"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			branch, err := RequiredParam[string](request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			message, err := RequiredParam[string](request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			filesObj, ok := request.GetArguments()["files"].([]interface{})
			if !ok {
				return mcp.NewToolResultError("files parameter must be an array of objects with path and content"), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get branch reference",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			baseCommit, resp, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get base commit",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			var entries []*github.TreeEntry

			for _, file := range filesObj {
				fileMap, ok := file.(map[string]interface{})
				if !ok {
					return mcp.NewToolResultError("each file must be an object with path and content"), nil
				}

				path, ok := fileMap["path"].(string)
				if !ok || path == "" {
					return mcp.NewToolResultError("each file must have a path"), nil
				}

				content, ok := fileMap["content"].(string)
				if !ok {
					return mcp.NewToolResultError("each file must have content"), nil
				}

				entries = append(entries, &github.TreeEntry{
					Path:    github.Ptr(path),
					Mode:    github.Ptr("100644"),
					Type:    github.Ptr("blob"),
					Content: github.Ptr(content),
				})
			}

			newTree, resp, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, entries)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create tree",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			commit := &github.Commit{
				Message: github.Ptr(message),
				Tree:    newTree,
				Parents: []*github.Commit{{SHA: baseCommit.SHA}},
			}
			newCommit, resp, err := client.Git.CreateCommit(ctx, owner, repo, commit, nil)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to create commit",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			ref.Object.SHA = newCommit.SHA
			updatedRef, resp, err := client.Git.UpdateRef(ctx, owner, repo, ref, false)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to update reference",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			r, err := json.Marshal(updatedRef)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListTags creates a tool to list tags in a GitHub repository.
func ListTags(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_tags",
			mcp.WithDescription(t("TOOL_LIST_TAGS_DESCRIPTION", "List git tags in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_TAGS_USER_TITLE", "List tags"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.ListOptions{
				Page:    pagination.page,
				PerPage: pagination.perPage,
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to list tags",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to list tags: %s", string(body))), nil
			}

			r, err := json.Marshal(tags)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// GetTag creates a tool to get details about a specific tag in a GitHub repository.
func GetTag(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_tag",
			mcp.WithDescription(t("TOOL_GET_TAG_DESCRIPTION", "Get details about a specific git tag in a GitHub repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_TAG_USER_TITLE", "Get tag details"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithString("tag",
				mcp.Required(),
				mcp.Description("Tag name"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			tag, err := RequiredParam[string](request, "tag")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			ref, resp, err := client.Git.GetRef(ctx, owner, repo, "refs/tags/"+tag)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get tag reference",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get tag reference: %s", string(body))), nil
			}

			tagObj, resp, err := client.Git.GetTag(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to get tag object",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get tag object: %s", string(body))), nil
			}

			r, err := json.Marshal(tagObj)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
