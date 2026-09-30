// Package db is the player's SQLite store. Files are split by table/concern:
//
//	store.go      Open, schema, Store
//	tracks.go     catalog rows and paths
//	history.go    listens, transitions, rec_stats, impressions
//	profile.go    taste snapshots and top artists/clusters
//	playlists.go  generated mixes, later, favorites
//	user_playlists.go user/smart playlists
//	contexts.go   taste contexts and session activation
//	rules.go      radio block/downrank/cooldown
//	entities.go   artist/album vectors
//	tags.go       custom tags and preferences
//	transition_stats.go outcome-aware A→B weights
//	jobs.go       worker job queue
//	metrics.go    weekly recommendation metrics
//	explore.go    bandit arms and explore bounds
//	radio.go      public radio share tokens
//	sessions.go   persisted play sessions
//	lyrics.go     lyrics rows
package db
