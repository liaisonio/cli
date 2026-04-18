// Package liaisoncli exposes assets that are embedded into the CLI binary at
// build time — currently the agent skill files under ./skills, which the
// `liaison skills install` subcommand writes into the user's Claude config.
package liaisoncli

import "embed"

// SkillsFS contains every file under ./skills, preserving the directory
// structure. Walk with fs.WalkDir; read files with fs.ReadFile.
//
//go:embed all:skills
var SkillsFS embed.FS
