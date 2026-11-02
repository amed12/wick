package main

import (
	"github.com/yogasw/wick/pkg/connector"
	"github.com/yogasw/wick/pkg/entity"
	"github.com/yogasw/wick/pkg/wickdocs"
	"github.com/yogasw/wick/plugins/tags"
)

// Config is per-instance configuration. Username + Profile are what make the
// saved-collection URLs work; the rest is pacing.
type Config struct {
	Username   string `wick:"required;default=fathullahachmad;desc=Instagram username whose saved collections are read. Must match the account logged in inside the browser profile."`
	Profile    string `wick:"default=instagram;desc=playwright_browser named profile holding that account's login. Create it once: open a session, log in by hand, close it — the profile keeps the cookies."`
	InstanceID string `wick:"desc=Optional playwright_browser instance id. Empty = the first instance the caller can see."`
	MaxPages   int    `wick:"default=3;desc=GraphQL pages of 100 posts per collection call (1-10). More pages = longer call; every browser op stops at 45s."`
	DelayMS    int    `wick:"default=650;desc=Pause between pages, in milliseconds. Instagram rate-limits aggressively below ~500."`
}

// ---------------------------------------------------------------- inputs

type loginStatusInput struct{}

type collectionInput struct {
	Collection string `wick:"required;desc=Saved collection as a URL (https://www.instagram.com/<user>/saved/ai/18326269657221437/) or slug/id (ai/18326269657221437)."`
	After      string `wick:"desc=Continuation cursor from a previous call's next_cursor. Leave empty to start from the newest saved post."`
	MaxPages   int    `wick:"desc=Override the configured page count for this call (1-10)."`
}

type postInput struct {
	URL string `wick:"required;desc=Instagram post or reel URL. Example: https://www.instagram.com/p/DfL5rjiGOKd/"`
}

// ---------------------------------------------------------------- module

func Module() connector.Module {
	return connector.Module{
		Meta: connector.Meta{
			Key:         "instagram",
			Name:        "Instagram Saved",
			Description: "Read saved collections and single posts from Instagram through a logged-in browser session. Returns caption, owner, media type and a continuation cursor.",
			Icon:        "📸",
			DefaultTags: []entity.DefaultTag{tags.Connector, tags.API},
		},
		Configs: entity.StructToConfigs(Config{}),
		Operations: []connector.Category{
			connector.Cat("Session", "Check the browser login before reading anything.",
				connector.Op("login_status", "Login Status",
					"Report whether the browser profile is still logged in to Instagram: returns url, title and logged_in. Call it first when a read fails.",
					loginStatusInput{}, handleLoginStatus, wickdocs.Docs{}),
			),
			connector.Cat("Saved", "Read saved collections.",
				connector.Op("collection", "Read Saved Collection",
					"Return up to max_pages×100 saved posts from one collection: code, owner, url, media type, carousel count and caption. Pass next_cursor back as after to continue. Posts come newest-first.",
					collectionInput{}, handleCollection, wickdocs.Docs{}),
			),
			connector.Cat("Posts", "Read one post.",
				connector.Op("post", "Read Post",
					"Open one Instagram post or reel and return its canonical url, title, description (caption) and video url when the post is a video.",
					postInput{}, handlePost, wickdocs.Docs{}),
			),
		},
	}
}
