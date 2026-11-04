package filters

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPairRawFieldsRequirePairSchemaAndValidPointers(t *testing.T) {
	for _, name := range []string{"left.raw:/Genre", "right.raw:/a~1b/~0key", "left.column:/Artist"} {
		require.True(t, ValidPairField(name))
		_, err := Compile(field(name), Schema{PairFields: true})
		require.NoError(t, err)
		_, err = Compile(field(name), Schema{RawFields: true})
		require.Error(t, err)
	}
	for _, name := range []string{"raw:/Genre", "left.unknown", "left.raw:Genre", "right.raw:/bad~2", "right.column:/a/b"} {
		require.False(t, ValidPairField(name))
		_, err := Compile(field(name), Schema{PairFields: true})
		require.Error(t, err)
	}
}
