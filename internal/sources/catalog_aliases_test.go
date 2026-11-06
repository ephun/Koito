package sources

import (
	"context"
	"testing"

	"github.com/gabehf/koito/internal/filters"
	"github.com/stretchr/testify/require"
)

func TestCatalogAliasesValidateAtomicallyWithNames(t *testing.T) {
	metadata := filters.Env{"original.aliases": aliasList([]string{"Original", "Other spelling"})}
	for _, test := range []struct {
		value string
		valid bool
		want  []string
	}{
		{`["Alternate","Alternate","alternate"]`, true, []string{"Alternate", "alternate"}},
		{`[]`, true, []string{}},
		{`[" "]`, false, []string{"Original", "Other spelling"}},
		{`[42]`, false, []string{"Original", "Other spelling"}},
	} {
		t.Run(test.value, func(t *testing.T) {
			rule := Rule{ID: "aliases", Type: "presentation", Subtype: "naming", Enabled: true, Program: namingFixture(t, `{"steps":[{"if":{"op":"eq","args":[{"op":"field","field":"catalog.kind"},{"op":"literal","value":"artist"}]},"then":[{"set":"display.artist","value":{"op":"literal","value":"Corrected"}},{"set":"display.aliases","value":{"op":"literal","value":`+test.value+`}}]}]}`)}
			programs, err := compileNaming(RuleConfig{Rules: []Rule{rule}})
			require.NoError(t, err)
			result, issues, err := resolveCatalogNaming(context.Background(), programs, "artist", 1, "Original", metadata)
			require.NoError(t, err)
			require.Equal(t, test.want, result.Aliases)
			require.Equal(t, test.valid, result.AliasesAssigned)
			if test.valid {
				require.Empty(t, issues)
				require.Equal(t, "Corrected", result.Name)
			} else {
				require.NotEmpty(t, issues)
				require.Equal(t, "Original", result.Name)
			}
		})
	}
}
