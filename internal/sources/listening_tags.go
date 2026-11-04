package sources

import "github.com/gabehf/koito/internal/filters"

func sourceTagProgram() *filters.Program {
	condition := operation("eq", fieldExpr("source.service"), literalExpr("spotify"))
	set := func(value filters.Expr) []filters.Statement {
		list := operation("list", value)
		return []filters.Statement{{Set: "display.tags", Value: &list}}
	}
	lastfm := operation("eq", fieldExpr("source.service"), literalExpr("lastfm"))
	itunes := operation("eq", fieldExpr("source.service"), literalExpr("itunes"))
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: set(literalExpr("Spotify")), ElseIf: []filters.Branch{{If: lastfm, Then: set(literalExpr("Last.fm"))}, {If: itunes, Then: set(literalExpr("iTunes"))}}, Else: set(fieldExpr("source.service"))}}}
}
