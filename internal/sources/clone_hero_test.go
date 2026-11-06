package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func cloneScoreFixture(version uint32) []byte {
	var data bytes.Buffer
	_ = binary.Write(&data, binary.LittleEndian, version)
	_ = binary.Write(&data, binary.LittleEndian, uint32(1))
	if version == 20180220 {
		data.WriteByte(32)
		data.WriteString("0102030405060708090a0b0c0d0e0f10")
	} else {
		data.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	}
	data.Write([]byte{1, 4, 0, 0})
	_ = binary.Write(&data, binary.LittleEndian, uint16(0))
	data.WriteByte(3)
	_ = binary.Write(&data, binary.LittleEndian, uint16(95))
	_ = binary.Write(&data, binary.LittleEndian, uint16(100))
	data.WriteByte(5)
	data.WriteByte(8)
	if version == 20211009 {
		data.Write([]byte{0, 0, 0})
	}
	_ = binary.Write(&data, binary.LittleEndian, uint32(12345))
	return data.Bytes()
}

func TestCloneScoresPreserveVersionsWithoutInventingPlays(t *testing.T) {
	for _, version := range []uint32{20180220, 20211009} {
		s := testStore(t)
		ctx := context.Background()
		data := cloneScoreFixture(version)
		first, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		require.NoError(t, err)
		require.Equal(t, 3, first.Records)
		again, err := s.Import(ctx, bytes.NewReader(data), "copy.bin", 1)
		require.NoError(t, err)
		require.Zero(t, again.Added)
		result, err := s.Rebuild(ctx)
		require.NoError(t, err)
		require.Zero(t, result.Plays)
		path, err := s.FilePath(first.Hash)
		require.NoError(t, err)
		original, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, data, original)
		for _, broken := range [][]byte{data[:len(data)-1], append(append([]byte{}, data...), 1)} {
			_, err = s.Import(ctx, bytes.NewReader(broken), "broken.bin", 1)
			require.Error(t, err)
			require.Equal(t, 3, count(t, s, "source_records"))
		}
	}
}

func TestGameBinaryRejectsBoundsAndInvalidText(t *testing.T) {
	for _, data := range [][]byte{{255, 255, 255, 255, 255}, {128, 128, 128, 128, 8}, {2, 'a'}, {1, 255}} {
		r := gameBinary{data: data}
		r.text()
		require.Error(t, r.finish())
	}
	r := gameBinary{data: []byte{128, 1}}
	r.text()
	require.Error(t, r.finish())
}

func TestGameArchiveKeepsUnsupportedProfilesAndSettings(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, data := range map[string][]byte{"scores.bin": cloneScoreFixture(20180220), "profiles.ini": []byte("[profiles]\nname=Player\n"), "profiles.bin": {0x74, 0x18, 0x34, 0x01, 0, 0, 0, 0}} {
		member, err := writer.Create(name)
		require.NoError(t, err)
		_, err = member.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	data := archive.Bytes()
	first, err := s.Import(ctx, bytes.NewReader(data), "game.zip", 1)
	require.NoError(t, err)
	require.Equal(t, 3, first.Records)
	var members int
	require.NoError(t, s.DB.QueryRow(`SELECT COUNT(*) FROM source_members WHERE parent_hash=?`, first.Hash).Scan(&members))
	require.Equal(t, 3, members)
	_, err = s.Import(ctx, bytes.NewReader([]byte("[profiles]\nname=Player\n")), "profiles.ini", 1)
	require.ErrorIs(t, err, ErrUnsupportedFile)
}

func TestLocalPhase7CloneHero(t *testing.T) {
	root := os.Getenv("KOITO_TEST_PHASE7_CLONE")
	if root == "" {
		t.Skip("set KOITO_TEST_PHASE7_CLONE to the Clone Hero LocalLow export")
	}
	s := testStore(t)
	ctx := context.Background()
	for _, name := range []string{"scores.bin", "scores_backup.bin", "scoredata.bin", "songcache.bin"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		first, err := s.Import(ctx, bytes.NewReader(data), name, 1)
		require.NoError(t, err)
		again, err := s.Import(ctx, bytes.NewReader(data), "renamed.data", 1)
		require.NoError(t, err)
		require.Zero(t, again.Added)
		require.Equal(t, first.Records, again.Reused)
		t.Logf("%s: %d records, repeat adds %d", name, first.Records, again.Added)
	}
	result, err := s.Rebuild(ctx)
	require.NoError(t, err)
	require.Zero(t, result.Plays)
}
