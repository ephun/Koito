package sources

import "github.com/gabehf/koito/internal/filters"

func assignWhen(condition filters.Expr, target string, value filters.Expr) *filters.Program {
	return &filters.Program{Steps: []filters.Statement{{If: &condition, Then: []filters.Statement{{Set: target, Value: &value}}}}}
}

func contextKindCondition(kind string) filters.Expr {
	return operation("and", operation("present", fieldExpr("event.kind")), operation("eq", fieldExpr("event.kind"), literalExpr(kind)))
}

func performanceTemplates(state *FilterState) {
	yarg, concerts := false, false
	for _, source := range state.Sources {
		yarg = yarg || source.Service == "yarg"
		concerts = concerts || source.Service == "setlistfm"
	}
	state.Blocks.Templates["inference:context"] = []ProgramTemplate{{"yargfm", "YARGfm input", assignWhen(operation("and", operation("present", fieldExpr("record.source_input")), operation("eq", fieldExpr("record.source_input"), literalExpr("YARGfm"))), "context.kind", literalExpr("gameplay"))}}
	fmResult := assignWhen(operation("and", operation("present", fieldExpr("record.source_input")), operation("eq", fieldExpr("record.source_input"), literalExpr("YARGfm"))), "time.meaning", literalExpr("result"))
	precision := operation("seconds", literalExpr(1))
	fmResult.Steps[0].Then = append(fmResult.Steps[0].Then, filters.Statement{Set: "time.precision", Value: &precision})
	state.Blocks.Templates["inference:time"] = append(state.Blocks.Templates["inference:time"], ProgramTemplate{"yargfm-result", "YARGfm result times", fmResult})
	if yarg {
		gameScrobble := operation("or", operation("and", operation("eq", fieldExpr("left.event.kind"), literalExpr("gameplay")), operation("eq", fieldExpr("right.source.service"), literalExpr("lastfm"))), operation("and", operation("eq", fieldExpr("right.event.kind"), literalExpr("gameplay")), operation("eq", fieldExpr("left.source.service"), literalExpr("lastfm"))))
		state.Blocks.Templates["matching:play"] = append(state.Blocks.Templates["matching:play"], ProgramTemplate{"game-scrobble", "Gameplay scrobbles", assignWhen(gameScrobble, "match.play", fieldExpr("pair.overlap"))})
		state.Blocks.Templates["inference:time"] = append(state.Blocks.Templates["inference:time"], ProgramTemplate{"yarg-zone", "YARG timezone", legacyTimeProgram(Rule{Scope: RuleScope{Service: "yarg"}, Settings: RuleSettings{Timezone: "UTC"}})})
		condition := operation("and", operation("eq", fieldExpr("source.format"), literalExpr(YARGDB)), operation("present", fieldExpr("game.catalog.duration_ms")), operation("present", fieldExpr("game.speed")), operation("gt", fieldExpr("game.speed"), literalExpr(0)))
		at := operation("subtract", fieldExpr("record.time"), operation("milliseconds", operation("divide", fieldExpr("game.catalog.duration_ms"), fieldExpr("game.speed"))))
		start := assignWhen(condition, "time.at", at)
		state.Blocks.Templates["inference:time"] = append(state.Blocks.Templates["inference:time"], ProgramTemplate{"yarg-start", "Estimate YARG starts", start})
		player := operation("and", operation("eq", fieldExpr("source.format"), literalExpr(YARGDB)), operation("present", fieldExpr("game.player_names")), operation("in", literalExpr(""), fieldExpr("game.player_names")))
		state.Blocks.Templates["attribution:direct"] = append(state.Blocks.Templates["attribution:direct"], ProgramTemplate{"yarg-player", "YARG player", assignWhen(player, "listener", literalExpr("self"))})
		complete := operation("and", operation("present", fieldExpr("game.with_replay")), operation("not", fieldExpr("game.with_replay")), operation("gt", fieldExpr("game.speed"), literalExpr(0)), operation("gt", operation("count", fieldExpr("game.scores")), literalExpr(0)))
		state.Blocks.Templates["qualification:completion"] = append(state.Blocks.Templates["qualification:completion"], ProgramTemplate{"yarg-completion", "YARG completed games", assignWhen(operation("eq", fieldExpr("source.format"), literalExpr(YARGDB)), "completion.count", complete)})
	}
	if concerts {
		attendance := operation("and", contextKindCondition("concert"), operation("present", fieldExpr("concert.attended")), fieldExpr("concert.attended"))
		state.Blocks.Templates["attribution:direct"] = append(state.Blocks.Templates["attribution:direct"], ProgramTemplate{"concert-attendance", "Concert attendance", assignWhen(attendance, "listener", literalExpr("self"))})
	}
	for _, kind := range []string{"gameplay", "concert"} {
		if kind == "gameplay" && !yarg || kind == "concert" && !concerts {
			continue
		}
		label := "Gameplay"
		if kind == "concert" {
			label = "Concerts"
		}
		state.Blocks.Templates["inclusion:scope"] = append(state.Blocks.Templates["inclusion:scope"], ProgramTemplate{kind, label, assignWhen(contextKindCondition(kind), "view.include", literalExpr(true))})
	}
	for key, schema := range state.Blocks.Schemas {
		if schema.Choices == nil {
			schema.Choices = map[string][]filters.Choice{}
		}
		if schema.Fields["event.kind"] != "" {
			schema.Choices["event.kind"] = []filters.Choice{{Label: "Gameplay", Value: "gameplay"}, {Label: "Concert", Value: "concert"}, {Label: "Unspecified", Value: "unknown"}}
		}
		if schema.Fields["source.format"] != "" {
			schema.Choices["source.format"] = append(schema.Choices["source.format"], filters.Choice{Label: "YARG scores database", Value: YARGDB}, filters.Choice{Label: "YARG song cache", Value: YARGCache}, filters.Choice{Label: "Clone Hero scores", Value: CloneScores}, filters.Choice{Label: "Clone Hero song cache", Value: CloneCache}, filters.Choice{Label: "Setlist.fm JSON", Value: SetlistJSON})
		}
		state.Blocks.Schemas[key] = schema
	}
}
