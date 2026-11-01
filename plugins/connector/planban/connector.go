package main

import (
	"github.com/yogasw/wick/pkg/connector"
	"github.com/yogasw/wick/pkg/entity"
	"github.com/yogasw/wick/pkg/wickdocs"
	"github.com/yogasw/wick/plugins/tags"
)

// Config is per-instance configuration. BaseURL points at the locally running
// Planban server; RepoID is the board, which Planban keys by its repoId slug
// (visible in /api/boards and in the /boards/<repoId> URL).
type Config struct {
	BaseURL string `wick:"url;default=http://127.0.0.1:4317;desc=Planban server base URL. Default: http://127.0.0.1:4317"`
	RepoID  string `wick:"required;default=fathullah-instagram-learning;desc=Board repoId. GET /api/boards lists every board and its repoId."`
}

// ---------------------------------------------------------------- inputs

type boardStateInput struct {
	RepoID string `wick:"desc=Override the configured board repoId."`
}

type findCardsInput struct {
	Query         string `wick:"desc=Case-insensitive substring matched against title, summary and topic."`
	Shortcode     string `wick:"desc=Instagram shortcode stored in metadata.instagramShortcode."`
	Group         string `wick:"desc=Group (column parent) title, case-insensitive. Example: Claude dan Claude Code"`
	Status        string `wick:"dropdown=pending|up-next|in-progress|complete|archived;desc=Only cards in this status."`
	MetadataKey   string `wick:"desc=Only cards whose metadata[key] equals metadata_value. Example: analysisStatus"`
	MetadataValue string `wick:"desc=Value metadata_key must equal. Example: pending"`
	Limit         int    `wick:"desc=Maximum cards returned. Default 50, max 500."`
	RepoID        string `wick:"desc=Override the configured board repoId."`
}

type upsertCardInput struct {
	Shortcode string `wick:"desc=Instagram shortcode or any stable external id. Used as the idempotency key; repeat calls update the same card instead of creating a new one."`
	Title     string `wick:"required;desc=Card title. Required when the card does not exist yet; ignored on update unless provided."`
	Summary   string `wick:"desc=One or two sentence summary shown on the card."`
	Group     string `wick:"desc=Group title to place the card under. Unknown group is an error — create it first with create_group."`
	Status    string `wick:"dropdown=pending|up-next|in-progress|complete|archived;desc=Card status. Default pending on create; unchanged on update."`
	Tags      string `wick:"desc=Comma-separated tags. Merged with existing tags on update."`
	SourceURL string `wick:"desc=Original URL of the saved item (stored in metadata.sourceUrl)."`
	Metadata  string `wick:"desc=Extra metadata as a JSON object string. Merged into existing metadata; existing keys win unless overwritten here."`
	RepoID    string `wick:"desc=Override the configured board repoId."`
}

type createGroupInput struct {
	Title   string `wick:"required;desc=Group title. Exact match is used for reuse, so keep it stable."`
	Summary string `wick:"desc=Group summary shown under the title."`
	RepoID  string `wick:"desc=Override the configured board repoId."`
}

type moveCardInput struct {
	CardID string `wick:"required;desc=Card id (stable slug shown in the board URL and returned by find_cards)."`
	Group  string `wick:"desc=Target group title. The group must already exist."`
	Status string `wick:"dropdown=pending|up-next|in-progress|complete|archived;desc=Target status."`
	RepoID string `wick:"desc=Override the configured board repoId."`
}

type setMetadataInput struct {
	CardID   string `wick:"required;desc=Card id whose metadata is updated."`
	Metadata string `wick:"required;desc=JSON object string merged into the card's metadata. Existing keys survive unless overridden here."`
	RepoID   string `wick:"desc=Override the configured board repoId."`
}

type writeDocInput struct {
	CardID   string `wick:"required;desc=Card id whose document is written."`
	Kind     string `wick:"dropdown=spec|plan;desc=Which document to write. Default spec."`
	Markdown string `wick:"textarea;required;desc=Full markdown body. It replaces the document — read it first (find_cards returns no doc body, so pass the whole text you want stored)."`
	RepoID   string `wick:"desc=Override the configured board repoId."`
}

type deleteCardInput struct {
	CardID string `wick:"required;desc=Card id to delete permanently."`
	RepoID string `wick:"desc=Override the configured board repoId."`
}

// ---------------------------------------------------------------- module

// Module returns the connector definition.
func Module() connector.Module {
	return connector.Module{
		Meta: connector.Meta{
			Key:         "planban",
			Name:        "Planban",
			Description: "Read and write cards on a local Planban board: list groups, find cards, upsert by external id, move between groups, delete.",
			Icon:        "🗂️",
			DefaultTags: []entity.DefaultTag{tags.Connector, tags.API, tags.Development},
		},
		Configs: entity.StructToConfigs(Config{}),
		Operations: []connector.Category{
			connector.Cat("Board", "Inspect the board before writing to it.",
				connector.Op("board_state", "Board State",
					"Return the board's groups with card counts, plus totals per status. Call this first to learn valid group titles.",
					boardStateInput{}, handleBoardState, wickdocs.Docs{}),
				connector.Op("find_cards", "Find Cards",
					"Search cards by text, Instagram shortcode, group title or status. Returns id, title, status, group, tags and the stored metadata. Use it to check whether something is already on the board before adding it.",
					findCardsInput{}, handleFindCards, wickdocs.Docs{}),
			),
			connector.Cat("Cards", "Write to the board.",
				connector.Op("upsert_card", "Upsert Card",
					"Create the card, or update the existing card that carries the same shortcode. Idempotent: calling it twice with one shortcode leaves exactly one card. Returns action=created|updated.",
					upsertCardInput{}, handleUpsertCard, wickdocs.Docs{}),
				connector.Op("create_group", "Create Group",
					"Create a group (column parent) if a group with that exact title does not exist yet. Returns the group id; reuse the title afterwards.",
					createGroupInput{}, handleCreateGroup, wickdocs.Docs{}),
				connector.Op("move_card", "Move Card",
					"Move a card to another group and/or status. Requires at least one of group or status.",
					moveCardInput{}, handleMoveCard, wickdocs.Docs{}),
				connector.Op("set_metadata", "Set Card Metadata",
					"Merge a JSON object into one card's metadata. Use it for bookkeeping flags (analysis status, transcript path) without touching the title or documents.",
					setMetadataInput{}, handleSetMetadata, wickdocs.Docs{}),
				connector.Op("write_doc", "Write Card Document",
					"Replace a card's spec or plan document with new markdown. Use it to attach an analysis (transcript, notes) to the card so the board carries the evidence.",
					writeDocInput{}, handleWriteDoc, wickdocs.Docs{}),
				connector.OpDestructive("delete_card", "Delete Card",
					"Permanently delete a card. Archives it first when needed (Planban only deletes archived cards), then removes it. Irreversible — notes on disk survive, the board entry does not.",
					deleteCardInput{}, handleDeleteCard, wickdocs.Docs{}),
			),
		},
	}
}
