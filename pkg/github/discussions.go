package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
)

// Discussion represents a GitHub Discussion with its essential fields
type Discussion struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Author      string `json:"author"`
	Locked      bool   `json:"locked"`
	UpvoteCount int    `json:"upvoteCount"`
}

// GetRepositoryDiscussions creates a tool to fetch discussions from a specific repository.
func GetRepositoryDiscussions(getGraphQLClient GetGraphQLClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_repository_discussions",
			mcp.WithDescription(t("TOOL_GET_REPOSITORY_DISCUSSIONS_DESCRIPTION", "Get discussions from a specific GitHub repository")),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			categoryId, err := OptionalParam[string](request, "categoryId")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GraphQL client
			client, err := getGraphQLClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub GraphQL client: %w", err)
			}

			// Define GraphQL query variables
			variables := map[string]interface{}{
				"owner":      githubv4.String(owner),
				"name":       githubv4.String(repo),
				"first":      githubv4.Int(pagination.perPage),
				"after":      (*githubv4.String)(nil), // For pagination - null means first page
				"categoryId": (*githubv4.ID)(nil),     // For category ID - null means no filter
			}

			// For pagination beyond the first page
			// TODO: Fix this to use actual cursor values
			// This is a placeholder for the cursor logic
			// In a real implementation, you should store and use actual cursor values
			if pagination.perPage > 0 && pagination.page > 1 {
			if pagination.page > 1 {
				// We'd need an actual cursor here, but for simplicity we'll compute a rough offset
				// In real implementation, you should store and use actual cursor values
				cursorStr := githubv4.String(fmt.Sprintf("%d", (pagination.page-1)*pagination.perPage))
				variables["after"] = &cursorStr
			}

			if categoryId != "" {
				variables["categoryId"] = githubv4.ID(categoryId)
			}

			// Define the GraphQL query structure
			var query struct {
				Repository struct {
					Discussions struct {
						TotalCount int
						Nodes      []struct {
							ID        githubv4.ID
							Number    int
							Title     string
							Body      string
							CreatedAt githubv4.DateTime
							UpdatedAt githubv4.DateTime
							URL       githubv4.URI
							Category  struct {
								Name string
							}
							Author struct {
								Login string
							}
							Locked      bool
							UpvoteCount int
						}
						PageInfo struct {
							EndCursor   githubv4.String
							HasNextPage bool
						}
					} `graphql:"discussions(first: $first, after: $after, categoryId: $categoryId)"`
				} `graphql:"repository(owner: $owner, name: $name)"`
			}

			// Only include categoryId in the query if it was provided
			if categoryId == "" {
				// Redefine the query without the categoryId filter
				query.Repository.Discussions = struct {
					TotalCount int
					Nodes      []struct {
						ID        githubv4.ID
						Number    int
						Title     string
						Body      string
						CreatedAt githubv4.DateTime
						UpdatedAt githubv4.DateTime
						URL       githubv4.URI
						Category  struct {
							Name string
						}
						Author struct {
							Login string
						}
						Locked      bool
						UpvoteCount int
					}
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage bool
					}
				}{}
			}

			// Execute the GraphQL query
			err = client.Query(ctx, &query, variables)
			if err != nil {
				return nil, fmt.Errorf("failed to query discussions: %w", err)
			}

			// Convert the GraphQL response to our Discussion type
			discussions := make([]Discussion, 0, len(query.Repository.Discussions.Nodes))
			for _, node := range query.Repository.Discussions.Nodes {
				discussion := Discussion{
					ID:          fmt.Sprintf("%v", node.ID),
					Number:      node.Number,
					Title:       node.Title,
					Body:        node.Body,
					CreatedAt:   node.CreatedAt.String(),
					UpdatedAt:   node.UpdatedAt.String(),
					URL:         node.URL.String(),
					Category:    node.Category.Name,
					Author:      node.Author.Login,
					Locked:      node.Locked,
					UpvoteCount: node.UpvoteCount,
				}
				discussions = append(discussions, discussion)
			}

			// Create the response
			result := struct {
				TotalCount  int          `json:"totalCount"`
				Discussions []Discussion `json:"discussions"`
				HasNextPage bool         `json:"hasNextPage"`
				EndCursor   string       `json:"endCursor"`
			}{
				TotalCount:  query.Repository.Discussions.TotalCount,
				Discussions: discussions,
				HasNextPage: query.Repository.Discussions.PageInfo.HasNextPage,
				EndCursor:   string(query.Repository.Discussions.PageInfo.EndCursor),
			}

			// Marshal the result to JSON
			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussions result: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
